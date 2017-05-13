package app

import (
	"time"

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

var ch chan notifierRequest = make(chan notifierRequest)

// showNotification submits the notification to the notificationsLoop to show
// and waits for the result.
func showNotification(note *notify.Notification) bool {
	chResult := make(chan bool)
	ch <- notifierRequest{
		note,
		chResult,
	}

	return <-chResult

}

// notificationsLoop starts a goroutine to show the desktop notifications
// submitted by showNotification one by one with a minimum 10 seconds interval.
//
// Returns a function to stop the loop.
func notificationsLoop() (stop func()) {
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
				n.note.ClickLabel = i18n.T("BACKEND_CLICK_LABEL")
				if err := notifier.Notify(n.note); err != nil {
					log.Errorf("Could not notify? %v", err)
					n.chResult <- false
				} else {
					n.chResult <- true
				}
				time.Sleep(notificationTimeout)
			case <-chStop:
				return
			}
		}
	}()
	return
}
