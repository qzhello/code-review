package model

// Scope defines which diff lines a rule applies to.
type Scope string

const (
	ScopeAdded   Scope = "added"
	ScopeRemoved Scope = "removed"
	ScopeAll     Scope = "all"
)

// Rule defines a single review rule loaded from YAML or JSON.
type Rule struct {
	ID              string `yaml:"id" json:"id"`
	Severity        string `yaml:"severity" json:"severity"`
	Description     string `yaml:"description" json:"description"`
	File            string `yaml:"file,omitempty" json:"file,omitempty"`                           // glob pattern
	Pattern         string `yaml:"pattern,omitempty" json:"pattern,omitempty"`                     // regex
	Scope           Scope  `yaml:"scope,omitempty" json:"scope,omitempty"`                         // added | removed | all (default: added)
	MaxChangedLines int    `yaml:"max_changed_lines,omitempty" json:"max_changed_lines,omitempty"` // structural rule
	Enabled         bool   `yaml:"-" json:"-"`                                                     // resolved at runtime
	Source          string `yaml:"-" json:"-"`                                                     // file path where rule was loaded from
}

// RuleFile represents a file containing rules (YAML or JSON).
type RuleFile struct {
	Rules []Rule `yaml:"rules" json:"rules"`
}
