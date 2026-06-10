// Package tenancy implements tenants: the ownership dimension that every
// asset-class object in Warren can optionally reference (design doc §5.8).
//
// Tenants are hierarchical: a tenant may have a parent tenant, and every
// level of the hierarchy is a real, assignable owner. Tenant slugs stay
// globally unique — tenants are referenced from many object types, so
// their refs are short and stable; the hierarchy is organizational, not
// an addressing scope.
package tenancy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/saku75/warren/internal/changelog"
	"github.com/saku75/warren/internal/core/fault"
	"github.com/saku75/warren/internal/core/id"
	"github.com/saku75/warren/internal/core/objtype"
	"github.com/saku75/warren/internal/core/slug"
	"github.com/saku75/warren/internal/core/tree"
	"github.com/saku75/warren/internal/db/gen"
)

// TypeTenant is the registered type key for tenants.
var TypeTenant = objtype.Register(objtype.Type{Key: "tenancy.tenant", Name: "Tenant", Plural: "Tenants"})

// Service implements tenant operations.
type Service struct {
	pool *pgxpool.Pool
	q    *gen.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: gen.New(pool)}
}

// Input are the writable tenant fields. An empty Slug is derived from
// Name. ParentRef (optional) is a tenant ID or slug; empty means a root
// tenant.
type Input struct {
	Name        string
	Slug        string
	ParentRef   string
	Description string
}

// Update carries partial changes; nil fields are left untouched. Clearing
// the parent is requested with a pointer to "".
type Update struct {
	Name        *string
	Slug        *string
	ParentRef   *string
	Description *string
}

func normalize(in Input) (Input, error) {
	if in.Name == "" {
		return in, fault.New(fault.Invalid, "name is required")
	}
	if in.Slug == "" {
		s, err := slug.Make(in.Name)
		if err != nil {
			return in, fault.Wrap(fault.Invalid, err, "cannot derive slug from name; provide one")
		}
		in.Slug = s
	} else if err := slug.Validate(in.Slug); err != nil {
		return in, fault.Wrap(fault.Invalid, err, "invalid slug")
	}
	return in, nil
}

func snapshot(t gen.Tenant) map[string]any {
	snap := map[string]any{
		"slug":        t.Slug,
		"name":        t.Name,
		"description": t.Description,
	}
	if t.ParentID != nil {
		snap["parent_id"] = t.ParentID.String()
	}
	return snap
}

// GetByRef resolves a tenant from an ID or its (global) slug.
func (s *Service) GetByRef(ctx context.Context, ref string) (gen.Tenant, error) {
	if tid, err := id.Parse(ref); err == nil {
		t, err := s.q.GetTenant(ctx, tid)
		return t, fault.FromDB(err, "tenant "+ref)
	}
	if err := slug.Validate(ref); err != nil {
		return gen.Tenant{}, fault.Wrap(fault.Invalid, err, "invalid tenant reference %q", ref)
	}
	t, err := s.q.GetTenantBySlug(ctx, ref)
	return t, fault.FromDB(err, "tenant "+ref)
}

// resolveParentRef turns an ID-or-slug into a tenant ID, or nil for "".
func (s *Service) resolveParentRef(ctx context.Context, ref string) (*id.ID, error) {
	if ref == "" {
		return nil, nil
	}
	t, err := s.GetByRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	return &t.ID, nil
}

// List returns tenants (with parent display fields) plus the total count.
func (s *Service) List(ctx context.Context, limit, offset int32) ([]gen.ListTenantsRow, int64, error) {
	total, err := s.q.CountTenants(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("tenancy: count: %w", err)
	}
	items, err := s.q.ListTenants(ctx, gen.ListTenantsParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, fmt.Errorf("tenancy: list: %w", err)
	}
	return items, total, nil
}

// Count returns the total number of tenants.
func (s *Service) Count(ctx context.Context) (int64, error) {
	return s.q.CountTenants(ctx)
}

// Tree assembles the full tenant hierarchy. Node paths are slug chains
// for display; tenants are still addressed by their global slug alone.
func (s *Service) Tree(ctx context.Context) ([]*tree.Node[gen.Tenant], error) {
	all, err := s.q.ListTenantsTree(ctx)
	if err != nil {
		return nil, fmt.Errorf("tenancy: list tree: %w", err)
	}
	return tree.Build(all,
		func(t gen.Tenant) id.ID { return t.ID },
		func(t gen.Tenant) *id.ID { return t.ParentID },
		func(t gen.Tenant) string { return t.Slug },
		"",
	), nil
}

// Children returns a tenant's direct children, name-ordered.
func (s *Service) Children(ctx context.Context, tenantID id.ID) ([]gen.Tenant, error) {
	items, err := s.q.ListTenantChildren(ctx, &tenantID)
	if err != nil {
		return nil, fmt.Errorf("tenancy: list children: %w", err)
	}
	return items, nil
}

// Create makes a tenant and records it in the change log atomically.
func (s *Service) Create(ctx context.Context, in Input) (gen.Tenant, error) {
	in, err := normalize(in)
	if err != nil {
		return gen.Tenant{}, err
	}
	parentID, err := s.resolveParentRef(ctx, in.ParentRef)
	if err != nil {
		return gen.Tenant{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.Tenant{}, fmt.Errorf("tenancy: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	t, err := q.CreateTenant(ctx, gen.CreateTenantParams{
		ID:          id.New(),
		ParentID:    parentID,
		Slug:        in.Slug,
		Name:        in.Name,
		Description: in.Description,
	})
	if err != nil {
		return gen.Tenant{}, fault.FromDB(err, "tenant "+in.Slug)
	}
	if err := changelog.Record(ctx, q, changelog.ActionCreate, TypeTenant.Key, t.ID, t.Name, nil, snapshot(t)); err != nil {
		return gen.Tenant{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.Tenant{}, fmt.Errorf("tenancy: commit: %w", err)
	}
	return t, nil
}

// UpdateByRef applies a partial update, rejecting reparenting cycles.
func (s *Service) UpdateByRef(ctx context.Context, ref string, up Update) (gen.Tenant, error) {
	cur, err := s.GetByRef(ctx, ref)
	if err != nil {
		return gen.Tenant{}, err
	}

	// Renaming does not re-derive the slug; slugs change only when
	// explicitly requested.
	next := Input{Name: cur.Name, Slug: cur.Slug, Description: cur.Description}
	if up.Name != nil {
		next.Name = *up.Name
	}
	if up.Slug != nil {
		next.Slug = *up.Slug
	}
	if up.Description != nil {
		next.Description = *up.Description
	}
	next, err = normalize(next)
	if err != nil {
		return gen.Tenant{}, err
	}

	parentID := cur.ParentID
	if up.ParentRef != nil {
		parentID, err = s.resolveParentRef(ctx, *up.ParentRef)
		if err != nil {
			return gen.Tenant{}, err
		}
		if err := s.checkNoCycle(ctx, cur.ID, parentID); err != nil {
			return gen.Tenant{}, err
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.Tenant{}, fmt.Errorf("tenancy: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	t, err := q.UpdateTenant(ctx, gen.UpdateTenantParams{
		ID:          cur.ID,
		ParentID:    parentID,
		Slug:        next.Slug,
		Name:        next.Name,
		Description: next.Description,
	})
	if err != nil {
		return gen.Tenant{}, fault.FromDB(err, "tenant "+ref)
	}
	if err := changelog.Record(ctx, q, changelog.ActionUpdate, TypeTenant.Key, t.ID, t.Name, snapshot(cur), snapshot(t)); err != nil {
		return gen.Tenant{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.Tenant{}, fmt.Errorf("tenancy: commit: %w", err)
	}
	return t, nil
}

// checkNoCycle rejects a reparent that would make moved its own ancestor.
func (s *Service) checkNoCycle(ctx context.Context, moved id.ID, newParent *id.ID) error {
	const maxDepth = 100
	cursor := newParent
	for depth := 0; cursor != nil; depth++ {
		if depth > maxDepth {
			return fault.New(fault.Invalid, "tenant tree deeper than %d levels", maxDepth)
		}
		if *cursor == moved {
			return fault.New(fault.Invalid, "cannot move a tenant under its own descendant")
		}
		anc, err := s.q.GetTenant(ctx, *cursor)
		if err != nil {
			return fault.FromDB(err, "tenant ancestry")
		}
		cursor = anc.ParentID
	}
	return nil
}

// DeleteByRef removes a tenant. Child tenants and objects still
// referencing it block the delete (foreign keys), surfaced as a Conflict.
func (s *Service) DeleteByRef(ctx context.Context, ref string) error {
	cur, err := s.GetByRef(ctx, ref)
	if err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("tenancy: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	if _, err := q.DeleteTenant(ctx, cur.ID); err != nil {
		return fault.FromDB(err, "tenant "+ref)
	}
	if err := changelog.Record(ctx, q, changelog.ActionDelete, TypeTenant.Key, cur.ID, cur.Name, snapshot(cur), nil); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("tenancy: commit: %w", err)
	}
	return nil
}
