// swift-tools-version: 6.0
import PackageDescription

let package = Package(
  name: "ShipqPlatform",
  platforms: [.macOS(.v13)],
  products: [
    .executable(name: "ShipqPlatform", targets: ["ShipqPlatform"])
  ],
  targets: [
    .executableTarget(name: "ShipqPlatform"),
    .testTarget(name: "ShipqPlatformTests", dependencies: ["ShipqPlatform"]),
  ]
)
