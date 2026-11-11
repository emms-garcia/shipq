package localapi

import (
	"encoding/json"
	"fmt"
	"time"

	v1 "github.com/emms-garcia/shipq/api/local/v1"
	"github.com/emms-garcia/shipq/internal/application"
	"github.com/emms-garcia/shipq/internal/domain"
	"github.com/emms-garcia/shipq/internal/domain/identity"
	"github.com/emms-garcia/shipq/internal/domain/pullrequests"
	"github.com/emms-garcia/shipq/internal/domain/segments"
)

func validateRule(data []byte) (application.RuleValidation, error) {
	if len(data) > segments.MaxEncodedBytes {
		return application.RuleValidation{}, ruleError("encoded_bytes", "maximum encoded definition size is 65536 bytes")
	}
	object, err := v1.DecodeObject(data, "definition", []string{"format_version", "condition"})
	if err != nil {
		return application.RuleValidation{}, err
	}
	var version int
	if err := v1.DecodeValue(object["format_version"], "format_version", &version); err != nil {
		return application.RuleValidation{}, err
	}
	if version != segments.FormatVersion {
		return application.RuleValidation{}, ruleError("format_version", "unsupported rule format version")
	}
	nodes := 0
	condition, err := decodeNode(object["condition"], "condition", 1, &nodes)
	if err != nil {
		return application.RuleValidation{}, err
	}
	return application.ValidateRule(application.ValidateRuleInput{
		Definition:   segments.Definition{FormatVersion: version, Condition: condition},
		EncodedBytes: len(data),
	})
}

func ruleError(path, message string) error {
	return &domain.ValidationError{Field: path, Message: message}
}

func decodeNode(data []byte, path string, depth int, nodes *int) (segments.Node, error) {
	*nodes++
	if depth > segments.MaxDepth || *nodes > segments.MaxNodes {
		return segments.Node{}, ruleError(path, "condition exceeds depth or node limits")
	}
	object, err := v1.DecodeObject(data, path, []string{"all", "any", "not", "field", "op", "value"})
	if err != nil {
		return segments.Node{}, err
	}
	for _, group := range []string{"all", "any", "not"} {
		if raw, present := object[group]; present {
			if len(object) != 1 {
				return segments.Node{}, &v1.SchemaError{Path: path, Message: "group must be the only node shape"}
			}
			if group == "not" {
				child, err := decodeNode(raw, path+".not", depth+1, nodes)
				return segments.Node{Not: &child}, err
			}
			var rawChildren []json.RawMessage
			if err := v1.DecodeValue(raw, path+"."+group, &rawChildren); err != nil {
				return segments.Node{}, err
			}
			children := make([]segments.Node, len(rawChildren))
			for i, child := range rawChildren {
				children[i], err = decodeNode(child, fmt.Sprintf("%s.%s[%d]", path, group, i), depth+1, nodes)
				if err != nil {
					return segments.Node{}, err
				}
			}
			if group == "all" {
				return segments.Node{All: children}, nil
			}
			return segments.Node{Any: children}, nil
		}
	}
	var field, op string
	if err := v1.DecodeValue(object["field"], path+".field", &field); err != nil {
		return segments.Node{}, err
	}
	if err := v1.DecodeValue(object["op"], path+".op", &op); err != nil {
		return segments.Node{}, err
	}
	if _, present := object["value"]; !present {
		return segments.Node{}, &v1.SchemaError{Path: path + ".value", Message: "leaf value is required"}
	}
	value, err := decodeRuleValue(pullrequests.Field(field), object["value"], path+".value")
	if err != nil {
		return segments.Node{}, err
	}
	return segments.Node{Leaf: &segments.Predicate{Field: pullrequests.Field(field), Op: segments.Operator(op), Value: value}}, nil
}

func decodeRuleValue(field pullrequests.Field, data []byte, path string) (segments.Value, error) {
	var value segments.Value
	switch field {
	case pullrequests.FieldApprovalCount:
		value.Kind = segments.Number
		err := v1.DecodeValue(data, path, &value.Number)
		return value, err
	case pullrequests.FieldDraft, pullrequests.FieldLocalWatched, pullrequests.FieldRepositoryWatched:
		value.Kind = segments.Boolean
		err := v1.DecodeValue(data, path, &value.Boolean)
		return value, err
	case pullrequests.FieldTitle:
		value.Kind = segments.Text
		err := v1.DecodeValue(data, path, &value.Text)
		return value, err
	case pullrequests.FieldCreatedAt, pullrequests.FieldUpdatedAt:
		var text string
		if err := v1.DecodeValue(data, path, &text); err != nil {
			return value, err
		}
		at, err := time.Parse(time.RFC3339Nano, text)
		if err != nil {
			return value, &v1.SchemaError{Path: path, Message: "must be an RFC 3339 UTC instant"}
		}
		if _, offset := at.Zone(); offset != 0 {
			return value, &v1.SchemaError{Path: path, Message: "must use UTC"}
		}
		return segments.Value{Kind: segments.Instant, Instant: at.UTC()}, nil
	case pullrequests.FieldState, pullrequests.FieldReviewState,
		pullrequests.FieldOrganization, pullrequests.FieldRepository, pullrequests.FieldAuthor,
		pullrequests.FieldAssignees, pullrequests.FieldRequestedReviewers, pullrequests.FieldReviewedBy,
		pullrequests.FieldApprovers, pullrequests.FieldLabels:
		var items []json.RawMessage
		if err := v1.DecodeValue(data, path, &items); err != nil {
			return value, err
		}
		if len(items) > segments.MaxSetValues {
			return value, ruleError(path, "maximum values per set is 100")
		}
		switch field {
		case pullrequests.FieldState, pullrequests.FieldReviewState:
			value.Kind = segments.Strings
			for i, raw := range items {
				var item string
				if err := v1.DecodeValue(raw, fmt.Sprintf("%s[%d]", path, i), &item); err != nil {
					return value, err
				}
				value.Strings = append(value.Strings, item)
			}
		case pullrequests.FieldLabels:
			value.Kind = segments.Labels
			for i, raw := range items {
				label, err := decodeLabel(raw, fmt.Sprintf("%s[%d]", path, i))
				if err != nil {
					return value, err
				}
				value.Labels = append(value.Labels, label)
			}
		default:
			value.Kind = segments.Identities
			kind := segments.User
			if field == pullrequests.FieldOrganization {
				kind = segments.Organization
			} else if field == pullrequests.FieldRepository {
				kind = segments.Repository
			}
			for i, raw := range items {
				selected, err := decodeIdentity(raw, kind, fmt.Sprintf("%s[%d]", path, i))
				if err != nil {
					return value, err
				}
				value.Identities = append(value.Identities, selected)
			}
		}
		return value, nil
	default:
		// Unknown fields are a domain validation error, not a guessed wire type.
		return value, nil
	}
}

func decodeIdentity(data []byte, defaultKind segments.IdentityKind, path string) (segments.IdentityValue, error) {
	var compact string
	if json.Unmarshal(data, &compact) == nil && compact != "" {
		if compact == "me" {
			return segments.IdentityValue{Kind: defaultKind, Me: true}, nil
		}
		reference, err := newReference("github.com", compact, path)
		return segments.IdentityValue{Kind: defaultKind, Reference: reference}, err
	}
	object, err := v1.DecodeObject(data, path, []string{"kind", "github_host", "node_id", "display_hint"})
	if err != nil {
		return segments.IdentityValue{}, err
	}
	host, nodeID, hint, err := referenceProperties(object, path)
	if err != nil {
		return segments.IdentityValue{}, err
	}
	var kind string
	if err := v1.DecodeValue(object["kind"], path+".kind", &kind); err != nil {
		return segments.IdentityValue{}, err
	}
	reference, err := newReference(host, nodeID, path)
	return segments.IdentityValue{Kind: segments.IdentityKind(kind), Reference: reference, DisplayHint: hint}, err
}

func decodeLabel(data []byte, path string) (segments.LabelValue, error) {
	object, err := v1.DecodeObject(data, path, []string{"github_host", "repository_node_id", "node_id", "display_hint"})
	if err != nil {
		return segments.LabelValue{}, err
	}
	host, nodeID, hint, err := referenceProperties(object, path)
	if err != nil {
		return segments.LabelValue{}, err
	}
	var repositoryID string
	if err := v1.DecodeValue(object["repository_node_id"], path+".repository_node_id", &repositoryID); err != nil {
		return segments.LabelValue{}, err
	}
	reference, err := newReference(host, nodeID, path)
	if err != nil {
		return segments.LabelValue{}, err
	}
	repository, err := newReference(host, repositoryID, path+".repository_node_id")
	return segments.LabelValue{Repository: repository, Reference: reference, DisplayHint: hint}, err
}

func referenceProperties(object map[string]json.RawMessage, path string) (string, string, string, error) {
	host, nodeID, hint := "github.com", "", ""
	if raw, present := object["github_host"]; present {
		if err := v1.DecodeValue(raw, path+".github_host", &host); err != nil {
			return "", "", "", err
		}
	}
	if err := v1.DecodeValue(object["node_id"], path+".node_id", &nodeID); err != nil {
		return "", "", "", err
	}
	if raw, present := object["display_hint"]; present {
		if err := v1.DecodeValue(raw, path+".display_hint", &hint); err != nil {
			return "", "", "", err
		}
	}
	return host, nodeID, hint, nil
}

func newReference(host, nodeID, path string) (identity.Reference, error) {
	reference, err := identity.NewReference(host, nodeID)
	if err != nil {
		return identity.Reference{}, ruleError(path, "invalid stable identity")
	}
	return reference, nil
}
