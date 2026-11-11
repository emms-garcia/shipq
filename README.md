# Shipq

A local-first macOS app for organizing GitHub pull requests into flexible
segments and receiving notifications when their membership changes.

**Current state:** app foundation and initial Go domain/review library
(T001-T006, T009-T010). The core models stable identities and observations and
reduces complete review snapshots to effective approval/participant facts.
The Flutter window still explains that GitHub connection and monitoring are
not implemented. The bundled internal Go host and Swift helper are explicit
stubs that exit with status 78; they do not
open a database, connect to GitHub, request permissions, or install a service.
There is no product CLI.

## Architecture

The accepted design uses a hexagonal Go core, SQLite infrastructure adapters,
Flutter presentation over a private Unix-socket API, and a separate native Swift
helper for macOS capabilities. Those integrations are not wired up yet.
Export/import and cloud synchronization remain deferred.

The review reducer counts distinct current approvers, retains submitted
participants separately, and does not infer mergeability. Incomplete pages,
unstable heads, conflicting records, or ambiguous decisive-review ordering
remain unknown rather than becoming zero approvals. Opinion and participant
completeness are independent. Approvals on older commits remain effective unless
the observed review state supersedes or dismisses them. Provider normalization
still requires the later GitHub adapter and opt-in live acceptance checks.

See the [specification](specs/001-pr-management/spec.md),
[plan](specs/001-pr-management/plan.md), and
[implementation tasks](specs/001-pr-management/tasks.md).

## Development requirements

Use macOS with full Xcode installed, its license accepted, and its command-line
developer directory selected. The deployment target is **macOS 13.0+**.

| Tool | Pinned version | Source |
|------|----------------|--------|
| Go | 1.26.2 | `.go-version`, `go.mod` |
| Flutter stable | 3.41.4 | `.flutter-version` |
| Dart | 3.11.1 | Bundled with pinned Flutter |
| Xcode | 27.1 (27A9275) | `.xcode-version` |
| Swift | 6.4 (swiftlang-6.4.0.34.1) | Bundled with pinned Xcode |

These are verified installed toolchains, not floating "latest" requirements.
Version files are manager-neutral; install/select them using your preferred
tool manager. `bash scripts/check_toolchains.sh` rejects mismatched Go,
Flutter, and Xcode versions without changing global installations. Python 3 and
GNU-compatible Make are also required; no third-party Python packages are used.

Go dependencies are pinned in `go.mod`/`go.sum`: `modernc.org/sqlite` v1.62.0
(CGo-free, Go 1.26 compatible) and `github.com/google/uuid` v1.6.0. SQLite's
transitive `modernc.org/libc` version follows its module requirements. The domain
uses the UUID utility for local IDs; SQLite remains reserved for its later
storage adapter. The host stub uses neither. Commit `pubspec.lock` to keep
application dependencies reproducible.

## Setup and build

From the repository root:

```sh
make deps
make check
make build
```

`make build` compiles the Go host and Swift helper for arm64 and x86_64, builds
the Flutter debug app for the current Mac, and assembles
`build/macos/<architecture>/Shipq.app`. The bundle includes
`Contents/MacOS/shipq-host`,
`Contents/Library/Helpers/ShipqPlatform.app`, and the inert launch-agent plist in
`Contents/Library/LaunchAgents/`.

Use `make build-arm64` or `make build-intel` to select an architecture explicitly.
Flutter is built one architecture at a time: the pinned Xcode 27.1 `lipo`
rejects multi-architecture verification used by Flutter 3.41.4. No shared SDK is
patched, and universal Flutter release packaging is not provided by this slice.
The host/helper binaries are universal in both bundles.

These are **ad-hoc-signed development builds**, not signed/notarized distribution
artifacts. Cross-compiling x86_64 does not prove execution on an Intel Mac or
macOS 13. Signed service,
Keychain, notification, and background-lifecycle acceptance is a later T019 gate.
The generated Flutter sandbox entitlements are retained pending that gate.
Nothing registers or starts the launch agent.

```sh
# Hot-reload the presentation scaffold (does not launch the internal stubs).
make run

# Open the assembled debug app without the Flutter debugger.
open "build/macos/$(uname -m)/Shipq.app"
```

## Checks and focused tests

`make check` checks toolchain pins, Go formatting/vet/race-enabled tests,
Dart formatting/analysis/widget tests, fixture integrity, Swift formatting,
plist validity, and shell syntax. `make format` applies source formatting.
Domain unit tests cover identity/observation invariants and fixture-backed review
reduction. Application currently declares only clock/ID-generator ports; it has
no executable use cases or coverage result yet. CI and separate 90% domain and
application coverage enforcement arrive with T021.

```sh
# One review regression, from the repository root.
GOTOOLCHAIN=local go test ./internal/domain/reviews \
  -run '^TestReduce$/latest_dismissal_never_resurrects_approval$' -count=1

# One responsive scaffold-widget case.
cd apps/desktop
flutter test --no-pub test/widget_test.dart \
  --plain-name 'scaffold explains unavailable features at Size(320.0, 240.0)'
cd ../..

# From the repository root: one fixture-integrity test.
python3 -m unittest discover -s tests/fixtures -p 'test_fixtures.py' \
  -k test_next_links_resolve_to_supplied_success_or_failure_pages

# Entire current widget and fixture suites.
make test-ui test-fixtures
```

For the implemented domain's aggregate statement coverage:

```sh
mkdir -p build/coverage
GOTOOLCHAIN=local go test -count=1 -race -coverpkg=./internal/domain/... \
  -coverprofile=build/coverage/review-domain.out \
  ./internal/domain/... ./internal/application/ports
GOTOOLCHAIN=local go tool cover -func=build/coverage/review-domain.out
```

Read the combined profile's `total`, not the per-test-package percentages.
The uncached run avoids mixing old source mappings into cross-package coverage.
There is deliberately no substitute application percentage for empty interfaces.

The GitHub/protocol/scenario fixtures under `tests/fixtures/` are synthetic.
Python integrity tests check pagination and references; Go review tests consume
the same fixtures. Neither proves live provider normalization or real
application transitions. No account credentials or OAuth registration are
needed. Fixture-host targets are deliberately failing reservations until T020;
core/adapter/coverage targets remain reserved until T021.

## Contributing

Use `feat/<slug>` branches, Conventional Commit subjects and PR titles, and the
[pull request template](.github/pull_request_template.md). For Spec Kit commands,
set `SPECIFY_FEATURE_DIRECTORY=specs/001-pr-management` on fresh checkouts;
feature selection is local, ignored state.
