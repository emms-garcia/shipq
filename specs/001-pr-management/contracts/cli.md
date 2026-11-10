# CLI Contract v1

These are planned commands, not commands supplied by the planning PR. The CLI
is a thin Go driving adapter over application operations. It does not own
filter evaluation, credentials, database access, or transition processing.

## Conventions

- Executable: `shipq`. Commands support `--help`; query/history output supports
  `--output table|json`. JSON is one envelope on stdout; diagnostics go to stderr.
- Exit codes: `0` success, `1` unexpected failure, `2` invalid input,
  `3` authentication required, `4` application host unavailable,
  `5` revision conflict, `6` expired pagination snapshot.
- A successful cached listing can have partial/stale coverage. Its JSON metadata
  and terminal warning must make that visible; `--require-fresh` instead fails
  with exit `1` when fresh, complete results are unavailable.
- Never print tokens or accept them as command-line arguments. Account connection
  uses a browser/device authorization flow.
- Human names in flags resolve to stable references before saving. `me` resolves
  to the connected GitHub account, not local Git author information.
- Destructive clearing requires an interactive confirmation or explicit `--yes`
  in noninteractive use.

## Commands

| Intent | Commands |
|--------|----------|
| Account | `account connect`, `account status`, `account disconnect [--clear-local-data] [--yes]` |
| Discover access | `organization list`, `repository list [--org NAME]` |
| Scope | `scope show`, `scope set --org NAME [--repo OWNER/NAME]` |
| PRs | `pr list`, `pr show OWNER/REPO#NUMBER`, `pr open OWNER/REPO#NUMBER` |
| Watches | `pr watch OWNER/REPO#NUMBER`, `pr unwatch OWNER/REPO#NUMBER` |
| Segments | `segment list`, `segment show ID`, `segment create --name NAME`, `segment update ID --expected-revision N`, `segment delete ID --expected-revision N` |
| History | `history list`, `history mark-read ID` |
| Monitor | `monitor status`, `monitor enable`, `monitor disable`, `monitor pause`, `monitor resume`, `monitor interval SECONDS`, `monitor refresh` |
| Host lifecycle | `service install`, `service uninstall`, `service status`, `service start`, `service stop`, `serve --foreground` |

`scope set` accepts repeated `--org` and `--repo`. Organization-only selection
includes all accessible repositories in the selected organizations; explicit
repository selection is a subset and must be shown before saving. Scope edits
use expected revisions like other configuration edits.

`monitor disable` persists disabled scheduling while keeping the host available
for cached browsing. `service stop` explicitly quits the application host and
disables automatic relaunch until an explicit start; clients show unavailable
and do not silently override that choice. `serve --foreground` is for development
and CLI-only operation. Installing the desktop service is explicit user consent.

## Filtering examples

```sh
# Exactly two approvals, including PRs I authored (the default).
shipq pr list --org example --approvals '=2'

# At least two approvals, excluding PRs I authored.
shipq pr list --org example --approvals '>=2' --exclude-author me

# At least two approvals, including my approval, regardless of authorship.
shipq pr list --org example --approvals '>=2' --approved-by me

# Require every selected approver.
shipq pr list --approved-by alice --approved-by bob --approver-mode all

# Use the complete grouped rule representation.
shipq pr list --where-file ./rule.json --output json

# Save a query with entry and exit notifications.
shipq segment create --name ready-for-me --where-file ./rule.json --notify both

# Evaluate the same saved definition without reconstructing filters.
shipq pr list --segment SEGMENT_ID --output json
```

Simple flags include repeated `--org`, `--repo`, `--author`, `--exclude-author`,
`--assignee`, `--requested-reviewer`, `--reviewed-by`, `--approved-by`, `--label`,
plus `--approver-mode any|all|none`, `--approvals`, `--state`, `--draft true|false`,
`--watched local|repository`, `--title`, `--created-from`, `--created-before`,
`--updated-from`, `--updated-before`, and `--review-state`. Repeated values within
one identity flag form a set; different flags form a conjunction. Negated/grouped
expressions use `--where-file` and the [rule contract](rules.md).

Count operators accept `=`, `>`, `>=`, `<`, and `<=`; a bare number is rejected
to avoid choosing equality versus minimum implicitly. Shell-sensitive
comparisons must be quoted. The CLI shows the core-provided readable summary
when saving a segment.

`segment create` accepts the same shorthand filters or `--where-file` as
`pr list`, making a query reusable without manually translating its conditions.

`--where-file`, `--segment`, and shorthand filtering are mutually exclusive.
Notification mode is `none|entry|exit|both`, default `none`. State filtering
defaults to open only for ad hoc interactive queries and is visible in saved
queries; saved segments are never altered by a hidden default.

## Pagination, history, and output

`pr list` and `history list` accept `--page-size` (default 50, maximum 200) and
`--cursor`. Cursors bind to one immutable local query snapshot and parameter set.
They expire after 15 minutes or a host restart; an explicit snapshot-expired
error requires starting from page one. Changing parameters invalidates a cursor.
The host never silently starts another snapshot mid-pagination.

History filtering includes segment ID, PR reference, entry/exit direction, read
state, and observation-time range. Sorting defaults to newest observation first.
PR sorting defaults to `updated_at desc, stable PR identity asc`; users may choose
creation time or approval count, retaining the stable tie-breaker.

The JSON response exposes items, query/snapshot ID, next cursor, effective rule,
and freshness/coverage metadata as defined by the local application contract.
No-match results are successful empty lists, distinct from inaccessible data or
failed refreshes.
