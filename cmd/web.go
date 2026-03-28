package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/qzhello/code-review/internal/config"
	"github.com/qzhello/code-review/internal/git"
	"github.com/qzhello/code-review/internal/model"
	"github.com/qzhello/code-review/internal/output"
	"github.com/qzhello/code-review/internal/review"
	"github.com/qzhello/code-review/internal/web"
)

var (
	webPort   int
	webNoOpen bool
)

var webCmd = &cobra.Command{
	Use:   "web [paths...]",
	Short: "Launch web-based code review UI",
	Long: `Run a code review and open the results in a web browser.

The web UI provides an interactive interface to browse findings,
chat with AI, and apply fixes — all from your browser.

Examples:
  cr web                          review all uncommitted changes
  cr web --staged                 review staged changes only
  cr web --branch main            compare current branch to main
  cr web --port 3000              use a specific port`,
	RunE: runWeb,
}

func init() {
	webCmd.Flags().BoolVar(&staged, "staged", false, "review staged changes only")
	webCmd.Flags().StringVar(&branch, "branch", "", "compare current branch to this base branch")
	webCmd.Flags().StringVar(&commit, "commit", "", "commit range")
	webCmd.Flags().StringVar(&mode, "mode", "", "review mode: hybrid, rules-only, agent-only")
	webCmd.Flags().StringVar(&minSeverity, "min-severity", "", "minimum severity")
	webCmd.Flags().StringVar(&focus, "focus", "", "override agent focus area")
	webCmd.Flags().StringVar(&prdFile, "prd", "", "path to PRD/requirements document")
	webCmd.Flags().StringVar(&lang, "lang", "", "output language for agent findings")
	webCmd.Flags().IntVar(&webPort, "port", 0, "port to listen on (default: auto)")
	webCmd.Flags().BoolVar(&webNoOpen, "no-open", false, "don't auto-open browser")
}

func runWeb(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

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
	if minSeverity != "" {
		cfg.Noise.MinSeverity = minSeverity
	}
	if focus != "" {
		cfg.Agent.Focus = []string{focus}
	}
	if lang != "" {
		cfg.Agent.Language = lang
	}

	// Load PRD
	var prdContent string
	if prdFile != "" {
		data, err := os.ReadFile(prdFile)
		if err != nil {
			return fmt.Errorf("failed to read PRD file %s: %w", prdFile, err)
		}
		prdContent = string(data)
	}

	// Get diff
	diffOpts := git.DiffOptions{
		Staged:       staged,
		Branch:       branch,
		Commit:       commit,
		ContextLines: cfg.Review.ContextLines,
		Paths:        args,
	}

	raw, err := git.ExecDiff(ctx, diffOpts)
	if err != nil {
		return fmt.Errorf("failed to get diff: %w", err)
	}

	if raw == "" {
		fmt.Println("No changes to review.")
		return nil
	}

	diff := git.ParseDiff(raw)
	diff = git.FilterFiles(diff, cfg.Review.Include, cfg.Review.Exclude)

	if len(diff.Files) == 0 {
		fmt.Println("No files to review after filtering.")
		return nil
	}

	// Print diff summary
	term := output.NewTerminal(cfg.Output.ShowContext, cfg.Output.Verbose || verbose)
	term.PrintDiffSummary(diff)

	// Run review
	engine := review.NewEngine(cfg, prdContent)

	if reviewMode == "hybrid" || reviewMode == "agent-only" {
		if cfg.Agent.Enabled {
			progress := output.NewProgress(len(diff.Files), true)
			engine.SetTotalCallback(func(total int) {
				progress.SetTotal(total)
			})
			engine.SetProgress(func(fileName string, done bool, cached bool) {
				if done {
					progress.Complete(fileName, cached)
				} else {
					progress.Start(fileName)
				}
			})
			defer progress.Finish()
		}
	}

	result, err := engine.Run(ctx, diff, reviewMode)
	if err != nil {
		return fmt.Errorf("review failed: %w", err)
	}

	if len(result.Findings) == 0 {
		fmt.Println("No issues found!")
		return nil
	}

	fmt.Printf("\n  Found %d issue(s). Starting web UI...\n\n", len(result.Findings))

	// Prepare agent config
	var agentCfg *model.AgentConfig
	if cfg.Agent.Enabled && cfg.Agent.APIKey != "" {
		agentCfg = &cfg.Agent
	}

	// Start web server
	srv := web.NewServer(result, diff, agentCfg)
	return srv.Start(webPort, !webNoOpen)
}
