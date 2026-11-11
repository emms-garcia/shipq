package segments

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/emms-garcia/shipq/internal/domain/identity"
	"github.com/emms-garcia/shipq/internal/domain/pullrequests"
	"golang.org/x/text/cases"
)

type Match string

const (
	Matches Match = "match"
	NoMatch Match = "no_match"
	Unknown Match = "unknown"
)

type Context struct {
	CurrentUser identity.Reference
	// Fresh must be explicitly established by the application, not inferred from age.
	Fresh                bool
	UnresolvedIdentities map[identity.Reference]bool
}

type Evidence struct {
	Path       string
	Field      pullrequests.Field
	Result     Match
	Reason     string
	ObservedAt time.Time
	Fresh      bool
}

type Evaluation struct {
	Result               Match
	Evidence             []Evidence
	NotificationEligible bool
}

func (r Rule) Evaluate(observation pullrequests.Observation, context Context) Evaluation {
	if r.definition.FormatVersion != FormatVersion {
		return Evaluation{Result: Unknown, Evidence: []Evidence{{Path: "condition", Result: Unknown, Reason: "unvalidated_rule"}}}
	}
	result := Evaluation{NotificationEligible: context.Fresh}
	var evaluate func(Node, string) Match
	evaluate = func(n Node, path string) Match {
		if n.Leaf != nil {
			evidence := evaluatePredicate(*n.Leaf, observation, context)
			evidence.Path = path
			result.Evidence = append(result.Evidence, evidence)
			if evidence.Result == Unknown {
				result.NotificationEligible = false
			}
			return evidence.Result
		}
		if n.Not != nil {
			return invert(evaluate(*n.Not, path+".not"))
		}
		children, group, combined := n.All, "all", Matches
		if n.Any != nil {
			children, group, combined = n.Any, "any", NoMatch
		}
		for i, child := range children {
			value := evaluate(child, fmt.Sprintf("%s.%s[%d]", path, group, i))
			if group == "all" {
				if combined == NoMatch || value == NoMatch {
					combined = NoMatch
				} else if combined == Unknown || value == Unknown {
					combined = Unknown
				}
			} else if combined == Matches || value == Matches {
				combined = Matches
			} else if combined == Unknown || value == Unknown {
				combined = Unknown
			}
		}
		return combined
	}
	result.Result = evaluate(r.definition.Condition, "condition")
	return result
}

func invert(value Match) Match {
	switch value {
	case Matches:
		return NoMatch
	case NoMatch:
		return Matches
	default:
		return Unknown
	}
}

func evaluatePredicate(p Predicate, o pullrequests.Observation, c Context) Evidence {
	e := Evidence{Field: p.Field, Result: Unknown, ObservedAt: o.Envelope.CompletedAt.UTC()}
	envelope := o.Envelope
	switch {
	case o.Reference.IsZero() || envelope.Revision == 0 || envelope.StartedAt.IsZero() ||
		envelope.CompletedAt.IsZero() || envelope.CompletedAt.Before(envelope.StartedAt):
		e.Reason = "invalid_observation"
	case !envelope.Status(p.Field).Complete:
		e.Reason = envelope.Status(p.Field).Reason
	default:
		e.Result, e.Reason = matchPredicate(p, o, c)
		e.Fresh = c.Fresh && e.Result != Unknown
	}
	return e
}

func matchPredicate(p Predicate, o pullrequests.Observation, c Context) (Match, string) {
	value := p.Value
	switch value.Kind {
	case Identities:
		actual := []subject{}
		add := func(kind IdentityKind, references ...identity.Reference) bool {
			for _, reference := range references {
				if reference.IsZero() || reference.Host() != o.Reference.Host() {
					return false
				}
				actual = append(actual, subject{kind, reference})
			}
			return true
		}
		valid := true
		switch p.Field {
		case pullrequests.FieldOrganization:
			valid = add(Organization, o.Organization)
		case pullrequests.FieldRepository:
			valid = add(Repository, o.Repository)
		case pullrequests.FieldAuthor:
			valid = add(User, o.Author)
		case pullrequests.FieldAssignees:
			valid = add(User, o.Assignees...)
		case pullrequests.FieldRequestedReviewers:
			valid = add(User, o.RequestedUsers...) && add(Team, o.RequestedTeams...)
		case pullrequests.FieldApprovers:
			if !o.Reviews.OpinionsKnown {
				return Unknown, "review_opinions_unknown"
			}
			valid = add(User, o.Reviews.Approvers...)
		case pullrequests.FieldReviewedBy:
			if !o.Reviews.ParticipantsKnown {
				return Unknown, "review_participants_unknown"
			}
			valid = add(User, o.Reviews.ReviewedBy...)
		}
		if !valid {
			return Unknown, "unresolved_observed_identity"
		}
		wanted := make([]subject, len(value.Identities))
		for i, selected := range value.Identities {
			reference := selected.Reference
			if selected.Me {
				reference = c.CurrentUser
				if reference.IsZero() || reference.Host() != o.Reference.Host() {
					return Unknown, "connected_account_unresolved"
				}
			}
			if c.UnresolvedIdentities[reference] {
				return Unknown, "selected_identity_unresolved"
			}
			wanted[i] = subject{selected.Kind, reference}
		}
		return setMatch(actual, wanted, p.Op), ""
	case Labels:
		actual := make([]labelKey, len(o.Labels))
		for i, label := range o.Labels {
			if label.Reference.IsZero() || label.Repository.IsZero() ||
				label.Repository != o.Repository || label.Reference.Host() != o.Reference.Host() {
				return Unknown, "unresolved_observed_label"
			}
			actual[i] = labelKey{label.Repository, label.Reference}
		}
		wanted := make([]labelKey, len(value.Labels))
		for i, label := range value.Labels {
			if c.UnresolvedIdentities[label.Reference] || c.UnresolvedIdentities[label.Repository] {
				return Unknown, "selected_identity_unresolved"
			}
			wanted[i] = labelKey{label.Repository, label.Reference}
		}
		return setMatch(actual, wanted, p.Op), ""
	case Number:
		count, known := o.Reviews.ApprovalCount()
		if !known {
			return Unknown, "review_opinions_unknown"
		}
		n := int64(count)
		matches := false
		switch p.Op {
		case Equal:
			matches = n == value.Number
		case Greater:
			matches = n > value.Number
		case AtLeast:
			matches = n >= value.Number
		case Less:
			matches = n < value.Number
		case AtMost:
			matches = n <= value.Number
		}
		return truth(matches), ""
	case Boolean:
		actual := o.Draft
		if p.Field == pullrequests.FieldLocalWatched {
			actual = o.LocalWatched
		} else if p.Field == pullrequests.FieldRepositoryWatched {
			actual = o.RepositoryWatched
		}
		return truth(actual == value.Boolean), ""
	case Text:
		if !utf8.ValidString(o.Title) {
			return Unknown, "invalid_observed_text"
		}
		fold := cases.Fold()
		return truth(strings.Contains(fold.String(o.Title), fold.String(value.Text))), ""
	case Instant:
		at := o.CreatedAt
		if p.Field == pullrequests.FieldUpdatedAt {
			at = o.UpdatedAt
		}
		if at.IsZero() {
			return Unknown, "missing_observed_instant"
		}
		matches := at.Before(value.Instant)
		if p.Op == AtLeast {
			matches = !matches
		}
		return truth(matches), ""
	case Strings:
		actual := string(o.State)
		if p.Field == pullrequests.FieldReviewState {
			decision, known := o.Reviews.Decision()
			if !known {
				return Unknown, "review_opinions_unknown"
			}
			actual = string(decision)
		} else if o.State != pullrequests.Open && o.State != pullrequests.Closed && o.State != pullrequests.Merged {
			return Unknown, "invalid_observed_state"
		}
		return setMatch([]string{actual}, value.Strings, p.Op), ""
	default:
		return Unknown, "unsupported_value"
	}
}

type subject struct {
	kind      IdentityKind
	reference identity.Reference
}

type labelKey struct {
	repository identity.Reference
	reference  identity.Reference
}

func setMatch[T comparable](actual, wanted []T, op Operator) Match {
	found, all := false, true
	for _, selected := range wanted {
		present := slices.Contains(actual, selected)
		found, all = found || present, all && present
	}
	switch op {
	case None, NotIn:
		return truth(!found)
	case All:
		return truth(all)
	default:
		return truth(found)
	}
}

func truth(value bool) Match {
	if value {
		return Matches
	}
	return NoMatch
}
