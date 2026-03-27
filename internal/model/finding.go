package model

import "fmt"

// Severity represents the severity level of a finding.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarn
	SeverityError
)

func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarn:
		return "warn"
	case SeverityError:
		return "error"
	default:
		return "unknown"
	}
}

// ParseSeverity converts a string to Severity.
func ParseSeverity(s string) (Severity, error) {
	switch s {
	case "info":
		return SeverityInfo, nil
	case "warn", "warning":
		return SeverityWarn, nil
	case "error":
		return SeverityError, nil
	default:
		return SeverityInfo, fmt.Errorf("unknown severity: %s", s)
	}
}

// Confidence represents the confidence level of an agent finding.
type Confidence int

const (
	ConfidenceLow Confidence = iota
	ConfidenceMedium
	ConfidenceHigh
)

func (c Confidence) String() string {
	switch c {
	case ConfidenceLow:
		return "low"
	case ConfidenceMedium:
		return "medium"
	case ConfidenceHigh:
		return "high"
	default:
		return "unknown"
	}
}

// ParseConfidence converts a string to Confidence.
func ParseConfidence(s string) Confidence {
	switch s {
	case "high":
		return ConfidenceHigh
	case "medium":
		return ConfidenceMedium
	default:
		return ConfidenceLow
	}
}

// Finding represents a single review finding.
type Finding struct {
	RuleID     string     `json:"rule_id"`
	Severity   Severity   `json:"severity"`
	Confidence Confidence `json:"confidence"`
	FilePath   string     `json:"file_path"`
	Line       int        `json:"line"`       // 0 = file-level finding
	EndLine    int        `json:"end_line"`   // for ranges; 0 = single line
	Message    string     `json:"message"`
	Category   string     `json:"category"`   // e.g., "security", "performance"
	Source     string     `json:"source"`     // "rule" or "agent"
	Suggestion string     `json:"suggestion"` // optional fix suggestion
}

// Location returns a formatted file:line string.
func (f *Finding) Location() string {
	if f.Line == 0 {
		return f.FilePath
	}
	if f.EndLine > 0 && f.EndLine != f.Line {
		return fmt.Sprintf("%s:%d-%d", f.FilePath, f.Line, f.EndLine)
	}
	return fmt.Sprintf("%s:%d", f.FilePath, f.Line)
}
