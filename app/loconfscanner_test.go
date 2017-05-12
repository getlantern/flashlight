package app

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
		log: golog.LoggerFor("flashlight.app.loconfer"),
		r:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	testURL := "https://testtesttest"
	us := &loconf.UninstallSurvey{
		URL:         testURL,
		Probability: 1.0,
		Pro:         true,
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

type set struct {
}

func (s *set) GetLanguage() string {
	return ""
}

func (s *set) getStringArray(name SettingName) []string {
	return make([]string, 0)
}

func (s *set) setStringArray(name SettingName, vals interface{}) {
}

func TestParsing(t *testing.T) {
	logger := golog.LoggerFor("flashlight.app.loconfloop-test")
	settings := &set{}
	stop := LoconfScanner(4*time.Hour, func() (bool, bool) {
		return true, false
	}, settings)
	stop()
	stop = LoconfScanner(4*time.Hour, func() (bool, bool) {
		return true, true
	}, settings)
	logger.Debug("Stopping")
	stop()

	loc := &loconfer{
		log: golog.LoggerFor("flashlight.app.loconfer-test"),
	}

	loc.scan(4*time.Hour, func() (bool, bool) {
		return true, true
	}, func(lc *loconf.LoConf, isPro bool) {
		logger.Debugf("lc %+v", lc)
	})
}
