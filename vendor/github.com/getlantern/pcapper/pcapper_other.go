// +build !linux

package pcapper

import (
	"time"
)

// StartCapturing doesn't do anything on this platform.
func StartCapturing(application string, interfaceName string, dir string, numIPs int, packetsPerIP int, snapLen int, timeout time.Duration) error {
	return nil
}

// Dump doesn't do anything on this platform.
func Dump(ip string, comment string) {}

// DumpAll doesn't do anything on this platform.
func DumpAll(comment string) {}
