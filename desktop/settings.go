package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/eventual"
	"github.com/getlantern/golog"
	"github.com/getlantern/launcher"
	"github.com/getlantern/timezone"
	"github.com/spf13/viper"

	"github.com/getlantern/flashlight/desktop/deviceid"
	"github.com/getlantern/flashlight/ws"
)

// SettingName is the name of a setting.
type SettingName string

const (
	SNAutoReport   SettingName = "autoReport"
	SNAutoLaunch   SettingName = "autoLaunch"
	SNProxyAll     SettingName = "proxyAll"
	SNSystemProxy  SettingName = "systemProxy"
	SNDisconnected SettingName = "disconnected"

	SNLanguage       SettingName = "lang"
	SNLocalHTTPToken SettingName = "localHTTPToken"

	SNDeviceID                  SettingName = "deviceID"
	SNUserID                    SettingName = "userID"
	SNUserToken                 SettingName = "userToken"
	SNMigratedDeviceIDForUserID SettingName = "migratedDeviceIDForUserID"
	SNTakenSurveys              SettingName = "takenSurveys"
	SNPastAnnouncements         SettingName = "pastAnnouncements"

	SNAddr      SettingName = "addr"
	SNSOCKSAddr SettingName = "socksAddr"
	SNUIAddr    SettingName = "uiAddr"

	SNVersion      SettingName = "version"
	SNBuildDate    SettingName = "buildDate"
	SNRevisionDate SettingName = "revisionDate"
)

// Settings is a struct of all settings unique to this particular Lantern instance.
type Settings struct {
	muNotifiers     sync.RWMutex
	changeNotifiers map[SettingName][]func(interface{})
	wsOut           chan<- interface{}
	sync.RWMutex
	filePath string

	log golog.Logger

	chrome chromeExtension
	config *viper.Viper
}

func loadSettingsFrom(version, revisionDate, buildDate, path string, chrome chromeExtension) *Settings {
	// Create default settings that may or may not be overridden from an existing file
	// on disk.
	sett := newSettings(path, chrome)

	// old lantern persist settings with all lower case, convert them to camel cased.
	// toCamelCase(set)
	sett.setVal(SNDeviceID, deviceid.Get())

	// Always just sync the auto-launch configuration on startup.
	go launcher.CreateLaunchFile(sett.IsAutoLaunch())

	// always override below 3 attributes as they are not meant to be persisted across versions
	sett.setVals(map[SettingName]interface{}{
		SNVersion:      version,
		SNBuildDate:    buildDate,
		SNRevisionDate: revisionDate,
	})

	// Disable for now to prevent performance issue particularly on Windows 7
	// go chrome.save(sett.mapToSave)
	return sett
}

// func toCamelCase(m map[SettingName]interface{}) {
// 	for k := range settingMeta {
// 		lowerCased := SettingName(strings.ToLower(string(k)))
// 		if v, exists := m[lowerCased]; exists {
// 			delete(m, lowerCased)
// 			m[k] = v
// 		}
// 	}
// }

func newSettings(filePath string, chrome chromeExtension) *Settings {
	log := golog.LoggerFor("app.settings")
	config := viper.New()
	config.SetConfigFile(filePath)
	config.SetConfigType("yaml")
	config.SetEnvPrefix("LANTERN")
	config.AutomaticEnv()
	config.SetDefault(string(SNUserID), int64(0))
	config.SetDefault(string(SNAutoReport), true)
	config.SetDefault(string(SNAutoLaunch), true)
	config.SetDefault(string(SNProxyAll), false)
	config.SetDefault(string(SNSystemProxy), true)
	config.SetDefault(string(SNDisconnected), false)
	config.SetDefault(string(SNLanguage), "")
	config.SetDefault(string(SNLocalHTTPToken), "")
	config.SetDefault(string(SNUserToken), "")
	config.SetDefault(string(SNUIAddr), "")
	config.SetDefault(string(SNMigratedDeviceIDForUserID), int64(0))
	config.SetDefault(string(SNDeviceID), "")
	config.SetDefault(string(SNTakenSurveys), []string(nil))
	config.SetTypeByDefaultValue(true)

	// Use settings from disk if they're available.
	if err := config.ReadInConfig(); err != nil {
		log.Debugf("Could not load settings %w", err)
	} else {
		log.Debugf("Loaded settings from %v", filePath)
	}
	return &Settings{
		filePath:        filePath,
		changeNotifiers: make(map[SettingName][]func(interface{})),
		log:             log,
		chrome:          chrome,
		config:          config,
	}
}

// StartService starts the settings service that synchronizes Lantern's configuration with
// every UI client
func (s *Settings) StartService(channel ws.UIChannel) error {
	helloFn := func(write func(interface{})) {
		s.log.Debugf("Sending Lantern settings to new client")
		write(s.uiMap())
	}

	service, err := channel.Register("settings", helloFn)
	if err != nil {
		return err
	}
	s.muNotifiers.Lock()
	s.wsOut = service.Out
	s.muNotifiers.Unlock()
	go s.read(service.In, service.Out)
	return nil
}

func (s *Settings) read(in <-chan interface{}, out chan<- interface{}) {
	s.RLock()
	m := s.config.AllSettings()
	s.RUnlock()
	s.log.Debugf("Start reading settings messages!!")
	for message := range in {
		s.log.Debugf("Read settings message %v", message)

		data, ok := (message).(map[string]interface{})
		if !ok {
			continue
		}

		for k, v := range data {
			if _, exists := m[strings.ToLower(k)]; !exists || k == string(SNDeviceID) {
				s.log.Errorf("Unknown settings name %s", k)
				continue
			}
			s.setVal(SettingName(k), v)
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

func (s *Settings) setInt64(name SettingName, v interface{}) {
	b, ok := v.(int64)
	if !ok {
		s.log.Errorf("Could not convert %s(%v) to int64", name, v)
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

func (s *Settings) setStringArray(name SettingName, v interface{}) {
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

func (s *Settings) setString(name SettingName, v interface{}) {
	str, ok := v.(string)
	if !ok {
		s.log.Errorf("Could not convert %s(%v) to string", name, v)
		return
	}
	s.setVal(name, str)
}

// save saves settings to disk.
func (s *Settings) save() {
	s.saveDefault()
	//s.chrome.saveOnce(s.mapToSave)
}

// save saves settings to disk as yaml in the default lantern user settings directory.
func (s *Settings) saveDefault() {
	s.RLock()
	defer s.RUnlock()

	s.log.Trace("Saving settings")
	if err := s.config.WriteConfig(); err != nil {
		s.log.Errorf("Could not save settings file: %v", err)
	} else {
		s.log.Tracef("Saved settings to %s", s.filePath)
	}
}

// uiMap makes a copy of our map for the UI with support for omitting empty
// values.
func (s *Settings) uiMap() map[string]interface{} {
	s.RLock()
	defer s.RUnlock()
	return s.config.AllSettings()
}

// GetTakenSurveys returns the IDs of surveys the user has already taken.
func (s *Settings) GetTakenSurveys() []string {
	return s.getStringArray(SNTakenSurveys)
}

// SetTakenSurveys sets the IDs of taken surveys.
func (s *Settings) SetTakenSurveys(campaigns []string) {
	s.setStringArray(SNTakenSurveys, campaigns)
}

// GetProxyAll returns whether or not to proxy all traffic.
func (s *Settings) GetProxyAll() bool {
	return s.getBool(SNProxyAll)
}

// SetProxyAll sets whether or not to proxy all traffic.
func (s *Settings) SetProxyAll(proxyAll bool) {
	s.setVal(SNProxyAll, proxyAll)
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

func (s *Settings) GetTimeZone() (string, error) {
	return timezone.IANANameForTime(time.Now())
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

// SetUIAddr sets the last known UI address.
func (s *Settings) SetUIAddr(uiaddr string) {
	s.setVal(SNUIAddr, uiaddr)
}

// GetAddr gets the HTTP proxy address.
func (s *Settings) GetAddr() string {
	return s.getString(SNAddr)
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

// SetUserIDAndToken sets the user ID and token atomically
func (s *Settings) SetUserIDAndToken(id int64, token string) {
	s.setVals(map[SettingName]interface{}{SNUserID: id, SNUserToken: token})
}

// GetUserID returns the user ID
func (s *Settings) GetUserID() int64 {
	return s.getInt64(SNUserID)
}

// GetToken returns the user token
func (s *Settings) GetToken() string {
	return s.getString(SNUserToken)
}

// GetMigratedDeviceIDForUserID returns the user ID (if any) for which the current device's ID has been migrated from the old style to the new style
func (s *Settings) GetMigratedDeviceIDForUserID() int64 {
	return s.getInt64(SNMigratedDeviceIDForUserID)
}

// SetMigratedDeviceIDForUserID stores the user ID (if any) for which the current device's ID has been migrated from the old style to the new style
func (s *Settings) SetMigratedDeviceIDForUserID(userID int64) {
	s.setInt64(SNMigratedDeviceIDForUserID, userID)
}

// GetInternalHeaders returns extra headers sent with requests to internal services
func (s *Settings) GetInternalHeaders() map[string]string {
	// stubbed
	return make(map[string]string)
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
		if v, ok := val.(int); ok {
			return int64(v)
		}
	}
	return int64(0)
}

func (s *Settings) getVal(name SettingName) (interface{}, error) {
	s.log.Tracef("Getting value for %v", name)
	s.RLock()
	defer s.RUnlock()
	return s.config.Get(string(name)), nil
}

func (s *Settings) setVal(name SettingName, val interface{}) {
	s.setVals(map[SettingName]interface{}{name: val})
}

func (s *Settings) setVals(vals map[SettingName]interface{}) {
	s.log.Debugf("Setting %v", vals)
	s.Lock()
	for name, val := range vals {
		v := val
		if jn, ok := val.(json.Number); ok {
			v, _ = jn.Int64()
		}
		s.config.Set(strings.ToLower(string(name)), v)
	}
	// Need to unlock here because s.save() will lock again.
	s.Unlock()
	s.save()
	for name, val := range vals {
		s.onChange(name, val)
	}
}

// GetInt64Eventually blocks returning an int64 until the int has a value
// other than the defualt.
func (s *Settings) GetInt64Eventually(name SettingName) (int64, error) {
	nval := eventual.NewValue()
	s.OnChange(name, func(val interface{}) {
		nval.Set(val)
	})

	val := s.getInt64(name)
	if val > 0 {
		return val, nil
	}

	eid, _ := nval.Get(-1)
	intVal, ok := eid.(int64)
	if !ok {
		return int64(0), errors.New("Could not cast to int64?")
	}
	return intVal, nil
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
	wsOut := s.wsOut
	s.muNotifiers.RUnlock()
	for _, fn := range notifiers {
		fn(value)
	}
	if wsOut != nil {
		// notify UI of changed settings
		wsOut <- s.uiMap()
	}
}
