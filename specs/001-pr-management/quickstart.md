# Quickstart and Acceptance Validation

## Status

This is the implementation-phase run/validation contract. Foundation build,
core/adapter/coverage, fixture-host, and desktop-fixture targets now exist.
Later user-story commands remain planned until their corresponding tasks are
implemented; do not report those future flows as passing checks.

## Prerequisites and planned setup

- macOS 13 or later in a logged-in GUI session; validate arm64 and x86_64.
- Supported Go stable and Flutter stable SDKs pinned when scaffolding, with the
  Dart SDK bundled by Flutter. Xcode command-line tools and macOS build tooling.
- For live authentication only: a registered GitHub OAuth App with device flow
  enabled, and its public client ID. No client secret belongs in this project.
- Signing/notarization credentials are needed for distribution and the native
  notification/lifecycle acceptance spike, not for core unit tests.

Foundation setup:

```sh
go mod download
cd apps/desktop
flutter pub get
cd ../..
make build
```

`make build` builds the internal Go host and debug desktop app with its native platform
helper. `make check` checks Go formatting, runs vet, race-enabled Go tests,
the core coverage gate, Dart formatting, Flutter analysis, and Flutter tests.
Implementation must add and document these targets, not assume they exist.

## Focused test commands

```sh
# One planned domain test.
go test ./internal/domain/segments -run '^TestApprovalCountComparisons$' -count=1

# Domain and use-case tests, with race detection and coverage.
make test-core

# Real temporary-SQLite and fixture-backed GitHub adapter contracts.
make test-adapters

# One planned Flutter widget test.
cd apps/desktop
flutter test test/segments/filter_editor_test.dart

# Full Flutter widget suite.
flutter test

# Fixture-backed macOS integration test; requires the fixture host described below.
flutter test integration_test/local_pr_flow_test.dart -d macos
```

`make check-core-coverage` enforces at least 90% statement coverage independently
for the domain group and application/use-case group, excluding adapters and
generated client DTOs. This is not a repository-wide target. It must fail if a
group unexpectedly contains no executable statements, not treat absent results
as 100%. The grouped gate makes low-coverage use cases visible even when the
domain has many more statements.

## Deterministic local scenario

The implementation must supply a scenario driver under `tests/fixtures/` that
implements the GitHub port without real tokens or network access. The driver
advances explicit scenario steps through a test-only control interface, not the
production API. Configure the app to connect to the fixture host's isolated
socket; it uses a temporary data directory and fake platform adapters.

Planned scenario commands:

```sh
make fixture-host

# In a second terminal; development tooling launches Flutter against the fixture host.
make run-desktop-fixture
```

The targets print the exact socket path and pass it to the Flutter test client
for that run; test fixtures may not silently use the user's production profile
or Keychain. No test-only account injection/control is compiled into release
builds. These are development tools, not a user-facing application CLI.

### Acceptance sequence

1. Seed two organizations, at least three repositories, and PRs authored by self
   and others, with one/two/three approvals and different reviewers. In the app,
   select exactly two versus at least two approvals, exclude authored PRs, and
   independently require self's approval. Compare IDs with the core's expected set.
2. Save a compound segment, reopen it in a new app session, and compare its
   membership with the ad hoc filter. Verify names and readable rule summaries;
   edit from two windows and prove a stale revision returns a conflict rather
   than overwriting newer work.
3. Perform the first refresh. Existing matches appear in listings; history and
   the fake notifier remain empty.
4. Advance fixture state through draft-to-ready, one-to-two approvals, addition
   of self's approval, two-to-three approvals, dismissal, and changes requested.
   Compare the exact expected entry/exit event sequence for each segment.
5. Repeat refreshes and restart the host. Assert one history entry per observed
   transition, unchanged identities, and no replay. Include A-to-B-to-A-to-B
   membership to prove legitimate repeated entries are not over-deduplicated.
6. Inject a failure on a later review page and repository access revocation.
   Keep trusted baselines, show partial coverage, and generate no false exits.
7. Disable networking. The app browses cache, edits rules, and reads history.
   A fresh-data request reports unavailable status while cached queries stay usable.
8. Walk a stable paginated snapshot while another refresh changes PRs. Verify
   no duplicates/omissions. Expire the snapshot and require restarting at page one.
9. Deny notification permission and inject notifier failure/ambiguous completion.
   Preserve one history entry and honest delivery state without claiming OS
   display or duplicating history.
10. Clear a test profile and verify credentials, persisted user records, cache,
    baselines, and in-memory query snapshots for that profile are invalidated.

## Real macOS and GitHub smoke checks

These are opt-in checks on a signed development/release bundle, separate from
routine fixture tests. No production PR mutation is automated by Shipq.

### Personal signing setup

Use only the developer team that owns this Shipq checkout. In Xcode, add that
Apple Account under **Settings > Accounts**, create an **Apple Development**
certificate, and use automatic signing for the `com.shipq.platform` helper
bundle on the current Mac. The resulting Mac App Development profile must
authorize the helper's application identifier and Keychain access group. Build
with the certificate and profile explicitly selected:

```sh
SHIPQ_CODE_SIGN_IDENTITY="Apple Development: Your Name (CERTIFICATE_ID)" \
SHIPQ_PLATFORM_PROVISIONING_PROFILE=/absolute/path/to/embedded.provisionprofile \
make build-arm64
```

`scripts/build_macos.sh` rejects a missing or mismatched profile, validates its
Team ID and `com.shipq.platform` application identifier, embeds it only in the
nested helper, and verifies the final bundle. Do not use an employer/client
certificate or provisioning profile for this acceptance path. Ad-hoc builds
remain the default for routine local checks.

- Connect using device flow, inspect scopes/identity, and authorize an
  organization if permitted. Confirm private data is visible only where allowed.
- Register the per-user background service with explicit consent. Close all
  Flutter windows and terminate the Flutter process, then trigger a known PR
  change externally.
  Verify refresh/history and native notification submission without the Flutter
  process. Verify permission denial is observable.
- Log out/in and test approved login startup, then disable it in System Settings
  and show the approval state accurately.
- Pause, quit the background service, reopen the app, and verify no silent
  restart of monitoring. Explicitly start/resume it and verify recovery.
- Sleep/wake and disconnect/reconnect networking. Compare last trustworthy state
  with the next observation; do not claim unseen intermediate transitions.
- Disconnect and confirm the Keychain token is removed and in-flight refreshes
  cannot publish further observations.

## Performance and release gates

Benchmark on an Apple Silicon Mac with at least 8 GiB RAM and an SSD, recording
the exact model, OS, SDK versions, and build mode in results. Seed 10,000 PRs and
100 representative segments, including grouped review predicates. Run at least
100 first-page queries in the release-mode app; the 95th percentile must be
at most two seconds, including IPC and UI rendering, not only SQL execution.

Measure refresh-to-outbox timing against the configured interval plus 60 seconds
under healthy fixture conditions. Report service backoff separately. Core race
tests, persistence migrations, app/core consistency fixtures, and signed native
notification/lifecycle checks must pass before calling the macOS MVP complete.

## Checks available in the planning PR

```sh
SPECIFY_FEATURE_DIRECTORY=specs/001-pr-management \
  python3 .specify/scripts/python/check_prerequisites.py --json --paths-only
SPECIFY_FEATURE_DIRECTORY=specs/001-pr-management \
  python3 .specify/scripts/python/check_prerequisites.py --json
git diff --check
```

The prerequisite check confirms artifact discovery, not application correctness.
Do not require `tasks.md` until `/speckit-tasks` has run after plan review.
