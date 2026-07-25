package cloudflare

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/behramkendra/korugan/internal/connector"
	"github.com/behramkendra/korugan/internal/domain"
)

// fixture server: replays canned Cloudflare responses and asserts auth.
func fixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusForbidden)
			return false
		}
		return true
	}
	mux.HandleFunc("/user/tokens/verify", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		w.Write([]byte(`{"success":true,"errors":[],"result":{"id":"t1","status":"active"}}`))
	})
	mux.HandleFunc("/zones", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		page := r.URL.Query().Get("page")
		if page == "2" {
			w.Write([]byte(`{"success":true,"errors":[],"result":[{"id":"z2","name":"ikinci.example"}],"result_info":{"page":2,"per_page":50,"total_pages":2}}`))
			return
		}
		w.Write([]byte(`{"success":true,"errors":[],"result":[{"id":"z1","name":"example.com"}],"result_info":{"page":1,"per_page":50,"total_pages":2}}`))
	})
	mux.HandleFunc("/zones/z1/dns_records", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		w.Write([]byte(`{"success":true,"errors":[],"result":[
			{"id":"d1","type":"A","name":"example.com","content":"203.0.113.10","proxied":true,"ttl":1},
			{"id":"d2","type":"TXT","name":"_dmarc.example.com","content":"v=DMARC1; p=none","proxied":false,"ttl":300}
		],"result_info":{"page":1,"per_page":100,"total_pages":1}}`))
	})
	mux.HandleFunc("/zones/z1", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		w.Write([]byte(`{"success":true,"errors":[],"result":{"id":"z1","status":"active","paused":false}}`))
	})
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		var req struct {
			Variables map[string]any `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Variables["zoneTag"] != "z1" {
			w.Write([]byte(`{"data":{"viewer":{"zones":[]}}}`))
			return
		}
		w.Write([]byte(`{"data":{"viewer":{"zones":[{"firewallEventsAdaptive":[
			{"action":"block","clientASN":"65001","clientCountryName":"TR","clientIP":"203.0.113.7",
			 "clientRequestHTTPHost":"example.com","clientRequestHTTPMethodName":"POST",
			 "clientRequestPath":"/wp-login.php","datetime":"2026-07-25T03:00:01Z",
			 "rayName":"ray1","ruleId":"4711","source":"firewallcustom","userAgent":"bot/1.0"},
			{"action":"managed_challenge","clientASN":"65002","clientCountryName":"DE","clientIP":"203.0.113.8",
			 "clientRequestHTTPHost":"example.com","clientRequestHTTPMethodName":"GET",
			 "clientRequestPath":"/login","datetime":"2026-07-25T03:00:02Z",
			 "rayName":"ray2","ruleId":"4712","source":"firewallmanaged","userAgent":"Mozilla/5.0"},
			{"action":"block","clientASN":"x","clientCountryName":"US","clientIP":"203.0.113.9",
			 "clientRequestHTTPHost":"example.com","clientRequestHTTPMethodName":"GET",
			 "clientRequestPath":"/api","datetime":"2026-07-25T03:00:03Z",
			 "rayName":"ray3","ruleId":"4711","source":"ratelimit","userAgent":"curl/8"}
		]}]}}}`))
	})
	return httptest.NewServer(mux)
}

func testConnector(t *testing.T) *Connector {
	srv := fixtureServer(t)
	t.Cleanup(srv.Close)
	return New("test-token", srv.URL, srv.Client())
}

func TestValidate(t *testing.T) {
	cf := testConnector(t)
	if err := cf.Validate(context.Background()); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestValidateRejectsBadToken(t *testing.T) {
	srv := fixtureServer(t)
	t.Cleanup(srv.Close)
	cf := New("wrong", srv.URL, srv.Client())
	if err := cf.Validate(context.Background()); err == nil {
		t.Fatal("bad token must fail validation")
	}
}

func TestResourcesPagination(t *testing.T) {
	cf := testConnector(t)
	res, err := cf.Resources(context.Background())
	if err != nil {
		t.Fatalf("resources: %v", err)
	}
	if len(res) != 2 || res[0].ExternalID != "z1" || res[1].ExternalID != "z2" {
		t.Fatalf("pagination broken: %+v", res)
	}
	if res[0].Kind != "zone" || res[0].Provider != domain.ProviderCloudflare {
		t.Fatalf("bad resource ref: %+v", res[0])
	}
}

func TestSnapshot(t *testing.T) {
	cf := testConnector(t)
	snap, err := cf.Snapshot(context.Background(), domain.ResourceRef{
		Provider: domain.ProviderCloudflare, Kind: "zone", ExternalID: "z1", Name: "example.com"})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(snap.DNSRecords) != 2 || snap.DNSRecords[0].Type != "A" {
		t.Fatalf("dns records: %+v", snap.DNSRecords)
	}
	if snap.Settings["zone_status"] != "active" {
		t.Fatalf("settings: %+v", snap.Settings)
	}
}

func TestEventsNormalization(t *testing.T) {
	cf := testConnector(t)
	ref := domain.ResourceRef{Provider: domain.ProviderCloudflare, Kind: "zone", ExternalID: "z1", Name: "example.com"}
	page, err := cf.Events(context.Background(), ref, "", connector.EventFilter{Limit: 500})
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(page.Events) != 3 {
		t.Fatalf("want 3 events, got %d", len(page.Events))
	}
	e0, e1, e2 := page.Events[0], page.Events[1], page.Events[2]

	if e0.Category != domain.CatWAFBlock || e0.Severity != domain.SevMedium {
		t.Fatalf("block mapping wrong: %+v", e0)
	}
	if e1.Category != domain.CatWAFChallenge || e1.Severity != domain.SevLow {
		t.Fatalf("challenge mapping wrong: %+v", e1)
	}
	if e2.Category != domain.CatRatelimitHit {
		t.Fatalf("ratelimit mapping wrong: %+v", e2)
	}
	if e2.Actor.ASN != 0 {
		t.Fatalf("malformed ASN must map to 0, got %d", e2.Actor.ASN)
	}
	if e0.ProviderEventID != "ray1:4711:block" {
		t.Fatalf("provider event id: %s", e0.ProviderEventID)
	}
	for _, e := range page.Events {
		if err := e.Validate(); err != nil {
			t.Fatalf("normalized event invalid: %v", err)
		}
	}
	if !page.Done {
		t.Fatal("short page must report done")
	}
	if page.Next != connector.Cursor("2026-07-25T03:00:03Z") {
		t.Fatalf("cursor must advance to last ts, got %q", page.Next)
	}
	// resumed pull re-uses cursor without error
	if _, err := cf.Events(context.Background(), ref, page.Next, connector.EventFilter{}); err != nil {
		t.Fatalf("resume: %v", err)
	}
}

func TestRegistryConstruction(t *testing.T) {
	if _, err := connector.New(domain.ProviderCloudflare, map[string]string{}); err == nil ||
		!strings.Contains(err.Error(), "api_token") {
		t.Fatalf("missing token must error, got %v", err)
	}
	c, err := connector.New(domain.ProviderCloudflare, map[string]string{"api_token": "x"})
	if err != nil || c == nil {
		t.Fatalf("construction failed: %v", err)
	}
	if got := connector.Providers(); len(got) != 1 || got[0] != domain.ProviderCloudflare {
		t.Fatalf("providers: %v", got)
	}
}
