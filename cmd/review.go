package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/quzhihao/code-review/internal/config"
	"github.com/quzhihao/code-review/internal/git"
	"github.com/quzhihao/code-review/internal/model"
	"github.com/quzhihao/code-review/internal/rules"
)

var (
	staged      bool
	branch      string
	mode        string
	minSeverity string
	jsonOutput  bool
	focus       string
)

var reviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Review code changes",
	Long: `Review uncommitted code changes using rules, AI agent, or both.

Examples:
  cr review                     # review all uncommitted changes
  cr review --staged            # review staged changes only
  cr review --branch main       # compare current branch to main
  cr review --mode rules-only   # skip AI agent, rules only
  cr review --json              # output as JSON (for CI)`,
	RunE: runReview,
}

func init() {
	reviewCmd.Flags().BoolVar(&staged, "staged", false, "review staged changes only")
	reviewCmd.Flags().StringVar(&branch, "branch", "", "compare current branch to this base branch")
	reviewCmd.Flags().StringVar(&mode, "mode", "", "review mode: hybrid, rules-only, agent-only (overrides config)")
	reviewCmd.Flags().StringVar(&minSeverity, "min-severity", "", "minimum severity: info, warn, error (overrides config)")
	reviewCmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	reviewCmd.Flags().StringVar(&focus, "focus", "", "override agent focus area (e.g., security)")
}

func runReview(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	start := time.Now()

	// Load config
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Apply flag overrides
	reviewMode := cfg.Review.Mode
	if mode != "" {
		reviewMode = mode
	}
	minSev := cfg.Noise.MinSeverity
	if minSeverity != "" {
		minSev = minSeverity
	}

	// Step 1: Get diff
	diffOpts := git.DiffOptions{
		Staged:       staged,
		Branch:       branch,
		ContextLines: cfg.Review.ContextLines,
	}

	raw, err := git.ExecDiff(ctx, diffOpts)
	if err != nil {
		return fmt.Errorf("failed to get diff: %w", err)
	}

	if raw == "" {
		fmt.Println("No changes to review.")
		return nil
	}

	// Step 2: Parse + filter diff
	diff := git.ParseDiff(raw)
	diff = git.FilterFiles(diff, cfg.Review.Include, cfg.Review.Exclude)

	if len(diff.Files) == 0 {
		fmt.Println("No files to review after filtering.")
		return nil
	}

	// Print diff summary
	printDiffSummary(diff)

	// Step 3: Run reviews based on mode
	var findings []model.Finding

	if reviewMode == "hybrid" || reviewMode == "rules-only" {
		ruleFindings, err := runRuleReview(cfg, diff)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: rule review failed: %s\n", err)
		} else {
			findings = append(findings, ruleFindings...)
		}
	}

	if reviewMode == "hybrid" || reviewMode == "agent-only" {
		// TODO: Phase 4 — agent review
		if cfg.Agent.Enabled && reviewMode != "rules-only" {
			if verbose {
				fmt.Println("Agent review: not yet implemented (Phase 4)")
			}
		}
	}

	// Step 4: Filter by severity
	findings = filterBySeverity(findings, minSev)

	// Step 5: Sort findings
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].FilePath != findings[j].FilePath {
			return findings[i].FilePath < findings[j].FilePath
		}
		return findings[i].Line < findings[j].Line
	})

	// Step 6: Output
	if jsonOutput {
		printFindingsJSON(findings)
	} else {
		printFindings(findings, time.Since(start))
	}

	// Exit code 1 if any errors
	for _, f := range findings {
		if f.Severity == model.SeverityError {
			os.Exit(1)
		}
	}

	return nil
}

func runRuleReview(cfg *model.Config, diff *model.DiffResult) ([]model.Finding, error) {
	disabledSet := make(map[string]bool)
	for _, id := range cfg.Rules.Disabled {
		disabledSet[id] = true
	}

	var allRules []model.Rule

	// Load built-in rules
	if cfg.Rules.Builtin {
		builtinRules, err := rules.LoadBuiltin(disabledSet)
		if err != nil {
			return nil, fmt.Errorf("failed to load built-in rules: %w", err)
		}
		allRules = append(allRules, builtinRules...)
	}

	// Load custom rules
	customRules, err := rules.LoadFromPaths(cfg.Rules.Paths, cfg.Rules.Disabled)
	if err != nil {
		return nil, fmt.Errorf("failed to load custom rules: %w", err)
	}
	allRules = append(allRules, customRules...)

	// Create and run engine
	engine, err := rules.NewEngine(allRules)
	if err != nil {
		return nil, err
	}

	return engine.Evaluate(diff), nil
}

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

func printDiffSummary(diff *model.DiffResult) {
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

		added, removed := 0, 0
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

func printFindings(findings []model.Finding, elapsed time.Duration) {
	bold := color.New(color.Bold)
	red := color.New(color.FgRed, color.Bold)
	yellow := color.New(color.FgYellow, color.Bold)
	cyan := color.New(color.FgCyan, color.Bold)

	if len(findings) == 0 {
		green := color.New(color.FgGreen, color.Bold)
		green.Println("No issues found!")
		fmt.Printf("  Completed in %s\n\n", elapsed.Round(time.Millisecond))
		return
	}

	bold.Printf("Review Findings (%d)\n\n", len(findings))

	errors, warnings, infos := 0, 0, 0
	for _, f := range findings {
		var sevColor *color.Color
		var sevLabel string
		switch f.Severity {
		case model.SeverityError:
			sevColor = red
			sevLabel = "ERROR"
			errors++
		case model.SeverityWarn:
			sevColor = yellow
			sevLabel = "WARN "
			warnings++
		default:
			sevColor = cyan
			sevLabel = "INFO "
			infos++
		}

		sevColor.Printf("  %s", sevLabel)
		fmt.Printf("  %s", f.Location())
		fmt.Printf("  [%s]", f.RuleID)
		fmt.Printf("\n         %s\n", f.Message)
	}

	fmt.Println()
	bold.Printf("Summary: ")
	if errors > 0 {
		red.Printf("%d errors  ", errors)
	}
	if warnings > 0 {
		yellow.Printf("%d warnings  ", warnings)
	}
	if infos > 0 {
		cyan.Printf("%d info", infos)
	}
	fmt.Printf("\n  Completed in %s\n\n", elapsed.Round(time.Millisecond))
}

func printFindingsJSON(findings []model.Finding) {
	fmt.Print("[")
	for i, f := range findings {
		if i > 0 {
			fmt.Print(",")
		}
		fmt.Printf("\n  {\"rule_id\":%q,\"severity\":%q,\"file\":%q,\"line\":%d,\"message\":%q,\"source\":%q}",
			f.RuleID, f.Severity.String(), f.FilePath, f.Line, f.Message, f.Source)
	}
	if len(findings) > 0 {
		fmt.Println()
	}
	fmt.Println("]")
}
