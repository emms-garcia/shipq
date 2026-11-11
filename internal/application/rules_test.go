package application_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/emms-garcia/shipq/internal/application"
	"github.com/emms-garcia/shipq/internal/domain"
	pr "github.com/emms-garcia/shipq/internal/domain/pullrequests"
	"github.com/emms-garcia/shipq/internal/domain/segments"
)

func TestValidateRule(t *testing.T) {
	definition := segments.Definition{FormatVersion: 1, Condition: segments.Node{Leaf: &segments.Predicate{
		Field: pr.FieldApprovalCount, Op: segments.AtLeast, Value: segments.Value{Kind: segments.Number, Number: 3},
	}}}
	result, err := application.ValidateRule(application.ValidateRuleInput{Definition: definition, EncodedBytes: 150})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Summary, "at least 3") ||
		!reflect.DeepEqual(result.RequiredFields, []pr.Field{pr.FieldApprovalCount}) ||
		result.Definition.Condition.Leaf.Field != pr.FieldApprovalCount {
		t.Fatalf("missing core-provided validation result: %+v", result)
	}
	definition.Condition.Leaf.Value.Number = 999
	if result.Definition.Condition.Leaf.Value.Number != 3 {
		t.Fatal("validation result aliases request")
	}
	for _, input := range []application.ValidateRuleInput{
		{Definition: definition, EncodedBytes: segments.MaxEncodedBytes + 1},
		{Definition: segments.Definition{FormatVersion: 2}, EncodedBytes: 100},
		{Definition: segments.Definition{FormatVersion: 1}, EncodedBytes: 100},
		{Definition: definition},
	} {
		result, err := application.ValidateRule(input)
		var validation *domain.ValidationError
		if !errors.As(err, &validation) || validation.Field == "" || result.Summary != "" {
			t.Fatalf("typed core errors must survive use case without a partial result: %+v, %v", result, err)
		}
	}
}
