package httpseverywhere

import (
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/armon/go-radix"
	"github.com/getlantern/golog"
	"github.com/getlantern/mtime"
)

// Rewrite changes an HTTP URL to rewrite.
type Rewrite func(url *url.URL) (string, bool)

type httpse struct {
	log             golog.Logger
	initOnce        sync.Once
	wildcardTargets atomic.Value // *radix.Tree
	plainTargets    atomic.Value // map[string]*ruleset
	stats           *httpseStats
	statsCh         chan *timing
}

type httpseStats struct {
	runs      int64
	totalTime int64
	max       int64
	maxHost   string
}

type timing struct {
	host string
	dur  time.Duration
}

// Default returns a lazily-initialized Rewrite using the default rules
func Default() Rewrite {
	h := newEmpty()
	h.initAsync()
	return h.rewrite
}

// Eager returns an eagerly-initialized Rewrite using the default rules
func Eager() Rewrite {
	h := newEmpty()
	h.init()
	return h.rewrite
}

func newEmpty() *httpse {
	h := &httpse{
		log:     golog.LoggerFor("httpse"),
		stats:   &httpseStats{},
		statsCh: make(chan *timing, 100),
	}
	go h.readTimings()
	h.wildcardTargets.Store(radix.New())
	h.plainTargets.Store(make(map[string]*ruleset))
	return h
}

func (h *httpse) init() {
	d := newDeserializer()
	plain, wildcard, err := d.newRulesets()
	if err != nil {
		return
	}
	h.plainTargets.Store(plain)
	h.wildcardTargets.Store(wildcard)
}

func (h *httpse) initAsync() {
	h.initOnce.Do(func() {
		go h.init()
	})
}

func (h *httpse) rewrite(url *url.URL) (string, bool) {
	if url.Scheme != "http" {
		return "", false
	}

	start := mtime.Now()
	defer func() {
		h.statsCh <- &timing{
			dur:  mtime.Now().Sub(start),
			host: url.String(),
		}
	}()
	if val, ok := h.plainTargets.Load().(map[string]*ruleset)[url.Host]; ok {
		if r, hit := h.rewriteWithRuleset(url, val); hit {
			return r, hit
		}
	}
	// Check prefixes (with reversing the URL host)
	if _, val, match := h.wildcardTargets.Load().(*radix.Tree).LongestPrefix(reverse(url.Host)); match {
		if r, hit := h.rewriteWithRuleset(url, val.(*ruleset)); hit {
			return r, hit
		}
	}

	// Check suffixes last because there are far fewer suffix rules.
	if _, val, match := h.wildcardTargets.Load().(*radix.Tree).LongestPrefix(url.Host); match {
		return h.rewriteWithRuleset(url, val.(*ruleset))
	}

	return "", false
}

// rewriteWithRuleset converts the given URL to HTTPS if there is an associated
// rule for it.
func (h *httpse) rewriteWithRuleset(fullURL *url.URL, r *ruleset) (string, bool) {
	url := fullURL.String()
	for _, exclude := range r.exclusion {
		if exclude.pattern.MatchString(url) {
			return "", false
		}
	}
	for _, rule := range r.rule {
		if rule.from.MatchString(url) {
			return rule.from.ReplaceAllString(url, rule.to), true
		}
	}
	return "", false
}

func reverse(input string) string {
	n := 0
	runes := make([]rune, len(input)+1)
	// Add a dot prefix to make sure we're only operating on subdomains
	runes[0] = '.'
	runes = runes[1:]
	for _, r := range input {
		runes[n] = r
		n++
	}
	runes = runes[0:n]
	// Reverse
	for i := 0; i < n/2; i++ {
		runes[i], runes[n-1-i] = runes[n-1-i], runes[i]
	}
	// Convert back to UTF-8.
	return string(runes)
}

func (h *httpse) readTimings() {
	for t := range h.statsCh {
		h.addTiming(t)
	}
}

func (h *httpse) addTiming(t *timing) {
	ms := t.dur.Nanoseconds() / int64(time.Millisecond)
	h.stats.runs++
	h.stats.totalTime += ms
	if ms > h.stats.max {
		h.stats.max = ms
		h.stats.maxHost = t.host
	}
}
