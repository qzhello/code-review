package model

import "time"

// Config is the top-level configuration.
type Config struct {
	Review ReviewConfig `yaml:"review" mapstructure:"review"`
	Rules  RulesConfig  `yaml:"rules" mapstructure:"rules"`
	Agent  AgentConfig  `yaml:"agent" mapstructure:"agent"`
	Noise  NoiseConfig  `yaml:"noise" mapstructure:"noise"`
	Output OutputConfig `yaml:"output" mapstructure:"output"`
	Store  StoreConfig  `yaml:"store" mapstructure:"store"`
}

// ReviewConfig controls what gets reviewed.
type ReviewConfig struct {
	Mode          string   `yaml:"mode" mapstructure:"mode"`                       // hybrid | rules-only | agent-only
	ContextLines  int      `yaml:"context_lines" mapstructure:"context_lines"`
	Include       []string `yaml:"include" mapstructure:"include"`
	Exclude       []string `yaml:"exclude" mapstructure:"exclude"`
	MaxFileSizeKB int      `yaml:"max_file_size_kb" mapstructure:"max_file_size_kb"`
}

// RulesConfig controls rule loading.
type RulesConfig struct {
	Paths    []string `yaml:"paths" mapstructure:"paths"`
	Builtin  bool     `yaml:"builtin" mapstructure:"builtin"`
	Disabled []string `yaml:"disabled" mapstructure:"disabled"`
}

// AgentConfig controls the LLM agent.
type AgentConfig struct {
	Enabled             bool          `yaml:"enabled" mapstructure:"enabled"`
	Provider            string        `yaml:"provider" mapstructure:"provider"`
	Model               string        `yaml:"model" mapstructure:"model"`
	APIKey              string        `yaml:"api_key" mapstructure:"api_key"`
	BaseURL             string        `yaml:"base_url" mapstructure:"base_url"`
	Temperature         float64       `yaml:"temperature" mapstructure:"temperature"`
	MaxConcurrency      int           `yaml:"max_concurrency" mapstructure:"max_concurrency"`
	Timeout             time.Duration `yaml:"timeout" mapstructure:"timeout"`
	ConfidenceThreshold string        `yaml:"confidence_threshold" mapstructure:"confidence_threshold"` // low | medium | high
	Focus               []string      `yaml:"focus" mapstructure:"focus"`
	Ignore              []string      `yaml:"ignore" mapstructure:"ignore"`
	Persona             string        `yaml:"persona" mapstructure:"persona"`
}

// NoiseConfig controls noise reduction.
type NoiseConfig struct {
	MinSeverity       string `yaml:"min_severity" mapstructure:"min_severity"`
	GroupThreshold    int    `yaml:"group_threshold" mapstructure:"group_threshold"`
	SuppressDismissed bool   `yaml:"suppress_dismissed" mapstructure:"suppress_dismissed"`
	Dedup             bool   `yaml:"dedup" mapstructure:"dedup"`
}

// OutputConfig controls output formatting.
type OutputConfig struct {
	Format      string `yaml:"format" mapstructure:"format"`           // terminal | json
	Color       string `yaml:"color" mapstructure:"color"`             // auto | always | never
	ShowContext bool   `yaml:"show_context" mapstructure:"show_context"`
	Verbose     bool   `yaml:"verbose" mapstructure:"verbose"`
}

// StoreConfig controls the SQLite backend.
type StoreConfig struct {
	Enabled bool   `yaml:"enabled" mapstructure:"enabled"`
	Path    string `yaml:"path" mapstructure:"path"`
}
