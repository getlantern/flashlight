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
The iOS application needs to run as a background process on iOS, meaning that it's severely memory restricted. Because of this, we disable a lot of protocols and extra features using `// +build !iosapp` in order to conserve memory.

### Why not use // +build !ios
go-mobile automatically sets the `ios` build tag when building for iOS. In our case, we don't use this because in addition to the iOS app, we also distribute an iOS SDK that's intended for embedding inside of user-interactice apps. This SDK does not have to run in the background and is thus not memory constrained in the same way as our iOS app. Consequently, the sdk can and does include all of the standard lantern protocols and features.

### Architecture

![Overview](https://user-images.githubusercontent.com/1143966/117667942-72c80a80-b173-11eb-8c0d-829f2ccd8cde.png)
