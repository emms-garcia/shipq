#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

expected_go="$(cat "$root/.go-version")"
actual_go="$(GOTOOLCHAIN=local go env GOVERSION)"
if [[ "$actual_go" != "go$expected_go" ]]; then
    printf 'Expected Go %s, found %s\n' "$expected_go" "$actual_go" >&2
    exit 1
fi

expected_flutter="$(cat "$root/.flutter-version")"
actual_flutter="$(flutter --version --machine | python3 -c 'import json,sys; print(json.load(sys.stdin)["frameworkVersion"])')"
if [[ "$actual_flutter" != "$expected_flutter" ]]; then
    printf 'Expected Flutter %s, found %s\n' "$expected_flutter" "$actual_flutter" >&2
    exit 1
fi

expected_xcode="$(cat "$root/.xcode-version")"
actual_xcode="$(xcodebuild -version | sed -n '1p')"
if [[ "$actual_xcode" != "Xcode $expected_xcode" ]]; then
    printf 'Expected Xcode %s, found %s\n' "$expected_xcode" "$actual_xcode" >&2
    exit 1
fi
