# Feature Specification: Local PR Management and Transition Notifications

**Feature Branch**: Not created; feature context is independent of the Git branch.

**Created**: 2026-11-10

**Status**: Draft

**Input**: Build a local-first CLI and macOS app for connecting a GitHub account, finding PRs across organizations, defining reusable rulesets/segments, and receiving notifications when PRs enter or leave them. Support flexible approval counts and reviewer conditions. Monitor in the background while the Mac is awake, even with windows closed. Prepare for future portability and cloud synchronization without implementing either.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Connect and Find Relevant PRs (Priority: P1)

As a developer, I can connect my GitHub account and browse paginated PRs across my accessible organizations and repositories from either the CLI or macOS app.

**Why this priority**: A reliable, cross-repository view provides value before saved segments or notifications exist.

**Independent Test**: Connect an account with access to multiple organizations, choose a scope, and retrieve the same filtered, ordered PR results through both interfaces.

**Acceptance Scenarios**:

1. **Given** a disconnected installation, **When** I connect my account, **Then** I can see the connected identity and choose from organizations and repositories it can access, including private repositories where authorized.
2. **Given** accessible PRs in multiple organizations, **When** I list PRs with organization, repository, author, assignee, requested-reviewer, reviewed-by, or watched filters, **Then** results match the selected conditions and clearly identify each PR's repository and relationship to me.
3. **Given** more results than fit on one page, **When** I traverse successive pages without changing the result snapshot, **Then** each matching PR appears once in deterministic order.
4. **Given** previously refreshed data, **When** GitHub is unavailable, **Then** I can browse cached results with their last-refresh time and an explicit stale or incomplete indicator.
5. **Given** I select a PR in either interface, **When** I open it, **Then** its GitHub page opens without changing the PR.

---

### User Story 2 - Express and Reuse PR Segments (Priority: P1)

As a developer, I can express precise PR filters and save them as named segments so I do not have to reconstruct my queries.

**Why this priority**: Flexible segmentation is the core differentiator and defines the conditions used by notifications.

**Independent Test**: Use a fixed set of PRs with different authors, reviewers, and approval counts to create segments and verify identical membership in both interfaces.

**Acceptance Scenarios**:

1. **Given** PRs with one, two, and three effective approvals, **When** I filter for exactly two approvals, **Then** only the two-approval PRs appear; **When** I filter for at least two, **Then** both the two- and three-approval PRs appear.
2. **Given** my PRs and other authors' PRs each with two approvals, **When** I exclude PRs authored by me, **Then** only other authors' PRs appear; **When** I remove that exclusion, **Then** both appear.
3. **Given** PRs with two approvals, some including mine, **When** I require my approval, **Then** only PRs with my effective approval appear regardless of their authorship.
4. **Given** PRs with different reviewers, **When** I require approvals from all of a specified set of people, or any of that set, **Then** membership follows the selected operator independently of the total approval-count condition.
5. **Given** a compound filter such as "in organization A, not authored by me, at least two approvals, and approved by me or person B," **When** I save and reopen the segment, **Then** its meaning and membership are preserved.
6. **Given** a segment created in one interface, **When** I view, edit, or delete it in the other, **Then** both interfaces reflect the same saved definition.

---

### User Story 3 - Receive Segment Transition Notifications (Priority: P2)

As a developer, I can enable notifications for entry into and/or exit from a segment, allowing me to respond to relevant PR changes without repeatedly checking GitHub.

**Why this priority**: Notifications build on trustworthy browsing and segmentation.

**Independent Test**: Establish a baseline, change PR states between successful refreshes, and verify entry/exit history and desktop notification attempts without duplicates.

**Acceptance Scenarios**:

1. **Given** a non-draft segment and a known draft PR, **When** the PR becomes ready and is observed in a successful refresh, **Then** an enabled entry notification identifies the PR and the draft-to-ready change.
2. **Given** a segment requiring at least two approvals, **When** a PR moves from one effective approval to two, **Then** it enters the segment and produces an enabled entry notification; a later move from two to three does not produce another entry.
3. **Given** a segment requiring exactly two approvals, **When** a matching PR gains a third approval, **Then** it leaves the segment and produces an enabled exit notification.
4. **Given** a segment requiring at least two approvals including mine, **When** another person's second approval arrives without mine, **Then** no entry occurs; **When** my approval subsequently satisfies the reviewer condition, **Then** entry occurs.
5. **Given** a matching PR whose approval is dismissed or replaced by a changes-requested review, **When** the effective conditions cease to hold, **Then** an enabled exit notification is recorded.
6. **Given** monitoring is enabled and all app windows are closed, **When** the awake, online Mac observes a qualifying transition, **Then** local history is updated and a desktop notification is attempted without an open CLI session.
7. **Given** a new segment, first refresh, or edited rule, **When** existing matches are evaluated, **Then** they appear in the PR listing and establish a baseline without generating desktop notifications or transition-history entries.
8. **Given** repeated refreshes or a monitor restart with unchanged PR state, **When** evaluation resumes, **Then** the same transition is not recorded again.

---

### User Story 4 - Control Local Monitoring and Review History (Priority: P2)

As a developer, I can control monitoring, inspect its health, and review notifications from either interface while retaining my configuration locally.

**Why this priority**: Background activity must be visible and controllable to be trustworthy.

**Independent Test**: Change settings, restart both interfaces, simulate denied notifications and connectivity failure, and verify persisted settings, history, and recovery behavior.

**Acceptance Scenarios**:

1. **Given** saved segments, watches, notification settings, and history, **When** I restart the app or CLI, **Then** these remain available with the same identities and definitions.
2. **Given** active background monitoring, **When** I pause it or explicitly quit the background monitor, **Then** refreshes stop and both interfaces show that status; closing a window alone does not stop monitoring.
3. **Given** desktop notifications are denied by macOS, **When** a qualifying transition occurs, **Then** it remains in local history and notification permission status is visible.
4. **Given** the Mac sleeps or loses connectivity, **When** monitoring resumes, **Then** it compares the last trustworthy state with the newly observed state, labels the observation time, and does not claim to have captured unseen intermediate transitions.
5. **Given** my access to a repository is revoked, **When** a refresh fails for that repository, **Then** its data is marked unavailable rather than generating segment-exit notifications.

### Edge Cases

- Multiple submitted reviews from one person count as one person's current effective review, not multiple approvals. A comment-only review does not itself revoke an earlier approval.
- Dismissed approvals do not count. New commits affect approval counts according to GitHub's effective review state; an approval threshold does not imply the PR satisfies all merge requirements.
- A PR can match multiple segments. Notifications and history identify the segment responsible; one transition per PR per segment is recorded, not one per application interface.
- A PR first discovered after monitoring begins establishes its own baseline without an entry notification, preventing newly granted access or an expanded scope from appearing as PR changes.
- A closed, merged, reopened, or draft PR is evaluated using its observed state and the segment's explicit state conditions.
- A failed, rate-limited, or incomplete refresh must not be interpreted as missing PRs, zero approvals, or segment exits. Conditions with unavailable required data remain unevaluated.
- Partial organization discovery or service result limits must be visible. Results must never be represented as complete when repositories or pages could not be retrieved.
- A changing result set must not silently corrupt pagination: retain the page snapshot or explicitly require restarting pagination after refresh.
- Simultaneous CLI, app, and monitor activity must not corrupt saved rules, lose history, or duplicate transition records.
- Invalid rules, contradictory count bounds, deleted reviewers, and renamed organizations/repositories produce actionable validation or resolution status rather than silently changing rule meaning.
- Changes to rule definitions, monitoring scope, or notification settings establish a new baseline where needed and do not synthesize PR transitions.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Users MUST be able to connect, inspect, and disconnect one GitHub.com account per local profile, with explicit feedback for expired credentials, missing organization authorization, and inaccessible repositories.
- **FR-002**: Credentials MUST be protected using operating-system credential facilities and MUST NOT appear in rule definitions, logs, notification history, or portable application records. Disconnecting MUST stop authenticated refreshes and clear credentials; users MUST also be able to explicitly clear locally cached account data.
- **FR-003**: Users MUST be able to select all accessible repositories in chosen organizations or a subset of repositories. Discovery MUST account for pagination and disclose any coverage limitations.
- **FR-004**: Both the CLI and macOS app MUST support listing, filtering, pagination, viewing PR details, and opening a PR on GitHub. Results MUST show title, repository, number, author, state, draft status, assignees, requested reviewers, effective approvers/count, update time, and data freshness.
- **FR-005**: Filters MUST cover organization, repository, author, assignee, requested reviewer, reviewed-by, locally watched PRs, repository-watch status, open/closed/merged state, draft/ready state, labels, title text, creation/update date ranges, approval count, effective approver identity, and review state (approved, changes requested, or no decisive review).
- **FR-006**: Approval-count conditions MUST support equality, greater-than, greater-than-or-equal, less-than, and less-than-or-equal with nonnegative whole numbers. Approval counting MUST use distinct reviewers with currently effective, non-dismissed approvals.
- **FR-007**: Reviewer conditions MUST support approvals by any, all, or none of a selected set of people, including the connected user. Authorship inclusion/exclusion MUST be a separate condition. "Include my PRs" MUST NOT imply "require my approval."
- **FR-008**: Users MUST be able to combine conditions using all-of, any-of, and negation with explicit grouping. Both interfaces MUST offer concise ways to express common count and identity filters, and show an unambiguous readable summary before saving.
- **FR-009**: Users MUST be able to create, inspect, rename, edit, and delete named segments. Saved definitions MUST preserve condition operators, grouping, selected identities, and notification settings across both interfaces and restarts.
- **FR-010**: Users MUST be able to watch/unwatch individual PRs locally. Local watching MUST remain distinct from GitHub repository watching and MUST NOT change GitHub subscriptions.
- **FR-011**: Users MUST be able to configure entry notifications, exit notifications, both, or neither per segment. Default new segments MUST have notifications disabled until explicitly enabled.
- **FR-012**: Monitoring MUST compare trustworthy observations against persisted baselines and record qualifying membership changes once per PR, segment, and observed transition. Initial observations and configuration changes MUST follow the baseline rules above.
- **FR-013**: Each notification-history entry MUST identify the PR, segment, direction, observation time, and the known condition changes explaining the transition. Users MUST be able to list/filter history, mark entries read, and open the associated PR from either interface.
- **FR-014**: Enabled transition notifications MUST be recorded locally and submitted for desktop notification delivery when permission allows. Delivery denial/failure MUST NOT remove history or cause duplicate transition records; actual display remains subject to macOS notification settings.
- **FR-015**: Users MUST be able to enable/disable background monitoring, pause/resume it, configure the refresh interval, request a manual refresh, and inspect last attempt, last success, coverage, errors, and notification permission status from either interface.
- **FR-016**: Enabled monitoring MUST continue while the Mac is awake with windows closed and no CLI session open. It MUST recover its settings and baselines after restart. Pausing or quitting the monitor MUST be distinct from closing a window.
- **FR-017**: Refreshes MUST respect GitHub service limits, expose delayed refreshes, and retry recoverable failures without treating incomplete or unavailable data as a membership change.
- **FR-018**: Cached browsing, saved-rule management, and history MUST remain available offline, with clear freshness indicators. Notifications MUST reflect observed changes only, not a guarantee of every transition during gaps in monitoring.
- **FR-019**: Application configuration, segments, watches, baselines, cached PRs, and notification history MUST persist on the user's device without a project-operated backend or cloud account. Access to GitHub remains necessary for authentication and refreshes.
- **FR-020**: CLI and desktop interfaces MUST share saved state and rule semantics. CLI listings and history MUST support structured output suitable for automation as well as readable terminal output.
- **FR-021**: User-owned records MUST have durable identities independent of display names and device-specific paths, with explicit record-format versions and modification metadata. Local upgrades MUST preserve those identities and rule meaning. Secrets and disposable GitHub caches MUST remain distinguishable from user-owned configuration to support later portability and synchronization without implementing those features now.
- **FR-022**: The MVP MUST be read-only toward GitHub PRs: approving, merging, commenting, assigning, and changing remote subscriptions are outside scope.

### Key Entities

- **Connected Account**: GitHub identity, authorization status, and a protected credential reference.
- **Monitoring Scope**: Selected organizations/repositories, discovery coverage, and accessibility status.
- **Pull Request Observation**: Stable GitHub identity, repository, descriptive fields, relationships, lifecycle state, review information, and observation freshness/completeness.
- **Effective Review**: A person's current decisive review state for a PR, accounting for superseded and dismissed reviews.
- **Local Watch**: A locally owned association between the user and a PR, separate from a remote subscription.
- **Segment**: Durable identity, display name, versioned grouped conditions, modification metadata, and entry/exit notification preferences.
- **Membership Baseline**: Last trustworthy membership and relevant observed state for a PR and segment definition.
- **Transition Event**: PR, segment, entry/exit direction, explanation, observation time, and identity used to avoid duplicate history.
- **Notification History Entry**: Transition reference, read status, and desktop delivery-attempt status.
- **Monitoring Preferences and Status**: Enabled/paused state, refresh interval, refresh health, and permission status.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Using a controlled collection spanning at least two organizations and three repositories, users can retrieve every accessible matching PR through pagination with zero duplicates or silent omissions in a stable snapshot.
- **SC-002**: Both interfaces produce identical membership for all acceptance fixtures, including exactly two versus at least two approvals, excluding/including authored PRs, and requiring the connected user's approval independently.
- **SC-003**: For a controlled sequence of observed changes, every enabled entry/exit produces exactly one history entry per affected segment, with zero false events caused by initial loading, rule edits, incomplete refreshes, or restarts.
- **SC-004**: While the Mac is awake and connectivity and GitHub access are healthy, qualifying changes are recorded and submitted for desktop notification delivery within one configured refresh interval plus 60 seconds. Service-limit delays are visible rather than hidden.
- **SC-005**: For 10,000 locally cached PRs and 100 saved segments, the first page of a cached filtered listing appears within two seconds in at least 95% of repeated measurements in both interfaces on the supported reference Mac identified during planning.
- **SC-006**: Restarting the CLI, desktop app, and monitor preserves all saved segment definitions, local watches, preferences, and history, and does not replay already recorded transitions.
- **SC-007**: All cached browsing, segment editing, and history acceptance scenarios remain usable with network access disabled, with no stale result presented as freshly verified.

## Assumptions

- The MVP targets macOS for both interfaces, but future desktop platforms must be considered when selecting technologies. Mobile and other operating systems are not MVP deliverables.
- One GitHub.com account per local profile is sufficient initially. Multiple simultaneous accounts and GitHub Enterprise Server are future scope.
- "Manage PRs" means organizing, monitoring, and opening them, not changing their remote state.
- Default interactive listings show open PRs, include the user's own PRs, and sort by most recently updated with a stable identity tie-breaker. Users can select other states and ordering.
- No approval-count operator is implicit in a saved rule: exactly two and at least two are distinct user choices, each available concisely.
- Confirmed with the user: existing matches on initial loading are communicated through the PR listing, not notifications. Notifications report subsequent observed transitions.
- "Watched" can mean locally watched PRs or PRs in GitHub-watched repositories; the interfaces must label these meanings separately.
- The default refresh interval is five minutes and the supported minimum is one minute, subject to service-limit backoff. Sleep, offline periods, and authorization failures pause freshness guarantees.
- Monitoring observes state snapshots, not a complete historical event stream. A PR that enters and leaves a segment entirely between observations may produce no notification.
- GitHub access and organizational policies determine discoverable data. The application cannot bypass permissions or promise complete coverage when the service prevents retrieval.
- Export/import, cross-device synchronization, cloud storage, shared/team rules, and mobile clients are deferred. The MVP retains compatible record boundaries and identities but does not implement transfer protocols or conflict resolution.
- Technology, persistence, and host-composition decisions are documented in the implementation plan; the local-only constraint remains a product requirement.
- Automated acceptance coverage is required for filter semantics, effective reviews, pagination, transition baselines/deduplication, persistence, offline behavior, and parity between interfaces.
