package app

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/getlantern/errors"
	"github.com/getlantern/notifier"

	"github.com/getlantern/flashlight/ui"
)

const loConfURL = "https://raw.githubusercontent.com/getlantern/loconf/master/ui.json"
const stagingLoConfURL = "https://raw.githubusercontent.com/getlantern/loconf/master/ui-staging.json"

type announcement struct {
	Pro      bool   `json:"pro"`
	Free     bool   `json:"free"`
	Campaign string `json:"campaign"`
	Title    string `json:"title"`
	Message  string `json:"message"`
	URL      string `json:"url"`
}

type loConf struct {
	Announcement struct {
		Default string `json:"default"`
		a       map[string]announcement
	} `json:"announcement"`
}

func a(title, msg string) {
	logo := ui.AddToken("/img/lantern_logo.png")
	note := &notify.Notification{
		Title:    title,
		Message:  msg,
		ClickURL: ui.GetUIAddr(),
		IconURL:  logo,
	}
	_ = note
}

func fetch(u string) ([]byte, error) {
	resp, err := http.Get(u)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("Unexpected %v", resp.StatusCode)
	}
	return ioutil.ReadAll(resp.Body)
}

func parse(buf []byte, lang string) (map[string]interface{}, error) {
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
		return nil, errors.New("Incorrect type")
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
		return nil, errors.New("Incorrect type")
	} else {
		return inLang, nil
	}
}
