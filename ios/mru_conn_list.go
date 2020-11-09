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

type bySequenceReversed []*connWithSequence

func (a bySequenceReversed) Len() int           { return len(a) }
func (a bySequenceReversed) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a bySequenceReversed) Less(i, j int) bool { return a[i].sequence > a[j].sequence }

func newMRUConnList() *mruConnList {
	return &mruConnList{
		conns: make(map[io.Closer]int64),
	}
}

type mruConnList struct {
	sync.RWMutex
	conns    map[io.Closer]int64
	sequence int64
}

func (l *mruConnList) mark(conn io.Closer) {
	l.Lock()
	l.conns[conn] = l.sequence
	l.sequence += 1
	l.Unlock()
}

func (l *mruConnList) remove(conn io.Closer) {
	l.Lock()
	delete(l.conns, conn)
	l.Unlock()
}

func (l *mruConnList) len() int {
	l.RLock()
	length := len(l.conns)
	l.RUnlock()
	return length
}

func (l *mruConnList) removeAll() map[io.Closer]int64 {
	l.Lock()
	conns := l.conns
	l.conns = make(map[io.Closer]int64)
	l.Unlock()
	return conns
}

func (l *mruConnList) removeNewest() (io.Closer, bool) {
	l.RLock()
	conns := make(bySequenceReversed, 0, len(l.conns))
	for conn, sequence := range l.conns {
		conns = append(conns, &connWithSequence{Closer: conn, sequence: sequence})
	}
	l.RUnlock()

	if len(conns) == 0 {
		return nil, false
	}

	sort.Sort(conns)
	conn := conns[0].Closer
	l.remove(conn)
	return conn, true
}
