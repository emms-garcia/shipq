package identity

import (
	"strings"
	"unicode"

	"github.com/google/uuid"

	"github.com/emms-garcia/shipq/internal/domain"
)

type ID [16]byte

func ParseID(value string) (ID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil || parsed == uuid.Nil {
		return ID{}, &domain.ValidationError{Field: "id", Message: "must be a nonzero UUID"}
	}
	return ID(parsed), nil
}

func (id ID) String() string {
	return uuid.UUID(id).String()
}

type Reference struct {
	host   string
	nodeID string
}

func NewReference(host, nodeID string) (Reference, error) {
	const maxHostLength = 253
	const maxLabelLength = 63
	host = strings.ToLower(host)
	if host == "" || len(host) > maxHostLength {
		return Reference{}, &domain.ValidationError{Field: "host", Message: "must be a DNS hostname without a scheme, port, or path"}
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > maxLabelLength || label[0] == '-' || label[len(label)-1] == '-' {
			return Reference{}, &domain.ValidationError{Field: "host", Message: "must contain valid DNS labels"}
		}
		for _, character := range label {
			if !(character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-') {
				return Reference{}, &domain.ValidationError{Field: "host", Message: "must contain valid DNS labels"}
			}
		}
	}
	if nodeID == "" || strings.ContainsFunc(nodeID, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return Reference{}, &domain.ValidationError{Field: "node_id", Message: "must be a nonempty opaque identifier without whitespace or control characters"}
	}
	return Reference{host: host, nodeID: nodeID}, nil
}

func (r Reference) Host() string   { return r.host }
func (r Reference) NodeID() string { return r.nodeID }
func (r Reference) IsZero() bool   { return r.nodeID == "" }
