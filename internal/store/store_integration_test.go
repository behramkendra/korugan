package store

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/behramkendra/korugan/internal/domain"
)

// Integration tests run only when DATABASE_URL_TEST is set (CI provides a
// disposable Postgres; locally use any scratch database — tables are
// created by migrations, data is namespaced by generated IDs).
func testStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL_TEST")
	if dsn == "" {
		t.Skip("DATABASE_URL_TEST not set; skipping store integration tests")
	}
	s, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestEventInsertDedupAndList(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	ref := domain.ResourceRef{Provider: domain.ProviderCloudflare, Kind: "zone", ExternalID: NewID(), Name: "it.example.com"}
	resID, err := s.UpsertResource(ctx, ref)
	if err != nil {
		t.Fatalf("upsert resource: %v", err)
	}

	ts := time.Now().UTC().Truncate(time.Second)
	evs := []domain.Event{
		{ProviderEventID: resID + "-e1", Resource: ref, Category: domain.CatWAFBlock, Severity: domain.SevMedium, TS: ts},
		{ProviderEventID: resID + "-e2", Resource: ref, Category: domain.CatWAFBlock, Severity: domain.SevMedium, TS: ts.Add(time.Second)},
	}
	n, err := s.InsertEvents(ctx, resID, evs)
	if err != nil || n != 2 {
		t.Fatalf("insert: n=%d err=%v", n, err)
	}
	// same batch again: full dedup
	n, err = s.InsertEvents(ctx, resID, evs)
	if err != nil || n != 0 {
		t.Fatalf("dedup failed: n=%d err=%v", n, err)
	}

	got, err := s.ListEvents(ctx, EventFilter{ResourceID: resID})
	if err != nil || len(got) != 2 {
		t.Fatalf("list: len=%d err=%v", len(got), err)
	}
	if got[0].TS.Before(got[1].TS) {
		t.Fatal("events must come newest-first")
	}

	counts, err := s.CountEventsByCategory(ctx, resID, ts.Add(-time.Minute), ts.Add(time.Minute))
	if err != nil || counts[domain.CatWAFBlock] != 2 {
		t.Fatalf("counts: %v err=%v", counts, err)
	}
}

func TestFindingUpsertRefreshesOpen(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	ref := domain.ResourceRef{Provider: domain.ProviderCloudflare, Kind: "zone", ExternalID: NewID(), Name: "f.example.com"}
	resID, err := s.UpsertResource(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}

	f := domain.Finding{Resource: ref, Kind: "cert_expiry", Severity: domain.SevMedium, Title: "cert expires in 20d", Source: "rule"}
	id1, err := s.UpsertOpenFinding(ctx, resID, f)
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	f.Severity = domain.SevHigh
	f.Title = "cert expires in 5d"
	id2, err := s.UpsertOpenFinding(ctx, resID, f)
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("open finding must refresh in place: %s != %s", id1, id2)
	}

	rows, err := s.ListFindings(ctx, "open", 50)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range rows {
		if r.ID == id1 {
			found = true
			if r.Severity != domain.SevHigh || r.Title != "cert expires in 5d" {
				t.Fatalf("finding not refreshed: %+v", r)
			}
		}
	}
	if !found {
		t.Fatal("upserted finding missing from list")
	}
}

func TestRecommendationAndActionRoundTrip(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	ref := domain.ResourceRef{Provider: domain.ProviderCloudflare, Kind: "zone", ExternalID: NewID(), Name: "act.example.com"}
	resID, err := s.UpsertResource(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	findingID, err := s.UpsertOpenFinding(ctx, resID, domain.Finding{
		Resource: ref, Kind: "blocked_traffic_spike", Severity: domain.SevHigh,
		Title: "spike", Source: "rule",
	})
	if err != nil {
		t.Fatal(err)
	}

	recID, err := s.InsertRecommendation(ctx, domain.Recommendation{
		FindingID: findingID, Resource: ref, ActionType: domain.ActWAFRuleCreate,
		Params:    map[string]any{"expression": `(http.request.uri.path eq "/wp-login.php")`, "action": "managed_challenge"},
		Rationale: "challenge login brute force", Rollback: "delete the created rule", Confidence: 0.8,
	})
	if err != nil {
		t.Fatalf("insert recommendation: %v", err)
	}

	got, err := s.GetRecommendation(ctx, recID)
	if err != nil || got == nil {
		t.Fatalf("get recommendation: %v", err)
	}
	if got.ActionType != domain.ActWAFRuleCreate || got.Resource.ExternalID != ref.ExternalID {
		t.Fatalf("recommendation round trip wrong: %+v", got)
	}
	if got.Params["action"] != "managed_challenge" {
		t.Fatalf("params not preserved: %+v", got.Params)
	}

	a := domain.Action{
		ID: NewID(), Type: domain.ActWAFRuleCreate, Resource: ref,
		Params: got.Params, State: domain.ActionApproved, RecommendationID: recID,
		ApprovedBy: "alice", IdempotencyKey: NewID(), AutonomyLevel: domain.L2,
	}
	if err := s.InsertAction(ctx, resID, a); err != nil {
		t.Fatalf("insert action: %v", err)
	}
	if err := s.SetActionState(ctx, a.ID, domain.ActionApplied, map[string]any{"provider_ref": "rule_1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetActionState(ctx, a.ID, domain.ActionVerified, nil); err != nil {
		t.Fatal(err)
	}
	row, err := s.GetAction(ctx, a.ID)
	if err != nil || row == nil {
		t.Fatalf("get action: %v", err)
	}
	if row.State != domain.ActionVerified {
		t.Fatalf("state not advanced: %s", row.State)
	}
	// COALESCE must preserve the apply result through the nil-result verify update
	if len(row.Result) == 0 || !strings.Contains(string(row.Result), "rule_1") {
		t.Fatalf("apply result not preserved through verify: %s", row.Result)
	}
	if row.ApprovedBy == nil || *row.ApprovedBy != "alice" {
		t.Fatalf("approver not stored: %+v", row.ApprovedBy)
	}
}

func TestCursorRoundTrip(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ext := NewID()

	cur, err := s.GetCursor(ctx, domain.ProviderCloudflare, ext, "firewall_events")
	if err != nil || cur != "" {
		t.Fatalf("empty cursor expected, got %q err=%v", cur, err)
	}
	if err := s.SetCursor(ctx, domain.ProviderCloudflare, ext, "firewall_events", "2026-07-25T00:00:00Z|p2"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCursor(ctx, domain.ProviderCloudflare, ext, "firewall_events", "2026-07-25T01:00:00Z|p1"); err != nil {
		t.Fatal(err)
	}
	cur, err = s.GetCursor(ctx, domain.ProviderCloudflare, ext, "firewall_events")
	if err != nil || cur != "2026-07-25T01:00:00Z|p1" {
		t.Fatalf("cursor round trip failed: %q err=%v", cur, err)
	}
}

func TestSecretRoundTrip(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	name := "test-" + NewID()

	ct, nonce, err := s.GetSecret(ctx, name)
	if err != nil || ct != nil || nonce != nil {
		t.Fatalf("missing secret must return nils: %v %v %v", ct, nonce, err)
	}
	if err := s.PutSecret(ctx, name, []byte{1, 2, 3}, []byte{9, 9}); err != nil {
		t.Fatal(err)
	}
	ct, nonce, err = s.GetSecret(ctx, name)
	if err != nil || string(ct) != "\x01\x02\x03" || len(nonce) != 2 {
		t.Fatalf("secret round trip failed: %v %v %v", ct, nonce, err)
	}
}
