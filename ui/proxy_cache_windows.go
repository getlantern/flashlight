// +build windows

package ui

import (
	"golang.org/x/sys/windows/registry"
)

// disableAutoProxyCache disables proxy caching on Windows, as using old PAC
// files causes issues.
func disableAutoProxyCache() {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Policies\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		log.Errorf("Could not open key %v", err)
	}
	defer k.Close()
	err = k.SetDWordValue("EnableAutoproxyResultCache", 0)
	if err != nil {
		log.Errorf("Could not set dword value %v", err)
	}
}
