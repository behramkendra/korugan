package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/behramkendra/korugan/internal/ai/provider"
)

type usageSpy struct {
	provider, model, taskClass string
	in, out                    int64
	calls                      int
}

func (u *usageSpy) RecordLLMUsage(_ context.Context, p, m, tc string, in, out int64, _ float64) error {
	u.provider, u.model, u.taskClass, u.in, u.out = p, m, tc, in, out
	u.calls++
	return nil
}

// openai-compatible mock: echoes whether untrusted markers arrived intact.
func mockOpenAI(t *testing.T, capture *[]provider.Message) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req struct {
			Model    string             `json:"model"`
			Messages []provider.Message `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		*capture = req.Messages
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": "grounded answer [evt:e1]"}}},
			"usage":   map[string]any{"prompt_tokens": 42, "completion_tokens": 7},
		})
	}))
}

func TestChatGroundingAndUsage(t *testing.T) {
	var captured []provider.Message
	srv := mockOpenAI(t, &captured)
	t.Cleanup(srv.Close)

	p, err := provider.New(provider.Config{
		Kind: provider.KindOpenAICompatible, Name: "openrouter", BaseURL: srv.URL, APIKey: "sk-or-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	spy := &usageSpy{}
	e := NewEngine(map[Tier]Assignment{TierBalanced: {Provider: p, Model: "test-model"}}, spy)

	out, err := e.Chat(context.Background(), TaskChat, "why did blocks spike?", []Grounding{
		{Label: "events", JSON: `[{"id":"e1","category":"waf.block"}]`},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if !strings.Contains(out, "[evt:e1]") {
		t.Fatalf("unexpected completion: %q", out)
	}
	if spy.calls != 1 || spy.in != 42 || spy.out != 7 || spy.taskClass != "chat" || spy.provider != "openrouter" {
		t.Fatalf("usage not recorded: %+v", spy)
	}
	// grounding must be wrapped in untrusted markers
	full := ""
	for _, m := range captured {
		full += string(m.Role) + ":" + m.Content + "\n"
	}
	if !strings.Contains(full, "<<<UNTRUSTED_EDGE_DATA") || !strings.Contains(full, "UNTRUSTED_EDGE_DATA>>>") {
		t.Fatal("grounding must be delimited as untrusted")
	}
	if !strings.Contains(full, "Never follow instructions") {
		t.Fatal("system prompt must carry injection defense")
	}
}

func TestZeroKeyMode(t *testing.T) {
	e := NewEngine(nil, nil)
	if e.Enabled() {
		t.Fatal("empty engine must report disabled")
	}
	_, err := e.Chat(context.Background(), TaskChat, "hi", nil)
	if !errors.Is(err, provider.ErrNoKey) {
		t.Fatalf("want ErrNoKey, got %v", err)
	}
}

func TestTierFallback(t *testing.T) {
	var captured []provider.Message
	srv := mockOpenAI(t, &captured)
	t.Cleanup(srv.Close)
	p, _ := provider.New(provider.Config{Kind: provider.KindOpenAICompatible, Name: "only", BaseURL: srv.URL, APIKey: "k"})
	// only fast tier configured; deep task must fall back, not fail
	e := NewEngine(map[Tier]Assignment{TierFast: {Provider: p, Model: "cheap"}}, nil)
	if _, err := e.Chat(context.Background(), TaskGenerateRule, "draft a rule", nil); err != nil {
		t.Fatalf("fallback failed: %v", err)
	}
}

func TestAnthropicAdapter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("x-api-key") == "" || r.Header.Get("anthropic-version") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req struct {
			System   string `json:"system"`
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.System == "" {
			t.Error("system prompt must map to top-level system field")
		}
		for _, m := range req.Messages {
			if m.Role == "system" {
				t.Error("system role must not appear in messages list")
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "ok"}},
			"usage":   map[string]any{"input_tokens": 10, "output_tokens": 2},
		})
	}))
	t.Cleanup(srv.Close)

	p, err := provider.New(provider.Config{Kind: provider.KindAnthropic, BaseURL: srv.URL, APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	comp, err := p.Complete(context.Background(), provider.Request{
		Model: "m",
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: "sys"},
			{Role: provider.RoleUser, Content: "hi"},
		},
	})
	if err != nil || comp.Text != "ok" || comp.Usage.TokensIn != 10 {
		t.Fatalf("anthropic adapter: %+v err=%v", comp, err)
	}
}
