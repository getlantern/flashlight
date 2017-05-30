package loconfscanner

import (
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/getlantern/golog"

	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/loconf"
	"github.com/getlantern/flashlight/service"
	"github.com/getlantern/flashlight/ui"

	"github.com/getlantern/flashlight/app/notifier"
)

var ServiceID service.ID     = "flashlight.desktop.loconfscanner"

type PastAnnouncements interface {
	Get() []string
	Add(string)
}

type ConfigOpts struct {
	Lang    string
	Country string
}

func (c *ConfigOpts) For() service.ID {
	return ServiceID
}

func (c *ConfigOpts) Complete() string {
	if c.Lang == "" {
		return "missing Lang"
	}
	return ""
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

type loconfer struct {
	log        golog.Logger
	r          *rand.Rand
	proChecker func() (bool, bool)
	past       PastAnnouncements
	interval   time.Duration
	chStop     chan bool

	mu   sync.RWMutex
	opts ConfigOpts
}

func New(
	interval time.Duration,
	proChecker func() (bool, bool),
	past PastAnnouncements,
) service.Service {
	return &loconfer{
		log:        golog.LoggerFor("loconfer"),
		r:          rand.New(rand.NewSource(time.Now().UnixNano())),
		past:       past,
		interval:   interval,
		proChecker: proChecker,
	}
}

func (loc *loconfer) GetID() service.ID {
	return ServiceID
}

func (loc *loconfer) Configure(opts service.ConfigOpts) {
	loc.mu.Lock()
	defer loc.mu.Unlock()
	loc.opts = *opts.(*ConfigOpts)
}

func (loc *loconfer) Start() {
	loc.chStop = make(chan bool)
	loc.scan(loc.onLoconf)
}
func (loc *loconfer) scan(onLoconf func(*loconf.LoConf, bool)) {
	t := time.NewTicker(loc.interval)
	checker := func() {
		lc, err := loconf.Get(http.DefaultClient, common.Staging)
		if err != nil {
			loc.log.Error(err)
			return
		}
		isPro, ok := loc.proChecker()
		if !ok {
			loc.log.Debugf("Skip checking announcement as user status is unknown")
			return
		}
		loc.onLoconf(lc, isPro)
	}
	go func() {
		for {
			checker()
			select {
			case <-t.C:
			case <-loc.chStop:
				t.Stop()
				return
			}
		}
	}()
}

func (loc *loconfer) Stop() {
	close(loc.chStop)
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
	loc.mu.RLock()
	lang, country := loc.opts.Lang, loc.opts.Country
	loc.mu.RUnlock()
	survey := lc.GetUninstallSurvey(lang, country, isPro)
	if survey == nil {
		loc.log.Debugf("No available uninstall survey")
		return
	}
	loc.writeURL(path, survey, isPro)
}

func (loc *loconfer) writeURL(path string, survey *loconf.UninstallSurvey, isPro bool) {
	var url string
	if survey.Enabled {
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
	loc.mu.RLock()
	lang := loc.opts.Lang
	loc.mu.RUnlock()
	current, err := lc.GetAnnouncement(lang, isPro)
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
	return notifier.Show(note, "global-announcement")
}

func in(s string, coll []string) bool {
	for _, v := range coll {
		if s == v {
			return true
		}
	}
	return false
}
