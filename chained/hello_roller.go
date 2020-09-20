package chained

import (
	"sync"

	tls "github.com/refraction-networking/utls"
)

type hello struct {
	// Most users of a hello will ignore spec if id is not tls.HelloCustom.
	id   tls.ClientHelloID
	spec *tls.ClientHelloSpec
}

type helloRoller struct {
	hellos          []hello
	index, advances int
	sync.Mutex
}

// Not concurrency safe.
func (hr *helloRoller) current() *hello {
	if len(hr.hellos) < 1 {
		return nil
	}
	if hr.index >= len(hr.hellos) {
		hr.index = 0
	}
	return &hr.hellos[hr.index]
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
	hellos := make([]hello, len(hr.hellos))
	copy(hellos, hr.hellos)
	return &helloRoller{hellos, hr.index, hr.advances, sync.Mutex{}}
}
