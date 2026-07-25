package httpapi

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/behramkendra/korugan/internal/ai"
)

// Router-level tests that need no database: health, auth gate, zero-key chat.
func testServer(token string) *Server {
	return &Server{
		Store:    nil, // endpoints touching the store are not exercised here
		Engine:   ai.NewEngine(nil, nil),
		Log:      slog.Default(),
		APIToken: token,
	}
}

func TestHealthz(t *testing.T) {
	srv := httptest.NewServer(testServer("").Router())
	t.Cleanup(srv.Close)
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz: %v %v", resp, err)
	}
}

func TestAuthGate(t *testing.T) {
	srv := httptest.NewServer(testServer("sekret").Router())
	t.Cleanup(srv.Close)

	resp, _ := http.Post(srv.URL+"/api/v1/chat", "application/json", strings.NewReader(`{"message":"x"}`))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing token must 401, got %d", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/chat", strings.NewReader(`{"message":"x"}`))
	req.Header.Set("Authorization", "Bearer sekret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	// authorized but zero-key → 503, not 401
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("zero-key chat must 503, got %d", resp.StatusCode)
	}
}

func TestChatZeroKeyMessage(t *testing.T) {
	srv := httptest.NewServer(testServer("").Router())
	t.Cleanup(srv.Close)
	resp, _ := http.Post(srv.URL+"/api/v1/chat", "application/json", strings.NewReader(`{"message":"hi"}`))
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503 in zero-key mode, got %d", resp.StatusCode)
	}
}
