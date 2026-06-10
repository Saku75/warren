// Package tenancy implements tenants: the ownership dimension that every
// asset-class object in Warren can optionally reference (design doc §5.8).
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
	"github.com/saku75/warren/internal/db/gen"
)

// TypeTenant is the registered type key for tenants. Tenant slugs are
// globally scoped: a tenant is addressed by its slug alone.
var TypeTenant = objtype.Register(objtype.Type{Key: "tenancy.tenant", Name: "Tenant", Plural: "Tenants"})

// Service implements tenant operations.
type Service struct {
	pool *pgxpool.Pool
	q    *gen.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: gen.New(pool)}
}

// Input are the writable tenant fields. An empty Slug is derived from Name.
type Input struct {
	Name        string
	Slug        string
	Description string
}

// Update carries partial changes; nil fields are left untouched.
type Update struct {
	Name        *string
	Slug        *string
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
	return map[string]any{
		"slug":        t.Slug,
		"name":        t.Name,
		"description": t.Description,
	}
}

// Create makes a tenant and records it in the change log atomically.
func (s *Service) Create(ctx context.Context, in Input) (gen.Tenant, error) {
	in, err := normalize(in)
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

// GetByRef resolves a tenant from an ID or a slug.
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

// List returns tenants ordered by name plus the total count.
func (s *Service) List(ctx context.Context, limit, offset int32) ([]gen.Tenant, int64, error) {
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

// UpdateByRef applies a partial update and records before/after.
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

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.Tenant{}, fmt.Errorf("tenancy: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	t, err := q.UpdateTenant(ctx, gen.UpdateTenantParams{
		ID:          cur.ID,
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

// DeleteByRef removes a tenant. Objects still referencing it block the
// delete (foreign keys), surfaced as a Conflict.
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
