package app

import (
	"sync/atomic"
	"testing"

	"github.com/getlantern/bandwidth"
	"github.com/getlantern/flashlight/ws"
	"github.com/getlantern/golog"
	"github.com/getlantern/notifier"
	"github.com/stretchr/testify/assert"
)

func TestPercents(t *testing.T) {
	var notified atomic.Value
	ns := notifyStatus{log: golog.LoggerFor("bandwidth-test"), pro: func() (bool, bool) {
		return false, true
	}, uiAddr: func() string {
		return "127.0.0.1:7777"
	},
		logoURL: func() string {
			return "http://127.0.0.1:7777/img/lantern_logo.png"
		},
		notifyFunc: func(note *notify.Notification) {
			notified.Store(note)
			//showNotification(note)
		},
	}

	stop := notificationsLoop()

	quota := &bandwidth.Quota{
		MiBAllowed: 1000,
		MiBUsed:    801,
	}

	assert.True(t, ns.isEightyOrMore(quota))

	quota.MiBUsed = 501
	assert.False(t, ns.isEightyOrMore(quota))
	assert.True(t, ns.isFiftyOrMore(quota))

	msg := "you have used %s of your data"
	expected := "you have used 80% of your data"
	assert.Equal(t, expected, ns.percentMsg(msg, 80))

	ns.notifyFreeUser("test", "test")

	assert.Equal(t, "test", notified.Load().(*notify.Notification).Title, "should have received a message")
	blank := &notify.Notification{
		Title:    "blank",
		Message:  "blank",
		ClickURL: "blank",
		IconURL:  "blank",
	}
	notified.Store(blank)
	ns.notifyFifty()
	assert.NotEqual(t, "blank", notified.Load().(*notify.Notification).Title, "should have received a message")

	notified.Store(blank)
	ns.notifyEighty()
	assert.NotEqual(t, "blank", notified.Load().(*notify.Notification).Title, "should have received a message")

	notified.Store(blank)
	ns.notifyCapHit()
	assert.NotEqual(t, "blank", notified.Load().(*notify.Notification).Title, "should have received a message")

	stop()

	ws.StartUIChannel()
	err := serveBandwidth(func() (bool, bool) {
		return false, true
	}, func() string {
		return "127.0.0.1:7777"
	}, func() string {
		return "http://127.0.0.1:7777/img/lantern_logo.png"
	}, func(note *notify.Notification) {
		notified.Store(note)
	}, "bandwidth-test")

	assert.NoError(t, err, "Unexpected error")
}
