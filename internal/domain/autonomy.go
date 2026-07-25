package domain

import "fmt"

// Level is the autonomy level of a (resource, action-type) pair — never
// global. The AI engine is level-agnostic (it only proposes); levels gate
// what the action pipeline will do.
type Level int

const (
	L0 Level = iota // observe: findings only
	L1              // recommend: proposals with diff + rollback plan
	L2              // approved apply: human approval per action
	L3              // autonomous: policy-scoped, reversible-only
)

func (l Level) String() string {
	switch l {
	case L0:
		return "L0"
	case L1:
		return "L1"
	case L2:
		return "L2"
	case L3:
		return "L3"
	}
	return fmt.Sprintf("L?(%d)", int(l))
}

func ParseLevel(s string) (Level, error) {
	switch s {
	case "L0", "0":
		return L0, nil
	case "L1", "1":
		return L1, nil
	case "L2", "2":
		return L2, nil
	case "L3", "3":
		return L3, nil
	}
	return L0, fmt.Errorf("invalid autonomy level %q", s)
}

// Policy scopes L3 autonomy. Absent a matching policy, L3 behaves as L2.
type Policy struct {
	ID            string       `json:"id"`
	Resource      ResourceRef  `json:"resource"`
	AllowedTypes  []ActionType `json:"allowed_types"`
	MaxPerHour    int          `json:"max_per_hour"`
	QuietHoursUTC [2]int       `json:"quiet_hours_utc"` // [start,end), 0-23; equal = disabled
	Enabled       bool         `json:"enabled"`
}

func (p Policy) Validate() error {
	if err := p.Resource.Validate(); err != nil {
		return err
	}
	if len(p.AllowedTypes) == 0 {
		return fmt.Errorf("policy must allow at least one action type")
	}
	for _, t := range p.AllowedTypes {
		if err := t.Validate(); err != nil {
			return err
		}
		if !t.Reversible() {
			return fmt.Errorf("irreversible action type %q cannot enter a policy", t)
		}
	}
	if p.MaxPerHour < 1 {
		return fmt.Errorf("policy rate limit must be >= 1/hour")
	}
	return nil
}

// Decision is the action pipeline's gate verdict.
type Decision struct {
	Allowed       bool
	NeedsApproval bool
	Reason        string
}

// Gate decides what the pipeline may do with an action at the given level.
// Rules, in order: forbidden/unknown types never pass; L0 blocks all
// actions; L1 blocks execution (recommend-only); L2 requires approval;
// L3 executes without per-action approval only inside an enabled,
// matching, reversible policy — otherwise it degrades to L2 behavior.
func Gate(a Action, lvl Level, pol *Policy) Decision {
	if err := a.Type.Validate(); err != nil {
		return Decision{Allowed: false, Reason: err.Error()}
	}
	switch lvl {
	case L0:
		return Decision{Allowed: false, Reason: "autonomy L0: observe only"}
	case L1:
		return Decision{Allowed: false, Reason: "autonomy L1: recommendations only"}
	case L2:
		return Decision{Allowed: true, NeedsApproval: true, Reason: "L2 requires human approval"}
	case L3:
		if !a.Type.Reversible() {
			return Decision{Allowed: true, NeedsApproval: true, Reason: "irreversible action: approval required even at L3"}
		}
		if pol == nil || !pol.Enabled {
			return Decision{Allowed: true, NeedsApproval: true, Reason: "no enabled policy: L3 degrades to L2"}
		}
		for _, t := range pol.AllowedTypes {
			if t == a.Type {
				return Decision{Allowed: true, NeedsApproval: false, Reason: "within policy " + pol.ID}
			}
		}
		return Decision{Allowed: true, NeedsApproval: true, Reason: "action type outside policy allowlist"}
	}
	return Decision{Allowed: false, Reason: "unknown autonomy level"}
}
