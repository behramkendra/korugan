// Package httpapi is Korugan's REST surface. The future web UI is a
// client of these endpoints — no private APIs.
package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/behramkendra/korugan/internal/ai"
	"github.com/behramkendra/korugan/internal/ai/provider"
	"github.com/behramkendra/korugan/internal/store"
)

type Server struct {
	Store  *store.Store
	Engine *ai.Engine
	Log    *slog.Logger
	// APIToken, when set, gates every /api route (Authorization: Bearer).
	// Empty means open — acceptable only for localhost development.
	APIToken string
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ai_enabled": s.Engine.Enabled()})
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(s.auth)
		r.Get("/resources", s.handleResources)
		r.Get("/events", s.handleEvents)
		r.Get("/findings", s.handleFindings)
		r.Post("/chat", s.handleChat)
	})
	return r
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.APIToken == "" {
			next.ServeHTTP(w, r)
			return
		}
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.APIToken)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleResources(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Store.ListResources(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"resources": rows})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.EventFilter{
		ResourceID: q.Get("resource_id"),
		Category:   q.Get("category"),
	}
	if v := q.Get("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.Since = t
		}
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Limit = n
		}
	}
	evs, err := s.Store.ListEvents(r.Context(), f)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": evs})
}

func (s *Server) handleFindings(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Store.ListFindings(r.Context(), r.URL.Query().Get("state"), 100)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"findings": rows})
}

type chatRequest struct {
	Message    string `json:"message"`
	ResourceID string `json:"resource_id,omitempty"`
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if !s.Engine.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "no LLM key configured — Korugan is running in zero-key mode",
		})
		return
	}
	var req chatRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil || strings.TrimSpace(req.Message) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "message is required"})
		return
	}

	// Grounding: recent events window (bounded), serialized compactly.
	evs, err := s.Store.ListEvents(r.Context(), store.EventFilter{
		ResourceID: req.ResourceID,
		Since:      time.Now().UTC().Add(-24 * time.Hour),
		Limit:      100,
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	evJSON, _ := json.Marshal(evs)
	findings, err := s.Store.ListFindings(r.Context(), "open", 25)
	if err != nil {
		s.fail(w, err)
		return
	}
	fJSON, _ := json.Marshal(findings)

	answer, err := s.Engine.Chat(r.Context(), ai.TaskChat, req.Message, []ai.Grounding{
		{Label: "normalized events, last 24h (newest first, max 100)", JSON: string(evJSON)},
		{Label: "open findings", JSON: string(fJSON)},
	})
	if err != nil {
		if err == provider.ErrNoKey {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "no LLM key configured"})
			return
		}
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"answer": answer, "grounded_events": len(evs)})
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	s.Log.Error("api error", "err", err)
	writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal error"})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
