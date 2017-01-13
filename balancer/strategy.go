package balancer

import "math/rand"

// Strategy determines the next dialer balancer will use given various
// metrics of each dialer.
type Strategy func(dialers []*dialer) dialerHeap

// Random strategy gives even chance to each dialer, act as a baseline to other
// strategies.
func Random(dialers []*dialer) dialerHeap {
	return dialerHeap{dialers: dialers, lessFunc: func(i, j int) bool {
		// we don't need good randomness, skip seeding
		return rand.Intn(2) != 0
	}}
}

// Sticky strategy always pick the dialer with the biggest difference between
// consecutive successes and consecutive failures.
func Sticky(dialers []*dialer) dialerHeap {
	return dialerHeap{dialers: dialers, lessFunc: func(i, j int) bool {
		q1 := dialers[i].ConsecSuccesses() - dialers[i].ConsecFailures()
		q2 := dialers[j].ConsecSuccesses() - dialers[j].ConsecFailures()
		return q1 > q2
	}}
}

// Fastest strategy always pick the dialer with lowest recent average connect time
func Fastest(dialers []*dialer) dialerHeap {
	return dialerHeap{dialers: dialers, lessFunc: func(i, j int) bool {
		return faster(dialers[i], dialers[j])
	}}
}

// QualityFirst strategy behaves the same as Fastest strategy when both dialers
// are good recently, and falls back to Sticky strategy in other cases.
func QualityFirst(dialers []*dialer) dialerHeap {
	return dialerHeap{dialers: dialers, lessFunc: func(i, j int) bool {
		di, dj := dialers[i], dialers[j]
		q1 := di.ConsecSuccesses() - di.ConsecFailures()
		q2 := dj.ConsecSuccesses() - dj.ConsecFailures()
		if q1 > 0 && q2 > 0 {
			return faster(di, dj)
		}
		return q1 > q2
	}}
}

func faster(di *dialer, dj *dialer) bool {
	li, lj := di.EstimatedThroughput(), dj.EstimatedThroughput()
	return li > lj
}
