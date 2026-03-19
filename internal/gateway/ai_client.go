package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type AIConfig struct {
	BaseURL      string
	APIKey       string
	Model        string
	SystemPrompt string
	Timeout      time.Duration
	MaxTokens    int
}

type AIClient struct {
	cfg    AIConfig
	client *http.Client
}

func NewAIClient(cfg AIConfig) *AIClient {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = "https://api.openai.com"
	}
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = "gpt-4.1-mini"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 20 * time.Second
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 400
	}
	if strings.TrimSpace(cfg.SystemPrompt) == "" {
		cfg.SystemPrompt = "You are a concise assistant for BBS users."
	}
	return &AIClient{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
	}
}

func (c *AIClient) Enabled() bool {
	return strings.TrimSpace(c.cfg.APIKey) != "" && strings.TrimSpace(c.cfg.Model) != ""
}

func (c *AIClient) Complete(ctx context.Context, prompt string) (string, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", errors.New("prompt is required")
	}
	if !c.Enabled() {
		return "", errors.New("ai gateway is not configured")
	}
	base := strings.TrimSpace(c.cfg.BaseURL)
	endpoint, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("invalid ai base url: %w", err)
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/v1/chat/completions"

	payload := map[string]interface{}{
		"model": c.cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": c.cfg.SystemPrompt},
			{"role": "user", "content": prompt},
		},
		"max_tokens":  c.cfg.MaxTokens,
		"temperature": 0.6,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.cfg.APIKey))

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai request failed: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(responseBody))
		if len(msg) > 200 {
			msg = msg[:200] + "..."
		}
		if msg == "" {
			msg = resp.Status
		}
		return "", fmt.Errorf("ai request failed: %s", msg)
	}

	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return "", fmt.Errorf("decode ai response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return "", errors.New("ai response had no choices")
	}
	content := strings.TrimSpace(decoded.Choices[0].Message.Content)
	if content == "" {
		return "", errors.New("ai response was empty")
	}
	return content, nil
}
