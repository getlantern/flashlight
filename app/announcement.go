package app

import (
	"net/http"
	"time"

	"github.com/getlantern/notifier"

	"github.com/getlantern/flashlight/announcement"
	"github.com/getlantern/flashlight/ui"
)

// AnnouncementsLoop starts a goroutine to periodically check for announcements
// and show them via desktop notification. Each announcement is shown only
// once.
//
// interval: The duration between each check.
//
// proChecker: A function to check if current user is Pro (to decide whether
// show the announcement or not).
//
// Returns a function to stop the loop.
func AnnouncementsLoop(interval time.Duration, proChecker func() bool) (stop func()) {
	isStaging := stagingMode == "true"

	chStop := make(chan bool)
	t := time.NewTicker(interval)
	go func() {
		for {
			isPro := proChecker()
			lang := settings.getString(SNLanguage)
			var past []string
			current, err := announcement.Get(http.DefaultClient, lang, isPro, isStaging)
			if err != nil {
				// An error can simply mean no announcement available. Consider
				// the low frequence, treat all as errors for simplification.
				log.Error(err)
				goto wait
			}
			past = settings.getStringArray(SNPastAnnouncements)
			if in(current, past) {
				goto wait
			}
			if show(current) {
				past = append(past, current.Campaign)
				settings.setStringArray(SNPastAnnouncements, past)
			}

		wait:
			select {
			case <-t.C:
			case <-chStop:
				t.Stop()
				return
			}
		}
	}()

	return func() {
		chStop <- true
	}
}

func in(current *announcement.Announcement, past []string) bool {
	for _, c := range past {
		if current.Campaign == c {
			return true
		}
	}
	return false
}

func show(a *announcement.Announcement) bool {
	logo := ui.AddToken("/img/lantern_logo.png")
	note := &notify.Notification{
		Title:       a.Title,
		Message:     a.Message,
		IsClickable: a.URL != "",
		ClickURL:    a.URL,
		IconURL:     logo,
	}
	if err := notify.NewNotifications().Notify(note); err != nil {
		log.Errorf("Could not notify? %v", err)
		return false
	}
	return true
}
