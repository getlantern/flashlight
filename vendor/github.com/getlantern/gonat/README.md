gonat [![GoDoc](https://godoc.org/github.com/getlantern/gonat?status.png)](http://godoc.org/github.com/getlantern/gonat)&nbsp;[![Build Status](https://drone.lantern.io/api/badges/getlantern/gonat/status.svg)](https://drone.lantern.io/getlantern/gonat)&nbsp;[![Coverage Status](https://coveralls.io/repos/github/getlantern/gonat/badge.svg?branch=init)](https://coveralls.io/github/getlantern/gonat)
==========

This library only works on Linux.

Dependencies are managed using Go modules. If using a version of Go prior to 1.13, use the environment
variable `GO111MODULE=on` to enable use of modules.

In order to work, this library needs to be able to open raw sockets and update the conntrack table
via netlink. You can give the binary the correct capabilities with:

`sudo setcap CAP_NET_RAW,CAP_NET_ADMIN+ep <name_of_binary>`

This library requires the nf_conntrack module to be installed at runtime.

```
modprobe nf_conntrack
modprobe nf_conntrack_ipv4
```

iptables needs to be configured to drop the outbound RST packets that the kernel would usually create in response to SYN/ACK
packets responding to our raw TCP connections. We do this only for tcp connections that are already in ESTABLISHED in conntrack.
The library manually adds these to conntrack since we're using raw sockets.

`sudo iptables -I OUTPUT -p tcp -m conntrack --ctstate ESTABLISHED --ctdir ORIGINAL --tcp-flags RST RST -j DROP`

To undo this, run the same command, but replace the `-I` flag with the `-D` flag.

To run the unit tests, you need to have root permissions. It's also useful to enable tracing while running the tests.

```
GO111MODULE=on go test -c && TRACE=true sudo -E ./gonat.test
```