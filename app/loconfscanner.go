package app

import (
	"net/http"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/notifier"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/loconf"
	"github.com/getlantern/flashlight/ui"
)

// LoconfScanner starts a goroutine to periodically check for new loconf files.
// This will show announcements via desktop notification. Each announcement
// is shown only once. It will also do things like check for updates to
// uninstall survey config
//
// interval: The duration between each check.
//
// proChecker: A function to check if current user is Pro (to decide whether
// show the announcement or not).
//
// Returns a function to stop the loop.
func LoconfScanner(interval time.Duration, proChecker func() (bool, bool)) (stop func()) {
	loc := &loconfer{
		log: golog.LoggerFor("loconfer"),
	}
	return loc.scan(interval, proChecker, loc.onLoconf)
}

func (loc *loconfer) scan(interval time.Duration, proChecker func() (bool, bool), onLoconf func(*loconf.LoConf, bool)) (stop func()) {
	chStop := make(chan bool)
	t := time.NewTicker(interval)
	isStaging := common.Staging
	checker := func() {
		lc, err := loconf.Get(http.DefaultClient, isStaging)
		if err != nil {
			loc.log.Error(err)
			return
		}
		isPro, ok := proChecker()
		if !ok {
			loc.log.Debugf("Skip checking announcement as user status is unknown")
			return
		}
		onLoconf(lc, isPro)
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

type loconfer struct {
	log golog.Logger
}

func (loc *loconfer) onLoconf(lc *loconf.LoConf, isPro bool) {
	go loc.setUninstallURL(lc, isPro)
	go loc.makeAnnouncements(lc, isPro)
}

func (loc *loconfer) setUninstallURL(lc *loconf.LoConf, isPro bool) {
	lang := settings.GetLanguage()
	survey, ok := lc.GetUninstallSurvey(lang)
	if !ok {
		loc.log.Debugf("No available uninstall survey")
		return
	}
	lc.SetUninstallURLInRegistry(survey, isPro)
}

func (loc *loconfer) makeAnnouncements(lc *loconf.LoConf, isPro bool) {
	lang := settings.GetLanguage()
	current, err := lc.GetAnnouncement(lang, isPro)
	if err != nil {
		if err == loconf.ErrNoAvailable {
			loc.log.Debugf("No available announcement")
		} else {
			loc.log.Error(err)
		}
		return
	}
	past := settings.getStringArray(SNPastAnnouncements)
	if in(current.Campaign, past) {
		loc.log.Debugf("Skip announcement %s", current.Campaign)
		return
	}
	if loc.showAnnouncement(current) {
		past = append(past, current.Campaign)
		settings.setStringArray(SNPastAnnouncements, past)
	}
}

func (loc *loconfer) showAnnouncement(a *loconf.Announcement) bool {
	logo := ui.AddToken("/img/lantern_logo.png")
	note := &notify.Notification{
		Title:    a.Title,
		Message:  a.Message,
		ClickURL: a.URL,
		IconURL:  logo,
	}
	return showNotification(note)
}
