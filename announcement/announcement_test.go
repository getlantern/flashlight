package announcement

import (
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const normalBody string = `{
	"announcement": {
		"default": "en-US",
		"en-US": {
			"campaign": "20160801",
			"expiry": "",
			"pro": true,
			"free": true,
			"title": "Try out the new feature",
			"message": "Believe or not, you'll definitely love it!",
			"url": ""
		}
	},
	"survey": {
		"anything": "else"
	}
}`

func TestParseLoConf(t *testing.T) {
	testCases := []struct {
		body string
		lang string
		err  string
	}{
		{`{}`, "en-US", "No announcement section"},
		{`{ "announcement": "" }`, "zh-CN", "Incorrect type"},
		{`{
			"announcement": {
				"en-US": {}
			}
		}`, "zh-CN", "No announcement for zh-CN"},
		{`{
			"announcement": {
				"default": "en-US",
				"fr": ""
			}
		}`, "zh-CN", "No announcement for either zh-CN or en-US"},
		{`{
			"announcement": {
				"default": "en-US",
				"en-US": ""
			}
		}`, "zh-CN", "Incorrect type"},
		{`{
			"announcement": {
				"default": "en-US",
				"zh-CN": ""
			}
		}`, "zh-CN", "Incorrect type"},
		{normalBody, "en-US", ""},
		{normalBody, "zh-CN", ""},
	}
	for _, c := range testCases {
		if !doTestParse(t, c.body, c.lang, c.err) {
			t.Logf("%+v", c)
		}
	}
}

func doTestParse(t *testing.T, body, lang string, expected string) bool {
	_, err := parse([]byte(body), lang)
	if expected != "" {
		if assert.Error(t, err, "should have error") {
			return assert.Contains(t, err.Error(), expected)
		}
		return false
	}
	return assert.NoError(t, err, "should parse without error")
}

func TestParseAnnouncement(t *testing.T) {
	for _, lang := range []string{"en-US", "zh-CN"} {
		parsed, err := parse([]byte(normalBody), lang)
		if !assert.NoError(t, err) {
			continue
		}
		assert.Equal(t, parsed.Campaign, "20160801")
		assert.Equal(t, parsed.Pro, true)
		assert.Equal(t, parsed.Free, true)
		assert.Equal(t, parsed.Title, "Try out the new feature")
		assert.Equal(t, parsed.Message, "Believe or not, you'll definitely love it!")
		assert.Equal(t, parsed.URL, "")
	}
}

type mockTransport struct {
	body string
}

func (m mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode:    200,
		Header:        http.Header{},
		ContentLength: int64(len(m.body)),
		Body:          ioutil.NopCloser(strings.NewReader(m.body)),
	}, nil
}

func hcWithBody(body string) *http.Client {
	return &http.Client{
		Transport: mockTransport{
			body: body,
		},
	}
}

func TestUserType(t *testing.T) {
	_, err := Get(hcWithBody(normalBody), "en-US", false, false)
	assert.NoError(t, err,
		"getting announcement for free should have no error")
	_, err = Get(hcWithBody(normalBody), "en-US", true, false)
	assert.NoError(t, err,
		"getting announcement for pro should have no error")

	notForPro := strings.Replace(normalBody, `"pro": true`, `"pro": false`, 1)
	_, err = Get(hcWithBody(notForPro), "en-US", false, false)
	assert.NoError(t, err,
		"getting announcement for free should have no error")
	_, err = Get(hcWithBody(notForPro), "en-US", true, false)
	if assert.Error(t, err,
		"getting announcement for pro should have error") {
		assert.Contains(t, err.Error(), "No announcement available")
	}

	notForFree := strings.Replace(normalBody, `"free": true`, `"free": false`, 1)
	_, err = Get(hcWithBody(notForFree), "en-US", false, false)
	if assert.Error(t, err,
		"getting announcement for free should have error") {
		assert.Contains(t, err.Error(), "No announcement available")
	}
	_, err = Get(hcWithBody(notForFree), "en-US", true, false)
	assert.NoError(t, err,
		"getting announcement for pro should have no error")
}

func TestExpiry(t *testing.T) {
	today := `"expiry": "` + time.Now().Format(time.RFC822Z) + `"`
	expired := strings.Replace(normalBody, `"expiry": ""`, today, 1)
	_, err := Get(hcWithBody(expired), "en-US", false, false)
	if assert.Error(t, err,
		"expired announcement should have error") {
		assert.Contains(t, err.Error(), "No announcement available")
	}

	nextDay := `"expiry": "` + time.Now().Add(24*time.Hour).Format(time.RFC822Z) + `"`
	valid := strings.Replace(normalBody, `"expiry": ""`, nextDay, 1)
	_, err = Get(hcWithBody(valid), "en-US", false, false)
	assert.NoError(t, err,
		"not expired announcement should have no error")

	invalid := strings.Replace(normalBody, `"expiry": ""`, `"expiry": "9999-12-32"`, 1)
	_, err = Get(hcWithBody(invalid), "en-US", false, false)
	if assert.Error(t, err,
		"announcement with invalid expiry format should have error") {
		assert.Contains(t, err.Error(), "error parse expiry")
	}
}
