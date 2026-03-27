package config

import (
	"time"

	"github.com/qzhello/code-review/internal/model"
)

// DefaultConfig returns the default configuration.
func DefaultConfig() *model.Config {
	return &model.Config{
		Review: model.ReviewConfig{
			Mode:          "hybrid",
			ContextLines:  3,
			Include:       []string{},
			Exclude:       []string{"vendor/**", "node_modules/**", "*.generated.go", "*.pb.go"},
			MaxFileSizeKB: 200,
		},
		Rules: model.RulesConfig{
			Paths:    []string{".cr/rules/"},
			Builtin:  true,
			Disabled: []string{},
		},
		Agent: model.AgentConfig{
			Enabled:             true,
			Provider:            "openai",
			Model:               "gpt-4o",
			Temperature:         0.1,
			MaxConcurrency:      5,
			Timeout:             2 * time.Minute,
			ConfidenceThreshold: "medium",
			Focus:               []string{"logic_errors", "security", "performance", "error_handling"},
			Ignore:              []string{"style", "naming"},
			Persona: `You are a senior software engineer performing code review.
Focus on bugs, security issues, and performance problems.
Do NOT comment on code style or formatting.
Be concise. Only flag real issues with clear explanations.`,
		},
		Noise: model.NoiseConfig{
			MinSeverity:       "info",
			GroupThreshold:    3,
			SuppressDismissed: true,
			Dedup:             true,
		},
		Output: model.OutputConfig{
			Format:      "terminal",
			Color:       "auto",
			ShowContext: true,
			Verbose:     false,
		},
		Store: model.StoreConfig{
			Enabled: true,
			Path:    "~/.cr/cr.db",
		},
	}
}

// DefaultConfigYAML returns the default config as YAML string for cr init.
const DefaultConfigYAML = `# cr — Code Review CLI Configuration
# Docs: cr config list

review:
  mode: hybrid              # hybrid | rules-only | agent-only
  context_lines: 3          # lines of context in diff
  include: []               # file globs to include (empty = all)
  exclude:                  # file globs to exclude
    - "vendor/**"
    - "node_modules/**"
    - "*.generated.go"
    - "*.pb.go"
  max_file_size_kb: 200     # skip files larger than this

rules:
  paths:
    - .cr/rules/
  builtin: true             # load built-in rules
  disabled: []              # rule IDs to disable

agent:
  enabled: true
  provider: openai
  model: gpt-4o
  # api_key: ${OPENAI_API_KEY}   # set via env var
  base_url: ""              # custom endpoint / proxy
  temperature: 0.1
  max_concurrency: 5
  confidence_threshold: medium  # low | medium | high
  focus:
    - logic_errors
    - security
    - performance
    - error_handling
  ignore:
    - style
    - naming
  persona: |
    You are a senior software engineer performing code review.
    Focus on bugs, security issues, and performance problems.
    Do NOT comment on code style or formatting.
    Be concise. Only flag real issues with clear explanations.

noise:
  min_severity: info        # info | warn | error
  group_threshold: 3        # group findings if >N of same rule in one file
  suppress_dismissed: true  # use history to suppress dismissed findings
  dedup: true               # deduplicate adjacent identical findings

output:
  format: terminal          # terminal | json
  color: auto               # auto | always | never
  show_context: true        # show code context around findings
  verbose: false

store:
  enabled: true
  path: ~/.cr/cr.db
`
