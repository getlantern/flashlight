package congestion

import (
	"log"
	"math"
	"time"

	"github.com/lucas-clemente/quic-go/internal/protocol"
)

type ConnectionStateOnSentPacket struct {
	sentTime                         time.Time
	size                             protocol.ByteCount
	totalBytesSent                   protocol.ByteCount
	totalBytesSentAtLastAckedPacket  protocol.ByteCount
	totalBytesAckedAtLastAckedPacket protocol.ByteCount
	lastAckedPacketSentTime          time.Time
	lastAckedPacketAckTime           time.Time
	isAppLimited                     bool
}

func NewConnectionStateOnSentPacket(sentTime time.Time, size protocol.ByteCount, sampler *BandwidthSampler) (cs *ConnectionStateOnSentPacket) {
	return &ConnectionStateOnSentPacket{
		sentTime:                         sentTime,
		size:                             size,
		totalBytesSent:                   sampler.totalBytesSent,
		totalBytesSentAtLastAckedPacket:  sampler.totalBytesSentAtLastAckedPacket,
		lastAckedPacketSentTime:          sampler.lastAckedPacketSentTime,
		lastAckedPacketAckTime:           sampler.lastAckedPacketAckTime,
		totalBytesAckedAtLastAckedPacket: sampler.totalBytesAcked,
		isAppLimited:                     sampler.isAppLimited,
	}
}

type BandwidthSampler struct {
	connectionStateMap              map[protocol.PacketNumber]*ConnectionStateOnSentPacket
	totalBytesSent                  protocol.ByteCount
	totalBytesAcked                 protocol.ByteCount
	totalBytesSentAtLastAckedPacket protocol.ByteCount
	lastAckedPacketSentTime         time.Time
	lastAckedPacketAckTime          time.Time
	lastSentPacket                  protocol.PacketNumber
	isAppLimited                    bool
	endOfAppLimitedPhase            protocol.PacketNumber
}

type BandwidthSample struct {
	Bandwidth    protocol.ByteCount
	RTT          time.Duration
	IsAppLimited bool
}

func (s *BandwidthSampler) OnPacketSent(sentTime time.Time,
	packetNumber protocol.PacketNumber,
	bytes protocol.ByteCount,
	bytesInFlight protocol.ByteCount,
	HasRetransmittableData bool) {
	//
	s.lastSentPacket = packetNumber
	// log.Printf("£ the size of connectionStateMap is %#v", s.connectionStateMap)

	if !HasRetransmittableData {
		return
	}

	s.totalBytesSent += bytes

	// If there are no packets in flight, the time at which the new transmission
	// opens can be treated as the A_0 point for the purpose of bandwidth
	// sampling. This underestimates bandwidth to some extent, and produces some
	// artificially low samples for most packets in flight, but it provides with
	// samples at important points where we would not have them otherwise, most
	// importantly at the beginning of the connection.
	if bytesInFlight == 0 {
		s.lastAckedPacketAckTime = sentTime
		s.totalBytesSentAtLastAckedPacket = s.totalBytesSent

		// In this situation ack compression is not a concern, set send rate to
		// effectively infinite.
		s.lastAckedPacketSentTime = sentTime
	}

	sample := NewConnectionStateOnSentPacket(sentTime, bytes, s)
	if s.connectionStateMap == nil {
		s.connectionStateMap = make(map[protocol.PacketNumber]*ConnectionStateOnSentPacket)
	}

	s.connectionStateMap[packetNumber] = sample
	if len(s.connectionStateMap) > 10000 {
		log.Printf("Warning: via BBR, connectionStateMap size is large (10k+)")
	}
}

func (s *BandwidthSampler) OnPacketAcknowledged(ackTime time.Time,
	packetNumber protocol.PacketNumber) BandwidthSample {
	//
	if s.connectionStateMap == nil {
		s.connectionStateMap = make(map[protocol.PacketNumber]*ConnectionStateOnSentPacket)
	}

	sentPacketState := s.connectionStateMap[packetNumber]
	if sentPacketState == nil {
		return BandwidthSample{
			Bandwidth:    0,
			RTT:          -1,
			IsAppLimited: false,
		}
	}

	sample := s.OnPacketAcknowledgedInner(ackTime, packetNumber, sentPacketState)
	delete(s.connectionStateMap, packetNumber)
	return sample
}

func (s *BandwidthSampler) OnPacketAcknowledgedInner(ackTime time.Time,
	packetNumber protocol.PacketNumber,
	sentPacketState *ConnectionStateOnSentPacket) BandwidthSample {
	//

	s.totalBytesAcked += sentPacketState.size
	s.totalBytesSentAtLastAckedPacket = sentPacketState.totalBytesSent
	s.lastAckedPacketSentTime = sentPacketState.sentTime
	s.lastAckedPacketAckTime = ackTime

	// Exit app-limited phase once a packet that was sent while the connection is
	// not app-limited is acknowledged.
	if s.isAppLimited && packetNumber > s.endOfAppLimitedPhase {
		s.isAppLimited = false
	}

	// There might have been no packets acknowledged at the moment when the
	// current packet was sent. In that case, there is no bandwidth sample to
	// make.
	if sentPacketState.lastAckedPacketSentTime.IsZero() {
		return BandwidthSample{}
	}

	// Infinite rate indicates that the sampler is supposed to discard the
	// current send rate sample and use only the ack rate.
	sendRate := Bandwidth(math.MaxUint64)
	if sentPacketState.sentTime.UnixNano() > sentPacketState.lastAckedPacketSentTime.UnixNano() {
		sendRate = BandwidthFromDelta(sentPacketState.totalBytesSent-sentPacketState.totalBytesSentAtLastAckedPacket,
			sentPacketState.sentTime.Sub(sentPacketState.lastAckedPacketSentTime))
	}

	// During the slope calculation, ensure that ack time of the current packet is
	// always larger than the time of the previous packet, otherwise division by
	// zero or integer underflow can occur.
	if ackTime.UnixNano() <= sentPacketState.lastAckedPacketSentTime.UnixNano() {
		log.Printf("bandwidth_sampler bug: Time of the previously acked is larger than the time of the current packet")
		return BandwidthSample{}
	}
	ackRate := BandwidthFromDelta(s.totalBytesAcked-sentPacketState.totalBytesAckedAtLastAckedPacket,
		ackTime.Sub(sentPacketState.lastAckedPacketAckTime))

	sam := BandwidthSample{}
	sam.Bandwidth = protocol.ByteCount(sendRate)
	if sam.Bandwidth > protocol.ByteCount(ackRate) {
		sam.Bandwidth = protocol.ByteCount(ackRate)
	}
	// Note: this sample does not account for delayed acknowledgement time.  This
	// means that the RTT measurements here can be artificially high, especially
	// on low bandwidth connections.
	sam.RTT = ackTime.Sub(sentPacketState.sentTime)
	// A sample is app-limited if the packet was sent during the app-limited
	// phase.
	sam.IsAppLimited = sentPacketState.isAppLimited
	return sam
}

func (s *BandwidthSampler) OnPacketLost(packetNumber protocol.PacketNumber) {
	//
	delete(s.connectionStateMap, packetNumber)
}

func (s *BandwidthSampler) OnAppLimited() {
	//
	s.isAppLimited = true
	s.endOfAppLimitedPhase = s.lastSentPacket
}

func (s *BandwidthSampler) RemoveObsoletePackets(leastUnacked protocol.PacketNumber) {
	//
	for pn := range s.connectionStateMap {
		if pn < leastUnacked {
			// delete(s.connectionStateMap, pn)
			// log.Printf("£ RemoveObsoletePackets removed packetNumber %v", pn)
		}
	}
}

func (s *BandwidthSampler) LeastUnacked() protocol.PacketNumber {
	// We don't use a unacked_map like chrome does, so i'm going to
	// "yolo" it and see if it works close enough
	lowest := protocol.PacketNumber(1)

	for pn := range s.connectionStateMap {
		if pn > lowest {
			lowest = pn
		}
	}
	return lowest
}

func (s *BandwidthSampler) TotalBytesAcked() protocol.ByteCount {
	//
	return s.totalBytesAcked
}

func (s *BandwidthSampler) IsAppLimited() bool {
	//
	return s.isAppLimited
}

func (s *BandwidthSampler) EndOfAppLimitedPahse() protocol.PacketNumber {
	//
	return s.endOfAppLimitedPhase
}
