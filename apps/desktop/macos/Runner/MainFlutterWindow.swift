import Cocoa
import FlutterMacOS

class MainFlutterWindow: NSWindow {
  private let platformBridge = PlatformBridge()

  override func awakeFromNib() {
    let flutterViewController = FlutterViewController()
    let windowFrame = self.frame
    self.contentViewController = flutterViewController
    self.setFrame(windowFrame, display: true)

    RegisterGeneratedPlugins(registry: flutterViewController)
    platformBridge.register(with: flutterViewController.engine.binaryMessenger)

    super.awakeFromNib()
  }
}
