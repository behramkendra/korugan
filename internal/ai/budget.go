package ai

import (
	"context"
	"errors"
	"time"
)

// ErrBudgetExhausted is returned when a task would exceed the configured
// spend cap. Rule-based analysis and previously-approved actions are
// unaffected — only new LLM calls stop.
var ErrBudgetExhausted = errors.New("llm budget exhausted")

// Budget caps LLM spend per workspace. Zero means unlimited. A fraction of
// the cap is reserved for interactive chat so a burned batch budget never
// removes the ability to ask what happened.
type Budget struct {
	DailyUSD   float64
	MonthlyUSD float64
	// ChatReserveFraction (0..1) of the cap that only chat may use; batch
	// tasks stop at cap*(1-fraction). Default 0.1 when a budget is set.
	ChatReserveFraction float64
}

// SpendReporter reports LLM cost already recorded since a time.
type SpendReporter interface {
	SpendSince(ctx context.Context, since time.Time) (float64, error)
}

// SetBudget enables enforcement. spend may be the store; nil disables it.
func (e *Engine) SetBudget(b Budget, spend SpendReporter) {
	if b.ChatReserveFraction <= 0 || b.ChatReserveFraction >= 1 {
		b.ChatReserveFraction = 0.1
	}
	e.budget = b
	e.spend = spend
}

// checkBudget returns nil when the task may proceed. Chat may use the full
// cap; batch tasks stop at cap*(1-reserve), keeping headroom for chat.
func (e *Engine) checkBudget(ctx context.Context, tc TaskClass) error {
	if e.spend == nil || (e.budget.DailyUSD == 0 && e.budget.MonthlyUSD == 0) {
		return nil
	}
	now := time.Now().UTC()
	reserve := e.budget.ChatReserveFraction
	factor := 1.0
	if tc != TaskChat {
		factor = 1 - reserve
	}

	if e.budget.DailyUSD > 0 {
		startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		spent, err := e.spend.SpendSince(ctx, startOfDay)
		if err != nil {
			return err
		}
		if spent >= e.budget.DailyUSD*factor {
			return ErrBudgetExhausted
		}
	}
	if e.budget.MonthlyUSD > 0 {
		startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		spent, err := e.spend.SpendSince(ctx, startOfMonth)
		if err != nil {
			return err
		}
		if spent >= e.budget.MonthlyUSD*factor {
			return ErrBudgetExhausted
		}
	}
	return nil
}

// estCost computes USD from token usage and an assignment's price table.
func estCost(a Assignment, in, out int64) float64 {
	return float64(in)/1000*a.PriceInPer1K + float64(out)/1000*a.PriceOutPer1K
}
