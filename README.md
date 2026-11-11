# Shipq

A local-first macOS app for organizing GitHub pull requests into flexible
segments and receiving notifications when their membership changes.

**Current state:** app foundation, Go rule/review core, SQLite initialization,
the private local API library, its thin Dart client, and native capability
adapters (T001-T018).
The core models stable identities and observations, reduces review snapshots
to effective approval/participant facts, and validates/evaluates typed rules.
The Flutter window still explains that GitHub connection and monitoring are
not implemented. The bundled internal Go host remains an explicit stub that
exits with status 78. The Swift helper handles private capability requests, but
no production host/UI calls it yet. Normal app startup does not open a database,
connect to GitHub, request permissions, or install a service.
There is no product CLI.

## Architecture

The accepted design uses a hexagonal Go core, SQLite infrastructure adapters,
Flutter presentation over a private Unix-socket API, and a separate native Swift
helper for macOS capabilities. SQLite lifecycle/ownership and the local API
library exist; production runtime integration is not wired up yet.
Export/import and cloud synchronization remain deferred.

The review reducer counts distinct current approvers, retains submitted
participants separately, and does not infer mergeability. Incomplete pages,
unstable heads, conflicting records, or ambiguous decisive-review ordering
remain unknown rather than becoming zero approvals. Opinion and participant
completeness are independent. Approvals on older commits remain effective unless
the observed review state supersedes or dismisses them. Provider normalization
still requires the later GitHub adapter and opt-in live acceptance checks.

The rule engine supports the complete v1 field/operator matrix, grouped
three-valued logic, Unicode case-folded titles, scoped identities/labels, and
inclusive-start/exclusive-end dates. `me` uses the explicitly supplied connected
account. Compiled rules are immutable snapshots with readable summaries and
field dependencies; saved evaluation never adds a hidden open-state filter.
Incomplete inputs remain unknown. Cached results are displayable, but
notification eligibility requires explicitly fresh, complete evidence for every
referenced field, even when Boolean short-circuiting establishes membership.
Eligibility is not a transition or notification; those state machines are not
implemented.

The application validation use case returns the core's normalized definition,
summary, dependencies, and typed error paths. Its caller must supply the actual
encoded definition byte count for the 64 KiB limit. The local API now performs
strict JSON decoding and measures the actual raw UTF-8 definition bytes.
Storage mapping remains separate; no JSON/HTTP types enter the core.

The local API exposes `GET /v1/status` and `POST /v1/rules/validate`.
Status reads an application port; rule handling maps wire values and calls the
application validator, never SQLite or a second evaluator. Generated request
UUIDs accompany typed success/error envelopes. Invalid shapes/types, null
required values, duplicate/unknown properties, browser-origin requests, wrong
media/charset, and oversized bodies fail explicitly. The request body limit is
1 MiB, independently of the 64 KiB rule limit.

The socket library accepts an absolute dedicated runtime directory, verifies
current-user ownership and `0700`, and creates `api.sock` with `0600`.
Socket paths are at most 103 UTF-8 bytes, excluding the terminator, for macOS
compatibility. A separate persistent `socket.lock` protects the runtime path.
Live listeners/unrelated files/symlinks are never replaced; refused stale
sockets can be recovered only while holding ownership. Shutdown retains
ownership on timeout and removes only its own socket inode. No TCP listener
or production host wiring is introduced by the socket library. The later
composition must initialize the owned profile before starting this API.

The Dart `LocalClient` calls status and rule validation over the explicitly
supplied Unix socket using `HttpClient.connectionFactory`. It disables proxies
and redirects, checks protocol/correlation headers and JSON payload types, and
preserves typed server errors, immutable metadata, and core-provided summaries.
Connection loss, timeouts, malformed responses, protocol mismatch, and explicit
client closure remain distinct failures, never empty successful lists.
Requests and responses are bounded to 1 MiB; a ten-second whole-request deadline
is configurable. Timeouts cancel only their own I/O; client closure cancels all
owned requests. The host remains authoritative for the independent 64 KiB rule limit.
The device-session model consumes synthetic wire guidance only and rejects
secret/unknown fields and unsafe verification URLs; no account route or OAuth
exchange is implemented in Dart. No screen calls this client yet; runtime socket
handoff and signed/sandboxed integration remain the later T019 gate. Shared
design tokens/theming must precede feature-screen implementation.

The host's profile initializer owns `<data-root>/<profile-uuid>/host.lock`
before opening `shipq.sqlite3`, and returns a usable profile only after all
pending migrations and the profile binding commit atomically. Schema versions,
names, and checksums are recorded separately from portable record versions.
Newer schemas, changed applied migrations, and another profile's UUID fail
explicitly. A failure closes the database before releasing ownership.

The database uses WAL, foreign keys on every connection, a one-second busy
timeout, and one pooled connection. Shutdown waits for outstanding transactions
before closing the pool and releasing the ownership lock. The lock file remains
on disk; the OS releases its lock on close or process death, so a leftover file
is not a reason to bypass ownership. Profile directories are `0700`; database
and ownership files are `0600`. These filesystem helpers currently target macOS
and Linux, not a Windows host.

The initial schema contains the profile UUID binding, repository identity and
discovery cache, an organization index, and resumable repository-refresh metadata.
PR/review/account stores and mappings remain T030; segments, monitoring, and
outbox tables belong to their later migrations. No secrets or OAuth tokens are
stored. The entry-point stub does not call this initializer or create a real
user profile yet.

The Swift capability helper accepts one bounded JSON request on stdin and
returns one correlated response on stdout, then exits. Its Go adapter launches
an explicitly supplied executable without capability arguments or inherited
environment secrets; credentials cross only private pipes and remain separate
from the public local API. Outputs are bounded to 1 MiB. Normal calls have a
ten-second deadline; explicit notification authorization allows up to two
minutes for user interaction, still bounded by the caller's context.

Keychain items use profile UUIDs and a fixed Shipq service namespace, with
non-synchronizing device-local storage. Upserts never delete an existing item
before replacement; locked, denied, missing, unavailable, and failed outcomes
remain distinct. `LAContext.interactionNotAllowed` prevents implicit Keychain
prompts. Notifications require explicit authorization intent and use stable
submission UUIDs; submission never requests permission. OS acceptance is not
proof of visible or exactly-once delivery. Lost helper acknowledgment after
submission is reported as ambiguous.

`SMAppService.agent` resolves its plist in the **calling app's** bundle. The
service wrapper therefore refuses to register the parent desktop app's agent
from its nested helper. T019 must invoke it through the desktop-owned bridge,
with the proper bundle metadata and explicit consent. The current helper
reports service ownership as unavailable rather than registering a different
job. Registration/approval, explicit-stop persistence, signed Keychain/native
permissions, sandbox handoff, and background lifecycle remain T019/later gates.
Unbundled helper calls fail closed without touching Keychain or requesting
notifications. Routine tests inject native adapters instead of using real user data.

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
(CGo-free, Go 1.26 compatible), `github.com/google/uuid` v1.6.0, and
`golang.org/x/text` v0.44.0 for full Unicode case folding. SQLite's
transitive `modernc.org/libc` version follows its module requirements. The domain
uses the UUID utility for local IDs; the lifecycle adapter now uses SQLite and
`golang.org/x/sys` v0.49.0 for filesystem protection/ownership primitives.
The host entry-point stub uses neither. Commit `pubspec.lock` to keep
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
Dart formatting/analysis/client and widget tests, fixture integrity, injected
Swift capability tests/formatting, plist validity, and shell syntax.
`make format` applies source formatting.
Domain unit tests cover identity/observation invariants, fixture-backed review
reduction, and the rule matrix, unknowns, summaries, and validation limits.
Application tests cover rule validation and honest status/error propagation.
API tests consume shared protocol fixtures and exercise strict rule wire
mapping, resource boundaries, permissions, ownership, and real HTTP round trips
over Unix sockets. Status capabilities in these tests are synthetic, not native
service evidence.
SQLite tests use real temporary files, transactional upgrade/failure and
cancellation cases, and WAL/constraint checks. Host tests use a real subprocess
to prove ownership exclusion and recovery after abrupt process termination.
CI and automated separate 90% domain/application coverage enforcement arrive
with T021.

```sh
# One review regression, from the repository root.
GOTOOLCHAIN=local go test ./internal/domain/reviews \
  -run '^TestReduce$/latest_dismissal_never_resurrects_approval$' -count=1

# Rule engine and validation use case.
GOTOOLCHAIN=local go test ./internal/domain/segments ./internal/application

# Real database lifecycle and process ownership, isolated in temporary profiles.
GOTOOLCHAIN=local go test -count=1 -race \
  ./internal/adapters/sqlite ./internal/host -timeout 60s

# Shared schema, private Unix HTTP transport, and application use cases.
GOTOOLCHAIN=local go test -count=1 -race \
  ./api/local/v1 ./internal/adapters/localapi ./internal/application -timeout 60s

# Dart shared schemas and real temporary Unix HTTP client tests.
cd apps/desktop
flutter test --no-pub test/application/local_client_test.dart --timeout 15s
cd ../..

# Private helper subprocess boundaries; signed smoke skips unless explicitly opted in.
GOTOOLCHAIN=local go test -count=1 -race \
  ./internal/adapters/macos ./tests/platform -timeout 60s

# Injected Swift capabilities (no real Keychain, service install, or permission requests).
make test-platform

# Optional offline Go-to-unbundled-Swift protocol round trip after test-platform built it.
SHIPQ_TEST_SWIFT_HELPER="$(swift build --package-path platform/macos \
  --scratch-path build/swift/tests --show-bin-path)/ShipqPlatform" \
  GOTOOLCHAIN=local go test -count=1 -race ./internal/adapters/macos \
  -run '^TestUnbundledSwiftHelper$' -timeout 30s

# One responsive scaffold-widget case.
cd apps/desktop
flutter test --no-pub test/widget_test.dart \
  --plain-name 'scaffold explains unavailable features at Size(320.0, 240.0)'
cd ../..

# From the repository root: one fixture-integrity test.
python3 -m unittest discover -s tests/fixtures -p 'test_fixtures.py' \
  -k test_next_links_resolve_to_supplied_success_or_failure_pages

# Entire current client/widget and fixture suites.
make test-ui test-fixtures
```

For independent domain and application aggregate statement coverage:

```sh
mkdir -p build/coverage
GOTOOLCHAIN=local go test -count=1 -race -coverpkg=./internal/domain/... \
  -coverprofile=build/coverage/rule-domain.out ./internal/domain/...
GOTOOLCHAIN=local go tool cover -func=build/coverage/rule-domain.out
GOTOOLCHAIN=local go test -count=1 -race -coverpkg=./internal/application/... \
  -coverprofile=build/coverage/rule-application.out ./internal/application/...
GOTOOLCHAIN=local go tool cover -func=build/coverage/rule-application.out
```

Read the combined profile's `total`, not the per-test-package percentages.
The uncached run avoids mixing old source mappings into cross-package coverage.
The application total covers implemented validation/status use cases, not empty
port interfaces or unbuilt future operations.

The GitHub/protocol/scenario fixtures under `tests/fixtures/` are synthetic.
Python integrity tests check pagination and references; Go review/schema tests
and Dart client tests consume the same fixtures. None proves live provider
normalization or real application transitions. No account credentials or OAuth registration are
needed. Fixture-host targets are deliberately failing reservations until T020;
core/adapter/coverage targets remain reserved until T021.

The opt-in `TestSignedPlatformSmoke` requires `SHIPQ_PLATFORM_SMOKE=1` and an
absolute `SHIPQ_SIGNED_APP` path to a genuinely development-signed bundle; it
rejects ad-hoc signatures. It writes/reads/deletes only its newly generated
synthetic profile credential. Optional
`SHIPQ_PLATFORM_SMOKE_REQUEST_NOTIFICATIONS=1` and
`SHIPQ_PLATFORM_SMOKE_SUBMIT_NOTIFICATION=1` separately authorize prompting and
synthetic OS submission. Do not run these flags without explicit user intent.
This capability harness does not install a service or satisfy T019 lifecycle,
Flutter-terminated notification, architecture, or visible-delivery acceptance.

## Contributing

Use `feat/<slug>` branches, Conventional Commit subjects and PR titles, and the
[pull request template](.github/pull_request_template.md). For Spec Kit commands,
set `SPECIFY_FEATURE_DIRECTORY=specs/001-pr-management` on fresh checkouts;
feature selection is local, ignored state.
