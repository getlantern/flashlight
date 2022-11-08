package ptlshs

import (
	"bytes"
	"io"
)

// A linked list of byte slices, tailored for prepending. Not concurrency-safe.
type bufferList struct {
	// Invariants:
	//	- next and r may be nil
	//	- if next is non-nil, next.r must be non-nil
	next *bufferList
	r    *bytes.Buffer

	// Reads through the tail. Created on the first call to Read.
	cachedReader io.Reader
}

func (bl *bufferList) len() int {
	if bl.r == nil {
		return 0
	}
	len := 0
	current := bl
	for current != nil {
		len += current.r.Len()
		current = current.next
	}
	return len
}

func (bl *bufferList) prepend(b []byte) {
	if bl.r == nil {
		bl.r = bytes.NewBuffer(b)
	} else {
		*bl = bufferList{
			&bufferList{bl.next, bl.r, bl.cachedReader},
			bytes.NewBuffer(b),
			nil,
		}
	}
}

func (bl *bufferList) Read(b []byte) (n int, err error) {
	if bl.r == nil {
		return 0, io.EOF
	}
	if bl.cachedReader == nil {
		readers := []io.Reader{}
		current := bl
		for current != nil {
			readers = append(readers, current.r)
			current = current.next
		}
		bl.cachedReader = io.MultiReader(readers...)
	}
	return bl.cachedReader.Read(b)
}
