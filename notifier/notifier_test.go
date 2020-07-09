package notifier

import (
	"testing"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/notifier"
	"github.com/stretchr/testify/assert"

	"github.com/getlantern/flashlight/analytics"
	"github.com/getlantern/flashlight/common"
)

func TestNotify(t *testing.T) {
	stop := loopFor(10*time.Millisecond, analytics.NullSession{})
	stop()

	stop = loopFor(10*time.Millisecond, analytics.NullSession{})
	note := &notify.Notification{
		Title:    "test",
		Message:  "test",
		ClickURL: "https://test.com",
		IconURL:  "https://test.com",
	}

	shown := ShowNotification(note, "test-campaign")

	assert.True(t, shown)
	stop()
}

func TestNormalizeClickURL(t *testing.T) {
	log := golog.LoggerFor("flashlight.notifier-test")
	note := &notify.Notification{
		Title:    "test",
		Message:  "test",
		ClickURL: "https://test.com",
		IconURL:  "https://test.com",
	}

	err := normalizeClickURL(note, "test-campaign")
	assert.NoError(t, err, "unexpected error")
	log.Debugf("url: %v", note.ClickURL)
	assert.Equal(t, "https://test.com?utm_campaign=test-campaign&utm_content=test-test&utm_medium=notification&utm_source="+common.Platform, note.ClickURL)

	note.ClickURL = ":"
	log.Debugf("url: %v", note.ClickURL)
	err = normalizeClickURL(note, "test-campaign")
	assert.Error(t, err, "expected an error")

	stop := loopFor(10*time.Millisecond, analytics.NullSession{})
	defer stop()
	assert.False(t, ShowNotification(note, "test-campaign"))
}
