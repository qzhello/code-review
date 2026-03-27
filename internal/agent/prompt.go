package agent

import (
	"fmt"
	"strings"

	"github.com/qzhello/code-review/internal/model"
)

// BuildSystemPrompt constructs the system prompt for the agent.
// prdContent is optional PRD/requirements document content for context.
func BuildSystemPrompt(cfg model.AgentConfig, existingFindings []model.Finding, prdContent string) string {
	var sb strings.Builder

	// Base persona
	persona := cfg.Persona
	if persona == "" {
		persona = defaultPersona
	}
	sb.WriteString(persona)
	sb.WriteString("\n\n")

	// Focus areas
	if len(cfg.Focus) > 0 {
		sb.WriteString("Focus your review on these areas: ")
		sb.WriteString(strings.Join(cfg.Focus, ", "))
		sb.WriteString(".\n")
	}

	// Ignore areas
	if len(cfg.Ignore) > 0 {
		sb.WriteString("Do NOT comment on: ")
		sb.WriteString(strings.Join(cfg.Ignore, ", "))
		sb.WriteString(".\n")
	}

	// PRD / requirements context
	if prdContent != "" {
		sb.WriteString("\n## Product Requirements Document (PRD)\n")
		sb.WriteString("The following PRD describes what this code should accomplish. ")
		sb.WriteString("Use it to evaluate whether the code changes correctly implement the requirements, ")
		sb.WriteString("miss any edge cases described in the PRD, or deviate from the expected behavior.\n\n")
		sb.WriteString(prdContent)
		sb.WriteString("\n\n")
	}

	// Existing findings (to avoid duplication)
	if len(existingFindings) > 0 {
		sb.WriteString("\nThe following issues have already been found by static rules. Do NOT repeat them:\n")
		for _, f := range existingFindings {
			sb.WriteString(fmt.Sprintf("- %s:%d — %s\n", f.FilePath, f.Line, f.Message))
		}
	}

	// Output language
	lang := cfg.Language
	if lang != "" && lang != "en" {
		langName := expandLanguageCode(lang)
		sb.WriteString(fmt.Sprintf("\nIMPORTANT: Write all \"message\" and \"suggestion\" fields in %s (%s). "+
			"Keep JSON keys, severity values, and file paths in English. Only the human-readable text should be in %s.\n", langName, lang, langName))
	}

	// Response format
	sb.WriteString(`
Respond with a JSON object containing a "findings" array. Each finding must have:
- "file": the file path
- "line": the line number in the new file (0 if file-level)
- "severity": "error", "warn", or "info"
- "confidence": "high", "medium", or "low"
- "category": one of "logic_errors", "security", "performance", "error_handling", "concurrency", "other"
- "message": clear, concise description of the issue
- "suggestion": optional fix suggestion (empty string if none)

If there are no issues, return {"findings": []}.

Only report real, actionable issues. Do not speculate or report hypothetical problems.
Be precise about line numbers — they must match the diff.
`)

	return sb.String()
}

// BuildUserMessage constructs the user message for a diff chunk.
func BuildUserMessage(chunk Chunk) string {
	return fmt.Sprintf("Please review the following code diff:\n\n```diff\n%s\n```", chunk.Content)
}

const defaultPersona = `You are a senior software engineer performing code review.
You have deep expertise in identifying bugs, security vulnerabilities, and performance issues.
You are thorough but pragmatic — only flag issues that genuinely matter.
Be concise and specific in your feedback.`

// expandLanguageCode maps language codes to full names.
func expandLanguageCode(code string) string {
	languages := map[string]string{
		"zh":    "Chinese (中文)",
		"zh-CN": "Simplified Chinese (简体中文)",
		"zh-TW": "Traditional Chinese (繁體中文)",
		"ja":    "Japanese (日本語)",
		"ko":    "Korean (한국어)",
		"es":    "Spanish (Español)",
		"fr":    "French (Français)",
		"de":    "German (Deutsch)",
		"pt":    "Portuguese (Português)",
		"ru":    "Russian (Русский)",
		"ar":    "Arabic (العربية)",
		"it":    "Italian (Italiano)",
	}
	if name, ok := languages[code]; ok {
		return name
	}
	return code
}
