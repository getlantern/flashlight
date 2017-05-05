package loconf

import "golang.org/x/sys/windows/registry"

// SetUninstallURLInRegistry sets the URL of the uninstall survey.
func (lc *LoConf) SetUninstallURLInRegistry(url string) {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Uninstall\Lantern`,
		registry.QUERY_VALUE)
	if err != nil {
		s.log.Errorf("Could not query registry value? %v", err)
		return
	}
	defer k.Close()

	if err = k.SetStringValue("UninstallSurveyURL", url); err != nil {
		s.log.Errorf("Could not set string value? %v", err)
	}
}
