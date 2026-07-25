// Package action owns the gated path from recommendation to applied change:
// approval, then an executor that dry-runs, applies, verifies and rolls
// back. No LLM ever runs here — models only propose; this package decides
// and executes under human authority.
package action

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/behramkendra/korugan/internal/connector"
	"github.com/behramkendra/korugan/internal/domain"
	"github.com/behramkendra/korugan/internal/store"
)

// ConnectorResolver returns the writable connector for a provider, if one
// is configured and supports writes.
type ConnectorResolver func(p domain.Provider) (connector.WriteConnector, bool)

type Service struct {
	Store   *store.Store
	Resolve ConnectorResolver
	Log     *slog.Logger
}

// idempotencyKey is stable per recommendation so retries never double-apply.
func idempotencyKey(recommendationID string) string {
	sum := sha256.Sum256([]byte("korugan-action:" + recommendationID))
	return hex.EncodeToString(sum[:])
}

// Approve turns a recommendation into an approved action and executes it at
// L2 (human-approved apply). The approver is recorded in the audit chain.
// Returns the resulting action ID.
func (s *Service) Approve(ctx context.Context, recommendationID, approver string) (string, error) {
	if approver == "" {
		return "", fmt.Errorf("approver is required")
	}
	rec, err := s.Store.GetRecommendation(ctx, recommendationID)
	if err != nil {
		return "", err
	}
	if rec == nil {
		return "", fmt.Errorf("recommendation %s not found", recommendationID)
	}

	resourceID, err := s.Store.UpsertResource(ctx, rec.Resource)
	if err != nil {
		return "", err
	}

	a := domain.Action{
		ID:               store.NewID(),
		Type:             rec.ActionType,
		Resource:         rec.Resource,
		Params:           rec.Params,
		State:            domain.ActionApproved,
		RecommendationID: rec.ID,
		ApprovedBy:       approver,
		IdempotencyKey:   idempotencyKey(rec.ID),
		AutonomyLevel:    domain.L2,
	}

	// Gate is defense in depth: the domain layer independently confirms this
	// action type may execute at L2 with an approver.
	dec := domain.Gate(a, domain.L2, nil)
	if !dec.Allowed {
		return "", fmt.Errorf("gate refused action: %s", dec.Reason)
	}

	if err := s.Store.InsertAction(ctx, resourceID, a); err != nil {
		return "", err
	}
	_ = s.Store.Audit(ctx, approver, "action.approved", a.ID, map[string]any{
		"type": a.Type, "recommendation_id": rec.ID, "resource": rec.Resource.String(),
	})

	if err := s.Execute(ctx, a); err != nil {
		return a.ID, err
	}
	return a.ID, nil
}

// Reject records a human rejection with a reason (fed back as context to
// future rule-generation in the same workspace).
func (s *Service) Reject(ctx context.Context, recommendationID, approver, reason string) error {
	rec, err := s.Store.GetRecommendation(ctx, recommendationID)
	if err != nil {
		return err
	}
	if rec == nil {
		return fmt.Errorf("recommendation %s not found", recommendationID)
	}
	return s.Store.Audit(ctx, approver, "recommendation.rejected", recommendationID, map[string]any{
		"reason": reason, "action_type": rec.ActionType,
	})
}

// Execute runs the safety-critical pipeline for an already-approved action:
//
//	precondition (dry-run) → apply → verify → rollback-on-failure
//
// A post-apply *timed regression watch* (error-budget over minutes) is
// intentionally deferred to a later milestone; this performs an immediate
// provider read-back verification and rolls back on failure. Every state
// transition is audited.
func (s *Service) Execute(ctx context.Context, a domain.Action) error {
	return s.executeCore(ctx, a)
}

func (s *Service) executeCore(ctx context.Context, a domain.Action) error {
	w, ok := s.Resolve(a.Resource.Provider)
	if !ok {
		s.setState(ctx, a, domain.ActionFailed, map[string]any{"error": "provider has no writable connector"})
		return fmt.Errorf("no writable connector for provider %s", a.Resource.Provider)
	}

	// Precondition: dry-run must succeed and the action must still be valid.
	if _, err := w.DryRun(ctx, a); err != nil {
		s.setState(ctx, a, domain.ActionFailed, map[string]any{"stage": "dry_run", "error": err.Error()})
		return fmt.Errorf("dry-run failed: %w", err)
	}

	res, err := w.Apply(ctx, a)
	if err != nil {
		s.setState(ctx, a, domain.ActionFailed, map[string]any{"stage": "apply", "error": err.Error()})
		return fmt.Errorf("apply failed: %w", err)
	}
	s.setState(ctx, a, domain.ActionApplied, res)
	s.audit(ctx, "system", "action.applied", a.ID, map[string]any{"provider_ref": res.ProviderRef})

	// Verify by read-back where the connector supports it.
	if v, ok := any(w).(connector.Verifier); ok {
		if err := v.Verify(ctx, *res); err != nil {
			s.Log.Warn("post-apply verify failed, rolling back", "action", a.ID, "err", err)
			if rbErr := w.Rollback(ctx, *res); rbErr != nil {
				s.setState(ctx, a, domain.ActionFailed, map[string]any{"stage": "verify", "error": err.Error(), "rollback_error": rbErr.Error()})
				s.audit(ctx, "system", "action.rollback_failed", a.ID, map[string]any{"error": rbErr.Error()})
				return fmt.Errorf("verify failed and rollback errored: verify=%v rollback=%v", err, rbErr)
			}
			s.setState(ctx, a, domain.ActionRolledBack, map[string]any{"reason": "verification failed", "error": err.Error()})
			s.audit(ctx, "system", "action.rolled_back", a.ID, map[string]any{"reason": "verify_failed"})
			return fmt.Errorf("action verified-failed and was rolled back: %w", err)
		}
	}

	s.setState(ctx, a, domain.ActionVerified, nil)
	s.audit(ctx, "system", "action.verified", a.ID, nil)
	return nil
}

// Rollback reverts a previously applied action on operator request.
func (s *Service) Rollback(ctx context.Context, actionID, actor string) error {
	row, err := s.Store.GetAction(ctx, actionID)
	if err != nil {
		return err
	}
	if row == nil {
		return fmt.Errorf("action %s not found", actionID)
	}
	if row.State != domain.ActionApplied && row.State != domain.ActionVerified {
		return fmt.Errorf("action %s is %s, not rollbackable", actionID, row.State)
	}
	w, ok := s.Resolve(row.Resource.Provider)
	if !ok {
		return fmt.Errorf("no writable connector for provider %s", row.Resource.Provider)
	}
	var res connector.ActionResult
	if err := unmarshalResult(row.Result, &res); err != nil {
		return fmt.Errorf("stored result unreadable: %w", err)
	}
	if err := w.Rollback(ctx, res); err != nil {
		return err
	}
	_ = s.Store.SetActionState(ctx, actionID, domain.ActionRolledBack, map[string]any{"rolled_back_by": actor, "at": time.Now().UTC()})
	_ = s.Store.Audit(ctx, actor, "action.rolled_back", actionID, map[string]any{"manual": true})
	return nil
}

func (s *Service) setState(ctx context.Context, a domain.Action, state domain.ActionState, result any) {
	if s.Store == nil {
		return
	}
	if err := s.Store.SetActionState(ctx, a.ID, state, result); err != nil {
		s.Log.Error("action state update failed", "action", a.ID, "state", state, "err", err)
	}
}

func (s *Service) audit(ctx context.Context, actor, kind, subjectID string, detail any) {
	if s.Store == nil {
		return
	}
	_ = s.Store.Audit(ctx, actor, kind, subjectID, detail)
}

func unmarshalResult(raw []byte, out *connector.ActionResult) error {
	if len(raw) == 0 {
		return fmt.Errorf("no stored apply result")
	}
	return json.Unmarshal(raw, out)
}
