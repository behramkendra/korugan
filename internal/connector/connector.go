// Package connector defines the port every provider adapter implements.
// Everything above this interface is provider-agnostic; if a feature
// can't be expressed here, it belongs in a provider extension, not core.
package connector

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/behramkendra/korugan/internal/domain"
)

// ErrNotImplemented marks capabilities a connector hasn't shipped yet.
// Callers treat it as capability degradation, never as failure.
var ErrNotImplemented = errors.New("connector: not implemented")

type ProviderInfo struct {
	Provider domain.Provider `json:"provider"`
	Label    string          `json:"label"`
	DocsURL  string          `json:"docs_url"`
	// AuthSpec names the credential fields the onboarding UI must collect.
	AuthSpec []AuthField `json:"auth_spec"`
}

type AuthField struct {
	Name   string `json:"name"`   // e.g. "api_token"
	Label  string `json:"label"`  // human label
	Secret bool   `json:"secret"` // masked in UI, sealed at rest
}

// Snapshot is normalized point-in-time configuration state.
type Snapshot struct {
	Resource   domain.ResourceRef `json:"resource"`
	TakenAt    time.Time          `json:"taken_at"`
	DNSRecords []DNSRecord        `json:"dns_records,omitempty"`
	Settings   map[string]any     `json:"settings,omitempty"`
}

type DNSRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
	TTL     int    `json:"ttl"`
}

// Cursor is an opaque provider-side resume position.
type Cursor string

type EventFilter struct {
	Since time.Time
	Until time.Time
	Limit int
}

// EventPage is one pull of normalized events. Done=false means call again
// with Next to continue draining; the poller persists Next between runs.
type EventPage struct {
	Events []domain.Event
	Next   Cursor
	Done   bool
}

type Connector interface {
	Info() ProviderInfo
	Capabilities(ctx context.Context) ([]domain.Capability, error)
	Validate(ctx context.Context) error
	Resources(ctx context.Context) ([]domain.ResourceRef, error)
	Snapshot(ctx context.Context, res domain.ResourceRef) (*Snapshot, error)
	Events(ctx context.Context, res domain.ResourceRef, cur Cursor, f EventFilter) (EventPage, error)
}

// --- registry ---

var (
	mu      sync.RWMutex
	factory = map[domain.Provider]func(creds map[string]string) (Connector, error){}
)

// Register wires a provider factory; called from adapter init().
func Register(p domain.Provider, fn func(creds map[string]string) (Connector, error)) {
	mu.Lock()
	defer mu.Unlock()
	factory[p] = fn
}

// New constructs a connector for provider p with sealed-then-unsealed creds.
func New(p domain.Provider, creds map[string]string) (Connector, error) {
	mu.RLock()
	fn, ok := factory[p]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("connector: unknown provider %q", p)
	}
	return fn(creds)
}

// Providers lists registered providers, sorted, for the API/UI.
func Providers() []domain.Provider {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]domain.Provider, 0, len(factory))
	for p := range factory {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
