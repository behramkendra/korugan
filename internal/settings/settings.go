// Package settings stores connector and LLM credentials sealed at rest,
// so they can be configured at runtime instead of only through environment
// variables. Secrets are AES-256-GCM sealed by internal/crypto before they
// reach the store; reads return masked hints, never the plaintext.
package settings

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/behramkendra/korugan/internal/crypto"
)

// SecretStore is the subset of the store this package needs. Keeping it an
// interface lets the service be unit-tested without a database.
type SecretStore interface {
	PutSecret(ctx context.Context, name string, ciphertext, nonce []byte) error
	GetSecret(ctx context.Context, name string) (ciphertext, nonce []byte, err error)
	Audit(ctx context.Context, actor, kind, subjectID string, detail any) error
}

const (
	keyCloudflareToken = "cloudflare.api_token"
	keyLLMConfig       = "llm.config"
)

// LLMConfig is the stored (sealed) LLM configuration.
type LLMConfig struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key"`
}

type Service struct {
	store  SecretStore
	sealer *crypto.Sealer
}

// New returns a service, or nil-capable behavior when sealer is nil
// (no master key configured). Callers check Enabled().
func New(store SecretStore, sealer *crypto.Sealer) *Service {
	return &Service{store: store, sealer: sealer}
}

// Enabled reports whether sealed settings are available (master key set).
func (s *Service) Enabled() bool { return s != nil && s.sealer != nil }

func (s *Service) put(ctx context.Context, name string, plaintext []byte) error {
	ct, nonce, err := s.sealer.Seal(plaintext)
	if err != nil {
		return err
	}
	return s.store.PutSecret(ctx, name, ct, nonce)
}

func (s *Service) get(ctx context.Context, name string) ([]byte, error) {
	ct, nonce, err := s.store.GetSecret(ctx, name)
	if err != nil {
		return nil, err
	}
	if ct == nil {
		return nil, nil // not configured
	}
	return s.sealer.Open(ct, nonce)
}

// SetCloudflareToken seals and stores a Cloudflare API token.
func (s *Service) SetCloudflareToken(ctx context.Context, actor, token string) error {
	if !s.Enabled() {
		return fmt.Errorf("settings disabled: set KORUGAN_MASTER_KEY to store sealed credentials")
	}
	if token == "" {
		return fmt.Errorf("token is required")
	}
	if err := s.put(ctx, keyCloudflareToken, []byte(token)); err != nil {
		return err
	}
	_ = s.store.Audit(ctx, actor, "settings.cloudflare_token_set", keyCloudflareToken, nil)
	return nil
}

// CloudflareToken returns the stored token, or "" if unset. Server-side only.
func (s *Service) CloudflareToken(ctx context.Context) (string, error) {
	if !s.Enabled() {
		return "", nil
	}
	b, err := s.get(ctx, keyCloudflareToken)
	return string(b), err
}

// SetLLM seals and stores LLM configuration.
func (s *Service) SetLLM(ctx context.Context, actor string, cfg LLMConfig) error {
	if !s.Enabled() {
		return fmt.Errorf("settings disabled: set KORUGAN_MASTER_KEY to store sealed credentials")
	}
	if cfg.Provider == "" || cfg.Model == "" {
		return fmt.Errorf("provider and model are required")
	}
	if cfg.APIKey == "" && cfg.Provider != "ollama" {
		return fmt.Errorf("api_key is required for provider %q", cfg.Provider)
	}
	blob, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := s.put(ctx, keyLLMConfig, blob); err != nil {
		return err
	}
	_ = s.store.Audit(ctx, actor, "settings.llm_set", keyLLMConfig,
		map[string]any{"provider": cfg.Provider, "model": cfg.Model})
	return nil
}

// LLM returns the stored config, or nil if unset. Server-side only.
func (s *Service) LLM(ctx context.Context) (*LLMConfig, error) {
	if !s.Enabled() {
		return nil, nil
	}
	b, err := s.get(ctx, keyLLMConfig)
	if err != nil || b == nil {
		return nil, err
	}
	var cfg LLMConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Status is the masked, safe-to-serve view of configured credentials.
type Status struct {
	SealedStorage bool          `json:"sealed_storage"`
	Cloudflare    CredStatus    `json:"cloudflare"`
	LLM           LLMCredStatus `json:"llm"`
}

type CredStatus struct {
	Configured bool   `json:"configured"`
	Hint       string `json:"hint,omitempty"`
}

type LLMCredStatus struct {
	Configured bool   `json:"configured"`
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model,omitempty"`
	KeyHint    string `json:"key_hint,omitempty"`
}

// Status returns masked configuration state for the UI. Never returns
// plaintext secrets, only presence and masked hints.
func (s *Service) Status(ctx context.Context) (Status, error) {
	st := Status{SealedStorage: s.Enabled()}
	if !s.Enabled() {
		return st, nil
	}
	if tok, err := s.CloudflareToken(ctx); err != nil {
		return st, err
	} else if tok != "" {
		st.Cloudflare = CredStatus{Configured: true, Hint: crypto.Mask(tok)}
	}
	if cfg, err := s.LLM(ctx); err != nil {
		return st, err
	} else if cfg != nil {
		st.LLM = LLMCredStatus{
			Configured: true, Provider: cfg.Provider, Model: cfg.Model,
			KeyHint: crypto.Mask(cfg.APIKey),
		}
	}
	return st, nil
}
