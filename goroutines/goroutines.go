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

	"github.com/getlantern/flashlight/logging"
)

var log = logging.LoggerFor("goroutines")

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
					printProfile(topN)
					lastN = num
				} else {
					log.Infof("goroutine profile: total %v", num)
				}
			case <-chStop:
				return
			}
		}
	}()
	return func() { close(chStop) }
}

func printProfile(topN int) {
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
	log.Info(strings.Join(lines, "\n"))
}
