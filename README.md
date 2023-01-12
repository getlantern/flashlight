# Lantern [![Go Actions Status](https://github.com/getlantern/flashlight/actions/workflows/go.yml/badge.svg)](https://github.com/getlantern/flashlight/actions) [![Coverage Status](https://coveralls.io/repos/github/getlantern/flashlight/badge.svg?t=C4SaZX)](https://coveralls.io/github/getlantern/flashlight)

*Maintainers*
@forkner, @oxtoacart, @hwh33, @myleshorton

This repo contains the core Lantern library as well as the Android and iOS bindings.

The Lantern desktop application can be found at [getlantern/lantern-desktop](lantern-desktop).

## Vendoring

When `flashlight` builds in CI, it uses vendored dependencies. To avoid having to remember to run this manually, you can install a git pre-commit hook with `make install-githooks`.

## Process for making changes to [config](config)
flashlight is configured with per-proxy configuration loaded from the config-server, and global configuration loaded from S3 at runtime.

The global configuration is generated by [genconfig](genconfig) running as a [CRON job](https://github.com/getlantern/lantern-infrastructure/tree/main/salt/update_masquerades) on cm-donyc3021etc. That job uses the latest version of `genconfig` that is pushed to releases in this repo via CI.

genconfig merges [embeddedconfig/global.yaml.tmpl](embeddedconfig/global.yaml.tmpl) with dynamically verified masquerade hosts to produce the final global config. [embeddedconfig/download_global.sh](embeddedconfig/download_global.sh) pulls in that change and runs anytime we run `go generate`. A [CI Job](https://github.com/getlantern/flashlight/blob/main/.github/workflows/globalconfig.yml) runs `go generate` and automatically commits the results to this repo.

**All clients, including old versions of flashlight, fetch the same global config from S3, so global.yaml.tmpl must remain backwards compatible with old clients.**

If you're simply changing the contents of `global.yaml.tmpl` without any structural changes to the Go files in `config`, you can just change `global.yaml.tmpl` directly and once it's committed to main, it will fairly soon be reflected in S3.

If you're making changes to structure of the configuration, you need to ensure that this stays backwards compatible with old clients. To this end, we keep copies of old versions of [config](config), for example [config_v1](config_v1). When making a structural change to the config, follow these steps:

1. Back up the current version of `config`, for example `cp -R config config_v2`
2. Update the code and tests in `config` as appropriate
3. Make sure the tests in `config_v2`, `config_v1` etc. still work

## Adding new domain fronting mappings

In addition to adding the domains that forward on Cloudfront and Akamai, you also have to add the appropriate lines to [this](https://github.com/getlantern/flashlight/blob/main/genconfig/provider_map.yaml).

Mappings on Cloudfront and Akamai can be added using the terraform config in `lantern-cloud`.

## Building
You can build an SDK for use by external applications either for Android or for iOS.

### Prerequisites

* [Go 1.18](https://golang.org/dl/) is the minimum supported version of Go
* [GNU Make](https://www.gnu.org/software/make/) if you want to use the Makefile
* Dependencies are managed with Go Modules.
* Force git to use ssh instead of https by running
  `git config --global url."git@github.com:".insteadOf "https://github.com/"`

### Android SDK
make lanternsdk-android.aar

### iOS SDK
make Lanternsdk.xcframework

#### iOS SDK Usage

The below shows how to start Lantern and use it. When iOS puts the hosting app the sleep and wakes it up again, Lantern's proxy listener
will hang because the socket becomes unconnected but Go doesn't notice it. So it's necessary to call `LanternsdkStart` every time the
app wakes, which will start a new listener and return its new address.

A good place to do this is in
[applicationDidBecomeActive](https://developer.apple.com/documentation/uikit/uiapplicationdelegate/1622956-applicationdidbecomeactive),
which is called when the app first starts and every time it wakes.

```swift

import Lanternsdk

...

let configDir = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)[0].appendingPathComponent(".lantern")
let deviceID = UIDevice.current.identifierForVendor!.uuidString

var error: NSError?
let proxyAddr = Lanternsdk.LanternsdkStart("TestApp",
                            configDir.path,
                            deviceID,
                            true, // proxyAll
                            60000, // start timeout
                            &error)
if let err = error {
    throw err
}

if let host = proxyAddr?.httpHost {
    if let port = proxyAddr?.httpPort {
        let proxyConfig = URLSessionConfiguration.default
        // see https://stackoverflow.com/questions/42617582/how-to-use-urlsession-with-proxy-in-swift-3#42731010
        proxyConfig.connectionProxyDictionary = [AnyHashable: Any]()
        proxyConfig.connectionProxyDictionary?[kCFNetworkProxiesHTTPEnable as String] = 1
        proxyConfig.connectionProxyDictionary?[kCFNetworkProxiesHTTPProxy as String] = host
        proxyConfig.connectionProxyDictionary?[kCFNetworkProxiesHTTPPort as String] = port
        proxyConfig.connectionProxyDictionary?[kCFStreamPropertyHTTPSProxyHost as String] = host
        proxyConfig.connectionProxyDictionary?[kCFStreamPropertyHTTPSProxyPort as String] = port

        let session = URLSession.init(configuration: proxyConfig)
        // you can now use this session and everything is proxied
    }
}

```


#### TestApp

lanternsdk/TestApp contains a test iOS application demonstrating use of the lanternsdk on iOS.

## A note on iOS and memory usage
The iOS application needs to run as a background process on iOS, meaning that it's severely memory restricted. Because of this, we disable a lot of protocols and extra features using `// go:build !ios` in order to conserve memory.

### Why not use // +build !ios
go-mobile automatically sets the `ios` build tag when building for iOS. In our case, we don't use this because in addition to the iOS app, we also distribute an iOS SDK that's intended for embedding inside of user-interactice apps. This SDK does not have to run in the background and is thus not memory constrained in the same way as our iOS app. Consequently, the sdk can and does include all of the standard lantern protocols and features.

### Architecture

![Overview](https://user-images.githubusercontent.com/1143966/117667942-72c80a80-b173-11eb-8c0d-829f2ccd8cde.png)

## Features

We use "features" to enable/disable different characteristics/techniques in Flashlight, usually through the global config.

See `./config/features.go` for a list of features. Below is a non-extensive description of each feature.

### p2pcensoredpeer and p2pfreepeer

Allows the client to act either as a FreePeer or a CensoredPeer.

See overview of the p2p-proxying story here: https://docs.google.com/document/d/1JUjZHgpnunmwG3wUwlSmCKFwOGOXkkwyGd7cgrOJzbs/edit
