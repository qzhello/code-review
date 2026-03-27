package agent

import (
	"context"
	"fmt"

	openai "github.com/sashabaranov/go-openai"

	"github.com/quzhihao/code-review/internal/model"
)

// Client wraps the OpenAI API for code review.
type Client struct {
	client *openai.Client
	cfg    model.AgentConfig
}

// NewClient creates a new OpenAI client from config.
func NewClient(cfg model.AgentConfig) (*Client, error) {
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
	}, nil
}

// ChatCompletion sends a chat completion request and returns the response text.
func (c *Client) ChatCompletion(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	model := c.cfg.Model
	if model == "" {
		model = "gpt-4o"
	}

	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:       model,
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
		return "", fmt.Errorf("OpenAI API error: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response from OpenAI API")
	}

	return resp.Choices[0].Message.Content, nil
}
