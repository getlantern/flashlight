//
//  Tests_iOS.swift
//  Tests iOS
//
//  Created by Ox Cart on 7/30/21.
//

import XCTest
import Lanternsdk

class Tests_iOS: XCTestCase {

    func testExample() throws {
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

                let plainIP = try fetchIP(URLSessionConfiguration.default)
                XCTAssertNotEqual("", plainIP, "Should have gotten plain IP")
                let proxiedIP = try fetchIP(proxyConfig)
                XCTAssertNotEqual("", proxiedIP, "Should have gotten proxied IP")
                XCTAssertNotEqual(plainIP, proxiedIP, "Plain and proxied IPs should differ")
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
                XCTFail("Client-side error: \(err)")
                return
            }

            if data == nil {
                XCTFail("Data from request is nil")
                return
            }

            if let httpResponse = response as? HTTPURLResponse {
                if httpResponse.statusCode != 200 {
                    XCTFail("Server-side error: \(httpResponse)")
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
}
