package chained

import (
	"net"
	"sync"

	"github.com/getlantern/errors"
	tls "github.com/refraction-networking/utls"
)

type helloSpec struct {
	// If id == tls.HelloCustom, sample must be non-nil. In this case, we will use
	// tls.FingerprintClientHello to generate a tls.ClientHelloSpec based on sample.
	//
	// We do this rather than store a tls.ClientHelloSpec on the hello type because (i)
	// tls.ClientHelloSpec instances cannot be re-used and (ii) it is easier to keep the sample
	// hello around than to deep-copy the tls.ClientHelloSpec on each use.
	id     tls.ClientHelloID
	sample []byte
}

// This function is guaranteed to return one of the following:
//  - A non-custom client hello ID (i.e. not utls.ClientHelloCustom)
//	- utls.ClientHelloCustom and a non-nil utls.ClientHelloSpec.
//  - An error (if the above is not possible)
func (hs helloSpec) utlsSpec() (tls.ClientHelloID, *tls.ClientHelloSpec, error) {
	const tlsRecordHeaderLen = 5

	if hs.id != tls.HelloCustom {
		return hs.id, nil, nil
	}
	if hs.sample == nil {
		return hs.id, nil, errors.New("illegal combination of HelloCustom and nil sample hello")
	}
	if len(hs.sample) < tlsRecordHeaderLen {
		return hs.id, nil, errors.New("sample hello is too small")
	}

	spec, err := tls.FingerprintClientHello(hs.sample[tlsRecordHeaderLen:])
	if err != nil {
		return hs.id, nil, errors.New("failed to fingerprint sample hello: %v", err)
	}
	return hs.id, spec, nil
}

func (hs helloSpec) uconn(transport net.Conn, cfg *tls.Config) (*tls.UConn, error) {
	id, spec, err := hs.utlsSpec()
	if err != nil {
		return nil, errors.Wrap(err)
	}
	uconn := tls.UClient(transport, cfg, id)
	if id != tls.HelloCustom {
		return uconn, nil
	}
	if err := uconn.ApplyPreset(spec); err != nil {
		return nil, errors.New("failed to apply custom hello: %v", err)
	}
	return uconn, nil
}

type helloRoller struct {
	hellos          []helloSpec
	index, advances int
	sync.Mutex
}

// Not concurrency safe.
func (hr *helloRoller) current() helloSpec {
	if len(hr.hellos) < 1 {
		panic("empty hello roller is invalid")
	}
	if hr.index >= len(hr.hellos) {
		hr.index = 0
	}
	return hr.hellos[hr.index]
}

// Not concurrency safe.
func (hr *helloRoller) advance() {
	hr.index++
	if hr.index >= len(hr.hellos) {
		hr.index = 0
	}
	hr.advances++
}

// Updates hr iff the input roller, 'other' has been advanced more than hr. It is safe to call this
// method concurrently for multiple references to hr, but not for multiple references to 'other'.
func (hr *helloRoller) updateTo(other *helloRoller) {
	hr.Lock()
	if other.advances > hr.advances {
		hr.index = other.index
		hr.advances = other.advances
	}
	hr.Unlock()
}

// Concurrency safe.
func (hr *helloRoller) getCopy() *helloRoller {
	hr.Lock()
	defer hr.Unlock()
	hellos := make([]helloSpec, len(hr.hellos))
	copy(hellos, hr.hellos)
	return &helloRoller{hellos, hr.index, hr.advances, sync.Mutex{}}
}
