package notifier

import (
	"net/url"
	"runtime"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/i18n"
	"github.com/getlantern/notifier"
)

type Notification notify.Notification

type notifierRequest struct {
	note     *notify.Notification
	chResult chan bool
}

var (
	log = golog.LoggerFor("flashlight.notifier")
	ch  = make(chan notifierRequest)

	notificationTimeout = 15 * time.Second
)

// Show submits the notification to the notificationsLoop to show
// and waits for the result.
func Show(note *Notification, campaign string) bool {
	err := normalizeClickURL(note, campaign)
	if err != nil {
		return false
	}
	chResult := make(chan bool)
	ch <- notifierRequest{
		(*notify.Notification)(note),
		chResult,
	}

	return <-chResult
}

func normalizeClickURL(note *Notification, campaign string) error {
	u, err := url.Parse(note.ClickURL)
	if err != nil {
		log.Errorf("Could not parse click URL: %v", err)
		return err
	}

	q := u.Query()
	q.Set("utm_source", runtime.GOOS)
	q.Set("utm_medium", "notification")
	q.Set("utm_campaign", campaign)
	q.Set("utm_content", note.Title+"-"+note.Message)
	u.RawQuery = q.Encode()

	note.ClickURL = u.String()
	return nil
}

// Start starts a goroutine to show the desktop notifications
// submitted by showNotification one by one with a minimum 10 seconds interval.
//
// Returns a function to stop the loop.
func Start() (stop func()) {
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
