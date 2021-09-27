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

```swift

import Lanternsdk

...

let configDir = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)[0].appendingPathComponent(".lantern")
let deviceID = UIDevice.current.identifierForVendor!.uuidString

var error: NSError?
Lanternsdk.LanternsdkStart("TestApp",
                            configDir.path,
                            deviceID,
                            true, // proxyAll
                            &error)
if let err = error {
    throw err
}

let proxyAddr = Lanternsdk.LanternsdkGetProxyAddr(60000, &error)
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

        let plainIP = try fetchIP(URLSessionConfiguration.default)
        XCTAssertNotEqual("", plainIP, "Should have gotten plain IP")
        let proxiedIP = try fetchIP(proxyConfig)
        XCTAssertNotEqual("", proxiedIP, "Should have gotten proxied IP")
        XCTAssertNotEqual(plainIP, proxiedIP, "Plain and proxied IPs should differ")
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
