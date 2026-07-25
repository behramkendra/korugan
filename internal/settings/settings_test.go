package settings

import (
	"context"
	"strings"
	"testing"

	"github.com/behramkendra/korugan/internal/crypto"
)

// memStore is an in-memory SecretStore for unit tests.
type memStore struct {
	ct     map[string][]byte
	nonce  map[string][]byte
	audits []string
}

func newMemStore() *memStore {
	return &memStore{ct: map[string][]byte{}, nonce: map[string][]byte{}}
}

func (m *memStore) PutSecret(_ context.Context, name string, ct, nonce []byte) error {
	m.ct[name] = ct
	m.nonce[name] = nonce
	return nil
}
func (m *memStore) GetSecret(_ context.Context, name string) ([]byte, []byte, error) {
	return m.ct[name], m.nonce[name], nil
}
func (m *memStore) Audit(_ context.Context, actor, kind, _ string, _ any) error {
	m.audits = append(m.audits, actor+":"+kind)
	return nil
}

func testService(t *testing.T) (*Service, *memStore) {
	t.Helper()
	key, _ := crypto.GenerateMasterKey()
	sealer, err := crypto.NewSealer(key)
	if err != nil {
		t.Fatal(err)
	}
	store := newMemStore()
	return New(store, sealer), store
}

func TestCloudflareTokenSealed(t *testing.T) {
	svc, store := testService(t)
	ctx := context.Background()

	if err := svc.SetCloudflareToken(ctx, "alice", "cf-secret-token-123456"); err != nil {
		t.Fatal(err)
	}
	// ciphertext must not contain the plaintext
	if strings.Contains(string(store.ct[keyCloudflareToken]), "cf-secret-token") {
		t.Fatal("token stored in cleartext")
	}
	got, err := svc.CloudflareToken(ctx)
	if err != nil || got != "cf-secret-token-123456" {
		t.Fatalf("round trip failed: %q %v", got, err)
	}
}

func TestStatusMasksSecrets(t *testing.T) {
	svc, _ := testService(t)
	ctx := context.Background()
	_ = svc.SetCloudflareToken(ctx, "a", "cf-abcdefghijklmnop")
	_ = svc.SetLLM(ctx, "a", LLMConfig{Provider: "openrouter", Model: "x", APIKey: "sk-or-verysecretkey12345"})

	st, err := svc.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Cloudflare.Configured || !st.LLM.Configured {
		t.Fatal("status should report configured")
	}
	if strings.Contains(st.Cloudflare.Hint, "abcdefgh") {
		t.Fatalf("cloudflare hint leaks token: %q", st.Cloudflare.Hint)
	}
	if strings.Contains(st.LLM.KeyHint, "verysecret") {
		t.Fatalf("llm key hint leaks key: %q", st.LLM.KeyHint)
	}
	if st.LLM.Provider != "openrouter" || st.LLM.Model != "x" {
		t.Fatalf("non-secret fields should be visible: %+v", st.LLM)
	}
}

func TestLLMValidation(t *testing.T) {
	svc, _ := testService(t)
	ctx := context.Background()
	if err := svc.SetLLM(ctx, "a", LLMConfig{Provider: "openai", Model: "gpt"}); err == nil {
		t.Fatal("missing api_key must fail for non-ollama")
	}
	if err := svc.SetLLM(ctx, "a", LLMConfig{Provider: "ollama", Model: "llama3"}); err != nil {
		t.Fatalf("ollama without key should be allowed: %v", err)
	}
}

func TestDisabledWithoutSealer(t *testing.T) {
	svc := New(newMemStore(), nil)
	if svc.Enabled() {
		t.Fatal("service without sealer must be disabled")
	}
	if err := svc.SetCloudflareToken(context.Background(), "a", "x"); err == nil {
		t.Fatal("disabled service must refuse writes")
	}
	st, err := svc.Status(context.Background())
	if err != nil || st.SealedStorage {
		t.Fatalf("disabled status: %+v %v", st, err)
	}
}
