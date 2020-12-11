package ios

import (
	"fmt"
	"io"
	"sort"
	"sync"
)

type connWithSequence struct {
	io.Closer
	sequence int64
}

func (l *connWithSequence) String() string {
	return fmt.Sprintf("%v: %d", l.Closer, l.sequence)
}

type bySequence []*connWithSequence

func (a bySequence) Len() int           { return len(a) }
func (a bySequence) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a bySequence) Less(i, j int) bool { return a[i].sequence < a[j].sequence }

func newLRUConnList() *lruConnList {
	return &lruConnList{
		conns: make(map[io.Closer]int64),
	}
}

type lruConnList struct {
	sync.RWMutex
	conns    map[io.Closer]int64
	sequence int64
}

func (l *lruConnList) mark(conn io.Closer) {
	l.Lock()
	l.conns[conn] = l.sequence
	l.sequence += 1
	l.Unlock()
}

func (l *lruConnList) remove(conn io.Closer) {
	l.Lock()
	delete(l.conns, conn)
	l.Unlock()
}

func (l *lruConnList) len() int {
	l.RLock()
	length := len(l.conns)
	l.RUnlock()
	return length
}

func (l *lruConnList) removeAll() map[io.Closer]int64 {
	l.Lock()
	conns := l.conns
	l.conns = make(map[io.Closer]int64)
	l.Unlock()
	return conns
}

func (l *lruConnList) removeOldest() (io.Closer, bool) {
	l.Lock()
	defer l.Unlock()

	if len(l.conns) == 0 {
		return nil, false
	}

	conns := make(bySequence, 0, len(l.conns))
	for conn, sequence := range l.conns {
		conns = append(conns, &connWithSequence{Closer: conn, sequence: sequence})
	}

	sort.Sort(conns)
	conn := conns[0].Closer
	delete(l.conns, conn)
	return conn, true
}
