// +build !windows

package loconf

// SetUninstallURLInRegistry is a noop on non_Windows platforms.
func (lc *LoConf) SetUninstallURLInRegistry(survey *UninstallSurvey, isPro bool) {
}
