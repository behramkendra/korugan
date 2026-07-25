// Package cloudflare is the first Korugan connector: read paths for zones,
// DNS, settings and firewall events. Write paths (Apply/DryRun) arrive in
// the recommend-and-apply phase.
package cloudflare

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/behramkendra/korugan/internal/connector"
	"github.com/behramkendra/korugan/internal/domain"
)

func init() {
	connector.Register(domain.ProviderCloudflare, func(creds map[string]string) (connector.Connector, error) {
		token := creds["api_token"]
		if token == "" {
			return nil, fmt.Errorf("cloudflare: api_token credential is required")
		}
		return New(token, "", nil), nil
	})
}

type Connector struct {
	c *client
}

// New builds the connector. baseURL and hc are overridable for tests.
func New(token, baseURL string, hc *http.Client) *Connector {
	return &Connector{c: newClient(token, baseURL, hc)}
}

func (cf *Connector) Info() connector.ProviderInfo {
	return connector.ProviderInfo{
		Provider: domain.ProviderCloudflare,
		Label:    "Cloudflare",
		DocsURL:  "https://developers.cloudflare.com/api/",
		AuthSpec: []connector.AuthField{
			{Name: "api_token", Label: "API Token (least-privilege, read scopes)", Secret: true},
		},
	}
}

func (cf *Connector) Capabilities(ctx context.Context) ([]domain.Capability, error) {
	// v0.1 ships read-only regardless of token scopes.
	return []domain.Capability{
		domain.CapDNSRead, domain.CapWAFRead, domain.CapAnalyticsRead,
		domain.CapEventsRead, domain.CapSSLRead, domain.CapConfigRead,
	}, nil
}

// Validate checks the token without side effects.
func (cf *Connector) Validate(ctx context.Context) error {
	env, err := cf.c.get(ctx, "/user/tokens/verify")
	if err != nil {
		return fmt.Errorf("token verify failed: %w", err)
	}
	var res struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(env.Result, &res); err != nil {
		return err
	}
	if res.Status != "active" {
		return fmt.Errorf("cloudflare token status %q, want active", res.Status)
	}
	return nil
}

// Resources lists zones, paging through the REST API.
func (cf *Connector) Resources(ctx context.Context) ([]domain.ResourceRef, error) {
	var out []domain.ResourceRef
	for page := 1; ; page++ {
		env, err := cf.c.get(ctx, fmt.Sprintf("/zones?page=%d&per_page=50", page))
		if err != nil {
			return nil, err
		}
		var zones []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(env.Result, &zones); err != nil {
			return nil, err
		}
		for _, z := range zones {
			out = append(out, domain.ResourceRef{
				Provider: domain.ProviderCloudflare, Kind: "zone", ExternalID: z.ID, Name: z.Name,
			})
		}
		if env.ResultInfo == nil || page >= env.ResultInfo.TotalPages {
			break
		}
	}
	return out, nil
}

// Snapshot pulls DNS records and selected zone facts.
func (cf *Connector) Snapshot(ctx context.Context, res domain.ResourceRef) (*connector.Snapshot, error) {
	snap := &connector.Snapshot{Resource: res, TakenAt: time.Now().UTC(), Settings: map[string]any{}}

	for page := 1; ; page++ {
		env, err := cf.c.get(ctx, fmt.Sprintf("/zones/%s/dns_records?page=%d&per_page=100", res.ExternalID, page))
		if err != nil {
			return nil, err
		}
		var recs []struct {
			ID      string `json:"id"`
			Type    string `json:"type"`
			Name    string `json:"name"`
			Content string `json:"content"`
			Proxied bool   `json:"proxied"`
			TTL     int    `json:"ttl"`
		}
		if err := json.Unmarshal(env.Result, &recs); err != nil {
			return nil, err
		}
		for _, r := range recs {
			snap.DNSRecords = append(snap.DNSRecords, connector.DNSRecord(r))
		}
		if env.ResultInfo == nil || page >= env.ResultInfo.TotalPages {
			break
		}
	}

	// Zone status/plan/ssl mode: cheap facts analyzers use for drift checks.
	env, err := cf.c.get(ctx, "/zones/"+res.ExternalID)
	if err != nil {
		return nil, err
	}
	var zone struct {
		Status string `json:"status"`
		Paused bool   `json:"paused"`
	}
	if err := json.Unmarshal(env.Result, &zone); err == nil {
		snap.Settings["zone_status"] = zone.Status
		snap.Settings["paused"] = zone.Paused
	}
	return snap, nil
}
