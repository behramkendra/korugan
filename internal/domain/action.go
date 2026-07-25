package domain

import (
	"fmt"
	"time"
)

// ActionType mirrors the event taxonomy: lowercase, dot-namespaced.
type ActionType string

const (
	ActWAFRuleCreate        ActionType = "waf.rule.create"
	ActWAFRuleUpdate        ActionType = "waf.rule.update"
	ActWAFRuleDisable       ActionType = "waf.rule.disable"
	ActCacheRuleCreate      ActionType = "cache.rule.create"
	ActCacheRuleUpdate      ActionType = "cache.rule.update"
	ActCacheRuleDisable     ActionType = "cache.rule.disable"
	ActCachePurge           ActionType = "cache.purge"
	ActRatelimitRuleCreate  ActionType = "ratelimit.rule.create"
	ActRatelimitRuleUpdate  ActionType = "ratelimit.rule.update"
	ActRatelimitRuleDisable ActionType = "ratelimit.rule.disable"
	ActDNSRecordUpdate      ActionType = "dns.record.update"
)

// knownActions maps each permitted action type to whether it is
// reversible by construction. cache.purge is the deliberate exception:
// irreversible but low-risk, allowed at L2, never in L3 policies.
var knownActions = map[ActionType]bool{
	ActWAFRuleCreate: true, ActWAFRuleUpdate: true, ActWAFRuleDisable: true,
	ActCacheRuleCreate: true, ActCacheRuleUpdate: true, ActCacheRuleDisable: true,
	ActCachePurge:          false,
	ActRatelimitRuleCreate: true, ActRatelimitRuleUpdate: true, ActRatelimitRuleDisable: true,
	ActDNSRecordUpdate: true,
}

// forbiddenActions are hard-excluded permanently, at every autonomy level.
// They exist so an attempt is a named, testable domain error — not a
// missing-enum accident.
var forbiddenActions = map[string]struct{}{
	"dns.record.delete": {}, "dns.record.create": {},
	"ssl.cert.change": {}, "zone.delete": {}, "account.settings": {},
}

func (t ActionType) Validate() error {
	if _, forbidden := forbiddenActions[string(t)]; forbidden {
		return fmt.Errorf("action type %q is permanently forbidden", t)
	}
	if _, ok := knownActions[t]; !ok {
		return fmt.Errorf("unknown action type %q", t)
	}
	return nil
}

// Reversible reports whether the action type supports rollback by design.
func (t ActionType) Reversible() bool { return knownActions[t] }

type ActionState string

const (
	ActionPending    ActionState = "pending"
	ActionApproved   ActionState = "approved"
	ActionApplied    ActionState = "applied"
	ActionVerified   ActionState = "verified"
	ActionRolledBack ActionState = "rolled_back"
	ActionRejected   ActionState = "rejected"
	ActionFailed     ActionState = "failed"
)

// Action is one executable, auditable change derived from a Recommendation.
type Action struct {
	ID               string         `json:"id"`
	Type             ActionType     `json:"type"`
	Resource         ResourceRef    `json:"resource"`
	Params           map[string]any `json:"params"`
	State            ActionState    `json:"state"`
	RecommendationID string         `json:"recommendation_id"`
	ApprovedBy       string         `json:"approved_by,omitempty"`
	IdempotencyKey   string         `json:"idempotency_key"`
	AutonomyLevel    Level          `json:"autonomy_level"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

func (a Action) Validate() error {
	if err := a.Type.Validate(); err != nil {
		return err
	}
	if err := a.Resource.Validate(); err != nil {
		return err
	}
	if a.IdempotencyKey == "" {
		return fmt.Errorf("idempotency key is required")
	}
	if a.State == ActionApproved && a.ApprovedBy == "" && a.AutonomyLevel < L3 {
		return fmt.Errorf("approved action below L3 must record an approver")
	}
	return nil
}
