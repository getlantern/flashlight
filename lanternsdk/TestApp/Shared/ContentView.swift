//
//  ContentView.swift
//  Shared
//
//  Created by Ox Cart on 7/30/21.
//

import SwiftUI
import Lanternsdk

struct ContentView: View {
    var body: some View {
        Button(action: {
            do {
                print("about to proxy stuff")
                try proxyStuff()
            } catch {
                print(error)
            }
        }) {
            Text("Test")
        }
    }
}

struct ContentView_Previews: PreviewProvider {
    static var previews: some View {
        ContentView()
    }
}

private func proxyStuff() throws {
    var error: NSError?
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
            if (plainIP == "") {
                print("didn't get plain IP")
                return
            }
            let proxiedIP = try fetchIP(proxyConfig)
            if (proxiedIP == "") {
                print("didn't get proxied IP")
                return
            }
            if (proxiedIP == plainIP) {
                print("proxied IP equalled plain IP")
                return
            }
            print("**************** Test Successful ***********************")
        }
    }
}

private func fetchIP(_ config: URLSessionConfiguration) throws -> String {
    let group = DispatchGroup()
    group.enter()
    let session = URLSession.init(configuration: config)

    let request = URLRequest(url: URL(string: "https://api.ipify.org")!)
    var ip = ""
    let task = session.dataTask(with: request) {
        (data: Data?, response: URLResponse?, error: Error?) in
        defer {
            group.leave()
        }

        if let err = error {
            print(err)
            return
        }

        if data == nil {
            print("data is nil")
            return
        }

        if let httpResponse = response as? HTTPURLResponse {
            if httpResponse.statusCode != 200 {
                print("Unexpected response status")
                return
            }

            let encodingName = response?.textEncodingName != nil ? response?.textEncodingName : "utf-8"
            let encoding = CFStringConvertEncodingToNSStringEncoding(CFStringConvertIANACharSetNameToEncoding(encodingName! as CFString))
            let stringData = String(data: data!, encoding: String.Encoding(rawValue: UInt(encoding)))
            session.invalidateAndCancel()
            ip = stringData!
        }
    }
    task.resume()

    group.wait()
    return ip
}
