package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// openaiCompatible speaks the /chat/completions dialect shared by OpenAI,
// DeepSeek, OpenRouter and Ollama. Base URL decides the vendor.
type openaiCompatible struct {
	name    string
	baseURL string
	apiKey  string
	http    *http.Client
}

func newOpenAICompatible(cfg Config) *openaiCompatible {
	base := cfg.BaseURL
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	return &openaiCompatible{
		name: cfg.Name, baseURL: base, apiKey: cfg.APIKey,
		http: &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *openaiCompatible) Name() string { return p.name }

func (p *openaiCompatible) Complete(ctx context.Context, req Request) (*Completion, error) {
	body, err := json.Marshal(map[string]any{
		"model":      req.Model,
		"messages":   req.Messages,
		"max_tokens": req.MaxTokens,
	})
	if err != nil {
		return nil, err
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		hreq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := p.http.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		// Body is vendor error JSON; status alone avoids echoing anything sensitive.
		return nil, fmt.Errorf("llm %s: status %d", p.name, resp.StatusCode)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("llm %s: decode: %w", p.name, err)
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("llm %s: empty choices", p.name)
	}
	return &Completion{
		Text:  out.Choices[0].Message.Content,
		Usage: Usage{TokensIn: out.Usage.PromptTokens, TokensOut: out.Usage.CompletionTokens},
	}, nil
}

func (p *openaiCompatible) Ping(ctx context.Context) error {
	hreq, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/models", nil)
	if err != nil {
		return err
	}
	if p.apiKey != "" {
		hreq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := p.http.Do(hreq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("llm %s: ping status %d", p.name, resp.StatusCode)
	}
	return nil
}
