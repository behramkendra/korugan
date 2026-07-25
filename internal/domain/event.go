package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

// Category is the normalized event taxonomy: lowercase, dot-namespaced.
// Adding a category is a documented change to CONNECTORS.md first;
// connectors must not invent ad-hoc categories.
type Category string

const (
	CatWAFBlock         Category = "waf.block"
	CatWAFChallenge     Category = "waf.challenge"
	CatWAFLog           Category = "waf.log"
	CatRatelimitHit     Category = "ratelimit.hit"
	CatBotDetected      Category = "bot.detected"
	CatOriginError      Category = "origin.error"
	CatOriginTimeout    Category = "origin.timeout"
	CatCacheMissSpike   Category = "cache.miss_spike"
	CatCachePurge       Category = "cache.purge"
	CatDNSChanged       Category = "dns.changed"
	CatSSLCertExpiring  Category = "ssl.cert_expiring"
	CatSSLHandshakeFail Category = "ssl.handshake_fail"
	CatConfigDrift      Category = "config.drift"
	CatTrafficAnomaly   Category = "traffic.anomaly"
	CatProviderIncident Category = "provider.incident"
)

var knownCategories = map[Category]struct{}{
	CatWAFBlock: {}, CatWAFChallenge: {}, CatWAFLog: {}, CatRatelimitHit: {},
	CatBotDetected: {}, CatOriginError: {}, CatOriginTimeout: {}, CatCacheMissSpike: {},
	CatCachePurge: {}, CatDNSChanged: {}, CatSSLCertExpiring: {}, CatSSLHandshakeFail: {},
	CatConfigDrift: {}, CatTrafficAnomaly: {}, CatProviderIncident: {},
}

func (c Category) Known() bool {
	_, ok := knownCategories[c]
	return ok
}

type Severity string

const (
	SevInfo     Severity = "info"
	SevLow      Severity = "low"
	SevMedium   Severity = "medium"
	SevHigh     Severity = "high"
	SevCritical Severity = "critical"
)

func (s Severity) Known() bool {
	switch s {
	case SevInfo, SevLow, SevMedium, SevHigh, SevCritical:
		return true
	}
	return false
}

// Actor describes who/what triggered an event, when the provider knows.
type Actor struct {
	IP        string `json:"ip,omitempty"`
	Country   string `json:"country,omitempty"`
	ASN       int64  `json:"asn,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
}

// Target describes what the event hit.
type Target struct {
	Host   string `json:"host,omitempty"`
	Path   string `json:"path,omitempty"`
	Method string `json:"method,omitempty"`
}

// Rule describes the provider rule involved, if any.
type Rule struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	ActionTaken string `json:"action_taken,omitempty"`
}

// Event is one normalized signal from a provider. Raw preserves the
// untouched provider payload for audit and future re-normalization.
type Event struct {
	ID              string          `json:"id"` // ULID, assigned by store
	ProviderEventID string          `json:"provider_event_id"`
	Resource        ResourceRef     `json:"resource"`
	Category        Category        `json:"category"`
	Severity        Severity        `json:"severity"`
	TS              time.Time       `json:"ts"`
	Actor           Actor           `json:"actor,omitempty"`
	Target          Target          `json:"target,omitempty"`
	Rule            Rule            `json:"rule,omitempty"`
	Fields          map[string]any  `json:"fields,omitempty"`
	Raw             json.RawMessage `json:"raw,omitempty"`
}

func (e Event) Validate() error {
	if err := e.Resource.Validate(); err != nil {
		return err
	}
	if !e.Category.Known() {
		return fmt.Errorf("unknown event category %q", e.Category)
	}
	if !e.Severity.Known() {
		return fmt.Errorf("unknown severity %q", e.Severity)
	}
	if e.TS.IsZero() {
		return fmt.Errorf("event timestamp is zero")
	}
	if e.ProviderEventID == "" {
		return fmt.Errorf("provider_event_id is required for dedup")
	}
	return nil
}
