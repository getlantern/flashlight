// chained package handles dialing to Lantern proxies. It contain code for
// representing proxies, sending them CONNECT requests, wrapping different
// net.Conn implementationsm etc.
//
// It mainly uses two packages for doing this:
//   - balancer: for load balancing proxies
//   - proxyimpl: for handling different protocol transports (e.g., QUIC,
//     http/https, lampshade, obfs4, etc.)
package chained
