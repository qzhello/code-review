package review

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/qzhello/code-review/internal/agent"
	"github.com/qzhello/code-review/internal/cache"
	"github.com/qzhello/code-review/internal/model"
	"github.com/qzhello/code-review/internal/rules"
)

// Engine orchestrates both rule-based and agent-based review.
type Engine struct {
	cfg        *model.Config
	prdContent string
}

// NewEngine creates a new review engine.
func NewEngine(cfg *model.Config, prdContent string) *Engine {
	return &Engine{cfg: cfg, prdContent: prdContent}
}

// Result holds the combined review output.
type Result struct {
	Findings []model.Finding
	Stats    Stats
}

// Stats aggregates review statistics.
type Stats struct {
	FilesReviewed int
	RuleFindings  int
	AgentFindings int
	Errors        int
	Warnings      int
	Infos         int
}

// Run executes the review based on config mode.
func (e *Engine) Run(ctx context.Context, diff *model.DiffResult, reviewMode string) (*Result, error) {
	var ruleFindings, agentFindings []model.Finding

	// Rule-based review (runs first so agent can see rule findings)
	if reviewMode == "hybrid" || reviewMode == "rules-only" {
		findings, err := e.runRules(diff)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: rule review failed: %s\n", err)
		} else {
			ruleFindings = findings
		}
	}

	// Agent review (uses the original ctx, not a derived one)
	if reviewMode == "hybrid" || reviewMode == "agent-only" {
		if e.cfg.Agent.Enabled {
			findings, err := e.runAgent(ctx, diff, ruleFindings)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: agent review failed: %s\n", err)
			} else {
				agentFindings = findings
			}
		}
	}

	// Merge and filter
	allFindings := append(ruleFindings, agentFindings...)
	allFindings = FilterFindings(allFindings, e.cfg.Noise)

	// Sort by file then line
	sort.Slice(allFindings, func(i, j int) bool {
		if allFindings[i].FilePath != allFindings[j].FilePath {
			return allFindings[i].FilePath < allFindings[j].FilePath
		}
		return allFindings[i].Line < allFindings[j].Line
	})

	// Compute stats
	stats := Stats{
		FilesReviewed: len(diff.Files),
		RuleFindings:  len(ruleFindings),
		AgentFindings: len(agentFindings),
	}
	for _, f := range allFindings {
		switch f.Severity {
		case model.SeverityError:
			stats.Errors++
		case model.SeverityWarn:
			stats.Warnings++
		case model.SeverityInfo:
			stats.Infos++
		}
	}

	return &Result{Findings: allFindings, Stats: stats}, nil
}

func (e *Engine) runRules(diff *model.DiffResult) ([]model.Finding, error) {
	disabledSet := make(map[string]bool)
	for _, id := range e.cfg.Rules.Disabled {
		disabledSet[id] = true
	}

	var allRules []model.Rule

	if e.cfg.Rules.Builtin {
		builtinRules, err := rules.LoadBuiltin(disabledSet)
		if err != nil {
			return nil, err
		}
		allRules = append(allRules, builtinRules...)
	}

	customRules, err := rules.LoadFromPaths(e.cfg.Rules.Paths, e.cfg.Rules.Disabled)
	if err != nil {
		return nil, err
	}
	allRules = append(allRules, customRules...)

	engine, err := rules.NewEngine(allRules)
	if err != nil {
		return nil, err
	}

	return engine.Evaluate(diff), nil
}

func (e *Engine) runAgent(ctx context.Context, diff *model.DiffResult, existingFindings []model.Finding) ([]model.Finding, error) {
	// Open cache if store is enabled
	var c *cache.Cache
	if e.cfg.Store.Enabled {
		var err error
		c, err = cache.New(e.cfg.Store.Path, 24*time.Hour)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to open cache: %s\n", err)
		} else {
			defer c.Close()
		}
	}

	reviewer, err := agent.NewReviewer(e.cfg.Agent, e.prdContent, c)
	if err != nil {
		return nil, err
	}
	return reviewer.Review(ctx, diff, existingFindings)
}
