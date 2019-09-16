// Package goroutines spawns a loop to periodically check for the count of
// goroutines. If the count reaches a limit and is increasing, print the top N
// entries of the goroutine profile.
package goroutines

import (
	"bytes"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/getlantern/golog"
)

var log = golog.LoggerFor("goroutines")

// Monitor keeps checking the number of goroutines in interval. If the number
// reaching the limit, it logs the topN entries of the goroutine profile
// whenever the number is higher than the previous check.
func Monitor(interval time.Duration, limit int, topN int) (stop func()) {
	chStop := make(chan struct{})
	go func() {
		var lastN int
		tk := time.NewTicker(interval)
		defer tk.Stop()
		for {
			select {
			case <-tk.C:
				num := runtime.NumGoroutine()
				if num >= limit && num > lastN {
					PrintProfile(topN)
					lastN = num
				} else {
					log.Debugf("goroutine profile: total %v", num)
				}
			case <-chStop:
				return
			}
		}
	}()
	return func() { close(chStop) }
}

// PrintProfile logs the topN entries of the goroutine profile
func PrintProfile(topN int) {
	var buf bytes.Buffer
	p := pprof.Lookup("goroutine")
	e := p.WriteTo(&buf, 1) // debug=1
	if e != nil {
		log.Errorf("Unable to collect goroutine profile: %v", e)
		return
	}

	var lines []string
	for count := 0; count < topN; {
		line, e := buf.ReadString(byte('\n'))
		if e != nil {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			count++
		}
		lines = append(lines, line)
	}
	log.Debug(strings.Join(lines, "\n"))
}
