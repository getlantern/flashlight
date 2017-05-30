package localurl

import (
	"net/url"
	"runtime"

	"github.com/getlantern/flashlight/util"
	"github.com/getlantern/golog"
)

type localurl struct {
	token string
	log   golog.Logger
}

type tokens interface {
	GetLocalHTTPToken() string
	SetLocalHTTPToken(string)
}

type LocalURL func(url, campaign, content, medium string) (string, error)

func NewLocalURL(toks tokens) LocalURL {
	l := &localurl{
		log:   golog.LoggerFor("flashlight.localurl"),
		token: getAndSetToken(toks),
	}
	return l.getURL
}

func (l *localurl) getURL(url, campaign, content, medium string) (string, error) {
	withToken := l.addToken(url)
	return l.addCampaign(withToken, campaign, content, medium)
}

// addCampaign adds Google Analytics campaign tracking to a URL and returns
// that URL.
func (l *localurl) addCampaign(urlStr, campaign, content, medium string) (string, error) {
	tokenized := l.addToken(urlStr)
	u, err := url.Parse(tokenized)
	if err != nil {
		l.log.Errorf("Could not parse click URL: %v", err)
		return "", err
	}

	q := u.Query()
	q.Set("utm_source", runtime.GOOS)
	q.Set("utm_medium", medium)
	q.Set("utm_campaign", campaign)
	q.Set("utm_content", content)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// addToken adds the UI domain and custom request token to the specified
// request URL. Without that token, the backend will reject the request to
// avoid web sites detecting Lantern.
func (l *localurl) addToken(url string) string {
	return util.SetURLParam(url, "token", l.token)
}

// getAndSetToken fetches the local HTTP token from disk if it's there, and
// otherwise creates a new one and stores it.
func getAndSetToken(set tokens) string {
	tok := set.GetLocalHTTPToken()
	if tok == "" {
		t := localHTTPToken()
		set.SetLocalHTTPToken(t)
		return t
	}
	return tok
}
