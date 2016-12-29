package balancer

import (
	"fmt"
	"math"
	"math/rand"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/ops"
	"github.com/getlantern/reachability"
)

const (
	reachabilityInterval = 5 * time.Second
	minCheckInterval     = 10 * time.Second
	maxCheckInterval     = 1 * time.Hour

	avoidDivideByZero = 0.000001
)

var (
	// Targets to use for testing reachability
	reachabilityTargets = []string{"aws.amazon.com", "www.microsoft.com", "www.Hsbc.com.hk"}

	// A fixed length list of full urls used to check this server. We would like
	// to include sites accessed by the user, but at the moment we just use a
	// canned set of popular sites.
	checkTargets = newURLList()

	// Lantern internal sites won't be used as check target.
	internalSiteSuffixes = []string{"getlantern.org", "getiantem.org", "lantern.io"}
)

type checker struct {
	b             *Balancer
	checkInterval time.Duration
	resetCheckCh  chan time.Time
	closeCh       chan bool
}

func (c *checker) runChecks() {
	checkReachability := reachability.NewChecker(2, 200*time.Millisecond, 5*time.Second, reachabilityTargets...)
	priorPLR := float64(0)                             // assume no packet loss to start
	emaRTT := newEMADuration(30*time.Millisecond, 0.5) // assume fast initially, large alpha to reflect changes quickly
	reachabilityTimer := time.NewTimer(0)              // check reachability immediately

	checkInterval := minCheckInterval
	checkTimer := time.NewTimer(0) // check immediately on Start

	var lastFinishedRecheck time.Time

	runChecks := func() {
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
			})
		}
		wg.Wait()
		// Exponentially back off checkInterval capped to MaxCheckInterval
		checkInterval *= 2
		if checkInterval > maxCheckInterval {
			checkInterval = maxCheckInterval
		}
		nextCheck := randomize(checkInterval)
		checkTimer.Reset(nextCheck)
		log.Debugf("Finished checking %d dialers, next check: %v", len(dialers), nextCheck)
		c.b.forceStats()
	}

	for {
		select {
		case <-c.closeCh:
			log.Trace("Balancer stopped")
			checkTimer.Stop()
			c.closeCh <- true
			return
		case <-reachabilityTimer.C:
			rtt, plr := checkReachability()
			log.Debugf("RTT: %v  PLR: %v", rtt, plr)
			priorRTT := emaRTT.Get()
			bigChangeInRTT := math.Abs(float64(rtt-priorRTT)/(float64(priorRTT)+avoidDivideByZero)) > 2
			bigChangeInPLR := math.Abs((plr-priorPLR)/(priorPLR+avoidDivideByZero)) > 2
			dramaticReachabilityChange := bigChangeInPLR || bigChangeInRTT
			emaRTT.UpdateWith(rtt)
			priorPLR = plr
			if dramaticReachabilityChange {
				// The purpose of this is to deal with situations like temporary
				// connectivity losses, temporary network congestion, etc.
				log.Debug("Dramatic change in reachability, force recheck")
				c.b.forceRecheck()
			}
			reachabilityTimer.Reset(randomize(reachabilityInterval))
		case recheckAt := <-c.resetCheckCh:
			if recheckAt.After(lastFinishedRecheck) {
				log.Debug("Forced recheck")
				checkInterval = minCheckInterval
				runChecks()
				lastFinishedRecheck = time.Now()
			}
		case <-checkTimer.C:
			runChecks()
		}
	}
}

func (c *checker) check(dialer *dialer, checkData interface{}) {
	if c.doCheck(dialer, checkData) {
		// If the check succeeded, we mark down a success.
		dialer.markSuccess()
	} else {
		// we call doMarkFailure so as not to trigger a recheck, since this failure
		// might just be due to the target we checked
		dialer.markFailure()
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
				// to within 2x of existing latency.
				latency = cap
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
