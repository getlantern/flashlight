// +build !android

package integrationtest

// SetProtocol sets the protocol to use when connecting to the test proxy
// (updates the config served by the config server).
func (helper *Helper) SetProtocol(protocol string) {
	helper.protocol.Store(protocol)
}
