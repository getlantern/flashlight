package loconfscanner

import (
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/getlantern/golog"

	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/loconf"
	"github.com/getlantern/flashlight/ui"

	"github.com/getlantern/flashlight/app/notifier"
)

type PastAnnouncements interface {
	Get() []string
	Add(string)
}

// Scanner starts a goroutine to periodically check for new loconf files.
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
func Scanner(interval time.Duration,
	proChecker func() (bool, bool),
	lang func() string,
	past PastAnnouncements,
) (stop func()) {
	loc := &loconfer{
		log:  golog.LoggerFor("loconfer"),
		r:    rand.New(rand.NewSource(time.Now().UnixNano())),
		lang: lang,
		past: past,
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
	log  golog.Logger
	r    *rand.Rand
	lang func() string
	past PastAnnouncements
}

func (loc *loconfer) onLoconf(lc *loconf.LoConf, isPro bool) {
	go loc.setUninstallURL(lc, isPro)
	go loc.makeAnnouncements(lc, isPro)
}

func (loc *loconfer) setUninstallURL(lc *loconf.LoConf, isPro bool) {
	path, err := client.InConfigDir("", "uninstall_url.txt")
	if err != nil {
		loc.log.Errorf("Could not get config path? %v", err)
		return
	}
	survey, ok := lc.GetUninstallSurvey(loc.lang())
	if !ok {
		loc.log.Debugf("No available uninstall survey")
		return
	}
	loc.writeURL(path, survey, isPro)
}

func (loc *loconfer) writeURL(path string, survey *loconf.UninstallSurvey, isPro bool) {
	var url string
	if survey.Enabled && (isPro && survey.Pro || !isPro && survey.Free) {
		if survey.Probability > loc.r.Float64() {
			loc.log.Debugf("Enabling survey at URL %v", survey.URL)
			url = survey.URL
		} else {
			loc.log.Debugf("Turning survey off probabalistically")
		}
	}
	outfile, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		loc.log.Errorf("Unable to open file %v for writing: %v", path, err)
		return
	}
	defer outfile.Close()

	_, err = outfile.Write([]byte(url))
	if err != nil {
		loc.log.Errorf("Unable to write url to file %v: %v", path, err)
	}
}

func (loc *loconfer) makeAnnouncements(lc *loconf.LoConf, isPro bool) {
	current, err := lc.GetAnnouncement(loc.lang(), isPro)
	if err != nil {
		if err == loconf.ErrNoAvailable {
			loc.log.Debugf("No available announcement")
		} else {
			loc.log.Error(err)
		}
		return
	}
	if in(current.Campaign, loc.past.Get()) {
		loc.log.Debugf("Skip announcement %s", current.Campaign)
		return
	}
	if loc.showAnnouncement(current) {
		loc.past.Add(current.Campaign)
	}
}

func (loc *loconfer) showAnnouncement(a *loconf.Announcement) bool {
	logo := ui.AddToken("/img/lantern_logo.png")
	note := &notifier.Notification{
		Title:    a.Title,
		Message:  a.Message,
		ClickURL: a.URL,
		IconURL:  logo,
	}
	return notifier.Show(note)
}
