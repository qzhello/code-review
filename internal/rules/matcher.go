package rules

import (
	"regexp"

	"github.com/bmatcuk/doublestar/v4"
)

// compiledRule is a rule with pre-compiled regex and glob.
type compiledRule struct {
	id          string
	severity    string
	description string
	fileGlob    string
	lineRegex   *regexp.Regexp
	scope       string // added | removed | all
	maxChanged  int    // structural: max changed lines (0 = no limit)
}

// compileRule compiles a rule's pattern into a regex.
func compileRule(id, severity, description, fileGlob, pattern, scope string, maxChanged int) (*compiledRule, error) {
	cr := &compiledRule{
		id:          id,
		severity:    severity,
		description: description,
		fileGlob:    fileGlob,
		scope:       scope,
		maxChanged:  maxChanged,
	}

	if pattern != "" {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
		cr.lineRegex = re
	}

	return cr, nil
}

// matchesFile checks if a file path matches the rule's glob pattern.
func (cr *compiledRule) matchesFile(path string) bool {
	if cr.fileGlob == "" {
		return true
	}
	ok, _ := doublestar.Match(cr.fileGlob, path)
	return ok
}

// matchesLine checks if a line content matches the rule's regex.
func (cr *compiledRule) matchesLine(content string) bool {
	if cr.lineRegex == nil {
		return false
	}
	return cr.lineRegex.MatchString(content)
}
