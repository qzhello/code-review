package output

import (
	"fmt"
	"time"

	"github.com/fatih/color"

	"github.com/qzhello/code-review/internal/model"
	"github.com/qzhello/code-review/internal/review"
)

// Terminal renders review results to the terminal with colors.
type Terminal struct {
	showContext bool
	verbose     bool
}

// NewTerminal creates a new terminal output renderer.
func NewTerminal(showContext, verbose bool) *Terminal {
	return &Terminal{showContext: showContext, verbose: verbose}
}

// PrintDiffSummary prints the diff summary header.
func (t *Terminal) PrintDiffSummary(diff *model.DiffResult) {
	bold := color.New(color.Bold)
	green := color.New(color.FgGreen)
	red := color.New(color.FgRed)
	cyan := color.New(color.FgCyan)

	bold.Printf("\nDiff Summary\n")
	fmt.Printf("  Files: %d | Changed lines: %d\n\n",
		len(diff.Files), diff.TotalChangedLines())

	for _, f := range diff.Files {
		statusColor := cyan
		switch f.Status {
		case model.FileAdded:
			statusColor = green
		case model.FileDeleted:
			statusColor = red
		}

		added, removed := countChanges(&f)

		statusColor.Printf("  %-10s", f.Status.String())
		fmt.Printf(" %s", f.Path())
		if added > 0 || removed > 0 {
			fmt.Printf("  ")
			if added > 0 {
				green.Printf("+%d", added)
			}
			if removed > 0 {
				if added > 0 {
					fmt.Printf(" ")
				}
				red.Printf("-%d", removed)
			}
		}
		fmt.Println()
	}
	fmt.Println()
}

// PrintResult prints the full review result.
func (t *Terminal) PrintResult(result *review.Result, elapsed time.Duration) {
	bold := color.New(color.Bold)
	errColor := color.New(color.FgRed, color.Bold)
	warnColor := color.New(color.FgYellow, color.Bold)
	infoColor := color.New(color.FgCyan, color.Bold)
	green := color.New(color.FgGreen, color.Bold)

	if len(result.Findings) == 0 {
		green.Println("No issues found!")
		t.printStats(result.Stats, elapsed)
		return
	}

	bold.Printf("Review Findings (%d)\n\n", len(result.Findings))

	for _, f := range result.Findings {
		var sevColor *color.Color
		var sevLabel string
		switch f.Severity {
		case model.SeverityError:
			sevColor = errColor
			sevLabel = "ERROR"
		case model.SeverityWarn:
			sevColor = warnColor
			sevLabel = "WARN "
		default:
			sevColor = infoColor
			sevLabel = "INFO "
		}

		sevColor.Printf("  %s", sevLabel)
		fmt.Printf("  %s", f.Location())

		source := f.Source
		if f.RuleID != "" && f.RuleID != "agent" {
			source = f.RuleID
		}
		fmt.Printf("  [%s]", source)
		fmt.Printf("\n         %s\n", f.Message)

		if f.Suggestion != "" {
			color.New(color.FgGreen).Printf("         Suggestion: %s\n", f.Suggestion)
		}
	}
	fmt.Println()

	// Summary line
	bold.Printf("Summary: ")
	if result.Stats.Errors > 0 {
		errColor.Printf("%d errors  ", result.Stats.Errors)
	}
	if result.Stats.Warnings > 0 {
		warnColor.Printf("%d warnings  ", result.Stats.Warnings)
	}
	if result.Stats.Infos > 0 {
		infoColor.Printf("%d info", result.Stats.Infos)
	}
	fmt.Println()

	t.printStats(result.Stats, elapsed)
}

func (t *Terminal) printStats(stats review.Stats, elapsed time.Duration) {
	if t.verbose {
		fmt.Printf("  Files reviewed: %d | Rule findings: %d | Agent findings: %d\n",
			stats.FilesReviewed, stats.RuleFindings, stats.AgentFindings)
	}
	fmt.Printf("  Completed in %s\n\n", elapsed.Round(time.Millisecond))
}

func countChanges(f *model.FileDiff) (added, removed int) {
	for _, h := range f.Hunks {
		for _, l := range h.Lines {
			switch l.Type {
			case model.LineAdded:
				added++
			case model.LineRemoved:
				removed++
			}
		}
	}
	return
}
