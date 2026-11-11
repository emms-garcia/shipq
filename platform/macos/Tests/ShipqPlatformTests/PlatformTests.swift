import Foundation
import LocalAuthentication
import Security
import ServiceManagement
import XCTest

@testable import ShipqPlatform

private let requestID = "11111111-1111-4111-8111-111111111111"
private let profileID = "22222222-2222-4222-8222-222222222222"

@MainActor private final class FakeCredentials: CredentialAccess {
  var values: [String: Data] = [:]
  var failure: PlatformFailure?
  func read(profile: String) throws -> Data {
    if let failure { throw failure }
    guard let value = values[profile] else { throw PlatformFailure.notFound }
    return value
  }
  func write(profile: String, value: Data) throws {
    if let failure { throw failure }
    values[profile] = value
  }
  func delete(profile: String) throws {
    if let failure { throw failure }
    values.removeValue(forKey: profile)
  }
}

@MainActor private final class FakeNotifications: NotificationAccess {
  var permission = NotificationPermission.notDetermined
  var authorizationCalls = 0
  var submitted: [String] = []
  var failure: PlatformFailure?
  func status() async -> NotificationPermission { permission }
  func authorize() async throws {
    authorizationCalls += 1
    if let failure { throw failure }
    permission = .authorized
  }
  func submit(id: String, title: String, body: String) async throws {
    if let failure { throw failure }
    submitted.append(id)
  }
}

@MainActor private final class FakeService: ServiceAccess {
  var state = ServiceState.notRegistered
  var registrations = 0
  var unregisters = 0
  var failure: PlatformFailure?
  func status() -> ServiceState { state }
  func register() throws {
    registrations += 1
    if let failure { throw failure }
    state = .requiresApproval
  }
  func unregister() throws {
    unregisters += 1
    if let failure { throw failure }
    state = .notRegistered
  }
}

@MainActor private final class FakeItems: KeychainItems {
  var updates: [OSStatus] = [errSecSuccess]
  var added = errSecSuccess
  var deleted = errSecSuccess
  var copied = errSecSuccess
  var query: [String: Any] = [:]
  var insertions = 0
  var deletions = 0
  func copy(_ query: [String: Any]) -> (OSStatus, Data?) {
    self.query = query
    return (copied, Data("synthetic-secret".utf8))
  }
  func update(_ query: [String: Any], _ attributes: [String: Any]) -> OSStatus {
    self.query = query
    return updates.removeFirst()
  }
  func add(_ attributes: [String: Any]) -> OSStatus {
    query = attributes
    insertions += 1
    return added
  }
  func delete(_ query: [String: Any]) -> OSStatus {
    self.query = query
    deletions += 1
    return deleted
  }
}

final class PlatformTests: XCTestCase {
  @MainActor private func invoke(
    _ dispatcher: PlatformDispatcher, _ operation: String,
    _ fields: [String: Any] = [:]
  ) async throws -> [String: Any] {
    var request = fields
    request["protocol_version"] = 1
    request["request_id"] = requestID
    request["operation"] = operation
    let response = await dispatcher.handle(try JSONSerialization.data(withJSONObject: request))
    let result = try XCTUnwrap(try JSONSerialization.jsonObject(with: response) as? [String: Any])
    XCTAssertEqual(result["request_id"] as? String, requestID)
    XCTAssertEqual(result["protocol_version"] as? Int, 1)
    return result
  }

  @MainActor func testCredentialIsolationAndFailures() async throws {
    let keychain = FakeCredentials()
    let dispatcher = PlatformDispatcher(
      keychain: keychain, notifications: FakeNotifications(), service: FakeService())
    let secret = Data("synthetic-secret".utf8).base64EncodedString()
    let written = try await invoke(
      dispatcher, "credential.write", ["profile_id": profileID, "credential": secret])
    XCTAssertEqual((written["data"] as? [String: Any])?["state"] as? String, "stored")
    let read = try await invoke(dispatcher, "credential.read", ["profile_id": profileID])
    XCTAssertEqual((read["data"] as? [String: Any])?["credential"] as? String, secret)
    let missing = try await invoke(dispatcher, "credential.read", ["profile_id": requestID])
    XCTAssertEqual((missing["error"] as? [String: Any])?["code"] as? String, "not_found")
    for failure in [PlatformFailure.locked, .denied, .unavailable, .failed] {
      keychain.failure = failure
      let result = try await invoke(dispatcher, "credential.read", ["profile_id": profileID])
      XCTAssertEqual((result["error"] as? [String: Any])?["code"] as? String, failure.rawValue)
    }
  }

  @MainActor func testAuthorizationRequiresIntentAndSubmissionDoesNotPrompt() async throws {
    let notifications = FakeNotifications()
    let dispatcher = PlatformDispatcher(
      keychain: FakeCredentials(), notifications: notifications, service: FakeService())
    let deniedIntent = try await invoke(dispatcher, "notification.authorize")
    XCTAssertEqual((deniedIntent["error"] as? [String: Any])?["code"] as? String, "denied")
    XCTAssertEqual(notifications.authorizationCalls, 0)
    let deniedSubmit = try await invoke(
      dispatcher, "notification.submit",
      ["notification_id": requestID, "title": "Synthetic PR", "body": "Entered a segment"])
    XCTAssertEqual((deniedSubmit["error"] as? [String: Any])?["code"] as? String, "denied")
    XCTAssertTrue(notifications.submitted.isEmpty)
    _ = try await invoke(dispatcher, "notification.authorize", ["user_initiated": true])
    let submitted = try await invoke(
      dispatcher, "notification.submit",
      ["notification_id": requestID, "title": "Synthetic PR", "body": "Entered a segment"])
    XCTAssertEqual((submitted["data"] as? [String: Any])?["state"] as? String, "submitted")
    XCTAssertEqual(notifications.submitted, [requestID])
    XCTAssertEqual(notifications.authorizationCalls, 1)
    notifications.failure = .failed
    let failed = try await invoke(
      dispatcher, "notification.submit",
      ["notification_id": requestID, "title": "Synthetic PR", "body": "Entered a segment"])
    XCTAssertEqual((failed["error"] as? [String: Any])?["code"] as? String, "failed")
  }

  @MainActor func testServiceApprovalIsNotReportedAsEnabled() async throws {
    let service = FakeService()
    let dispatcher = PlatformDispatcher(
      keychain: FakeCredentials(), notifications: FakeNotifications(), service: service)
    _ = try await invoke(dispatcher, "service.register")
    XCTAssertEqual(service.registrations, 0)
    let result = try await invoke(dispatcher, "service.register", ["user_initiated": true])
    XCTAssertEqual((result["data"] as? [String: Any])?["state"] as? String, "requires_approval")
    let status = try await invoke(dispatcher, "service.status")
    XCTAssertEqual((status["data"] as? [String: Any])?["state"] as? String, "requires_approval")
    _ = try await invoke(dispatcher, "service.unregister", ["user_initiated": true])
    XCTAssertEqual(service.unregisters, 1)
    service.failure = .denied
    let denied = try await invoke(dispatcher, "service.register", ["user_initiated": true])
    XCTAssertEqual((denied["error"] as? [String: Any])?["code"] as? String, "denied")
  }

  @MainActor func testInvalidRequestsCannotReachCapabilities() async throws {
    let keychain = FakeCredentials()
    let notifications = FakeNotifications()
    let service = FakeService()
    let dispatcher = PlatformDispatcher(
      keychain: keychain, notifications: notifications, service: service)
    for fields: [String: Any] in [
      ["profile_id": "not-a-uuid", "credential": "c2VjcmV0"],
      ["profile_id": profileID, "credential": "invalid-base64"],
      ["profile_id": profileID, "credential": "", "extra": "synthetic-secret"],
      ["profile_id": profileID, "credential": "c2VjcmV0", "user_initiated": true],
    ] {
      let result = try await invoke(dispatcher, "credential.write", fields)
      XCTAssertEqual((result["error"] as? [String: Any])?["code"] as? String, "invalid_request")
    }
    XCTAssertTrue(keychain.values.isEmpty)
    let invalid = await dispatcher.handle(Data("synthetic-secret: invalid JSON".utf8))
    XCTAssertFalse(String(decoding: invalid, as: UTF8.self).contains("synthetic-secret"))
    let version = await dispatcher.handle(
      Data(
        """
        {"protocol_version":2,"request_id":"\(requestID)","operation":"notification.status"}
        """.utf8))
    let result = try XCTUnwrap(try JSONSerialization.jsonObject(with: version) as? [String: Any])
    XCTAssertEqual((result["error"] as? [String: Any])?["code"] as? String, "unsupported_version")
  }

  @MainActor func testKeychainUpsertDoesNotDeleteAndDisablesInteraction() throws {
    let items = FakeItems()
    items.updates = [errSecItemNotFound, errSecSuccess]
    items.added = errSecDuplicateItem
    let keychain = Keychain(items: items, available: true)
    try keychain.write(profile: profileID, value: Data("synthetic-secret".utf8))
    XCTAssertEqual(items.insertions, 1)
    XCTAssertEqual(items.deletions, 0)
    XCTAssertEqual(items.query[kSecAttrAccount as String] as? String, profileID)
    XCTAssertEqual(items.query[kSecAttrService as String] as? String, "com.shipq.github.oauth")
    XCTAssertEqual(items.query[kSecAttrSynchronizable as String] as? Bool, false)
    let context = try XCTUnwrap(items.query[kSecUseAuthenticationContext as String] as? LAContext)
    XCTAssertTrue(context.interactionNotAllowed)
    items.deleted = errSecItemNotFound
    try keychain.delete(profile: profileID)
    items.updates = [errSecAuthFailed]
    XCTAssertThrowsError(
      try keychain.write(profile: profileID, value: Data("synthetic-secret".utf8))
    ) {
      XCTAssertEqual($0 as? PlatformFailure, .denied)
    }
  }

  @MainActor func testNativeOSStatusMappingNeverExposesRawErrors() throws {
    for (status, expected): (OSStatus, PlatformFailure) in [
      (errSecItemNotFound, .notFound), (errSecInteractionNotAllowed, .locked),
      (errSecAuthFailed, .denied), (errSecUserCanceled, .denied),
      (errSecNotAvailable, .unavailable), (errSecParam, .failed),
    ] {
      XCTAssertThrowsError(try Keychain.check(status)) {
        XCTAssertEqual($0 as? PlatformFailure, expected)
      }
    }
  }

  @MainActor func testUnbundledCapabilitiesFailWithoutRequestingPermissions() async throws {
    let keychain = Keychain(available: false)
    XCTAssertThrowsError(try keychain.read(profile: profileID)) {
      XCTAssertEqual($0 as? PlatformFailure, .unavailable)
    }
    let notifications = Notifications(available: false)
    let permission = await notifications.status()
    XCTAssertEqual(permission, .unavailable)
    let service = ServiceManagement()
    XCTAssertEqual(service.status(), .unavailable)
    XCTAssertThrowsError(try service.register()) {
      XCTAssertEqual($0 as? PlatformFailure, .unavailable)
    }
  }

  @MainActor func testServiceNativeDenialAndSignatureErrorsAreSafe() {
    for (code, expected): (Int, PlatformFailure) in [
      (Int(kSMErrorLaunchDeniedByUser), .denied),
      (Int(kSMErrorAuthorizationFailure), .denied),
      (Int(kSMErrorInvalidSignature), .unavailable),
      (Int(kSMErrorServiceUnavailable), .unavailable),
      (Int(kSMErrorInternalFailure), .failed),
    ] {
      let error = NSError(
        domain: "SMAppServiceErrorDomain", code: code,
        userInfo: [NSLocalizedDescriptionKey: "synthetic-secret"])
      XCTAssertEqual(ServiceManagement.failure(error), expected)
    }
  }
}
