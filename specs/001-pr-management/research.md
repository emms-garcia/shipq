# Architecture Research and Decisions

**Date**: 2026-11-10
**Scope**: Go core, Flutter presentation, local macOS app MVP, future extension
boundaries without implementing export/import or cloud synchronization.

These are design selections for review in the planning PR, not implemented or
benchmarked capabilities. The user already agreed to Go, Flutter, SQLite,
hexagonal architecture, and the testing policy. Further confirmation was
unavailable during planning, so the bridge and authentication choices below use
the recommended defaults and explicitly disclose their tradeoffs. The user
subsequently removed the product CLI; this design now targets only the macOS app.

## 1. One local application host; HTTP/JSON over a Unix socket

**Decision**: The Go host owns the core, SQLite, scheduling coordination, and
outbox. Flutter calls a versioned local HTTP/JSON adapter over
a private Unix socket. The core itself remains a reusable Go library independent
of this process composition.

**Rationale**: One state owner avoids competing schedulers/database writers and
supports background monitoring without the Flutter process.
Dart documents `HttpClient.connectionFactory` with
`Socket.startConnect(InternetAddress(path, type: InternetAddressType.unix), 0)`
as its Unix-socket pattern. Standard Go `net/http` can serve a Unix listener.
No browser server, localhost TCP port, or custom binary ABI is needed.

**Alternatives considered**: Go `c-shared` plus Dart FFI can embed the Go runtime,
but still needs a separate background host and explicit memory/thread ownership.
Wails/Tauri would change the agreed Flutter presentation choice. gRPC is not
needed for the initial local request/response contract.

**Constraint**: This desktop composition is not a promise that mobile can run a
persistent Go daemon. Future mobile requires its own lifecycle/embedding adapter.
The single core and explicit application operations preserve that option.

Sources:
- https://api.dart.dev/dart-io/HttpClient/connectionFactory.html
- https://pkg.go.dev/net#Listen
- https://pkg.go.dev/net/http#Serve
- https://pkg.go.dev/cmd/go#hdr-Build_modes

## 2. Per-user macOS service and a native platform helper

**Decision**: Support macOS 13+ for the MVP. Package a per-user Go LaunchAgent
inside the signed desktop bundle, registered through `SMAppService` after
explicit consent. A small bundled Swift helper app owns Keychain and
UserNotifications integration. It can run independently of Flutter and receives
requests from the Go platform adapter through private child-process IPC.
No privileged system daemon is required.

**Rationale**: Apple's modern service API manages bundled helpers and user
approval. Flutter notification plugins alone do not prove delivery when Flutter
is absent. The native helper supplies an app identity for notification permission
and delivery, and keeps platform-specific credential APIs outside the core.

**Alternatives considered**: Keeping Flutter permanently resident is possible,
but unnecessarily couples platform delivery to the presentation runtime.
Shell `osascript` notifications do not provide the intended explicit app
identity/delivery-state contract. Direct CGo integration would broaden the Go
build requirements without removing platform-specific lifecycle work.

**Required implementation acceptance gate**: On a signed development bundle,
register the Go agent, terminate Flutter, obtain notification permission through
the helper, and submit a notification from a fixture transition. Verify
identity, user approval/denial, launch-at-login, explicit service stop, helper
restart, Keychain lock/unavailability, and arm64/x86_64 packaging.
Documentation alone is not proof this bundle arrangement works. Complete this
bounded spike before building the full desktop feature set; if the helper
arrangement fails, revise the host adapter/design explicitly rather than hiding
the issue behind a Flutter-only path.

Sources:
- https://developer.apple.com/documentation/servicemanagement/smappservice
- https://developer.apple.com/documentation/usernotifications/unusernotificationcenter
- https://developer.apple.com/documentation/usernotifications/asking-permission-to-use-notifications
- https://developer.apple.com/documentation/security/keychain-services

## 3. OAuth App device flow, with scope disclosure

**Decision**: Use a registered GitHub OAuth App with device flow enabled.
Ship a public client ID, never a client secret. Request `repo` and `read:org`
for private PRs and organization discovery; use the documented authenticated-user
subscription endpoints for repository watching. Store access/refresh credentials,
when returned, only through the Keychain port. Respect device-flow polling
intervals, slow-down responses, cancellation, and expiry.

**Rationale**: This supports a distributed local client without a service-owned
backend or installing a GitHub App into every organization. GitHub's documented
device-code exchange takes client ID, device code, and grant type, not a
distributed client secret.

**Important tradeoff**: Classic `repo` scope includes write capabilities.
Shipq remains read-only by application design, not by token permissions.
Show the requested scope and explain it before login. The GitHub adapter exposes
only read operations; no token reaches Flutter, local API responses,
rule definitions, or logs. Organization policies can still deny access.

**Alternatives considered**: A GitHub App provides narrower permissions but its
installation/user-access intersection may not cover all repositories already
accessible to the user. PAT-only setup is less approachable and creates another
credential-input workflow. Neither is included in this MVP.

**Expiry handling**: Do not assume OAuth App credentials have the same expiry
configuration as GitHub App user tokens. Honor actual returned expiry/refresh
fields and the registered application's supported flow. Test expired/revoked
credentials and reauthorization; do not invent an undocumented refresh grant.
The implementation must verify the registration settings and documented token
response before enabling refresh-token behavior.

Sources:
- https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps#device-flow
- https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/scopes-for-oauth-apps
- https://docs.github.com/en/apps/differences-between-github-apps-and-oauth-apps
- https://docs.github.com/en/rest/activity/watching

## 4. Per-repository REST collection, explicit coverage

**Decision**: Use GitHub REST endpoints, enumerate accessible repositories,
and follow all pagination links. Initial backfill lists PRs with `state=all`
per repository without an arbitrary history/result cap. Publish progress so
large backfills do not block cached browsing or imply completeness prematurely.
Do not use the cross-repository Search API as the authoritative inventory.

**Rationale**: GitHub search is limited to 1,000 results per query. Standard
repository list endpoints avoid that search cap. Every nested collection used
by predicates (reviews, assignees, labels, requested reviewers) must be complete;
a small first-page list is not a complete observation.

**Refresh strategy**: Reconcile selected repository discovery and changed PR
inventory with overlapping update windows. Re-fetch monitored open PRs and known
PRs relevant to active baselines on a fair scheduled sweep, including closed or
merged PRs. Periodic full reconciliation repairs watermark misses. A matching
PR that disappears from a page is not evidence that it left a segment.
Persist per-repository progress and resume interrupted backfills.

No update timestamp/watermark is assumed to capture every review or dismissal.
If rate limits prevent the configured freshness target, expose the delay and
prioritize fairly; do not silently truncate. Follow `Retry-After`, primary-limit
reset headers, and secondary-limit guidance with bounded concurrency/backoff.

**Consistency**: Collect a candidate PR observation, all dependent pages, and
recheck PR head/update metadata before publication. Retry detected changes.
GitHub does not promise atomic multi-request snapshots; this check reduces
known races but is not a guarantee against undetectable concurrent review
changes. Preserve observation intervals and emit events only from complete,
trustworthy observations. Frozen local query snapshots provide stable client
pagination independently of remote collection.

**Alternatives considered**: GraphQL can reduce N+1 requests, but adds nested
pagination and cost accounting. Its aggregate `reviewDecision` is not a count
of distinct approvers. Keep the GitHub port replaceable and optimize only after
measuring the REST adapter.

Sources:
- https://docs.github.com/en/rest/search/search
- https://docs.github.com/en/rest/pulls/pulls#list-pull-requests
- https://docs.github.com/en/rest/pulls/reviews#list-reviews-for-a-pull-request
- https://docs.github.com/en/rest/using-the-rest-api/using-pagination-in-the-rest-api
- https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api
- https://docs.github.com/en/rest/using-the-rest-api/best-practices-for-using-the-rest-api

## 5. Effective reviews are normalized, then evaluated in the core

**Decision**: Fetch complete review pages and map stable reviewer IDs and
chronological submitted review history to domain facts. For the current effective
opinion, ignore pending/comment-only records, retain the latest decisive record,
and let a latest dismissed decisive record clear that user's earlier opinion
rather than resurrecting an old approval. Count distinct effective approvers.
`reviewed_by` separately preserves all submitted-review participants.

**Rationale**: Historical approval rows are not an approval tally. The product
requires explicit approval counts and approver identities, not branch-protection
merge eligibility. The [rule contract](contracts/rules.md) defines the product
meaning once; adapter DTO quirks do not leak into Flutter or SQL predicates.

**Acceptance gate**: Capture sanitized fixtures for approve -> comment,
approve -> changes requested, repeated approvals, dismissal of the current
opinion, dismissal of an older opinion after a newer review, and stale-review
dismissal after a new commit. Validate against GitHub's visible state in an
opt-in test repository before declaring normalization correct. If the returned
history cannot resolve a case, mark it unknown and refine the adapter; do not
guess a zero count or substitute aggregate mergeability.

Sources:
- https://docs.github.com/en/rest/pulls/reviews
- https://docs.github.com/en/graphql/reference/objects#pullrequest

## 6. Persistence, dependencies, and toolchain policy

**Decision**: Use `database/sql` with `modernc.org/sqlite` as the SQLite adapter,
ordered SQL migrations embedded in that adapter, WAL, foreign keys, bounded
busy timeouts, and one host writer. No ORM or generic persistence framework.
Use Go standard HTTP/JSON/logging/testing packages and UUID record IDs.
Flutter uses `dart:io` for the local client and
its standard testing tools. Keep platform integration in a small Swift adapter.

**Rationale**: The SQLite driver is CGo-free and documents darwin arm64/amd64
support. This avoids a C toolchain for Go-only core builds; Flutter/native
packaging still needs Xcode. Respect the driver's documented compatible
`modernc.org/libc` version instead of independently upgrading it.

**Version policy**: At scaffold time select the then-supported stable Go and
Flutter releases, commit exact toolchain pins and lockfiles, and record the
bundled Dart/Xcode versions. Do not copy unverified "latest" version numbers into
the plan or require dependencies to be installed in this documentation PR.
Target macOS 13+ because of `SMAppService`; confirm the selected Flutter release
still supports that deployment target before pinning it, or explicitly revise
the platform floor in review.

**Alternatives considered**: `mattn/go-sqlite3` is established but adds CGo;
direct SQLite access from Dart would introduce a second authoritative storage
path. A cloud replica engine is unnecessary before sync is in scope.

Sources:
- https://pkg.go.dev/modernc.org/sqlite
- https://go.dev/dl/
- https://docs.flutter.dev/install/archive
- https://docs.flutter.dev/platform-integration/macos/building

## 7. Testing and coverage

**Decision**: Apply the agreed layered tests and 90% core coverage gate. Enforce
90% statement coverage for each of the domain and application groups so a
well-covered domain does not hide untested workflows. No global or Flutter
percentage target initially.

Use temporary real SQLite databases, injected clocks/identities/ports, fixture
GitHub responses, shared Go API/Dart client protocol examples, Flutter widget
tests, and a small app-to-core end-to-end suite. Add deterministic architecture import
checks and core race tests. Reserve live GitHub and signed platform tests for
explicit opt-in smoke/release validation.

**Rationale**: The highest-risk failures are semantic branches and restart/
incomplete-data behavior, not uncovered boilerplate. The
[quickstart](quickstart.md) defines runnable implementation-phase checks and
separates them from checks actually available in this planning PR.
