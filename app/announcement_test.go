package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const normalBody string = `{
	"announcement": {
		"default": "en-US",
		"en-US": {
			"enabled": false,
			"campaign": "20160801",
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

func TestParseAnnouncement(t *testing.T) {
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
