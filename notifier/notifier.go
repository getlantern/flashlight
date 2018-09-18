package notifier

import (
	"time"

	"github.com/getlantern/flashlight/analytics"
	"github.com/getlantern/zaplog"
	"github.com/getlantern/i18n"
	"github.com/getlantern/notifier"
)

const (
	notificationTimeout = 15 * time.Second
)

type notifierRequest struct {
	note     *notify.Notification
	chResult chan bool
}

var (
	log = zaplog.LoggerFor("flashlight.notifier")
	ch  = make(chan notifierRequest)
)

// ShowNotification submits the notification to the notificationsLoop to show
// and waits for the result.
func ShowNotification(note *notify.Notification, campaign string) bool {
	err := normalizeClickURL(note, campaign)
	if err != nil {
		return false
	}
	chResult := make(chan bool)
	ch <- notifierRequest{
		note,
		chResult,
	}

	return <-chResult
}

func normalizeClickURL(note *notify.Notification, campaign string) error {
	ga, err := analytics.AddCampaign(note.ClickURL, campaign, note.Title+"-"+note.Message, "notification")
	if err != nil {
		return err
	}

	note.ClickURL = ga
	return nil
}

// NotificationsLoop starts a goroutine to show the desktop notifications
// submitted by showNotification one by one with a minimum 10 seconds interval.
//
// Returns a function to stop the loop.
func NotificationsLoop() (stop func()) {
	return loopFor(notificationTimeout)
}

// NotificationsLoop starts a goroutine to show the desktop notifications
// submitted by showNotification one by one with a minimum 10 seconds interval.
//
// Returns a function to stop the loop.
func loopFor(delay time.Duration) (stop func()) {
	notifier := notify.NewNotifications()
	// buffered channel to avoid blocking stop() when goroutine is sleeping
	chStop := make(chan bool, 1)
	stop = func() { chStop <- true }
	go func() {
		for {
			select {
			case n := <-ch:
				n.note.Sender = "com.getlantern.lantern"
				n.note.AutoDismissAfter = notificationTimeout
				if n.note.ClickLabel == "" {
					n.note.ClickLabel = i18n.T("BACKEND_CLICK_LABEL_OPEN")
				}
				if err := notifier.Notify(n.note); err != nil {
					log.Errorf("Could not notify? %v", err)
					n.chResult <- false
				} else {
					n.chResult <- true
				}
				time.Sleep(delay)
			case <-chStop:
				return
			}
		}
	}()
	return
}
