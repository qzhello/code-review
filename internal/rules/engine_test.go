package rules

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/quzhihao/code-review/internal/model"
)

func TestEngine_PatternRule(t *testing.T) {
	rules := []model.Rule{
		{
			ID:       "no-println",
			Severity: "warn",
			Description: "Debug print detected",
			File:     "*.go",
			Pattern:  `fmt\.Println`,
			Scope:    model.ScopeAdded,
			Enabled:  true,
		},
	}

	engine, err := NewEngine(rules)
	require.NoError(t, err)

	diff := &model.DiffResult{
		Files: []model.FileDiff{
			{
				NewPath: "main.go",
				Status:  model.FileModified,
				Hunks: []model.Hunk{
					{
						NewStart: 10,
						Lines: []model.Line{
							{Type: model.LineContext, Content: "func main() {", NewNum: 10},
							{Type: model.LineAdded, Content: `	fmt.Println("debug")`, NewNum: 11},
							{Type: model.LineContext, Content: "}", NewNum: 12},
						},
					},
				},
			},
		},
	}

	findings := engine.Evaluate(diff)
	require.Len(t, findings, 1)
	assert.Equal(t, "no-println", findings[0].RuleID)
	assert.Equal(t, model.SeverityWarn, findings[0].Severity)
	assert.Equal(t, "main.go", findings[0].FilePath)
	assert.Equal(t, 11, findings[0].Line)
	assert.Equal(t, "rule", findings[0].Source)
}

func TestEngine_PatternRule_NoMatch(t *testing.T) {
	rules := []model.Rule{
		{
			ID:       "no-println",
			Severity: "warn",
			Description: "Debug print detected",
			File:     "*.go",
			Pattern:  `fmt\.Println`,
			Scope:    model.ScopeAdded,
			Enabled:  true,
		},
	}

	engine, err := NewEngine(rules)
	require.NoError(t, err)

	diff := &model.DiffResult{
		Files: []model.FileDiff{
			{
				NewPath: "main.go",
				Status:  model.FileModified,
				Hunks: []model.Hunk{
					{
						Lines: []model.Line{
							{Type: model.LineAdded, Content: `	log.Info("safe")`, NewNum: 5},
						},
					},
				},
			},
		},
	}

	findings := engine.Evaluate(diff)
	assert.Len(t, findings, 0)
}

func TestEngine_FileGlob(t *testing.T) {
	rules := []model.Rule{
		{
			ID:       "no-console",
			Severity: "warn",
			Description: "console.log detected",
			File:     "*.js",
			Pattern:  `console\.log`,
			Scope:    model.ScopeAdded,
			Enabled:  true,
		},
	}

	engine, err := NewEngine(rules)
	require.NoError(t, err)

	diff := &model.DiffResult{
		Files: []model.FileDiff{
			{
				NewPath: "main.go",
				Hunks: []model.Hunk{
					{Lines: []model.Line{{Type: model.LineAdded, Content: `console.log("test")`, NewNum: 1}}},
				},
			},
			{
				NewPath: "app.js",
				Hunks: []model.Hunk{
					{Lines: []model.Line{{Type: model.LineAdded, Content: `console.log("test")`, NewNum: 1}}},
				},
			},
		},
	}

	findings := engine.Evaluate(diff)
	require.Len(t, findings, 1)
	assert.Equal(t, "app.js", findings[0].FilePath)
}

func TestEngine_StructuralRule(t *testing.T) {
	rules := []model.Rule{
		{
			ID:              "large-diff",
			Severity:        "info",
			Description:     "Large diff",
			MaxChangedLines: 5,
			Enabled:         true,
		},
	}

	engine, err := NewEngine(rules)
	require.NoError(t, err)

	// 10 added lines
	lines := make([]model.Line, 10)
	for i := range lines {
		lines[i] = model.Line{Type: model.LineAdded, Content: "x", NewNum: i + 1}
	}

	diff := &model.DiffResult{
		Files: []model.FileDiff{
			{NewPath: "big.go", Hunks: []model.Hunk{{Lines: lines}}},
		},
	}

	findings := engine.Evaluate(diff)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0].Message, "10 changed lines")
}

func TestEngine_ScopeRemoved(t *testing.T) {
	rules := []model.Rule{
		{
			ID:       "removed-check",
			Severity: "info",
			Description: "Important code removed",
			Pattern:  `IMPORTANT`,
			Scope:    model.ScopeRemoved,
			Enabled:  true,
		},
	}

	engine, err := NewEngine(rules)
	require.NoError(t, err)

	diff := &model.DiffResult{
		Files: []model.FileDiff{
			{
				NewPath: "main.go",
				Hunks: []model.Hunk{
					{
						Lines: []model.Line{
							{Type: model.LineAdded, Content: "// IMPORTANT new", NewNum: 1},
							{Type: model.LineRemoved, Content: "// IMPORTANT old", OldNum: 1},
						},
					},
				},
			},
		},
	}

	findings := engine.Evaluate(diff)
	require.Len(t, findings, 1)
	assert.Equal(t, 1, findings[0].Line) // OldNum
}

func TestEngine_DisabledRule(t *testing.T) {
	rules := []model.Rule{
		{
			ID:       "disabled-rule",
			Severity: "warn",
			Pattern:  `.*`,
			Scope:    model.ScopeAdded,
			Enabled:  false, // disabled
		},
	}

	engine, err := NewEngine(rules)
	require.NoError(t, err)

	diff := &model.DiffResult{
		Files: []model.FileDiff{
			{
				NewPath: "main.go",
				Hunks: []model.Hunk{
					{Lines: []model.Line{{Type: model.LineAdded, Content: "anything", NewNum: 1}}},
				},
			},
		},
	}

	findings := engine.Evaluate(diff)
	assert.Len(t, findings, 0)
}
