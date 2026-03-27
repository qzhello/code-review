package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/quzhihao/code-review/internal/model"
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

// Reviewer orchestrates LLM-powered code review.
type Reviewer struct {
	client     *Client
	cfg        model.AgentConfig
	prdContent string
}

// NewReviewer creates a new agent reviewer.
// prdContent is optional PRD/requirements document for additional review context.
func NewReviewer(cfg model.AgentConfig, prdContent string) (*Reviewer, error) {
	client, err := NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &Reviewer{client: client, cfg: cfg, prdContent: prdContent}, nil
}

// Review runs the agent review on a diff, returning findings.
// existingFindings are passed to the prompt to avoid duplication.
func (r *Reviewer) Review(ctx context.Context, diff *model.DiffResult, existingFindings []model.Finding) ([]model.Finding, error) {
	chunks := ChunkDiff(diff, 200)
	if len(chunks) == 0 {
		return nil, nil
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

			findings, err := r.reviewChunk(ctx, systemPrompt, c)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			allFindings = append(allFindings, findings...)
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

	response, err := r.client.ChatCompletion(ctx, systemPrompt, userMsg)
	if err != nil {
		return nil, fmt.Errorf("agent review failed for %s: %w", chunk.FilePath, err)
	}

	return parseAgentResponse(response, chunk.FilePath)
}

func parseAgentResponse(response string, defaultFile string) ([]model.Finding, error) {
	var resp agentResponse
	if err := json.Unmarshal([]byte(response), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse agent response: %w\nResponse: %s", err, response)
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
