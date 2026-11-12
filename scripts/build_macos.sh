#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

target_arch="${1:-$(uname -m)}"
sign_identity="${SHIPQ_CODE_SIGN_IDENTITY:--}"
platform_profile="${SHIPQ_PLATFORM_PROVISIONING_PROFILE:-}"
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
    -derivedDataPath "$derived_data" ARCHS="$target_arch" ONLY_ACTIVE_ARCH=NO \
    CODE_SIGN_IDENTITY=- build -quiet

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

codesign --force --sign "$sign_identity" --identifier com.shipq.host \
    "$bundle/Contents/MacOS/shipq-host"
if [[ "$sign_identity" == "-" ]]; then
    codesign --force --sign "$sign_identity" "$helper"
else
    # The helper targets macOS's data-protection Keychain. Unlike an ad-hoc
    # signature, that access requires a profile-authorized application ID and
    # access group, so refuse to produce a misleadingly "signed" bundle.
    if [[ -z "$platform_profile" || ! -f "$platform_profile" ]]; then
        printf 'Signed builds require SHIPQ_PLATFORM_PROVISIONING_PROFILE to name an existing profile.\n' >&2
        exit 1
    fi
    profile_plist="$root/build/macos/$target_arch/ShipqPlatform.profile.plist"
    helper_entitlements="$root/build/macos/$target_arch/ShipqPlatform.entitlements"
    security cms -D -i "$platform_profile" -o "$profile_plist"
    team_id="$(/usr/libexec/PlistBuddy -c 'Print :Entitlements:com.apple.developer.team-identifier' "$profile_plist")"
    application_id="$(/usr/libexec/PlistBuddy -c 'Print :Entitlements:com.apple.application-identifier' "$profile_plist")"
    keychain_groups="$(/usr/libexec/PlistBuddy -c 'Print :Entitlements:keychain-access-groups' "$profile_plist")"
    expected_application_id="$team_id.com.shipq.platform"
    wildcard_keychain_group="$team_id.*"
    if [[ "$application_id" != "$expected_application_id" || \
          ( "$keychain_groups" != *"$expected_application_id"* && \
            "$keychain_groups" != *"$wildcard_keychain_group"* ) ]]; then
        printf 'Profile must authorize %s and its Keychain access group.\n' \
            "$expected_application_id" >&2
        exit 1
    fi
    plutil -create xml1 "$helper_entitlements"
    /usr/libexec/PlistBuddy -c "Add :com.apple.application-identifier string $application_id" \
        "$helper_entitlements"
    /usr/libexec/PlistBuddy -c "Add :com.apple.developer.team-identifier string $team_id" \
        "$helper_entitlements"
    /usr/libexec/PlistBuddy -c 'Add :keychain-access-groups array' "$helper_entitlements"
    /usr/libexec/PlistBuddy -c "Add :keychain-access-groups:0 string $application_id" \
        "$helper_entitlements"
    cp "$platform_profile" "$helper/Contents/embedded.provisionprofile"
    codesign --force --sign "$sign_identity" --entitlements "$helper_entitlements" "$helper"
    signed_team="$(codesign --display --verbose=4 "$helper" 2>&1 | sed -n 's/^TeamIdentifier=//p')"
    if [[ "$signed_team" != "$team_id" ]]; then
        printf 'Signing identity team %s does not match profile team %s.\n' \
            "${signed_team:-not set}" "$team_id" >&2
        exit 1
    fi
fi
codesign --force --sign "$sign_identity" --preserve-metadata=entitlements "$bundle"
codesign --verify --deep --strict "$bundle"

if [[ "$sign_identity" == "-" ]]; then
    printf '\nBuilt %s (ad-hoc development signature; no service installed)\n' "$bundle"
else
    printf '\nBuilt %s (identity %s; no service installed)\n' "$bundle" "$sign_identity"
fi
