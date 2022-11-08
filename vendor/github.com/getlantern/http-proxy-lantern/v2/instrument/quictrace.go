package instrument

import (
	"context"
	"net"
	"time"

	qlog "github.com/lucas-clemente/quic-go/logging"
)

// QuicTracer is a quic-go/logging.Tracer implementation which counts the sent and
// lost packets and exports the data to Prometheus.
type QuicTracer struct {
	inst Instrument
}

func NewQuicTracer(inst Instrument) *QuicTracer {
	tracer := &QuicTracer{inst: inst}
	return tracer
}

func (t *QuicTracer) TracerForConnection(ctx context.Context, p qlog.Perspective, odcid qlog.ConnectionID) qlog.ConnectionTracer {
	return &QuicConnTracer{inst: t.inst}
}

func (t *QuicTracer) SentPacket(addr net.Addr, h *qlog.Header, size qlog.ByteCount, frames []qlog.Frame) {
}

func (t *QuicTracer) DroppedPacket(addr net.Addr, ptype qlog.PacketType, size qlog.ByteCount, reason qlog.PacketDropReason) {
	// this indicates some kind of malformed or unexpected packet from remote (vs a lost packet)
}

type QuicConnTracer struct {
	inst Instrument
}

func (t *QuicConnTracer) SentPacket(hdr *qlog.ExtendedHeader, size qlog.ByteCount, ack *qlog.AckFrame, frames []qlog.Frame) {
	t.inst.quicSentPacket()
}

func (t *QuicConnTracer) LostPacket(level qlog.EncryptionLevel, pn qlog.PacketNumber, reason qlog.PacketLossReason) {
	t.inst.quicLostPacket()
}

func (t *QuicConnTracer) StartedConnection(local, remote net.Addr, srcConnID, destConnID qlog.ConnectionID) {
}
func (t *QuicConnTracer) NegotiatedVersion(chosen qlog.VersionNumber, clientVersions, serverVersions []qlog.VersionNumber) {
}
func (t *QuicConnTracer) ClosedConnection(error)                                           {}
func (t *QuicConnTracer) SentTransportParameters(parameters *qlog.TransportParameters)     {}
func (t *QuicConnTracer) ReceivedTransportParameters(parameters *qlog.TransportParameters) {}
func (t *QuicConnTracer) RestoredTransportParameters(parameters *qlog.TransportParameters) {}
func (t *QuicConnTracer) ReceivedVersionNegotiationPacket(hdr *qlog.Header, versions []qlog.VersionNumber) {
}
func (t *QuicConnTracer) ReceivedRetry(hdr *qlog.Header) {}
func (t *QuicConnTracer) ReceivedPacket(hdr *qlog.ExtendedHeader, size qlog.ByteCount, frames []qlog.Frame) {
}
func (t *QuicConnTracer) BufferedPacket(pt qlog.PacketType) {}
func (t *QuicConnTracer) DroppedPacket(pt qlog.PacketType, size qlog.ByteCount, reason qlog.PacketDropReason) {
}
func (t *QuicConnTracer) UpdatedMetrics(rttStats *qlog.RTTStats, cwnd, bytesInFlight qlog.ByteCount, packetsInFlight int) {
}
func (t *QuicConnTracer) AcknowledgedPacket(level qlog.EncryptionLevel, pn qlog.PacketNumber)      {}
func (t *QuicConnTracer) UpdatedCongestionState(state qlog.CongestionState)                        {}
func (t *QuicConnTracer) UpdatedPTOCount(value uint32)                                             {}
func (t *QuicConnTracer) UpdatedKeyFromTLS(level qlog.EncryptionLevel, pers qlog.Perspective)      {}
func (t *QuicConnTracer) UpdatedKey(generation qlog.KeyPhase, remote bool)                         {}
func (t *QuicConnTracer) DroppedEncryptionLevel(level qlog.EncryptionLevel)                        {}
func (t *QuicConnTracer) DroppedKey(generation qlog.KeyPhase)                                      {}
func (t *QuicConnTracer) SetLossTimer(tt qlog.TimerType, level qlog.EncryptionLevel, at time.Time) {}
func (t *QuicConnTracer) LossTimerExpired(tt qlog.TimerType, level qlog.EncryptionLevel)           {}
func (t *QuicConnTracer) LossTimerCanceled()                                                       {}
func (t *QuicConnTracer) Close()                                                                   {}
func (t *QuicConnTracer) Debug(name, msg string)                                                   {}
