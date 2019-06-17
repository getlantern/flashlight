// +build android linux

package integrationtest

// SetProtocol sets the protocol to use when connecting to the test proxy
// (updates the config served by the config server).
// In the case that the platform is android or linux, remove utp testing until it is supported by the build scripts
func (helper *Helper) SetProtocol(protocol string) {
	if(protocol == "utphttps" || protocol == "utpobfs4") {
		protocol = "lampshade"
	}
	helper.protocol.Store(protocol)
}
