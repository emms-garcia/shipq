import Foundation
import ServiceManagement

@MainActor final class ServiceManagement: ServiceAccess {
  private let service: SMAppService?

  init() {
    // SMAppService resolves agent metadata in the calling app, not a parent app.
    let bundle = Bundle.main
    let plist = bundle.bundleURL.appendingPathComponent(
      "Contents/Library/LaunchAgents/com.shipq.host.plist")
    let host = bundle.bundleURL.appendingPathComponent("Contents/MacOS/shipq-host")
    let metadata = (try? Data(contentsOf: plist)).flatMap {
      (try? PropertyListSerialization.propertyList(from: $0, format: nil)) as? [String: Any]
    }
    if bundle.bundleIdentifier == "com.shipq.desktop",
      metadata?["Label"] as? String == "com.shipq.host",
      metadata?["BundleProgram"] as? String == "Contents/MacOS/shipq-host",
      FileManager.default.isExecutableFile(atPath: host.path)
    {
      service = SMAppService.agent(plistName: "com.shipq.host.plist")
    } else {
      service = nil
    }
  }

  func status() -> ServiceState {
    guard let service else { return .unavailable }
    switch service.status {
    case .notRegistered: return .notRegistered
    case .enabled: return .enabled
    case .requiresApproval: return .requiresApproval
    case .notFound: return .notFound
    @unknown default: return .unavailable
    }
  }

  func register() throws {
    guard let service else { throw PlatformFailure.unavailable }
    if service.status == .enabled || service.status == .requiresApproval { return }
    do { try service.register() } catch { throw Self.failure(error) }
  }

  func unregister() throws {
    guard let service else { throw PlatformFailure.unavailable }
    if service.status == .notRegistered { return }
    do { try service.unregister() } catch { throw Self.failure(error) }
  }

  static func failure(_ error: Error) -> PlatformFailure {
    let native = error as NSError
    // The public domain constant starts in macOS 15; retain the same domain on 13/14.
    var domain = "SMAppServiceErrorDomain"
    if #available(macOS 15.0, *) {
      domain = SMAppServiceErrorDomain
    }
    if native.domain == domain {
      switch native.code {
      case Int(kSMErrorAuthorizationFailure), Int(kSMErrorLaunchDeniedByUser): return .denied
      case Int(kSMErrorInvalidSignature), Int(kSMErrorServiceUnavailable),
        Int(kSMErrorToolNotValid):
        return .unavailable
      default: break
      }
    }
    return .failed
  }
}
