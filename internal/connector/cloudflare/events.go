package cloudflare

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/behramkendra/korugan/internal/connector"
	"github.com/behramkendra/korugan/internal/domain"
)

const firewallEventsQuery = `
query ($zoneTag: string!, $start: Time!, $end: Time!, $limit: Int!) {
  viewer {
    zones(filter: {zoneTag: $zoneTag}) {
      firewallEventsAdaptive(
        filter: {datetime_geq: $start, datetime_leq: $end},
        limit: $limit,
        orderBy: [datetime_ASC]
      ) {
        action
        clientASN
        clientCountryName
        clientIP
        clientRequestHTTPHost
        clientRequestHTTPMethodName
        clientRequestPath
        datetime
        rayName
        ruleId
        source
        userAgent
      }
    }
  }
}`

type fwEvent struct {
	Action    string `json:"action"`
	ClientASN string `json:"clientASN"`
	Country   string `json:"clientCountryName"`
	ClientIP  string `json:"clientIP"`
	Host      string `json:"clientRequestHTTPHost"`
	Method    string `json:"clientRequestHTTPMethodName"`
	Path      string `json:"clientRequestPath"`
	Datetime  string `json:"datetime"`
	RayName   string `json:"rayName"`
	RuleID    string `json:"ruleId"`
	Source    string `json:"source"`
	UserAgent string `json:"userAgent"`
}

// Events pulls firewall events after cur (RFC3339 datetime cursor).
// Duplicate protection is layered: cursor is inclusive (datetime_geq) so
// boundary events re-appear, and the store dedups on provider_event_id.
func (cf *Connector) Events(ctx context.Context, res domain.ResourceRef, cur connector.Cursor, f connector.EventFilter) (connector.EventPage, error) {
	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	start := f.Since
	if cur != "" {
		if t, err := time.Parse(time.RFC3339Nano, string(cur)); err == nil {
			start = t
		}
	}
	if start.IsZero() {
		start = time.Now().UTC().Add(-24 * time.Hour)
	}
	end := f.Until
	if end.IsZero() {
		end = time.Now().UTC()
	}

	data, err := cf.c.gql(ctx, firewallEventsQuery, map[string]any{
		"zoneTag": res.ExternalID,
		"start":   start.UTC().Format(time.RFC3339),
		"end":     end.UTC().Format(time.RFC3339),
		"limit":   limit,
	})
	if err != nil {
		return connector.EventPage{}, err
	}

	var payload struct {
		Viewer struct {
			Zones []struct {
				FirewallEventsAdaptive []fwEvent `json:"firewallEventsAdaptive"`
			} `json:"zones"`
		} `json:"viewer"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return connector.EventPage{}, fmt.Errorf("firewall events decode: %w", err)
	}
	if len(payload.Viewer.Zones) == 0 {
		return connector.EventPage{Done: true, Next: cur}, nil
	}

	raw := payload.Viewer.Zones[0].FirewallEventsAdaptive
	events := make([]domain.Event, 0, len(raw))
	var lastTS time.Time
	for _, fe := range raw {
		ts, err := time.Parse(time.RFC3339Nano, fe.Datetime)
		if err != nil {
			continue // never let one malformed row poison the batch
		}
		if ts.After(lastTS) {
			lastTS = ts
		}
		rawJSON, _ := json.Marshal(fe)
		events = append(events, domain.Event{
			ProviderEventID: fmt.Sprintf("%s:%s:%s", fe.RayName, fe.RuleID, fe.Action),
			Resource:        res,
			Category:        categoryFor(fe),
			Severity:        severityFor(fe.Action),
			TS:              ts,
			Actor:           domain.Actor{IP: fe.ClientIP, Country: fe.Country, UserAgent: fe.UserAgent, ASN: asn(fe.ClientASN)},
			Target:          domain.Target{Host: fe.Host, Path: fe.Path, Method: fe.Method},
			Rule:            domain.Rule{ID: fe.RuleID, ActionTaken: fe.Action},
			Fields:          map[string]any{"source": fe.Source},
			Raw:             rawJSON,
		})
	}

	next := cur
	if !lastTS.IsZero() {
		next = connector.Cursor(lastTS.UTC().Format(time.RFC3339Nano))
	}
	// A full page means there may be more in the window.
	return connector.EventPage{Events: events, Next: next, Done: len(raw) < limit}, nil
}

func categoryFor(fe fwEvent) domain.Category {
	if fe.Source == "ratelimit" {
		return domain.CatRatelimitHit
	}
	switch fe.Action {
	case "block":
		return domain.CatWAFBlock
	case "challenge", "jschallenge", "managed_challenge", "interactive_challenge":
		return domain.CatWAFChallenge
	default:
		return domain.CatWAFLog
	}
}

func severityFor(action string) domain.Severity {
	switch action {
	case "block":
		return domain.SevMedium
	case "challenge", "jschallenge", "managed_challenge", "interactive_challenge":
		return domain.SevLow
	default:
		return domain.SevInfo
	}
}

func asn(s string) int64 {
	var n int64
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int64(r-'0')
	}
	return n
}
