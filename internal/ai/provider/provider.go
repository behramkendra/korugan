// Package provider abstracts LLM vendors behind one port. Two adapters
// cover the launch set: openai-compatible (OpenAI, DeepSeek, OpenRouter,
// Ollama) and anthropic. BYOK: keys are the user's, always.
package provider

import (
	"context"
	"errors"
	"fmt"
)

// ErrNoKey signals zero-key mode: callers degrade AI features, never crash.
var ErrNoKey = errors.New("llm: no api key configured")

type Role string

const (
	RoleSystem Role = "system"
	RoleUser   Role = "user"
	RoleModel  Role = "assistant"
)

type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	Model     string
	Messages  []Message
	MaxTokens int
}

type Usage struct {
	TokensIn  int64
	TokensOut int64
}

type Completion struct {
	Text  string
	Usage Usage
}

// LLMProvider is one vendor account (one key).
type LLMProvider interface {
	Name() string
	Complete(ctx context.Context, req Request) (*Completion, error)
	Ping(ctx context.Context) error
}

// Kind selects an adapter; base URL differentiates compatible vendors.
type Kind string

const (
	KindOpenAICompatible Kind = "openai-compatible"
	KindAnthropic        Kind = "anthropic"
)

type Config struct {
	Kind    Kind
	Name    string // display: "openrouter", "deepseek", ...
	BaseURL string
	APIKey  string
}

func New(cfg Config) (LLMProvider, error) {
	if cfg.APIKey == "" && cfg.Kind != KindOpenAICompatible {
		return nil, ErrNoKey
	}
	switch cfg.Kind {
	case KindOpenAICompatible:
		// Ollama runs keyless; other compatible vendors require one.
		return newOpenAICompatible(cfg), nil
	case KindAnthropic:
		return newAnthropic(cfg), nil
	}
	return nil, fmt.Errorf("llm: unknown provider kind %q", cfg.Kind)
}
