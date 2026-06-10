package tenancy

import (
	"context"
	"fmt"
	"strings"

	"github.com/saku75/warren/internal/changelog"
	"github.com/saku75/warren/internal/core/fault"
	"github.com/saku75/warren/internal/core/id"
	"github.com/saku75/warren/internal/core/objtype"
	"github.com/saku75/warren/internal/core/slug"
	"github.com/saku75/warren/internal/core/tree"
	"github.com/saku75/warren/internal/db/gen"
)

// TypeTenantGroup is the registered type key for tenant groups. Groups
// form one global tree; slugs are scoped to the parent group and a group
// is addressed by its slug path, e.g. "emea/dk".
var TypeTenantGroup = objtype.Register(objtype.Type{Key: "tenancy.tenant-group", Name: "Tenant group", Plural: "Tenant groups"})

// GroupInput are the writable tenant group fields. ParentRef (optional)
// is an ID or a slug path; empty means a root group.
type GroupInput struct {
	Name        string
	Slug        string
	ParentRef   string
	Description string
}

// GroupUpdate carries partial changes; nil fields are untouched.
// ParentRef pointing at "" moves the group to the root.
type GroupUpdate struct {
	Name        *string
	Slug        *string
	ParentRef   *string
	Description *string
}

func groupSnapshot(g gen.TenantGroup) map[string]any {
	snap := map[string]any{
		"slug":        g.Slug,
		"name":        g.Name,
		"description": g.Description,
	}
	if g.ParentID != nil {
		snap["parent_id"] = g.ParentID.String()
	}
	return snap
}

func normalizeGroup(in GroupInput) (GroupInput, error) {
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

// GetGroupByRef resolves a tenant group from an ID or a slug path.
func (s *Service) GetGroupByRef(ctx context.Context, ref string) (gen.TenantGroup, error) {
	if gid, err := id.Parse(ref); err == nil {
		g, err := s.q.GetTenantGroup(ctx, gid)
		return g, fault.FromDB(err, "tenant group "+ref)
	}
	var parentID *id.ID
	var g gen.TenantGroup
	for _, seg := range strings.Split(ref, "/") {
		if err := slug.Validate(seg); err != nil {
			return gen.TenantGroup{}, fault.Wrap(fault.Invalid, err, "invalid path segment %q", seg)
		}
		var err error
		g, err = s.q.GetTenantGroupByScope(ctx, gen.GetTenantGroupByScopeParams{
			ParentID: parentID,
			Slug:     seg,
		})
		if err != nil {
			return gen.TenantGroup{}, fault.FromDB(err, "tenant group "+ref)
		}
		parentID = &g.ID
	}
	return g, nil
}

// resolveGroupRef turns an ID-or-path into a group ID, or nil for "".
func (s *Service) resolveGroupRef(ctx context.Context, ref string) (*id.ID, error) {
	if ref == "" {
		return nil, nil
	}
	g, err := s.GetGroupByRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	return &g.ID, nil
}

// GroupPath returns the group's full slug path.
func (s *Service) GroupPath(ctx context.Context, groupID id.ID) (string, error) {
	p, err := s.q.GetTenantGroupPath(ctx, groupID)
	if err != nil {
		return "", fault.FromDB(err, "tenant group path")
	}
	return p, nil
}

// GroupTree assembles the full tenant group tree.
func (s *Service) GroupTree(ctx context.Context) ([]*tree.Node[gen.TenantGroup], error) {
	all, err := s.q.ListTenantGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("tenancy: list groups: %w", err)
	}
	return tree.Build(all,
		func(g gen.TenantGroup) id.ID { return g.ID },
		func(g gen.TenantGroup) *id.ID { return g.ParentID },
		func(g gen.TenantGroup) string { return g.Slug },
		"",
	), nil
}

// GroupChildren returns a group's direct children, name-ordered.
func (s *Service) GroupChildren(ctx context.Context, groupID id.ID) ([]gen.TenantGroup, error) {
	items, err := s.q.ListTenantGroupChildren(ctx, &groupID)
	if err != nil {
		return nil, fmt.Errorf("tenancy: list group children: %w", err)
	}
	return items, nil
}

// TenantsInGroup returns the tenants directly assigned to the group.
func (s *Service) TenantsInGroup(ctx context.Context, groupID id.ID) ([]gen.Tenant, error) {
	items, err := s.q.ListTenantsByGroup(ctx, &groupID)
	if err != nil {
		return nil, fmt.Errorf("tenancy: list group tenants: %w", err)
	}
	return items, nil
}

// CreateGroup makes a tenant group and records it atomically.
func (s *Service) CreateGroup(ctx context.Context, in GroupInput) (gen.TenantGroup, error) {
	in, err := normalizeGroup(in)
	if err != nil {
		return gen.TenantGroup{}, err
	}
	parentID, err := s.resolveGroupRef(ctx, in.ParentRef)
	if err != nil {
		return gen.TenantGroup{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.TenantGroup{}, fmt.Errorf("tenancy: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	g, err := q.CreateTenantGroup(ctx, gen.CreateTenantGroupParams{
		ID:          id.New(),
		ParentID:    parentID,
		Slug:        in.Slug,
		Name:        in.Name,
		Description: in.Description,
	})
	if err != nil {
		return gen.TenantGroup{}, fault.FromDB(err, "tenant group "+in.Slug)
	}
	if err := changelog.Record(ctx, q, changelog.ActionCreate, TypeTenantGroup.Key, g.ID, g.Name, nil, groupSnapshot(g)); err != nil {
		return gen.TenantGroup{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.TenantGroup{}, fmt.Errorf("tenancy: commit: %w", err)
	}
	return g, nil
}

// UpdateGroupByRef applies a partial update, rejecting reparenting cycles.
func (s *Service) UpdateGroupByRef(ctx context.Context, ref string, up GroupUpdate) (gen.TenantGroup, error) {
	cur, err := s.GetGroupByRef(ctx, ref)
	if err != nil {
		return gen.TenantGroup{}, err
	}

	next := GroupInput{Name: cur.Name, Slug: cur.Slug, Description: cur.Description}
	if up.Name != nil {
		next.Name = *up.Name
	}
	if up.Slug != nil {
		next.Slug = *up.Slug
	}
	if up.Description != nil {
		next.Description = *up.Description
	}
	next, err = normalizeGroup(next)
	if err != nil {
		return gen.TenantGroup{}, err
	}

	parentID := cur.ParentID
	if up.ParentRef != nil {
		parentID, err = s.resolveGroupRef(ctx, *up.ParentRef)
		if err != nil {
			return gen.TenantGroup{}, err
		}
		if err := s.checkNoGroupCycle(ctx, cur.ID, parentID); err != nil {
			return gen.TenantGroup{}, err
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.TenantGroup{}, fmt.Errorf("tenancy: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	g, err := q.UpdateTenantGroup(ctx, gen.UpdateTenantGroupParams{
		ID:          cur.ID,
		ParentID:    parentID,
		Slug:        next.Slug,
		Name:        next.Name,
		Description: next.Description,
	})
	if err != nil {
		return gen.TenantGroup{}, fault.FromDB(err, "tenant group "+ref)
	}
	if err := changelog.Record(ctx, q, changelog.ActionUpdate, TypeTenantGroup.Key, g.ID, g.Name, groupSnapshot(cur), groupSnapshot(g)); err != nil {
		return gen.TenantGroup{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.TenantGroup{}, fmt.Errorf("tenancy: commit: %w", err)
	}
	return g, nil
}

// checkNoGroupCycle rejects a reparent that would make moved its own
// ancestor; same bounded walk as locations.
func (s *Service) checkNoGroupCycle(ctx context.Context, moved id.ID, newParent *id.ID) error {
	const maxDepth = 100
	cursor := newParent
	for depth := 0; cursor != nil; depth++ {
		if depth > maxDepth {
			return fault.New(fault.Invalid, "group tree deeper than %d levels", maxDepth)
		}
		if *cursor == moved {
			return fault.New(fault.Invalid, "cannot move a group under its own descendant")
		}
		anc, err := s.q.GetTenantGroup(ctx, *cursor)
		if err != nil {
			return fault.FromDB(err, "tenant group ancestry")
		}
		cursor = anc.ParentID
	}
	return nil
}

// DeleteGroupByRef removes a tenant group. Children and member tenants
// block the delete via foreign keys, surfaced as a Conflict.
func (s *Service) DeleteGroupByRef(ctx context.Context, ref string) error {
	cur, err := s.GetGroupByRef(ctx, ref)
	if err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("tenancy: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	if _, err := q.DeleteTenantGroup(ctx, cur.ID); err != nil {
		return fault.FromDB(err, "tenant group "+ref)
	}
	if err := changelog.Record(ctx, q, changelog.ActionDelete, TypeTenantGroup.Key, cur.ID, cur.Name, groupSnapshot(cur), nil); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("tenancy: commit: %w", err)
	}
	return nil
}
