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

// ollamaHTTPClient has a 10-second dial timeout and a 5-minute overall timeout:
// a batch of category suggestions from a 4B model on a busy CPU-only laptop can
// take minutes. These calls run in the background, not in web requests.
var ollamaHTTPClient = &http.Client{
	Timeout: 5 * time.Minute,
	Transport: &http.Transport{
		DialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
	},
}

// ---- Ollama wire types ----

type ollamaMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ollamaNumCtx is the context size for every request. It must be the same for
// all of them (and match OLLAMA_CONTEXT_LENGTH in docker-compose.yml): Ollama
// reloads the model whenever num_ctx changes, which takes several seconds on a
// CPU-only host.
const ollamaNumCtx = 8192

type ollamaOptions struct {
	NumCtx     int `json:"num_ctx"`
	NumPredict int `json:"num_predict"`
	NumThread  int `json:"num_thread,omitempty"`
}

type ollamaReq struct {
	Model    string        `json:"model"`
	Messages []ollamaMsg   `json:"messages"`
	Stream   bool          `json:"stream"`
	Think    *bool         `json:"think,omitempty"`
	Options  ollamaOptions `json:"options"`
}

func boolPtr(b bool) *bool { return &b }

type ollamaChunk struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}

// ---- OllamaProvider ----

// OllamaProvider implements LLMProvider using a local Ollama instance.
type OllamaProvider struct {
	url       string
	model     string
	numThread int
}

func newOllamaProvider(url, model string, numThread int) *OllamaProvider {
	return &OllamaProvider{url: url, model: model, numThread: numThread}
}

func (p *OllamaProvider) ProviderName() string { return "Ollama" }

func (p *OllamaProvider) Complete(ctx context.Context, prompt string) (string, error) {
	body, _ := json.Marshal(ollamaReq{
		Model:    p.model,
		Messages: []ollamaMsg{{Role: "user", Content: prompt}},
		Stream:   false,
		Think:    boolPtr(false),
		Options:  ollamaOptions{NumCtx: ollamaNumCtx, NumPredict: 512, NumThread: p.numThread},
	})
	raw, err := p.post(ctx, body)
	if err != nil {
		return "", err
	}
	var chunk ollamaChunk
	if err := json.Unmarshal(raw, &chunk); err != nil {
		return "", fmt.Errorf("parse ollama response: %w", err)
	}
	return chunk.Message.Content, nil
}

func (p *OllamaProvider) post(ctx context.Context, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := ollamaHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama unavailable: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama error %d: %s", resp.StatusCode, string(raw))
	}
	return raw, nil
}
