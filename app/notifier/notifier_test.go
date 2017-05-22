package notifier

import (
	"runtime"
	"testing"
	"time"

	"github.com/getlantern/golog"
	"github.com/stretchr/testify/assert"
)

func init() {
	// set to speed up test
	notificationTimeout = 10 * time.Millisecond
}

func TestNotify(t *testing.T) {
	stop := Start()
	stop()

	stop = Start()
	note := &Notification{
		Title:    "test",
		Message:  "test",
		ClickURL: "https://test.com",
		IconURL:  "https://test.com",
	}

	shown := Show(note, "test-campaign")

	assert.True(t, shown)
	stop()
}

func TestNormalizeClickURL(t *testing.T) {
	log := golog.LoggerFor("flashlight.notifier-test")
	note := &Notification{
		Title:    "test",
		Message:  "test",
		ClickURL: "https://test.com",
		IconURL:  "https://test.com",
	}

	err := normalizeClickURL(note, "test-campaign")
	assert.NoError(t, err, "unexpected error")
	log.Debugf("url: %v", note.ClickURL)
	assert.Equal(t, "https://test.com?utm_campaign=test-campaign&utm_content=test-test&utm_medium=notification&utm_source="+runtime.GOOS, note.ClickURL)

	note.ClickURL = ":"
	log.Debugf("url: %v", note.ClickURL)
	err = normalizeClickURL(note, "test-campaign")
	assert.Error(t, err, "expected an error")

	stop := Start()
	defer stop()
	assert.False(t, Show(note, "test-campaign"))
}
