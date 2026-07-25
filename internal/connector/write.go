package connector

import (
	"context"

	"github.com/behramkendra/korugan/internal/domain"
)

// Diff is a connector's prediction of an action's effect, computed by
// DryRun. Before is nil for pure creations.
type Diff struct {
	Before any    `json:"before"`
	After  any    `json:"after"`
	Human  string `json:"human"` // one-line reviewer summary
}

// ActionResult is the outcome of Apply. RollbackToken carries whatever the
// connector needs to reverse the change later (prior ruleset JSON, deleted
// rule ID, previous DNS content) — opaque to the rest of the system.
type ActionResult struct {
	Action        domain.Action `json:"action"`
	Applied       bool          `json:"applied"`
	ProviderRef   string        `json:"provider_ref"`   // e.g. created rule ID
	RollbackToken []byte        `json:"rollback_token"` // connector-defined
	Detail        string        `json:"detail"`
}

// WriteConnector is implemented only by connectors that can change provider
// state. The action pipeline type-asserts for it; a read-only connector
// simply never receives write actions (capability degradation, not error).
type WriteConnector interface {
	Connector

	// DryRun predicts effect without applying. Connectors lacking native
	// dry-run compute the diff locally from current state.
	DryRun(ctx context.Context, a domain.Action) (*Diff, error)

	// Apply executes the action. MUST be idempotent under retry via
	// a.IdempotencyKey. Returns rollback material when the action is
	// reversible.
	Apply(ctx context.Context, a domain.Action) (*ActionResult, error)

	// Rollback reverts a previously applied action using its result.
	Rollback(ctx context.Context, prev ActionResult) error
}

// AsWriter returns the connector as a WriteConnector if it supports writes.
func AsWriter(c Connector) (WriteConnector, bool) {
	w, ok := c.(WriteConnector)
	return w, ok
}

// Verifier is optionally implemented by connectors that can read back
// provider state to confirm an apply. The executor uses it for post-apply
// verification; connectors without it skip that check.
type Verifier interface {
	Verify(ctx context.Context, res ActionResult) error
}
