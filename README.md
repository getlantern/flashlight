# Lantern [![Go Actions Status](https://github.com/getlantern/flashlight/actions/workflows/go.yml/badge.svg)](https://github.com/getlantern/flashlight/actions) [![Coverage Status](https://coveralls.io/repos/github/getlantern/flashlight/badge.svg?t=C4SaZX)](https://coveralls.io/github/getlantern/flashlight)

This repo contains the core Lantern library as well as the Android and iOS bindings.

The Lantern desktop application can be found at [getlantern/lantern-desktop](lantern-desktop).

## Building
You can build an SDK for use by external applications either for Android or for iOS.

### Prerequisites

* [Go 1.15](https://golang.org/dl/) is the minimum supported version of Go
* [GNU Make](https://www.gnu.org/software/make/) if you want to use the Makefile
* Dependencies are managed with Go Modules.
* Force git to use ssh instead of https by running
  `git config --global url."git@github.com:".insteadOf "https://github.com/"`

### Android SDK
make lanternsdk-android.aar

### iOS SDK
make LanternSDK.framework

## A note on iOS and memory usage
The iOS application needs to run as a background process on iOS, meaning that it's severely memory restricted. Because of this, we disable a lot of protocols and extra features using `// +build !iosapp` in order to conserve memory.

### Why not use // +build !ios
go-mobile automatically sets the `ios` build tag when building for iOS. In our case, we don't use this because in addition to the iOS app, we also distribute an iOS SDK that's intended for embedding inside of user-interactice apps. This SDK does not have to run in the background and is thus not memory constrained in the same way as our iOS app. Consequently, the sdk can and does include all of the standard lantern protocols and features.

### Architecture

![Overview](https://user-images.githubusercontent.com/1143966/117667942-72c80a80-b173-11eb-8c0d-829f2ccd8cde.png)
