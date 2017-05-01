package app

import (
	"time"

	"github.com/getlantern/notifier"
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
	stop = func() {
		log.Debug("Requested stop of notifications loop")
		chStop <- true
	}
	go func() {
		for {
			select {
			case n := <-ch:
				if err := notifier.Notify(n.note); err != nil {
					log.Errorf("Could not notify? %v", err)
					n.chResult <- false
				} else {
					n.chResult <- true
				}
				time.Sleep(10 * time.Second)
			case <-chStop:
				log.Debug("Stopped notifications loop")
				return
			}
		}
	}()
	return
}
