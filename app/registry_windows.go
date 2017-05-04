package app

import "golang.org/x/sys/windows/registry"

// setLanguageInRegistry sets the user language
func (s *Settings) setLanguageInRegistry(language string) {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Uninstall\Lantern`,
		registry.QUERY_VALUE)
	if err != nil {
		s.log.Errorf("Could not query registry value? %v", err)
		return
	}
	defer k.Close()

	var url string
	if language == "en_US" {
		url = "https://www.surveymonkey.com/r/HLZ5WBS"
	} else {
		url = "https://www.surveymonkey.com/r/HPJWRNP"
	}
	if err = k.SetStringValue("UninstallSurveyURL", url); err != nil {
		s.log.Errorf("Could not set string value? %v", err)
	}
}
