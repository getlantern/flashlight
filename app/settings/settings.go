package settings

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"sync"

	"github.com/getlantern/golog"
	"github.com/getlantern/launcher"
	"github.com/getlantern/uuid"
	"github.com/getlantern/yaml"

	"github.com/getlantern/flashlight/service"
	"github.com/getlantern/flashlight/ws"
)

// SettingName is the name of a setting.
type SettingName string

const (
	SNAutoReport  SettingName = "autoReport"
	SNAutoLaunch  SettingName = "autoLaunch"
	SNProxyAll    SettingName = "proxyAll"
	SNSystemProxy SettingName = "systemProxy"

	SNLanguage       SettingName = "language"
	SNLocalHTTPToken SettingName = "localHTTPToken"

	SNDeviceID          SettingName = "deviceID"
	SNUserID            SettingName = "userID"
	SNUserToken         SettingName = "userToken"
	SNTakenSurveys      SettingName = "takenSurveys"
	SNPastAnnouncements SettingName = "pastAnnouncements"

	SNAddr      SettingName = "addr"
	SNSOCKSAddr SettingName = "socksAddr"
	SNUIAddr    SettingName = "uiAddr"

	SNVersion      SettingName = "version"
	SNBuildDate    SettingName = "buildDate"
	SNRevisionDate SettingName = "revisionDate"
)

type settingType byte

const (
	stBool settingType = iota
	stNumber
	stString
	stStringArray
)

var settingMeta = map[SettingName]struct {
	sType     settingType
	persist   bool
	omitempty bool
}{
	SNAutoReport:  {stBool, true, false},
	SNAutoLaunch:  {stBool, true, false},
	SNProxyAll:    {stBool, true, false},
	SNSystemProxy: {stBool, true, false},

	SNLanguage:       {stString, true, true},
	SNLocalHTTPToken: {stString, true, true},

	// SNDeviceID: intentionally omit, to avoid setting it from UI
	SNUserID:            {stNumber, true, true},
	SNUserToken:         {stString, true, true},
	SNTakenSurveys:      {stStringArray, true, true},
	SNPastAnnouncements: {stStringArray, true, true},

	SNAddr:      {stString, true, true},
	SNSOCKSAddr: {stString, true, true},
	SNUIAddr:    {stString, true, true},

	SNVersion:      {stString, false, false},
	SNBuildDate:    {stString, false, false},
	SNRevisionDate: {stString, false, false},
}

var (
	log = golog.LoggerFor("flashlight.desktop.settings")

	ServiceType service.Type = "flashlight.desktop.settings"
)

// Settings is a struct of all settings unique to this particular Lantern instance.
type Settings struct {
	muNotifiers     sync.RWMutex
	changeNotifiers map[SettingName][]func(interface{})

	m map[SettingName]interface{}
	sync.RWMutex
	filePath string

	log golog.Logger
}

type ConfigOpts struct {
	Version      string
	RevisionDate string
	BuildDate    string
	FilePath     string
}

func (c *ConfigOpts) For() service.Type {
	return ServiceType
}

func (c *ConfigOpts) Complete() string {
	if c.Version == "" {
		return "missing Version"
	}

	if c.RevisionDate == "" {
		return "missing RevisionDate"
	}

	if c.BuildDate == "" {
		return "missing BuildDate"
	}

	if c.FilePath == "" {
		return "missing FilePath"
	}
	return ""
}

func New() *Settings {
	return &Settings{
		m: map[SettingName]interface{}{
			SNUserID:         int64(0),
			SNAutoReport:     true,
			SNAutoLaunch:     true,
			SNProxyAll:       false,
			SNSystemProxy:    true,
			SNLanguage:       "",
			SNLocalHTTPToken: "",
			SNUserToken:      "",
			SNUIAddr:         "",
		},
		filePath:        "/dev/null",
		changeNotifiers: make(map[SettingName][]func(interface{})),
		log:             golog.LoggerFor("app.settings"),
	}
}

func (s *Settings) Configure(opts service.ConfigOpts) {
	o := opts.(*ConfigOpts)
	s.filePath = o.FilePath
	log.Debug("Loading settings")
	// Use s.settings from disk if they're available.
	if bytes, err := ioutil.ReadFile(s.filePath); err != nil {
		s.log.Debugf("Could not read file %v", err)
	} else if err := yaml.Unmarshal(bytes, s.m); err != nil {
		s.log.Errorf("Could not load yaml %v", err)
		// Just keep going with the original s.settings not from disk.
	} else {
		s.log.Debugf("Loaded s.settings from %v", o.FilePath)
	}
	// old lantern persist s.settings with all lower case, convert them to camel cased.
	toCamelCase(s.m)

	// We always just s.m the device ID to the MAC address on the system. Note
	// this ignores what's on disk, if anything.
	s.m[SNDeviceID] = base64.StdEncoding.EncodeToString(uuid.NodeID())
	// always override below 3 attributes as they are not meant to be persisted across versions
	s.m[SNVersion] = o.Version
	s.m[SNBuildDate] = o.BuildDate
	s.m[SNRevisionDate] = o.RevisionDate

	// SNUserID may be unmarshalled as int, which causes panic when GetUserID().
	// Make sure to store it as int64.
	if id, ok := s.m[SNUserID].(int); ok {
		s.m[SNUserID] = int64(id)
	}
}

func (s *Settings) Start() {
	// Always just sync the auto-launch configuration on startup.
	go launcher.CreateLaunchFile(s.IsAutoLaunch())
}

// StartService starts the settings service that synchronizes Lantern's configuration with
// every UI client
func (s *Settings) StartService() error {
	helloFn := func(write func(interface{})) {
		s.log.Debugf("Sending Lantern settings to new client")
		write(s.uiMap())
	}

	service, err := ws.Register("settings", helloFn)
	if err != nil {
		return err
	}
	go s.read(service.In, service.Out)
	return nil
}

func (s *Settings) StopService() {
	ws.Unregister("settings")
}

func (s *Settings) read(in <-chan interface{}, out chan<- interface{}) {
	s.log.Debugf("Start reading settings messages!!")
	for message := range in {
		s.log.Debugf("Read settings message %v", message)

		data, ok := (message).(map[string]interface{})
		if !ok {
			continue
		}

		for k, v := range data {
			name := SettingName(k)
			t, exists := settingMeta[name]
			if !exists {
				s.log.Errorf("Unknown settings name %s", k)
				continue
			}
			switch t.sType {
			case stBool:
				s.setBool(name, v)
			case stString:
				s.SetString(name, v)
			case stNumber:
				s.setNum(name, v)
			case stStringArray:
				s.SetStringArray(name, v)
			}
		}

		out <- s.uiMap()
	}
}

func (s *Settings) setBool(name SettingName, v interface{}) {
	b, ok := v.(bool)
	if !ok {
		s.log.Errorf("Could not convert %s(%v) to bool", name, v)
		return
	}
	s.setVal(name, b)
}

func (s *Settings) setNum(name SettingName, v interface{}) {
	number, ok := v.(json.Number)
	if !ok {
		s.log.Errorf("Could not convert %v of type %v", name, reflect.TypeOf(v))
		return
	}
	bigint, err := number.Int64()
	if err != nil {
		s.log.Errorf("Could not get int64 value for %v with error %v", name, err)
		return
	}
	s.setVal(name, bigint)
}

func (s *Settings) SetStringArray(name SettingName, v interface{}) {
	sa, ok := v.([]string)
	if !ok {
		ss, ok := v.([]interface{})
		if !ok {
			s.log.Errorf("Could not convert %s(%v) to array", name, v)
			return
		}
		for i := range ss {
			sa = append(sa, fmt.Sprintf("%v", ss[i]))
		}
	}
	s.setVal(name, sa)
}

func (s *Settings) SetString(name SettingName, v interface{}) {
	str, ok := v.(string)
	if !ok {
		s.log.Errorf("Could not convert %s(%v) to string", name, v)
		return
	}
	s.setVal(name, str)
}

// save saves settings to disk.
func (s *Settings) save() {
	log.Trace("Saving settings")
	if f, err := os.Create(s.filePath); err != nil {
		s.log.Errorf("Could not open settings file for writing: %v", err)
	} else if _, err := s.WriteTo(f); err != nil {
		s.log.Errorf("Could not save settings file: %v", err)
	} else {
		log.Tracef("Saved settings to %s", s.filePath)
	}
}

func (s *Settings) WriteTo(w io.Writer) (int, error) {
	toBeSaved := s.mapToSave()
	if bytes, err := yaml.Marshal(toBeSaved); err != nil {
		return 0, err
	} else {
		return w.Write(bytes)
	}
}

func (s *Settings) mapToSave() map[string]interface{} {
	m := make(map[string]interface{})
	s.RLock()
	defer s.RUnlock()
	for k, v := range s.m {
		if settingMeta[k].persist {
			m[string(k)] = v
		}
	}
	return m
}

// uiMap makes a copy of our map for the UI with support for omitting empty
// values.
func (s *Settings) uiMap() map[string]interface{} {
	m := make(map[string]interface{})
	s.RLock()
	defer s.RUnlock()
	for key, v := range s.m {
		meta := settingMeta[key]
		k := string(key)
		// This mimics https://golang.org/pkg/encoding/json/ for what are considered
		// empty values.
		if !meta.omitempty {
			m[k] = v
		} else {
			if v == nil {
				continue
			}
			switch meta.sType {
			case stBool:
				if v.(bool) {
					m[k] = v
				}
			case stString:
				if v != "" {
					m[k] = v
				}
			case stStringArray:
				if a, ok := v.([]string); ok {
					m[k] = a
				}
			case stNumber:
				if v != 0 {
					m[k] = v
				}
			}
		}
	}
	return m
}

// GetTakenSurveys returns the IDs of surveys the user has already taken.
func (s *Settings) GetTakenSurveys() []string {
	return s.GetStringArray(SNTakenSurveys)
}

// SetTakenSurveys sets the IDs of taken surveys.
func (s *Settings) SetTakenSurveys(campaigns []string) {
	s.SetStringArray(SNTakenSurveys, campaigns)
}

// GetProxyAll returns whether or not to proxy all traffic.
func (s *Settings) GetProxyAll() bool {
	return s.GetBool(SNProxyAll)
}

// SetProxyAll sets whether or not to proxy all traffic.
func (s *Settings) SetProxyAll(proxyAll bool) {
	s.setVal(SNProxyAll, proxyAll)
}

// IsAutoReport returns whether or not to auto-report debugging and analytics data.
func (s *Settings) IsAutoReport() bool {
	return s.GetBool(SNAutoReport)
}

// IsAutoLaunch returns whether or not to automatically launch on system
// startup.
func (s *Settings) IsAutoLaunch() bool {
	return s.GetBool(SNAutoLaunch)
}

// SetLanguage sets the user language
func (s *Settings) SetLanguage(language string) {
	s.setVal(SNLanguage, language)
}

// GetLanguage returns the user language
func (s *Settings) GetLanguage() string {
	return s.GetString(SNLanguage)
}

// SetLocalHTTPToken sets the local HTTP token, stored on disk because we've
// seen weird issues on Windows where the OS remembers old, inactive PAC URLs
// with old tokens and uses them, breaking Edge and IE.
func (s *Settings) SetLocalHTTPToken(token string) {
	s.setVal(SNLocalHTTPToken, token)
}

// GetLocalHTTPToken returns the local HTTP token.
func (s *Settings) GetLocalHTTPToken() string {
	return s.GetString(SNLocalHTTPToken)
}

// SetUIAddr sets the last known UI address.
func (s *Settings) SetUIAddr(uiaddr string) {
	s.setVal(SNUIAddr, uiaddr)
}

// GetAddr gets the HTTP proxy address.
func (s *Settings) GetAddr() string {
	return s.GetString(SNAddr)
}

// GetUIAddr returns the address of the UI, stored across runs to avoid a
// different port on each run, which breaks things like local storage in the UI.
func (s *Settings) GetUIAddr() string {
	return s.GetString(SNUIAddr)
}

// GetDeviceID returns the unique ID of this device.
func (s *Settings) GetDeviceID() string {
	return s.GetString(SNDeviceID)
}

// GetToken returns the user token
func (s *Settings) GetToken() string {
	return s.GetString(SNUserToken)
}

// SetUserID sets the user ID
func (s *Settings) SetUserID(id int64) {
	s.setVal(SNUserID, id)
}

// GetUserID returns the user ID
func (s *Settings) GetUserID() int64 {
	return s.GetInt64(SNUserID)
}

// GetSystemProxy returns whether or not to set system proxy when lantern starts
func (s *Settings) GetSystemProxy() bool {
	return s.GetBool(SNSystemProxy)
}

func (s *Settings) GetBool(name SettingName) bool {
	if val, err := s.getVal(name); err == nil {
		if v, ok := val.(bool); ok {
			return v
		}
	}
	return false
}

func (s *Settings) GetStringArray(name SettingName) []string {
	if val, err := s.getVal(name); err == nil {
		if v, ok := val.([]string); ok {
			return v
		}
		if v, ok := val.([]interface{}); ok {
			var sa []string
			for _, item := range v {
				sa = append(sa, fmt.Sprintf("%v", item))
			}
			return sa
		}
	}
	return nil
}

func (s *Settings) GetString(name SettingName) string {
	if val, err := s.getVal(name); err == nil {
		if v, ok := val.(string); ok {
			return v
		}
	}
	return ""
}

func (s *Settings) GetInt64(name SettingName) int64 {
	if val, err := s.getVal(name); err == nil {
		if v, ok := val.(int64); ok {
			return v
		}
	}
	return int64(0)
}

func (s *Settings) getVal(name SettingName) (interface{}, error) {
	log.Tracef("Getting value for %v", name)
	s.RLock()
	defer s.RUnlock()
	if val, ok := s.m[name]; ok {
		return val, nil
	}
	s.log.Errorf("Could not get value for %s", name)
	return nil, fmt.Errorf("No value for %v", name)
}

func (s *Settings) setVal(name SettingName, val interface{}) {
	s.log.Debugf("Setting %v to %v in %v", name, val, s.m)
	s.Lock()
	s.m[name] = val
	// Need to unlock here because s.save() will lock again.
	s.Unlock()
	s.save()
	s.onChange(name, val)
}

// GetSetting gets the in memory setting with the name specified by attr
func (s *Settings) GetSetting(name SettingName) interface{} {
	if val, ok := settingMeta[name]; ok {
		switch val.sType {
		case stBool:
			return s.GetBool(name)
		case stNumber:
			return s.GetInt64(name)
		case stString:
			return s.GetString(name)
		}
	} else {
		s.log.Errorf("Looking for non-existent setting? %v", name)
	}

	// should never reach here.
	return nil
}

// OnChange sets a callback cb to get called when attr is changed from UI.
func (s *Settings) OnChange(attr SettingName, cb func(interface{})) {
	s.muNotifiers.Lock()
	s.changeNotifiers[attr] = append(s.changeNotifiers[attr], cb)
	s.muNotifiers.Unlock()
}

// onChange is called when attr is changed from UI
func (s *Settings) onChange(attr SettingName, value interface{}) {
	s.muNotifiers.RLock()
	notifiers := s.changeNotifiers[attr]
	s.muNotifiers.RUnlock()
	for _, fn := range notifiers {
		fn(value)
	}
}

func toCamelCase(m map[SettingName]interface{}) {
	for k := range settingMeta {
		lowerCased := SettingName(strings.ToLower(string(k)))
		if v, exists := m[lowerCased]; exists {
			delete(m, lowerCased)
			m[k] = v
		}
	}
}
