package rules

import (
	"embed"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/quzhihao/code-review/internal/model"
)

//go:embed builtin/*.yaml
var builtinFS embed.FS

// LoadBuiltin loads all embedded built-in rules.
func LoadBuiltin(disabled map[string]bool) ([]model.Rule, error) {
	entries, err := builtinFS.ReadDir("builtin")
	if err != nil {
		return nil, fmt.Errorf("failed to read built-in rules: %w", err)
	}

	var allRules []model.Rule

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := builtinFS.ReadFile("builtin/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to read built-in rule %s: %w", entry.Name(), err)
		}

		var rf model.RuleFile
		if err := yaml.Unmarshal(data, &rf); err != nil {
			return nil, fmt.Errorf("failed to parse built-in rule %s: %w", entry.Name(), err)
		}

		for i := range rf.Rules {
			rf.Rules[i].Source = "builtin/" + entry.Name()
			rf.Rules[i].Enabled = !disabled[rf.Rules[i].ID]
			if rf.Rules[i].Scope == "" {
				rf.Rules[i].Scope = model.ScopeAdded
			}
		}

		allRules = append(allRules, rf.Rules...)
	}

	return allRules, nil
}
