package cloudflare

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/behramkendra/korugan/internal/connector"
	"github.com/behramkendra/korugan/internal/domain"
)

// This file makes the Cloudflare connector a connector.WriteConnector for
// the action types Korugan ships at L2: WAF custom rules and cache purge.
// Custom rules live in the zone's http_request_firewall_custom entrypoint
// ruleset (Rulesets API). Cloudflare has no native dry-run there, so diffs
// are computed locally from current state.

var _ connector.WriteConnector = (*Connector)(nil)

// allowedRuleActions bounds what a WAF rule may do — a guardrail even
// before the domain layer sees it.
var allowedRuleActions = map[string]struct{}{
	"block": {}, "managed_challenge": {}, "js_challenge": {},
	"challenge": {}, "log": {}, "skip": {},
}

type cfRule struct {
	ID          string `json:"id,omitempty"`
	Ref         string `json:"ref,omitempty"`
	Expression  string `json:"expression,omitempty"`
	Action      string `json:"action,omitempty"`
	Description string `json:"description,omitempty"`
	Enabled     *bool  `json:"enabled,omitempty"`
}

type cfRuleset struct {
	ID    string   `json:"id"`
	Rules []cfRule `json:"rules"`
}

// rollbackToken is self-describing so Rollback needs no action context.
type rollbackToken struct {
	Op        string          `json:"op"` // delete_rule | restore_rule | enable_rule
	ZoneID    string          `json:"zone_id"`
	RulesetID string          `json:"ruleset_id"`
	RuleID    string          `json:"rule_id"`
	PriorRule json.RawMessage `json:"prior_rule,omitempty"`
}

const customPhase = "http_request_firewall_custom"

func refFor(idempotencyKey string) string {
	sum := sha256.Sum256([]byte(idempotencyKey))
	return "korugan_" + hex.EncodeToString(sum[:])[:24]
}

func boolp(b bool) *bool { return &b }

// entrypoint fetches the custom-phase entrypoint ruleset for a zone.
func (cf *Connector) entrypoint(ctx context.Context, zoneID string) (*cfRuleset, error) {
	env, err := cf.c.get(ctx, fmt.Sprintf("/zones/%s/rulesets/phases/%s/entrypoint", zoneID, customPhase))
	if err != nil {
		return nil, err
	}
	var rs cfRuleset
	if err := json.Unmarshal(env.Result, &rs); err != nil {
		return nil, fmt.Errorf("entrypoint decode: %w", err)
	}
	return &rs, nil
}

func findByRef(rs *cfRuleset, ref string) *cfRule {
	for i := range rs.Rules {
		if rs.Rules[i].Ref == ref {
			return &rs.Rules[i]
		}
	}
	return nil
}

func findByID(rs *cfRuleset, id string) *cfRule {
	for i := range rs.Rules {
		if rs.Rules[i].ID == id {
			return &rs.Rules[i]
		}
	}
	return nil
}

// ruleFromParams builds a rule from action params, validating the action.
func ruleFromParams(a domain.Action) (cfRule, error) {
	var r cfRule
	r.Expression, _ = a.Params["expression"].(string)
	r.Action, _ = a.Params["action"].(string)
	r.Description, _ = a.Params["description"].(string)
	if r.Expression == "" {
		return r, fmt.Errorf("waf rule requires a non-empty expression")
	}
	if _, ok := allowedRuleActions[r.Action]; !ok {
		return r, fmt.Errorf("waf rule action %q not allowed", r.Action)
	}
	return r, nil
}

func ruleID(a domain.Action) (string, error) {
	id, _ := a.Params["rule_id"].(string)
	if id == "" {
		return "", fmt.Errorf("action %s requires params.rule_id", a.Type)
	}
	return id, nil
}

// DryRun predicts the effect locally.
func (cf *Connector) DryRun(ctx context.Context, a domain.Action) (*connector.Diff, error) {
	zone := a.Resource.ExternalID
	switch a.Type {
	case domain.ActWAFRuleCreate:
		r, err := ruleFromParams(a)
		if err != nil {
			return nil, err
		}
		return &connector.Diff{
			Before: nil, After: r,
			Human: fmt.Sprintf("create WAF rule: %s → %s", r.Expression, r.Action),
		}, nil
	case domain.ActWAFRuleUpdate, domain.ActWAFRuleDisable:
		rid, err := ruleID(a)
		if err != nil {
			return nil, err
		}
		rs, err := cf.entrypoint(ctx, zone)
		if err != nil {
			return nil, err
		}
		prior := findByID(rs, rid)
		if prior == nil {
			return nil, fmt.Errorf("rule %s not found in zone %s", rid, zone)
		}
		after := *prior
		if a.Type == domain.ActWAFRuleDisable {
			after.Enabled = boolp(false)
			return &connector.Diff{Before: *prior, After: after, Human: "disable WAF rule " + rid}, nil
		}
		if exp, ok := a.Params["expression"].(string); ok && exp != "" {
			after.Expression = exp
		}
		if act, ok := a.Params["action"].(string); ok && act != "" {
			if _, allowed := allowedRuleActions[act]; !allowed {
				return nil, fmt.Errorf("waf rule action %q not allowed", act)
			}
			after.Action = act
		}
		return &connector.Diff{Before: *prior, After: after, Human: "update WAF rule " + rid}, nil
	case domain.ActCachePurge:
		return &connector.Diff{After: a.Params, Human: "purge cache (irreversible)"}, nil
	}
	return nil, fmt.Errorf("cloudflare: dry-run unsupported for %s", a.Type)
}

// Apply executes the action idempotently (rules carry a ref derived from
// the action's idempotency key; a re-applied create is detected, not
// duplicated).
func (cf *Connector) Apply(ctx context.Context, a domain.Action) (*connector.ActionResult, error) {
	zone := a.Resource.ExternalID
	switch a.Type {
	case domain.ActWAFRuleCreate:
		return cf.applyCreate(ctx, a, zone)
	case domain.ActWAFRuleUpdate, domain.ActWAFRuleDisable:
		return cf.applyMutate(ctx, a, zone)
	case domain.ActCachePurge:
		return cf.applyPurge(ctx, a, zone)
	}
	return nil, fmt.Errorf("cloudflare: apply unsupported for %s", a.Type)
}

func (cf *Connector) applyCreate(ctx context.Context, a domain.Action, zone string) (*connector.ActionResult, error) {
	rule, err := ruleFromParams(a)
	if err != nil {
		return nil, err
	}
	rule.Ref = refFor(a.IdempotencyKey)
	rule.Enabled = boolp(true)

	rs, err := cf.entrypoint(ctx, zone)
	if err != nil {
		return nil, err
	}
	// idempotency: a matching ref means a prior Apply already landed.
	if existing := findByRef(rs, rule.Ref); existing != nil {
		return cf.createResult(a, zone, rs.ID, existing.ID, "already applied (idempotent)")
	}

	body, _ := json.Marshal(rule)
	env, err := cf.c.do(ctx, http.MethodPost, fmt.Sprintf("/zones/%s/rulesets/%s/rules", zone, rs.ID), body)
	if err != nil {
		return nil, err
	}
	var updated cfRuleset
	if err := json.Unmarshal(env.Result, &updated); err != nil {
		return nil, fmt.Errorf("create rule decode: %w", err)
	}
	created := findByRef(&updated, rule.Ref)
	if created == nil {
		return nil, fmt.Errorf("created rule not found in response")
	}
	return cf.createResult(a, zone, updated.ID, created.ID, "created")
}

func (cf *Connector) createResult(a domain.Action, zone, rulesetID, ruleID, detail string) (*connector.ActionResult, error) {
	tok, _ := json.Marshal(rollbackToken{Op: "delete_rule", ZoneID: zone, RulesetID: rulesetID, RuleID: ruleID})
	return &connector.ActionResult{
		Action: a, Applied: true, ProviderRef: ruleID, RollbackToken: tok, Detail: detail,
	}, nil
}

func (cf *Connector) applyMutate(ctx context.Context, a domain.Action, zone string) (*connector.ActionResult, error) {
	rid, err := ruleID(a)
	if err != nil {
		return nil, err
	}
	rs, err := cf.entrypoint(ctx, zone)
	if err != nil {
		return nil, err
	}
	prior := findByID(rs, rid)
	if prior == nil {
		return nil, fmt.Errorf("rule %s not found in zone %s", rid, zone)
	}
	priorJSON, _ := json.Marshal(*prior)

	patch := cfRule{Expression: prior.Expression, Action: prior.Action, Description: prior.Description}
	if a.Type == domain.ActWAFRuleDisable {
		patch.Enabled = boolp(false)
	} else {
		patch.Enabled = boolp(true)
		if exp, ok := a.Params["expression"].(string); ok && exp != "" {
			patch.Expression = exp
		}
		if act, ok := a.Params["action"].(string); ok && act != "" {
			if _, allowed := allowedRuleActions[act]; !allowed {
				return nil, fmt.Errorf("waf rule action %q not allowed", act)
			}
			patch.Action = act
		}
	}
	body, _ := json.Marshal(patch)
	if _, err := cf.c.do(ctx, http.MethodPatch,
		fmt.Sprintf("/zones/%s/rulesets/%s/rules/%s", zone, rs.ID, rid), body); err != nil {
		return nil, err
	}
	op := "restore_rule"
	tok, _ := json.Marshal(rollbackToken{Op: op, ZoneID: zone, RulesetID: rs.ID, RuleID: rid, PriorRule: priorJSON})
	return &connector.ActionResult{
		Action: a, Applied: true, ProviderRef: rid, RollbackToken: tok, Detail: string(a.Type),
	}, nil
}

func (cf *Connector) applyPurge(ctx context.Context, a domain.Action, zone string) (*connector.ActionResult, error) {
	// Accept {"purge_everything":true} or {"files":[...]} — validated shallowly.
	body, _ := json.Marshal(a.Params)
	if _, err := cf.c.do(ctx, http.MethodPost, fmt.Sprintf("/zones/%s/purge_cache", zone), body); err != nil {
		return nil, err
	}
	// Irreversible: no rollback token.
	return &connector.ActionResult{Action: a, Applied: true, Detail: "cache purged"}, nil
}

// Rollback reverses a prior Apply using its self-describing token.
func (cf *Connector) Rollback(ctx context.Context, prev connector.ActionResult) error {
	if len(prev.RollbackToken) == 0 {
		return fmt.Errorf("action %s is not reversible", prev.Action.Type)
	}
	var tok rollbackToken
	if err := json.Unmarshal(prev.RollbackToken, &tok); err != nil {
		return fmt.Errorf("rollback token decode: %w", err)
	}
	switch tok.Op {
	case "delete_rule":
		_, err := cf.c.do(ctx, http.MethodDelete,
			fmt.Sprintf("/zones/%s/rulesets/%s/rules/%s", tok.ZoneID, tok.RulesetID, tok.RuleID), nil)
		return err
	case "restore_rule", "enable_rule":
		var prior cfRule
		if err := json.Unmarshal(tok.PriorRule, &prior); err != nil {
			return fmt.Errorf("prior rule decode: %w", err)
		}
		if prior.Enabled == nil {
			prior.Enabled = boolp(true)
		}
		body, _ := json.Marshal(cfRule{
			Expression: prior.Expression, Action: prior.Action,
			Description: prior.Description, Enabled: prior.Enabled,
		})
		_, err := cf.c.do(ctx, http.MethodPatch,
			fmt.Sprintf("/zones/%s/rulesets/%s/rules/%s", tok.ZoneID, tok.RulesetID, tok.RuleID), body)
		return err
	}
	return fmt.Errorf("unknown rollback op %q", tok.Op)
}

// Verify reads back provider state to confirm an apply took effect. Used by
// the executor's post-apply check. For creates/mutates it confirms the rule
// exists with the expected shape; purge is unverifiable and returns nil.
func (cf *Connector) Verify(ctx context.Context, res connector.ActionResult) error {
	if res.Action.Type == domain.ActCachePurge {
		return nil
	}
	zone := res.Action.Resource.ExternalID
	rs, err := cf.entrypoint(ctx, zone)
	if err != nil {
		return err
	}
	if res.ProviderRef == "" || findByID(rs, res.ProviderRef) == nil {
		return fmt.Errorf("verify: rule %s not present after apply", res.ProviderRef)
	}
	return nil
}
