//
//  TestAppApp.swift
//  Shared
//
//  Created by Ox Cart on 7/30/21.
//

import SwiftUI
import Lanternsdk

@main
struct TestAppApp: App {
    init() {
        let configDir = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)[0].appendingPathComponent(".lantern")
        let deviceID = UIDevice.current.identifierForVendor!.uuidString

        var error: NSError?
        Lanternsdk.LanternsdkStart("TestApp",
                                   configDir.path,
                                   deviceID,
                                   true, // proxyAll
                                   &error)
        NSLog("hi")
    }

    var body: some Scene {
        WindowGroup {
            ContentView()
        }
    }
}
