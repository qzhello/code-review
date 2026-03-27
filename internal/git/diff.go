package git

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/qzhello/code-review/internal/model"
)

var (
	diffHeaderRe = regexp.MustCompile(`^diff --git a/(.+) b/(.+)$`)
	hunkHeaderRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)$`)
	oldFileRe    = regexp.MustCompile(`^--- (?:a/(.+)|/dev/null)$`)
	newFileRe    = regexp.MustCompile(`^\+\+\+ (?:b/(.+)|/dev/null)$`)
)

// ParseDiff parses unified diff output into a DiffResult.
func ParseDiff(raw string) *model.DiffResult {
	result := &model.DiffResult{}
	if raw == "" {
		return result
	}

	lines := strings.Split(raw, "\n")
	var currentFile *model.FileDiff
	var currentHunk *model.Hunk
	oldNum, newNum := 0, 0

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// New file diff header
		if m := diffHeaderRe.FindStringSubmatch(line); m != nil {
			if currentFile != nil {
				if currentHunk != nil {
					currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
					currentHunk = nil
				}
				result.Files = append(result.Files, *currentFile)
			}
			currentFile = &model.FileDiff{
				OldPath: m[1],
				NewPath: m[2],
				Status:  model.FileModified,
			}
			continue
		}

		if currentFile == nil {
			continue
		}

		// Binary file
		if strings.HasPrefix(line, "Binary files") {
			currentFile.IsBinary = true
			continue
		}

		// Old file path
		if m := oldFileRe.FindStringSubmatch(line); m != nil {
			if m[1] == "" {
				currentFile.Status = model.FileAdded
				currentFile.OldPath = ""
			}
			continue
		}

		// New file path
		if m := newFileRe.FindStringSubmatch(line); m != nil {
			if m[1] == "" {
				currentFile.Status = model.FileDeleted
				currentFile.NewPath = ""
			} else if currentFile.OldPath != m[1] && currentFile.OldPath != "" {
				currentFile.Status = model.FileRenamed
			}
			continue
		}

		// Hunk header
		if m := hunkHeaderRe.FindStringSubmatch(line); m != nil {
			if currentHunk != nil {
				currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
			}
			oldStart := atoi(m[1])
			oldCount := 1
			if m[2] != "" {
				oldCount = atoi(m[2])
			}
			newStart := atoi(m[3])
			newCount := 1
			if m[4] != "" {
				newCount = atoi(m[4])
			}
			currentHunk = &model.Hunk{
				OldStart: oldStart,
				OldCount: oldCount,
				NewStart: newStart,
				NewCount: newCount,
				Header:   strings.TrimSpace(m[5]),
			}
			oldNum = oldStart
			newNum = newStart
			continue
		}

		// Diff lines (only inside a hunk)
		if currentHunk == nil {
			continue
		}

		if strings.HasPrefix(line, "+") {
			currentHunk.Lines = append(currentHunk.Lines, model.Line{
				Type:    model.LineAdded,
				Content: line[1:],
				NewNum:  newNum,
			})
			newNum++
		} else if strings.HasPrefix(line, "-") {
			currentHunk.Lines = append(currentHunk.Lines, model.Line{
				Type:    model.LineRemoved,
				Content: line[1:],
				OldNum:  oldNum,
			})
			oldNum++
		} else if strings.HasPrefix(line, " ") {
			currentHunk.Lines = append(currentHunk.Lines, model.Line{
				Type:    model.LineContext,
				Content: line[1:],
				OldNum:  oldNum,
				NewNum:  newNum,
			})
			oldNum++
			newNum++
		} else if strings.HasPrefix(line, `\`) {
			// "\ No newline at end of file" — skip
			continue
		}
	}

	// Flush last hunk and file
	if currentFile != nil {
		if currentHunk != nil {
			currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
		}
		result.Files = append(result.Files, *currentFile)
	}

	return result
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
