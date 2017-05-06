package loconf

import (
	"encoding/json"
	"io/ioutil"
	"math/rand"
	"net/http"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
)

const (
	loConfURL        = "https://raw.githubusercontent.com/getlantern/loconf/master/desktop-ui.json"
	stagingLoConfURL = "https://raw.githubusercontent.com/getlantern/loconf/master/ui-staging.json"
)

var (
	// ErrNoAvailable indicates that there's no valid announcement for the
	// current user.
	ErrNoAvailable error = errors.New("no announcement available")

	log = golog.LoggerFor("loconf")

	r = rand.New(rand.NewSource(time.Now().UnixNano()))
)

// LoConf is a struct representing the locale-based configuration file data.
type LoConf struct {
	Surveys          map[string]*Survey          `json:"survey,omitempty"`
	Announcements    map[string]json.RawMessage  `json:"announcement,omitempty"`
	UninstallSurveys map[string]*UninstallSurvey `json:"uninstall-survey,omitempty"`
	log              golog.Logger
}

// BaseSurvey contains the core elements of any survey type.
type BaseSurvey struct {
	Enabled     bool    `json:"enabled,omitempty"`
	Probability float64 `json:"probability,omitempty"`
	Campaign    string  `json:"campaign,omitempty"`
	URL         string  `json:"url,omitempty"`
	Pro         bool    `json:"pro,omitempty"`
	Free        bool    `json:"free,omitempty"`
}

// UninstallSurvey contains all elements of uninstall surveys.
type UninstallSurvey BaseSurvey

// Survey contains all elements of standard surveys.
type Survey struct {
	BaseSurvey
	Message string `json:"message,omitempty"`
	Thanks  string `json:"thanks,omitempty"`
	Button  string `json:"button,omitempty"`
}

// Announcement is what caller get when there's not-expired announcement for
// the current user.
type Announcement struct {
	Campaign string `json:"campaign,omitempty"`
	Title    string `json:"title,omitempty"`
	Message  string `json:"message,omitempty"`
	URL      string `json:"url,omitempty"`
	Pro      bool   `json:"pro,omitempty"`
	Free     bool   `json:"free,omitempty"`
	Expiry   string `json:"expiry,omitempty"`
}

// Get gets announcement via the HTTP client, based on the locale and staging flags.
func Get(hc *http.Client, isStaging bool) (*LoConf, error) {
	return get(hc, isStaging, loConfURL, stagingLoConfURL)
}

// get gets announcement via the HTTP client, based on the locale and staging flags.
func get(hc *http.Client, isStaging bool, prodURL, stagingURL string) (*LoConf, error) {
	u := prodURL
	if isStaging {
		u = stagingURL
	}
	b, efetch := fetch(hc, u)
	if efetch != nil {
		return nil, errors.Wrap(efetch)
	}
	parsed, eparse := parse(b)
	if eparse != nil {
		return nil, errors.Wrap(eparse)
	}
	return parsed, nil
}

func fetch(hc *http.Client, u string) (b []byte, err error) {
	req, ereq := http.NewRequest("GET", u, nil)
	if ereq != nil {
		return nil, ereq
	}

	// Prevents intermediate nodes (domain-fronters) from caching the content
	req.Header.Set("Cache-Control", "no-cache")

	resp, efetch := hc.Do(req)
	if efetch != nil {
		return nil, efetch
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("unexpected status %v", resp.StatusCode)
	}
	b, err = ioutil.ReadAll(resp.Body)
	_ = resp.Body.Close() // can do nothing about the error
	return
}

func parse(buf []byte) (*LoConf, error) {
	obj := LoConf{log: golog.LoggerFor("flashlight.loconf")}
	if ejson := json.Unmarshal(buf, &obj); ejson != nil {
		log.Errorf("Could not parse JSON %v", ejson)
		return nil, errors.New("error parsing loconf section: %v", ejson)
	}
	return &obj, nil
}

// GetSurvey returns the uninstall survey for the specified locale and
// pro state, or an error if no match exists.
func (lc *LoConf) GetSurvey(locale string) (*Survey, bool) {
	val, ok := lc.Surveys[locale]
	return val, ok
}

// GetUninstallSurvey returns the uninstall survey for the specified locale and
// pro state, or an error if no match exists.
func (lc *LoConf) GetUninstallSurvey(locale string) (*UninstallSurvey, bool) {
	val, ok := lc.UninstallSurveys[locale]
	return val, ok
}

// GetAnnouncement returns the announcement for the specified locale, pro, etc
// or returns an error if no announcement is availabe.
func (lc *LoConf) GetAnnouncement(locale string, isPro bool) (*Announcement, error) {
	if len(lc.Announcements) == 0 {
		return nil, errors.New("no announcement section")
	}
	section, exist := lc.Announcements[locale]
	if !exist {
		defaultField, hasDefault := lc.Announcements["default"]
		if !hasDefault {
			return nil, ErrNoAvailable
		}
		var defaultLocale string
		if e := json.Unmarshal(defaultField, &defaultLocale); e != nil {
			return nil, errors.New("error parsing \"default\" field: %v", e)
		}
		if defaultLocale == "" {
			return nil, ErrNoAvailable
		}
		if section, exist = lc.Announcements[defaultLocale]; !exist {
			return nil, errors.New("no announcement for either %s or %s", locale, defaultLocale)
		}
		locale = defaultLocale
	}
	var inLocale Announcement
	if e := json.Unmarshal(section, &inLocale); e != nil {
		return nil, errors.New("error parsing \"%v\" section: %v", locale, e)
	}

	if inLocale.Expiry != "" {
		expiry, eexpiry := time.Parse(time.RFC3339, inLocale.Expiry)
		if eexpiry != nil {
			return nil, errors.New("error parsing expiry: %v", eexpiry)
		}
		if expiry.Before(time.Now()) {
			return nil, ErrNoAvailable
		}
	}

	if isPro && inLocale.Pro || !isPro && inLocale.Free {
		return &inLocale, nil
	}
	return nil, ErrNoAvailable
}
