// package announcement gets announcement from the loconf JSON endpoint. The
// format is as follows:
//
// {
//   "announcement": {
//     "default": "en-US", # when there's no section for the specific user locale, fallback to default. Can be empty.
//     "en-US": {
//       "campaign": "20160801-new-feature", # uniquely identify an announcement.
//       "pro": true, # true to show announcement for pro users.
//       "free": true, # true to show announcement for free users.
//       "expiry": "2018-02-02T15:00:00+07:00", # in RFC3339 format, or leave empty if not expire at all.
//       "title": "Try out the new feature",
//       "message": "Believe or not, you'll definitely love it!",
//       "url": ""
//     },
//     "zh-CN": {
//       ...
//     },
//     ... # for other locales
//   },
//   ... # other parts of the JSON file
// }
package announcement

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/getlantern/errors"
)

const (
	loConfURL        = "https://raw.githubusercontent.com/getlantern/loconf/master/desktop-ui.json"
	stagingLoConfURL = "https://raw.githubusercontent.com/getlantern/loconf/master/ui-staging.json"
)

var (
	// ErrNoAvailable indicates that there's no valid announcement for the
	// current user.
	ErrNoAvailable error = errors.New("no announcement available")
)

// Announcement is what caller get when there's not-expired announcement for
// the current user.
type Announcement struct {
	Campaign string `json:"campaign"`
	Title    string `json:"title"`
	Message  string `json:"message"`
	URL      string `json:"url"`
}

type announcement struct {
	Pro    bool   `json:"pro"`
	Free   bool   `json:"free"`
	Expiry string `json:"expiry"`
	Announcement
}

// Get gets announcement via the HTTP client, based on the locale, pro and staging flag.
func Get(hc *http.Client, locale string, isPro bool, isStaging bool) (*Announcement, error) {
	u := loConfURL
	if isStaging {
		u = stagingLoConfURL
	}
	b, efetch := fetch(hc, u)
	if efetch != nil {
		return nil, errors.Wrap(efetch)
	}
	parsed, eparse := parse(b, locale)
	if eparse != nil {
		return nil, errors.Wrap(eparse)
	}

	if parsed.Expiry != "" {
		expiry, eexpiry := time.Parse(time.RFC3339, parsed.Expiry)
		if eexpiry != nil {
			return nil, errors.New("error parsing expiry: %v", eexpiry)
		}
		if expiry.Before(time.Now()) {
			return nil, ErrNoAvailable
		}
	}

	if isPro && parsed.Pro || !isPro && parsed.Free {
		return &parsed.Announcement, nil
	}
	return nil, ErrNoAvailable
}

func fetch(hc *http.Client, u string) (b []byte, err error) {
	req, ereq := http.NewRequest("GET", u, nil)
	if ereq != nil {
		return nil, ereq
	}
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

type loconf struct {
	Announcement map[string]json.RawMessage `json:"announcement"`
	// survey, etc, ignored.
}

func parse(buf []byte, locale string) (*announcement, error) {
	obj := loconf{}
	if ejson := json.Unmarshal(buf, &obj); ejson != nil {
		return nil, errors.New("error parsing \"announcement\" section: %v", ejson)
	}
	if len(obj.Announcement) == 0 {
		return nil, errors.New("no announcement section")
	}
	section, exist := obj.Announcement[locale]
	if !exist {
		defaultField, hasDefault := obj.Announcement["default"]
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
		if section, exist = obj.Announcement[defaultLocale]; !exist {
			return nil, errors.New("no announcement for either %s or %s", locale, defaultLocale)
		}
		locale = defaultLocale
	}
	var inLocale announcement
	if e := json.Unmarshal(section, &inLocale); e != nil {
		return nil, errors.New("error parsing \"%v\" section: %v", locale, e)
	}
	return &inLocale, nil
}
