package domain

import (
	"fmt"
	"time"
)

// FindingState is the lifecycle of a detected issue.
type FindingState string

const (
	FindingOpen     FindingState = "open"
	FindingAcked    FindingState = "acked"
	FindingResolved FindingState = "resolved"
	FindingExpired  FindingState = "expired"
)

// Finding is a detected issue: rule-based or AI-produced. Evidence links
// back to the exact events it derives from — uncited findings are invalid.
type Finding struct {
	ID        string       `json:"id"`
	Resource  ResourceRef  `json:"resource"`
	Kind      string       `json:"kind"` // e.g. "cert_expiry", "error_rate_spike", "ai.waf_pattern"
	Severity  Severity     `json:"severity"`
	State     FindingState `json:"state"`
	Title     string       `json:"title"`
	Detail    string       `json:"detail"`
	Evidence  []string     `json:"evidence"` // event IDs
	Source    string       `json:"source"`   // "rule" | "ai"
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

func (f Finding) Validate() error {
	if err := f.Resource.Validate(); err != nil {
		return err
	}
	if f.Kind == "" || f.Title == "" {
		return fmt.Errorf("finding kind and title are required")
	}
	if !f.Severity.Known() {
		return fmt.Errorf("unknown severity %q", f.Severity)
	}
	if f.Source != "rule" && f.Source != "ai" {
		return fmt.Errorf("finding source must be rule or ai, got %q", f.Source)
	}
	if f.Source == "ai" && len(f.Evidence) == 0 {
		return fmt.Errorf("ai findings must cite evidence event IDs")
	}
	return nil
}

// Recommendation is a provider-ready change proposal attached to a finding.
// It is never self-applying: the action pipeline owns execution.
type Recommendation struct {
	ID         string         `json:"id"`
	FindingID  string         `json:"finding_id"`
	Resource   ResourceRef    `json:"resource"`
	ActionType ActionType     `json:"action_type"`
	Params     map[string]any `json:"params"`
	DiffBefore any            `json:"diff_before"`
	DiffAfter  any            `json:"diff_after"`
	Rationale  string         `json:"rationale"`
	Rollback   string         `json:"rollback_plan"`
	Confidence float64        `json:"confidence"` // 0..1
	CreatedAt  time.Time      `json:"created_at"`
}

func (r Recommendation) Validate() error {
	if r.FindingID == "" {
		return fmt.Errorf("recommendation must reference a finding")
	}
	if err := r.Resource.Validate(); err != nil {
		return err
	}
	if err := r.ActionType.Validate(); err != nil {
		return err
	}
	if r.Rationale == "" || r.Rollback == "" {
		return fmt.Errorf("rationale and rollback plan are required")
	}
	if r.Confidence < 0 || r.Confidence > 1 {
		return fmt.Errorf("confidence out of range: %v", r.Confidence)
	}
	return nil
}
