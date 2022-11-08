// Package ring provides circular data structures.
package ring

// List is a bounded circular list.
type List interface {
	// Push pushes a value to the head of the buffer
	Push(interface{})

	// IterateForward iterates forward through the values starting at the tail.
	// Iteration stops if the callback function returns false.
	IterateForward(func(interface{}) bool)

	// IterateBackward iterates backwards through the values starting at the head.
	// Iteration stops if the callback function returns false.
	IterateBackward(func(interface{}) bool)

	// Len returns the number of items in the buffer
	Len() int
}

type list struct {
	data []interface{}
	cap  int
	len  int
	head int
}

// NewList constructs a new List bounded to cap. If cap <= 0, the List will have
// a capacity of 1.
func NewList(cap int) List {
	if cap <= 0 {
		// need at least 1
		cap = 1
	}
	return &list{
		cap:  cap,
		len:  0,
		head: -1,
	}
}

func (l *list) Push(val interface{}) {
	l.head++
	if l.len < l.cap {
		l.data = append(l.data, val)
		l.len++
	} else {
		if l.head >= l.cap {
			// wrap
			l.head -= l.cap
		}
		l.data[l.head] = val
	}
}

func (l *list) IterateForward(cb func(interface{}) bool) {
	for i := l.len - 1; i >= 0; i-- {
		idx := l.head - i
		if idx < 0 {
			// wrap around
			idx += l.cap
		}
		if !cb(l.data[idx]) {
			return
		}
	}
}

func (l *list) IterateBackward(cb func(interface{}) bool) {
	if l.empty() {
		return
	}

	for i := 0; i < l.len; i++ {
		idx := l.head + i
		if idx >= l.cap {
			// wrap around
			idx -= l.cap
		}
		if !cb(l.data[idx]) {
			return
		}
	}
}

func (l *list) Len() int {
	return l.len
}

func (l *list) empty() bool {
	return l.len == 0
}
