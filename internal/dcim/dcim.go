// Package dcim implements datacenter infrastructure models. Phase 1
// covers sites and the location tree; racks and devices follow.
package dcim

import (
	"slices"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/saku75/warren/internal/core/objtype"
	"github.com/saku75/warren/internal/db/gen"
)

var (
	// TypeSite: site slugs are globally scoped — sites are the roots of
	// every location path.
	TypeSite = objtype.Register(objtype.Type{Key: "dcim.site", Name: "Site", Plural: "Sites"})
	// TypeLocation: location slugs are scoped to their parent (or the
	// site, for roots); a location is addressed by its slug path,
	// e.g. "cph-dc1/building-a/floor-2".
	TypeLocation = objtype.Register(objtype.Type{Key: "dcim.location", Name: "Location", Plural: "Locations"})
)

// Statuses sites and locations may have; mirrors the CHECK constraints in
// migration 0001.
var Statuses = []string{"planned", "staging", "active", "decommissioning", "retired"}

// LocationKinds a location may have; mirrors migration 0001.
var LocationKinds = []string{"building", "floor", "room", "aisle", "row", "cage", "area"}

func validStatus(s string) bool       { return slices.Contains(Statuses, s) }
func validLocationKind(k string) bool { return slices.Contains(LocationKinds, k) }

// Service implements DCIM operations.
type Service struct {
	pool *pgxpool.Pool
	q    *gen.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: gen.New(pool)}
}
