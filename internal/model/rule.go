package model

// Scope defines which diff lines a rule applies to.
type Scope string

const (
	ScopeAdded   Scope = "added"
	ScopeRemoved Scope = "removed"
	ScopeAll     Scope = "all"
)

// Rule defines a single review rule loaded from YAML.
type Rule struct {
	ID              string   `yaml:"id"`
	Severity        string   `yaml:"severity"`
	Description     string   `yaml:"description"`
	File            string   `yaml:"file,omitempty"`    // glob pattern
	Pattern         string   `yaml:"pattern,omitempty"` // regex
	Scope           Scope    `yaml:"scope,omitempty"`   // added | removed | all (default: added)
	MaxChangedLines int      `yaml:"max_changed_lines,omitempty"` // structural rule
	Enabled         bool     `yaml:"-"` // resolved at runtime
	Source          string   `yaml:"-"` // file path where rule was loaded from
}

// RuleFile represents a YAML file containing rules.
type RuleFile struct {
	Rules []Rule `yaml:"rules"`
}
