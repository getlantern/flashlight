// +build !windows

package app

// setLanguageInRegistry is a noop on non_Windows platforms.
func (s *Settings) setLanguageInRegistry(language string) {
}
