package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	openai "github.com/sashabaranov/go-openai"

	"github.com/qzhello/code-review/internal/cache"
	"github.com/qzhello/code-review/internal/model"
)

// ChatMessage represents a single message in the conversation.
type ChatMessage struct {
	Role    string // "user" or "assistant"
	Content string // display content for the user
}

// ChatResponse is the parsed response from the LLM in chat mode.
type ChatResponse struct {
	Message      string `json:"message"`
	Action       string `json:"action"`        // see buildChatSystemPrompt for full list
	FixedContent string `json:"fixed_content"` // only when action is "fix"
	Target       string `json:"target"`        // "current", "all", "errors", "warnings", "infos"
}

// ChatSession maintains a multi-turn conversation about code review findings.
type ChatSession struct {
	client   *openai.Client
	cfg      model.AgentConfig
	messages []openai.ChatCompletionMessage
}

// NewChatSession creates a new chat session for interactive code review.
func NewChatSession(cfg model.AgentConfig) (*ChatSession, error) {
	client, err := NewClient(cfg, (*cache.Cache)(nil))
	if err != nil {
		return nil, err
	}

	systemPrompt := buildChatSystemPrompt(cfg)

	return &ChatSession{
		client: client.client,
		cfg:    cfg,
		messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
		},
	}, nil
}

// SetFindingContext updates the conversation context for a specific finding.
// This adds a system-level context message so the AI knows which finding is being discussed.
func (s *ChatSession) SetFindingContext(finding model.Finding) error {
	fileContent, err := os.ReadFile(finding.FilePath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", finding.FilePath, err)
	}

	contextMsg := buildFindingContext(finding, string(fileContent))

	// Keep system prompt, replace or add finding context
	// System prompt is always messages[0]. Finding context is messages[1] if it exists.
	if len(s.messages) > 1 && s.messages[1].Role == openai.ChatMessageRoleSystem {
		s.messages[1] = openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: contextMsg,
		}
		// Clear conversation history when switching findings
		s.messages = s.messages[:2]
	} else {
		// Insert finding context after system prompt
		s.messages = []openai.ChatCompletionMessage{
			s.messages[0],
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: contextMsg,
			},
		}
	}

	return nil
}

// Send sends a user message and returns the AI response.
// The response may include an action (fix, dismiss, accept) along with the message.
func (s *ChatSession) Send(ctx context.Context, userMsg string) (*ChatResponse, error) {
	s.messages = append(s.messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: userMsg,
	})

	mdl := s.cfg.Model
	if mdl == "" {
		mdl = "gpt-4o"
	}

	req := openai.ChatCompletionRequest{
		Model:       mdl,
		Temperature: float32(s.cfg.Temperature),
		Messages:    s.messages,
	}

	if isOfficialOpenAI(s.cfg.BaseURL) {
		req.ResponseFormat = &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		}
	}

	resp, err := s.client.CreateChatCompletion(ctx, req)
	if err != nil {
		// Remove the failed user message so it can be retried
		s.messages = s.messages[:len(s.messages)-1]
		return nil, fmt.Errorf("API error: %w", err)
	}

	if len(resp.Choices) == 0 {
		s.messages = s.messages[:len(s.messages)-1]
		return nil, fmt.Errorf("no response from API")
	}

	content := resp.Choices[0].Message.Content

	// Add assistant response to history
	s.messages = append(s.messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleAssistant,
		Content: content,
	})

	return parseChatResponse(content)
}

// ApplyFix writes the fixed content to the file.
func ApplyFix(filePath string, fixedContent string) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to stat %s: %w", filePath, err)
	}
	return os.WriteFile(filePath, []byte(fixedContent), info.Mode())
}

func buildChatSystemPrompt(cfg model.AgentConfig) string {
	var sb strings.Builder

	sb.WriteString(`You are a senior software engineer helping with interactive code review.
You are in a conversation with a developer who is reviewing code findings (issues found in their code).

For each message, you MUST respond with a JSON object:
{
  "message": "Your response text here (use \n for newlines)",
  "action": "none",
  "fixed_content": "",
  "target": "current"
}

## Available actions

The "action" field controls what happens after your response:

| action     | effect                                          |
|------------|-------------------------------------------------|
| "none"     | Just respond/explain, no side effects           |
| "fix"      | Fix the code — MUST set "fixed_content"         |
| "dismiss"  | Mark finding(s) as dismissed (not a real issue) |
| "accept"   | Mark finding(s) as accepted (acknowledged)      |
| "next"     | Navigate to the next finding                    |
| "prev"     | Navigate to the previous finding                |

## Target scope

The "target" field controls which findings are affected (for fix/dismiss/accept):

| target     | scope                                |
|------------|--------------------------------------|
| "current"  | Only the current finding (default)   |
| "all"      | All pending findings                 |
| "errors"   | All pending findings with severity=error |
| "warnings" | All pending findings with severity=warn  |
| "infos"    | All pending findings with severity=info  |

Examples:
- User says "dismiss all info findings" → action:"dismiss", target:"infos"
- User says "accept everything" → action:"accept", target:"all"
- User says "fix this" → action:"fix", target:"current"
- User says "go to the next one" → action:"next", target:"current"
- User says "skip" → action:"next", target:"current"

## Fixing code

When the user asks you to fix something:
1. Set action to "fix"
2. Set fixed_content to the COMPLETE file content with the fix applied
3. In message, briefly explain what you changed

Rules for fixing:
- Fix ONLY the specific issue discussed
- Do NOT change any other code, formatting, or style
- Preserve all existing whitespace, indentation, and line endings
- The fixed_content must be the complete file, not a snippet
- If you cannot fix the issue, set action to "none" and explain why

## Capabilities

You can help the developer by:
- Fixing code issues (set action:"fix")
- Explaining why an issue is a problem
- Suggesting alternative approaches or best practices
- Answering questions about the code
- Navigating between findings (action:"next"/"prev")
- Batch-processing findings (action:"dismiss"/"accept" with target:"all"/"errors"/etc.)
- Discussing trade-offs of different fix approaches
- Providing code examples in your message

Be concise and helpful. Understand the user's intent — if they say "ok" or "looks good", that likely means accept. If they say "not a problem" or "ignore", that means dismiss. If they say "next" or "skip", navigate forward.
`)

	lang := cfg.Language
	if lang != "" && lang != "en" {
		langName := expandLanguageCode(lang)
		sb.WriteString(fmt.Sprintf("\nIMPORTANT: Write all \"message\" text in %s (%s). Keep JSON keys and action values in English.\n", langName, lang))
	}

	return sb.String()
}

func buildFindingContext(finding model.Finding, fileContent string) string {
	var sb strings.Builder

	sb.WriteString("## Current finding being discussed\n\n")
	sb.WriteString(fmt.Sprintf("- File: %s\n", finding.FilePath))
	sb.WriteString(fmt.Sprintf("- Line: %d\n", finding.Line))
	sb.WriteString(fmt.Sprintf("- Severity: %s\n", finding.Severity.String()))
	sb.WriteString(fmt.Sprintf("- Message: %s\n", finding.Message))
	if finding.Suggestion != "" {
		sb.WriteString(fmt.Sprintf("- Suggestion: %s\n", finding.Suggestion))
	}
	if finding.Category != "" {
		sb.WriteString(fmt.Sprintf("- Category: %s\n", finding.Category))
	}
	sb.WriteString(fmt.Sprintf("- Source: %s\n", finding.Source))

	sb.WriteString("\n## Full file content\n\n```\n")
	sb.WriteString(fileContent)
	sb.WriteString("\n```\n")

	return sb.String()
}

func parseChatResponse(content string) (*ChatResponse, error) {
	content = stripThinkingTags(content)
	content = extractJSON(content)

	var resp ChatResponse
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		// If JSON parsing fails, treat the whole response as a plain message
		return &ChatResponse{
			Message: content,
			Action:  "none",
		}, nil
	}

	return &resp, nil
}
