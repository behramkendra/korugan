package cloudflare

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/behramkendra/korugan/internal/connector"
	"github.com/behramkendra/korugan/internal/domain"
)

// rulesetState is a tiny in-memory Cloudflare rulesets fake: enough to
// exercise create → verify → rollback and mutate → rollback round-trips.
type rulesetState struct {
	mu    sync.Mutex
	id    string
	rules []cfRule
	seq   int
}

func newRulesetFake() *rulesetState {
	return &rulesetState{id: "rs_entry"}
}

func (s *rulesetState) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	prefix := "/zones/z1/rulesets"

	mux.HandleFunc("/zones/z1/rulesets/phases/http_request_firewall_custom/entrypoint", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		writeResult(w, cfRuleset{ID: s.id, Rules: append([]cfRule{}, s.rules...)})
	})

	mux.HandleFunc(prefix+"/rs_entry/rules", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		var rule cfRule
		json.NewDecoder(r.Body).Decode(&rule)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.seq++
		rule.ID = "rule_" + itoa(s.seq)
		s.rules = append(s.rules, rule)
		writeResult(w, cfRuleset{ID: s.id, Rules: append([]cfRule{}, s.rules...)})
	})

	mux.HandleFunc(prefix+"/rs_entry/rules/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, prefix+"/rs_entry/rules/")
		s.mu.Lock()
		defer s.mu.Unlock()
		switch r.Method {
		case http.MethodPatch:
			var patch cfRule
			json.NewDecoder(r.Body).Decode(&patch)
			for i := range s.rules {
				if s.rules[i].ID == id {
					patch.ID = id
					patch.Ref = s.rules[i].Ref
					s.rules[i] = patch
				}
			}
			writeResult(w, cfRuleset{ID: s.id, Rules: append([]cfRule{}, s.rules...)})
		case http.MethodDelete:
			kept := s.rules[:0]
			for _, ru := range s.rules {
				if ru.ID != id {
					kept = append(kept, ru)
				}
			}
			s.rules = kept
			writeResult(w, cfRuleset{ID: s.id, Rules: append([]cfRule{}, s.rules...)})
		default:
			w.WriteHeader(405)
		}
	})

	return mux
}

func writeResult(w http.ResponseWriter, result any) {
	b, _ := json.Marshal(result)
	env := map[string]any{"success": true, "errors": []any{}, "result": json.RawMessage(b)}
	json.NewEncoder(w).Encode(env)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func writeConnector(t *testing.T) (*Connector, *rulesetState) {
	fake := newRulesetFake()
	srv := httptest.NewServer(withAuth(fake.handler(t)))
	t.Cleanup(srv.Close)
	return New("test-token", srv.URL, srv.Client()), fake
}

func withAuth(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func zoneRef() domain.ResourceRef {
	return domain.ResourceRef{Provider: domain.ProviderCloudflare, Kind: "zone", ExternalID: "z1", Name: "example.com"}
}

func TestWriteConnectorInterface(t *testing.T) {
	cf, _ := writeConnector(t)
	if _, ok := connector.AsWriter(cf); !ok {
		t.Fatal("cloudflare connector must implement WriteConnector")
	}
	if _, ok := any(cf).(connector.Verifier); !ok {
		t.Fatal("cloudflare connector must implement Verifier")
	}
}

func TestApplyCreateVerifyRollback(t *testing.T) {
	cf, fake := writeConnector(t)
	ctx := context.Background()
	a := domain.Action{
		Type: domain.ActWAFRuleCreate, Resource: zoneRef(), IdempotencyKey: "k-create-1",
		Params: map[string]any{
			"expression": `(http.request.uri.path eq "/wp-login.php")`,
			"action":     "managed_challenge", "description": "login protection",
		},
	}

	diff, err := cf.DryRun(ctx, a)
	if err != nil || diff.Before != nil {
		t.Fatalf("dry-run create: %+v err=%v", diff, err)
	}

	res, err := cf.Apply(ctx, a)
	if err != nil || !res.Applied || res.ProviderRef == "" {
		t.Fatalf("apply: %+v err=%v", res, err)
	}
	if len(fake.rules) != 1 {
		t.Fatalf("want 1 rule, got %d", len(fake.rules))
	}
	if err := cf.Verify(ctx, *res); err != nil {
		t.Fatalf("verify after create: %v", err)
	}

	// idempotent re-apply: no duplicate
	res2, err := cf.Apply(ctx, a)
	if err != nil || len(fake.rules) != 1 {
		t.Fatalf("re-apply must be idempotent: rules=%d err=%v", len(fake.rules), err)
	}
	if res2.ProviderRef != res.ProviderRef {
		t.Fatal("idempotent apply must return same rule ID")
	}

	if err := cf.Rollback(ctx, *res); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if len(fake.rules) != 0 {
		t.Fatalf("rollback must delete the rule, %d remain", len(fake.rules))
	}
	if err := cf.Verify(ctx, *res); err == nil {
		t.Fatal("verify must fail after rollback")
	}
}

func TestApplyDisableThenRollbackRestores(t *testing.T) {
	cf, fake := writeConnector(t)
	ctx := context.Background()
	fake.rules = []cfRule{{ID: "rule_99", Ref: "existing", Expression: "true", Action: "block", Enabled: boolp(true)}}

	a := domain.Action{
		Type: domain.ActWAFRuleDisable, Resource: zoneRef(), IdempotencyKey: "k-disable",
		Params: map[string]any{"rule_id": "rule_99"},
	}
	res, err := cf.Apply(ctx, a)
	if err != nil {
		t.Fatalf("apply disable: %v", err)
	}
	if fake.rules[0].Enabled == nil || *fake.rules[0].Enabled {
		t.Fatal("rule must be disabled after apply")
	}
	if err := cf.Rollback(ctx, *res); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if fake.rules[0].Enabled == nil || !*fake.rules[0].Enabled {
		t.Fatal("rollback must re-enable the rule")
	}
	if fake.rules[0].Action != "block" || fake.rules[0].Expression != "true" {
		t.Fatalf("rollback must restore prior fields: %+v", fake.rules[0])
	}
}

func TestApplyRejectsBadAction(t *testing.T) {
	cf, _ := writeConnector(t)
	ctx := context.Background()
	a := domain.Action{
		Type: domain.ActWAFRuleCreate, Resource: zoneRef(), IdempotencyKey: "k",
		Params: map[string]any{"expression": "true", "action": "nuke_from_orbit"},
	}
	if _, err := cf.Apply(ctx, a); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("disallowed rule action must be rejected, got %v", err)
	}
}

func TestPurgeIsIrreversible(t *testing.T) {
	cf, _ := writeConnector(t)
	// purge endpoint fake
	res := connector.ActionResult{Action: domain.Action{Type: domain.ActCachePurge, Resource: zoneRef()}}
	if err := cf.Rollback(context.Background(), res); err == nil {
		t.Fatal("purge (no rollback token) must not be reversible")
	}
	if err := cf.Verify(context.Background(), res); err != nil {
		t.Fatalf("purge verify must be a no-op, got %v", err)
	}
}
