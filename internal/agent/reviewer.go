package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/qzhello/code-review/internal/cache"
	"github.com/qzhello/code-review/internal/model"
)

// agentResponse is the expected JSON response from the LLM.
type agentResponse struct {
	Findings []agentFinding `json:"findings"`
}

type agentFinding struct {
	File       string `json:"file"`
	Line       int    `json:"line"`
	Severity   string `json:"severity"`
	Confidence string `json:"confidence"`
	Category   string `json:"category"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
}

// ProgressFunc is called to report review progress.
// fileName is the file being reviewed, done is true when the chunk is complete.
type ProgressFunc func(fileName string, done bool, cached bool)

// TotalFunc is called once with the actual number of chunks to review.
type TotalFunc func(total int)

// Reviewer orchestrates LLM-powered code review.
type Reviewer struct {
	client     *Client
	cfg        model.AgentConfig
	prdContent string
	onProgress ProgressFunc
	onTotal    TotalFunc
}

// NewReviewer creates a new agent reviewer.
// prdContent is optional PRD/requirements document for additional review context.
// c is optional cache for caching LLM responses.
func NewReviewer(cfg model.AgentConfig, prdContent string, c *cache.Cache) (*Reviewer, error) {
	client, err := NewClient(cfg, c)
	if err != nil {
		return nil, err
	}
	return &Reviewer{client: client, cfg: cfg, prdContent: prdContent}, nil
}

// SetProgress sets the progress callback.
func (r *Reviewer) SetProgress(fn ProgressFunc) {
	r.onProgress = fn
}

// SetTotalCallback sets the callback to report the actual chunk count.
func (r *Reviewer) SetTotalCallback(fn TotalFunc) {
	r.onTotal = fn
}

// Review runs the agent review on a diff, returning findings.
// existingFindings are passed to the prompt to avoid duplication.
func (r *Reviewer) Review(ctx context.Context, diff *model.DiffResult, existingFindings []model.Finding) ([]model.Finding, error) {
	chunks := ChunkDiff(diff, 200)
	if len(chunks) == 0 {
		return nil, nil
	}

	if r.onTotal != nil {
		r.onTotal(len(chunks))
	}

	systemPrompt := BuildSystemPrompt(r.cfg, existingFindings, r.prdContent)

	// Concurrent review with bounded concurrency
	concurrency := r.cfg.MaxConcurrency
	if concurrency <= 0 {
		concurrency = 5
	}
	sem := make(chan struct{}, concurrency)

	var mu sync.Mutex
	var allFindings []model.Finding
	var firstErr error

	var wg sync.WaitGroup
	for _, chunk := range chunks {
		wg.Add(1)
		go func(c Chunk) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			if r.onProgress != nil {
				r.onProgress(c.FilePath, false, false)
			}

			findings, err := r.reviewChunk(ctx, systemPrompt, c)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				if r.onProgress != nil {
					r.onProgress(c.FilePath, true, false)
				}
				return
			}
			allFindings = append(allFindings, findings...)
			if r.onProgress != nil {
				// Check if this was a cache hit
				wasCached := len(findings) == 0 // approximate; actual cache hit tracked in client
				r.onProgress(c.FilePath, true, wasCached)
			}
		}(chunk)
	}
	wg.Wait()

	if firstErr != nil && len(allFindings) == 0 {
		return nil, firstErr
	}

	// Filter by confidence threshold
	allFindings = filterByConfidence(allFindings, r.cfg.ConfidenceThreshold)

	return allFindings, nil
}

func (r *Reviewer) reviewChunk(ctx context.Context, systemPrompt string, chunk Chunk) ([]model.Finding, error) {
	userMsg := BuildUserMessage(chunk)

	result, err := r.client.ChatCompletion(ctx, systemPrompt, userMsg)
	if err != nil {
		return nil, fmt.Errorf("agent review failed for %s: %w", chunk.FilePath, err)
	}

	return parseAgentResponse(result.Content, chunk.FilePath)
}

func parseAgentResponse(response string, defaultFile string) ([]model.Finding, error) {
	// Strip <think>...</think> blocks from reasoning models (e.g., MiniMax, DeepSeek)
	response = stripThinkingTags(response)

	// Extract JSON from response (some models wrap JSON in markdown code blocks)
	response = extractJSON(response)

	var resp agentResponse
	if err := json.Unmarshal([]byte(response), &resp); err != nil {
		// Truncate long responses in error message
		preview := response
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		return nil, fmt.Errorf("failed to parse agent response: %w\nResponse: %s", err, preview)
	}

	var findings []model.Finding
	for _, af := range resp.Findings {
		sev, _ := model.ParseSeverity(af.Severity)
		filePath := af.File
		if filePath == "" {
			filePath = defaultFile
		}

		findings = append(findings, model.Finding{
			RuleID:     "agent",
			Severity:   sev,
			Confidence: model.ParseConfidence(af.Confidence),
			FilePath:   filePath,
			Line:       af.Line,
			Message:    af.Message,
			Category:   af.Category,
			Source:     "agent",
			Suggestion: af.Suggestion,
		})
	}

	return findings, nil
}

func filterByConfidence(findings []model.Finding, threshold string) []model.Finding {
	minConfidence := model.ParseConfidence(threshold)

	var filtered []model.Finding
	for _, f := range findings {
		if f.Source != "agent" || f.Confidence >= minConfidence {
			filtered = append(filtered, f)
		}
	}
	return filtered
}

var thinkingRe = regexp.MustCompile(`(?s)<think>.*?</think>`)

// stripThinkingTags removes <think>...</think> blocks from reasoning model responses.
func stripThinkingTags(s string) string {
	return strings.TrimSpace(thinkingRe.ReplaceAllString(s, ""))
}

// extractJSON extracts a JSON object from a response that may contain markdown code blocks.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)

	// Already starts with { — return as-is
	if strings.HasPrefix(s, "{") {
		return s
	}

	// Try to extract from ```json ... ``` or ``` ... ```
	re := regexp.MustCompile("(?s)```(?:json)?\\s*\\n?(\\{.*?\\})\\s*```")
	if m := re.FindStringSubmatch(s); len(m) > 1 {
		return m[1]
	}

	// Try to find first { ... } block
	start := strings.Index(s, "{")
	if start >= 0 {
		// Find matching closing brace
		depth := 0
		for i := start; i < len(s); i++ {
			switch s[i] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					return s[start : i+1]
				}
			}
		}
	}

	return s
}
