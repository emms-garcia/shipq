# Tasks: Local PR Management and Transition Notifications

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md),
[data-model.md](data-model.md), [contracts/](contracts/), and
[quickstart.md](quickstart.md).
**Branch for this checklist**: `feat/mvp-tasks`
**Scope**: macOS Flutter app, Go hexagonal core/internal background host, SQLite,
and native macOS adapters. No product CLI, cloud sync, or export/import.

**Tests**: Required by the specification and agreed testing policy. Write the
listed behavioral tests before implementing their behavior; establish meaningful
failing assertions, then make them pass. Core statement coverage must reach 90%
independently for domain and application groups. No global or Flutter quota.

## Format and Execution Rules

- Every task uses an unchecked box, sequential ID, optional `[P]`, story label
  for story phases, concrete repository-relative file paths, and prerequisites.
- `[P]` means the task can run alongside other ready tasks with disjoint files,
  after its prerequisites and phase gate are satisfied. It is not permission
  to edit shared files concurrently.
- `Depends: none` identifies an independent setup task. Every foundational task
  also requires Phase 1. Every story task also requires Phase 2; US2 requires the
  US1 checkpoint, US3 the US2 checkpoint, and US4 the US3 checkpoint. Final tasks
  require the US4 checkpoint. Explicit dependencies below add to these gates.
- Paths describe planned implementation files, not files that already exist.
  Keep the Go domain/application independent of HTTP, SQL, Flutter, and native
  frameworks; add only ports consumed by actual MVP use cases.
- All tasks remain unchecked until their implementation and relevant checks are
  complete. The existing requirements checklist is a read-only implementation
  gate, not a task-completion tracker.
- A missing OAuth registration, signing identity, or GUI test environment blocks
  the affected live acceptance gate. Never mark it passed using fake evidence.
  Keep routine tests fixture-backed and secret-free.

## Phase 1: Setup

**Goal**: Reproducible Go, Flutter, and native macOS build inputs without a second
product interface.

- [X] T001 Initialize the single Go module and internal host entry point in `go.mod`, `go.sum`, `.go-version`, and `cmd/shipq-host/main.go`; pin a supported stable toolchain and compatible SQLite/UUID dependencies under the research version policy, with no user-facing commands. Depends: none.
- [X] T002 [P] Scaffold the Flutter macOS app and test setup in `apps/desktop/pubspec.yaml`, `apps/desktop/pubspec.lock`, `.flutter-version`, and `apps/desktop/macos/Runner.xcodeproj/project.pbxproj`; pin Flutter/Dart, verify macOS 13+ compatibility and arm64/x86_64 build support, and record any required reviewed platform-floor change. Depends: none.
- [X] T003 [P] Scaffold the Swift platform helper and bundle/service metadata in `platform/macos/Package.swift`, `platform/macos/Sources/ShipqPlatform/main.swift`, `platform/macos/ShipqPlatform/Info.plist`, and `platform/macos/LaunchAgents/com.shipq.host.plist`; separate helper identity from Flutter presentation and avoid privileged installation. Depends: none.
- [X] T004 [P] Create synthetic two-organization/three-repository PR, review, protocol, and failure fixtures in `tests/fixtures/github/pr_pages.json`, `tests/fixtures/github/reviews.json`, `tests/fixtures/protocol/base_envelopes.json`, and `tests/fixtures/scenarios/pr_management.json`; include multiple pages, self/other authors, one/two/three approvals, dismissed/comment-only reviews, and incomplete responses without real account data or credentials. Depends: none.
- [X] T005 Add build/developer entry points and artifact ignores in `Makefile`, `scripts/build_macos.sh`, `.gitignore`, and `README.md`; build the internal host, native helper, and Flutter debug bundle, document exact pinned toolchains and single-test commands, and reserve fixture/test targets for their later implementing tasks rather than reporting placeholders as successful. Depends: T001, T002, T003.

## Phase 2: Foundational and Risk-First Gates

**Goal**: Establish shared rules, storage/transport boundaries, deterministic
test infrastructure, and genuine integration evidence before product slices.

- [X] T006 Define shared domain observations, stable references, typed errors, and narrow application ports in `internal/domain/pullrequests/observation.go`, `internal/domain/identity/reference.go`, and `internal/application/ports/`; preserve "All local record IDs are UUIDs", remote `(github_host, node_id)` identity, UTC boundary instants, completeness metadata, and injected clocks/IDs without concrete adapter dependencies. Depends: T001, T004.
- [X] T007 [P] Write table-driven rule and validation-use-case tests in `internal/domain/segments/rules_test.go` and `internal/application/rules_test.go`; cover the complete field/operator matrix, exact/minimum counts, independent authorship/approvers, any/all/none, grouped negation, three-valued unknowns, type/version errors, and "maximum depth 16, 256 total nodes, 100 values per set, and 64 KiB encoded definition". Depends: T006.
- [X] T008 Implement typed rule trees, validation, evaluation, and readable summaries in `internal/domain/segments/rules.go`, `internal/domain/segments/evaluate.go`, and `internal/application/rules.go`; satisfy T007, resolve `me` from account identity, enforce user-only approval sets/team-only requested-reviewer support, repository-scoped labels, Unicode case-folded title matching, and start-inclusive/end-exclusive dates without hidden saved-rule defaults. Depends: T007.
- [X] T009 [P] Write effective-review reduction tests in `internal/domain/reviews/effective_test.go` using T004 fixtures; cover repeated approvals, approve-then-comment, changes requested, pending/dismissed states, dismissal of older versus current opinions, new commits, deleted reviewers, and separation of submitted participants from effective approvers. Depends: T006.
- [X] T010 Implement review facts/reduction in `internal/domain/reviews/effective.go`; count distinct current approvers, never resurrect an older approval after dismissal of the latest decisive opinion, preserve `reviewed_by`, use the contract's review-state precedence, and return unknown when evidence cannot establish an answer. Depends: T009.
- [X] T011 [P] Write real temporary-SQLite tests for initialization, migration failure, profile isolation, constraints, and process ownership in `internal/adapters/sqlite/database_test.go` and `internal/host/ownership_test.go`; verify interrupted initialization never exposes a ready API or bypasses a held ownership lock. Depends: T006.
- [X] T012 Implement database lifecycle, ordered embedded migrations, core metadata/cache tables, and ownership lock in `internal/adapters/sqlite/database.go`, `internal/adapters/sqlite/migrations/001_base.sql`, and `internal/host/ownership.go`; enable WAL/foreign keys/bounded busy timeouts and keep "One application host owns the SQLite connection pool and mutations". Depends: T011.
- [X] T013 [P] Write local transport and boundary-schema tests in `internal/adapters/localapi/server_test.go` and `api/local/v1/schema_test.go`; cover directory `0700`, socket `0600`, short socket paths, ownership collisions, protocol/error envelopes, origin/content-type/unknown-property rejection, and "1 MiB maximum" bodies without a default TCP listener. Depends: T006.
- [X] T014 Implement versioned local API routing, DTO/error mapping, status/rule-validation handlers, and private socket setup in `internal/adapters/localapi/server.go`, `api/local/v1/types.go`, and `internal/adapters/localapi/rules.go`; use application ports rather than evaluating rules or opening SQLite in handlers. Depends: T008, T013.
- [X] T015 [P] Write Dart schema/Unix-socket connection tests in `apps/desktop/test/application/local_client_test.dart` against shared protocol fixtures; cover valid/invalid envelopes, host-unavailable versus empty lists, version mismatch, and secret-free device-session responses. Depends: T002, T004, T006.
- [X] T016 Implement the thin Dart local client and typed responses in `apps/desktop/lib/application/local_client.dart` and `apps/desktop/lib/application/local_models.dart` using `HttpClient.connectionFactory`/Unix sockets; map typed errors and keep tokens, rule evaluation, SQLite, and GitHub HTTP out of Flutter. Depends: T014, T015.
- [X] T017 Write platform capability tests and an opt-in signed smoke harness in `platform/macos/Tests/ShipqPlatformTests/PlatformTests.swift` and `tests/platform/macos_host_smoke_test.go`; cover explicit permission intent, denied/locked/unavailable states, safe private IPC, helper failure, and Flutter-independent notification submission. Depends: T003, T006.
- [X] T018 Implement native Keychain/UserNotifications/ServiceManagement capabilities and their Go adapter in `platform/macos/Sources/ShipqPlatform/Keychain.swift`, `platform/macos/Sources/ShipqPlatform/Notifications.swift`, `platform/macos/Sources/ShipqPlatform/ServiceManagement.swift`, and `internal/adapters/macos/platform.go`; keep secrets out of arguments/logs and expose genuine denial/failure/approval status. Depends: T017.
- [ ] T019 Wire a minimal host/Flutter/native composition and execute the signed risk spike in `internal/host/runtime.go`, `apps/desktop/macos/Runner/PlatformBridge.swift`, `scripts/build_macos.sh`, and `tests/platform/artifacts/host_smoke.json`; prove one Unix-socket round trip, Keychain access, approved login service, helper restart, explicit stop, and notification submission with Flutter terminated on the supported architectures; block full desktop rollout if the real bundle arrangement fails. Depends: T005, T010, T012, T014, T016, T018.
- [ ] T020 Implement the isolated fixture host/scenario driver and app launch targets in `tests/support/fixturehost/main.go`, `tests/support/scenario/driver.go`, and `Makefile`; provide `fixture-host` and `run-desktop-fixture` with temporary profiles, fake ports/clocks/IDs, explicit socket handoff, and test-only step controls absent from release builds. Depends: T004, T019.
- [ ] T021 Add tested architecture/coverage enforcement and routine CI in `scripts/check_architecture.py`, `scripts/check_core_coverage.py`, `scripts/tests/`, `.github/workflows/ci.yml`, and `Makefile`; enforce inward production imports, Go formatting/vet/race tests, at least 90% statements separately for domain/application with empty groups failing, Flutter formatting/analysis/tests, and `test-core`, `test-adapters`, `check-core-coverage`, and `check` targets without requiring live accounts or signing in routine CI. Depends: T008, T010, T012, T014, T016, T018, T020.

**Foundation gate**: T019 has genuine platform evidence (or an explicitly
reviewed design revision), T020 isolates fixtures from production, and T021
passes for the implemented foundation. No story starts by duplicating missing
core behavior in Flutter. The constitution remains a scaffold; its examples
are not additional requirements.

## Phase 3: US1 - Connect and Find Relevant PRs (Priority: P1)

**Goal**: Connect an account, choose organization/repository scope, and browse
accurate filtered PRs with stable pagination and honest cached/offline states.
**Independent test**: Using the fixed collection, complete app onboarding,
filter/list/detail/open/watch flows, traverse pages while refreshes occur, then
repeat cached browsing with networking disabled. No saved segments are required.

### Tests first

- [ ] T022 [P] [US1] Write device-flow/account tests in `internal/adapters/github/device_flow_test.go` and `internal/application/account_test.go`; cover polling/slow-down/expiry/cancel, `repo` scope disclosure, expired/revoked access, Keychain failure, account isolation, and disconnect preventing further publication without exposing tokens or inventing a refresh grant. Depends: T021.
- [ ] T023 [P] [US1] Write paginated discovery/PR-collection fixture tests in `internal/adapters/github/discovery_test.go` and `internal/adapters/github/observations_test.go`; cover >1,000 inventory items without Search, all nested pages, complete-field checks, review normalization, head/update races, private-repo denial, and `Retry-After`/primary/secondary limit responses. Depends: T021.
- [ ] T024 [P] [US1] Write query/cache/refresh/watch tests in `internal/application/pr_queries_test.go`, `internal/application/refresh_test.go`, `internal/application/watches_test.go`, and `internal/adapters/sqlite/observations_test.go`; cover stable sort/tie-breakers, expiry/LRU/restart invalidation, incomplete-candidate suppression, refresh cancellation, unknown/stale coverage, require-fresh failure, immutable pages during updates, and persistence without GitHub subscription changes. Depends: T021.
- [ ] T025 [P] [US1] Write onboarding/scope/list/filter widget tests in `apps/desktop/test/account/onboarding_test.dart`, `apps/desktop/test/scope/scope_picker_test.dart`, and `apps/desktop/test/prs/pr_list_test.dart`; verify visible scope permissions, loading/denied/offline/empty distinctions, explicit operators, author/approver separation, all listed result fields, and validated browser opening. Depends: T021.
- [ ] T026 [P] [US1] Write fixture-backed app acceptance tests in `apps/desktop/integration_test/local_pr_flow_test.dart` and read-only opt-in live checks in `tests/platform/github_account_smoke_test.go`; cover onboarding through watch/unwatch and disconnect, organization/repository pagination, restart persistence, and cached browsing without a terminal or live credentials in routine runs. Depends: T021.

### Implementation

- [ ] T027 [US1] Model profiles/scopes and their validation in `internal/domain/accounts/profile.go`, `internal/domain/accounts/scope.go`, and `internal/application/ports/accounts.go`; enforce "One GitHub.com account per profile; no token in SQLite" and "Explicit all-repositories-in-org versus selected-repositories mode", with version/revision/timestamps and stable identity instead of display-name ownership. Depends: T022, T023.
- [ ] T028 [US1] Implement OAuth App device exchange in `internal/adapters/github/device_flow.go` and `internal/adapters/github/device_client.go`; use only the public client ID, honor documented intervals/cancellation/returned expiry fields, verify the live registration in the opt-in harness, and avoid requesting unsupported refresh behavior. Depends: T022, T027.
- [ ] T029 [US1] Implement connect/status/disconnect and scope use cases in `internal/application/account.go` and `internal/application/scope.go`; store credentials through the Keychain port, expose only safe session data, cancel authenticated work on disconnect, retain caches unless explicitly cleared, and reject silent identity replacement. Depends: T028.
- [ ] T030 [US1] Implement profile/scope/observation/review/subscription stores and mappings in `internal/adapters/sqlite/accounts.go`, `internal/adapters/sqlite/observations.go`, and `internal/adapters/sqlite/migrations/002_pr_cache.sql`; preserve IDs, user-record versions, per-field completeness, observation intervals, merged-versus-closed state, and separate historical participants from effective opinions. Depends: T024, T027.
- [ ] T031 [US1] Implement read-only GitHub repository discovery, all-state PR backfill, nested collection pagination, and normalization in `internal/adapters/github/discovery.go`, `internal/adapters/github/observations.go`, and `internal/adapters/github/transport.go`; resolve every required page, follow rate-limit guidance, recheck detected changes, mark ambiguous reviews unknown, and validate sanitized dismissal fixtures against opt-in GitHub evidence. Depends: T023, T028.
- [ ] T032 [US1] Implement manual refresh, resumable per-repository progress, coverage, and trusted publication in `internal/application/refresh.go`; never overwrite trusted data with incomplete candidates or treat page absence/access denial as an exit, support overlapping update discovery plus later fair rechecks, and stop publication after disconnect/clear cancellation. Depends: T029, T030, T031.
- [ ] T033 [US1] Implement shared ad hoc query evaluation and frozen query storage in `internal/application/pr_queries.go` and `internal/host/query_snapshots.go`; preserve "Client pages see one stable view; not portable state", page size 50/max 200, 15-minute expiry, 32 MiB per-profile LRU budget, query-bound opaque cursors, stable sorting, visible open-state default, and explicit unknown/expired responses. Depends: T008, T024, T030.
- [ ] T034 [US1] Implement local watches in `internal/application/watches.go`, `internal/adapters/sqlite/watches.go`, and `internal/adapters/sqlite/migrations/003_watches.sql`; enforce "Unique profile/PR; never changes a GitHub subscription", durable UUID/revision metadata, idempotent watch state, and separate local versus repository watching. Depends: T024, T030.
- [ ] T035 [US1] Wire account/session, scope/discovery, PR query/detail/watch, refresh/job, and explicit local-data clearing routes in `internal/adapters/localapi/account.go`, `internal/adapters/localapi/prs.go`, and `internal/adapters/localapi/refresh.go`; preserve typed HTTP errors, expected revisions, fresh-versus-cached behavior, and token exclusion from every response. Depends: T029, T032, T033, T034.
- [ ] T036 [US1] Implement account/device-session onboarding in `apps/desktop/lib/features/account/onboarding.dart` and `apps/desktop/lib/features/account/account_controller.dart`; show requested scopes, copy/open/cancel/expiry/retry controls, connected identity, missing organization authorization, and safe disconnect without implementing OAuth exchange in Dart. Depends: T025, T035.
- [ ] T037 [US1] Implement organization/repository selection in `apps/desktop/lib/features/scope/scope_picker.dart` and `apps/desktop/lib/features/scope/scope_controller.dart`; distinguish selected organizations' full accessible inventories from explicit repository subsets and render discovery progress, denied access, and revision conflicts. Depends: T025, T035.
- [ ] T038 [US1] Implement ad hoc field/operator controls in `apps/desktop/lib/features/filters/field_controls.dart` and `apps/desktop/lib/features/filters/filter_controller.dart`; cover FR-005 fields, independent authorship/approvers, explicit count operators, watch meanings, date ranges, and the core-provided readable summary without another evaluator. Depends: T025, T033, T035.
- [ ] T039 [US1] Implement paginated PR list/detail, sorts, refresh progress, local watches, and validated GitHub opening in `apps/desktop/lib/features/prs/pr_list.dart`, `apps/desktop/lib/features/prs/pr_detail.dart`, and `apps/desktop/lib/features/prs/pr_controller.dart`; show FR-004 fields, immutable page navigation, snapshot-expired recovery, and stale/partial/unknown status instead of empty-success substitutions. Depends: T036, T037, T038.
- [ ] T040 [US1] Compose the production account/browsing slice in `internal/host/runtime.go`, `cmd/shipq-host/main.go`, and `apps/desktop/lib/main.dart`; expose app-driven local service start with explicit consent, keep one database owner, and ensure the signed app starts and browses without developer commands or fixture hooks. Depends: T019, T035, T039.
- [ ] T041 [US1] Execute US1 checks from `quickstart.md` using `apps/desktop/integration_test/local_pr_flow_test.dart` and `tests/platform/github_account_smoke_test.go`; record fixture results separately from opt-in live authorization evidence in `tests/acceptance/artifacts/us1.json`, proving no pagination omissions, safe disconnect, and usable offline cache before accepting this slice. Depends: T026, T040.

**Checkpoint**: A useful browsing app is demonstrable independently of saved
segments or transition notifications. US1 includes ad hoc filters/local watches;
it is not the complete agreed MVP.

## Phase 4: US2 - Express and Reuse PR Segments (Priority: P1)

**Goal**: Save, edit, and reuse precise grouped rules without changing their
meaning across sessions.
**Independent test**: Seed US1 observations, create a compound rule in the app,
compare its result IDs with an ad hoc query, restart, and verify persistence,
rename identity, deletion, and stale-edit conflict behavior without notifications.

### Tests first

- [ ] T042 [P] [US2] Write segment entity/store/use-case tests in `internal/domain/segments/segment_test.go`, `internal/adapters/sqlite/segments_test.go`, and `internal/application/segments_test.go`; cover "unique normalized name", "identity survives rename", versioned definitions, atomic optimistic updates, deletion, and unchanged grouping after persistence. Depends: T041.
- [ ] T043 [P] [US2] Write segment/rule API contract tests in `internal/adapters/localapi/segments_test.go` and shared segment fixtures in `tests/fixtures/protocol/segments/`; cover invalid/unsupported definitions, wrong property types, path-addressable validation errors, conflict revisions, and querying a saved segment without inserting hidden defaults. Depends: T041.
- [ ] T044 [P] [US2] Write grouped-editor/segment widget tests in `apps/desktop/test/segments/filter_editor_test.dart` and `apps/desktop/test/segments/segment_list_test.dart`; cover any/all/not grouping, exact versus minimum approvals, author versus reviewer controls, all/any/none approvers, unsupported identities, readable summaries, and notification default `none`. Depends: T041.
- [ ] T045 [P] [US2] Write saved-segment app acceptance tests in `apps/desktop/integration_test/segments_flow_test.dart`; assert result parity with ad hoc queries, restart/rename/delete behavior, no app restart to see edits, and explicit conflict recovery from two windows. Depends: T041.

### Implementation

- [ ] T046 [US2] Implement segment entities and storage-facing ports in `internal/domain/segments/segment.go` and `internal/application/ports/segments.go`; retain UUID, normalized name, definition/format version, notification mode, revision/timestamps, and "identity survives rename"; reject invalid definitions using the existing evaluator's validator. Depends: T042, T043.
- [ ] T047 [US2] Implement persisted segments and compare-and-swap updates in `internal/adapters/sqlite/segments.go` and `internal/adapters/sqlite/migrations/004_segments.sql`; enforce "unique normalized name" per profile and separate format/schema/protocol versions without adding export/import or sync metadata. Depends: T046.
- [ ] T048 [US2] Implement create/read/list/rename/edit/delete and validation-summary use cases in `internal/application/segments.go`; preserve operators/grouping/selected IDs, accept explicit expected revisions, distinguish semantic edits from renames for later epochs, and default entry/exit mode to `none`. Depends: T047.
- [ ] T049 [US2] Implement rule identity/label resolution in `internal/application/resolve_rule.go` and read-only lookup methods in `internal/adapters/github/identities.go`; retain display hints while saving stable references, resolve `me` against the connected account, support teams only for requested reviewers, and make deleted/renamed identities actionable without retargeting. Depends: T043, T048.
- [ ] T050 [US2] Wire segment CRUD, rule validation, and saved-segment PR queries in `internal/adapters/localapi/segments.go` and `internal/adapters/localapi/prs.go`; use shared use cases/DTO mapping, preserve revision errors and effective-rule summaries, and retain bounded immutable query semantics. Depends: T043, T049.
- [ ] T051 [US2] Implement the grouped condition editor in `apps/desktop/lib/features/filters/condition_editor.dart` using the existing field controls; offer all-of/any-of/negation, explicit operators, core validation paths, and visible unknown identities without requiring JSON or duplicating domain evaluation. Depends: T044, T050.
- [ ] T052 [US2] Implement segment list/create/rename/edit/delete screens in `apps/desktop/lib/features/segments/segment_list.dart` and `apps/desktop/lib/features/segments/segment_editor.dart`; save the exact visible rule/default state, support none/entry/exit/both preferences, and present persisted readable summaries without claiming an inactive monitor is running. Depends: T048, T051.
- [ ] T053 [US2] Implement revision-conflict and unsupported-definition handling in `apps/desktop/lib/features/segments/segment_controller.dart`; show current-versus-edited state and require explicit resolution, preserving durable segment identity and never silently downgrading an unsupported format. Depends: T050, T052.
- [ ] T054 [US2] Wire saved-query navigation and app refresh in `apps/desktop/lib/features/segments/segment_navigation.dart` and `apps/desktop/lib/features/prs/pr_controller.dart`; reflect edited/deleted segments without restarting the app while keeping existing pagination snapshots immutable and explicitly starting a new query after a definition change. Depends: T053.
- [ ] T055 [US2] Execute `apps/desktop/integration_test/segments_flow_test.dart`, segment persistence/migration tests, and `apps/desktop/test/segments/filter_editor_test.dart`; record results in `tests/acceptance/artifacts/us2.json` showing the exact/minimum/self examples and independent persisted-segment behavior without generating transition notifications. Depends: T045, T054.

**Checkpoint**: Browsing and saved segments are complete and independently
verifiable against fixed observations. Notification preferences are stored;
delivery begins only with US3.

## Phase 5: US3 - Receive Segment Transition Notifications (Priority: P2)

**Goal**: Record real observed segment entries/exits once and submit enabled
desktop notifications even with Flutter absent.
**Independent test**: Seed PRs/rules through fixture ports, establish baselines,
advance explicit observations, and compare exact history/outbox sequences and
native submission attempts. No full history/settings screen is required.

### Tests first

- [ ] T056 [P] [US3] Write transition/epoch tests in `internal/domain/segments/transitions_test.go` and `internal/application/evaluate_observation_test.go`; cover first/newly discovered PRs, semantic edits versus renames, scope/watch/notification changes, unknown observations, draft-to-ready, approval entry/exit, disabled notifications, A-to-B-to-A-to-B, and restart/replay with no false events. Depends: T055.
- [ ] T057 [P] [US3] Write fault-injected SQLite baseline/event/history/outbox transaction tests in `internal/adapters/sqlite/evaluations_test.go`; prove atomic rollback/publication, "Unique evaluation key/sequence", sequence persistence, and one history item despite retry/crash boundaries. Depends: T055.
- [ ] T058 [P] [US3] Write outbox/native capability tests in `internal/host/outbox_test.go` and `internal/adapters/macos/notifications_test.go`; cover stable request IDs, permission denial/revocation, helper failure, retry/ambiguous completion, safe explanations, and no claim of exactly-once visible OS delivery. Depends: T055.
- [ ] T059 [P] [US3] Write fixture transition acceptance, scheduling, and signed notification integration cases in `tests/integration/transitions_test.go`, `internal/host/scheduler_test.go`, `apps/desktop/test/segments/notification_preferences_test.dart`, and `tests/platform/segment_notifications_test.go`; include first-load silence, ruleset overlap, epoch resets, persisted default intervals, coalescing/fair rechecks/backoff, and the Flutter-terminated background path. Depends: T055.

### Implementation

- [ ] T060 [US3] Implement baseline epochs, monotonic transition sequences, and explanation values in `internal/domain/segments/transitions.go` and `internal/domain/notifications/event.go`; preserve "Semantically changed inputs never masquerade as remote PR transitions" and "Unknown observations do not overwrite trusted state", with the exact key/state machine in `contracts/rules.md`. Depends: T056.
- [ ] T061 [US3] Implement transactional baseline/event/history/outbox storage and initial persisted monitor preferences in `internal/adapters/sqlite/evaluations.go`, `internal/adapters/sqlite/monitor.go`, and `internal/adapters/sqlite/migrations/005_monitoring.sql`; enforce "Unique evaluation key/sequence", "One history entry per enabled event, preserved if delivery fails", and "Same event/request identity across retries", including pending/submitted/denied/failed/ambiguous states and safe display snapshots. Depends: T057, T059, T060.
- [ ] T062 [US3] Implement observation evaluation and affected-epoch invalidation in `internal/application/evaluate_observation.go` and `internal/application/invalidate_baselines.go`; atomically publish observations/baselines/events/outbox, baseline newly discovered PRs silently, retain trusted state on incomplete fields/access loss, and connect segment/scope/watch/notification edits without converting configuration changes into remote transitions. Depends: T060, T061.
- [ ] T063 [US3] Implement the outbox worker in `internal/host/outbox.go`; submit stable request IDs through the notification port, persist genuine submitted/denied/failed/ambiguous outcomes, preserve history on delivery failure, and recover after crashes without duplicating history or over-deduplicating later legitimate transitions. Depends: T058, T061.
- [ ] T064 [US3] Implement minimal history query/read-state application/storage operations in `internal/application/history.go` and `internal/adapters/sqlite/history.go`; expose event explanations and delivery state for acceptance tests, preserve readable history after segment deletion, and keep "Read state is independent of notification delivery". Depends: T061.
- [ ] T065 [US3] Wire transition/history, explicit notification-authorization, and minimal monitor enable/status operations in `internal/application/monitor.go`, `internal/adapters/localapi/history.go`, and `internal/adapters/localapi/monitor.go`; never prompt during refresh or expose secrets, and make stored-but-disabled notification settings distinguishable from active monitoring. Depends: T062, T063, T064.
- [ ] T066 [US3] Implement host scheduling and refresh/evaluation integration in `internal/host/scheduler.go` and `internal/host/runtime.go`; use default 300/minimum 60-second persisted preferences, one coalesced refresh per profile, fair rechecks including relevant closed/merged PRs, overlap/full reconciliation, visible backoff, and recovery without inventing unseen intermediate transitions. Depends: T032, T062, T065.
- [ ] T067 [US3] Connect the production native notification adapter and signed host bundle in `internal/adapters/macos/notifications.go` and `scripts/build_macos.sh`; retain app identity and explicit permission intent with no Flutter process, reuse the foundation helper instead of adding notification logic to Flutter, and propagate real platform availability/delivery errors. Depends: T018, T019, T063.
- [ ] T068 [US3] Connect segment notification preferences to explicit authorization/start-monitoring actions in `apps/desktop/lib/features/segments/notification_preferences.dart`; reuse the existing segment mode editor, display denied/inactive status, and never enable login startup or request permission as a hidden effect of saving a rule. Depends: T059, T065, T066, T067.
- [ ] T069 [US3] Extend the isolated scenario driver in `tests/support/scenario/transitions.go` and run `tests/integration/transitions_test.go`; verify the entire notification acceptance sequence, duplicate/restart safety, incomplete-page suppression, epoch changes, and legitimate repeated entries using controllable clocks rather than real sleeps. Depends: T059, T066, T068.
- [ ] T070 [US3] Run `tests/platform/segment_notifications_test.go` on the signed bundle with Flutter terminated and record `tests/acceptance/artifacts/us3.json`; verify permission-allowed/denied results and entry/exit history, distinguishing genuine platform evidence from fixture results and blocking acceptance if the background path fails. Depends: T067, T069.

**Checkpoint**: Transition notifications work independently of an open Flutter
window. Initial matching PRs remain listing-only; history is durable even when
desktop delivery is denied.

## Phase 6: US4 - Control Monitoring and Review History (Priority: P2)

**Goal**: Expose usable history/settings and transparent monitoring recovery in
the app, with safe local-data clearing.
**Independent test**: Seed persistent rules/baselines/history, change settings,
exercise pause/stop/restart/denial/offline/sleep recovery, and verify both stored
state and honest UI without requiring a new transition first.

### Tests first

- [ ] T071 [P] [US4] Write monitor-preference/control tests in `internal/application/monitor_test.go` and `internal/host/recovery_test.go`; enforce "Default 300; minimum 60; only explicit resume clears pause", revision conflicts, pause versus disable versus stop, last-attempt/last-success separation, and no auto-resume after explicit quit. Depends: T070.
- [ ] T072 [P] [US4] Write history/filter/clear tests in `internal/application/history_queries_test.go`, `internal/adapters/localapi/history_test.go`, and `internal/application/clear_profile_test.go`; cover read-state revisions, snapshot filtering/expiry, deleted-segment display, and clearing credentials/cache/rules/watches/baselines/outbox/history plus in-memory snapshots without post-clear publication. Depends: T070.
- [ ] T073 [P] [US4] Write monitoring/history/settings widget tests in `apps/desktop/test/monitoring/settings_test.dart`, `apps/desktop/test/history/history_test.dart`, and `apps/desktop/test/account/clear_data_test.dart`; distinguish permission denial, service approval, backoff, stale coverage, unknown versus empty data, and explicit destructive confirmation. Depends: T070.
- [ ] T074 [P] [US4] Write recovery app integration scenarios in `apps/desktop/integration_test/recovery_flow_test.dart` and `tests/platform/lifecycle_recovery_test.go`; cover sleep/wake, revoked repository access, offline history/rule edits, two-window conflicts, helper restart, login approval, and explicit stop followed by app reopening. Depends: T070.

### Implementation

- [ ] T075 [US4] Implement complete monitor-preference/status use cases and persistence in `internal/application/monitor.go` and `internal/adapters/sqlite/monitor.go`; enforce "Default 300; minimum 60; only explicit resume clears pause", expected revisions, durable enabled/paused state, and separate refresh/coverage/backoff/permission status rather than conflating them. Depends: T071.
- [ ] T076 [US4] Complete history queries/read updates in `internal/application/history_queries.go`, `internal/adapters/sqlite/history.go`, and `internal/adapters/localapi/history.go`; support segment/PR/direction/read/time filters, immutable pagination, stable ordering, idempotent read updates with conflicts, and "Read state is independent of notification delivery". Depends: T072.
- [ ] T077 [US4] Complete monitor/settings routes and safe profile teardown in `internal/adapters/localapi/monitor.go` and `internal/application/clear_profile.go`; validate intervals/revisions, cancel work before clearing all profile-owned records and Keychain entries, invalidate snapshots, and prevent old in-flight responses or worker jobs from repopulating a cleared profile. Depends: T072, T075, T076.
- [ ] T078 [US4] Implement monitoring/settings UI in `apps/desktop/lib/features/monitoring/settings.dart` and `apps/desktop/lib/features/monitoring/monitor_controller.dart`; provide enable/disable, pause/resume, refresh-now, interval, explicit service start/quit and approved login-startup controls, with last success/attempt, coverage/backoff/errors, and notification permission state without terminal setup. Depends: T073, T077.
- [ ] T079 [US4] Implement paginated history/filter/read/open UI in `apps/desktop/lib/features/history/history_list.dart` and `apps/desktop/lib/features/history/history_controller.dart`; display PR/segment/direction/observation time/known change explanation and actual delivery state, preserve offline use and deleted-segment snapshots, and open only validated GitHub URLs. Depends: T073, T076.
- [ ] T080 [US4] Wire native lifecycle/deep-link recovery and explicit service-stop persistence in `platform/macos/Sources/ShipqPlatform/ServiceManagement.swift`, `apps/desktop/macos/Runner/AppDelegate.swift`, and `apps/desktop/lib/application/lifecycle_controller.dart`; keep close-window distinct from quit-service, reconnect to one host, show externally revoked approval, and offer rather than silently perform restart after explicit stop. Depends: T074, T078.
- [ ] T081 [US4] Add explicit disconnect-versus-clear workflows in `apps/desktop/lib/features/account/clear_data.dart`; confirm destructive clearing, invalidate visible profile/query state, retain cached user data on plain disconnect, and surface genuine Keychain/teardown failures rather than claiming success. Depends: T073, T077, T080.
- [ ] T082 [US4] Execute `apps/desktop/integration_test/recovery_flow_test.dart`, `tests/platform/lifecycle_recovery_test.go`, and `make check`; record `tests/acceptance/artifacts/us4.json` proving restart persistence, no replay/false exits, truthful denied/offline states, and no automatic restart after explicit service quit. Depends: T074, T079, T081.

**Checkpoint**: All four stories are functional. Passing fixture tests alone
does not replace required signed native and opt-in authorization evidence.

## Phase 7: Polish and Cross-Cutting Release Gates

**Goal**: Demonstrate the complete agreed MVP, not add another platform or feature.

- [ ] T083 [P] Add migration-upgrade and format-compatibility fixtures/tests in `tests/fixtures/storage/`, `internal/adapters/sqlite/migrations_test.go`, and `tests/integration/record_versions_test.go`; prove UUID/meaning preservation across supported versions, independent rule/schema/protocol versions, and safe rejection of unsupported formats without implementing sync or transfer protocols. Depends: T082.
- [ ] T084 [P] Add the repeatable 10,000-PR/100-segment performance scenario in `tests/performance/`, `apps/desktop/integration_test/query_performance_test.dart`, and `Makefile`; measure at least 100 release-app first-page queries on the documented reference Mac and healthy refresh-to-outbox timing, failing above two-second p95 or interval-plus-60-second targets while reporting backoff separately. Depends: T082.
- [ ] T085 [P] Verify credential/redaction and release-isolation contracts in `tests/integration/secret_boundaries_test.go` and `tests/platform/release_bundle_test.go`; ensure no credentials in rules/SQLite/logs/history/DTOs/arguments, no default TCP server or remote PR mutation adapter, and no test scenario injection or unintended product CLI in release artifacts. Depends: T082.
- [ ] T086 Finalize arm64/x86_64 signed packaging and distribution validation in `scripts/package_macos.sh`, `.github/workflows/release.yml`, and `tests/platform/artifacts/release_checks.json`; validate bundled host/helper identities, notarization workflow, normal app onboarding with no terminal, and genuine login/notification authorization without committing signing credentials or requiring them for routine CI. Depends: T083, T084, T085.
- [ ] T087 Update actual build/run/single-test commands and acceptance prerequisites in `README.md`, `specs/001-pr-management/quickstart.md`, and `.github/copilot-instructions.md`; replace planned commands with verified results where appropriate, document scope/permission disclosure, and keep remaining limitations explicit rather than claiming deferred cloud/mobile features. Depends: T086.
- [ ] T088 Execute the full agreed acceptance guide and all required checks from `specs/001-pr-management/quickstart.md`, recording sanitized results in `tests/acceptance/artifacts/mvp.json`; verify FR-001 through FR-022 and SC-001 through SC-007, both core coverage gates, race/static checks, app/core semantic consistency, and signed/live gates before marking the MVP complete. Depends: T087.

## Dependencies and Delivery Order

```text
Setup T001-T005
       |
Foundation T006-T021 (signed/native + IPC gates)
       |
US1 T022-T041: connect and browse (first useful demo)
       |
US2 T042-T055: reusable segments
       |
US3 T056-T070: observed transitions and notifications
       |
US4 T071-T082: complete history/settings/recovery
       |
Release gates T083-T088
```

Stories are independently verifiable with seeded fixtures, not independent
implementations of shared dependencies. US2 uses US1 observations/queries;
US3 uses US2 definitions; US4 exposes the monitor/history created by US3.
Do not claim these stories can be built in arbitrary parallel order.

Within a phase, use explicit task dependencies as the readiness graph; task
number alone is not a dependency. Phase gates are additional prerequisites.
T021 is the shared foundation checkpoint; T041, T055, T070, and T082 are the
story checkpoints. Validate every referenced task ID before execution.

### Parallel Opportunities

| Ready batch | Independent work | Constraint |
|-------------|------------------|------------|
| Setup | T001, T002, T003, T004 | T005 integrates their outputs |
| Foundation after T006 | T007, T009, T011, T013, T015 | Disjoint tests; T015 also needs setup Flutter/fixtures |
| US1 after T021 | T022, T023, T024, T025, T026 | Separate test targets; all consume shared fixtures read-only |
| US2 after T041 | T042, T043, T044, T045 | Separate domain/store/API/UI/integration test files |
| US3 after T055 | T056, T057, T058, T059 | Pure state, transaction, delivery, and integration tests |
| US4 after T070 | T071, T072, T073, T074 | Control/history/widget/recovery test groups |
| Release after T082 | T083, T084, T085 | Separate compatibility, performance, and isolation targets |

### Per-Story Parallel Examples

- **US1**: write T023 GitHub pagination/failure tests while T025 builds widget
  expectations; neither edits the fixture definitions or the other's files.
- **US2**: write T042 persistence/CAS cases while T044 builds grouped-editor
  scenarios; share the contract, not implementation code.
- **US3**: write T057 transaction fault injection while T058 tests notification
  delivery outcomes; integrate only after their typed contracts agree.
- **US4**: write T071 scheduler/control behavior while T073 defines visible
  settings/history states; complete the prerequisite story before either starts.

## Requirement and Outcome Coverage

| Requirements/outcomes | Principal task IDs |
|-----------------------|--------------------|
| FR-001, FR-002 | T018, T022, T027-T029, T035-T036, T072, T077, T081, T085 |
| FR-003, FR-004 | T023-T026, T030-T033, T035, T037-T041 |
| FR-005, FR-006, FR-007, FR-008 | T007-T010, T031, T033-T034, T038-T039, T043-T044, T049-T051 |
| FR-009, FR-010 | T024, T034, T042, T046-T055 |
| FR-011, FR-012 | T052, T056-T062, T065-T070 |
| FR-013, FR-014 | T058-T059, T061, T063-T065, T067, T070, T072, T076, T079 |
| FR-015, FR-016 | T017-T019, T065-T068, T070-T071, T074-T075, T077-T078, T080, T082 |
| FR-017, FR-018 | T023-T024, T031-T033, T039, T056, T066, T074, T079, T082 |
| FR-019, FR-020, FR-021 | T006, T011-T016, T030, T040, T047-T055, T061-T062, T077, T083 |
| FR-022 | T023, T031, T034, T085 |
| SC-001 | T023-T024, T026, T033, T039, T041 |
| SC-002 | T007-T010, T026, T038, T043-T045, T055, T059, T069 |
| SC-003 | T056-T063, T069-T070, T082 |
| SC-004 | T066-T070, T084 |
| SC-005 | T033, T084 |
| SC-006 | T041, T045, T055, T057, T069-T070, T074, T080, T082 |
| SC-007 | T024-T026, T039, T041, T045, T074, T079, T082 |

Data-model constraints belong to their first consuming slice: shared IDs and
observation completeness in T006/T030, profiles/scopes in T027, watches in T034,
segments in T046-T047, transactional monitoring in T060-T064, preferences in
T075, and read-state/history in T064/T076. T083 protects their upgrade boundaries.

## Implementation Strategy

1. Deliver setup/foundation in bounded implementation PRs, keeping the native
   risk spike visible rather than postponing it behind a complete UI.
2. Deliver US1 as the first useful app demo. Do not market browsing alone as the
   full MVP: saved segments, notifications, and monitoring controls are required.
3. Add US2, US3, and US4 as separate testable vertical slices. A slice can use
   multiple small PRs; keep each branch `feat/<slug>` and use Conventional Commit
   subjects and the repository PR template.
4. Run each slice's targeted tests before its checkpoint; keep CI/race/core
   coverage gates active as behavior grows. Signed/live validation is explicit,
   not a fabricated routine-CI success.
5. Execute release gates only after the user stories pass. Keep secrets,
   machine-specific artifacts, and test account identities out of the public
   repository. Preserve stable ports without building deferred sync features.

**Checklist summary**: 88 tasks: setup 5, foundation 16, US1 20, US2 14, US3 15,
US4 12, and release gates 6. There are 28 tasks explicitly marked `[P]`;
additional independent setup work is shown in the parallel table.
