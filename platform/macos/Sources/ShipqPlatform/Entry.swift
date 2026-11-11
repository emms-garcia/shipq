import Foundation

@main struct Entry {
  @MainActor static func main() async {
    let dispatcher = PlatformDispatcher(
      keychain: Keychain(), notifications: Notifications(), service: ServiceManagement())
    var input = Data()
    do {
      while let chunk = try FileHandle.standardInput.read(upToCount: 16 * 1024), !chunk.isEmpty {
        input.append(chunk)
        if input.count > PlatformDispatcher.maxBytes { break }
      }
      let response = await dispatcher.handle(input)
      try FileHandle.standardOutput.write(contentsOf: response)
    } catch {
      // Never print request bytes, credentials, or raw native errors.
      return
    }
  }
}
