package reviews

import (
	"maps"
	"slices"
	"strings"

	"github.com/emms-garcia/shipq/internal/domain/identity"
)

func Reduce(snapshot Snapshot) Facts {
	if !snapshot.Complete {
		return unknown(IncompletePages)
	}
	if snapshot.PullRequest.IsZero() {
		return unknown(InvalidReview)
	}
	if snapshot.HeadBefore == "" || snapshot.HeadAfter == "" {
		return unknown(UnverifiedHead)
	}
	if snapshot.HeadBefore != snapshot.HeadAfter {
		return unknown(ChangedHead)
	}

	facts := Facts{OpinionsKnown: true, ParticipantsKnown: true}
	reasons := make(map[UnknownReason]struct{})
	seen := make(map[identity.Reference]Review)
	participants := make(map[identity.Reference]struct{})
	type opinion struct {
		review    Review
		ambiguous bool
	}
	latest := make(map[identity.Reference]opinion)
	for _, review := range snapshot.Reviews {
		if review.Reference.IsZero() || review.Reference.Host() != snapshot.PullRequest.Host() ||
			review.PullRequest != snapshot.PullRequest {
			return unknown(InvalidReview)
		}
		switch review.State {
		case Approved, ChangesRequested, Commented, Dismissed, Pending:
		default:
			return unknown(InvalidReview)
		}
		if previous, ok := seen[review.Reference]; ok {
			if !sameReview(previous, review) {
				return unknown(ConflictingReview)
			}
			continue
		}
		seen[review.Reference] = review
		if review.State == Pending {
			continue
		}
		if review.Reviewer.IsZero() {
			facts.ParticipantsKnown = false
			if review.State != Commented {
				facts.OpinionsKnown = false
			}
			reasons[UnresolvedReviewer] = struct{}{}
			continue
		}
		if review.Reviewer.Host() != snapshot.PullRequest.Host() {
			return unknown(InvalidReview)
		}
		participants[review.Reviewer] = struct{}{}
		if review.State == Commented {
			continue
		}
		if review.SubmittedAt.IsZero() {
			facts.OpinionsKnown = false
			reasons[MissingOrder] = struct{}{}
			continue
		}
		if review.Commit == "" {
			facts.OpinionsKnown = false
			reasons[InvalidReview] = struct{}{}
			continue
		}

		// Old-commit approvals can remain effective. Trust the observed state,
		// not commit equality; the snapshot's head bookends guard collection races.
		previous, exists := latest[review.Reviewer]
		if !exists || review.SubmittedAt.After(previous.review.SubmittedAt) {
			latest[review.Reviewer] = opinion{review: review}
		} else if review.SubmittedAt.Equal(previous.review.SubmittedAt) && review.State != previous.review.State {
			previous.ambiguous = true
			latest[review.Reviewer] = previous
		}
	}

	approvers := make(map[identity.Reference]struct{})
	requestingChanges := make(map[identity.Reference]struct{})
	for reviewer, opinion := range latest {
		if opinion.ambiguous {
			facts.OpinionsKnown = false
			reasons[AmbiguousOrder] = struct{}{}
			continue
		}
		switch opinion.review.State {
		case Approved:
			approvers[reviewer] = struct{}{}
		case ChangesRequested:
			requestingChanges[reviewer] = struct{}{}
		}
	}
	if facts.OpinionsKnown {
		facts.Approvers = sortedReferences(approvers)
		facts.ChangesRequestedBy = sortedReferences(requestingChanges)
	}
	if facts.ParticipantsKnown {
		facts.ReviewedBy = sortedReferences(participants)
	}
	facts.UnknownReasons = slices.Sorted(maps.Keys(reasons))
	return facts
}

func unknown(reason UnknownReason) Facts {
	return Facts{UnknownReasons: []UnknownReason{reason}}
}

func sameReview(a, b Review) bool {
	return a.PullRequest == b.PullRequest && a.Reviewer == b.Reviewer &&
		a.State == b.State && a.Commit == b.Commit && a.SubmittedAt.Equal(b.SubmittedAt)
}

func sortedReferences(set map[identity.Reference]struct{}) []identity.Reference {
	result := slices.Collect(maps.Keys(set))
	slices.SortFunc(result, func(a, b identity.Reference) int {
		return strings.Compare(a.NodeID(), b.NodeID())
	})
	return result
}
