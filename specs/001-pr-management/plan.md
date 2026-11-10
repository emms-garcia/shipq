# Implementation Plan: Local PR Management and Transition Notifications

**Branch**: `feat/mvp-plan` | **Date**: 2026-11-10 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/001-pr-management/spec.md`.
**Status**: Proposed design for review; no application implementation in this PR.

## Summary

Build Shipq as a Go application core using hexagonal architecture and Flutter
desktop presentation. A single per-user Go host owns SQLite and monitoring;
the macOS app uses a private Unix-socket HTTP/JSON adapter.
A small signed Swift platform helper supplies macOS Keychain and notifications;
`SMAppService` manages approved per-user background startup.

The MVP is local-only and read-only toward GitHub PRs. Flexible rule trees,
effective-review normalization, immutable result pagination, trustworthy
membership baselines, and transactional history/outbox processing supply
consistent results across browsing, segments, and monitoring. Preserve ports and durable record formats
for future export/import and cloud adapters without building those features.

## Technical Context

**Language/Version**: Go supported stable, Flutter stable with its bundled Dart,
and Swift matching the selected Xcode. Exact toolchain versions and dependency
locks are pinned during scaffolding under the [version policy](research.md).
This planning PR does not invent an unverified current release number.

**Primary Dependencies**: Go standard `net/http`, `encoding/json`, `database/sql`,
`log/slog`, and testing packages; `modernc.org/sqlite`; UUID generation.
Flutter `dart:io` local client and Flutter test/integration-test tooling.
Native Swift ServiceManagement, Security, and UserNotifications frameworks.

**Storage**: One host-owned SQLite database per profile, WAL and ordered
migrations. Keychain stores GitHub credentials. User configuration, cache,
baseline state, and notification history/outbox have explicit boundaries.

**Testing**: Table-driven core tests; fake ports/clocks; real temporary SQLite
integration tests; fixture-backed GitHub tests; Go API/Dart client contract fixtures;
Flutter widget tests; limited end-to-end scenarios. Race-enabled Go tests,
90% statement coverage independently for domain and application groups, no
repository-wide or Flutter percentage quota. Signed native smoke tests are
separate from routine offline CI.

**Target Platform**: macOS 13+ in a logged-in user session, arm64 and x86_64.
Validate the pinned Flutter release's deployment floor. Future Windows/Linux
hosts or mobile embedding are separate adapters, not MVP deliverables.

**Project Type**: Local application core/library, background host, and
Flutter desktop app in one repository.

**Performance Goals**: With 10,000 cached PRs and 100 representative segments,
95th-percentile first-page latency at most two seconds through the release-mode
app. Reference class: Apple Silicon Mac, at least 8 GiB RAM and SSD; record
the exact machine/OS/toolchains in benchmark results. Healthy monitoring records
and submits qualifying notifications within the interval plus 60 seconds;
service-limit delays remain explicit.

**Constraints**: No project backend/cloud account; no secrets in SQLite or client
DTOs; read-only GitHub adapter; no initial-match notifications; no false exits
from incomplete data; no duplicate history across refreshes/restarts. Default
refresh interval 300 seconds, minimum 60, subject to visible backoff.

**Scale/Scope**: One GitHub.com account per local profile, multiple organizations
and repositories, explicit discovery/backfill coverage. No silent result/history
cap to evade rate limits. Cached queries remain available during backfill and
offline periods.

## Constitution Check

*Evaluated before research and again after the design.*

`.specify/memory/constitution.md` is an unfilled scaffold, not adopted governance.
Its example principles cannot be asserted as mandatory or claimed as satisfied.
Use the explicit user decisions and `.github/copilot-instructions.md` as the
current project constraints; do not silently ratify a constitution in this PR.

| Agreed constraint | Before design | After design |
|------------------|---------------|--------------|
| Go core, Flutter macOS presentation, no product CLI | Pass: revised user scope | Pass: the app calls shared application operations |
| Hexagonal dependencies and no duplicated rules | Pass: required boundary | Pass: core-facing ports, boundary mappers, import checks |
| Local MVP, SQLite adapter, no cloud sync | Pass: bounded scope | Pass: no cloud service or speculative sync engine |
| Initial matches list only; notifications track subsequent observations | Pass: specified invariant | Pass: explicit baseline epochs and unknown-state handling |
| Layered tests and 90% core coverage | Pass: agreed policy | Pass: validation guide and CI gates |
| Branch/title/PR conventions | Pass: `feat/mvp-plan` | Pass: Conventional Commit and repository PR template |

No agreed constraint is waived. Native packaging and live API semantics still
require implementation acceptance evidence; design compliance is not proof of
runtime correctness.

## Project Structure

### Documentation for this feature

```text
specs/001-pr-management/
  spec.md
  plan.md
  research.md
  data-model.md
  quickstart.md
  contracts/
    rules.md
    local-api.md
    desktop.md
```

`tasks.md` is produced by `/speckit-tasks` only after review of this plan.

### Planned source structure

```text
cmd/shipq-host/               # Internal bundled host entry point, not a product CLI
internal/domain/             # PR/review values, segment rules, pure transitions
internal/application/        # Use cases and narrow core-facing ports
internal/adapters/github/    # GitHub HTTP DTOs, pagination, normalization
internal/adapters/sqlite/    # Rows, migrations, transactions, mappings
internal/adapters/localapi/  # HTTP server and application DTO mapping
internal/adapters/macos/     # Go client of native platform capabilities
internal/host/               # Composition, refresh scheduling, outbox worker
api/local/v1/                # Transport DTOs/schema examples, no business rules
apps/desktop/                # Flutter UI and thin local application client
platform/macos/              # Swift helper, service registration, packaging
tests/                       # Shared fixtures, contracts, end-to-end scenarios
scripts/                     # Coverage/import checks and packaging commands
```

**Structure Decision**: One Go module; the core is independent of its host and
transport. Start with real capability boundaries rather than a package per
entity or a generic plugin framework. Domain imports no application/adapters.
Application imports domain and declares ports; adapters implement ports.
Only the composition root wires concrete adapters. Keep HTTP/row conversion at
the corresponding boundary; do not proliferate mapper packages without need.
Flutter imports neither SQLite nor a second GitHub integration.

## Runtime and Application Design

### Ports and adapters

Use focused capabilities for GitHub discovery/PR observations, observation and
evaluation commits, segment/configuration storage, history queries, credentials,
notification delivery, clocks, and IDs. The transaction port preserves baseline/
event/outbox atomicity; it does not expose a generic SQL transaction to domain
code. The scheduler is a host concern calling explicit refresh/evaluate use cases.

Remote DTO normalization produces domain facts; the core owns approval reduction
and predicate meaning. The SQLite adapter may optimize coarse selection, but
the domain evaluator remains authoritative. Optimizations must pass the same
parity fixtures before replacing any evaluation path.

### Process ownership and lifecycle

The application host is the sole database writer and refresh/outbox coordinator.
Flutter uses the [local contract](contracts/local-api.md); opening another app
window does not start another monitor. Private socket permissions and a
process-ownership lock isolate local profiles. No TCP listener or remote API.

The signed desktop bundle embeds the Go host and Swift platform helper.
The host may start the platform helper on demand; the helper never evaluates
rules or opens SQLite. Credentials cross only private host/helper IPC and are
never placed in command arguments or logs.

Installing/enabling login startup requests user consent and macOS approval.
Closing a Flutter window does not stop the host. Pause disables scheduled
refreshes while preserving cached functionality; explicit service stop disables
automatic relaunch until an explicit start. Native registration state is shown
separately from monitoring preferences and refresh health.
Developer tooling can run the host in the foreground without desktop service
installation; users do not need terminal commands for normal app operation.
Notifications require the bundled native platform capability;
absence is reported, not concealed by a fake success.

### Refresh and query pipeline

1. Discover permitted organizations/repositories and selected scope; paginate
   initial all-state PR backfill with resumable progress.
2. Collect complete per-PR fields/reviews and verify known head/update changes.
   Publish a trusted candidate or retain the older observation with explicit
   incomplete status. Do not equate inaccessible data with absent PRs.
3. Use overlapping incremental discovery plus fair rechecks of monitored known
   PRs, including relevant closed/merged PRs, and full reconciliation.
   Expose rate-limit backoff and coverage rather than asserting total freshness.
4. Evaluate changed observations against affected segment definitions. The
   [rule contract](contracts/rules.md) defines tri-valued evaluation, baseline
   epochs, identity semantics, and notification eligibility.
5. Commit baselines, enabled transition events, history, and outbox atomically.
   Replay uses event identities/sequences, not just content hashes.
6. Submit outbox requests to the native notification adapter with stable request
   IDs. Exactly-once visible OS delivery is not guaranteed across an ambiguous
   crash; history remains deduplicated and delivery status honest.
7. Freeze evaluated DTOs and coverage into bounded, expiring local query
   snapshots for deterministic pagination while refreshes continue.

### Presentation

Flutter provides account onboarding/scope selection, paginated PR list/detail,
grouped filter/segment editor, notification history, and monitor settings/status.
It renders core summaries and errors instead of reproducing domain decisions.
Show stale/partial/unknown results, authorization failures, service approval,
notification permission, and revision conflicts explicitly.

The [desktop contract](contracts/desktop.md) defines account/scope onboarding,
filter operators and grouping, saved segments, paginated results, history, and
lifecycle controls. App tests compare these workflows against fixture-backed
core expectations. No product CLI or terminal output contract is included.

## Implementation Sequence and Acceptance Gates

1. **Risk-first foundation**: scaffold locked toolchains, architecture checks,
   Go/Flutter local IPC round trip, and the signed macOS helper/service/
   notification spike. Verify device-flow registration and review-normalization
   fixtures. This is bounded integration validation, not full product behavior.
2. **Browsing vertical slice (US1)**: domain values, GitHub adapter, persistence,
   account/scope and cached paginated listings in the macOS app.
3. **Segments vertical slice (US2)**: shared evaluator, grouped rule editing,
   identity resolution, saved definitions, optimistic edits, and watches.
4. **Transitions/control (US3/US4)**: baselines, outbox/history, monitor controls,
   restart recovery, backoff, native notifications, and offline behavior.
5. **MVP release gates**: core coverage and race checks, real SQLite migrations,
   app/core consistency, signed lifecycle tests, and measured performance outcomes.

Do not generate implementation task IDs or application code in this phase.
Each later slice includes tests for its new behavior, including negative cases;
do not defer all tests to the end or chase coverage through trivial assertions.

## Requirement Traceability

| Requirements | Design location |
|--------------|-----------------|
| FR-001, FR-002 | Device-flow/Keychain decisions; account routes and lifecycle |
| FR-003, FR-004 | Repository discovery, backfill, frozen queries, app lists |
| FR-005 through FR-008 | Versioned rule AST, typed identity sets, review normalization |
| FR-009, FR-010 | Versioned user-owned segments/watches and revision checks |
| FR-011 through FR-014 | Baseline epochs, transactional history/outbox, native notifier |
| FR-015 through FR-018 | Per-user host, explicit controls, backoff, cached operations |
| FR-019 through FR-021 | Single SQLite owner, stable record boundaries, shared API |
| FR-022 | Read-only GitHub port; no PR mutation operations |
| SC-001 through SC-007 | Fixture, restart, offline, timing, app/core consistency and benchmark checks in quickstart |

## Complexity Tracking

No constitution exception is requested. The independent Go host is needed by
background monitoring and shared-state ownership; the native helper is needed
for macOS identity, permissions, and lifecycle. They are infrastructure adapters,
not additional business engines. Avoid additional services, a sync framework,
a plugin registry, or a separately implemented Flutter persistence layer.
