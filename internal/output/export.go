package output

import (
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/qzhello/code-review/internal/model"
	"github.com/qzhello/code-review/internal/review"
)

// ExportMarkdown writes a clean Markdown report of the review results.
func ExportMarkdown(result *review.Result, diff *model.DiffResult, w io.Writer) error {
	// Title
	fmt.Fprintf(w, "# Code Review Report\n\n")
	fmt.Fprintf(w, "**Date:** %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

	// Summary table
	fmt.Fprintf(w, "## Summary\n\n")
	fmt.Fprintf(w, "| Metric | Count |\n")
	fmt.Fprintf(w, "|--------|-------|\n")
	fmt.Fprintf(w, "| Total findings | %d |\n", len(result.Findings))
	fmt.Fprintf(w, "| Errors | %d |\n", result.Stats.Errors)
	fmt.Fprintf(w, "| Warnings | %d |\n", result.Stats.Warnings)
	fmt.Fprintf(w, "| Infos | %d |\n", result.Stats.Infos)
	fmt.Fprintf(w, "\n")

	if len(result.Findings) == 0 {
		fmt.Fprintf(w, "No issues found.\n")
		return nil
	}

	// Group findings by file
	grouped := groupFindingsByFile(result.Findings)

	// Sort file names for deterministic output
	fileNames := make([]string, 0, len(grouped))
	for name := range grouped {
		fileNames = append(fileNames, name)
	}
	sort.Strings(fileNames)

	// Build a lookup from file path to FileDiff for diff context
	fileDiffMap := make(map[string]*model.FileDiff)
	if diff != nil {
		for i := range diff.Files {
			fileDiffMap[diff.Files[i].Path()] = &diff.Files[i]
		}
	}

	fmt.Fprintf(w, "## Findings\n\n")

	for _, fileName := range fileNames {
		findings := grouped[fileName]
		fmt.Fprintf(w, "### %s\n\n", fileName)

		for _, f := range findings {
			severityBadge := severityToBadge(f.Severity)
			fmt.Fprintf(w, "#### %s %s\n\n", severityBadge, f.Location())
			fmt.Fprintf(w, "**Message:** %s\n\n", f.Message)

			if f.Suggestion != "" {
				fmt.Fprintf(w, "**Suggestion:** %s\n\n", f.Suggestion)
			}

			// Inline diff context
			if fd, ok := fileDiffMap[f.FilePath]; ok {
				context := extractDiffContext(fd, f.Line, f.EndLine)
				if context != "" {
					fmt.Fprintf(w, "```diff\n%s```\n\n", context)
				}
			}
		}
	}

	return nil
}

// severityToBadge returns a Markdown badge string for the severity.
func severityToBadge(s model.Severity) string {
	switch s {
	case model.SeverityError:
		return "`ERROR`"
	case model.SeverityWarn:
		return "`WARN`"
	default:
		return "`INFO`"
	}
}

// groupFindingsByFile groups findings by their file path.
func groupFindingsByFile(findings []model.Finding) map[string][]model.Finding {
	grouped := make(map[string][]model.Finding)
	for _, f := range findings {
		key := f.FilePath
		if key == "" {
			key = "(unknown)"
		}
		grouped[key] = append(grouped[key], f)
	}
	return grouped
}

// extractDiffContext extracts the relevant diff hunk lines around the given line range.
func extractDiffContext(fd *model.FileDiff, line, endLine int) string {
	if line == 0 {
		return ""
	}
	if endLine == 0 {
		endLine = line
	}

	var result string
	for _, h := range fd.Hunks {
		for _, l := range h.Lines {
			lineNum := l.NewNum
			if l.Type == model.LineRemoved {
				lineNum = l.OldNum
			}
			if lineNum >= line && lineNum <= endLine {
				switch l.Type {
				case model.LineAdded:
					result += fmt.Sprintf("+%s\n", l.Content)
				case model.LineRemoved:
					result += fmt.Sprintf("-%s\n", l.Content)
				default:
					result += fmt.Sprintf(" %s\n", l.Content)
				}
			}
		}
	}
	return result
}
