# Data Model

## Boundaries

Domain values and use cases are independent of SQLite row shapes, GitHub JSON,
client DTOs, and platform-specific paths. User-owned records, disposable remote
cache, internal monitoring state, and credentials are separate categories.
The persistence adapter provides transactions in application terms.

All local record IDs are UUIDs; remote entities use `(github_host, node_id)` as
their canonical reference, retaining numeric IDs and display hints when useful.
UTC instants use RFC 3339 at external boundaries. User-owned records include
`format_version`, `revision`, `created_at`, and `updated_at`.
Revision increments support optimistic concurrency now, not a future sync protocol.

## User-owned records

| Entity | Principal fields and relationships | Constraints |
|--------|------------------------------------|-------------|
| Profile | `id`, GitHub host, connected user ID/login, `credential_ref`, authorization status | One GitHub.com account per profile; no token in SQLite |
| MonitoringScope | `id`, `profile_id`, organization IDs, repository selection mode and IDs, revision | Explicit all-repositories-in-org versus selected-repositories mode |
| Segment | `id`, `profile_id`, unique normalized name, definition version, condition tree, notification mode, revision | Validated by [rules](contracts/rules.md); identity survives rename |
| LocalWatch | `id`, `profile_id`, PR reference, watched status, revision | Unique profile/PR; never changes a GitHub subscription |
| MonitoringPreferences | `profile_id`, enabled/paused state, interval seconds, revision | Default 300; minimum 60; only explicit resume clears pause |
| HistoryReadState | `history_entry_id`, read flag, revision, `updated_at` | Read state is independent of notification delivery |

The profile record stores a reference to a credential, not an exportable secret.
Disconnect keeps cached/user-owned records unless the user separately confirms
clearing them. Switching to another identity requires a new profile or clearing
the old one; identities cannot silently inherit another account's baselines.

## Disposable GitHub observations

| Entity | Principal fields and relationships |
|--------|------------------------------------|
| Organization / Repository | Stable references, owner/name, authorization state, last discovered time |
| User / Team / Label | Stable reference and mutable display hints; label belongs to repository |
| PullRequestObservation | PR reference, repository, number, title, URL, author, assignees, requested users/teams, state, draft, labels, created/updated times, head revision |
| ReviewObservation | Review ID, PR, reviewer reference, submitted time, state, reviewed commit; normalized effective opinion |
| RepositorySubscription | Profile/repository, watched/ignored/not-watching state, observation status/time |
| ObservationEnvelope | PR reference, observation revision, collection start/end, complete-field set, missing-field reasons, semantic content hash |
| RefreshRun / Coverage | Profile/scope, attempted/succeeded times, repository/page progress, next retry time, error category and safe message |

Keep all submitted-review participants needed for `reviewed_by` separately from
effective opinions. An inaccessible or deleted actor is represented explicitly,
not as an empty approval set. A closed PR records merged status separately so
the normalized state can distinguish `closed` and `merged`.

An observation is published only after the referenced fields and their
paginated collections are collected and validated. An incomplete candidate does
not overwrite the last trusted observation. Preserve collection timestamps and
expose partial coverage; GitHub does not provide an atomic multi-request snapshot.

## Internal monitoring and delivery records

| Entity | Principal fields and relationships | Invariant |
|--------|------------------------------------|-----------|
| BaselineEpoch | Segment, definition revision, scope/input revision, notification configuration | Semantically changed inputs never masquerade as remote PR transitions |
| MembershipBaseline | Evaluation key, last trusted membership, observation revision, explanation inputs, sequence | Unknown observations do not overwrite trusted state |
| TransitionEvent | ID, evaluation key, sequence, direction, before/after observation refs, observed time, explanation | Unique evaluation key/sequence |
| NotificationHistory | ID, event ID, safe PR/segment display snapshot, read-state reference | One history entry per enabled event, preserved if delivery fails |
| NotificationOutbox | Event ID, request ID, pending/submitted/denied/failed/ambiguous state, attempt time, error code | Same event/request identity across retries |
| QuerySnapshot | In-memory ID, profile, query hash, immutable result DTOs, coverage, expiry | Client pages see one stable view; not portable state |

History snapshots preserve readable titles and segment names even when a rule is
deleted or a cache is cleared. Clearing all profile data explicitly removes
history, outbox, watches, rules, cache, and credentials.

Desktop submission is not an exactly-once visible-delivery guarantee: a crash
between OS acceptance and acknowledgment can be ambiguous. Use a stable OS
request identifier, record the uncertainty, and never create another history
entry for that retry.

## Transactions and concurrency

- One application host owns the SQLite connection pool and mutations. The app
  never opens the database directly. Migrations run before accepting requests.
- Commit observation publication and dependent baseline/event/outbox changes
  atomically per PR; the run may report partial repository coverage.
- Application ports express `commit observation and evaluations` and `save
  segment if revision matches`, not SQL operations or generic arbitrary
  repositories.
- Edits carry `expected_revision`; conflicting app windows receive a conflict and
  current revision. Do not silently overwrite a newer segment or preference.
- Maintain foreign keys, unique identity constraints, and indexes for
  profile/repository/state/update time, reviewers, and outbox status.
- Use WAL, bounded busy timeouts, transactions, and a process-ownership lock.
  Interrupted migrations fail safely before the API is ready.
- Query snapshots copy evaluated result DTOs and coverage after one consistent
  local read. New refreshes do not modify an existing page set.

## Persistence evolution

Schema migrations are ordered, versioned, and tested from supported older
fixtures. User-owned format versions are separate from SQLite schema versions
and the local protocol version. Unsupported rule versions fail explicitly.

Keep credentials in Keychain, remote payload/cache behind adapter boundaries,
and stable user configuration independent of machine paths. This supports
future export/import and optional synchronization adapters without adding
tombstones, replicated clocks, conflict resolution, or cloud dependencies now.

## State and validation checks

- Account: disconnected -> authorizing -> connected; permission/expiry failures
  yield reauthorization-required; disconnect cancels outstanding refresh work.
- Monitoring: stopped/running/paused/backoff with last-attempt/last-success and
  coverage independently visible; interval validation rejects values below 60.
- Baselines and events follow the [rule state machine](contracts/rules.md).
- Renamed entities keep stable IDs. Unresolvable references remain visible and
  cannot quietly retarget to another user/repository with the same name.
- Portable record boundaries are design constraints only; no export/import or
  cross-device merge behavior is implemented in this feature.
