package desktop

import (
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/getlantern/notifier"
	"go.uber.org/zap"

	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/geolookup"
	"github.com/getlantern/flashlight/loconf"
	"github.com/getlantern/flashlight/logging"
	"github.com/getlantern/flashlight/notifier"
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
func LoconfScanner(interval time.Duration, proChecker func() (bool, bool), iconURL func() string) (stop func()) {
	loc := &loconfer{
		log:     logging.LoggerFor("loconfer"),
		r:       rand.New(rand.NewSource(time.Now().UnixNano())),
		iconURL: iconURL,
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
			loc.log.Infof("Skip checking announcement as user status is unknown")
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
	log     *zap.SugaredLogger
	r       *rand.Rand
	iconURL func() string
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

	survey := lc.GetUninstallSurvey(settings.GetLanguage(), geolookup.GetCountry(time.Second*30), isPro)
	if survey == nil {
		loc.log.Infof("No available uninstall survey")
		return
	}
	loc.writeURL(path, survey, isPro)
}

func (loc *loconfer) writeURL(path string, survey *loconf.UninstallSurvey, isPro bool) {
	var url string
	if survey.Enabled {
		if survey.Probability > loc.r.Float64() {
			loc.log.Infof("Enabling survey at URL %v", survey.URL)
			url = survey.URL
		} else {
			loc.log.Infof("Turning survey off probabalistically")
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
	lang := settings.GetLanguage()
	current, err := lc.GetAnnouncement(lang, isPro)
	if err != nil {
		if err == loconf.ErrNoAvailable {
			loc.log.Infof("No available announcement")
		} else {
			loc.log.Error(err)
		}
		return
	}
	past := settings.getStringArray(SNPastAnnouncements)
	if in(current.Campaign, past) {
		loc.log.Infof("Skip announcement %s", current.Campaign)
		return
	}
	if loc.showAnnouncement(current) {
		past = append(past, current.Campaign)
		settings.setStringArray(SNPastAnnouncements, past)
	}
}

func (loc *loconfer) showAnnouncement(a *loconf.Announcement) bool {
	note := &notify.Notification{
		Title:    a.Title,
		Message:  a.Message,
		ClickURL: a.URL,
		IconURL:  loc.iconURL(),
	}
	return notifier.ShowNotification(note, "global-announcement")
}
