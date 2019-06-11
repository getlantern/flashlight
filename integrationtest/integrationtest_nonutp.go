// +build !android, !linux

package integrationtest

// SetProtocol sets the protocol to use when connecting to the test proxy
// (updates the config served by the config server).
func (helper *Helper) SetProtocol(protocol string) {
	if(protocol == "utphttps" || protocol == "utpobfs4") {
		protocol = "lampshade"
	}
	helper.protocol.Store(protocol)
}
