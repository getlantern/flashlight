package app

import (
	"net/http"
	"time"

	"github.com/getlantern/notifier"

	"github.com/getlantern/flashlight/announcement"
	"github.com/getlantern/flashlight/common"
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
func AnnouncementsLoop(interval time.Duration, proChecker func() (bool, bool)) (stop func()) {
	chStop := make(chan bool)
	t := time.NewTicker(interval)
	isStaging := common.Staging
	checker := func() {
		isPro, ok := proChecker()
		if !ok {
			log.Debugf("Skip checking announcement as user status is unknown")
			return
		}
		lang := settings.getString(SNLanguage)
		current, err := announcement.Get(http.DefaultClient, lang, isPro, isStaging)
		if err != nil {
			if err == announcement.ErrNoAvailable {
				log.Debugf("No available announcement")
			} else {
				log.Error(err)
			}
			return
		}
		past := settings.getStringArray(SNPastAnnouncements)
		if in(current.Campaign, past) {
			log.Debugf("Skip announcement %s", current.Campaign)
			return
		}
		if showAnnouncement(current) {
			past = append(past, current.Campaign)
			settings.setStringArray(SNPastAnnouncements, past)
		}
	}
	go func() {
		for {
			checker()
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

func in(s string, coll []string) bool {
	for _, v := range coll {
		if s == v {
			return true
		}
	}
	return false
}

func showAnnouncement(a *announcement.Announcement) bool {
	logo := ui.AddToken("/img/lantern_logo.png")
	note := &notify.Notification{
		Title:    a.Title,
		Message:  a.Message,
		ClickURL: a.URL,
		IconURL:  logo,
	}
	return showNotification(note)
}
