package domain

import (
	"strings"
	"testing"
	"time"
)

func validEvent() Event {
	return Event{
		ProviderEventID: "cf-evt-1",
		Resource:        ResourceRef{Provider: ProviderCloudflare, Kind: "zone", ExternalID: "abc", Name: "example.com"},
		Category:        CatWAFBlock,
		Severity:        SevMedium,
		TS:              time.Date(2026, 7, 25, 3, 0, 0, 0, time.UTC),
	}
}

func TestEventValidate(t *testing.T) {
	if err := validEvent().Validate(); err != nil {
		t.Fatalf("valid event rejected: %v", err)
	}
	cases := []struct {
		name   string
		mutate func(*Event)
		want   string
	}{
		{"unknown category", func(e *Event) { e.Category = "waf.exploded" }, "unknown event category"},
		{"unknown severity", func(e *Event) { e.Severity = "mega" }, "unknown severity"},
		{"zero ts", func(e *Event) { e.TS = time.Time{} }, "timestamp is zero"},
		{"missing provider id", func(e *Event) { e.ProviderEventID = "" }, "provider_event_id"},
		{"incomplete resource", func(e *Event) { e.Resource.ExternalID = "" }, "resource ref incomplete"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := validEvent()
			c.mutate(&e)
			err := e.Validate()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error containing %q, got %v", c.want, err)
			}
		})
	}
}

func TestActionTypeRules(t *testing.T) {
	if err := ActionType("dns.record.delete").Validate(); err == nil {
		t.Fatal("dns.record.delete must be permanently forbidden")
	}
	if err := ActionType("zone.delete").Validate(); err == nil {
		t.Fatal("zone.delete must be permanently forbidden")
	}
	if err := ActionType("waf.rule.nuke").Validate(); err == nil {
		t.Fatal("unknown action types must be rejected")
	}
	if ActCachePurge.Reversible() {
		t.Fatal("cache.purge must be irreversible")
	}
	if !ActWAFRuleCreate.Reversible() {
		t.Fatal("waf.rule.create must be reversible")
	}
}

func TestAIFindingRequiresEvidence(t *testing.T) {
	f := Finding{
		Resource: ResourceRef{Provider: ProviderCloudflare, Kind: "zone", ExternalID: "abc", Name: "example.com"},
		Kind:     "ai.waf_pattern", Severity: SevHigh, Title: "t", Source: "ai",
	}
	if err := f.Validate(); err == nil || !strings.Contains(err.Error(), "evidence") {
		t.Fatalf("ai finding without evidence must fail, got %v", err)
	}
	f.Evidence = []string{"evt_1"}
	if err := f.Validate(); err != nil {
		t.Fatalf("valid ai finding rejected: %v", err)
	}
}

func TestGate(t *testing.T) {
	res := ResourceRef{Provider: ProviderCloudflare, Kind: "zone", ExternalID: "abc", Name: "example.com"}
	act := Action{Type: ActWAFRuleCreate, Resource: res, IdempotencyKey: "k"}
	purge := Action{Type: ActCachePurge, Resource: res, IdempotencyKey: "k2"}
	pol := &Policy{
		ID: "pol1", Resource: res, AllowedTypes: []ActionType{ActWAFRuleCreate},
		MaxPerHour: 5, Enabled: true,
	}

	if d := Gate(act, L0, nil); d.Allowed {
		t.Fatal("L0 must block execution")
	}
	if d := Gate(act, L1, nil); d.Allowed {
		t.Fatal("L1 must block execution")
	}
	if d := Gate(act, L2, nil); !d.Allowed || !d.NeedsApproval {
		t.Fatal("L2 must allow with approval")
	}
	if d := Gate(act, L3, pol); !d.Allowed || d.NeedsApproval {
		t.Fatalf("L3 within policy must skip approval: %+v", d)
	}
	if d := Gate(act, L3, nil); !d.NeedsApproval {
		t.Fatal("L3 without policy must degrade to approval")
	}
	if d := Gate(purge, L3, pol); !d.NeedsApproval {
		t.Fatal("irreversible action must always need approval")
	}
	if d := Gate(Action{Type: "dns.record.delete", Resource: res, IdempotencyKey: "k3"}, L3, pol); d.Allowed {
		t.Fatal("forbidden type must never pass the gate")
	}
}

func TestPolicyRejectsIrreversible(t *testing.T) {
	res := ResourceRef{Provider: ProviderCloudflare, Kind: "zone", ExternalID: "abc", Name: "example.com"}
	p := Policy{ID: "p", Resource: res, AllowedTypes: []ActionType{ActCachePurge}, MaxPerHour: 1, Enabled: true}
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "irreversible") {
		t.Fatalf("policy with cache.purge must be rejected, got %v", err)
	}
}
