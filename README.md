# Lantern [![wercker status](https://app.wercker.com/status/51826d53d0eeedd6efce16085874d82c/s/devel "wercker status")](https://app.wercker.com/project/byKey/51826d53d0eeedd6efce16085874d82c) [![Coverage Status](https://coveralls.io/repos/github/getlantern/flashlight/badge.svg?branch=HEAD&t=C4SaZX)](https://coveralls.io/github/getlantern/flashlight?branch=HEAD)

### Prerequisites

* [Go 1.12](https://golang.org/dl/) is the minimum supported version of Go
* [GNU Make](https://www.gnu.org/software/make/) if you want to use the Makefile
* Dependencies are managed with Go Modules. Use `GO111MODULE=on` in front of `go`
  command to automatically pick up dependencies.
* Force git to use ssh instead of https by running
  `git config --global url."git@github.com:".insteadOf "https://github.com/"`

This repo contains the core Lantern logic as well as the Lantern desktop
program. To build using your gopath: 

`make lantern`
### Dependency Management
Dependencies are managed using Go modules. If using a Go version prior to 1.13, you'll need to set the environment variable `GO111MODULE=on` in order to pick up dependencies automatically.

### Building
This repo contains the core Lantern logic as well as the Lantern desktop
program. To build using your gopath: 

`GO111MODULE=on make lantern`

You can also just build using the usual `go install` etc.

### Updating Icons

The icons used for the system tray are stored in `icons_src`. To apply changes
to the icons, make your updates in the `icons_src` folder and then run
`make update-icons`.

### Forcing Ad Swapping

By default, only a small percentage of web ads are swapped and only if the user
is free (not pro), making it difficult to test ad swapping. However, you can
force 100% of eligible ads to be swapped even when pro by setting the
environment variable `FORCEADS=true`.

### Running CI Locally

You can run CI locally with the following:

```
brew tap wercker/wercker
brew install wercker-cli
wercker build
```

### Throttling Proxies
On OS X, you can use the [throttle](throttle) script to throttle individual
proxies by IP, for example `throttle 1 104.131.91.213 1Mbit/s 0.05` throttles the
proxy `104.131.91.213` to 1Mbit/s with a 5% packet loss rate.

### VPN Mode
Flashlight has experimental support for running as a whole-device VPN. So far this has
only been tested in Linux (Ubuntu 18.04 specifically).

1. Run `make lantern && sudo ./lantern -headless -stealth -vpn 2>&1`. Using `stealthmode` disables split tunneling, which seems to be necessary right now.
2. Right now, the VPN doesn't play nicely with IPv6, so temporarily disable it with ```
sudo sysctl -w net.ipv6.conf.all.disable_ipv6=1
sudo sysctl -w net.ipv6.conf.default.disable_ipv6=1
```
3. The VPN needs to handle DNS traffic, so run `echo "nameserver 127.0.0.1" | sudo tee /etc/resolv.conf`
4. You need to set up your routes to route traffic through the VPN. Assuming your default gateway is 192.168.1.1, run ```
sudo route delete default
sudo route add default gw 10.0.0.2
sudo route add 8.8.8.8 gw 192.168.1.1
```