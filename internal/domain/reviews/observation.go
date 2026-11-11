package reviews

import (
	"time"

	"github.com/emms-garcia/shipq/internal/domain/identity"
)

type State string

const (
	Approved         State = "approved"
	ChangesRequested State = "changes_requested"
	Commented        State = "commented"
	Dismissed        State = "dismissed"
	Pending          State = "pending"
)

type Review struct {
	Reference   identity.Reference
	PullRequest identity.Reference
	// A zero reviewer represents an unresolved/deleted actor, not an empty set.
	Reviewer    identity.Reference
	State       State
	SubmittedAt time.Time
	Commit      string
}

type Snapshot struct {
	PullRequest identity.Reference
	HeadBefore  string
	HeadAfter   string
	Complete    bool
	Reviews     []Review
}

type UnknownReason string

const (
	IncompletePages    UnknownReason = "incomplete_pages"
	UnverifiedHead     UnknownReason = "unverified_head"
	ChangedHead        UnknownReason = "changed_head"
	InvalidReview      UnknownReason = "invalid_review"
	UnresolvedReviewer UnknownReason = "unresolved_reviewer"
	MissingOrder       UnknownReason = "missing_review_order"
	AmbiguousOrder     UnknownReason = "ambiguous_review_order"
	ConflictingReview  UnknownReason = "conflicting_review"
)

type Decision string

const (
	DecisionNone             Decision = "none"
	DecisionApproved         Decision = "approved"
	DecisionChangesRequested Decision = "changes_requested"
)

// Unknown sets are withheld. Opinion and submitted-participant completeness
// are independent; a missing comment author need not obscure the approval count.
type Facts struct {
	Approvers          []identity.Reference
	ChangesRequestedBy []identity.Reference
	ReviewedBy         []identity.Reference
	OpinionsKnown      bool
	ParticipantsKnown  bool
	UnknownReasons     []UnknownReason
}

func (f Facts) ApprovalCount() (int, bool) {
	return len(f.Approvers), f.OpinionsKnown
}

func (f Facts) Decision() (Decision, bool) {
	if !f.OpinionsKnown {
		return "", false
	}
	if len(f.ChangesRequestedBy) != 0 {
		return DecisionChangesRequested, true
	}
	if len(f.Approvers) != 0 {
		return DecisionApproved, true
	}
	return DecisionNone, true
}
