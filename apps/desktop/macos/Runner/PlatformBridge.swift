import AppKit
import Darwin
import FlutterMacOS
import Foundation
import ServiceManagement

@MainActor final class PlatformBridge {
  static let channelName = "com.shipq.desktop/platform"
  private var hostProcess: Process?
  private lazy var service = SMAppService.agent(plistName: "com.shipq.host.plist")

  func register(with messenger: FlutterBinaryMessenger) {
    let channel = FlutterMethodChannel(name: Self.channelName, binaryMessenger: messenger)
    channel.setMethodCallHandler { [weak self] call, result in
      guard let self else {
        result(FlutterError(code: "platform_unavailable", message: nil, details: nil))
        return
      }
      Task { @MainActor in
        do {
          switch call.method {
          case "socketPath": result(self.socketPath)
          case "startHost":
            try await self.startHost()
            result(nil)
          case "stopHost":
            try self.stopHost()
            result(nil)
          case "serviceStatus": result(self.serviceState)
          case "registerService":
            try self.registerService()
            result(self.serviceState)
          case "unregisterService":
            try self.unregisterService()
            result(self.serviceState)
          default: result(FlutterMethodNotImplemented)
          }
        } catch {
          result(FlutterError(code: "platform_unavailable", message: nil, details: nil))
        }
      }
    }
  }

  private var socketPath: String { "/tmp/shipq-\(getuid())/api.sock" }
  private var pidPath: String { "/tmp/shipq-\(getuid())/host.pid" }
  private var hostURL: URL {
    Bundle.main.bundleURL.appendingPathComponent("Contents/MacOS/shipq-host")
  }
  private var serviceState: String {
    switch service.status {
    case .notRegistered: return "not_registered"
    case .enabled: return "enabled"
    case .requiresApproval: return "requires_approval"
    case .notFound: return "not_found"
    @unknown default: return "unavailable"
    }
  }

  private func startHost() async throws {
    guard FileManager.default.isExecutableFile(atPath: hostURL.path) else {
      throw BridgeFailure.unavailable
    }
    if let process = hostProcess, process.isRunning { return }
    let process = Process()
    process.executableURL = hostURL
    let environment = ProcessInfo.processInfo.environment
    process.environment = [
      "HOME": environment["HOME"] ?? NSHomeDirectory(),
      "LANG": environment["LANG"] ?? "en_US.UTF-8",
      "LOGNAME": environment["LOGNAME"] ?? NSUserName(),
      "PATH": "/usr/bin:/bin",
      "USER": environment["USER"] ?? NSUserName(),
    ]
    process.standardInput = FileHandle.nullDevice
    process.standardOutput = FileHandle.nullDevice
    process.standardError = FileHandle.nullDevice
    try process.run()
    hostProcess = process
    for _ in 0..<50 {
      if FileManager.default.fileExists(atPath: socketPath) { return }
      if !process.isRunning { throw BridgeFailure.unavailable }
      try await Task.sleep(for: .milliseconds(100))
    }
    process.terminate()
    throw BridgeFailure.unavailable
  }

  private func stopHost() throws {
    if service.status != .notRegistered { try service.unregister() }
    if let process = hostProcess, process.isRunning {
      process.terminate()
      process.waitUntilExit()
    } else if let text = try? String(contentsOfFile: pidPath, encoding: .utf8),
      let pid = Int32(text.trimmingCharacters(in: .whitespacesAndNewlines)),
      let application = NSRunningApplication(processIdentifier: pid),
      application.executableURL?.standardizedFileURL == hostURL.standardizedFileURL
    {
      application.terminate()
    }
    hostProcess = nil
  }

  private func registerService() throws {
    if let process = hostProcess, process.isRunning {
      process.terminate()
      process.waitUntilExit()
      hostProcess = nil
    }
    if service.status == .notRegistered || service.status == .notFound {
      try service.register()
    }
  }

  private func unregisterService() throws {
    if service.status != .notRegistered { try service.unregister() }
  }
}

private enum BridgeFailure: Error { case unavailable }
