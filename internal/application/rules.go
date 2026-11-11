package application

import (
	"github.com/emms-garcia/shipq/internal/domain/pullrequests"
	"github.com/emms-garcia/shipq/internal/domain/segments"
)

type ValidateRuleInput struct {
	Definition   segments.Definition
	EncodedBytes int
}

type RuleValidation struct {
	Definition     segments.Definition
	Summary        string
	RequiredFields []pullrequests.Field
}

func ValidateRule(input ValidateRuleInput) (RuleValidation, error) {
	rule, err := segments.Compile(input.Definition, input.EncodedBytes)
	if err != nil {
		return RuleValidation{}, err
	}
	return RuleValidation{Definition: rule.Definition(), Summary: rule.Summary(), RequiredFields: rule.RequiredFields()}, nil
}
