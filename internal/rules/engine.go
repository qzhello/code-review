package rules

import (
	"fmt"

	"github.com/quzhihao/code-review/internal/model"
)

// Engine evaluates rules against a diff result.
type Engine struct {
	rules []*compiledRule
}

// NewEngine creates a rule engine from loaded rules.
func NewEngine(rules []model.Rule) (*Engine, error) {
	var compiled []*compiledRule

	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		cr, err := compileRule(r.ID, r.Severity, r.Description, r.File, r.Pattern, string(r.Scope), r.MaxChangedLines)
		if err != nil {
			return nil, fmt.Errorf("failed to compile rule %q: %w", r.ID, err)
		}
		compiled = append(compiled, cr)
	}

	return &Engine{rules: compiled}, nil
}

// Evaluate runs all rules against the diff and returns findings.
func (e *Engine) Evaluate(diff *model.DiffResult) []model.Finding {
	var findings []model.Finding

	for _, rule := range e.rules {
		// Structural rules (file-level)
		if rule.maxChanged > 0 {
			findings = append(findings, e.evalStructural(rule, diff)...)
			continue
		}

		// Pattern rules (line-level)
		if rule.lineRegex != nil {
			findings = append(findings, e.evalPattern(rule, diff)...)
			continue
		}
	}

	return findings
}

func (e *Engine) evalStructural(rule *compiledRule, diff *model.DiffResult) []model.Finding {
	var findings []model.Finding

	for _, f := range diff.Files {
		if !rule.matchesFile(f.Path()) {
			continue
		}
		changed := f.ChangedLines()
		if rule.maxChanged > 0 && changed > rule.maxChanged {
			sev, _ := model.ParseSeverity(rule.severity)
			findings = append(findings, model.Finding{
				RuleID:   rule.id,
				Severity: sev,
				FilePath: f.Path(),
				Message:  fmt.Sprintf("%s (%d changed lines, threshold: %d)", rule.description, changed, rule.maxChanged),
				Source:   "rule",
			})
		}
	}

	return findings
}

func (e *Engine) evalPattern(rule *compiledRule, diff *model.DiffResult) []model.Finding {
	var findings []model.Finding

	for _, f := range diff.Files {
		if f.IsBinary {
			continue
		}
		if !rule.matchesFile(f.Path()) {
			continue
		}

		for _, hunk := range f.Hunks {
			for _, line := range hunk.Lines {
				if !lineMatchesScope(line, rule.scope) {
					continue
				}

				if rule.matchesLine(line.Content) {
					lineNum := line.NewNum
					if lineNum == 0 {
						lineNum = line.OldNum
					}

					sev, _ := model.ParseSeverity(rule.severity)
					findings = append(findings, model.Finding{
						RuleID:   rule.id,
						Severity: sev,
						FilePath: f.Path(),
						Line:     lineNum,
						Message:  rule.description,
						Source:   "rule",
					})
				}
			}
		}
	}

	return findings
}

func lineMatchesScope(line model.Line, scope string) bool {
	switch scope {
	case "added":
		return line.Type == model.LineAdded
	case "removed":
		return line.Type == model.LineRemoved
	case "all":
		return line.Type == model.LineAdded || line.Type == model.LineRemoved
	default:
		return line.Type == model.LineAdded
	}
}
