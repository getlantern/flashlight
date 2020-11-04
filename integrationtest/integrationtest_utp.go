// +build !android,!linux

package integrationtest

// SetProtocol sets the protocol to use when connecting to the test proxy
// (updates the config served by the config server).
func (helper *Helper) SetProtocol(protocol string) {
	log.Debug("set protocol to " + protocol)
	helper.protocol.Store(protocol)
}
