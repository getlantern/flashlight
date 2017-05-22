package loconfscanner

import (
	"io/ioutil"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/getlantern/flashlight/loconf"
	"github.com/getlantern/golog"
	"github.com/stretchr/testify/assert"
)

func TestWriteURL(t *testing.T) {
	loc := &loconfer{
		log: golog.LoggerFor("loconfer"),
		r:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	testURL := "https://testtesttest"
	us := &loconf.UninstallSurvey{
		//log: golog.LoggerFor("loconfer"),
		URL:         testURL,
		Probability: 1.0,
		Enabled:     true,
	}
	file, err := ioutil.TempFile(os.TempDir(), "urlfiletest")
	assert.Nil(t, err)
	//defer os.Remove(file.Name())
	//path := "test/testpath"
	loc.writeURL(file.Name(), us, true)

	dat, err := ioutil.ReadFile(file.Name())
	assert.Nil(t, err)
	assert.Equal(t, testURL, string(dat))
}

func TestParsing(t *testing.T) {
	logger := golog.LoggerFor("loconfloop-test")
	s := New(4*time.Hour,
		func() (bool, bool) {
			return true, false
		},
		nil,
	).(*loconfer)
	s.Start()
	s.Stop()

	s = New(4*time.Hour,
		func() (bool, bool) {
			return true, true
		},
		nil,
	).(*loconfer)
	s.Start()
	logger.Debug("Stopping")
	s.Stop()

	loc := &loconfer{
		log:      golog.LoggerFor("loconfer"),
		interval: time.Millisecond,
		proChecker: func() (bool, bool) {
			return true, true
		},
	}

	loc.scan(
		func(lc *loconf.LoConf, isPro bool) {
			logger.Debugf("lc %+v", lc)
		})
	time.Sleep(1 * time.Second)
}
