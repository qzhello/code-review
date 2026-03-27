package agent

import (
	"fmt"
	"strings"

	"github.com/quzhihao/code-review/internal/model"
)

const (
	maxChunkTokens = 3000 // approximate token limit per chunk
	charsPerToken  = 4    // rough estimate
)

// Chunk represents a piece of diff sent to the agent.
type Chunk struct {
	FilePath string
	Content  string
	StartLine int
}

// ChunkDiff splits a DiffResult into reviewable chunks.
// Strategy: one chunk per file. If a file is too large, split by hunks.
func ChunkDiff(diff *model.DiffResult, maxFileSizeKB int) []Chunk {
	var chunks []Chunk

	for _, f := range diff.Files {
		if f.IsBinary {
			continue
		}

		fileDiffText := formatFileDiff(&f)

		// If within token budget, send as one chunk
		estimatedTokens := len(fileDiffText) / charsPerToken
		if estimatedTokens <= maxChunkTokens {
			chunks = append(chunks, Chunk{
				FilePath:  f.Path(),
				Content:   fileDiffText,
				StartLine: firstLineNum(&f),
			})
			continue
		}

		// Too large — split by hunks
		for i, hunk := range f.Hunks {
			hunkText := formatHunk(&f, &hunk, i)
			chunks = append(chunks, Chunk{
				FilePath:  f.Path(),
				Content:   hunkText,
				StartLine: hunk.NewStart,
			})
		}
	}

	return chunks
}

func formatFileDiff(f *model.FileDiff) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("File: %s (%s)\n", f.Path(), f.Status.String()))
	for i, hunk := range f.Hunks {
		sb.WriteString(formatHunk(f, &hunk, i))
	}
	return sb.String()
}

func formatHunk(f *model.FileDiff, hunk *model.Hunk, index int) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- %s (hunk %d) ---\n", f.Path(), index+1))
	sb.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@ %s\n",
		hunk.OldStart, hunk.OldCount, hunk.NewStart, hunk.NewCount, hunk.Header))

	for _, line := range hunk.Lines {
		switch line.Type {
		case model.LineAdded:
			sb.WriteString(fmt.Sprintf("+%s\n", line.Content))
		case model.LineRemoved:
			sb.WriteString(fmt.Sprintf("-%s\n", line.Content))
		default:
			sb.WriteString(fmt.Sprintf(" %s\n", line.Content))
		}
	}

	return sb.String()
}

func firstLineNum(f *model.FileDiff) int {
	if len(f.Hunks) > 0 {
		return f.Hunks[0].NewStart
	}
	return 1
}
