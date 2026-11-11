package pullrequests_test

import (
	"errors"
	"testing"
	"time"

	"github.com/emms-garcia/shipq/internal/domain"
	"github.com/emms-garcia/shipq/internal/domain/identity"
	"github.com/emms-garcia/shipq/internal/domain/pullrequests"
)

func TestObservationEnvelope(t *testing.T) {
	start := time.Date(2026, 1, 2, 10, 0, 0, 0, time.FixedZone("fixture", -6*60*60))
	fields := map[pullrequests.Field]pullrequests.FieldStatus{
		pullrequests.FieldTitle:         {Complete: true},
		pullrequests.FieldApprovalCount: {Reason: "incomplete_pages"},
	}
	envelope, err := pullrequests.NewEnvelope(1, start, start.Add(time.Minute), fields)
	if err != nil {
		t.Fatal(err)
	}
	if envelope.StartedAt.Location() != time.UTC || envelope.CompletedAt.Location() != time.UTC ||
		!envelope.StartedAt.Equal(start) || envelope.Revision != 1 {
		t.Fatalf("collection times must retain their instants in UTC: %+v", envelope)
	}
	fields[pullrequests.FieldTitle] = pullrequests.FieldStatus{Reason: "unavailable"}
	if !envelope.HasCompleteFields(pullrequests.FieldTitle) {
		t.Fatal("envelope must not alias the caller's completeness map")
	}
	if envelope.HasCompleteFields(pullrequests.FieldTitle, pullrequests.FieldApprovalCount) ||
		envelope.HasCompleteFields(pullrequests.FieldAuthor) {
		t.Fatal("partial and uncollected fields must not become complete")
	}
	if envelope.Status(pullrequests.FieldAuthor).Reason != pullrequests.NotCollected {
		t.Fatal("missing fields need explicit unknown status")
	}
}

func TestEnvelopeRejectsUntrustworthyMetadata(t *testing.T) {
	now := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name     string
		revision uint64
		start    time.Time
		end      time.Time
		fields   map[pullrequests.Field]pullrequests.FieldStatus
	}{
		{"missing revision", 0, now, now, nil},
		{"missing start", 1, time.Time{}, now, nil},
		{"missing end", 1, now, time.Time{}, nil},
		{"backwards interval", 1, now, now.Add(-time.Second), nil},
		{"contradictory status", 1, now, now, map[pullrequests.Field]pullrequests.FieldStatus{pullrequests.FieldTitle: {Complete: true, Reason: "unavailable"}}},
		{"missing reason", 1, now, now, map[pullrequests.Field]pullrequests.FieldStatus{pullrequests.FieldTitle: {}}},
		{"empty field", 1, now, now, map[pullrequests.Field]pullrequests.FieldStatus{"": {Complete: true}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := pullrequests.NewEnvelope(test.revision, test.start, test.end, test.fields)
			var validation *domain.ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("expected typed validation error, got %v", err)
			}
		})
	}
}

func TestObservation(t *testing.T) {
	start := time.Date(2026, 1, 2, 10, 0, 0, 0, time.FixedZone("fixture", 3600))
	envelope, err := pullrequests.NewEnvelope(1, start, start, nil)
	if err != nil {
		t.Fatal(err)
	}
	ref := func(node string) identity.Reference {
		t.Helper()
		result, err := identity.NewReference("github.com", node)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	value := pullrequests.Observation{
		Reference: ref("PR_fixture"), Repository: ref("R_fixture"), Number: 1,
		CreatedAt: start, UpdatedAt: start.Add(time.Minute), Envelope: envelope,
		Assignees: []identity.Reference{ref("U_fixture")},
	}
	result, err := pullrequests.NewObservation(value)
	if err != nil {
		t.Fatal(err)
	}
	value.Assignees[0] = identity.Reference{}
	if result.CreatedAt.Location() != time.UTC || result.UpdatedAt.Location() != time.UTC ||
		!result.CreatedAt.Equal(start) || result.Assignees[0].IsZero() {
		t.Fatal("observation must normalize instants and own its collections")
	}
	for _, change := range []func(*pullrequests.Observation){
		func(o *pullrequests.Observation) { o.Reference = identity.Reference{} },
		func(o *pullrequests.Observation) { o.Repository = identity.Reference{} },
		func(o *pullrequests.Observation) {
			o.Repository, _ = identity.NewReference("other.example", "R_fixture")
		},
		func(o *pullrequests.Observation) { o.Number = 0 },
		func(o *pullrequests.Observation) { o.Envelope = pullrequests.Envelope{} },
	} {
		invalid := result
		change(&invalid)
		if _, err := pullrequests.NewObservation(invalid); err == nil {
			t.Fatal("invalid observation accepted")
		}
	}
}
