package rules

import (
	"embed"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/qzhello/code-review/internal/model"
)

//go:embed builtin/*.yaml builtin/*.json
var builtinFS embed.FS

// LoadBuiltin loads all embedded built-in rules (YAML and JSON).
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

		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			continue
		}

		data, err := builtinFS.ReadFile("builtin/" + name)
		if err != nil {
			return nil, fmt.Errorf("failed to read built-in rule %s: %w", name, err)
		}

		var rf model.RuleFile
		if ext == ".json" {
			if err := json.Unmarshal(data, &rf); err != nil {
				return nil, fmt.Errorf("failed to parse built-in rule %s: %w", name, err)
			}
		} else {
			if err := yaml.Unmarshal(data, &rf); err != nil {
				return nil, fmt.Errorf("failed to parse built-in rule %s: %w", name, err)
			}
		}

		for i := range rf.Rules {
			rf.Rules[i].Source = "builtin/" + name
			rf.Rules[i].Enabled = !disabled[rf.Rules[i].ID]
			if rf.Rules[i].Scope == "" {
				rf.Rules[i].Scope = model.ScopeAdded
			}
		}

		allRules = append(allRules, rf.Rules...)
	}

	return allRules, nil
}
