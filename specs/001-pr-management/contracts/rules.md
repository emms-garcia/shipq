# Segment Rule Contract v1

## Ownership and representation

The Go domain defines and evaluates one typed condition tree. Flutter controls
construct that tree without implementing a second evaluator.
Storage and client DTOs map to the domain at their boundaries.

A saved definition contains `format_version: 1`, a `condition`, and an explicit
default listing state. Conditions use exactly one of these shapes:

```json
{
  "format_version": 1,
  "condition": {
    "all": [
      {"field": "organization", "op": "in", "value": ["ORG_NODE_ID"]},
      {"not": {"field": "author", "op": "in", "value": ["me"]}},
      {"field": "approval_count", "op": "gte", "value": 2},
      {"field": "approvers", "op": "any", "value": ["me", "USER_NODE_ID"]}
    ]
  }
}
```

`all` and `any` take nonempty condition lists; `not` takes one condition.
A leaf has `field`, `op`, and `value`. Unknown versions, fields, operators,
properties, incompatible types, empty identity sets, and contradictory numeric
bounds in a conjunction fail validation rather than being silently ignored.
Limits: maximum depth 16, 256 total nodes, 100 values per set, and 64 KiB encoded
definition. Report a typed validation error with a path to the failing node.

## Field/operator matrix

| Field | Values | Operators |
|-------|--------|-----------|
| `organization`, `repository` | Stable GitHub node IDs | `in`, `not_in` |
| `author` | User node IDs or `me` | `in`, `not_in` |
| `assignees`, `requested_reviewers`, `reviewed_by`, `approvers` | User/team references as supported below | `any`, `all`, `none` |
| `approval_count` | Nonnegative integer | `eq`, `gt`, `gte`, `lt`, `lte` |
| `state` | `open`, `closed`, `merged` | `in`, `not_in` |
| `draft`, `local_watched`, `repository_watched` | Boolean | `eq` |
| `labels` | Repository-scoped label references | `any`, `all`, `none` |
| `title` | Nonempty text | `contains` |
| `created_at`, `updated_at` | UTC RFC 3339 instant | `gte`, `lt` |
| `review_state` | `approved`, `changes_requested`, `none` | `in`, `not_in` |

`me` is resolved against the connected account for each evaluation, never guessed
from local Git configuration. Display names/logins are resolved to stable IDs
when saving. Persist the original display hint for an actionable unresolved
identity message. Team references are allowed only for `requested_reviewers`;
approval sets and `reviewed_by` use individual user IDs.

Labels are resolved within each selected repository; a name selected across
repositories expands into explicit repository/label references. `title contains`
uses Unicode case-folded substring matching, not regular expressions. Date ranges
are start-inclusive/end-exclusive. All omitted fields impose no condition.

Interactive ad hoc listings default to `state in ["open"]`. Creating a segment
from such a listing persists that state condition visibly; evaluation of a
saved definition never inserts a hidden state restriction.

## Review semantics

- Count distinct users whose effective decisive review is `APPROVED`. A user's
  multiple approvals count once; approval count is not GitHub mergeability.
- GitHub normalization must account for dismissal, supersession, and new commits.
  A comment-only or pending review is not a new decisive opinion.
- `reviewed_by` means any submitted review by that person, including a
  comment-only or subsequently dismissed review; pending reviews do not count.
  It is distinct from `approvers` and `requested_reviewers`.
- The normalized `review_state` is `changes_requested` when at least one current
  decisive opinion requests changes; otherwise `approved` when at least one
  effective approval exists; otherwise `none`. It is not a claim that branch
  protection has been satisfied.
- Deleted/unresolvable required identities and incomplete review pages produce
  an unknown result where their absence cannot be established.

## Three-valued evaluation

Evaluate to `match`, `no_match`, or `unknown`, with required-field freshness and
explanations. Missing data must not be coerced to false, zero, or an empty set.
`not unknown = unknown`; `all` is false if any child is false, true if all are
true, otherwise unknown; `any` is true if any child is true, false if all are
false, otherwise unknown.

Listings expose unknown results separately from confirmed matches, with a count
and reasons in coverage metadata. Notification evaluation requires a complete,
trustworthy observation for all referenced fields; cached fallback data is
displayable but cannot silently supply missing fresh fields for a new event.

## Membership and notification state machine

The evaluation key is `(profile_id, segment_id, baseline_epoch, pr_id)`.
The epoch changes for semantic definition edits, scope changes, watched-input
configuration changes, or notification-mode changes; pure renames do not reset it.
Reset only affected segments when dependencies can be identified.

| Previous state | New observation | Result |
|----------------|-----------------|--------|
| No baseline | Known match/nonmatch | Set baseline; no event or notification |
| Any | Unknown/incomplete | Keep trusted baseline; show incomplete status |
| Nonmatch | Match | Record entry if enabled; advance baseline |
| Match | Nonmatch | Record exit if enabled; advance baseline |
| Match/nonmatch | Unchanged membership | No event; advance trusted observation |
| Any old epoch | First observation in new epoch | Set baseline without event |

Newly discovered PRs always establish a baseline first, including PRs discovered
after monitoring has started. Disabled notifications still allow trusted
baselines to advance; enabling them never replays past changes.

Commit each new trusted baseline, event, history entry, and notification-outbox
item in one transaction. Assign a monotonic transition sequence to each key;
a unique `(key, transition_sequence)` prevents duplicate records after retries.
Do not deduplicate only by state hash: a real A-to-B-to-A-to-B sequence contains
two distinct entries into B.

Explanations retain the relevant previous/current values and observation times,
not OAuth credentials or complete API payloads. Changes entirely between
observations cannot be reconstructed or promised.
