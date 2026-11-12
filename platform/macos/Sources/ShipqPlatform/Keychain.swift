import Foundation
import LocalAuthentication
import Security

@MainActor protocol KeychainItems {
  func copy(_ query: [String: Any]) -> (OSStatus, Data?)
  func update(_ query: [String: Any], _ attributes: [String: Any]) -> OSStatus
  func add(_ attributes: [String: Any]) -> OSStatus
  func delete(_ query: [String: Any]) -> OSStatus
}

@MainActor private struct SystemKeychainItems: KeychainItems {
  func copy(_ query: [String: Any]) -> (OSStatus, Data?) {
    var result: CFTypeRef?
    let status = SecItemCopyMatching(query as CFDictionary, &result)
    return (status, result as? Data)
  }
  func update(_ query: [String: Any], _ attributes: [String: Any]) -> OSStatus {
    SecItemUpdate(query as CFDictionary, attributes as CFDictionary)
  }
  func add(_ attributes: [String: Any]) -> OSStatus { SecItemAdd(attributes as CFDictionary, nil) }
  func delete(_ query: [String: Any]) -> OSStatus { SecItemDelete(query as CFDictionary) }
}

@MainActor final class Keychain: CredentialAccess {
  private let items: any KeychainItems
  private let available: Bool
  init(
    items: (any KeychainItems)? = nil,
    available: Bool = Bundle.main.bundleIdentifier == "com.shipq.platform"
  ) {
    self.items = items ?? SystemKeychainItems()
    self.available = available
  }

  private func query(_ profile: String) throws -> [String: Any] {
    guard available else { throw PlatformFailure.unavailable }
    let authentication = LAContext()
    authentication.interactionNotAllowed = true
    return [
      kSecClass as String: kSecClassGenericPassword,
      kSecAttrService as String: "com.shipq.github.oauth",
      kSecAttrAccount as String: try canonicalID(profile),
      kSecAttrSynchronizable as String: false,
      // Data-protection Keychain items are isolated by the signed app's
      // application identifier; the build embeds the matching profile/group.
      kSecUseDataProtectionKeychain as String: true,
      // Capability calls must never trigger an implicit credential prompt.
      kSecUseAuthenticationContext as String: authentication,
    ]
  }

  static func check(_ status: OSStatus) throws {
    switch status {
    case errSecSuccess: return
    case errSecItemNotFound: throw PlatformFailure.notFound
    case errSecInteractionNotAllowed: throw PlatformFailure.locked
    case errSecAuthFailed, errSecUserCanceled: throw PlatformFailure.denied
    case errSecNotAvailable: throw PlatformFailure.unavailable
    default: throw PlatformFailure.failed
    }
  }

  func read(profile: String) throws -> Data {
    var query = try query(profile)
    query[kSecReturnData as String] = true
    query[kSecMatchLimit as String] = kSecMatchLimitOne
    let (status, value) = items.copy(query)
    try Self.check(status)
    guard let value, !value.isEmpty else { throw PlatformFailure.failed }
    return value
  }

  func write(profile: String, value: Data) throws {
    guard !value.isEmpty, value.count <= PlatformDispatcher.maxCredentialBytes else {
      throw PlatformFailure.invalidRequest
    }
    let query = try query(profile)
    let attributes = [kSecValueData as String: value]
    let updated = items.update(query, attributes)
    if updated != errSecItemNotFound {
      try Self.check(updated)
      return
    }
    var inserted = query
    inserted[kSecValueData as String] = value
    inserted[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
    let added = items.add(inserted)
    if added == errSecDuplicateItem {
      try Self.check(items.update(query, attributes))
    } else {
      try Self.check(added)
    }
  }

  func delete(profile: String) throws {
    let status = items.delete(try query(profile))
    if status != errSecItemNotFound { try Self.check(status) }
  }
}
