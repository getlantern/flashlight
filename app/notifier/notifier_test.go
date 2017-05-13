package notifier

import (
	"testing"

	"github.com/getlantern/notifier"
	"github.com/stretchr/testify/assert"
)

func TestNotify(t *testing.T) {
	stop := NotificationsLoop()

	note := &notify.Notification{
		Title:    "test",
		Message:  "test",
		ClickURL: "https://test.com",
		IconURL:  "https://test.com",
	}

	shown := ShowNotification(note)

	assert.True(t, shown)
	stop()
}
