package output

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/qzhello/code-review/internal/review"
)

// JSONOutput represents the JSON output structure.
type JSONOutput struct {
	Findings []FindingJSON `json:"findings"`
	Stats    StatsJSON     `json:"stats"`
}

// FindingJSON is the JSON representation of a finding.
type FindingJSON struct {
	RuleID     string `json:"rule_id"`
	Severity   string `json:"severity"`
	Confidence string `json:"confidence,omitempty"`
	FilePath   string `json:"file"`
	Line       int    `json:"line"`
	EndLine    int    `json:"end_line,omitempty"`
	Message    string `json:"message"`
	Category   string `json:"category,omitempty"`
	Source     string `json:"source"`
	Suggestion string `json:"suggestion,omitempty"`
}

// StatsJSON is the JSON representation of review stats.
type StatsJSON struct {
	FilesReviewed int    `json:"files_reviewed"`
	RuleFindings  int    `json:"rule_findings"`
	AgentFindings int    `json:"agent_findings"`
	Errors        int    `json:"errors"`
	Warnings      int    `json:"warnings"`
	Infos         int    `json:"infos"`
	Duration      string `json:"duration"`
}

// PrintJSON outputs the review result as JSON.
func PrintJSON(result *review.Result, elapsed time.Duration) {
	output := JSONOutput{
		Stats: StatsJSON{
			FilesReviewed: result.Stats.FilesReviewed,
			RuleFindings:  result.Stats.RuleFindings,
			AgentFindings: result.Stats.AgentFindings,
			Errors:        result.Stats.Errors,
			Warnings:      result.Stats.Warnings,
			Infos:         result.Stats.Infos,
			Duration:      elapsed.Round(time.Millisecond).String(),
		},
	}

	for _, f := range result.Findings {
		fj := FindingJSON{
			RuleID:   f.RuleID,
			Severity: f.Severity.String(),
			FilePath: f.FilePath,
			Line:     f.Line,
			Message:  f.Message,
			Source:   f.Source,
		}
		if f.EndLine > 0 {
			fj.EndLine = f.EndLine
		}
		if f.Confidence > 0 {
			fj.Confidence = f.Confidence.String()
		}
		if f.Category != "" {
			fj.Category = f.Category
		}
		if f.Suggestion != "" {
			fj.Suggestion = f.Suggestion
		}
		output.Findings = append(output.Findings, fj)
	}

	if output.Findings == nil {
		output.Findings = []FindingJSON{}
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to marshal JSON: %s\n", err)
		return
	}
	fmt.Println(string(data))
}
