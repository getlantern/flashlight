// proxyimpl package implements the different dial methods for different
// pluggable transports.
//
// Each pluggable transport has its own implementation of the ProxyImpl. Each
// implementation should basically have a DialServer() and Close() methods for
// the client to talk to the proxy.
package proxyimpl
