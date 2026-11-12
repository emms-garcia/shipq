# Shipq

## Project context

Shipq is a local-first GitHub PR manager focused on a macOS application.
The repository contains Spec Kit artifacts, the macOS app foundation, and a Go
domain library for stable identities, observations, effective reviews, and typed
rule evaluation, plus an application rule-validation use case.
SQLite lifecycle/migrations and a host profile-ownership initializer also exist.
The local API library implements strict v1 schemas, status/rule-validation
handlers, and owned private Unix sockets. The host initializes one owned profile
before serving and shuts transport down before releasing ownership.
Flutter has a thin typed Unix-socket client for status/rule validation, tested
against shared fixtures and temporary sockets; the foundation screen uses status
after an explicit host start.
The Swift helper implements bounded private capability IPC and native wrappers;
the Go macOS adapter invokes it through private pipes. A desktop-owned bridge
controls explicit host/service actions. Synthetic fixtures, an isolated scenario
host, architecture/coverage checks, and CI exist. The architecture below is the agreed
target; GitHub access, user-record/PR stores, monitoring, and signed native acceptance
are not implemented yet.

The active feature is recorded in `.specify/feature.json`. Read its `spec.md`
for behavior and acceptance criteria, then `plan.md` and `tasks.md` when present.
The initial feature lives in `specs/001-pr-management/`.

## Target architecture

- Use hexagonal architecture: a shared Go core owns business rules and
  application use cases; ports describe the external capabilities it needs.
  Dependencies point inward and `scripts/check_architecture.py` enforces the Go
  production boundaries. The core must not depend on Flutter,
  concrete GitHub clients, SQLite, or operating-system integrations.
- The Flutter application is a presentation/driving adapter over Go application
  operations. It does not reimplement approval counting, filter evaluation,
  segment membership, or transition detection. A user-facing CLI is out of scope.
- GitHub access and SQLite persistence are infrastructure adapters implementing
  core-facing ports. Normalize external and stored representations at those
  boundaries rather than exposing them as domain models.
- Keep ports focused on application needs and required consistency guarantees,
  not SQL tables or GitHub endpoints. Add abstractions for actual MVP boundaries,
  not hypothetical future services.
- Open profile databases through `host.OpenProfile`: acquire ownership before
  initialization and close the database before releasing the lock. The low-level
  SQLite opener requires its caller to hold ownership. Never remove a live or
  stale-looking lock file; OS locking, not file existence, determines ownership.
- Start the local API only after owned profile initialization. Give it a short,
  dedicated current-user runtime directory, never the long persistence path.
  Keep socket ownership through graceful shutdown; do not replace live sockets
  or remove a different owner's path.
- Pass the runtime socket path explicitly to `LocalClient`; never fall back to
  TCP or start a host as a connection side effect. Keep transport/schema errors
  distinct from successful empty results, and close clients owned by controllers.
- Establish shared design tokens/theming before implementing feature screens;
  do not hard-code spacing, colors, or other visual styling.
- Platform adapters own credential storage, desktop notification delivery, and
  background-process lifecycle. Core monitoring operations must be callable
  independently of an open Flutter window.
- Keep native-helper credentials on private stdin/stdout pipes, never arguments,
  inherited secret environment, public DTOs, or logs. Use an explicit trusted
  bundle executable path; retain ambiguous notification submission on lost ack.
- Invoke service registration from the desktop-owned bundle bridge:
  `SMAppService.agent` uses the calling app's LaunchAgents directory, not its
  parent bundle. The nested helper must report unavailable rather than register
  the wrong job. No permission request or service mutation without user intent.
- The merged implementation plan selects one Go application host owning
  SQLite and monitoring, with Flutter calling HTTP/JSON over a
  private Unix socket. See `specs/001-pr-management/plan.md` and its contracts;
  this is an accepted design, not implemented infrastructure.
- SQLite is the MVP persistence adapter, not the definition of core behavior.
  Preserve durable identities and explicit record versions for user-owned data.
  Keep credentials and disposable GitHub caches separate from portable records.
- Export/import and cloud synchronization are deferred. Preserve extension
  boundaries, but do not implement sync protocols, cloud infrastructure, conflict
  resolution, or speculative sync adapters in the MVP.

## Product invariants

- PR authorship and approver identity are independent filter dimensions.
  Exactly two approvals and at least two approvals are distinct operators.
- Ad hoc browsing, saved segments, and monitoring use the same rule semantics
  and persisted configuration.
- Initial matches appear in PR listings, not desktop notifications or transition
  history. Notifications describe subsequent observed membership changes.
- Failed or incomplete refreshes must not create false segment exits. Preserve
  trustworthy baselines and avoid duplicate events across app sessions and restarts.
- MVP GitHub access is read-only. Local watching does not change remote
  subscriptions.

## Agreed testing and coverage policy

These are implementation and CI requirements. Grouped coverage enforcement and
routine macOS CI are implemented; signed/live checks remain opt-in.

- Go domain rules use table-driven unit tests. Application use cases use fake
  ports, clocks, and GitHub responses so behavior is deterministic.
- Adapter integration tests use real temporary SQLite databases and
  recorded or synthetic GitHub fixtures. Routine tests must not require live
  GitHub accounts.
- Flutter widget tests cover filter editing, PR listings, and notification
  settings. Prioritize meaningful interactions rather than a UI coverage quota.
- Keep a small end-to-end suite covering account connection, refresh, segment
  matching, transitions, and history, including restart recovery.
- Enforce at least 90% statement coverage for Go domain and use-case packages.
  Do not impose a repository-wide coverage percentage initially.
- Percentage coverage does not replace explicit behavioral cases. Cover filter
  semantics, effective reviews, pagination, persistence, offline behavior,
  app/core consistency, and especially cases that must not generate notifications,
  including initial matches, rule edits, incomplete refreshes, and replay after
  restart.
- CI must check Go formatting, run vet, race-enabled tests, and collect/enforce
  coverage. It must also check Flutter formatting, run static analysis, and run
  Flutter tests.

## Current developer commands

- Read `README.md` for pinned tools and limitations. Do not silently upgrade
  shared tool installations.
- Run `make deps` after dependency-manifest changes or missing-dependency
  failures; keep the Go sums and Flutter application lockfile committed.
- `make check` runs architecture, script, Go race, independent core coverage,
  Dart/Flutter, fixture, and Swift checks. `make test-ui` and `make test-fixtures`
  run focused suites; `make test-platform` runs injected Swift capability tests.
  Signed platform smoke remains explicitly opt-in, not routine acceptance.
- Follow `README.md` for focused rule/review tests and independent aggregate
  domain/application coverage. Never report an empty application group as
  100% covered or imply future use cases have already been exercised.
- `make build` assembles a native-architecture ad-hoc-signed debug app;
  `make build-arm64` and `make build-intel` select an architecture explicitly.
  None installs a service or proves the signed T019 native capabilities.

## Branches, commits, and pull requests

- Name working branches `feat/<slug>`.
- Use Conventional Commits 1.0.0 for commit subjects and PR titles:
  `<type>[optional scope][!]: <description>`. Choose the type that describes the
  change, such as `docs` for specifications or `chore` for tooling; the branch
  prefix does not require the commit type to be `feat`.
- Follow `.github/pull_request_template.md`: Description, Changes, ELI5, and
  Testing. Keep the description and testing notes concise, the changes a short
  list, and ELI5 a simple explanation.

## Spec Kit workflow

Use the corresponding `.github/skills/speckit-*/SKILL.md` when performing a
Spec Kit phase. The normal sequence is specify, clarify when needed, plan,
tasks, then implement; the bundled workflow has spec and plan review gates.

Run Python helpers from the repository root; the configured script mode is
Python. Useful commands:

```sh
# Inspect feature paths without changing persisted feature selection.
python3 .specify/scripts/python/check_prerequisites.py --json --paths-only

# Validate implementation prerequisites and discover supporting documents.
python3 .specify/scripts/python/check_prerequisites.py --json --require-tasks --include-tasks

# Resolve the composed spec template rather than copying a single template file.
python3 .specify/scripts/python/resolve_template.py spec-template --json
```

Feature directories are independent of Git branch names. Helpers resolve
`SPECIFY_FEATURE_DIRECTORY` before `.specify/feature.json`; `SPECIFY_FEATURE`
alone does not select a directory. `SPECIFY_FEATURE_NO_PERSIST=1` prevents
helper invocations from persisting feature selection.
The feature pointer is ignored machine-local state. On a fresh checkout, set
`SPECIFY_FEATURE_DIRECTORY=specs/001-pr-management` when working on the initial
feature.

Use template composition so project overrides, presets, and extensions are
preserved. Generated artifacts must replace template examples and placeholders;
the constitution currently contains placeholders, not adopted project principles.

Tasks use IDs such as `T001`, optional `[P]` parallel markers, and user-story
markers such as `[US1]`. Parallel tasks must have no dependencies or shared-file
conflicts. Mark completed implementation tasks `[X]`.

Checklist markers describe requirements quality, not implementation completion.
Implementation reads them as a gate and must not mark them complete itself.
Check the applicable skill for extension hooks if `.specify/extensions.yml`
is introduced.
