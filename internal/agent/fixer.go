package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/qzhello/code-review/internal/cache"
	"github.com/qzhello/code-review/internal/model"
)

// fixResponse is the expected JSON response from the LLM for fix requests.
type fixResponse struct {
	FixedContent string `json:"fixed_content"`
	Explanation  string `json:"explanation"`
}

// Fixer uses the LLM to generate and apply code fixes.
type Fixer struct {
	client *Client
	cfg    model.AgentConfig
}

// NewFixer creates a new Fixer. Cache is intentionally nil for fixes (not cacheable).
func NewFixer(cfg model.AgentConfig) (*Fixer, error) {
	client, err := NewClient(cfg, (*cache.Cache)(nil))
	if err != nil {
		return nil, err
	}
	return &Fixer{client: client, cfg: cfg}, nil
}

// Fix reads the source file, asks the LLM to fix the finding, and writes the result back.
// Returns an explanation of the fix, or an error.
func (f *Fixer) Fix(ctx context.Context, finding model.Finding) (string, error) {
	// Read the source file
	content, err := os.ReadFile(finding.FilePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", finding.FilePath, err)
	}

	systemPrompt := buildFixSystemPrompt(f.cfg)
	userMsg := buildFixUserMessage(finding, string(content))

	result, err := f.client.ChatCompletion(ctx, systemPrompt, userMsg)
	if err != nil {
		return "", fmt.Errorf("fix generation failed: %w", err)
	}

	resp, err := parseFixResponse(result.Content)
	if err != nil {
		return "", err
	}

	if resp.FixedContent == "" {
		return "", fmt.Errorf("LLM returned empty fix")
	}

	// Write the fixed content back to the file
	info, err := os.Stat(finding.FilePath)
	if err != nil {
		return "", fmt.Errorf("failed to stat file %s: %w", finding.FilePath, err)
	}
	if err := os.WriteFile(finding.FilePath, []byte(resp.FixedContent), info.Mode()); err != nil {
		return "", fmt.Errorf("failed to write fixed file %s: %w", finding.FilePath, err)
	}

	return resp.Explanation, nil
}

func buildFixSystemPrompt(cfg model.AgentConfig) string {
	var sb strings.Builder

	sb.WriteString(`You are a senior software engineer. Your task is to fix a specific code issue.
You will receive:
1. The full source file content
2. A finding describing the issue (file, line, message, suggestion)

You must return a JSON object with:
- "fixed_content": the complete fixed file content (the entire file, not just the changed part)
- "explanation": a brief one-line explanation of what you changed

Rules:
- Fix ONLY the specific issue described in the finding.
- Do NOT change any other code, formatting, or style.
- Do NOT add comments explaining the fix unless absolutely necessary.
- Preserve all existing whitespace, indentation, and line endings.
- The fixed_content must be the complete file — not a snippet or diff.
- If you cannot fix the issue, return the original content unchanged and explain why.
`)

	// Output language
	lang := cfg.Language
	if lang != "" && lang != "en" {
		langName := expandLanguageCode(lang)
		sb.WriteString(fmt.Sprintf("\nWrite the \"explanation\" field in %s (%s).\n", langName, lang))
	}

	return sb.String()
}

func buildFixUserMessage(finding model.Finding, fileContent string) string {
	var sb strings.Builder

	sb.WriteString("## Finding to fix\n\n")
	sb.WriteString(fmt.Sprintf("- **File**: %s\n", finding.FilePath))
	sb.WriteString(fmt.Sprintf("- **Line**: %d\n", finding.Line))
	sb.WriteString(fmt.Sprintf("- **Severity**: %s\n", finding.Severity.String()))
	sb.WriteString(fmt.Sprintf("- **Message**: %s\n", finding.Message))
	if finding.Suggestion != "" {
		sb.WriteString(fmt.Sprintf("- **Suggestion**: %s\n", finding.Suggestion))
	}
	if finding.Category != "" {
		sb.WriteString(fmt.Sprintf("- **Category**: %s\n", finding.Category))
	}

	sb.WriteString("\n## Source file content\n\n```\n")
	sb.WriteString(fileContent)
	sb.WriteString("\n```\n")

	return sb.String()
}

func parseFixResponse(response string) (*fixResponse, error) {
	response = stripThinkingTags(response)
	response = extractJSON(response)

	var resp fixResponse
	if err := json.Unmarshal([]byte(response), &resp); err != nil {
		preview := response
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		return nil, fmt.Errorf("failed to parse fix response: %w\nResponse: %s", err, preview)
	}

	return &resp, nil
}
