package app

import (
	"testing"
	"time"

	"github.com/getlantern/flashlight/loconf"
	"github.com/getlantern/golog"
)

func TestParsing(t *testing.T) {
	log := golog.LoggerFor("loconfloop-test")
	stop := LoconfScanner(4*time.Hour, func() (bool, bool) {
		return true, false
	})
	stop()
	stop = LoconfScanner(4*time.Hour, func() (bool, bool) {
		return true, true
	})
	log.Debug("Stopping")
	stop()

	loc := &loconfer{
		log: golog.LoggerFor("loconfer"),
	}

	loc.scan(4*time.Hour, func() (bool, bool) {
		return true, true
	}, func(lc *loconf.LoConf, isPro bool) {
		log.Debugf("lc %+v", lc)
	})
}
