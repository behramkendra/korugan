package analysis

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/behramkendra/korugan/internal/connector"
	"github.com/behramkendra/korugan/internal/domain"
	"github.com/behramkendra/korugan/internal/store"
)

// Runner executes rule-based analyzers for one resource after each sync.
type Runner struct {
	Store *store.Store
	Log   *slog.Logger
}

// Run applies every analyzer; failures are logged, never fatal —
// analysis must not break ingestion.
func (r *Runner) Run(ctx context.Context, resourceID string, ref domain.ResourceRef, snap *connector.Snapshot) {
	now := time.Now().UTC()
	r.blockedSpike(ctx, resourceID, ref, now)
	r.zonePaused(ctx, resourceID, ref, snap)
}

// blockedSpike: current hour of waf.block + ratelimit.hit vs the previous
// 24 one-hour buckets.
func (r *Runner) blockedSpike(ctx context.Context, resourceID string, ref domain.ResourceRef, now time.Time) {
	cur := now.Truncate(time.Hour)
	current, err := r.blockedCount(ctx, resourceID, cur, now)
	if err != nil {
		r.Log.Warn("analyzer blocked_spike aggregate failed", "err", err)
		return
	}
	history := make([]int64, 0, 24)
	for i := 1; i <= 24; i++ {
		from := cur.Add(-time.Duration(i) * time.Hour)
		n, err := r.blockedCount(ctx, resourceID, from, from.Add(time.Hour))
		if err != nil {
			r.Log.Warn("analyzer blocked_spike history failed", "err", err)
			return
		}
		history = append(history, n)
	}
	v := DetectSpike(current, history)
	if !v.Spike {
		return
	}
	f := domain.Finding{
		Resource: ref,
		Kind:     "blocked_traffic_spike",
		Severity: domain.SevHigh,
		Title:    fmt.Sprintf("Blocked traffic spike on %s", ref.Name),
		Detail: fmt.Sprintf("Blocked+rate-limited requests this hour: %d, baseline %.0f/h. %s",
			v.Current, v.Baseline, v.Detail),
		Source: "rule",
	}
	if _, err := r.Store.UpsertOpenFinding(ctx, resourceID, f); err != nil {
		r.Log.Warn("analyzer blocked_spike upsert failed", "err", err)
	}
}

func (r *Runner) blockedCount(ctx context.Context, resourceID string, from, to time.Time) (int64, error) {
	counts, err := r.Store.CountEventsByCategory(ctx, resourceID, from, to)
	if err != nil {
		return 0, err
	}
	return counts[domain.CatWAFBlock] + counts[domain.CatRatelimitHit], nil
}

// zonePaused: a paused zone serves origin-direct with no protection —
// almost always drift, occasionally intentional; medium either way.
func (r *Runner) zonePaused(ctx context.Context, resourceID string, ref domain.ResourceRef, snap *connector.Snapshot) {
	if snap == nil {
		return
	}
	paused, _ := snap.Settings["paused"].(bool)
	if !paused {
		return
	}
	f := domain.Finding{
		Resource: ref,
		Kind:     "zone_paused",
		Severity: domain.SevMedium,
		Title:    fmt.Sprintf("Zone %s is paused on Cloudflare", ref.Name),
		Detail:   "Traffic bypasses Cloudflare entirely while a zone is paused: no WAF, no caching, origin exposed.",
		Source:   "rule",
	}
	if _, err := r.Store.UpsertOpenFinding(ctx, resourceID, f); err != nil {
		r.Log.Warn("analyzer zone_paused upsert failed", "err", err)
	}
}
