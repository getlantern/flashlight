package announcement

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/getlantern/errors"
)

const loConfURL = "https://raw.githubusercontent.com/getlantern/loconf/master/desktop-ui.json"
const stagingLoConfURL = "https://raw.githubusercontent.com/getlantern/loconf/master/ui-staging.json"

var ENoAvailable error = errors.New("No announcement available")

var eIncorrectType error = errors.New("Incorrect type")

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

func Get(hc *http.Client, lang string, isPro bool, isStaging bool) (*Announcement, error) {
	u := loConfURL
	if isStaging {
		u = stagingLoConfURL
	}
	b, efetch := fetch(hc, u)
	if efetch != nil {
		return nil, errors.Wrap(efetch)
	}
	parsed, eparse := parse(b, lang)
	if eparse != nil {
		return nil, errors.Wrap(eparse)
	}

	if parsed.Expiry != "" {
		expiry, eexpiry := time.Parse("2006-01-02", parsed.Expiry)
		if eexpiry != nil {
			return nil, errors.Wrap(eexpiry)
		}
		y, m, d := time.Now().Date()
		if !expiry.After(time.Date(y, m, d, 0, 0, 0, 0, time.UTC)) {
			return nil, ENoAvailable
		}
	}

	if isPro && parsed.Pro || !isPro && parsed.Free {
		return &parsed.Announcement, nil
	}
	return nil, ENoAvailable
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

func parse(buf []byte, lang string) (*announcement, error) {
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
	inLang, exist := announcement[lang]
	if !exist {
		defaultLoc := announcement["default"]
		if defaultLang, ok := defaultLoc.(string); !ok {
			return nil, errors.New("No announcement for %s", lang)
		} else if inLang, exist = announcement[defaultLang]; !exist {
			return nil, errors.New("No announcement for either %s or %s", lang, defaultLang)
		}
	}
	if inLang, ok := inLang.(map[string]interface{}); !ok {
		return nil, eIncorrectType
	} else {
		return mapToAnnouncement(inLang)
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
