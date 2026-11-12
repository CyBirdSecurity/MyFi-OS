package services

import (
	"context"
	"fmt"
)

// LLMProvider abstracts the underlying LLM backend.
type LLMProvider interface {
	// Complete sends a single user prompt and returns the text response.
	Complete(ctx context.Context, prompt string) (string, error)
	// ProviderName returns a short display name (e.g. "Ollama", "Claude").
	ProviderName() string
}

// NewLLMProvider builds the provider selected by the LLM_PROVIDER env var.
// "anthropic" requires a non-empty anthropicKey.
// Everything else (or empty) defaults to Ollama.
// ollamaNumThread pins Ollama's CPU threads (0 = Ollama's default).
func NewLLMProvider(provider, ollamaURL, ollamaModel string, ollamaNumThread int, anthropicKey, anthropicModel string) (LLMProvider, error) {
	switch provider {
	case "anthropic":
		if anthropicKey == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY is required when LLM_PROVIDER=anthropic")
		}
		return newAnthropicProvider(anthropicKey, anthropicModel), nil
	default:
		return newOllamaProvider(ollamaURL, ollamaModel, ollamaNumThread), nil
	}
}
