# macOS App Contract

The Flutter app is the only user-facing client in this feature. It presents Go
application operations through the [local API](local-api.md), not a second
implementation of GitHub access, persistence, rules, or monitoring.
The Go host is an internal bundled executable, not a product CLI.

## Onboarding and account

- Show connected identity, authorization status, and the scope/permissions
  disclosure before starting device authorization.
- Present the verification URL and user code, with open-in-browser, copy, cancel,
  expiry, and retry states. Never display or retain an access token.
- Discover organizations and repositories with paginated progress and visible
  access failures. Distinguish all repositories in selected organizations from
  an explicitly selected subset.
- Disconnect removes credentials and stops authenticated work. Clearing all
  local profile data is a separate action with explicit confirmation.
- Offer enabling the local background service and login startup with visible
  user consent and macOS approval status. Normal use must not require a terminal.

## PR browsing and details

- Default to open PRs sorted by most recently updated with a stable identity
  tie-breaker. Users can also sort by creation time or approval count, retaining
  the stable tie-breaker. Make the active state condition visible.
- Show title, organization/repository, number, author, state/draft status,
  assignees, requested reviewers, effective approvers/count, update time, and
  freshness. Detail views expose review participants and incomplete fields.
- Support page size 50 by default, maximum 200. Next/previous navigation stays
  within a frozen local query snapshot even while monitoring refreshes data.
  Expired or evicted snapshots request an explicit restart from page one.
- Refresh requests background work and displays progress; it does not fabricate
  fresh results from stale cached fields.
- Cached results remain usable offline. Stale/partial/unknown results and coverage
  counts remain distinct from an authoritative empty list.
- Open PR uses the validated GitHub URL in the default browser. Watch/unwatch
  changes only the local watch state.

## Filter and segment editor

- Provide labeled fields/operators matching the [rule contract](rules.md).
  Approval count explicitly offers exactly, more than, at least, fewer than,
  and at most; no implicit comparison is inferred from a bare number.
- PR authorship inclusion/exclusion and approver identity are separate controls.
  Approver selection supports any, all, or none, including the connected user.
- Allow grouped all-of, any-of, and negation without requiring users to write
  JSON or expressions. Display the Go core's readable summary and typed
  validation errors adjacent to the affected controls.
- Support organization/repository, assignee, requested reviewer, reviewed-by,
  local/repository watch, state/draft, labels, title, dates, and review-state
  filters in addition to approval conditions.
- Saving a filter creates a named segment with the exact visible conditions.
  Notification mode is none, entry, exit, or both; default none.
- Segment creation, rename, edit, and deletion persist across sessions. Save
  with expected revisions; conflicts from another window show the current
  definition rather than silently overwriting it.
- Initial matches appear immediately in the list once known, without transition
  history or desktop notifications. Semantic edits establish a new baseline.

## History and monitoring settings

- History shows PR, segment, entry/exit direction, observed time, explanation,
  read state, and notification submission/denial/failure status.
- Filter history by segment, PR, direction, read state, and observation-time
  range; mark entries read and open the associated PR.
- Expose enable/disable, pause/resume, refresh-now, and interval controls
  (default 300 seconds, minimum 60), plus last attempt/success, backoff,
  coverage/errors, login-startup approval, and notification permission.
- Ask for notification permission only after a user action, not silently during
  a refresh. Denial must not remove local history.
- Closing a window leaves the background service running. Pause monitoring,
  disable monitoring, and quit the background service are distinct actions.
  After an explicit service quit, reopening the app offers an explicit start
  action rather than silently resuming monitoring.

## Validation expectations

Flutter widget tests cover filter operators/grouping, saving/restoring
definitions, invalid input, stale/conflict states, and monitoring/history controls.
Shared transport fixtures compare the Dart client with the Go API adapter.
Fixture-backed app integration tests compare ad hoc results, saved-segment
membership, and monitor events against the same expected PR set.

No application CLI commands, terminal output formats, or terminal onboarding
are deliverables. Internal build tools and fixture drivers are developer tooling,
not an additional product interface.
