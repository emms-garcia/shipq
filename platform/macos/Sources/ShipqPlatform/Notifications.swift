import Foundation
@preconcurrency import UserNotifications

@MainActor final class Notifications: NotificationAccess {
  private let available: Bool
  init(available: Bool = Bundle.main.bundleIdentifier == "com.shipq.platform") {
    self.available = available
  }

  func status() async -> NotificationPermission {
    guard available else { return .unavailable }
    let settings = await UNUserNotificationCenter.current().notificationSettings()
    switch settings.authorizationStatus {
    case .notDetermined: return .notDetermined
    case .denied: return .denied
    case .authorized: return .authorized
    case .provisional: return .provisional
    @unknown default: return .unavailable
    }
  }

  func authorize() async throws {
    guard available else { throw PlatformFailure.unavailable }
    do {
      _ = try await UNUserNotificationCenter.current().requestAuthorization(options: [
        .alert, .sound,
      ])
    } catch { throw PlatformFailure.failed }
  }

  func submit(id: String, title: String, body: String) async throws {
    guard available else { throw PlatformFailure.unavailable }
    let content = UNMutableNotificationContent()
    content.title = title
    content.body = body
    content.sound = .default
    let request = UNNotificationRequest(identifier: id, content: content, trigger: nil)
    do { try await UNUserNotificationCenter.current().add(request) } catch {
      throw PlatformFailure.failed
    }
  }
}
