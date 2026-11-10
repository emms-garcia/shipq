import Darwin
import Foundation

FileHandle.standardError.write(
  Data("ShipqPlatform: native platform capabilities are not implemented yet\n".utf8)
)
exit(78)
