package segments

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/emms-garcia/shipq/internal/domain"
	"github.com/emms-garcia/shipq/internal/domain/identity"
	"github.com/emms-garcia/shipq/internal/domain/pullrequests"
)

const (
	FormatVersion   = 1
	MaxDepth        = 16
	MaxNodes        = 256
	MaxSetValues    = 100
	MaxEncodedBytes = 64 * 1024
)

type Operator string

const (
	In       Operator = "in"
	NotIn    Operator = "not_in"
	Any      Operator = "any"
	All      Operator = "all"
	None     Operator = "none"
	Equal    Operator = "eq"
	Greater  Operator = "gt"
	AtLeast  Operator = "gte"
	Less     Operator = "lt"
	AtMost   Operator = "lte"
	Contains Operator = "contains"
)

type ValueKind string

const (
	Identities ValueKind = "identities"
	Number     ValueKind = "number"
	Boolean    ValueKind = "boolean"
	Text       ValueKind = "text"
	Instant    ValueKind = "instant"
	Strings    ValueKind = "strings"
	Labels     ValueKind = "labels"
)

type IdentityKind string

const (
	User         IdentityKind = "user"
	Team         IdentityKind = "team"
	Organization IdentityKind = "organization"
	Repository   IdentityKind = "repository"
)

type IdentityValue struct {
	Kind        IdentityKind
	Reference   identity.Reference
	Me          bool
	DisplayHint string
}

type LabelValue struct {
	Repository  identity.Reference
	Reference   identity.Reference
	DisplayHint string
}

type Value struct {
	Kind       ValueKind
	Identities []IdentityValue
	Number     int64
	Boolean    bool
	Text       string
	Instant    time.Time
	Strings    []string
	Labels     []LabelValue
}

type Predicate struct {
	Field pullrequests.Field
	Op    Operator
	Value Value
}

type Node struct {
	All  []Node
	Any  []Node
	Not  *Node
	Leaf *Predicate
}

type Definition struct {
	FormatVersion int
	Condition     Node
}

type Rule struct {
	definition Definition
	summary    string
	fields     []pullrequests.Field
}

// encodedBytes is the actual definition size measured by the driving boundary.
// Wire parsing and unknown-property rejection belong to that boundary.
func Compile(definition Definition, encodedBytes int) (Rule, error) {
	if encodedBytes <= 0 || encodedBytes > MaxEncodedBytes {
		return Rule{}, invalid("encoded_bytes", "must be the measured size, between 1 and 65536 bytes")
	}
	if definition.FormatVersion != FormatVersion {
		return Rule{}, invalid("format_version", "unsupported rule format version")
	}
	v := validator{fields: make(map[pullrequests.Field]bool)}
	if err := v.node(definition.Condition, "condition", 1); err != nil {
		return Rule{}, err
	}
	definition.Condition = cloneNode(definition.Condition)
	fields := make([]pullrequests.Field, 0, len(v.fields))
	for field := range v.fields {
		fields = append(fields, field)
	}
	slices.Sort(fields)
	return Rule{definition: definition, summary: describe(definition.Condition), fields: fields}, nil
}

func (r Rule) Summary() string { return r.summary }

func (r Rule) RequiredFields() []pullrequests.Field { return slices.Clone(r.fields) }

func (r Rule) Definition() Definition {
	return Definition{FormatVersion: r.definition.FormatVersion, Condition: cloneNode(r.definition.Condition)}
}

func invalid(path, message string) error {
	return &domain.ValidationError{Field: path, Message: message}
}

type validator struct {
	nodes  int
	fields map[pullrequests.Field]bool
}

func (v *validator) node(n Node, path string, depth int) error {
	v.nodes++
	if depth > MaxDepth {
		return invalid(path, "maximum condition depth is 16")
	}
	if v.nodes > MaxNodes {
		return invalid(path, "maximum condition node count is 256")
	}
	shapes := 0
	for _, present := range []bool{n.All != nil, n.Any != nil, n.Not != nil, n.Leaf != nil} {
		if present {
			shapes++
		}
	}
	if shapes != 1 {
		return invalid(path, "must have exactly one of all, any, not, or a leaf")
	}
	if n.Leaf != nil {
		return v.predicate(*n.Leaf, path)
	}
	if n.Not != nil {
		return v.node(*n.Not, path+".not", depth+1)
	}
	children, group := n.All, "all"
	if n.Any != nil {
		children, group = n.Any, "any"
	}
	if len(children) == 0 {
		return invalid(path+"."+group, "must contain at least one condition")
	}
	for i, child := range children {
		if err := v.node(child, fmt.Sprintf("%s.%s[%d]", path, group, i), depth+1); err != nil {
			return err
		}
	}
	if n.All != nil && !consistentBounds(n) {
		return invalid(path, "contradictory approval-count bounds in conjunction")
	}
	return nil
}

func fieldSpec(field pullrequests.Field) (ValueKind, []Operator) {
	switch field {
	case pullrequests.FieldOrganization, pullrequests.FieldRepository, pullrequests.FieldAuthor:
		return Identities, []Operator{In, NotIn}
	case pullrequests.FieldAssignees, pullrequests.FieldRequestedReviewers, pullrequests.FieldReviewedBy, pullrequests.FieldApprovers:
		return Identities, []Operator{Any, All, None}
	case pullrequests.FieldApprovalCount:
		return Number, []Operator{Equal, Greater, AtLeast, Less, AtMost}
	case pullrequests.FieldState, pullrequests.FieldReviewState:
		return Strings, []Operator{In, NotIn}
	case pullrequests.FieldDraft, pullrequests.FieldLocalWatched, pullrequests.FieldRepositoryWatched:
		return Boolean, []Operator{Equal}
	case pullrequests.FieldLabels:
		return Labels, []Operator{Any, All, None}
	case pullrequests.FieldTitle:
		return Text, []Operator{Contains}
	case pullrequests.FieldCreatedAt, pullrequests.FieldUpdatedAt:
		return Instant, []Operator{AtLeast, Less}
	default:
		return "", nil
	}
}

func (v *validator) predicate(p Predicate, path string) error {
	kind, operators := fieldSpec(p.Field)
	if kind == "" {
		return invalid(path+".field", "unsupported field")
	}
	if !slices.Contains(operators, p.Op) {
		return invalid(path+".op", "unsupported operator for this field")
	}
	value := p.Value
	if value.Kind != kind ||
		(kind != Identities && len(value.Identities) != 0) ||
		(kind != Number && value.Number != 0) ||
		(kind != Boolean && value.Boolean) ||
		(kind != Text && value.Text != "") ||
		(kind != Instant && !value.Instant.IsZero()) ||
		(kind != Strings && len(value.Strings) != 0) ||
		(kind != Labels && len(value.Labels) != 0) {
		return invalid(path+".value", "value must have exactly the field's expected type")
	}
	path += ".value"
	switch kind {
	case Number:
		if value.Number < 0 || !consistentBounds(Node{Leaf: &p}) {
			return invalid(path, "must describe a possible nonnegative integer count")
		}
	case Text:
		if value.Text == "" || !utf8.ValidString(value.Text) {
			return invalid(path, "must be nonempty valid Unicode text")
		}
	case Instant:
		if value.Instant.IsZero() {
			return invalid(path, "must be a nonzero instant")
		}
	case Identities:
		if err := validateSetSize(len(value.Identities), path); err != nil {
			return err
		}
		for i, selected := range value.Identities {
			expected := User
			if p.Field == pullrequests.FieldOrganization {
				expected = Organization
			} else if p.Field == pullrequests.FieldRepository {
				expected = Repository
			}
			allowedTeam := p.Field == pullrequests.FieldRequestedReviewers && selected.Kind == Team
			if (selected.Kind != expected && !allowedTeam) ||
				(selected.Me && (selected.Kind != User || !selected.Reference.IsZero())) ||
				(!selected.Me && selected.Reference.IsZero()) {
				return invalid(fmt.Sprintf("%s[%d]", path, i), "invalid identity kind or reference; me must be a user without a fixed reference")
			}
		}
	case Labels:
		if err := validateSetSize(len(value.Labels), path); err != nil {
			return err
		}
		for i, label := range value.Labels {
			if label.Reference.IsZero() || label.Repository.IsZero() || label.Reference.Host() != label.Repository.Host() {
				return invalid(fmt.Sprintf("%s[%d]", path, i), "label must have stable label and repository identities on the same host")
			}
		}
	case Strings:
		if err := validateSetSize(len(value.Strings), path); err != nil {
			return err
		}
		allowed := []string{"open", "closed", "merged"}
		if p.Field == pullrequests.FieldReviewState {
			allowed = []string{"approved", "changes_requested", "none"}
		}
		for i, item := range value.Strings {
			if !slices.Contains(allowed, item) {
				return invalid(fmt.Sprintf("%s[%d]", path, i), "unsupported state")
			}
		}
	}
	v.fields[p.Field] = true
	return nil
}

func validateSetSize(size int, path string) error {
	if size == 0 || size > MaxSetValues {
		return invalid(path, "sets must contain between 1 and 100 values")
	}
	return nil
}

func consistentBounds(n Node) bool {
	minimum, maximum := int64(0), int64(math.MaxInt64)
	excluded := make(map[int64]bool)
	var intersect func(Node, bool) bool
	intersect = func(n Node, negated bool) bool {
		if n.Not != nil {
			return intersect(*n.Not, !negated)
		}
		// Intersect only mandatory conjuncts, never alternative branches.
		if (n.All != nil && !negated) || (n.Any != nil && negated) {
			children := n.All
			if negated {
				children = n.Any
			}
			for _, child := range children {
				if !intersect(child, negated) {
					return false
				}
			}
		} else if n.Leaf != nil && n.Leaf.Field == pullrequests.FieldApprovalCount {
			value := n.Leaf.Value.Number
			op := n.Leaf.Op
			if negated {
				switch op {
				case Equal:
					excluded[value] = true
					return true
				case Greater:
					op = AtMost
				case AtLeast:
					op = Less
				case Less:
					op = AtLeast
				case AtMost:
					op = Greater
				}
			}
			switch op {
			case Equal:
				minimum, maximum = max(minimum, value), min(maximum, value)
			case Greater:
				if value == math.MaxInt64 {
					return false
				}
				minimum = max(minimum, value+1)
			case AtLeast:
				minimum = max(minimum, value)
			case Less:
				maximum = min(maximum, value-1)
			case AtMost:
				maximum = min(maximum, value)
			}
		}
		return minimum <= maximum
	}
	if !intersect(n, false) {
		return false
	}
	excludedWithinBounds := int64(0)
	for value := range excluded {
		if value >= minimum && value <= maximum {
			excludedWithinBounds++
		}
	}
	return maximum-minimum >= excludedWithinBounds
}

func cloneNode(n Node) Node {
	if n.Leaf != nil {
		p := *n.Leaf
		p.Value.Identities = slices.Clone(p.Value.Identities)
		p.Value.Strings = slices.Clone(p.Value.Strings)
		p.Value.Labels = slices.Clone(p.Value.Labels)
		if !p.Value.Instant.IsZero() {
			p.Value.Instant = p.Value.Instant.UTC()
		}
		n.Leaf = &p
	}
	if n.Not != nil {
		child := cloneNode(*n.Not)
		n.Not = &child
	}
	if n.All != nil {
		n.All = slices.Clone(n.All)
		for i := range n.All {
			n.All[i] = cloneNode(n.All[i])
		}
	}
	if n.Any != nil {
		n.Any = slices.Clone(n.Any)
		for i := range n.Any {
			n.Any[i] = cloneNode(n.Any[i])
		}
	}
	return n
}

func describe(n Node) string {
	if n.Not != nil {
		return "not (" + describe(*n.Not) + ")"
	}
	if n.Leaf == nil {
		children, prefix := n.All, "all of ("
		if n.Any != nil {
			children, prefix = n.Any, "any of ("
		}
		parts := make([]string, len(children))
		for i, child := range children {
			parts[i] = describe(child)
		}
		return prefix + strings.Join(parts, "; ") + ")"
	}
	p := *n.Leaf
	verb := map[Operator]string{
		In: "is one of", NotIn: "is not one of", Any: "includes any of",
		All: "includes all of", None: "includes none of", Equal: "is exactly",
		Greater: "is greater than", AtLeast: "is at least", Less: "is less than",
		AtMost: "is at most", Contains: "contains",
	}[p.Op]
	var value string
	switch p.Value.Kind {
	case Number:
		value = strconv.FormatInt(p.Value.Number, 10)
	case Boolean:
		value = strconv.FormatBool(p.Value.Boolean)
	case Text:
		value = strconv.Quote(p.Value.Text)
	case Instant:
		verb = "is on or after"
		if p.Op == Less {
			verb = "is before"
		}
		value = p.Value.Instant.Format(time.RFC3339Nano)
	default:
		items := []string{}
		for _, selected := range p.Value.Identities {
			if selected.Me {
				items = append(items, "me (connected account)")
			} else {
				items = append(items, hint(selected.DisplayHint)+string(selected.Kind)+" "+referenceText(selected.Reference))
			}
		}
		for _, label := range p.Value.Labels {
			items = append(items, hint(label.DisplayHint)+"label "+referenceText(label.Reference)+" in repository "+referenceText(label.Repository))
		}
		for _, state := range p.Value.Strings {
			items = append(items, strconv.Quote(state))
		}
		value = "[" + strings.Join(items, ", ") + "]"
	}
	return strings.ReplaceAll(string(p.Field), "_", " ") + " " + verb + " " + value
}

func referenceText(r identity.Reference) string { return strconv.Quote(r.Host() + "/" + r.NodeID()) }

func hint(value string) string {
	if value == "" {
		return ""
	}
	return strconv.Quote(value) + " "
}
