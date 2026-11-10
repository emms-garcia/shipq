#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

target_arch="${1:-$(uname -m)}"
case "$target_arch" in
    arm64|x86_64) ;;
    *) printf 'Usage: bash scripts/build_macos.sh [arm64|x86_64]\n' >&2; exit 2 ;;
esac

bash scripts/check_toolchains.sh
mkdir -p build/bin/debug "build/macos/$target_arch"

for arch in arm64 amd64; do
    CGO_ENABLED=0 GOOS=darwin GOARCH="$arch" GOTOOLCHAIN=local \
        go build -trimpath -o "build/bin/debug/shipq-host-$arch" ./cmd/shipq-host
done
xcrun lipo -create build/bin/debug/shipq-host-arm64 \
    build/bin/debug/shipq-host-amd64 -output build/bin/debug/shipq-host

helper_binaries=()
for arch in arm64 x86_64; do
    scratch="$root/build/swift/debug/$arch"
    swift build --package-path platform/macos --scratch-path "$scratch" \
        --configuration debug --triple "$arch-apple-macosx13.0" --product ShipqPlatform
    bin_path="$(swift build --package-path platform/macos --scratch-path "$scratch" \
        --configuration debug --triple "$arch-apple-macosx13.0" --show-bin-path)"
    helper_binaries+=("$bin_path/ShipqPlatform")
done
xcrun lipo -create "${helper_binaries[@]}" -output build/bin/debug/ShipqPlatform

(
    cd apps/desktop
    flutter build macos --debug --config-only --no-pub
)

# Build one Flutter architecture at a time: Xcode 27.1's lipo rejects the
# multi-architecture verification used by the pinned Flutter SDK.
derived_data="$root/build/flutter/$target_arch"
xcrun xcodebuild -workspace apps/desktop/macos/Runner.xcworkspace \
    -scheme Runner -configuration Debug -destination 'generic/platform=macOS' \
    -derivedDataPath "$derived_data" ARCHS="$target_arch" ONLY_ACTIVE_ARCH=NO build -quiet

bundle="$root/build/macos/$target_arch/Shipq.app"
ditto "$derived_data/Build/Products/Debug/Shipq.app" "$bundle"

helper="$bundle/Contents/Library/Helpers/ShipqPlatform.app"
mkdir -p "$helper/Contents/MacOS" "$bundle/Contents/Library/LaunchAgents"
cp build/bin/debug/shipq-host "$bundle/Contents/MacOS/shipq-host"
cp build/bin/debug/ShipqPlatform "$helper/Contents/MacOS/ShipqPlatform"
cp platform/macos/ShipqPlatform/Info.plist "$helper/Contents/Info.plist"
cp platform/macos/LaunchAgents/com.shipq.host.plist "$bundle/Contents/Library/LaunchAgents/"

for binary in "$bundle/Contents/MacOS/shipq-host" "$helper/Contents/MacOS/ShipqPlatform"; do
    architectures="$(xcrun lipo -archs "$binary")"
    if [[ " $architectures " != *" arm64 "* || " $architectures " != *" x86_64 "* ]]; then
        printf 'Expected arm64 and x86_64 in %s; found %s\n' "$binary" "$architectures" >&2
        exit 1
    fi
done
xcrun lipo "$bundle/Contents/MacOS/Shipq" -verify_arch "$target_arch"
plutil -lint "$helper/Contents/Info.plist" \
    "$bundle/Contents/Library/LaunchAgents/com.shipq.host.plist"

codesign --force --sign - "$bundle/Contents/MacOS/shipq-host"
codesign --force --sign - "$helper"
codesign --force --sign - --preserve-metadata=entitlements "$bundle"
codesign --verify --deep --strict "$bundle"

printf '\nBuilt %s (ad-hoc development signature; no service installed)\n' "$bundle"
