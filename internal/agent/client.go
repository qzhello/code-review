package agent

import (
	"context"
	"fmt"

	openai "github.com/sashabaranov/go-openai"

	"github.com/qzhello/code-review/internal/cache"
	"github.com/qzhello/code-review/internal/model"
)

// Client wraps the OpenAI API for code review.
type Client struct {
	client *openai.Client
	cfg    model.AgentConfig
	cache  *cache.Cache
}

// NewClient creates a new OpenAI client from config.
func NewClient(cfg model.AgentConfig, c *cache.Cache) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("agent API key not configured (set OPENAI_API_KEY or agent.api_key in config)")
	}

	clientCfg := openai.DefaultConfig(cfg.APIKey)
	if cfg.BaseURL != "" {
		clientCfg.BaseURL = cfg.BaseURL
	}

	return &Client{
		client: openai.NewClientWithConfig(clientCfg),
		cfg:    cfg,
		cache:  c,
	}, nil
}

// CompletionResult holds the response and token usage from an API call.
type CompletionResult struct {
	Content   string
	TokensIn  int
	TokensOut int
	Cached    bool
}

// ChatCompletion sends a chat completion request, using cache when available.
func (c *Client) ChatCompletion(ctx context.Context, systemPrompt, userMessage string) (*CompletionResult, error) {
	mdl := c.cfg.Model
	if mdl == "" {
		mdl = "gpt-4o"
	}

	// Check cache
	if c.cache != nil {
		cacheKey := cache.DiffHash(userMessage, mdl, systemPrompt)
		if cached, ok := c.cache.Get(cacheKey); ok {
			// Log cache hit
			c.cache.LogUsage(mdl, 0, 0, 0, true)
			return &CompletionResult{Content: cached, Cached: true}, nil
		}
	}

	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:       mdl,
		Temperature: float32(c.cfg.Temperature),
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: userMessage},
		},
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("OpenAI API error: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from OpenAI API")
	}

	content := resp.Choices[0].Message.Content
	tokensIn := resp.Usage.PromptTokens
	tokensOut := resp.Usage.CompletionTokens

	// Store in cache + log usage
	if c.cache != nil {
		cacheKey := cache.DiffHash(userMessage, mdl, systemPrompt)
		c.cache.Put(cacheKey, content, mdl, tokensIn, tokensOut)

		cost := estimateCost(mdl, tokensIn, tokensOut)
		c.cache.LogUsage(mdl, tokensIn, tokensOut, cost, false)
	}

	return &CompletionResult{
		Content:   content,
		TokensIn:  tokensIn,
		TokensOut: tokensOut,
	}, nil
}

// estimateCost returns an approximate cost in USD.
func estimateCost(model string, tokensIn, tokensOut int) float64 {
	// Approximate pricing per 1M tokens (as of 2025)
	var inPer1M, outPer1M float64
	switch model {
	case "gpt-4o":
		inPer1M, outPer1M = 2.50, 10.00
	case "gpt-4o-mini":
		inPer1M, outPer1M = 0.15, 0.60
	case "gpt-4-turbo":
		inPer1M, outPer1M = 10.00, 30.00
	default:
		inPer1M, outPer1M = 2.50, 10.00
	}

	return (float64(tokensIn)/1_000_000)*inPer1M + (float64(tokensOut)/1_000_000)*outPer1M
}
