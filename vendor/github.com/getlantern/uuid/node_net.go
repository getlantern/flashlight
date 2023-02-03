// Copyright 2017 Google Inc.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !js

package uuid

import (
	"bytes"
	"net"
	"runtime"
)

var interfaces []net.Interface // cached list of interfaces

var touchbarAddr = touchbarAddress()

// getHardwareInterface returns the name and hardware address of interface name.
// If name is "" then the name and hardware address of one of the system's
// interfaces is returned.  If no interfaces are found (name does not exist or
// there are no interfaces) then "", nil is returned.
//
// Only addresses of at least 6 bytes are returned.
func getHardwareInterface(name string) (string, []byte) {
	if interfaces == nil {
		var err error
		interfaces, err = net.Interfaces()
		if err != nil {
			return "", nil
		}
	}
	for _, ifs := range interfaces {
		if len(ifs.HardwareAddr) >= 6 && (name == "" || name == ifs.Name) {
			if runtime.GOOS == "darwin" {
				if bytes.Equal(ifs.HardwareAddr, touchbarAddr) {
					continue
				}
			}
			return ifs.Name, ifs.HardwareAddr
		}
	}
	return "", nil
}

// The touchbar address somehow frequently appears as the default network address
// on OSX. This ignores it.
func touchbarAddress() []byte {
	touchbar, err := net.ParseMAC("ac:de:48:00:11:22")
	if err != nil {
		// This should never happen.
		return make([]byte, 0)
	}
	return touchbar
}
