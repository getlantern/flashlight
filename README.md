# Lantern [![Go Actions Status](https://github.com/getlantern/flashlight/actions/workflows/go.yml/badge.svg)](https://github.com/getlantern/flashlight/actions) [![Coverage Status](https://coveralls.io/repos/github/getlantern/flashlight/badge.svg?branch=HEAD&t=C4SaZX)](https://coveralls.io/github/getlantern/flashlight?branch=HEAD)

### Architecture

![Overview](https://user-images.githubusercontent.com/1143966/117667942-72c80a80-b173-11eb-8c0d-829f2ccd8cde.png)

### Prerequisites

* [Go 1.15](https://golang.org/dl/) is the minimum supported version of Go
* [GNU Make](https://www.gnu.org/software/make/) if you want to use the Makefile
* Dependencies are managed with Go Modules.
* Force git to use ssh instead of https by running
  `git config --global url."git@github.com:".insteadOf "https://github.com/"`

### Building
This repo contains the core Lantern logic as well as the Lantern desktop
program. To build, run:

`make lantern`

Or to build beam:

`make beam`

To develop in tandem with the [lantern-desktop-ui](https://github.com/getlantern/lantern-desktop-ui) frontend, prefix the make invocation with `env DISABLE_PORT_RANDOMIZATION=1` so that the development ui can reliably connect to this backend.

### Running against a specific proxy
It is often useful to force lantern to use a specific proxy, either for testing some new
change on the server side or for trying to replicate issues a specific user has seen.
To do so, you can use either:

`./hitProxy.py [name or ip]`

or

`./runAsUser.py [user id]`

It can also be useful to do this in conjuction with running over a VPN in China, for example,
to more closely simulate the user experience, albeit with the added latency of running 
traffic through the VPS itself.

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
Flashlight has experimental support for running as a whole-device VPN. This is
only supported on Linux and has only been tested on Ubuntu 18.04 specifically.

// TODO: update vpn mode docs now that stealth mode doesn't exist as a standalone concept.

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

## Lantern SDK
You can build an SDK for use by external applications either for Android or for iOS.

### Android SDK
make lanternsdk-android.aar

### iOS SDK
make LanternSDK.framework

## A note on iOS and memory usage
The iOS application needs to run as a background process on iOS, meaning that it's severely memory restricted. Because of this, we disable a lot of protocols and extra features using `// +build !iosapp` in order to conserve memory.

### Why not use // +build !ios
go-mobile automatically sets the `ios` build tag when building for iOS. In our case, we don't use this because in addition to the iOS app, we also distribute an iOS SDK that's intended for embedding inside of user-interactice apps. This SDK does not have to run in the background and is thus not memory constrained in the same way as our iOS app. Consequently, the sdk can and does include all of the standard lantern protocols and features.
