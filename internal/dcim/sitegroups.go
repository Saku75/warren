package dcim

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/saku75/warren/internal/changelog"
	"github.com/saku75/warren/internal/core/fault"
	"github.com/saku75/warren/internal/core/id"
	"github.com/saku75/warren/internal/core/objtype"
	"github.com/saku75/warren/internal/core/slug"
	"github.com/saku75/warren/internal/core/tree"
	"github.com/saku75/warren/internal/db/gen"
)

// TypeSiteGroup is the registered type key for site groups: one nestable
// tree replacing NetBox's separate Region and SiteGroup models, with a
// kind label carrying the distinction (design doc §5.3). Slugs are scoped
// to the parent group; groups are addressed by slug path ("emea/dk").
var TypeSiteGroup = objtype.Register(objtype.Type{Key: "dcim.site-group", Name: "Site group", Plural: "Site groups"})

// SiteGroupKinds a site group may have; mirrors migration 0002.
var SiteGroupKinds = []string{"region", "group"}

func validSiteGroupKind(k string) bool { return slices.Contains(SiteGroupKinds, k) }

// SiteGroupInput are the writable site group fields. ParentRef (optional)
// is an ID or slug path; empty Kind defaults to group.
type SiteGroupInput struct {
	Name        string
	Slug        string
	Kind        string
	ParentRef   string
	Description string
}

// SiteGroupUpdate carries partial changes; nil fields are untouched.
// ParentRef pointing at "" moves the group to the root.
type SiteGroupUpdate struct {
	Name        *string
	Slug        *string
	Kind        *string
	ParentRef   *string
	Description *string
}

func siteGroupSnapshot(g gen.SiteGroup) map[string]any {
	snap := map[string]any{
		"slug":        g.Slug,
		"name":        g.Name,
		"kind":        g.Kind,
		"description": g.Description,
	}
	if g.ParentID != nil {
		snap["parent_id"] = g.ParentID.String()
	}
	return snap
}

func normalizeSiteGroup(in SiteGroupInput) (SiteGroupInput, error) {
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
	if in.Kind == "" {
		in.Kind = "group"
	}
	if !validSiteGroupKind(in.Kind) {
		return in, fault.New(fault.Invalid, "invalid kind %q (allowed: %v)", in.Kind, SiteGroupKinds)
	}
	return in, nil
}

// GetSiteGroupByRef resolves a site group from an ID or a slug path.
func (s *Service) GetSiteGroupByRef(ctx context.Context, ref string) (gen.SiteGroup, error) {
	if gid, err := id.Parse(ref); err == nil {
		g, err := s.q.GetSiteGroup(ctx, gid)
		return g, fault.FromDB(err, "site group "+ref)
	}
	var parentID *id.ID
	var g gen.SiteGroup
	for _, seg := range strings.Split(ref, "/") {
		if err := slug.Validate(seg); err != nil {
			return gen.SiteGroup{}, fault.Wrap(fault.Invalid, err, "invalid path segment %q", seg)
		}
		var err error
		g, err = s.q.GetSiteGroupByScope(ctx, gen.GetSiteGroupByScopeParams{
			ParentID: parentID,
			Slug:     seg,
		})
		if err != nil {
			return gen.SiteGroup{}, fault.FromDB(err, "site group "+ref)
		}
		parentID = &g.ID
	}
	return g, nil
}

// resolveSiteGroupRef turns an ID-or-path into a group ID, or nil for "".
func (s *Service) resolveSiteGroupRef(ctx context.Context, ref string) (*id.ID, error) {
	if ref == "" {
		return nil, nil
	}
	g, err := s.GetSiteGroupByRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	return &g.ID, nil
}

// SiteGroupPath returns the group's full slug path.
func (s *Service) SiteGroupPath(ctx context.Context, groupID id.ID) (string, error) {
	p, err := s.q.GetSiteGroupPath(ctx, groupID)
	if err != nil {
		return "", fault.FromDB(err, "site group path")
	}
	return p, nil
}

// SiteGroupTree assembles the full site group tree.
func (s *Service) SiteGroupTree(ctx context.Context) ([]*tree.Node[gen.SiteGroup], error) {
	all, err := s.q.ListSiteGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("dcim: list site groups: %w", err)
	}
	return tree.Build(all,
		func(g gen.SiteGroup) id.ID { return g.ID },
		func(g gen.SiteGroup) *id.ID { return g.ParentID },
		func(g gen.SiteGroup) string { return g.Slug },
		"",
	), nil
}

// SiteGroupChildren returns a group's direct children, name-ordered.
func (s *Service) SiteGroupChildren(ctx context.Context, groupID id.ID) ([]gen.SiteGroup, error) {
	items, err := s.q.ListSiteGroupChildren(ctx, &groupID)
	if err != nil {
		return nil, fmt.Errorf("dcim: list site group children: %w", err)
	}
	return items, nil
}

// SitesInGroup returns the sites directly assigned to the group.
func (s *Service) SitesInGroup(ctx context.Context, groupID id.ID) ([]gen.Site, error) {
	items, err := s.q.ListSitesByGroup(ctx, &groupID)
	if err != nil {
		return nil, fmt.Errorf("dcim: list group sites: %w", err)
	}
	return items, nil
}

// CreateSiteGroup makes a site group and records it atomically.
func (s *Service) CreateSiteGroup(ctx context.Context, in SiteGroupInput) (gen.SiteGroup, error) {
	in, err := normalizeSiteGroup(in)
	if err != nil {
		return gen.SiteGroup{}, err
	}
	parentID, err := s.resolveSiteGroupRef(ctx, in.ParentRef)
	if err != nil {
		return gen.SiteGroup{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.SiteGroup{}, fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	g, err := q.CreateSiteGroup(ctx, gen.CreateSiteGroupParams{
		ID:          id.New(),
		ParentID:    parentID,
		Slug:        in.Slug,
		Name:        in.Name,
		Kind:        in.Kind,
		Description: in.Description,
	})
	if err != nil {
		return gen.SiteGroup{}, fault.FromDB(err, "site group "+in.Slug)
	}
	if err := changelog.Record(ctx, q, changelog.ActionCreate, TypeSiteGroup.Key, g.ID, g.Name, nil, siteGroupSnapshot(g)); err != nil {
		return gen.SiteGroup{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.SiteGroup{}, fmt.Errorf("dcim: commit: %w", err)
	}
	return g, nil
}

// UpdateSiteGroupByRef applies a partial update, rejecting cycles.
func (s *Service) UpdateSiteGroupByRef(ctx context.Context, ref string, up SiteGroupUpdate) (gen.SiteGroup, error) {
	cur, err := s.GetSiteGroupByRef(ctx, ref)
	if err != nil {
		return gen.SiteGroup{}, err
	}

	next := SiteGroupInput{Name: cur.Name, Slug: cur.Slug, Kind: cur.Kind, Description: cur.Description}
	if up.Name != nil {
		next.Name = *up.Name
	}
	if up.Slug != nil {
		next.Slug = *up.Slug
	}
	if up.Kind != nil {
		next.Kind = *up.Kind
	}
	if up.Description != nil {
		next.Description = *up.Description
	}
	next, err = normalizeSiteGroup(next)
	if err != nil {
		return gen.SiteGroup{}, err
	}

	parentID := cur.ParentID
	if up.ParentRef != nil {
		parentID, err = s.resolveSiteGroupRef(ctx, *up.ParentRef)
		if err != nil {
			return gen.SiteGroup{}, err
		}
		if err := s.checkNoSiteGroupCycle(ctx, cur.ID, parentID); err != nil {
			return gen.SiteGroup{}, err
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.SiteGroup{}, fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	g, err := q.UpdateSiteGroup(ctx, gen.UpdateSiteGroupParams{
		ID:          cur.ID,
		ParentID:    parentID,
		Slug:        next.Slug,
		Name:        next.Name,
		Kind:        next.Kind,
		Description: next.Description,
	})
	if err != nil {
		return gen.SiteGroup{}, fault.FromDB(err, "site group "+ref)
	}
	if err := changelog.Record(ctx, q, changelog.ActionUpdate, TypeSiteGroup.Key, g.ID, g.Name, siteGroupSnapshot(cur), siteGroupSnapshot(g)); err != nil {
		return gen.SiteGroup{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.SiteGroup{}, fmt.Errorf("dcim: commit: %w", err)
	}
	return g, nil
}

func (s *Service) checkNoSiteGroupCycle(ctx context.Context, moved id.ID, newParent *id.ID) error {
	const maxDepth = 100
	cursor := newParent
	for depth := 0; cursor != nil; depth++ {
		if depth > maxDepth {
			return fault.New(fault.Invalid, "group tree deeper than %d levels", maxDepth)
		}
		if *cursor == moved {
			return fault.New(fault.Invalid, "cannot move a group under its own descendant")
		}
		anc, err := s.q.GetSiteGroup(ctx, *cursor)
		if err != nil {
			return fault.FromDB(err, "site group ancestry")
		}
		cursor = anc.ParentID
	}
	return nil
}

// DeleteSiteGroupByRef removes a site group. Children and member sites
// block the delete via foreign keys, surfaced as a Conflict.
func (s *Service) DeleteSiteGroupByRef(ctx context.Context, ref string) error {
	cur, err := s.GetSiteGroupByRef(ctx, ref)
	if err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	if _, err := q.DeleteSiteGroup(ctx, cur.ID); err != nil {
		return fault.FromDB(err, "site group "+ref)
	}
	if err := changelog.Record(ctx, q, changelog.ActionDelete, TypeSiteGroup.Key, cur.ID, cur.Name, siteGroupSnapshot(cur), nil); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("dcim: commit: %w", err)
	}
	return nil
}
