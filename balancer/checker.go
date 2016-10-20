package balancer

import (
	"fmt"
	"math/rand"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/ops"
)

const (
	defaultMinCheckInterval = 10 * time.Second
	defaultMaxCheckInterval = 1 * time.Hour
)

var (
	// A fixed length list of full urls used to check this server. We would like
	// to include sites accessed by the user, but at the moment we just use a
	// canned set of popular sites.
	checkTargets = newURLList()

	// Lantern internal sites won't be used as check target.
	internalSiteSuffixes = []string{"getlantern.org", "getiantem.org", "lantern.io"}
)

type checker struct {
	b                *Balancer
	checkInterval    time.Duration
	minCheckInterval time.Duration
	maxCheckInterval time.Duration
	resetCheckCh     chan bool
	closeCh          chan bool
}

func (c *checker) runChecks() {
	checkInterval := c.minCheckInterval
	checkTimer := time.NewTimer(0) // check immediately on Start

	for {
		select {
		case <-c.closeCh:
			log.Trace("Balancer stopped")
			checkTimer.Stop()
			c.closeCh <- true
			return
		case <-c.resetCheckCh:
			checkInterval = c.minCheckInterval
			checkTimer.Reset(c.checkInterval)
		case <-checkTimer.C:
			// Obtain check data and then run checks for all using the same
			// check data. This ensures that if the specific checks vary over time,
			// we get an apples to apples comparison across all dialers.
			checkData := checkTargets.top(10)
			dialers := c.b.dialersToCheck()
			log.Debugf("Checking %d dialers using %v", len(dialers), checkData)
			var wg sync.WaitGroup
			wg.Add(len(dialers))
			for _, dialer := range dialers {
				d := dialer
				ops.Go(func() {
					c.check(d, checkData)
					wg.Done()
					c.b.forceStats()
				})
			}
			wg.Wait()
			// Exponentially back off checkInterval capped to MaxCheckInterval
			checkInterval *= 2
			if checkInterval > c.maxCheckInterval {
				checkInterval = c.maxCheckInterval
			}
			checkTimer.Reset(randomize(checkInterval))
			log.Debugf("Finished checking %d dialers", len(dialers))
		}
	}
}

func (c *checker) check(dialer *dialer, checkData interface{}) {
	if c.doCheck(dialer, checkData) {
		// If the check succeeded, we mark down a success.
		dialer.markSuccess()
		dialer.lastCheckSucceeded = true
	} else {
		if dialer.lastCheckSucceeded {
			// On first failure after success, force recheck
			dialer.forceRecheck()
		}
		dialer.lastCheckSucceeded = false
		// we call doMarkFailure so as not to trigger a recheck, since this failure
		// might just be due to the target we checked
		dialer.doMarkFailure()
	}
}

func (c *checker) doCheck(dialer *dialer, checkData interface{}) bool {
	ok, latency := dialer.Check(checkData, func(url string) {
		checkTargets.checkFailed(url)
	})
	if ok {
		dialer.markSuccess()
		oldLatency := dialer.emaLatency.Get()
		if oldLatency > 0 {
			cap := oldLatency * 2
			if latency > cap {
				// To avoid random large fluctuations in latency, keep change in latency
				// to within 2x of existing latency. If this happens, force a recheck.
				latency = cap
				dialer.forceRecheck()
			} else if latency < oldLatency/2 {
				// On major reduction in latency, force a recheck to see if this is a
				// more permanent change.
				dialer.forceRecheck()
			}
		}
		newEMA := dialer.emaLatency.UpdateWith(latency)
		log.Tracef("Updated dialer %s emaLatency to %v", dialer.Label, newEMA)
	} else {
		log.Tracef("Dialer %s failed check", dialer.Label)
	}

	return ok
}

// AddCheckTarget records another targeting of the specified address, allowing
// Lantern to run metrics on the most visited addresses.
func AddCheckTarget(addr string) {
	host, port, e := net.SplitHostPort(addr)
	if e != nil {
		log.Errorf("failed to split port from %s", addr)
		return
	}
	if port != "443" {
		log.Tracef("Skip setting non-HTTPS site %s as check target", addr)
		return
	}
	for _, s := range internalSiteSuffixes {
		if strings.HasSuffix(host, s) {
			log.Tracef("Skip setting internal site %s as check target", addr)
			return
		}
	}
	checkTargets.hit(fmt.Sprintf("https://%s/index.html", addr))
}

// adds randomization to make requests less distinguishable on the network.
func randomize(d time.Duration) time.Duration {
	return time.Duration((d.Nanoseconds() / 2) + rand.Int63n(d.Nanoseconds()))
}

type urlList struct {
	urls        map[string]int
	uncheckable map[string]bool
	mx          sync.RWMutex
}

func newURLList() *urlList {
	l := &urlList{urls: make(map[string]int, 5), uncheckable: make(map[string]bool)}
	// Prepopulate list with popular URLs
	// TODO: persist the url list to disk and use the hit counts from prior runs
	// to better match this specific user's patterns.
	l.hit("https://www.google.com/favicon.ico")
	l.hit("https://www.facebook.com/humans.txt")
	l.hit("https://www.tumblr.com/humans.txt")
	l.hit("https://www.youtube.com/robots.txt")
	return l
}

func (l *urlList) hit(url string) {
	l.mx.Lock()
	l.urls[url] = l.urls[url] + 1
	l.mx.Unlock()
}

func (l *urlList) checkFailed(url string) {
	l.mx.Lock()
	l.uncheckable[url] = true
	l.mx.Unlock()
}

func (l *urlList) top(n int) []string {
	l.mx.RLock()
	sorted := make(byCount, 0, len(l.urls))
	for url, count := range l.urls {
		if !l.uncheckable[url] {
			sorted = append(sorted, &pair{url, count})
		}
	}
	l.mx.RUnlock()
	sort.Sort(sorted)
	topN := make([]string, 0, n)
	for i := 0; i < n && i < len(sorted); i++ {
		topN = append(topN, sorted[i].url)
	}
	return topN
}

type pair struct {
	url   string
	count int
}

type byCount []*pair

func (a byCount) Len() int           { return len(a) }
func (a byCount) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byCount) Less(i, j int) bool { return a[i].count > a[j].count }
