package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	client       *openai.Client
	cfg          model.AgentConfig
	systemPrompt string // base system prompt (without finding context)
	plainMode    bool   // plain markdown mode (no JSON) for web streaming
	messages     []openai.ChatCompletionMessage
}

// NewChatSession creates a new chat session for interactive code review (JSON mode for TUI).
func NewChatSession(cfg model.AgentConfig) (*ChatSession, error) {
	client, err := NewClient(cfg, (*cache.Cache)(nil))
	if err != nil {
		return nil, err
	}

	systemPrompt := buildChatSystemPrompt(cfg)

	return &ChatSession{
		client:       client.client,
		cfg:          cfg,
		systemPrompt: systemPrompt,
		messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
		},
	}, nil
}

// NewPlainChatSession creates a chat session for web UI — plain markdown output, no JSON.
// This enables reliable streaming since the LLM outputs clean text directly.
func NewPlainChatSession(cfg model.AgentConfig) (*ChatSession, error) {
	client, err := NewClient(cfg, (*cache.Cache)(nil))
	if err != nil {
		return nil, err
	}

	systemPrompt := buildPlainChatSystemPrompt(cfg)

	return &ChatSession{
		client:       client.client,
		cfg:          cfg,
		systemPrompt: systemPrompt,
		plainMode:    true,
		messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
		},
	}, nil
}

// SetFindingContext updates the conversation context for a specific finding.
// The finding context is merged into a single system message to ensure compatibility
// with APIs that only support one system message (e.g., MiniMax, some OpenAI-compatible providers).
func (s *ChatSession) SetFindingContext(finding model.Finding) error {
	fileContent, err := os.ReadFile(finding.FilePath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", finding.FilePath, err)
	}

	contextMsg := buildFindingContext(finding, string(fileContent))

	// Merge system prompt + finding context into a single system message
	// Many non-OpenAI APIs reject multiple system messages
	mergedSystem := s.systemPrompt + "\n\n" + contextMsg

	// Reset to single system message + clear conversation history
	s.messages = []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: mergedSystem},
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

// SendStream sends a user message and streams the response token by token.
// onToken is called for each token. If streaming fails, it transparently falls back
// to a non-streaming request and delivers the full response at once via onToken.
func (s *ChatSession) SendStream(ctx context.Context, userMsg string, onToken func(token string)) (*ChatResponse, error) {
	s.messages = append(s.messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: userMsg,
	})

	mdl := s.cfg.Model
	if mdl == "" {
		mdl = "gpt-4o"
	}

	// Try streaming first
	content, err := s.tryStream(ctx, mdl, onToken)
	if err != nil {
		// Streaming failed — fall back to non-streaming
		// Remove the user message (Send will re-add it)
		s.messages = s.messages[:len(s.messages)-1]

		// Signal frontend to discard partial content
		if onToken != nil {
			onToken("\x00__RESET__")
		}

		return s.sendNonStream(ctx, userMsg, onToken)
	}

	// Add to conversation history
	s.messages = append(s.messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleAssistant,
		Content: content,
	})

	return parseChatResponse(content)
}

// tryStream attempts a streaming request. Returns the full content or an error.
func (s *ChatSession) tryStream(ctx context.Context, mdl string, onToken func(token string)) (string, error) {
	req := openai.ChatCompletionRequest{
		Model:       mdl,
		Temperature: float32(s.cfg.Temperature),
		Messages:    s.messages,
		Stream:      true,
	}

	stream, err := s.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return "", err
	}
	defer stream.Close()

	var fullContent strings.Builder
	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// Got partial content but stream broke — return error so we fall back
			return "", fmt.Errorf("stream interrupted: %w", err)
		}
		if len(resp.Choices) > 0 {
			token := resp.Choices[0].Delta.Content
			if token != "" {
				fullContent.WriteString(token)
				if onToken != nil {
					onToken(token)
				}
			}
		}
	}

	content := fullContent.String()
	if content == "" {
		return "", fmt.Errorf("empty stream response")
	}
	return content, nil
}

// sendNonStream does a regular (non-streaming) request and delivers the result via onToken.
func (s *ChatSession) sendNonStream(ctx context.Context, userMsg string, onToken func(token string)) (*ChatResponse, error) {
	resp, err := s.Send(ctx, userMsg)
	if err != nil {
		return nil, err
	}
	// Deliver the full message as a single "token" so the frontend shows it
	if onToken != nil && resp.Message != "" {
		onToken(resp.Message)
	}
	return resp, nil
}

// SendPlainStream streams the LLM response directly as plain markdown text.
// Returns the full response text (not parsed as JSON). Used by the web UI.
func (s *ChatSession) SendPlainStream(ctx context.Context, userMsg string, onToken func(token string)) (string, error) {
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
		Stream:      true,
	}

	stream, err := s.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		// Streaming not supported — fall back to non-streaming
		s.messages = s.messages[:len(s.messages)-1]
		return s.sendPlainNonStream(ctx, userMsg, onToken)
	}
	defer stream.Close()

	var fullContent strings.Builder
	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if fullContent.Len() > 0 {
				break // use what we have
			}
			// Fall back to non-streaming
			s.messages = s.messages[:len(s.messages)-1]
			return s.sendPlainNonStream(ctx, userMsg, onToken)
		}
		if len(resp.Choices) > 0 {
			token := resp.Choices[0].Delta.Content
			if token != "" {
				fullContent.WriteString(token)
				if onToken != nil {
					onToken(token)
				}
			}
		}
	}

	content := fullContent.String()

	// Add to conversation history
	s.messages = append(s.messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleAssistant,
		Content: content,
	})

	return content, nil
}

// sendPlainNonStream is the fallback for SendPlainStream.
func (s *ChatSession) sendPlainNonStream(ctx context.Context, userMsg string, onToken func(token string)) (string, error) {
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

	resp, err := s.client.CreateChatCompletion(ctx, req)
	if err != nil {
		s.messages = s.messages[:len(s.messages)-1]
		return "", fmt.Errorf("API error: %w", err)
	}

	if len(resp.Choices) == 0 {
		s.messages = s.messages[:len(s.messages)-1]
		return "", fmt.Errorf("no response from API")
	}

	content := resp.Choices[0].Message.Content
	content = stripThinkingTags(content)

	s.messages = append(s.messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleAssistant,
		Content: content,
	})

	if onToken != nil && content != "" {
		onToken(content)
	}

	return content, nil
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

	sb.WriteString(`You are an expert code review assistant. You help developers understand and resolve code findings through focused, structured dialogue.

## CRITICAL: Response format

You MUST ALWAYS respond with ONLY a valid JSON object. No other text, no code blocks wrapping it, no explanation outside the JSON. Your entire response must be parseable JSON.

The JSON object:
{"message": "Your response (Markdown formatted)", "action": "none", "fixed_content": "", "target": "current"}

NEVER echo back source code from the file context as your raw response. NEVER respond with Go/Python/JS code outside the JSON structure. ALL your text must be inside the "message" field as a string.

## Response style rules (CRITICAL)

Your "message" MUST follow these rules strictly:

1. **Be concise** — Get to the point immediately. No greetings, no filler, no "Sure!", no "Great question!". Start directly with the answer.
2. **Use structured Markdown** — Always organize your response with:
   - **Bold headers** for sections
   - Bullet points for lists
   - ` + "`code`" + ` for inline code references
   - Code blocks with language tags for examples
3. **Key points first** — Lead with the most important information. Use this structure:
   - **Problem**: What's wrong (1-2 sentences max)
   - **Impact**: Why it matters (1 sentence)
   - **Fix**: How to resolve it (concrete, actionable)
4. **No repetition** — Never restate what the user said. Never repeat the finding description they can already see.
5. **No filler** — Remove words like "basically", "essentially", "in order to", "it's worth noting". Every sentence must carry information.
6. **Show, don't tell** — Prefer a code example over a paragraph of explanation. A 3-line code snippet is better than 3 paragraphs.
7. **One concept per response** — Don't dump everything at once. Answer what was asked, nothing more.

### Bad response example:
"Sure! That's a great question. So basically, the issue here is that you have a potential null pointer dereference. This means that when the variable is null, accessing its properties could cause a runtime error. To fix this, you could add a null check before accessing the property. Here's how you might do it..."

### Good response example:
"**Problem**: ` + "`user.Profile`" + ` can be ` + "`nil`" + ` when the account is newly created.\n\n**Fix**:\n` + "```" + `go\nif user.Profile != nil {\n    name = user.Profile.Name\n}\n` + "```" + `\n\nThis prevents the nil pointer panic on line 42."

## Available actions

| action     | effect                                          |
|------------|-------------------------------------------------|
| "none"     | Just respond/explain, no side effects           |
| "fix"      | Fix the code — MUST set "fixed_content"         |
| "dismiss"  | Mark finding(s) as dismissed (not a real issue) |
| "accept"   | Mark finding(s) as accepted (acknowledged)      |
| "next"     | Navigate to the next finding                    |
| "prev"     | Navigate to the previous finding                |

## Target scope (for fix/dismiss/accept)

| target     | scope                                |
|------------|--------------------------------------|
| "current"  | Only the current finding (default)   |
| "all"      | All pending findings                 |
| "errors"   | All pending findings with severity=error |
| "warnings" | All pending findings with severity=warn  |
| "infos"    | All pending findings with severity=info  |

## Fixing code

When fixing:
1. Set action to "fix", fixed_content to the COMPLETE file with the fix applied
2. In message, briefly explain the change (2-3 sentences max, with a code diff snippet)
3. Fix ONLY the specific issue — do NOT change formatting, style, or unrelated code
4. Preserve all existing whitespace and line endings
5. If you cannot fix it, set action to "none" and explain why in 1-2 sentences

## Intent recognition

Understand the user's intent from short messages:
- "ok" / "looks good" / "agree" → action:"accept"
- "not a problem" / "ignore" / "skip this" → action:"dismiss"
- "fix" / "fix this" / "fix it" → action:"fix"
- "next" / "skip" / "move on" → action:"next"
- "dismiss all info" → action:"dismiss", target:"infos"
- "accept everything" → action:"accept", target:"all"
`)

	lang := cfg.Language
	if lang != "" && lang != "en" {
		langName := expandLanguageCode(lang)
		sb.WriteString(fmt.Sprintf("\nIMPORTANT: Write all \"message\" text in %s (%s). Keep JSON keys and action values in English.\n", langName, lang))
	}

	return sb.String()
}

func buildPlainChatSystemPrompt(cfg model.AgentConfig) string {
	var sb strings.Builder

	sb.WriteString(`You are an expert code review assistant. You help developers understand and resolve code findings through focused, structured dialogue.

## Response rules (CRITICAL)

Respond in **plain Markdown**. Do NOT wrap your response in JSON, code blocks, or any other structure. Just write your answer directly.

1. **Be concise** — Get to the point. No greetings, no filler ("Sure!", "Great question!"). Start with the answer.
2. **Structured Markdown** — Use:
   - **Bold** for key terms
   - Bullet points for lists
   - ` + "`code`" + ` for inline references
   - Fenced code blocks (with language tag) for examples
3. **Key points first** — Structure as:
   - **Problem**: What's wrong (1-2 sentences)
   - **Impact**: Why it matters (1 sentence)
   - **Fix**: How to resolve it (code example preferred)
4. **No repetition** — Don't restate the finding or what the user said.
5. **Show, don't tell** — A 3-line code snippet beats 3 paragraphs.
6. **One concept per response** — Answer what was asked, nothing more.
7. **Never dump the entire file** — Only show relevant snippets.
`)

	lang := cfg.Language
	if lang != "" && lang != "en" {
		langName := expandLanguageCode(lang)
		sb.WriteString(fmt.Sprintf("\nIMPORTANT: Respond in %s (%s).\n", langName, lang))
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
		// JSON parsing failed — the LLM didn't follow instructions.
		// Truncate overly long raw responses (likely echoed source code).
		msg := content
		if len(msg) > 500 {
			msg = msg[:500] + "\n\n*(response truncated — AI returned invalid format)*"
		}
		return &ChatResponse{
			Message: msg,
			Action:  "none",
		}, nil
	}

	// Sanity check: if message is empty but we got content, use content
	if resp.Message == "" && content != "" {
		resp.Message = "(AI returned empty message)"
	}

	return &resp, nil
}
