# Local Application API v1

## Transport and ownership

The Go application host owns the core, adapters, SQLite, refresh coordination,
and notification outbox. The Flutter app calls its HTTP/JSON application API
over a Unix-domain socket. This is internal IPC, not a public or cloud API.
Future mobile embedding is a separate host adapter, not part of this feature.

The socket lives in a short per-user runtime directory (directory mode `0700`,
socket mode `0600`). Paths must fit macOS Unix-socket limits; do not assume the
long application-support path will fit. Persistent data lives separately under
`~/Library/Application Support/Shipq/`. Only one host may acquire the profile
ownership lock. Never expose a TCP listener by default.

Use `/v1` routing, UTF-8 JSON, UTC timestamps, opaque cursors, and request IDs.
The host rejects unsupported versions, oversized bodies (1 MiB maximum), invalid
content types, unknown mutation properties, and browser-origin requests. No CORS
access is enabled. Socket access protects against other OS users, not arbitrary
malicious programs already running as the same user.

DTOs and application operations are explicitly mapped at this adapter boundary;
the core does not import HTTP or JSON types. A shared schema-fixture suite
verifies the Go API adapter and Dart client against the same examples.

## Operations

| Method and route | Intent |
|------------------|--------|
| `GET /v1/status` | Protocol/host versions, active profile, monitor state, permission state, coverage |
| `POST /v1/account/connections` | Start a device-authorization session |
| `GET /v1/account/connections/{id}` | Device flow status; never returns an access token |
| `DELETE /v1/account/connections/{id}` | Cancel an authorization session |
| `GET /v1/account` | Connected identity and authorization status |
| `DELETE /v1/account` | Disconnect, cancel refresh work, and delete credentials |
| `DELETE /v1/profile/data` | Explicitly confirmed clearing of local profile data |
| `GET /v1/organizations` | Paginated accessible organizations with discovery coverage |
| `GET /v1/repositories` | Paginated repositories filtered by organization |
| `GET /v1/scope`, `PUT /v1/scope` | Read/update monitoring scope with revision check |
| `POST /v1/pr-queries` | Validate/evaluate a rule and establish an immutable local result snapshot |
| `GET /v1/pr-queries/{id}?cursor=...` | Retrieve the next page of the same snapshot |
| `GET /v1/prs/{node_id}` | Cached PR details with per-field observation status |
| `PUT /v1/watches/{pr_id}`, `DELETE /v1/watches/{pr_id}` | Local watch/unwatch |
| `GET /v1/segments`, `POST /v1/segments` | List/create saved segments |
| `GET /v1/segments/{id}`, `PUT /v1/segments/{id}`, `DELETE /v1/segments/{id}` | Read/update/delete with revision checks |
| `POST /v1/rules/validate` | Validate and obtain the core's canonical readable summary |
| `POST /v1/history-queries` | Filter history and establish a paginated snapshot |
| `GET /v1/history-queries/{id}?cursor=...` | Next history page |
| `PATCH /v1/history/{id}` | Set read status with revision check |
| `GET /v1/monitor`, `PATCH /v1/monitor` | Read/update enabled, paused, and interval preferences |
| `POST /v1/refreshes` | Start/coalesce a refresh; return a job ID |
| `GET /v1/refreshes/{id}` | Progress, completion, safe errors, and coverage |
| `POST /v1/platform/notification-authorization` | Request authorization through the signed native helper after user intent |

Service registration/start/stop is a host-platform operation, not a generic
business use case. Flutter lifecycle controls use the macOS adapter; normal
onboarding requires no terminal commands. Notification authorization cannot be
requested implicitly by a background refresh.

## Request and response shapes

Segment mutations carry `expected_revision` when editing/deleting, name, the
[versioned rule](rules.md), and notification mode. Creation returns an ID and
revision. Deletion retains display snapshots in existing history.

PR-query creation accepts exactly one of `segment_id` or `rule`, plus sort,
page size, and `require_fresh`. Snapshot creation returns its first page:

```json
{
  "request_id": "request-id",
  "data": {
    "items": [],
    "snapshot_id": "opaque-id",
    "next_cursor": null,
    "effective_rule": {"format_version": 1, "condition": {"field": "state", "op": "in", "value": ["open"]}}
  },
  "meta": {
    "observed_at": "2026-11-10T22:00:00Z",
    "freshness": "stale",
    "coverage": {
      "state": "partial",
      "repositories_complete": 2,
      "repositories_selected": 3,
      "unknown_pr_count": 4,
      "reasons": ["rate_limited"]
    }
  }
}
```

PR DTOs include the fields in FR-004, stable identities, effective review
information, known local/remote watch states, and completeness metadata. A stale
or partial response is not a claim that the empty result contains every match.
The host's rule validation response supplies a readable summary and typed
validation paths for the UI; Flutter only edits/displays the definition.

`POST /v1/rules/validate` accepts the definition itself as its JSON body
(`format_version` and `condition`), not a wrapper or encoded string. The raw
UTF-8 body, including whitespace, is subject to the independent 64 KiB rule
limit. A successful response has `data: {summary, required_fields}`.
Malformed shapes/types, null required values, duplicate or unknown properties
return `400 invalid_request`; unsupported rule versions and domain-invalid
rules return `422` with `details.path`. Identity/label wire values are defined
in [rules](rules.md).

Status data includes `protocol_version`, `host_version`, `active_profile_id`,
`monitor_state`, `notification_permission`, and `coverage: {state}`. Unbuilt
monitoring/native capabilities remain explicitly `unavailable`; unknown
coverage is not complete coverage. An unavailable/uninitialized status provider
returns `503 host_unavailable`. Rich refresh/coverage reporting is added with
its implementing slices.

Every handled response carries a generated UUID `request_id` and matching
`X-Request-ID`, plus `X-Shipq-Protocol-Version: 1`; client-supplied correlation
headers are not echoed. Response envelopes contain exactly one of `data` or
`error`. Mutation bodies require uncompressed `application/json`, with no
charset or UTF-8 charset. Browser-origin/fetch requests return
`403 forbidden_origin`, media failures `415 unsupported_media_type`, and
over-limit bodies `413 payload_too_large`. Unknown routes and wrong methods
return typed `404 not_found` and `405 method_not_allowed` envelopes.

Successful single-object commands return `data`; list queries add immutable
snapshot/cursor fields. Snapshot expiry is 15 minutes, with a bounded 32 MiB
per-profile memory budget and least-recently-used eviction. Expired, evicted, or
restart-invalidated snapshots return an explicit error. They never substitute
fresh results for page two of an old query.

Authorization-session creation returns a session ID, verification URL, short
user code, expiry, and polling guidance. The host owns the device code and token
exchange. The verification URL is validated against GitHub's documented host
before opening it. Flutter never receives the resulting token.

## Errors and concurrency

Errors use `error: {code, message, retryable, details}` with no secrets or raw
upstream bodies. Stable codes include `validation_failed`, `auth_required`,
`authorization_denied`, `repository_unavailable`, `rate_limited`,
`revision_conflict`, `snapshot_expired`, `unsupported_version`,
`platform_unavailable`, and `internal`.

Use HTTP `400` for invalid input, `401` for missing GitHub authorization,
`403` for denied local operations, `404` for unknown local IDs, `409` for
revision conflicts, `410` for expired query snapshots, `422` for invalid rules,
and `503` for unavailable required capabilities. Cached queries remain possible
after a GitHub authorization failure unless `require_fresh` was requested.

Serialize refreshes per profile and coalesce concurrent manual/scheduled
requests. A refresh cancellation cannot roll back already published PR
observations but prevents further publication after disconnect or profile clear.
Configuration mutations use revision checks; repeated watch/read-state updates
are idempotent.

## Client behavior

Flutter retains no authoritative configuration store. It can read cached state
through an offline-capable host without reaching GitHub.
Flutter polls host/job status while visible; push streams are deferred.
Connection loss shows host-unavailable state, not an empty PR list.

The thin Dart client takes an explicit absolute Unix socket path (at most
103 UTF-8 bytes), with no proxy, redirect following, TCP fallback, or automatic
host startup. It validates response media, matching request IDs, protocol
headers/status versions, and typed payloads. Transport loss, whole-request
timeout (default ten seconds), malformed response, incompatible protocol,
explicit closure, and server API errors are distinct failures. It bounds
request/response bodies to 1 MiB and cancels owned I/O on timeout/closure.
The Go validator remains authoritative for rule meaning and the 64 KiB limit.
Metadata is retained without treating partial/unknown coverage as complete.
The device-session guidance DTO is strict and accepts only HTTPS
`github.com/login/device` verification URLs without credentials, query, fragment,
or a nonstandard port. Account endpoints and terminal-session shapes arrive
with the account slice; the current client only calls status and validation.

Close Window, Pause Monitoring, Disable Monitoring, and Quit Background Service
are distinct actions. Opening the app after explicit service stop must not
silently restart monitoring. Installing/enabling login startup requires visible
user consent and macOS approval.
