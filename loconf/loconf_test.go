package loconf

import (
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/getlantern/golog"
	"github.com/stretchr/testify/assert"
)

func TestRoundTrip(t *testing.T) {
	lc, err := Get(http.DefaultClient, false)
	assert.Nil(t, err)

	assert.True(t, len(lc.Surveys) > 0)

	lc, err = Get(http.DefaultClient, true)
	assert.Nil(t, err)

	assert.True(t, len(lc.Surveys) > 0)
}

func TestParsing(t *testing.T) {
	log := golog.LoggerFor("loconf-test")
	buf, _ := ioutil.ReadFile("test/desktop-ui.json")

	lc, err := parse(buf)

	assert.Nil(t, err)
	//log.Debugf("Got loconf: %+v", lc)
	assert.NotNil(t, lc)

	us, ok := lc.GetUninstallSurvey("zh-CN")

	assert.NotNil(t, us)
	assert.True(t, ok)

	log.Debugf("Got uninstall survey: %+v", us)

	us, ok = lc.GetUninstallSurvey("nothereatall")

	assert.Nil(t, us)
	assert.False(t, ok)
}

func TestUninstallSurvey(t *testing.T) {
	buf := `
  {
    "uninstall-survey": {
      "en-US": {
        "enabled": true,
        "probability": 1.00,
        "pro": false
      }
    }
  }
  `
	lc, err := parse([]byte(buf))
	assert.Nil(t, err)
	assert.NotNil(t, lc)

	sur, ok := lc.GetUninstallSurvey("en-US")

	assert.True(t, ok)
	assert.NotNil(t, sur)

	buf = `
  {
    "uninstall-survey": {
      "en-US": {
        "enabled": false,
        "probability": 1.00,
        "pro": true
      }
    }
  }
  `
	lc, err = parse([]byte(buf))
	assert.Nil(t, err)
	assert.NotNil(t, lc)

	sur, ok = lc.GetUninstallSurvey("en-US")

	assert.True(t, ok)
	assert.NotNil(t, sur)

	buf = `
  {
    "uninstall-survey": {
      "en-US": {
        "enabled": true,
        "probability": 1.00,
        "pro": true
      }
    }
  }
  `
	lc, err = parse([]byte(buf))
	assert.NotNil(t, lc)
	assert.Nil(t, err)

	sur, ok = lc.GetUninstallSurvey("en-US")

	assert.True(t, ok)
	assert.NotNil(t, sur)

	// Make sure we don't return the survey with 0 probability
	buf = `
  {
    "uninstall-survey": {
      "en-US": {
        "enabled": true,
        "probability": 0.00,
        "pro": true
      }
    }
  }
  `
	lc, err = parse([]byte(buf))
	assert.NotNil(t, lc)
	assert.Nil(t, err)

	sur, ok = lc.GetUninstallSurvey("en-US")

	assert.True(t, ok)
	assert.NotNil(t, sur)
}

func TestSurvey(t *testing.T) {
	buf := `
  {
    "survey": {
      "en-US": {
        "enabled": true,
        "probability": 1.0,
        "message": "Lantern is hiring an engineer in India! For more info, and to apply",
        "thanks": "Thanks for your application!",
        "button": "Click Here",
        "pro": true
      }
    }
  }
  `
	lc, err := parse([]byte(buf))
	assert.Nil(t, err)
	assert.NotNil(t, lc)

	sur, ok := lc.GetSurvey("en-US")

	assert.True(t, ok)
	assert.NotNil(t, sur)

	assert.Equal(t, "Click Here", sur.Button)

	sur, ok = lc.GetSurvey("nothereatall")

	assert.Nil(t, sur)
	assert.False(t, ok)

	// Make sure we don't return the survey with 0 probability
	buf = `
  {
    "survey": {
      "en-US": {
        "enabled": true,
        "probability": 0.00,
        "pro": true
      }
    }
  }
  `
	lc, err = parse([]byte(buf))
	assert.NotNil(t, lc)
	assert.Nil(t, err)

	sur, ok = lc.GetSurvey("en-US")

	assert.True(t, ok)
	assert.NotNil(t, sur)
}

func TestAnnouncements(t *testing.T) {
	buf := `
  {
    "announcement": {
       "default": "en-US",
       "en-US": {
         "campaign": "20160801-new-feature",
         "pro": true,
         "free": true,
         "expiry": "2018-02-02T15:00:00+07:00",
         "title": "Try out the new feature",
         "message": "Believe or not, you'll definitely love it!",
         "url": ""
       }
    }
  }`
	lc, err := parse([]byte(buf))

	assert.Nil(t, err)
	assert.NotNil(t, lc)

	ann, err := lc.GetAnnouncement("en-US", true)

	assert.Nil(t, err)
	assert.NotNil(t, ann)

	assert.Equal(t, "Try out the new feature", ann.Title)

	// Test that checking for an invalid locale still returns the default.
	ann, err = lc.GetAnnouncement("FAKE-LOCALE", true)
	assert.Nil(t, err)
	assert.NotNil(t, ann)

	// Now test missing default.
	buf = `
  {
    "announcement": {
       "default": "zh-CN",
       "en-US": {
         "campaign": "20160801-new-feature",
         "pro": true,
         "free": true,
         "expiry": "2018-02-02T15:00:00+07:00",
         "title": "Try out the new feature",
         "message": "Believe or not, you'll definitely love it!",
         "url": ""
       }
    }
  }`
	lc, err = parse([]byte(buf))

	assert.Nil(t, err)
	//log.Debugf("Got loconf: %+v", lc)
	assert.NotNil(t, lc)

	// Test that checking for an invalid locale no longer returns the default.
	ann, err = lc.GetAnnouncement("FAKE-LOCALE", true)
	assert.NotNil(t, err)
	assert.Nil(t, ann)

	// Now test no default.
	buf = `
  {
    "announcement": {
      "en-US": {
        "campaign": "20160801-new-feature",
        "pro": true
      }
    }
  }`
	lc, err = parse([]byte(buf))

	assert.Nil(t, err)
	//log.Debugf("Got loconf: %+v", lc)
	assert.NotNil(t, lc)

	// Test that checking for an invalid locale no longer returns the default.
	ann, err = lc.GetAnnouncement("FAKE-LOCALE", true)
	assert.NotNil(t, err)
	assert.Nil(t, ann)
}

func TestAnnouncementsExpiry(t *testing.T) {
	buf := `
  {
    "announcement": {
       "default": "en-US",
       "en-US": {
         "campaign": "20160801-new-feature",
         "pro": true,
         "free": true,
         "expiry": "2016-02-02T15:00:00+07:00",
         "title": "Try out the new feature",
         "message": "Believe or not, you'll definitely love it!",
         "url": ""
       }
    }
  }`
	lc, err := parse([]byte(buf))

	assert.Nil(t, err)
	assert.NotNil(t, lc)

	// Announcement should be expired!
	ann, err := lc.GetAnnouncement("en-US", true)

	assert.NotNil(t, err)
	assert.Nil(t, ann)

	// Now test a bogus expiry time.

	buf = `
  {
    "announcement": {
       "default": "en-US",
       "en-US": {
         "campaign": "20160801-new-feature",
         "pro": true,
         "free": true,
         "expiry": "woah-not-correct",
         "title": "Try out the new feature",
         "message": "Believe or not, you'll definitely love it!",
         "url": ""
       }
    }
  }`
	lc, err = parse([]byte(buf))

	assert.Nil(t, err)
	assert.NotNil(t, lc)

	// Should not have been able to parse expiry
	ann, err = lc.GetAnnouncement("en-US", true)

	assert.NotNil(t, err)
	assert.Nil(t, ann)
}
