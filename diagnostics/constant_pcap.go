package diagnostics

import (
	"io"
	"time"
)

// TrafficLog is a log of network traffic.
// TODO: document ring-buffer mechanics of captured traffic and saved captures
type TrafficLog struct{}

// NewTrafficLog returns a new TrafficLog.
func (tl TrafficLog) NewTrafficLog(addresses []string) (*TrafficLog, error) {
	// TODO: implement me!
	return nil, nil
}

// StartCapture starts packet capture on the current set of addresses. This is a blocking function.
func (tl TrafficLog) StartCapture() error {
	// TODO: implement me!
	return nil
}

// UpdateAddresses updates the addresses for which traffic is being captured.
func (tl TrafficLog) UpdateAddresses(addresses []string) error {
	// TODO: implement me!
	return nil
}

// SaveCaptures saves all captures for the given address received in the past duration d.
func (tl TrafficLog) SaveCaptures(address string, d time.Duration) error {
	// TODO: implement me!
	return nil
}

// WritePcapng writes saved captures in pcapng file format.
func (tl TrafficLog) WritePcapng(w io.Writer) error {
	// TODO: implement me!
	return nil
}
