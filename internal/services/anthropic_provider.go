package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

const anthropicAPIBase = "https://api.anthropic.com/v1"
const anthropicAPIVersion = "2023-06-01"
const anthropicDefaultModel = "claude-haiku-4-5-20251001"

var anthropicHTTPClient = &http.Client{
	Timeout: 60 * time.Second,
	Transport: &http.Transport{
		DialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
	},
}

// AnthropicProvider implements LLMProvider using the Anthropic Messages API.
type AnthropicProvider struct {
	apiKey string
	model  string
}

func newAnthropicProvider(apiKey, model string) *AnthropicProvider {
	if model == "" {
		model = anthropicDefaultModel
	}
	return &AnthropicProvider{apiKey: apiKey, model: model}
}

func (p *AnthropicProvider) ProviderName() string { return "Claude" }

// ---- Anthropic wire types ----

// anthropicResponseBlock is used to parse content blocks in the API response.
type anthropicResponseBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type anthropicAPIResponse struct {
	Content    []anthropicResponseBlock `json:"content"`
	StopReason string                   `json:"stop_reason"`
}

// ---- LLMProvider methods ----

func (p *AnthropicProvider) Complete(ctx context.Context, prompt string) (string, error) {
	raw, err := p.postMessages(ctx, map[string]interface{}{
		"model":      p.model,
		"max_tokens": 256,
		"messages":   []map[string]interface{}{{"role": "user", "content": prompt}},
	})
	if err != nil {
		return "", err
	}
	var resp anthropicAPIResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("parse anthropic response: %w", err)
	}
	for _, block := range resp.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}
	return "", nil
}

// ---- helpers ----

func (p *AnthropicProvider) postMessages(ctx context.Context, reqBody interface{}) ([]byte, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPIBase+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := anthropicHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic unavailable: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic error %d: %s", resp.StatusCode, string(raw))
	}
	return raw, nil
}
