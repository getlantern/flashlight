package congestion

import (
	"math"
	"math/rand"
	"time"

	"github.com/lucas-clemente/quic-go/internal/protocol"
	"github.com/lucas-clemente/quic-go/internal/utils"
)

type bbrMode uint8

const (
	BBR_INVALID   = iota
	BBR_STARTUP   /* ramp up sending rate rapidly to fill pipe */
	BBR_DRAIN     /* drain any queue created during startup */
	BBR_PROBE_BW  /* discover, share bw: pace around estimated bw */
	BBR_PROBE_RTT /* cut inflight to min to probe min_rtt */
)

type bbrRecoveryState uint8

const (
	// No limits
	NOT_IN_RECOVERY = iota
	// Allow an extra outstanding byte for each byte acknowledged
	CONSERVATION
	// Allow two extra outstanding bytes for each byte acknowledged (slow
	// start).
	GROWTH
)

const (
	bbrHighGain                                    = 2.885
	bbrDrainGain                                   = 1.0 / bbrHighGain
	bbrGainCycleLength                             = 8
	bbrBandwidthWindowSize                         = bbrGainCycleLength + 2
	bbrMinRTTExpiry                                = time.Second * 10
	bbrProbeRttTime                                = time.Millisecond * 200
	bbrStartupGrowthTarget                         = 1.25
	bbrRoundTripsWithoutGrowthBeforeExitingStartup = 3
)

var (
	bbrPacingGain = []float64{1.25, 0.75, 1, 1, 1, 1, 1, 1}
)

type bbrInflightTracker map[protocol.PacketNumber]bool

func (t bbrInflightTracker) Count() int {
	n := 0
	for _, v := range t {
		if v {
			n++
		}
	}
	return n
}

type bbrSender struct {
	mode                         bbrMode
	rateBasedRecovery            bool
	congestionWindow             protocol.ByteCount
	recoveryWindow               protocol.ByteCount
	recoveryState                bbrRecoveryState
	sampler                      BandwidthSampler
	lastAckedPacket              protocol.PacketNumber
	lastSentPacket               protocol.PacketNumber
	currentRoundTripEnd          protocol.PacketNumber
	roundTripCount               int
	lastSampleIsAppLimited       bool
	maxBandwidth                 *windowFilter
	maxAckHeight                 *windowFilter
	minRTTTimestamp              time.Time
	minRTT                       time.Duration
	endRecoveryAt                protocol.PacketNumber
	aggregationEpochStartTime    time.Time
	aggregationEpochBytes        protocol.ByteCount
	inFlightPackets              bbrInflightTracker
	lastCycleStart               time.Time
	pacingGain                   float64
	pacingRate                   protocol.ByteCount
	congestionWindowGain         float64
	congestionWindowGainConstant float64
	initialCongestionWindow      protocol.ByteCount
	cycleCurrentOffset           int
	isAtFullBandwidth            bool
	bandwidthAtLastRound         protocol.ByteCount
	roundsWithoutBandwidthGain   int
	numStartupRtts               int
	exitStartupOnLoss            bool
	exitingQuiescence            bool
	exitProbeRTTat               time.Time
	rttStats                     *utils.RTTStats
	rttVarianceWeight            float64 // no internal writers, Good for tweaking perf though
	probeRTTRoundPassed          bool
	minCongestionWindow          protocol.ByteCount // Needs init
	maxCongestionWindow          protocol.ByteCount // Needs init
	pacer                        *pacer

	// maxAggergationBytesMultiplier float64 // Unused, No need to implement

}

func NewBBRSender(rs *utils.RTTStats, initialMaxDatagramSize protocol.ByteCount) (b *bbrSender) {

	bbrMinimumCongestionWindow := protocol.ByteCount(initialMaxDatagramSize * 4)

	b = &bbrSender{
		maxAckHeight: &windowFilter{
			MaxOrMinFilter: false,
		},
		maxBandwidth: &windowFilter{
			MaxOrMinFilter: true,
		},
		inFlightPackets:              make(bbrInflightTracker),
		rttStats:                     rs,
		mode:                         BBR_STARTUP,
		congestionWindow:             bbrMinimumCongestionWindow,
		congestionWindowGain:         1,
		initialCongestionWindow:      bbrMinimumCongestionWindow,
		minCongestionWindow:          bbrMinimumCongestionWindow,
		maxCongestionWindow:          5000 * 1024,
		congestionWindowGainConstant: 2.0,
	}
	b.pacer = newPacer(b.bandwidthEstimateForPacer)

	return
}

func (b *bbrSender) TimeUntilSend(bytesInFlight protocol.ByteCount) time.Time {
	return b.pacer.TimeUntilSend()
}

func (b *bbrSender) HasPacingBudget() bool {
	return b.pacer.Budget(time.Now()) >= maxDatagramSize
}

func (b *bbrSender) OnPacketSent(sentTime time.Time, bytesInFlight protocol.ByteCount, packetNumber protocol.PacketNumber, bytes protocol.ByteCount, isRetransmittable bool) {
	b.lastSentPacket = packetNumber

	if bytesInFlight == 0 && b.sampler.isAppLimited {
		b.exitingQuiescence = true
	}

	if !b.aggregationEpochStartTime.IsZero() {
		b.aggregationEpochStartTime = sentTime
	}

	b.sampler.OnPacketSent(sentTime, packetNumber, bytes, bytesInFlight, isRetransmittable)
}

func (b *bbrSender) CanSend(bytesInFlight protocol.ByteCount) bool {
	return bytesInFlight < b.GetCongestionWindow()
}

func (b *bbrSender) MaybeExitSlowStart() {
	// Nothing to do here, does get called though when the RTT gets updated. so maybe that's a useful stat
}

func (b *bbrSender) OnPacketAcked(lastPacketNumber protocol.PacketNumber, ackedBytes protocol.ByteCount, priorInFlight protocol.ByteCount, eventTime time.Time) {

	totalBytesAckedBefore := b.sampler.totalBytesAcked

	isRoundStart := b.updateRoundTripCounter(lastPacketNumber)
	minRttExpired := b.updateBandwidthAndMinRtt(eventTime, lastPacketNumber)
	b.updateRecoveryState(lastPacketNumber, false /*TODO!*/, isRoundStart)

	// Unclear what changes between totalBytesAcked and totalBytesAckedBefore
	// I Assume it's hidden inside the call stack
	bytesAcked := b.sampler.totalBytesAcked - totalBytesAckedBefore

	b.updateAckAggregationBytes(eventTime, bytesAcked)
	b.onCongestionEvent(lastPacketNumber, ackedBytes, math.MaxInt64, priorInFlight, eventTime, isRoundStart, minRttExpired)
}

func (b *bbrSender) SetMaxDatagramSize(sz protocol.ByteCount) {
	// According to the cubic implementation this can only legally go up...
	// (it panics if it goes down)
	b.congestionWindow = sz
}

func (b *bbrSender) updateAckAggregationBytes(ackTime time.Time, bytesAcked protocol.ByteCount) {
	// Compute how many bytes are expected to be delivered, assuming max bandwidth
	// is correct.
	period := ackTime.Sub(b.aggregationEpochStartTime)

	expectedBytesAcked := int64(period.Seconds() * float64(b.maxBandwidth.GetBest()))
	// expectedBytesAcked is now bytes per second... I think

	if b.aggregationEpochBytes <= protocol.ByteCount(expectedBytesAcked) {
		// Reset to start measuring a new aggregation epoch.
		b.aggregationEpochBytes = bytesAcked
		b.aggregationEpochStartTime = ackTime
		return
	}

	// Compute how many extra bytes were delivered vs max bandwidth.
	// Include the bytes most recently acknowledged to account for stretch acks.
	b.aggregationEpochBytes += bytesAcked
	b.maxAckHeight.Update(int64(b.aggregationEpochBytes-protocol.ByteCount(expectedBytesAcked)),
		time.Unix(0, int64(b.roundTripCount))) // TODO: fix the window filter to accept just int64 times
}

func (b *bbrSender) updateRecoveryState(lastAckedPacket protocol.PacketNumber, hasLosses, isRoundStart bool) {
	// Exit recovery when there are no losses for a round.
	if hasLosses {
		b.endRecoveryAt = b.lastSentPacket
	}

	switch b.recoveryState {

	case NOT_IN_RECOVERY:
		if hasLosses {
			b.recoveryState = CONSERVATION
			// This will cause the |recovery_window_| to be set to the correct
			// value in CalculateRecoveryWindow().
			b.recoveryWindow = 0
			// Since the conservation phase is meant to be lasting for a whole
			// round, extend the current round as if it were started right now.
			b.currentRoundTripEnd = b.lastSentPacket
		}
		break

	case CONSERVATION:
		if isRoundStart {
			b.recoveryState = GROWTH
		}
		// Unclear if there should be a break here or not... It's that way in proto-quic,
		// so I assume this is just sneaky behaviour.

	case GROWTH:
		if !hasLosses && lastAckedPacket > b.endRecoveryAt {
			b.recoveryState = NOT_IN_RECOVERY
		}
		break
	}
}

func (b *bbrSender) updateBandwidthAndMinRtt(eventTime time.Time, lastAckedPacket protocol.PacketNumber) bool {

	bSample := b.sampler.OnPacketAcknowledged(eventTime, lastAckedPacket)
	b.lastSampleIsAppLimited = bSample.IsAppLimited
	if bSample.RTT == 0 {
		return false // If the sample is  not valid, we can't safely calculate any of this.
	}

	if !bSample.IsAppLimited ||
		bSample.Bandwidth > b.bandwidthEstimate() {
		//
		b.maxBandwidth.Update(
			int64(bSample.Bandwidth),
			time.Unix(int64(b.roundTripCount), 0)) // TODO: Port maxBandwidth.Update to be ints only, this is dumb

	}

	// If none of the RTT samples are valid, return immediately.
	if bSample.RTT == -1 {
		return false
	}

	minRTTexpired := (bSample.RTT != 0) &&
		(eventTime.UnixNano() > (b.minRTTTimestamp.UnixNano() + bbrMinRTTExpiry.Nanoseconds()))

	if minRTTexpired || bSample.RTT < b.minRTT || b.minRTT == 0 {
		b.minRTT = bSample.RTT
		b.minRTTTimestamp = eventTime
	}

	return minRTTexpired
}

func (b *bbrSender) BandwidthEstimate() Bandwidth {
	/* Duplicated for the possiblity of swapping it out with ack bytes rate */
	return Bandwidth(b.maxBandwidth.GetBest())
}

func (b *bbrSender) bandwidthEstimate() protocol.ByteCount {
	return protocol.ByteCount(b.maxBandwidth.GetBest())
}

func (b *bbrSender) bandwidthEstimateForPacer() Bandwidth {
	return Bandwidth(b.maxBandwidth.GetBest())
}

func (b *bbrSender) updateRoundTripCounter(lastAckedPacket protocol.PacketNumber) bool {
	if lastAckedPacket > b.currentRoundTripEnd {
		b.roundTripCount++
		b.currentRoundTripEnd = b.lastSentPacket
		return true
	}

	return false
}

func (b *bbrSender) OnPacketLost(number protocol.PacketNumber, lostBytes protocol.ByteCount, priorInFlight protocol.ByteCount) {
	b.sampler.OnPacketLost(number)
	b.onCongestionEvent(number, math.MaxInt64, lostBytes, priorInFlight, time.Now(), false, false)
}

func (b *bbrSender) onCongestionEvent(number protocol.PacketNumber,
	ackedBytes protocol.ByteCount,
	lostBytes protocol.ByteCount,
	priorInFlight protocol.ByteCount,
	eventTime time.Time,
	isRoundStart, minRttExpired bool) {
	//

	if b.mode == BBR_PROBE_BW {
		b.updateGainCyclePhase(eventTime, priorInFlight, lostBytes != math.MaxInt64)
	}

	// Handle logic specific to STARTUP and DRAIN modes.
	if isRoundStart && !b.isAtFullBandwidth {
		b.checkIfFullBandwidthReached()
	}
	b.maybeExitStartupOrDrain(eventTime)

	// Handle logic specific to PROBE_RTT.
	b.maybeEnterOrExitProbeRtt(eventTime, isRoundStart, minRttExpired, priorInFlight)

	// Calculate number of packets acked and lost.
	// ackedBytes
	// lostBytes

	// After the model is updated, recalculate the pacing rate and congestion
	// window.
	b.calculatePacingRate()
	b.calculateCongestionWindow(ackedBytes)
	b.calculateRecoveryWindow(ackedBytes, lostBytes, priorInFlight)

	// Cleanup internal state.
	b.sampler.RemoveObsoletePackets(b.sampler.LeastUnacked())
}

func (b *bbrSender) calculateRecoveryWindow(ackedBytes, lostBytes protocol.ByteCount, priorInFlight protocol.ByteCount) {
	if b.rateBasedRecovery {
		return
	}

	if b.recoveryState == NOT_IN_RECOVERY {
		return
	}

	// Set up the initial recovery window.
	if b.recoveryWindow == 0 {
		b.recoveryWindow = priorInFlight + ackedBytes
		if b.minCongestionWindow > b.recoveryWindow {
			b.recoveryWindow = b.minCongestionWindow
			return
		}
	}

	// Remove losses from the recovery window, while accounting for a potential
	// integer underflow.
	if b.recoveryWindow >= lostBytes {
		b.recoveryWindow = b.recoveryWindow - lostBytes
	} else {
		b.recoveryWindow = maxDatagramSize
	}

	if b.recoveryState == GROWTH {
		b.recoveryWindow += ackedBytes
	}

	// Sanity checks.  Ensure that we always allow to send at least
	// |bytes_acked| in response.
	//   recovery_window_ = std::max(
	// 	recovery_window_, unacked_packets_->bytes_in_flight() + bytes_acked);
	if b.minCongestionWindow > b.recoveryWindow {
		b.recoveryWindow = b.minCongestionWindow
	}
}

func (b *bbrSender) calculateCongestionWindow(ackedBytes protocol.ByteCount) {
	if b.mode == BBR_PROBE_RTT {
		return
	}

	targetWindow := b.getTargetCongestionWindow(b.congestionWindowGain)
	if b.rttVarianceWeight > 0 && (b.bandwidthEstimate() != 0) {
		targetWindow += protocol.ByteCount(b.rttVarianceWeight) * protocol.ByteCount(b.rttStats.MeanDeviation()) * b.bandwidthEstimate()
		// Likely at least 1 bug here
	} else if b.isAtFullBandwidth {
		targetWindow += protocol.ByteCount(b.maxAckHeight.GetBest())
	}

	// Instead of immediately setting the target CWND as the new one, BBR grows
	// the CWND towards |target_window| by only increasing it |bytes_acked| at a
	// time.
	if b.isAtFullBandwidth {
		if targetWindow > b.congestionWindow+ackedBytes {
			b.congestionWindow = b.congestionWindow + ackedBytes
		}
		b.congestionWindow = targetWindow
	} else if b.congestionWindow < targetWindow ||
		b.sampler.totalBytesAcked < b.initialCongestionWindow {
		// If the connection is not yet out of startup phase, do not decrease the
		// window.
		b.congestionWindow = b.congestionWindow + ackedBytes
	}

	// Enforce the limits on the congestion window.

	// congestion_window_ = std::max(congestion_window_, kMinimumCongestionWindow);
	if b.congestionWindow < b.minCongestionWindow {
		b.congestionWindow = b.minCongestionWindow
	}

	// congestion_window_ = std::min(congestion_window_, max_congestion_window_)£ mode;
	if b.congestionWindow > b.maxCongestionWindow {
		b.congestionWindow = b.maxCongestionWindow
	}

}

func (b *bbrSender) calculatePacingRate() {
	if b.bandwidthEstimate() == 0 {
		return
	}

	targetRate := protocol.ByteCount(b.pacingGain * float64(b.bandwidthEstimate()))

	if b.rateBasedRecovery && b.InRecovery() {
		b.pacingRate = protocol.ByteCount(b.pacingGain * float64(b.maxBandwidth.GetThirdBest()))
	}

	if b.isAtFullBandwidth {
		b.pacingRate = targetRate
	}
	// Pace at the rate of initial_window / RTT as soon as RTT measurements are
	// available.

	if b.pacingRate == 0 && b.rttStats.MinRTT() != 0 {
		// b.pacingRate = b.initialCongestionWindow
		b.pacingRate = protocol.ByteCount(float64(b.initialCongestionWindow) / b.rttStats.MinRTT().Seconds())
		// ^ Aka bytes per second
		return
	}

	if b.pacingRate < targetRate {
		b.pacingRate = targetRate
	}
	return
}

func (b *bbrSender) maybeEnterOrExitProbeRtt(eventTime time.Time, isRoundStart, minRttExpired bool, priorInFlight protocol.ByteCount) {
	if minRttExpired && b.exitingQuiescence && b.mode != BBR_PROBE_RTT {
		b.mode = BBR_PROBE_RTT
		b.pacingGain = 1
		// Do not decide on the time to exit PROBE_RTT until the |bytes_in_flight|
		// is at the target small value.
		b.exitProbeRTTat = time.Time{}
	}

	if b.mode == BBR_PROBE_RTT {
		b.sampler.OnAppLimited()

		if b.exitProbeRTTat.IsZero() {
			// If the window has reached the appropriate size, schedule exiting
			// PROBE_RTT.  The CWND during PROBE_RTT is kMinimumCongestionWindow, but
			// we allow an extra packet since QUIC checks CWND before sending a
			// packet.
			if priorInFlight < b.minCongestionWindow+maxDatagramSize {
				b.exitProbeRTTat = time.Now().Add(bbrProbeRttTime)
				b.probeRTTRoundPassed = false
			}
		} else {
			if isRoundStart {
				b.probeRTTRoundPassed = true
			}

			if eventTime.UnixNano() >= b.exitProbeRTTat.UnixNano() && b.probeRTTRoundPassed {
				b.minRTTTimestamp = eventTime
				if !b.isAtFullBandwidth {
					b.enterStartupMode()
				} else {
					b.enterProbeBandwidthMode(eventTime)
				}
			}

		}

	}
	b.exitingQuiescence = false
}

func (b *bbrSender) enterStartupMode() {
	b.mode = BBR_STARTUP
	b.pacingGain = bbrHighGain
	b.congestionWindowGain = bbrHighGain
}

func (b *bbrSender) enterProbeBandwidthMode(eventTime time.Time) {
	b.mode = BBR_PROBE_BW
	b.congestionWindowGain = b.congestionWindowGainConstant

	// Pick a random offset for the gain cycle out of {0, 2..7} range. 1 is
	// excluded because in that case increased gain and decreased gain would not
	// follow each other.
	// cycle_current_offset_ = random_->RandUint64() % (kGainCycleLength - 1);
	b.cycleCurrentOffset = rand.Intn(bbrGainCycleLength - 1)
	if b.cycleCurrentOffset >= 1 {
		b.cycleCurrentOffset++
	}

	b.lastCycleStart = eventTime
	b.pacingGain = bbrPacingGain[b.cycleCurrentOffset]
}

func (b *bbrSender) maybeExitStartupOrDrain(eventTime time.Time) {
	if b.mode == BBR_STARTUP && b.isAtFullBandwidth {
		b.mode = BBR_DRAIN
		b.pacingGain = bbrDrainGain
		b.congestionWindowGain = bbrHighGain
	}

	if b.mode == BBR_DRAIN /* && unacked_packets_->bytes_in_flight() <= GetTargetCongestionWindow(1) */ {
		b.enterProbeBandwidthMode(eventTime)
	}
}

func (b *bbrSender) checkIfFullBandwidthReached() {
	if b.lastSampleIsAppLimited {
		return
	}

	target := protocol.ByteCount(float64(b.bandwidthAtLastRound) * bbrStartupGrowthTarget)
	if b.bandwidthEstimate() >= target {
		b.bandwidthAtLastRound = b.bandwidthEstimate()
		b.roundsWithoutBandwidthGain = 0
		return
	}

	b.roundsWithoutBandwidthGain++
	if b.roundsWithoutBandwidthGain >= b.numStartupRtts ||
		(b.exitStartupOnLoss && b.InRecovery()) {
		b.isAtFullBandwidth = true
	}

}

func (b *bbrSender) getMinRTT() time.Duration {
	if b.minRTT == 0 {
		return b.rttStats.MinRTT()
	}
	return b.minRTT
}

func (b *bbrSender) updateGainCyclePhase(eventTime time.Time, priorInFlight protocol.ByteCount, hasLosses bool) {
	// In most cases, the cycle is advanced after an RTT passes.
	shouldAdvanceGainCycling := eventTime.Sub(b.lastCycleStart) > b.getMinRTT()

	// If the pacing gain is above 1.0, the connection is trying to probe the
	// bandwidth by increasing the number of bytes in flight to at least
	// pacing_gain * BDP.  Make sure that it actually reaches the target, as long
	// as there are no losses suggesting that the buffers are not able to hold
	// that much.
	if b.pacingGain > 1.0 && !hasLosses &&
		priorInFlight < b.getTargetCongestionWindow(b.pacingGain) {
		shouldAdvanceGainCycling = false
	}

	// If pacing gain is below 1.0, the connection is trying to drain the extra
	// queue which could have been incurred by probing prior to it.  If the number
	// of bytes in flight falls down to the estimated BDP value earlier, conclude
	// that the queue has been successfully drained and exit this cycle early.
	if b.pacingGain < 1.0 && priorInFlight <= b.getTargetCongestionWindow(1) {
		shouldAdvanceGainCycling = true
	}

	if shouldAdvanceGainCycling {
		b.lastCycleStart = eventTime
		b.cycleCurrentOffset = (b.cycleCurrentOffset + 1) % bbrGainCycleLength
		b.pacingGain = bbrPacingGain[b.cycleCurrentOffset]
	}
}

func (b *bbrSender) getTargetCongestionWindow(gain float64) protocol.ByteCount {
	bdp := protocol.ByteCount(float64(b.getMinRTT().Seconds()) * float64(b.maxBandwidth.GetBest())) // in bytes per second
	congestionWindow := protocol.ByteCount(gain * float64(bdp))

	if congestionWindow == 0 {
		congestionWindow = protocol.ByteCount(float64(gain) * float64(b.initialCongestionWindow))
	}

	if congestionWindow > b.minCongestionWindow {
		return congestionWindow
	}
	return b.minCongestionWindow
}

func (b *bbrSender) OnRetransmissionTimeout(packetsRetransmitted bool) {
	// ./shrug - not a signal we look for
}

func (b *bbrSender) InSlowStart() bool {
	return b.mode == BBR_STARTUP
}

func (b *bbrSender) InRecovery() bool {
	return b.recoveryState != NOT_IN_RECOVERY
}

func (b *bbrSender) GetCongestionWindow() protocol.ByteCount {
	if b.mode == BBR_PROBE_RTT {
		return b.minCongestionWindow
	}

	if b.InRecovery() && !b.rateBasedRecovery {
		if b.congestionWindow > b.recoveryWindow {
			return b.recoveryWindow
		}
		return b.recoveryWindow
	}

	return b.congestionWindow
}
