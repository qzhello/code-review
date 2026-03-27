package git

import (
	"github.com/bmatcuk/doublestar/v4"
	"github.com/quzhihao/code-review/internal/model"
)

// FilterFiles filters a DiffResult based on include/exclude glob patterns.
// If include is non-empty, only files matching at least one include pattern are kept.
// Files matching any exclude pattern are removed.
func FilterFiles(diff *model.DiffResult, include, exclude []string) *model.DiffResult {
	if len(include) == 0 && len(exclude) == 0 {
		return diff
	}

	filtered := &model.DiffResult{}
	for _, f := range diff.Files {
		path := f.Path()
		if !matchesAny(path, include, true) {
			continue
		}
		if matchesAny(path, exclude, false) {
			continue
		}
		filtered.Files = append(filtered.Files, f)
	}
	return filtered
}

// matchesAny returns true if path matches any of the given patterns.
// If patterns is empty, returns defaultVal.
func matchesAny(path string, patterns []string, defaultVal bool) bool {
	if len(patterns) == 0 {
		return defaultVal
	}
	for _, p := range patterns {
		if ok, _ := doublestar.Match(p, path); ok {
			return true
		}
	}
	return false
}
