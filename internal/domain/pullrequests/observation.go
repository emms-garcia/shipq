package pullrequests

import (
	"maps"
	"slices"
	"time"

	"github.com/emms-garcia/shipq/internal/domain"
	"github.com/emms-garcia/shipq/internal/domain/identity"
	"github.com/emms-garcia/shipq/internal/domain/reviews"
)

type State string

const (
	Open   State = "open"
	Closed State = "closed"
	Merged State = "merged"
)

type Field string

const (
	FieldOrganization       Field = "organization"
	FieldRepository         Field = "repository"
	FieldAuthor             Field = "author"
	FieldAssignees          Field = "assignees"
	FieldRequestedReviewers Field = "requested_reviewers"
	FieldReviewedBy         Field = "reviewed_by"
	FieldApprovers          Field = "approvers"
	FieldApprovalCount      Field = "approval_count"
	FieldReviewState        Field = "review_state"
	FieldState              Field = "state"
	FieldDraft              Field = "draft"
	FieldLabels             Field = "labels"
	FieldTitle              Field = "title"
	FieldCreatedAt          Field = "created_at"
	FieldUpdatedAt          Field = "updated_at"
	FieldLocalWatched       Field = "local_watched"
	FieldRepositoryWatched  Field = "repository_watched"
)

const NotCollected = "not_collected"

type FieldStatus struct {
	Complete bool
	Reason   string
}

type Envelope struct {
	Revision    uint64
	StartedAt   time.Time
	CompletedAt time.Time
	ContentHash string
	fields      map[Field]FieldStatus
}

func NewEnvelope(revision uint64, start, end time.Time, fields map[Field]FieldStatus) (Envelope, error) {
	if revision == 0 {
		return Envelope{}, &domain.ValidationError{Field: "revision", Message: "must be positive"}
	}
	if start.IsZero() || end.IsZero() || end.Before(start) {
		return Envelope{}, &domain.ValidationError{Field: "collection_interval", Message: "must contain ordered, nonzero instants"}
	}
	for field, status := range fields {
		if field == "" || (status.Complete && status.Reason != "") || (!status.Complete && status.Reason == "") {
			return Envelope{}, &domain.ValidationError{Field: "fields", Message: "each named field must be complete or have an unknown reason"}
		}
	}
	return Envelope{Revision: revision, StartedAt: start.UTC(), CompletedAt: end.UTC(), fields: maps.Clone(fields)}, nil
}

func (e Envelope) Status(field Field) FieldStatus {
	if status, ok := e.fields[field]; ok {
		return status
	}
	return FieldStatus{Reason: NotCollected}
}

func (e Envelope) HasCompleteFields(fields ...Field) bool {
	for _, field := range fields {
		if !e.Status(field).Complete {
			return false
		}
	}
	return true
}

type Label struct {
	Reference  identity.Reference
	Repository identity.Reference
	Name       string
}

type Observation struct {
	Reference         identity.Reference
	Repository        identity.Reference
	Organization      identity.Reference
	Number            int
	Title             string
	URL               string
	Author            identity.Reference
	Assignees         []identity.Reference
	RequestedUsers    []identity.Reference
	RequestedTeams    []identity.Reference
	Labels            []Label
	State             State
	Draft             bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
	HeadCommit        string
	LocalWatched      bool
	RepositoryWatched bool
	Reviews           reviews.Facts
	Envelope          Envelope
}

func NewObservation(value Observation) (Observation, error) {
	if value.Reference.IsZero() || value.Repository.IsZero() ||
		value.Reference.Host() != value.Repository.Host() {
		return Observation{}, &domain.ValidationError{Field: "reference", Message: "PR and repository must have identities on the same host"}
	}
	if value.Number <= 0 {
		return Observation{}, &domain.ValidationError{Field: "number", Message: "must be positive"}
	}
	envelope, err := NewEnvelope(value.Envelope.Revision, value.Envelope.StartedAt, value.Envelope.CompletedAt, value.Envelope.fields)
	if err != nil {
		return Observation{}, err
	}
	envelope.ContentHash = value.Envelope.ContentHash
	value.Envelope = envelope
	value.CreatedAt = value.CreatedAt.UTC()
	value.UpdatedAt = value.UpdatedAt.UTC()
	value.Assignees = slices.Clone(value.Assignees)
	value.RequestedUsers = slices.Clone(value.RequestedUsers)
	value.RequestedTeams = slices.Clone(value.RequestedTeams)
	value.Labels = slices.Clone(value.Labels)
	value.Reviews.Approvers = slices.Clone(value.Reviews.Approvers)
	value.Reviews.ChangesRequestedBy = slices.Clone(value.Reviews.ChangesRequestedBy)
	value.Reviews.ReviewedBy = slices.Clone(value.Reviews.ReviewedBy)
	value.Reviews.UnknownReasons = slices.Clone(value.Reviews.UnknownReasons)
	return value, nil
}
