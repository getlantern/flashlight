package app

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/getlantern/appdir"
	"github.com/getlantern/launcher"
	"github.com/getlantern/uuid"
	"github.com/getlantern/yaml"

	"github.com/getlantern/flashlight/logging"
	"github.com/getlantern/flashlight/ui"
)

// SettingName is the name of a setting.
type SettingName string

const (
	SNAutoReport  SettingName = "autoReport"
	SNAutoLaunch  SettingName = "autoLaunch"
	SNProxyAll    SettingName = "proxyAll"
	SNSystemProxy SettingName = "systemProxy"

	SNLanguage SettingName = "language"

	SNDeviceID     SettingName = "deviceID"
	SNUserID       SettingName = "userID"
	SNUserToken    SettingName = "userToken"
	SNTakenSurveys SettingName = "takenSurveys"

	SNAddr      SettingName = "addr"
	SNSOCKSAddr SettingName = "socksAddr"
	SNUIAddr    SettingName = "uiAddr"

	SNVersion        SettingName = "version"
	SNBuildDate      SettingName = "buildDate"
	SNRevisionDate   SettingName = "revisionDate"
	SNLocalHTTPToken SettingName = "localHTTPToken"
)

type settingType byte

const (
	stBool settingType = iota
	stNumber
	stString
	stStringArray
)

const (
	messageType = `settings`
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

	SNLanguage: {stString, true, true},

	// SNDeviceID: intentionally omit, to avoid setting it from UI
	SNUserID:         {stNumber, true, true},
	SNUserToken:      {stString, true, true},
	SNTakenSurveys:   {stStringArray, true, true},
	SNLocalHTTPToken: {stString, true, true},

	SNAddr:      {stString, true, true},
	SNSOCKSAddr: {stString, true, true},
	SNUIAddr:    {stString, true, true},

	SNVersion:      {stString, false, false},
	SNBuildDate:    {stString, false, false},
	SNRevisionDate: {stString, false, false},
}

var (
	service     *ui.Service
	httpClient  *http.Client
	defaultPath = filepath.Join(appdir.General("Lantern"), "settings.yaml")
	once        = &sync.Once{}
)

// Settings is a struct of all settings unique to this particular Lantern instance.
type Settings struct {
	muNotifiers     sync.RWMutex
	changeNotifiers map[SettingName][]func(interface{})

	m map[SettingName]interface{}
	sync.RWMutex
	filePath string
}

func loadSettings(version, revisionDate, buildDate string) *Settings {
	return loadSettingsFrom(version, revisionDate, buildDate, defaultPath)
}

// loadSettings loads the initial settings at startup, either from disk or using defaults.
func loadSettingsFrom(version, revisionDate, buildDate, path string) *Settings {
	log.Debug("Loading settings")
	// Create default settings that may or may not be overridden from an existing file
	// on disk.
	sett := newSettings(path)
	set := sett.m

	// Use settings from disk if they're available.
	if bytes, err := ioutil.ReadFile(path); err != nil {
		log.Debugf("Could not read file %v", err)
	} else if err := yaml.Unmarshal(bytes, set); err != nil {
		log.Errorf("Could not load yaml %v", err)
		// Just keep going with the original settings not from disk.
	} else {
		log.Debugf("Loaded settings from %v", path)
	}
	// old lantern persist settings with all lower case, convert them to camel cased.
	toCamelCase(set)

	// We always just set the device ID to the MAC address on the system. Note
	// this ignores what's on disk, if anything.
	set[SNDeviceID] = base64.StdEncoding.EncodeToString(uuid.NodeID())

	// SNUserID may be unmarshalled as int, which causes panic when GetUserID().
	// Make sure to store it as int64.
	switch id := set[SNUserID].(type) {
	case int:
		set[SNUserID] = int64(id)
	}

	// Always just sync the auto-launch configuration on startup.
	go launcher.CreateLaunchFile(sett.IsAutoLaunch())

	// always override below 3 attributes as they are not meant to be persisted across versions
	set[SNVersion] = version
	set[SNBuildDate] = buildDate
	set[SNRevisionDate] = revisionDate

	// Only configure the UI once. This will typically be the case in the normal
	// application flow, but tests might call Load twice, for example, which we
	// want to allow.
	once.Do(func() {
		err := sett.start()
		if err != nil {
			log.Errorf("Unable to register settings service: %q", err)
			return
		}
		go sett.read(service.In, service.Out)
	})
	return sett
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

func newSettings(filePath string) *Settings {
	return &Settings{
		m: map[SettingName]interface{}{
			SNUserID:         int64(0),
			SNAutoReport:     true,
			SNAutoLaunch:     true,
			SNProxyAll:       false,
			SNSystemProxy:    true,
			SNLanguage:       "",
			SNUserToken:      "",
			SNUIAddr:         "",
			SNLocalHTTPToken: "",
		},
		filePath:        filePath,
		changeNotifiers: make(map[SettingName][]func(interface{})),
	}
}

// start the settings service that synchronizes Lantern's configuration with every UI client
func (s *Settings) start() error {
	var err error

	ui.PreferProxiedUI(s.GetSystemProxy())
	helloFn := func(write func(interface{}) error) error {
		log.Debugf("Sending Lantern settings to new client")
		uiMap := s.uiMap()
		return write(uiMap)
	}
	service, err = ui.Register(messageType, helloFn)
	s.OnChange(SNSystemProxy, func(val interface{}) {
		enable := val.(bool)
		preferredUIAddr, addrChanged := ui.PreferProxiedUI(enable)
		if !enable && addrChanged {
			log.Debugf("System proxying disabled, redirect UI to: %v", preferredUIAddr)
			service.Out <- map[string]string{"redirectTo": preferredUIAddr}
		}
	})
	return err
}

func (s *Settings) read(in <-chan interface{}, out chan<- interface{}) {
	log.Debugf("Start reading settings messages!!")
	for message := range in {
		log.Debugf("Read settings message %v", message)

		data, ok := (message).(map[string]interface{})
		if !ok {
			continue
		}

		for k, v := range data {
			name := SettingName(k)
			t, exists := settingMeta[name]
			if !exists {
				log.Errorf("Unknown settings name %s", k)
				continue
			}
			switch t.sType {
			case stBool:
				s.setBool(name, v)
			case stString:
				s.setString(name, v)
			case stNumber:
				s.setNum(name, v)
			case stStringArray:
				s.setStringArray(name, v)
			}
		}

		out <- s
	}
}

func (s *Settings) setBool(name SettingName, v interface{}) {
	b, ok := v.(bool)
	if !ok {
		log.Errorf("Could not convert %s(%v) to bool", name, v)
		return
	}
	if name == SNAutoReport {
		logging.SetReportingEnabled(b)
	}
	s.setVal(name, b)
}

func (s *Settings) setNum(name SettingName, v interface{}) {
	number, ok := v.(json.Number)
	if !ok {
		log.Errorf("Could not convert %v of type %v", name, reflect.TypeOf(v))
		return
	}
	bigint, err := number.Int64()
	if err != nil {
		log.Errorf("Could not get int64 value for %v with error %v", name, err)
		return
	}
	s.setVal(name, bigint)
}

func (s *Settings) setStringArray(name SettingName, v interface{}) {
	var sa []string
	ss, ok := v.([]interface{})
	if !ok {
		log.Errorf("Could not convert %s(%v) to array", name, v)
		return
	}
	for i := range ss {
		sa = append(sa, fmt.Sprintf("%v", ss[i]))
	}
	s.setVal(name, sa)
}

func (s *Settings) setString(name SettingName, v interface{}) {
	str, ok := v.(string)
	if !ok {
		log.Errorf("Could not convert %s(%v) to string", name, v)
		return
	}
	s.setVal(name, str)
}

// save saves settings to disk.
func (s *Settings) save() {
	log.Trace("Saving settings")
	if f, err := os.Create(s.filePath); err != nil {
		log.Errorf("Could not open settings file for writing: %v", err)
	} else if _, err := s.writeTo(f); err != nil {
		log.Errorf("Could not save settings file: %v", err)
	} else {
		log.Tracef("Saved settings to %s", s.filePath)
	}
}

func (s *Settings) writeTo(w io.Writer) (int, error) {
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
	return s.getStringArray(SNTakenSurveys)
}

// SetTakenSurveys sets the IDs of taken surveys.
func (s *Settings) SetTakenSurveys(campaigns []string) {
	s.setVal(SNTakenSurveys, campaigns)
}

// GetProxyAll returns whether or not to proxy all traffic.
func (s *Settings) GetProxyAll() bool {
	return s.getBool(SNProxyAll)
}

// SetUIAddr sets the last known UI address.
func (s *Settings) SetUIAddr(uiaddr string) {
	s.setVal(SNUIAddr, uiaddr)
}

// SetProxyAll sets whether or not to proxy all traffic.
func (s *Settings) SetProxyAll(proxyAll bool) {
	s.setVal(SNProxyAll, proxyAll)
	// Cycle the PAC file so that browser picks up changes
	cyclePAC()
}

// IsAutoReport returns whether or not to auto-report debugging and analytics data.
func (s *Settings) IsAutoReport() bool {
	return s.getBool(SNAutoReport)
}

// IsAutoLaunch returns whether or not to automatically launch on system
// startup.
func (s *Settings) IsAutoLaunch() bool {
	return s.getBool(SNAutoLaunch)
}

// SetLanguage sets the user language
func (s *Settings) SetLanguage(language string) {
	s.setVal(SNLanguage, language)
}

// GetLanguage returns the user language
func (s *Settings) GetLanguage() string {
	return s.getString(SNLanguage)
}

// SetLocalHTTPToken sets the local HTTP token, stored on disk because we've
// seen weird issues on Windows where the OS remembers old, inactive PAC URLs
// with old tokens and uses them, breaking Edge and IE.
func (s *Settings) SetLocalHTTPToken(token string) {
	s.setVal(SNLocalHTTPToken, token)
}

// GetLocalHTTPToken returns the local HTTP token.
func (s *Settings) GetLocalHTTPToken() string {
	return s.getString(SNLocalHTTPToken)
}

// GetUIAddr returns the address of the UI, stored across runs to avoid a
// different port on each run, which breaks things like local storage in the UI.
func (s *Settings) GetUIAddr() string {
	return s.getString(SNUIAddr)
}

// GetDeviceID returns the unique ID of this device.
func (s *Settings) GetDeviceID() string {
	return s.getString(SNDeviceID)
}

// GetToken returns the user token
func (s *Settings) GetToken() string {
	return s.getString(SNUserToken)
}

// SetUserID sets the user ID
func (s *Settings) SetUserID(id int64) {
	s.setVal(SNUserID, id)
}

// GetUserID returns the user ID
func (s *Settings) GetUserID() int64 {
	return s.getInt64(SNUserID)
}

// GetSystemProxy returns whether or not to set system proxy when lantern starts
func (s *Settings) GetSystemProxy() bool {
	return s.getBool(SNSystemProxy)
}

func (s *Settings) getBool(name SettingName) bool {
	if val, err := s.getVal(name); err == nil {
		if v, ok := val.(bool); ok {
			return v
		}
	}
	return false
}

func (s *Settings) getStringArray(name SettingName) []string {
	if val, err := s.getVal(name); err == nil {
		if v, ok := val.([]string); ok {
			return v
		}
	}
	return nil
}

func (s *Settings) getString(name SettingName) string {
	if val, err := s.getVal(name); err == nil {
		if v, ok := val.(string); ok {
			return v
		}
	}
	return ""
}

func (s *Settings) getInt64(name SettingName) int64 {
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
	log.Errorf("Could not get value for %s", name)
	return nil, fmt.Errorf("No value for %v", name)
}

func (s *Settings) setVal(name SettingName, val interface{}) {
	log.Debugf("Setting %v to %v in %v", name, val, s.m)
	s.Lock()
	s.m[name] = val
	// Need to unlock here because s.save() will lock again.
	s.Unlock()
	s.save()
	s.onChange(name, val)
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
