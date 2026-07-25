// Package ingest drives the sync loop: connectors → normalizer → store,
// then hands each resource to the analysis runner.
package ingest

import (
	"context"
	"log/slog"
	"time"

	"github.com/behramkendra/korugan/internal/analysis"
	"github.com/behramkendra/korugan/internal/connector"
	"github.com/behramkendra/korugan/internal/domain"
	"github.com/behramkendra/korugan/internal/store"
)

type Poller struct {
	Store    *store.Store
	Log      *slog.Logger
	Interval time.Duration
	Analysis *analysis.Runner

	// Connectors are constructed at startup from stored credentials.
	Connectors []connector.Connector
}

// Run blocks until ctx is done, syncing on every tick. The first sync
// happens immediately so a fresh install shows data without waiting.
func (p *Poller) Run(ctx context.Context) {
	t := time.NewTicker(p.Interval)
	defer t.Stop()
	p.syncAll(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.syncAll(ctx)
		}
	}
}

func (p *Poller) syncAll(ctx context.Context) {
	for _, c := range p.Connectors {
		if err := p.syncConnector(ctx, c); err != nil {
			p.Log.Error("sync failed", "provider", c.Info().Provider, "err", err)
		}
	}
}

func (p *Poller) syncConnector(ctx context.Context, c connector.Connector) error {
	refs, err := c.Resources(ctx)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		resourceID, err := p.Store.UpsertResource(ctx, ref)
		if err != nil {
			p.Log.Error("resource upsert failed", "resource", ref.String(), "err", err)
			continue
		}
		snap := p.syncSnapshot(ctx, c, ref)
		p.syncEvents(ctx, c, ref, resourceID)
		if p.Analysis != nil {
			p.Analysis.Run(ctx, resourceID, ref, snap)
		}
	}
	return nil
}

func (p *Poller) syncSnapshot(ctx context.Context, c connector.Connector, ref domain.ResourceRef) *connector.Snapshot {
	snap, err := c.Snapshot(ctx, ref)
	if err != nil {
		p.Log.Warn("snapshot failed", "resource", ref.String(), "err", err)
		return nil
	}
	return snap
}

func (p *Poller) syncEvents(ctx context.Context, c connector.Connector, ref domain.ResourceRef, resourceID string) {
	const stream = "firewall_events"
	cur, err := p.Store.GetCursor(ctx, ref.Provider, ref.ExternalID, stream)
	if err != nil {
		p.Log.Warn("cursor read failed", "resource", ref.String(), "err", err)
		return
	}
	// Drain pages until the connector reports done — bounded per tick so a
	// backlogged zone cannot starve the loop.
	for pages := 0; pages < 20; pages++ {
		page, err := c.Events(ctx, ref, connector.Cursor(cur), connector.EventFilter{})
		if err != nil {
			p.Log.Warn("event pull failed", "resource", ref.String(), "err", err)
			return
		}
		if len(page.Events) > 0 {
			inserted, err := p.Store.InsertEvents(ctx, resourceID, page.Events)
			if err != nil {
				p.Log.Error("event insert failed", "resource", ref.String(), "err", err)
				return
			}
			p.Log.Info("events ingested", "resource", ref.Name, "pulled", len(page.Events), "new", inserted)
		}
		if string(page.Next) != "" && string(page.Next) != cur {
			cur = string(page.Next)
			if err := p.Store.SetCursor(ctx, ref.Provider, ref.ExternalID, stream, cur); err != nil {
				p.Log.Warn("cursor save failed", "resource", ref.String(), "err", err)
			}
		}
		if page.Done {
			return
		}
	}
}
