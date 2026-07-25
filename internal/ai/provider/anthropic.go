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

// anthropic speaks the native Messages API.
type anthropic struct {
	name    string
	baseURL string
	apiKey  string
	http    *http.Client
}

func newAnthropic(cfg Config) *anthropic {
	base := cfg.BaseURL
	if base == "" {
		base = "https://api.anthropic.com"
	}
	name := cfg.Name
	if name == "" {
		name = "anthropic"
	}
	return &anthropic{
		name: name, baseURL: base, apiKey: cfg.APIKey,
		http: &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *anthropic) Name() string { return p.name }

func (p *anthropic) Complete(ctx context.Context, req Request) (*Completion, error) {
	// Messages API separates system from the turn list.
	var system string
	msgs := make([]map[string]string, 0, len(req.Messages))
	for _, m := range req.Messages {
		if m.Role == RoleSystem {
			if system != "" {
				system += "\n\n"
			}
			system += m.Content
			continue
		}
		msgs = append(msgs, map[string]string{"role": string(m.Role), "content": m.Content})
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}
	payload := map[string]any{
		"model": req.Model, "max_tokens": maxTokens, "messages": msgs,
	}
	if system != "" {
		payload["system"] = system
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("x-api-key", p.apiKey)
	hreq.Header.Set("anthropic-version", "2023-06-01")

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
		return nil, fmt.Errorf("llm %s: status %d", p.name, resp.StatusCode)
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("llm %s: decode: %w", p.name, err)
	}
	text := ""
	for _, c := range out.Content {
		if c.Type == "text" {
			text += c.Text
		}
	}
	return &Completion{
		Text:  text,
		Usage: Usage{TokensIn: out.Usage.InputTokens, TokensOut: out.Usage.OutputTokens},
	}, nil
}

func (p *anthropic) Ping(ctx context.Context) error {
	// Minimal-cost validation: a 1-token request. A 401 means bad key.
	_, err := p.Complete(ctx, Request{
		Model: "claude-haiku-4-5-20251001", MaxTokens: 1,
		Messages: []Message{{Role: RoleUser, Content: "ping"}},
	})
	return err
}
