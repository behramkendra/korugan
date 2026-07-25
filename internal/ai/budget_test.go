package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/behramkendra/korugan/internal/ai/provider"
)

type fakeSpend struct{ usd float64 }

func (f *fakeSpend) SpendSince(context.Context, time.Time) (float64, error) { return f.usd, nil }

func engineWithBudget(t *testing.T, b Budget, spent float64) *Engine {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": "ok"}}},
			"usage":   map[string]any{"prompt_tokens": 1000, "completion_tokens": 1000},
		})
	}))
	t.Cleanup(srv.Close)
	p, _ := provider.New(provider.Config{Kind: provider.KindOpenAICompatible, Name: "p", BaseURL: srv.URL, APIKey: "k"})
	a := Assignment{Provider: p, Model: "m", PriceInPer1K: 0.001, PriceOutPer1K: 0.002}
	e := NewEngine(map[Tier]Assignment{TierFast: a, TierBalanced: a, TierDeep: a}, nil)
	e.SetBudget(b, &fakeSpend{usd: spent})
	return e
}

func TestBudgetBlocksBatchButAllowsChat(t *testing.T) {
	// cap 10/day, 10% reserved for chat → batch stops at 9, chat at 10.
	e := engineWithBudget(t, Budget{DailyUSD: 10}, 9.5) // spent past batch cap, under chat cap

	if _, err := e.Chat(context.Background(), TaskSummarizeEvents, "digest", nil); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("batch task must be blocked at 9.5/9 cap, got %v", err)
	}
	if _, err := e.Chat(context.Background(), TaskChat, "what happened?", nil); err != nil {
		t.Fatalf("chat must still run within reserved slice: %v", err)
	}
}

func TestBudgetHardStopChat(t *testing.T) {
	e := engineWithBudget(t, Budget{DailyUSD: 10}, 10.0)
	if _, err := e.Chat(context.Background(), TaskChat, "hi", nil); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("chat must stop at full cap, got %v", err)
	}
}

func TestBudgetUnlimitedByDefault(t *testing.T) {
	e := engineWithBudget(t, Budget{}, 1e9)
	if _, err := e.Chat(context.Background(), TaskChat, "hi", nil); err != nil {
		t.Fatalf("no budget set → unlimited, got %v", err)
	}
}

func TestEstCost(t *testing.T) {
	a := Assignment{PriceInPer1K: 0.001, PriceOutPer1K: 0.002}
	if got := estCost(a, 1000, 500); got != 0.001+0.001 {
		t.Fatalf("est cost wrong: %v", got)
	}
}
