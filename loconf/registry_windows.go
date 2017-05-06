package loconf

import "golang.org/x/sys/windows/registry"

// SetUninstallURLInRegistry sets the URL of the uninstall survey.
func (lc *LoConf) SetUninstallURLInRegistry(survey *UninstallSurvey, isPro bool) {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Uninstall\Lantern`,
		registry.QUERY_VALUE)
	if err != nil {
		s.log.Errorf("Could not query registry value? %v", err)
		return
	}
	defer k.Close()

	if survey.Enabled && (isPro && survey.Pro || !isPro && survey.Free) {
		if val.Probability > r.Float64() {
			if err = k.SetStringValue("UninstallSurveyURL", survey.URL); err != nil {
				s.log.Errorf("Could not set string value? %v", err)
			}
		} else {
			lc.log.Debugf("Turning survey off probabalistically")
		}
	}
	k.DeleteValue("UninstallSurveyURL")
}
