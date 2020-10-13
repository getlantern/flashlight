package desktop

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

var logger = golog.LoggerFor("desktop-test")

var tempConfigDir string

func TestMain(m *testing.M) {
	tempConfigDir, err := ioutil.TempDir("", "loconfscanner_test")
	if err != nil {
		logger.Errorf("Unable to create temp config dir: %v", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tempConfigDir)
	os.Exit(m.Run())
}

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
	stop := LoconfScanner(tempConfigDir, 4*time.Hour, func() (bool, bool) {
		return true, false
	}, func() string { return "" })
	stop()
	stop = LoconfScanner(tempConfigDir, 4*time.Hour, func() (bool, bool) {
		return true, true
	}, func() string { return "" })
	logger.Debug("Stopping")
	stop()

	loc := &loconfer{
		log: golog.LoggerFor("loconfer"),
	}

	loc.scan(4*time.Hour, func() (bool, bool) {
		return true, true
	}, func(lc *loconf.LoConf, isPro bool) {
		logger.Debugf("lc %+v", lc)
	})
}
