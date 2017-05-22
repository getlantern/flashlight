package settings

import (
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/getlantern/uuid"
	"github.com/stretchr/testify/assert"
)

func TestRead(t *testing.T) {
	// Avoid polluting real settings.
	tmpfile, err := ioutil.TempFile("", "test")
	if err != nil {
		t.Errorf("Could not create temp file %v", err)
	}
	defer os.Remove(tmpfile.Name())

	var uid int64
	s := New("1", "1/1/1", "1/1/1", tmpfile.Name())
	assert.Equal(t, s.GetProxyAll(), false)
	assert.Equal(t, s.GetUserID(), uid)
	assert.Equal(t, s.GetSystemProxy(), true)
	assert.Equal(t, s.IsAutoReport(), true)
	assert.Equal(t, s.GetLanguage(), "")
	assert.Equal(t, s.GetLanguage(), "")
	assert.Equal(t, s.GetUIAddr(), "")
	assert.Equal(t, s.GetLocalHTTPToken(), "")
	assert.Equal(t, s.GetTakenSurveys(), []string(nil))

	// Start with raw JSON so we actually decode the map from scratch, as that
	// will then simulate real world use where we rely on Go to generate the
	// actual types of the JSON values. For example, all numbers will be
	// decoded as float64.
	var data = `{
		"autoReport": false,
		"proxyAll": true,
		"autoLaunch": false,
		"systemProxy": false,
		"deviceID": "8208fja09493",
		"userID": 890238588,
		"language": "en-US",
		"takenSurveys": ["foo", "bar"],
		"localHTTPToken": "0jafdjlka",
		"uiAddr": "127.0.0.1:1234"
	}`

	var m map[string]interface{}
	d := json.NewDecoder(strings.NewReader(data))

	// Make sure to use json.Number here to avoid issues with 64 bit integers.
	d.UseNumber()
	err = d.Decode(&m)

	in := make(chan interface{}, 100)
	in <- m
	out := make(chan interface{})
	go s.read(in, out)

	<-out

	uid = 890238588
	assert.Equal(t, s.GetProxyAll(), true)
	assert.Equal(t, s.GetSystemProxy(), false)
	assert.Equal(t, s.IsAutoReport(), false)
	assert.Equal(t, s.GetUserID(), uid)
	assert.Equal(t, s.GetDeviceID(), base64.StdEncoding.EncodeToString(uuid.NodeID()))
	assert.Equal(t, s.GetLanguage(), "en-US")
	assert.Equal(t, s.GetUIAddr(), "127.0.0.1:1234")
	assert.Equal(t, s.GetLocalHTTPToken(), "0jafdjlka")
	assert.Equal(t, s.GetTakenSurveys(), []string{"foo", "bar"})

	// Test that setting something random doesn't break stuff.
	m["randomjfdklajfla"] = "fadldjfdla"

	// Test tokens while we're at it.
	token := "token"
	m["userToken"] = token
	in <- m
	<-out
	assert.Equal(t, s.GetProxyAll(), true)
	assert.Equal(t, s.GetToken(), token)

	// Test with an actual user ID.
	var id json.Number = "483109"
	var expected int64 = 483109
	m["userID"] = id
	in <- m
	<-out
	assert.Equal(t, expected, s.GetUserID())
	assert.Equal(t, true, s.GetProxyAll())
}

func TestSetNum(t *testing.T) {
	snTest := SettingName("test")
	set := New("", "", "", "/dev/null")
	var val json.Number = "4809"
	var expected int64 = 4809
	set.setNum(snTest, val)
	assert.Equal(t, expected, set.m[snTest])

	set.SetString(snTest, val)

	// The above should not have worked since it's not a string -- should
	// still be an int64
	assert.Equal(t, expected, set.m[snTest])

	set.setBool(snTest, val)

	// The above should not have worked since it's not a bool -- should
	// still be an int64
	assert.Equal(t, expected, set.m[snTest])
}

func TestStringArray(t *testing.T) {
	set := New("", "", "", "/dev/null")
	assert.Nil(t, set.GetStringArray("key"), "string array should be nil initially")
	set.SetStringArray("key", []string{"value"})
	assert.Equal(t, []string{"value"}, set.GetStringArray("key"), "should set string array")
	set.SetStringArray("key", []string{"value2"})
	assert.Equal(t, []string{"value2"}, set.GetStringArray("key"),
		"should replace existing array altogether")
	set.SetStringArray("key", []interface{}{"value3"})
	assert.Equal(t, []string{"value3"}, set.GetStringArray("key"),
		"should accept []interface{}")

	set.setVal("key", []interface{}{"value3"})
	assert.Equal(t, []string{"value3"}, set.GetStringArray("key"),
		"should read value from []interface{} too")
}

func TestPersistAndLoad(t *testing.T) {
	version := "version-not-on-disk"
	revisionDate := "1970-1-1"
	buildDate := "1970-1-1"
	yamlFile := "./test.yaml"
	s := New(version, revisionDate, buildDate, yamlFile)
	assert.Equal(t, version, s.m["version"], "Should be set to version")
	assert.Equal(t, revisionDate, s.m["revisionDate"], "Should be set to revisionDate")
	assert.Equal(t, buildDate, s.m["buildDate"], "Should be set to buildDate")
	assert.Equal(t, "en", s.GetLanguage(), "Should load language from file")
	assert.Equal(t, int64(1), s.GetUserID(), "Should load user id from file")

	s.SetLanguage("leet")
	s.SetUserID(1234)
	s2 := New(version, revisionDate, buildDate, yamlFile)
	assert.Equal(t, "leet", s2.GetLanguage(), "Should save language to file and reload")
	assert.Equal(t, int64(1234), s2.GetUserID(), "Should save user id to file and reload")
	s2.SetLanguage("en")
	s2.SetUserID(1)
}

func TestLoadLowerCased(t *testing.T) {
	s := New("", "", "", "./lowercased.yaml")
	assert.Equal(t, int64(1234), s.GetUserID(), "Should load user id from lower cased yaml")
	assert.Equal(t, "abcd", s.GetToken(), "Should load user token from lower cased yaml")
}

func TestOnChange(t *testing.T) {
	set := New("", "", "", "/dev/null")
	in := make(chan interface{})
	out := make(chan interface{})
	var c1, c2 string
	set.OnChange(SNLanguage, func(v interface{}) {
		c1 = v.(string)
	})
	set.OnChange(SNLanguage, func(v interface{}) {
		c2 = v.(string)
	})
	go func() {
		set.read(in, out)
	}()
	in <- map[string]interface{}{"language": "en"}
	_ = <-out
	assert.Equal(t, "en", c1, "should call OnChange callback")
	assert.Equal(t, "en", c2, "should call all OnChange callbacks")
}

func TestInvalidType(t *testing.T) {
	set := New("", "", "", "/dev/null")
	set.setVal("test", nil)
	assert.Equal(t, "", set.GetString("test"))
	assert.Equal(t, false, set.GetBool("test"))
	assert.Equal(t, int64(0), set.GetInt64("test"))
	assert.Equal(t, []string(nil), set.GetStringArray("test"))
}
