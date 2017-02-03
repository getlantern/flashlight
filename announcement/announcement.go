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
//       "expiry": "02 Feb 17 15:04 +0700", # in RFC822Z format, or leave empty if not expire at all.
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
	ErrNoAvailable error = errors.New("No announcement available")

	eIncorrectType error = errors.New("Incorrect type")
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
		expiry, eexpiry := time.Parse(time.RFC822Z, parsed.Expiry)
		if eexpiry != nil {
			return nil, errors.New("error parse expiry: %v", eexpiry)
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

func fetch(hc *http.Client, u string) ([]byte, error) {
	req, ereq := http.NewRequest("GET", u, nil)
	if ereq != nil {
		return nil, ereq
	}
	resp, efetch := hc.Do(req)
	if efetch != nil {
		return nil, efetch
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("Unexpected status %v", resp.StatusCode)
	}
	return ioutil.ReadAll(resp.Body)
}

func parse(buf []byte, locale string) (*announcement, error) {
	obj := map[string]interface{}{}
	if ejson := json.Unmarshal(buf, &obj); ejson != nil {
		return nil, ejson
	}
	section, ok := obj["announcement"]
	if section == nil {
		return nil, errors.New("No announcement section")
	}
	announcement, ok := section.(map[string]interface{})
	if !ok {
		return nil, eIncorrectType
	}
	inLoc, exist := announcement[locale]
	if !exist {
		defaultLoc := announcement["default"]
		if defaultLocale, ok := defaultLoc.(string); !ok {
			return nil, errors.New("No announcement for %s", locale)
		} else if inLoc, exist = announcement[defaultLocale]; !exist {
			return nil, errors.New("No announcement for either %s or %s", locale, defaultLocale)
		}
	}
	if inLocale, ok := inLoc.(map[string]interface{}); !ok {
		return nil, eIncorrectType
	} else {
		return mapToAnnouncement(inLocale)
	}
}

func mapToAnnouncement(m map[string]interface{}) (*announcement, error) {
	var errorFields []string
	retval := &announcement{
		Pro:    exactBool(m, "pro", &errorFields),
		Free:   exactBool(m, "free", &errorFields),
		Expiry: exactString(m, "expiry", &errorFields),
		Announcement: Announcement{
			Campaign: exactString(m, "campaign", &errorFields),
			Title:    exactString(m, "title", &errorFields),
			Message:  exactString(m, "message", &errorFields),
			URL:      exactString(m, "url", &errorFields),
		},
	}
	if len(errorFields) > 0 {
		return nil, errors.New("Invalid fields %v", errorFields)
	}
	return retval, nil
}

func exactBool(m map[string]interface{}, key string, errorFields *[]string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	*errorFields = append(*errorFields, key)
	return false
}

func exactString(m map[string]interface{}, key string, errorFields *[]string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	*errorFields = append(*errorFields, key)
	return ""
}
