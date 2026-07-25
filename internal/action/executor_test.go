package action

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/behramkendra/korugan/internal/connector"
	"github.com/behramkendra/korugan/internal/domain"
)

// fakeWriter is a WriteConnector + Verifier recording the calls the
// executor makes, with injectable failures per stage.
type fakeWriter struct {
	dryErr     error
	applyErr   error
	verifyErr  error
	rbErr      error
	applied    bool
	rolledBack bool
	verifyRan  bool
}

func (f *fakeWriter) Info() connector.ProviderInfo {
	return connector.ProviderInfo{Provider: domain.ProviderCloudflare}
}
func (f *fakeWriter) Capabilities(context.Context) ([]domain.Capability, error) { return nil, nil }
func (f *fakeWriter) Validate(context.Context) error                            { return nil }
func (f *fakeWriter) Resources(context.Context) ([]domain.ResourceRef, error)   { return nil, nil }
func (f *fakeWriter) Snapshot(context.Context, domain.ResourceRef) (*connector.Snapshot, error) {
	return nil, nil
}
func (f *fakeWriter) Events(context.Context, domain.ResourceRef, connector.Cursor, connector.EventFilter) (connector.EventPage, error) {
	return connector.EventPage{Done: true}, nil
}
func (f *fakeWriter) DryRun(context.Context, domain.Action) (*connector.Diff, error) {
	return &connector.Diff{Human: "diff"}, f.dryErr
}
func (f *fakeWriter) Apply(_ context.Context, a domain.Action) (*connector.ActionResult, error) {
	if f.applyErr != nil {
		return nil, f.applyErr
	}
	f.applied = true
	return &connector.ActionResult{Action: a, Applied: true, ProviderRef: "rule_1", RollbackToken: []byte(`{"op":"delete_rule"}`)}, nil
}
func (f *fakeWriter) Rollback(context.Context, connector.ActionResult) error {
	f.rolledBack = true
	return f.rbErr
}
func (f *fakeWriter) Verify(context.Context, connector.ActionResult) error {
	f.verifyRan = true
	return f.verifyErr
}

// The executor's DB writes (setState/audit) are nil-guarded, so these tests
// run without a database and assert on the connector call sequence and the
// returned error. State persistence is covered by store integration tests.

func newTestService(w connector.WriteConnector) *Service {
	return &Service{
		Store:   nil,
		Resolve: func(domain.Provider) (connector.WriteConnector, bool) { return w, w != nil },
		Log:     slog.Default(),
	}
}

func act() domain.Action {
	return domain.Action{
		ID:   "act_1",
		Type: domain.ActWAFRuleCreate,
		Resource: domain.ResourceRef{
			Provider: domain.ProviderCloudflare, Kind: "zone", ExternalID: "z1", Name: "example.com",
		},
		Params:         map[string]any{"expression": "true", "action": "block"},
		IdempotencyKey: "k",
		AutonomyLevel:  domain.L2,
	}
}

func TestExecuteHappyPath(t *testing.T) {
	w := &fakeWriter{}
	s := newTestService(w)
	if err := s.executeCore(context.Background(), act()); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !w.applied || !w.verifyRan || w.rolledBack {
		t.Fatalf("expected apply+verify, no rollback: %+v", w)
	}
}

func TestExecuteRollsBackOnVerifyFailure(t *testing.T) {
	w := &fakeWriter{verifyErr: errors.New("rule missing")}
	s := newTestService(w)
	err := s.executeCore(context.Background(), act())
	if err == nil {
		t.Fatal("verify failure must surface as error")
	}
	if !w.applied || !w.rolledBack {
		t.Fatalf("verify failure must trigger rollback: %+v", w)
	}
}

func TestExecuteApplyFailureNoRollback(t *testing.T) {
	w := &fakeWriter{applyErr: errors.New("cloudflare 500")}
	s := newTestService(w)
	if err := s.executeCore(context.Background(), act()); err == nil {
		t.Fatal("apply failure must error")
	}
	if w.rolledBack {
		t.Fatal("apply failure means nothing to roll back")
	}
}

func TestExecuteNoWritableConnector(t *testing.T) {
	s := newTestService(nil)
	if err := s.executeCore(context.Background(), act()); err == nil {
		t.Fatal("missing writable connector must error")
	}
}

func TestExecuteDryRunFailureStops(t *testing.T) {
	w := &fakeWriter{dryErr: errors.New("bad expression")}
	s := newTestService(w)
	if err := s.executeCore(context.Background(), act()); err == nil {
		t.Fatal("dry-run failure must stop before apply")
	}
	if w.applied {
		t.Fatal("apply must not run after dry-run failure")
	}
}
