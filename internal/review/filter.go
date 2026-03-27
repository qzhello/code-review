package review

import (
	"crypto/sha256"
	"fmt"

	"github.com/quzhihao/code-review/internal/model"
)

// FilterFindings applies noise reduction to a list of findings.
func FilterFindings(findings []model.Finding, cfg model.NoiseConfig) []model.Finding {
	findings = filterBySeverity(findings, cfg.MinSeverity)
	if cfg.Dedup {
		findings = dedup(findings)
	}
	if cfg.GroupThreshold > 0 {
		findings = groupFindings(findings, cfg.GroupThreshold)
	}
	return findings
}

// filterBySeverity removes findings below the minimum severity.
func filterBySeverity(findings []model.Finding, minSev string) []model.Finding {
	threshold, err := model.ParseSeverity(minSev)
	if err != nil {
		return findings
	}

	var filtered []model.Finding
	for _, f := range findings {
		if f.Severity >= threshold {
			filtered = append(filtered, f)
		}
	}
	return filtered
}

// dedup removes exact duplicate findings (same rule, file, line, message).
func dedup(findings []model.Finding) []model.Finding {
	seen := make(map[string]bool)
	var unique []model.Finding

	for _, f := range findings {
		key := findingKey(f)
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, f)
	}

	return unique
}

// groupFindings collapses repeated findings of the same rule in the same file.
// If a rule fires more than `threshold` times in one file, they are merged into
// a single grouped finding.
func groupFindings(findings []model.Finding, threshold int) []model.Finding {
	// Count occurrences per (rule, file)
	type key struct{ ruleID, filePath string }
	counts := make(map[key][]int) // map to indices

	for i, f := range findings {
		k := key{f.RuleID, f.FilePath}
		counts[k] = append(counts[k], i)
	}

	// Build grouped result
	grouped := make(map[int]bool) // indices that got grouped
	var result []model.Finding

	for k, indices := range counts {
		if len(indices) <= threshold {
			continue
		}
		// Group these into one finding
		first := findings[indices[0]]
		minLine := first.Line
		maxLine := first.Line
		for _, idx := range indices {
			if findings[idx].Line < minLine {
				minLine = findings[idx].Line
			}
			if findings[idx].Line > maxLine {
				maxLine = findings[idx].Line
			}
			grouped[idx] = true
		}

		result = append(result, model.Finding{
			RuleID:   k.ruleID,
			Severity: first.Severity,
			FilePath: k.filePath,
			Line:     minLine,
			EndLine:  maxLine,
			Message:  fmt.Sprintf("%s (%d occurrences)", first.Message, len(indices)),
			Source:   first.Source,
		})
	}

	// Add non-grouped findings
	for i, f := range findings {
		if !grouped[i] {
			result = append(result, f)
		}
	}

	return result
}

func findingKey(f model.Finding) string {
	raw := fmt.Sprintf("%s|%s|%d|%s", f.RuleID, f.FilePath, f.Line, f.Message)
	hash := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", hash[:8])
}

// ContentHash returns a hash of a finding's content (for dismissal tracking).
func ContentHash(f model.Finding) string {
	raw := fmt.Sprintf("%s|%s|%s", f.RuleID, f.FilePath, f.Message)
	hash := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", hash[:16])
}
