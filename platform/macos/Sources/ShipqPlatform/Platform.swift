import Foundation

enum PlatformFailure: String, Error {
  case invalidRequest = "invalid_request"
  case unsupportedVersion = "unsupported_version"
  case denied, locked, unavailable, failed
  case notFound = "not_found"
}

enum NotificationPermission: String {
  case notDetermined = "not_determined"
  case denied, authorized, provisional, unavailable
}

enum ServiceState: String {
  case notRegistered = "not_registered"
  case enabled
  case requiresApproval = "requires_approval"
  case notFound = "not_found"
  case unavailable
}

@MainActor protocol CredentialAccess {
  func read(profile: String) throws -> Data
  func write(profile: String, value: Data) throws
  func delete(profile: String) throws
}

@MainActor protocol NotificationAccess {
  func status() async -> NotificationPermission
  func authorize() async throws
  func submit(id: String, title: String, body: String) async throws
}

@MainActor protocol ServiceAccess {
  func status() -> ServiceState
  func register() throws
  func unregister() throws
}

private struct Request: Decodable {
  let protocolVersion: Int
  let requestID: String
  let operation: String
  let profileID: String?
  let credential: String?
  let userInitiated: Bool?
  let notificationID: String?
  let title: String?
  let body: String?

  enum CodingKeys: String, CodingKey {
    case protocolVersion = "protocol_version"
    case requestID = "request_id"
    case profileID = "profile_id"
    case userInitiated = "user_initiated"
    case notificationID = "notification_id"
    case operation, credential, title, body
  }
}

private struct Result: Encodable {
  let state: String
  var credential: String? = nil
}

private struct SafeError: Encodable { let code: String }

private struct Response: Encodable {
  let protocolVersion = 1
  let requestID: String
  var data: Result? = nil
  var error: SafeError? = nil

  enum CodingKeys: String, CodingKey {
    case protocolVersion = "protocol_version"
    case requestID = "request_id"
    case data, error
  }
}

func canonicalID(_ text: String?) throws -> String {
  guard let text, let id = UUID(uuidString: text),
    id.uuidString.lowercased() == text,
    text != "00000000-0000-0000-0000-000000000000"
  else { throw PlatformFailure.invalidRequest }
  return text
}

@MainActor final class PlatformDispatcher {
  private let keychain: any CredentialAccess
  private let notifications: any NotificationAccess
  private let service: any ServiceAccess
  static let maxBytes = 1024 * 1024
  static let maxCredentialBytes = 64 * 1024

  init(
    keychain: any CredentialAccess, notifications: any NotificationAccess,
    service: any ServiceAccess
  ) {
    self.keychain = keychain
    self.notifications = notifications
    self.service = service
  }

  func handle(_ bytes: Data) async -> Data {
    var response = Response(requestID: UUID().uuidString.lowercased())
    do {
      guard bytes.count <= Self.maxBytes,
        let object = try JSONSerialization.jsonObject(with: bytes) as? [String: Any]
      else { throw PlatformFailure.invalidRequest }
      response = Response(requestID: try canonicalID(object["request_id"] as? String))
      let request: Request
      do { request = try JSONDecoder().decode(Request.self, from: bytes) } catch {
        throw PlatformFailure.invalidRequest
      }
      guard request.protocolVersion == 1 else { throw PlatformFailure.unsupportedVersion }
      let fields: Set<String>
      switch request.operation {
      case "credential.read", "credential.delete": fields = ["profile_id"]
      case "credential.write": fields = ["profile_id", "credential"]
      case "notification.status", "service.status": fields = []
      case "notification.authorize", "service.register", "service.unregister":
        fields = ["user_initiated"]
      case "notification.submit": fields = ["notification_id", "title", "body"]
      default: throw PlatformFailure.invalidRequest
      }
      let allowed = fields.union(["protocol_version", "request_id", "operation"])
      guard Set(object.keys).isSubset(of: allowed) else { throw PlatformFailure.invalidRequest }
      response.data = try await execute(request)
    } catch let failure as PlatformFailure {
      response.error = SafeError(code: failure.rawValue)
    } catch {
      response.error = SafeError(code: PlatformFailure.invalidRequest.rawValue)
    }
    return (try? JSONEncoder().encode(response)) ?? Data()
  }

  private func execute(_ request: Request) async throws -> Result {
    switch request.operation {
    case "credential.read":
      let value = try keychain.read(profile: canonicalID(request.profileID))
      guard !value.isEmpty, value.count <= Self.maxCredentialBytes else {
        throw PlatformFailure.failed
      }
      return Result(state: "found", credential: value.base64EncodedString())
    case "credential.write":
      let profile = try canonicalID(request.profileID)
      guard let encoded = request.credential,
        let value = Data(base64Encoded: encoded),
        !value.isEmpty, value.count <= Self.maxCredentialBytes
      else { throw PlatformFailure.invalidRequest }
      try keychain.write(profile: profile, value: value)
      return Result(state: "stored")
    case "credential.delete":
      try keychain.delete(profile: canonicalID(request.profileID))
      return Result(state: "deleted")
    case "notification.status":
      return Result(state: await notifications.status().rawValue)
    case "notification.authorize":
      guard request.userInitiated == true else { throw PlatformFailure.denied }
      try await notifications.authorize()
      return Result(state: await notifications.status().rawValue)
    case "notification.submit":
      let id = try canonicalID(request.notificationID)
      guard let title = request.title, !title.isEmpty,
        let body = request.body, !body.isEmpty
      else { throw PlatformFailure.invalidRequest }
      let permission = await notifications.status()
      guard permission != .unavailable else { throw PlatformFailure.unavailable }
      guard permission == .authorized || permission == .provisional else {
        throw PlatformFailure.denied
      }
      try await notifications.submit(id: id, title: title, body: body)
      return Result(state: "submitted")
    case "service.status":
      return Result(state: service.status().rawValue)
    case "service.register", "service.unregister":
      guard request.userInitiated == true else { throw PlatformFailure.denied }
      if request.operation == "service.register" {
        try service.register()
      } else {
        try service.unregister()
      }
      return Result(state: service.status().rawValue)
    default: throw PlatformFailure.invalidRequest
    }
  }
}
