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
    @Environment(\.scenePhase) var scenePhase

    static var host: String?
    static var port: Int?

    var body: some Scene {
        WindowGroup {
            ContentView()
        }
        .onChange(of: scenePhase) { newScenePhase in
              switch newScenePhase {
              case .active:
                print("App is active")
                startLantern()
              case .inactive:
                print("App is inactive")
              case .background:
                print("App is in background")
              @unknown default:
                print("Oh - interesting: I received an unexpected new value.")
              }
            }
    }

    private func startLantern() {
        print("starting lantern")
        let configDir = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)[0].appendingPathComponent(".lantern")
        let deviceID = UIDevice.current.identifierForVendor!.uuidString

        var error: NSError?
        let proxyAddr = Lanternsdk.LanternsdkStart("TestApp",
                                                   configDir.path,
                                                   deviceID,
                                                   true, // proxyAll
                                                   60000,
                                                   &error)
        if let err = error {
            print(err)
            return
        }

        TestAppApp.host = proxyAddr?.httpHost
        TestAppApp.port = proxyAddr?.httpPort
        print(TestAppApp.host)
        print(TestAppApp.port)
    }
}
