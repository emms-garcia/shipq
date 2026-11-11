package segments_test

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emms-garcia/shipq/internal/domain"
	"github.com/emms-garcia/shipq/internal/domain/identity"
	pr "github.com/emms-garcia/shipq/internal/domain/pullrequests"
	"github.com/emms-garcia/shipq/internal/domain/reviews"
	s "github.com/emms-garcia/shipq/internal/domain/segments"
)

func ref(id string) identity.Reference {
	r, err := identity.NewReference("github.com", id)
	if err != nil {
		panic(err)
	}
	return r
}

func leaf(field pr.Field, op s.Operator, value s.Value) s.Node {
	return s.Node{Leaf: &s.Predicate{Field: field, Op: op, Value: value}}
}

func number(op s.Operator, n int64) s.Node {
	return leaf(pr.FieldApprovalCount, op, s.Value{Kind: s.Number, Number: n})
}

func boolean(field pr.Field, value bool) s.Node {
	return leaf(field, s.Equal, s.Value{Kind: s.Boolean, Boolean: value})
}

func ids(field pr.Field, op s.Operator, kind s.IdentityKind, names ...string) s.Node {
	values := make([]s.IdentityValue, len(names))
	for i, name := range names {
		values[i] = s.IdentityValue{Kind: kind, Reference: ref(name), DisplayHint: name}
		if name == "me" {
			values[i].Me = true
			values[i].Reference = identity.Reference{}
		}
	}
	return leaf(field, op, s.Value{Kind: s.Identities, Identities: values})
}

func stringSet(field pr.Field, op s.Operator, values ...string) s.Node {
	return leaf(field, op, s.Value{Kind: s.Strings, Strings: values})
}

func instant(field pr.Field, op s.Operator, value time.Time) s.Node {
	return leaf(field, op, s.Value{Kind: s.Instant, Instant: value})
}

func negate(node s.Node) s.Node { return s.Node{Not: &node} }

func definition(node s.Node) s.Definition {
	return s.Definition{FormatVersion: s.FormatVersion, Condition: node}
}

func compile(t *testing.T, node s.Node) s.Rule {
	t.Helper()
	rule, err := s.Compile(definition(node), 1024)
	if err != nil {
		t.Fatal(err)
	}
	return rule
}

var observedAt = time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

func observation(t *testing.T, missing ...pr.Field) pr.Observation {
	t.Helper()
	fields := map[pr.Field]pr.FieldStatus{}
	for _, field := range []pr.Field{
		pr.FieldOrganization, pr.FieldRepository, pr.FieldAuthor, pr.FieldAssignees,
		pr.FieldRequestedReviewers, pr.FieldReviewedBy, pr.FieldApprovers, pr.FieldApprovalCount,
		pr.FieldReviewState, pr.FieldState, pr.FieldDraft, pr.FieldLabels, pr.FieldTitle,
		pr.FieldCreatedAt, pr.FieldUpdatedAt, pr.FieldLocalWatched, pr.FieldRepositoryWatched,
	} {
		if !slices.Contains(missing, field) {
			fields[field] = pr.FieldStatus{Complete: true}
		}
	}
	envelope, err := pr.NewEnvelope(1, observedAt.Add(-time.Second), observedAt, fields)
	if err != nil {
		t.Fatal(err)
	}
	return pr.Observation{
		Reference: ref("PR"), Repository: ref("REPO"), Organization: ref("ORG"), Number: 1,
		Author: ref("ALICE"), Assignees: []identity.Reference{ref("BOB")},
		RequestedUsers: []identity.Reference{ref("BOB")}, RequestedTeams: []identity.Reference{ref("TEAM")},
		Reviews: reviews.Facts{
			Approvers:     []identity.Reference{ref("ALICE"), ref("BOB")},
			ReviewedBy:    []identity.Reference{ref("ALICE"), ref("BOB"), ref("COMMENTER")},
			OpinionsKnown: true, ParticipantsKnown: true,
		},
		Labels: []pr.Label{{Reference: ref("LABEL"), Repository: ref("REPO"), Name: "bug"}},
		State:  pr.Merged, Draft: false, Title: "Fix Straße and Σ behavior",
		CreatedAt: observedAt.Add(-time.Hour), UpdatedAt: observedAt, Envelope: envelope,
		LocalWatched: true, RepositoryWatched: false,
	}
}

func TestFieldOperatorMatrix(t *testing.T) {
	o := observation(t)
	tests := []struct {
		name string
		node s.Node
		want s.Match
	}{}
	add := func(name string, node s.Node, want s.Match) {
		tests = append(tests, struct {
			name string
			node s.Node
			want s.Match
		}{name, node, want})
	}
	for _, field := range []pr.Field{pr.FieldOrganization, pr.FieldRepository, pr.FieldAuthor} {
		kind, name := s.Organization, "ORG"
		if field == pr.FieldRepository {
			kind, name = s.Repository, "REPO"
		} else if field == pr.FieldAuthor {
			kind, name = s.User, "ALICE"
		}
		add(string(field)+"/in", ids(field, s.In, kind, name), s.Matches)
		add(string(field)+"/not_in", ids(field, s.NotIn, kind, name), s.NoMatch)
		add(string(field)+"/absent", ids(field, s.In, kind, "OTHER"), s.NoMatch)
		add(string(field)+"/excluded", ids(field, s.NotIn, kind, "OTHER"), s.Matches)
	}
	for _, field := range []pr.Field{pr.FieldAssignees, pr.FieldRequestedReviewers, pr.FieldReviewedBy, pr.FieldApprovers} {
		for _, tc := range []struct {
			op    s.Operator
			names []string
			want  s.Match
		}{
			{s.Any, []string{"BOB", "OTHER"}, s.Matches},
			{s.Any, []string{"OTHER"}, s.NoMatch},
			{s.All, []string{"BOB"}, s.Matches},
			{s.All, []string{"BOB", "OTHER"}, s.NoMatch},
			{s.None, []string{"BOB"}, s.NoMatch},
			{s.None, []string{"OTHER"}, s.Matches},
		} {
			add(fmt.Sprintf("%s/%s/%v", field, tc.op, tc.names), ids(field, tc.op, s.User, tc.names...), tc.want)
		}
	}
	for _, tc := range []struct {
		op   s.Operator
		n    int64
		want s.Match
	}{
		{s.Equal, 2, s.Matches}, {s.Equal, 1, s.NoMatch},
		{s.Greater, 1, s.Matches}, {s.Greater, 2, s.NoMatch},
		{s.AtLeast, 2, s.Matches}, {s.AtLeast, 3, s.NoMatch},
		{s.Less, 3, s.Matches}, {s.Less, 2, s.NoMatch},
		{s.AtMost, 2, s.Matches}, {s.AtMost, 1, s.NoMatch},
	} {
		add(fmt.Sprintf("count/%s/%d", tc.op, tc.n), number(tc.op, tc.n), tc.want)
	}
	for _, field := range []pr.Field{pr.FieldState, pr.FieldReviewState} {
		present := "merged"
		if field == pr.FieldReviewState {
			present = "approved"
		}
		add(string(field)+"/in", stringSet(field, s.In, present), s.Matches)
		add(string(field)+"/not_in", stringSet(field, s.NotIn, present), s.NoMatch)
		absent := "open"
		if field == pr.FieldReviewState {
			absent = "none"
		}
		add(string(field)+"/absent", stringSet(field, s.In, absent), s.NoMatch)
		add(string(field)+"/excluded", stringSet(field, s.NotIn, absent), s.Matches)
	}
	for _, field := range []pr.Field{pr.FieldDraft, pr.FieldLocalWatched, pr.FieldRepositoryWatched} {
		value := field == pr.FieldLocalWatched
		add(string(field)+"/equal", boolean(field, value), s.Matches)
		add(string(field)+"/different", boolean(field, !value), s.NoMatch)
	}
	for _, op := range []s.Operator{s.Any, s.All, s.None} {
		for _, present := range []bool{true, false} {
			label := s.LabelValue{Repository: ref("REPO"), Reference: ref("LABEL"), DisplayHint: "bug"}
			if !present {
				label.Repository = ref("OTHER_REPO")
			}
			want := s.NoMatch
			if present != (op == s.None) {
				want = s.Matches
			}
			add(fmt.Sprintf("labels/%s/%t", op, present), leaf(pr.FieldLabels, op, s.Value{Kind: s.Labels, Labels: []s.LabelValue{label}}), want)
		}
	}
	for _, tc := range []struct {
		text string
		want s.Match
	}{
		{"STRASSE", s.Matches}, {"σ", s.Matches}, {"ς", s.Matches},
		{"^Fix", s.NoMatch}, {"unrelated", s.NoMatch},
	} {
		add("title/"+tc.text, leaf(pr.FieldTitle, s.Contains, s.Value{Kind: s.Text, Text: tc.text}), tc.want)
	}
	for _, field := range []pr.Field{pr.FieldCreatedAt, pr.FieldUpdatedAt} {
		at := o.CreatedAt
		if field == pr.FieldUpdatedAt {
			at = o.UpdatedAt
		}
		add(string(field)+"/start_inclusive", instant(field, s.AtLeast, at), s.Matches)
		add(string(field)+"/before_start", instant(field, s.AtLeast, at.Add(time.Nanosecond)), s.NoMatch)
		add(string(field)+"/end_exclusive", instant(field, s.Less, at), s.NoMatch)
		add(string(field)+"/before_end", instant(field, s.Less, at.Add(time.Nanosecond)), s.Matches)
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := compile(t, tc.node).Evaluate(o, s.Context{Fresh: true})
			if got.Result != tc.want || !got.NotificationEligible || len(got.Evidence) != 1 {
				t.Fatalf("got %+v, want %s and trustworthy evidence", got, tc.want)
			}
			if got.Evidence[0].Path != "condition" || !got.Evidence[0].ObservedAt.Equal(observedAt) || !got.Evidence[0].Fresh {
				t.Fatalf("missing field provenance: %+v", got.Evidence)
			}
		})
	}
}

func TestSelfApprovalsAndAuthorshipAreIndependent(t *testing.T) {
	o := observation(t)
	for _, tc := range []struct {
		name string
		node s.Node
		want s.Match
	}{
		{"including_my_pr", s.Node{All: []s.Node{number(s.AtLeast, 2), ids(pr.FieldAuthor, s.In, s.User, "me")}}, s.Matches},
		{"excluding_my_pr", s.Node{All: []s.Node{number(s.AtLeast, 2), ids(pr.FieldAuthor, s.NotIn, s.User, "me")}}, s.NoMatch},
		{"including_my_approval", s.Node{All: []s.Node{number(s.Equal, 2), ids(pr.FieldApprovers, s.All, s.User, "me", "BOB")}}, s.Matches},
		{"commenter_is_participant", ids(pr.FieldReviewedBy, s.Any, s.User, "COMMENTER"), s.Matches},
		{"commenter_not_approver", ids(pr.FieldApprovers, s.Any, s.User, "COMMENTER"), s.NoMatch},
		{"requested_team", ids(pr.FieldRequestedReviewers, s.Any, s.Team, "TEAM"), s.Matches},
		{"team_is_not_user", ids(pr.FieldRequestedReviewers, s.Any, s.User, "TEAM"), s.NoMatch},
		{"no_implicit_open_state", number(s.Equal, 2), s.Matches},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := compile(t, tc.node).Evaluate(o, s.Context{CurrentUser: ref("ALICE")}); got.Result != tc.want {
				t.Fatalf("got %+v, want %s", got, tc.want)
			}
		})
	}
	o.Reviews.Approvers = append(o.Reviews.Approvers, ref("THIRD"))
	if compile(t, number(s.Equal, 2)).Evaluate(o, s.Context{}).Result != s.NoMatch ||
		compile(t, number(s.AtLeast, 2)).Evaluate(o, s.Context{}).Result != s.Matches {
		t.Fatal("exact and minimum approvals are not interchangeable")
	}
}

func TestThreeValuedLogic(t *testing.T) {
	o := observation(t, pr.FieldTitle)
	nodes := map[s.Match]s.Node{
		s.Matches: boolean(pr.FieldDraft, false), s.NoMatch: boolean(pr.FieldDraft, true),
		s.Unknown: leaf(pr.FieldTitle, s.Contains, s.Value{Kind: s.Text, Text: "missing"}),
	}
	for left, a := range nodes {
		wantNot := s.Unknown
		if left == s.Matches {
			wantNot = s.NoMatch
		} else if left == s.NoMatch {
			wantNot = s.Matches
		}
		if got := compile(t, negate(a)).Evaluate(o, s.Context{}).Result; got != wantNot {
			t.Fatalf("not %s = %s", left, got)
		}
		for right, b := range nodes {
			for _, all := range []bool{true, false} {
				node, want := s.Node{Any: []s.Node{a, b}}, s.Unknown
				if all {
					node = s.Node{All: []s.Node{a, b}}
					if left == s.NoMatch || right == s.NoMatch {
						want = s.NoMatch
					} else if left == s.Matches && right == s.Matches {
						want = s.Matches
					}
				} else if left == s.Matches || right == s.Matches {
					want = s.Matches
				} else if left == s.NoMatch && right == s.NoMatch {
					want = s.NoMatch
				}
				got := compile(t, node).Evaluate(o, s.Context{Fresh: true})
				if got.Result != want || len(got.Evidence) != 2 {
					t.Fatalf("all=%t (%s,%s): %+v, want %s", all, left, right, got, want)
				}
				if got.NotificationEligible != (left != s.Unknown && right != s.Unknown) {
					t.Fatal("short-circuit truth must not hide missing notification dependencies")
				}
			}
		}
	}
	group := negate(s.Node{Any: []s.Node{nodes[s.NoMatch], negate(nodes[s.Matches])}})
	if compile(t, group).Evaluate(o, s.Context{}).Result != s.Matches {
		t.Fatal("nested grouping or negation lost")
	}
}

func TestUnknownEvidenceAndFreshness(t *testing.T) {
	tests := []struct {
		name   string
		node   s.Node
		change func(*pr.Observation, *s.Context)
	}{
		{"missing_field", number(s.Equal, 0), func(o *pr.Observation, _ *s.Context) { *o = observation(t, pr.FieldApprovalCount) }},
		{"unknown_opinions", number(s.Equal, 0), func(o *pr.Observation, _ *s.Context) { o.Reviews.OpinionsKnown = false }},
		{"unknown_approvers", ids(pr.FieldApprovers, s.None, s.User, "ALICE"), func(o *pr.Observation, _ *s.Context) { o.Reviews.OpinionsKnown = false }},
		{"unknown_decision", stringSet(pr.FieldReviewState, s.In, "none"), func(o *pr.Observation, _ *s.Context) { o.Reviews.OpinionsKnown = false }},
		{"unknown_participants", ids(pr.FieldReviewedBy, s.None, s.User, "ALICE"), func(o *pr.Observation, _ *s.Context) { o.Reviews.ParticipantsKnown = false }},
		{"unresolved_me", ids(pr.FieldAuthor, s.NotIn, s.User, "me"), func(_ *pr.Observation, c *s.Context) { c.CurrentUser = identity.Reference{} }},
		{"wrong_host_me", ids(pr.FieldAuthor, s.In, s.User, "me"), func(_ *pr.Observation, c *s.Context) {
			c.CurrentUser, _ = identity.NewReference("ghe.example", "ALICE")
		}},
		{"deleted_selection", ids(pr.FieldAuthor, s.NotIn, s.User, "DELETED"), func(_ *pr.Observation, c *s.Context) {
			c.UnresolvedIdentities = map[identity.Reference]bool{ref("DELETED"): true}
		}},
		{"missing_author", ids(pr.FieldAuthor, s.NotIn, s.User, "ALICE"), func(o *pr.Observation, _ *s.Context) { o.Author = identity.Reference{} }},
		{"missing_organization", ids(pr.FieldOrganization, s.NotIn, s.Organization, "ORG"), func(o *pr.Observation, _ *s.Context) { o.Organization = identity.Reference{} }},
		{"missing_repository", ids(pr.FieldRepository, s.NotIn, s.Repository, "REPO"), func(o *pr.Observation, _ *s.Context) { o.Repository = identity.Reference{} }},
		{"invalid_assignee", ids(pr.FieldAssignees, s.None, s.User, "ALICE"), func(o *pr.Observation, _ *s.Context) { o.Assignees = []identity.Reference{{}} }},
		{"invalid_team", ids(pr.FieldRequestedReviewers, s.None, s.Team, "TEAM"), func(o *pr.Observation, _ *s.Context) { o.RequestedTeams = []identity.Reference{{}} }},
		{"missing_date", instant(pr.FieldCreatedAt, s.Less, observedAt), func(o *pr.Observation, _ *s.Context) { o.CreatedAt = time.Time{} }},
		{"invalid_state", stringSet(pr.FieldState, s.NotIn, "open"), func(o *pr.Observation, _ *s.Context) { o.State = "bad" }},
		{"invalid_title", leaf(pr.FieldTitle, s.Contains, s.Value{Kind: s.Text, Text: "Fix"}), func(o *pr.Observation, _ *s.Context) { o.Title = "\xff" }},
		{"invalid_label", leaf(pr.FieldLabels, s.None, s.Value{Kind: s.Labels, Labels: []s.LabelValue{{Repository: ref("REPO"), Reference: ref("LABEL")}}}), func(o *pr.Observation, _ *s.Context) { o.Labels[0].Reference = identity.Reference{} }},
		{"missing_envelope", boolean(pr.FieldDraft, false), func(o *pr.Observation, _ *s.Context) { o.Envelope = pr.Envelope{} }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o, c := observation(t), s.Context{CurrentUser: ref("ALICE"), Fresh: true}
			tc.change(&o, &c)
			got := compile(t, tc.node).Evaluate(o, c)
			if got.Result != s.Unknown || got.NotificationEligible || len(got.Evidence) != 1 || got.Evidence[0].Reason == "" {
				t.Fatalf("unknown evidence erased: %+v", got)
			}
		})
	}
	o := observation(t)
	o.Reviews.ParticipantsKnown = false
	rule := compile(t, number(s.Equal, 2))
	got := rule.Evaluate(o, s.Context{})
	if got.Result != s.Matches || got.NotificationEligible || got.Evidence[0].Fresh {
		t.Fatalf("stale data should remain displayable, not notification-eligible: %+v", got)
	}
	if !rule.Evaluate(o, s.Context{Fresh: true}).NotificationEligible {
		t.Fatal("unreferenced participant incompleteness must not obscure approval facts")
	}
	o.Reviews.OpinionsKnown = false
	o.Reviews.ParticipantsKnown = true
	if compile(t, ids(pr.FieldReviewedBy, s.Any, s.User, "COMMENTER")).Evaluate(o, s.Context{}).Result != s.Matches {
		t.Fatal("known participants must survive unknown opinions")
	}
	if got := (s.Rule{}).Evaluate(o, s.Context{Fresh: true}); got.Result != s.Unknown || got.NotificationEligible {
		t.Fatal("zero rule must not match")
	}
}

func TestKnownEmptySetsAndReviewDecisions(t *testing.T) {
	o := observation(t)
	o.Assignees, o.RequestedUsers, o.RequestedTeams, o.Labels = nil, nil, nil, nil
	o.Reviews.Approvers, o.Reviews.ReviewedBy = nil, nil
	for _, node := range []s.Node{
		number(s.Equal, 0),
		stringSet(pr.FieldReviewState, s.In, "none"),
		ids(pr.FieldAssignees, s.None, s.User, "ALICE"),
		ids(pr.FieldRequestedReviewers, s.None, s.Team, "TEAM"),
		ids(pr.FieldReviewedBy, s.None, s.User, "ALICE"),
		ids(pr.FieldApprovers, s.None, s.User, "ALICE"),
		leaf(pr.FieldLabels, s.None, s.Value{Kind: s.Labels, Labels: []s.LabelValue{{Repository: ref("REPO"), Reference: ref("LABEL")}}}),
	} {
		if got := compile(t, node).Evaluate(o, s.Context{Fresh: true}); got.Result != s.Matches || !got.NotificationEligible {
			t.Fatalf("known empty is distinct from missing: %+v", got)
		}
	}
	o.Reviews.Approvers = []identity.Reference{ref("ALICE")}
	o.Reviews.ChangesRequestedBy = []identity.Reference{ref("BOB")}
	if compile(t, stringSet(pr.FieldReviewState, s.In, "changes_requested")).Evaluate(o, s.Context{}).Result != s.Matches {
		t.Fatal("changes requested takes precedence over approvals")
	}
	rule := compile(t, ids(pr.FieldAuthor, s.In, s.User, "me"))
	if rule.Evaluate(o, s.Context{CurrentUser: ref("ALICE")}).Result != s.Matches ||
		rule.Evaluate(o, s.Context{CurrentUser: ref("BOB")}).Result != s.NoMatch {
		t.Fatal("me must resolve anew against the explicitly connected account")
	}
	o.Envelope, _ = pr.NewEnvelope(2, observedAt, observedAt, map[pr.Field]pr.FieldStatus{
		pr.FieldApprovalCount: {Reason: "incomplete_pages"},
	})
	got := compile(t, number(s.Equal, 1)).Evaluate(o, s.Context{Fresh: true})
	if got.Result != s.Unknown || got.Evidence[0].Reason != "incomplete_pages" || got.NotificationEligible {
		t.Fatalf("field completeness must override a populated cached value: %+v", got)
	}
}

func TestInvalidDefinitions(t *testing.T) {
	label := s.LabelValue{Reference: ref("LABEL"), Repository: ref("REPO")}
	otherHost, _ := identity.NewReference("ghe.example", "LABEL")
	tests := []struct {
		name string
		node s.Node
	}{
		{"no_shape", s.Node{}},
		{"empty_all", s.Node{All: []s.Node{}}},
		{"empty_any", s.Node{Any: []s.Node{}}},
		{"two_shapes", s.Node{All: []s.Node{number(s.Equal, 2)}, Leaf: number(s.Equal, 2).Leaf}},
		{"unknown_field", leaf("unknown", s.Equal, s.Value{Kind: s.Number})},
		{"unknown_operator", number("between", 2)},
		{"wrong_type", leaf(pr.FieldApprovalCount, s.Equal, s.Value{Kind: s.Text, Text: "2"})},
		{"missing_type", leaf(pr.FieldApprovalCount, s.Equal, s.Value{})},
		{"mixed_value", leaf(pr.FieldApprovalCount, s.Equal, s.Value{Kind: s.Number, Number: 2, Text: "ignored"})},
		{"negative_count", number(s.Equal, -1)},
		{"empty_ids", ids(pr.FieldAuthor, s.In, s.User)},
		{"unknown_identity_kind", ids(pr.FieldAuthor, s.In, "alien", "ALICE")},
		{"team_approver", ids(pr.FieldApprovers, s.Any, s.Team, "TEAM")},
		{"team_participant", ids(pr.FieldReviewedBy, s.Any, s.Team, "TEAM")},
		{"team_assignee", ids(pr.FieldAssignees, s.Any, s.Team, "TEAM")},
		{"org_not_repository", ids(pr.FieldOrganization, s.In, s.Repository, "ORG")},
		{"me_team", ids(pr.FieldRequestedReviewers, s.Any, s.Team, "me")},
		{"me_plus_id", leaf(pr.FieldAuthor, s.In, s.Value{Kind: s.Identities, Identities: []s.IdentityValue{{Kind: s.User, Me: true, Reference: ref("ALICE")}}})},
		{"zero_identity", leaf(pr.FieldAuthor, s.In, s.Value{Kind: s.Identities, Identities: []s.IdentityValue{{Kind: s.User}}})},
		{"invalid_state", stringSet(pr.FieldState, s.In, "approved")},
		{"invalid_review_state", stringSet(pr.FieldReviewState, s.In, "open")},
		{"empty_states", stringSet(pr.FieldState, s.In)},
		{"empty_title", leaf(pr.FieldTitle, s.Contains, s.Value{Kind: s.Text})},
		{"invalid_unicode", leaf(pr.FieldTitle, s.Contains, s.Value{Kind: s.Text, Text: "\xff"})},
		{"missing_date", instant(pr.FieldCreatedAt, s.AtLeast, time.Time{})},
		{"empty_labels", leaf(pr.FieldLabels, s.Any, s.Value{Kind: s.Labels})},
		{"unscoped_label", leaf(pr.FieldLabels, s.Any, s.Value{Kind: s.Labels, Labels: []s.LabelValue{{Reference: ref("LABEL")}}})},
		{"cross_host_label", leaf(pr.FieldLabels, s.Any, s.Value{Kind: s.Labels, Labels: []s.LabelValue{{Reference: otherHost, Repository: label.Repository}}})},
		{"conflicting_bounds", s.Node{All: []s.Node{number(s.AtLeast, 3), number(s.AtMost, 2)}}},
		{"fractional_gap", s.Node{All: []s.Node{number(s.Greater, 2), number(s.Less, 3)}}},
		{"different_equalities", s.Node{All: []s.Node{number(s.Equal, 2), number(s.Equal, 3)}}},
		{"nested_bounds", s.Node{All: []s.Node{number(s.AtMost, 1), {All: []s.Node{number(s.AtLeast, 2)}}}}},
		{"negated_lower_bound", s.Node{All: []s.Node{number(s.AtLeast, 2), negate(number(s.AtLeast, 2))}}},
		{"negated_upper_bound", s.Node{All: []s.Node{number(s.AtMost, 2), negate(number(s.AtMost, 2))}}},
		{"negated_strict_lower", s.Node{All: []s.Node{number(s.Greater, 2), negate(number(s.Greater, 2))}}},
		{"negated_strict_upper", s.Node{All: []s.Node{number(s.Less, 2), negate(number(s.Less, 2))}}},
		{"excluded_equality", s.Node{All: []s.Node{number(s.Equal, 2), negate(number(s.Equal, 2))}}},
		{"excluded_interval", s.Node{All: []s.Node{number(s.AtLeast, 2), number(s.AtMost, 3), negate(number(s.Equal, 2)), negate(number(s.Equal, 3))}}},
		{"impossible_upper", number(s.Less, 0)},
		{"overflow_lower", number(s.Greater, math.MaxInt64)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.Compile(definition(s.Node{Not: &tc.node}), 1024)
			var validation *domain.ValidationError
			if !errors.As(err, &validation) || !strings.HasPrefix(validation.Field, "condition.not") || validation.Message == "" {
				t.Fatalf("want typed node-path error, got %v", err)
			}
		})
	}
	for _, version := range []int{0, 2, -1} {
		_, err := s.Compile(s.Definition{FormatVersion: version, Condition: number(s.Equal, 2)}, 1024)
		var validation *domain.ValidationError
		if !errors.As(err, &validation) || validation.Field != "format_version" {
			t.Fatalf("version %d: %v", version, err)
		}
	}
	_, err := s.Compile(definition(s.Node{All: []s.Node{number(s.AtLeast, 1), {Not: &s.Node{}}}}), 1024)
	var validation *domain.ValidationError
	if !errors.As(err, &validation) || validation.Field != "condition.all[1].not" {
		t.Fatalf("want precise nested error path: %v", err)
	}
	for _, node := range []s.Node{
		{Any: []s.Node{number(s.Less, 2), number(s.Greater, 3)}},
		{All: []s.Node{number(s.AtLeast, 2), number(s.AtMost, 2)}},
		{All: []s.Node{number(s.AtLeast, 2), negate(number(s.Equal, 3))}},
		{All: []s.Node{number(s.AtLeast, 2), {Any: []s.Node{number(s.Less, 2), number(s.Greater, 3)}}}},
		number(s.AtMost, math.MaxInt64),
	} {
		compile(t, node)
	}
}

func TestAllUnsupportedFieldOperatorPairs(t *testing.T) {
	fields := []struct {
		node      s.Node
		supported []s.Operator
	}{
		{ids(pr.FieldOrganization, s.In, s.Organization, "ORG"), []s.Operator{s.In, s.NotIn}},
		{ids(pr.FieldRepository, s.In, s.Repository, "REPO"), []s.Operator{s.In, s.NotIn}},
		{ids(pr.FieldAuthor, s.In, s.User, "ALICE"), []s.Operator{s.In, s.NotIn}},
		{ids(pr.FieldAssignees, s.Any, s.User, "ALICE"), []s.Operator{s.Any, s.All, s.None}},
		{ids(pr.FieldRequestedReviewers, s.Any, s.User, "ALICE"), []s.Operator{s.Any, s.All, s.None}},
		{ids(pr.FieldReviewedBy, s.Any, s.User, "ALICE"), []s.Operator{s.Any, s.All, s.None}},
		{ids(pr.FieldApprovers, s.Any, s.User, "ALICE"), []s.Operator{s.Any, s.All, s.None}},
		{number(s.Equal, 1), []s.Operator{s.Equal, s.Greater, s.AtLeast, s.Less, s.AtMost}},
		{stringSet(pr.FieldState, s.In, "open"), []s.Operator{s.In, s.NotIn}},
		{stringSet(pr.FieldReviewState, s.In, "none"), []s.Operator{s.In, s.NotIn}},
		{boolean(pr.FieldDraft, true), []s.Operator{s.Equal}},
		{boolean(pr.FieldLocalWatched, true), []s.Operator{s.Equal}},
		{boolean(pr.FieldRepositoryWatched, true), []s.Operator{s.Equal}},
		{leaf(pr.FieldLabels, s.Any, s.Value{Kind: s.Labels, Labels: []s.LabelValue{{Reference: ref("L"), Repository: ref("R")}}}), []s.Operator{s.Any, s.All, s.None}},
		{leaf(pr.FieldTitle, s.Contains, s.Value{Kind: s.Text, Text: "x"}), []s.Operator{s.Contains}},
		{instant(pr.FieldCreatedAt, s.Less, observedAt), []s.Operator{s.AtLeast, s.Less}},
		{instant(pr.FieldUpdatedAt, s.Less, observedAt), []s.Operator{s.AtLeast, s.Less}},
	}
	operators := []s.Operator{s.In, s.NotIn, s.Any, s.All, s.None, s.Equal, s.Greater, s.AtLeast, s.Less, s.AtMost, s.Contains}
	for _, field := range fields {
		for _, op := range operators {
			predicate := *field.node.Leaf
			predicate.Op = op
			_, err := s.Compile(definition(s.Node{Leaf: &predicate}), 1024)
			if (err == nil) != slices.Contains(field.supported, op) {
				t.Errorf("%s/%s: %v", predicate.Field, op, err)
			}
		}
	}
}

func TestRuleLimits(t *testing.T) {
	for _, n := range []int{s.MaxDepth, s.MaxDepth + 1} {
		node := number(s.Equal, 2)
		for i := 1; i < n; i++ {
			node = negate(node)
		}
		_, err := s.Compile(definition(node), 1024)
		if (err == nil) != (n == s.MaxDepth) {
			t.Fatalf("depth %d: %v", n, err)
		}
	}
	for _, n := range []int{s.MaxNodes, s.MaxNodes + 1} {
		nodes := make([]s.Node, n-1)
		for i := range nodes {
			nodes[i] = number(s.Equal, 2)
		}
		_, err := s.Compile(definition(s.Node{All: nodes}), 1024)
		if (err == nil) != (n == s.MaxNodes) {
			t.Fatalf("nodes %d: %v", n, err)
		}
	}
	for _, n := range []int{s.MaxSetValues, s.MaxSetValues + 1} {
		names := make([]string, n)
		states := make([]string, n)
		labels := make([]s.LabelValue, n)
		for i := range names {
			names[i], states[i] = fmt.Sprintf("U%d", i), "open"
			labels[i] = s.LabelValue{Reference: ref(fmt.Sprintf("L%d", i)), Repository: ref("R")}
		}
		for _, node := range []s.Node{
			ids(pr.FieldApprovers, s.Any, s.User, names...),
			stringSet(pr.FieldState, s.In, states...),
			leaf(pr.FieldLabels, s.Any, s.Value{Kind: s.Labels, Labels: labels}),
		} {
			_, err := s.Compile(definition(node), 1024)
			if (err == nil) != (n == s.MaxSetValues) {
				t.Fatalf("set %s/%d: %v", node.Leaf.Field, n, err)
			}
		}
	}
	for _, n := range []int{-1, 0, s.MaxEncodedBytes, s.MaxEncodedBytes + 1} {
		_, err := s.Compile(definition(number(s.Equal, 2)), n)
		var validation *domain.ValidationError
		if n == s.MaxEncodedBytes {
			if err != nil {
				t.Fatal(err)
			}
		} else if !errors.As(err, &validation) || validation.Field != "encoded_bytes" {
			t.Fatalf("bytes %d: %v", n, err)
		}
	}
}

func TestSummaryAndImmutableCompilation(t *testing.T) {
	n := s.Node{All: []s.Node{
		ids(pr.FieldAuthor, s.In, s.User, "me"),
		{Any: []s.Node{number(s.AtLeast, 2), negate(boolean(pr.FieldDraft, true))}},
		leaf(pr.FieldTitle, s.Contains, s.Value{Kind: s.Text, Text: "x\"\n OR anything"}),
	}}
	rule := compile(t, n)
	summary := rule.Summary()
	for _, text := range []string{"all of (", "any of (", "not (", "connected account", "approval count", "at least 2", `x\"\n OR anything`} {
		if !strings.Contains(summary, text) {
			t.Fatalf("missing %q: %s", text, summary)
		}
	}
	n.All[0].Leaf.Value.Identities[0].DisplayHint = "MUTATED"
	n.All[1].Any[0].Leaf.Value.Number = 999
	copy := rule.Definition()
	copy.Condition.All[0].Leaf.Value.Identities[0].Kind = s.Team
	copy.Condition.All[1].Any[0].Leaf.Value.Number = 999
	copy.Condition.All[1].Any[1].Not.Leaf.Value.Boolean = false
	copy.Condition.All[2].Leaf.Value.Text = "MUTATED"
	if rule.Summary() != summary || rule.Definition().Condition.All[1].Any[0].Leaf.Value.Number != 2 {
		t.Fatal("compiled tree or summary aliases caller memory")
	}
	fields := rule.RequiredFields()
	want := []pr.Field{pr.FieldApprovalCount, pr.FieldAuthor, pr.FieldDraft, pr.FieldTitle}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("fields: %v", fields)
	}
	fields[0] = pr.FieldState
	if !reflect.DeepEqual(rule.RequiredFields(), want) {
		t.Fatal("field dependencies alias caller memory")
	}
	for _, node := range []s.Node{
		stringSet(pr.FieldState, s.In, "merged"),
		leaf(pr.FieldLabels, s.All, s.Value{Kind: s.Labels, Labels: []s.LabelValue{{Reference: ref("LABEL"), Repository: ref("REPO"), DisplayHint: "bug"}}}),
		ids(pr.FieldRequestedReviewers, s.Any, s.Team, "TEAM"),
		instant(pr.FieldCreatedAt, s.AtLeast, observedAt.In(time.FixedZone("offset", 3600))),
	} {
		compiled := compile(t, node)
		if compiled.Summary() == "" {
			t.Fatal("missing summary")
		}
		copied := compiled.Definition()
		if copied.Condition.Leaf.Value.Kind == s.Strings {
			copied.Condition.Leaf.Value.Strings[0] = "open"
			if compiled.Definition().Condition.Leaf.Value.Strings[0] != "merged" {
				t.Fatal("aliased states")
			}
		}
		if copied.Condition.Leaf.Value.Kind == s.Labels {
			copied.Condition.Leaf.Value.Labels[0].Reference = ref("OTHER")
			if compiled.Definition().Condition.Leaf.Value.Labels[0].Reference != ref("LABEL") {
				t.Fatal("aliased labels")
			}
			if !strings.Contains(compiled.Summary(), "github.com/REPO") || !strings.Contains(compiled.Summary(), "github.com/LABEL") {
				t.Fatal("label summary lost scope")
			}
		}
		if copied.Condition.Leaf.Value.Kind == s.Instant && copied.Condition.Leaf.Value.Instant.Location() != time.UTC {
			t.Fatal("date not normalized to UTC")
		}
	}
}
