package review

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/quzhihao/code-review/internal/model"
)

func TestFilterBySeverity(t *testing.T) {
	findings := []model.Finding{
		{Severity: model.SeverityInfo, Message: "info"},
		{Severity: model.SeverityWarn, Message: "warn"},
		{Severity: model.SeverityError, Message: "error"},
	}

	result := filterBySeverity(findings, "warn")
	assert.Len(t, result, 2)
	assert.Equal(t, "warn", result[0].Message)
	assert.Equal(t, "error", result[1].Message)
}

func TestDedup(t *testing.T) {
	findings := []model.Finding{
		{RuleID: "r1", FilePath: "a.go", Line: 1, Message: "dup"},
		{RuleID: "r1", FilePath: "a.go", Line: 1, Message: "dup"},
		{RuleID: "r1", FilePath: "a.go", Line: 2, Message: "different"},
	}

	result := dedup(findings)
	assert.Len(t, result, 2)
}

func TestGroupFindings(t *testing.T) {
	findings := []model.Finding{
		{RuleID: "r1", FilePath: "a.go", Line: 1, Severity: model.SeverityWarn, Message: "msg", Source: "rule"},
		{RuleID: "r1", FilePath: "a.go", Line: 5, Severity: model.SeverityWarn, Message: "msg", Source: "rule"},
		{RuleID: "r1", FilePath: "a.go", Line: 10, Severity: model.SeverityWarn, Message: "msg", Source: "rule"},
		{RuleID: "r1", FilePath: "a.go", Line: 15, Severity: model.SeverityWarn, Message: "msg", Source: "rule"},
		{RuleID: "r2", FilePath: "b.go", Line: 1, Severity: model.SeverityError, Message: "other", Source: "rule"},
	}

	result := groupFindings(findings, 3)

	// r1 in a.go had 4 occurrences (>3 threshold), should be grouped
	// r2 in b.go had 1 occurrence, should remain
	grouped := 0
	ungrouped := 0
	for _, f := range result {
		if f.EndLine > 0 {
			grouped++
			assert.Contains(t, f.Message, "4 occurrences")
			assert.Equal(t, 1, f.Line)
			assert.Equal(t, 15, f.EndLine)
		} else {
			ungrouped++
		}
	}
	assert.Equal(t, 1, grouped)
	assert.Equal(t, 1, ungrouped)
}
