package app

import (
	"time"

	"github.com/getlantern/notifier"
)

var ch chan *notify.Notification = make(chan *notify.Notification)

// showNotification submits the notification to the notificationsLoop to show.
func showNotification(note *notify.Notification) {
	ch <- note
}

// notificationsLoop starts a goroutine to show the desktop notifications submitted by showNotification one by one with a minimum 10 seconds interval.
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
			case note := <-ch:
				if err := notifier.Notify(note); err != nil {
					log.Errorf("Could not notify? %v", err)
				}
				time.Sleep(10 * time.Second)
			case <-chStop:
				return
			}
		}
	}()
	return
}
