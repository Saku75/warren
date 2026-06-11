package dcim

import (
	"encoding/json"
	"slices"

	"github.com/saku75/warren/internal/core/fault"
)

// ComponentKinds supported by the unified component model (design doc
// 0003 §1). Front/rear ports join with cabling in Phase 4.
var ComponentKinds = []string{"interface", "console-port", "power-port", "power-outlet", "module-bay"}

func validComponentKind(k string) bool { return slices.Contains(ComponentKinds, k) }

// ComponentAttrs is the union of kind-specific attributes; Validate
// enforces which fields each kind may use. Stored as JSONB on templates
// and components.
type ComponentAttrs struct {
	// Type is the hardware type, e.g. "10gbase-x-sfpp", "rj-45",
	// "iec-60320-c14" (interface, console-port, power-port, power-outlet).
	Type string `json:"type,omitempty"`
	// MgmtOnly marks out-of-band interfaces (interface).
	MgmtOnly bool `json:"mgmt_only,omitempty"`
	// MaxDrawW / AllocatedDrawW in watts (power-port).
	MaxDrawW       int32 `json:"max_draw_w,omitempty"`
	AllocatedDrawW int32 `json:"allocated_draw_w,omitempty"`
	// FeedLeg for three-phase feeds: A, B or C (power-outlet).
	FeedLeg string `json:"feed_leg,omitempty"`
	// Position identifies a module bay slot; it substitutes the {module}
	// token in template names on installation (module-bay).
	Position string `json:"position,omitempty"`
}

// attrFieldsByKind whitelists which attrs each kind may set.
var attrFieldsByKind = map[string][]string{
	"interface":    {"type", "mgmt_only"},
	"console-port": {"type"},
	"power-port":   {"type", "max_draw_w", "allocated_draw_w"},
	"power-outlet": {"type", "feed_leg"},
	"module-bay":   {"position"},
}

// Validate checks that only fields allowed for kind are set.
func (a ComponentAttrs) Validate(kind string) error {
	allowed := attrFieldsByKind[kind]
	has := func(f string) bool { return slices.Contains(allowed, f) }

	if a.Type != "" && !has("type") {
		return fault.New(fault.Invalid, "%s components have no type attribute", kind)
	}
	if a.MgmtOnly && !has("mgmt_only") {
		return fault.New(fault.Invalid, "%s components have no mgmt_only attribute", kind)
	}
	if (a.MaxDrawW != 0 || a.AllocatedDrawW != 0) && !has("max_draw_w") {
		return fault.New(fault.Invalid, "%s components have no power draw attributes", kind)
	}
	if a.MaxDrawW < 0 || a.AllocatedDrawW < 0 {
		return fault.New(fault.Invalid, "power draw must not be negative")
	}
	if a.FeedLeg != "" {
		if !has("feed_leg") {
			return fault.New(fault.Invalid, "%s components have no feed_leg attribute", kind)
		}
		if a.FeedLeg != "A" && a.FeedLeg != "B" && a.FeedLeg != "C" {
			return fault.New(fault.Invalid, "feed_leg must be A, B or C")
		}
	}
	if a.Position != "" && !has("position") {
		return fault.New(fault.Invalid, "%s components have no position attribute", kind)
	}
	return nil
}

// Marshal serializes attrs for storage.
func (a ComponentAttrs) Marshal() ([]byte, error) {
	b, err := json.Marshal(a)
	if err != nil {
		return nil, fault.Wrap(fault.Internal, err, "encoding component attrs")
	}
	return b, nil
}

// UnmarshalAttrs parses stored attrs.
func UnmarshalAttrs(b []byte) (ComponentAttrs, error) {
	var a ComponentAttrs
	if len(b) == 0 {
		return a, nil
	}
	if err := json.Unmarshal(b, &a); err != nil {
		return a, fault.Wrap(fault.Internal, err, "decoding component attrs")
	}
	return a, nil
}
