package reviews_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emms-garcia/shipq/internal/domain/identity"
	"github.com/emms-garcia/shipq/internal/domain/reviews"
)

type expectation struct {
	approvers           []string
	requestingChanges   []string
	participants        []string
	decision            reviews.Decision
	unknownOpinions     bool
	unknownParticipants bool
	reasons             []reviews.UnknownReason
}

func TestReduce(t *testing.T) {
	approve := review(t, "a", "alice", reviews.Approved, 0)
	laterApproval := review(t, "b", "alice", reviews.Approved, 1)
	change := review(t, "c", "alice", reviews.ChangesRequested, 1)
	comment := review(t, "d", "alice", reviews.Commented, 2)
	dismissed := review(t, "e", "alice", reviews.Dismissed, 1)
	pending := review(t, "f", "alice", reviews.Pending, 3)
	bob := review(t, "g", "bob", reviews.Approved, 0)
	self := review(t, "h", "self", reviews.Approved, 0)
	oldDismissal := review(t, "i", "alice", reviews.Dismissed, -1)
	unknown := expectation{unknownOpinions: true, unknownParticipants: true}

	for _, test := range []struct {
		name   string
		input  []reviews.Review
		change func(*reviews.Snapshot)
		want   expectation
	}{
		{name: "complete empty", want: expectation{decision: reviews.DecisionNone}},
		{name: "one approval", input: []reviews.Review{approve}, want: expectation{approvers: users("alice"), participants: users("alice"), decision: reviews.DecisionApproved}},
		{name: "two approvals", input: []reviews.Review{approve, bob}, want: expectation{approvers: users("alice", "bob"), participants: users("alice", "bob"), decision: reviews.DecisionApproved}},
		{name: "three approvals including self", input: []reviews.Review{approve, bob, self}, want: expectation{approvers: users("alice", "bob", "self"), participants: users("alice", "bob", "self"), decision: reviews.DecisionApproved}},
		{name: "repeated approvals count once", input: []reviews.Review{approve, laterApproval, bob}, want: expectation{approvers: users("alice", "bob"), participants: users("alice", "bob"), decision: reviews.DecisionApproved}},
		{name: "comment does not revoke approval", input: []reviews.Review{approve, comment}, want: expectation{approvers: users("alice"), participants: users("alice"), decision: reviews.DecisionApproved}},
		{name: "pending does not revoke approval", input: []reviews.Review{approve, pending}, want: expectation{approvers: users("alice"), participants: users("alice"), decision: reviews.DecisionApproved}},
		{name: "pending is not a participant", input: []reviews.Review{pending}, want: expectation{decision: reviews.DecisionNone}},
		{name: "comment is a participant not an approver", input: []reviews.Review{comment}, want: expectation{participants: users("alice"), decision: reviews.DecisionNone}},
		{name: "changes requested supersedes approval", input: []reviews.Review{approve, change, comment}, want: expectation{requestingChanges: users("alice"), participants: users("alice"), decision: reviews.DecisionChangesRequested}},
		{name: "approval supersedes changes requested", input: []reviews.Review{change, review(t, "j", "alice", reviews.Approved, 2)}, want: expectation{approvers: users("alice"), participants: users("alice"), decision: reviews.DecisionApproved}},
		{name: "latest dismissal never resurrects approval", input: []reviews.Review{approve, dismissed}, want: expectation{participants: users("alice"), decision: reviews.DecisionNone}},
		{name: "older dismissal does not clear newer approval", input: []reviews.Review{laterApproval, oldDismissal}, want: expectation{approvers: users("alice"), participants: users("alice"), decision: reviews.DecisionApproved}},
		{name: "dismissal clears changes requested", input: []reviews.Review{change, review(t, "k", "alice", reviews.Dismissed, 2)}, want: expectation{participants: users("alice"), decision: reviews.DecisionNone}},
		{name: "changes requested takes precedence", input: []reviews.Review{bob, change}, want: expectation{approvers: users("bob"), requestingChanges: users("alice"), participants: users("alice", "bob"), decision: reviews.DecisionChangesRequested}},
		{name: "out of order history", input: []reviews.Review{comment, change, approve}, want: expectation{requestingChanges: users("alice"), participants: users("alice"), decision: reviews.DecisionChangesRequested}},
		{name: "same time same opinion", input: []reviews.Review{approve, review(t, "l", "alice", reviews.Approved, 0)}, want: expectation{approvers: users("alice"), participants: users("alice"), decision: reviews.DecisionApproved}},
		{name: "same time conflicting opinions", input: []reviews.Review{approve, review(t, "m", "alice", reviews.ChangesRequested, 0)}, want: expectation{participants: users("alice"), unknownOpinions: true, reasons: []reviews.UnknownReason{reviews.AmbiguousOrder}}},
		{name: "newer opinion resolves older tie", input: []reviews.Review{approve, review(t, "n", "alice", reviews.ChangesRequested, 0), laterApproval}, want: expectation{approvers: users("alice"), participants: users("alice"), decision: reviews.DecisionApproved}},
		{name: "older tie cannot poison newer opinion", input: []reviews.Review{laterApproval, approve, review(t, "o", "alice", reviews.ChangesRequested, 0)}, want: expectation{approvers: users("alice"), participants: users("alice"), decision: reviews.DecisionApproved}},
		{name: "duplicate page row", input: []reviews.Review{approve, approve, bob}, want: expectation{approvers: users("alice", "bob"), participants: users("alice", "bob"), decision: reviews.DecisionApproved}},
		{name: "conflicting duplicate row", input: []reviews.Review{approve, approve}, change: func(s *reviews.Snapshot) { s.Reviews[1].State = reviews.Dismissed }, want: withReasons(unknown, reviews.ConflictingReview)},
		{name: "partial pages are not zero", input: []reviews.Review{approve}, change: func(s *reviews.Snapshot) { s.Complete = false }, want: withReasons(unknown, reviews.IncompletePages)},
		{name: "missing initial head", input: []reviews.Review{approve}, change: func(s *reviews.Snapshot) { s.HeadBefore = "" }, want: withReasons(unknown, reviews.UnverifiedHead)},
		{name: "missing final head", input: []reviews.Review{approve}, change: func(s *reviews.Snapshot) { s.HeadAfter = "" }, want: withReasons(unknown, reviews.UnverifiedHead)},
		{name: "head changed during collection", input: []reviews.Review{approve}, change: func(s *reviews.Snapshot) { s.HeadAfter = "new_head" }, want: withReasons(unknown, reviews.ChangedHead)},
		{name: "approval on previous commit remains effective", input: []reviews.Review{approve}, change: func(s *reviews.Snapshot) { s.Reviews[0].Commit = "previous_head" }, want: expectation{approvers: users("alice"), participants: users("alice"), decision: reviews.DecisionApproved}},
		{name: "stale approval explicitly dismissed after new commit", input: []reviews.Review{approve}, change: func(s *reviews.Snapshot) {
			s.Reviews[0].Commit = "previous_head"
			s.Reviews[0].State = reviews.Dismissed
		}, want: expectation{participants: users("alice"), decision: reviews.DecisionNone}},
		{name: "deleted reviewer cannot become zero approvals", input: []reviews.Review{approve}, change: func(s *reviews.Snapshot) { s.Reviews[0].Reviewer = identity.Reference{} }, want: withReasons(unknown, reviews.UnresolvedReviewer)},
		{name: "unresolved comment only obscures participants", input: []reviews.Review{comment, bob}, change: func(s *reviews.Snapshot) { s.Reviews[0].Reviewer = identity.Reference{} }, want: expectation{approvers: users("bob"), decision: reviews.DecisionApproved, unknownParticipants: true, reasons: []reviews.UnknownReason{reviews.UnresolvedReviewer}}},
		{name: "unresolved pending actor irrelevant", input: []reviews.Review{approve, pending}, change: func(s *reviews.Snapshot) { s.Reviews[1].Reviewer = identity.Reference{} }, want: expectation{approvers: users("alice"), participants: users("alice"), decision: reviews.DecisionApproved}},
		{name: "missing order preserves participant", input: []reviews.Review{approve}, change: func(s *reviews.Snapshot) { s.Reviews[0].SubmittedAt = time.Time{} }, want: expectation{participants: users("alice"), unknownOpinions: true, reasons: []reviews.UnknownReason{reviews.MissingOrder}}},
		{name: "missing reviewed commit", input: []reviews.Review{approve}, change: func(s *reviews.Snapshot) { s.Reviews[0].Commit = "" }, want: expectation{participants: users("alice"), unknownOpinions: true, reasons: []reviews.UnknownReason{reviews.InvalidReview}}},
		{name: "missing comment time does not invent an opinion", input: []reviews.Review{comment}, change: func(s *reviews.Snapshot) { s.Reviews[0].SubmittedAt = time.Time{} }, want: expectation{participants: users("alice"), decision: reviews.DecisionNone}},
		{name: "unknown review state", input: []reviews.Review{approve}, change: func(s *reviews.Snapshot) { s.Reviews[0].State = "unsupported" }, want: withReasons(unknown, reviews.InvalidReview)},
		{name: "missing review reference", input: []reviews.Review{approve}, change: func(s *reviews.Snapshot) { s.Reviews[0].Reference = identity.Reference{} }, want: withReasons(unknown, reviews.InvalidReview)},
		{name: "missing PR reference", change: func(s *reviews.Snapshot) { s.PullRequest = identity.Reference{} }, want: withReasons(unknown, reviews.InvalidReview)},
		{name: "mixed PR pages", input: []reviews.Review{approve}, change: func(s *reviews.Snapshot) { s.Reviews[0].PullRequest = ref(t, "github.com", "PR_other") }, want: withReasons(unknown, reviews.InvalidReview)},
		{name: "review from another host", input: []reviews.Review{approve}, change: func(s *reviews.Snapshot) { s.Reviews[0].Reference = ref(t, "other.example", "RV_fixture_a") }, want: withReasons(unknown, reviews.InvalidReview)},
		{name: "reviewer from another host", input: []reviews.Review{approve}, change: func(s *reviews.Snapshot) { s.Reviews[0].Reviewer = ref(t, "other.example", "U_fixture_alice") }, want: withReasons(unknown, reviews.InvalidReview)},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := snapshot(t, slices.Clone(test.input))
			if test.change != nil {
				test.change(&snapshot)
			}
			assertFacts(t, reviews.Reduce(snapshot), test.want)
		})
	}
}

func TestReduceStableAndIndependent(t *testing.T) {
	input := []reviews.Review{
		review(t, "a", "alice", reviews.Approved, 0),
		review(t, "b", "alice", reviews.ChangesRequested, 1),
		review(t, "c", "bob", reviews.Approved, 0),
	}
	original := slices.Clone(input)
	want := reviews.Reduce(snapshot(t, input))
	for range len(input) {
		input = append(input[1:], input[0])
		if got := reviews.Reduce(snapshot(t, input)); !reflect.DeepEqual(got, want) {
			t.Fatalf("input ordering changed effective facts: %+v != %+v", got, want)
		}
	}
	if !reflect.DeepEqual(input, original) {
		t.Fatal("reduction mutated the input")
	}
	want.Approvers[0] = identity.Reference{}
	assertFacts(t, reviews.Reduce(snapshot(t, original)), expectation{
		approvers: users("bob"), requestingChanges: users("alice"), participants: users("alice", "bob"), decision: reviews.DecisionChangesRequested,
	})
}

func TestReduceEqualInstantsAndDuplicateRows(t *testing.T) {
	first := review(t, "a", "alice", reviews.Approved, 0)
	equivalent := first
	equivalent.SubmittedAt = first.SubmittedAt.In(time.FixedZone("fixture", 3600))
	assertFacts(t, reviews.Reduce(snapshot(t, []reviews.Review{first, equivalent})), expectation{
		approvers: users("alice"), participants: users("alice"), decision: reviews.DecisionApproved,
	})
	equivalent.Reference = ref(t, "github.com", "RV_other")
	equivalent.State = reviews.Dismissed
	assertFacts(t, reviews.Reduce(snapshot(t, []reviews.Review{first, equivalent})), expectation{
		participants: users("alice"), unknownOpinions: true, reasons: []reviews.UnknownReason{reviews.AmbiguousOrder},
	})
}

func TestReduceFixtures(t *testing.T) {
	fixture := loadFixtures(t)
	participants := map[string][]string{
		"PR_fixture_101": users("self", "bob"),
		"PR_fixture_102": users("alice", "bob"),
		"PR_fixture_103": users("self", "alice", "bob"),
		"PR_fixture_104": users("alice"),
		"PR_fixture_105": nil,
		"PR_fixture_106": users("self", "bob"),
	}
	for pr, approvers := range fixture.reviews.ExpectedApprovers {
		t.Run(pr, func(t *testing.T) {
			decision := reviews.DecisionNone
			if len(approvers) != 0 {
				decision = reviews.DecisionApproved
			}
			assertFacts(t, reviews.Reduce(fixture.snapshot(t, pr, fixture.initial.ReviewPages)), expectation{
				approvers: approvers, participants: participants[pr], decision: decision,
			})
		})
	}
	t.Run("later review page fails", func(t *testing.T) {
		got := reviews.Reduce(fixture.snapshot(t, "PR_fixture_103", []string{"reviews-103-incomplete-page-1", "reviews-103-incomplete-page-2"}))
		assertFacts(t, got, expectation{unknownOpinions: true, unknownParticipants: true, reasons: []reviews.UnknownReason{reviews.IncompletePages}})
	})
}

func assertFacts(t *testing.T, got reviews.Facts, want expectation) {
	t.Helper()
	count, countKnown := got.ApprovalCount()
	decision, decisionKnown := got.Decision()
	if countKnown != !want.unknownOpinions || decisionKnown != !want.unknownOpinions ||
		got.ParticipantsKnown != !want.unknownParticipants {
		t.Fatalf("unexpected knowledge: %+v; want %+v", got, want)
	}
	if countKnown && count != len(want.approvers) {
		t.Fatalf("approval count = %d, want %d", count, len(want.approvers))
	}
	if decision != want.decision {
		t.Fatalf("decision = %q, want %q", decision, want.decision)
	}
	for _, set := range []struct {
		got  []identity.Reference
		want []string
	}{
		{got.Approvers, want.approvers},
		{got.ChangesRequestedBy, want.requestingChanges},
		{got.ReviewedBy, want.participants},
	} {
		actual := make([]string, len(set.got))
		for i, ref := range set.got {
			actual[i] = ref.NodeID()
		}
		expected := slices.Clone(set.want)
		slices.Sort(expected)
		if !slices.Equal(actual, expected) {
			t.Fatalf("identity set = %v, want sorted %v", actual, expected)
		}
	}
	expectedReasons := slices.Clone(want.reasons)
	slices.Sort(expectedReasons)
	if !slices.Equal(got.UnknownReasons, expectedReasons) {
		t.Fatalf("unknown reasons = %v, want %v", got.UnknownReasons, expectedReasons)
	}
}

func withReasons(want expectation, reasons ...reviews.UnknownReason) expectation {
	want.reasons = reasons
	return want
}

func users(names ...string) []string {
	result := make([]string, len(names))
	for i, name := range names {
		result[i] = "U_fixture_" + name
	}
	return result
}

func ref(t *testing.T, host, node string) identity.Reference {
	t.Helper()
	value, err := identity.NewReference(host, node)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func review(t *testing.T, id, user string, state reviews.State, minute int) reviews.Review {
	t.Helper()
	when := time.Date(2026, 1, 2, 10, minute, 0, 0, time.UTC)
	if state == reviews.Pending {
		when = time.Time{}
	}
	return reviews.Review{
		Reference: ref(t, "github.com", "RV_fixture_"+id), PullRequest: ref(t, "github.com", "PR_fixture"),
		Reviewer: ref(t, "github.com", "U_fixture_"+user), State: state, SubmittedAt: when, Commit: "head_current",
	}
}

func snapshot(t *testing.T, input []reviews.Review) reviews.Snapshot {
	t.Helper()
	return reviews.Snapshot{PullRequest: ref(t, "github.com", "PR_fixture"), HeadBefore: "head_current", HeadAfter: "head_current", Complete: true, Reviews: input}
}

type fixtureReviewPage struct {
	ID       string `json:"id"`
	PR       string `json:"pr_node_id"`
	Response struct {
		Status int             `json:"status"`
		Body   json.RawMessage `json:"body"`
	} `json:"response"`
}

type fixtureReview struct {
	NodeID string `json:"node_id"`
	User   struct {
		NodeID string `json:"node_id"`
	} `json:"user"`
	State       string    `json:"state"`
	SubmittedAt time.Time `json:"submitted_at"`
	Commit      string    `json:"commit_id"`
}

type fixtures struct {
	prs struct {
		Pages []struct {
			Response struct {
				Body []struct {
					NodeID string `json:"node_id"`
					Head   struct {
						SHA string `json:"sha"`
					} `json:"head"`
				} `json:"body"`
			} `json:"response"`
		} `json:"pages"`
	}
	reviews struct {
		Pages             []fixtureReviewPage `json:"pages"`
		Failures          []fixtureReviewPage `json:"failures"`
		ExpectedApprovers map[string][]string `json:"expected_effective_approvers"`
	}
	initial struct {
		ReviewPages []string `json:"review_pages"`
	}
}

func loadFixtures(t *testing.T) fixtures {
	t.Helper()
	read := func(name string, target any) {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("..", "..", "..", "tests", "fixtures", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, target); err != nil {
			t.Fatal(err)
		}
	}
	var result fixtures
	read("github/pr_pages.json", &result.prs)
	read("github/reviews.json", &result.reviews)
	var scenarios struct {
		Initial json.RawMessage `json:"initial"`
	}
	read("scenarios/pr_management.json", &scenarios)
	if err := json.Unmarshal(scenarios.Initial, &result.initial); err != nil {
		t.Fatal(err)
	}
	return result
}

func (f fixtures) snapshot(t *testing.T, pr string, pageIDs []string) reviews.Snapshot {
	t.Helper()
	result := reviews.Snapshot{PullRequest: ref(t, "github.com", pr), Complete: true}
	for _, page := range f.prs.Pages {
		for _, record := range page.Response.Body {
			if record.NodeID == pr {
				result.HeadBefore = record.Head.SHA
				result.HeadAfter = record.Head.SHA
			}
		}
	}
	for _, id := range pageIDs {
		found := false
		for _, page := range append(slices.Clone(f.reviews.Pages), f.reviews.Failures...) {
			if page.ID != id {
				continue
			}
			found = true
			if page.PR != pr {
				continue
			}
			if page.Response.Status != 200 {
				result.Complete = false
				continue
			}
			var rows []fixtureReview
			if err := json.Unmarshal(page.Response.Body, &rows); err != nil {
				t.Fatal(err)
			}
			for _, row := range rows {
				result.Reviews = append(result.Reviews, reviews.Review{
					Reference: ref(t, "github.com", row.NodeID), PullRequest: result.PullRequest,
					Reviewer: ref(t, "github.com", row.User.NodeID), State: reviews.State(strings.ToLower(row.State)),
					SubmittedAt: row.SubmittedAt, Commit: row.Commit,
				})
			}
		}
		if !found {
			t.Fatalf("fixture page %q not found", id)
		}
	}
	return result
}
