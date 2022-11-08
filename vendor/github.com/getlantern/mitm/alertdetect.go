package mitm

import (
	"net"
	"sync/atomic"
)

const (
	recordTypeAlert = 21
)

// alertDetectingConn is a tls.Conn that detects if the first data written was
// a TLS alert (assumes that it definitely is a TLS record).
type alertDetectingConn struct {
	net.Conn
	initialized int32
	alerted     int32
}

// Write implements the method from net.Conn
func (adc *alertDetectingConn) Write(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}

	if atomic.CompareAndSwapInt32(&adc.initialized, 0, 1) {
		alerted := b[0] == recordTypeAlert
		if alerted {
			atomic.StoreInt32(&adc.alerted, 1)
		}
	}

	if adc.sawAlert() {
		return 0, nil
	}

	return adc.Conn.Write(b)
}

func (adc *alertDetectingConn) sawAlert() bool {
	return atomic.LoadInt32(&adc.alerted) == 1
}
