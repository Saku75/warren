package dcim

import (
	"context"
	"fmt"
	"strings"

	"github.com/saku75/warren/internal/changelog"
	"github.com/saku75/warren/internal/core/fault"
	"github.com/saku75/warren/internal/core/id"
	"github.com/saku75/warren/internal/core/slug"
	"github.com/saku75/warren/internal/db/gen"
)

// LocationInput are the writable location fields. SiteRef is required;
// ParentRef (optional) is an ID or a path relative to the site, e.g.
// "building-a/floor-2". Empty Slug derives from Name, empty Kind defaults
// to area, empty Status to active.
type LocationInput struct {
	SiteRef     string
	ParentRef   string
	Name        string
	Slug        string
	Kind        string
	Status      string
	TenantRef   string
	Description string
}

// LocationUpdate carries partial changes; nil fields are untouched.
// ParentRef pointing at "" moves the location to the site root.
type LocationUpdate struct {
	ParentRef   *string
	Name        *string
	Slug        *string
	Kind        *string
	Status      *string
	TenantRef   *string
	Description *string
}

func locationSnapshot(l gen.Location) map[string]any {
	snap := map[string]any{
		"site_id":     l.SiteID.String(),
		"slug":        l.Slug,
		"name":        l.Name,
		"kind":        l.Kind,
		"status":      l.Status,
		"description": l.Description,
	}
	if l.ParentID != nil {
		snap["parent_id"] = l.ParentID.String()
	}
	if l.TenantID != nil {
		snap["tenant_id"] = l.TenantID.String()
	}
	return snap
}

func normalizeLocation(in LocationInput) (LocationInput, error) {
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
		in.Kind = "area"
	}
	if !validLocationKind(in.Kind) {
		return in, fault.New(fault.Invalid, "invalid kind %q (allowed: %v)", in.Kind, LocationKinds)
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if !validStatus(in.Status) {
		return in, fault.New(fault.Invalid, "invalid status %q (allowed: %v)", in.Status, Statuses)
	}
	return in, nil
}

// GetLocationByRef resolves a location from an ID or a full slug path
// ("site/loc/…/loc"). A bare site slug is not a location.
func (s *Service) GetLocationByRef(ctx context.Context, ref string) (gen.Location, error) {
	if lid, err := id.Parse(ref); err == nil {
		loc, err := s.q.GetLocation(ctx, lid)
		return loc, fault.FromDB(err, "location "+ref)
	}
	segments := strings.Split(ref, "/")
	if len(segments) < 2 {
		return gen.Location{}, fault.New(fault.Invalid, "location reference %q must be an ID or a path like site/location", ref)
	}
	site, err := s.GetSiteByRef(ctx, segments[0])
	if err != nil {
		return gen.Location{}, err
	}
	return s.resolveLocationPath(ctx, site.ID, segments[1:], ref)
}

// resolveLocationPath walks slug segments from the site root.
func (s *Service) resolveLocationPath(ctx context.Context, siteID id.ID, segments []string, display string) (gen.Location, error) {
	var parentID *id.ID
	var loc gen.Location
	for _, seg := range segments {
		if err := slug.Validate(seg); err != nil {
			return gen.Location{}, fault.Wrap(fault.Invalid, err, "invalid path segment %q", seg)
		}
		var err error
		loc, err = s.q.GetLocationByScope(ctx, gen.GetLocationByScopeParams{
			SiteID:   siteID,
			ParentID: parentID,
			Slug:     seg,
		})
		if err != nil {
			return gen.Location{}, fault.FromDB(err, "location "+display)
		}
		parentID = &loc.ID
	}
	return loc, nil
}

// resolveParentRef resolves a parent reference (ID or site-relative path)
// to a location that must belong to siteID.
func (s *Service) resolveParentRef(ctx context.Context, siteID id.ID, ref string) (*id.ID, error) {
	if ref == "" {
		return nil, nil
	}
	var parent gen.Location
	if pid, err := id.Parse(ref); err == nil {
		parent, err = s.q.GetLocation(ctx, pid)
		if err != nil {
			return nil, fault.FromDB(err, "parent location "+ref)
		}
	} else {
		parent, err = s.resolveLocationPath(ctx, siteID, strings.Split(ref, "/"), ref)
		if err != nil {
			return nil, err
		}
	}
	if parent.SiteID != siteID {
		return nil, fault.New(fault.Invalid, "parent location %q belongs to a different site", ref)
	}
	return &parent.ID, nil
}

// LocationPath returns the location's full slug path, site slug included.
func (s *Service) LocationPath(ctx context.Context, locationID id.ID) (string, error) {
	p, err := s.q.GetLocationPath(ctx, locationID)
	if err != nil {
		return "", fault.FromDB(err, "location path")
	}
	return p, nil
}

// CreateLocation makes a location and records it atomically.
func (s *Service) CreateLocation(ctx context.Context, in LocationInput) (gen.Location, error) {
	in, err := normalizeLocation(in)
	if err != nil {
		return gen.Location{}, err
	}
	if in.SiteRef == "" {
		return gen.Location{}, fault.New(fault.Invalid, "site is required")
	}
	site, err := s.GetSiteByRef(ctx, in.SiteRef)
	if err != nil {
		return gen.Location{}, err
	}
	parentID, err := s.resolveParentRef(ctx, site.ID, in.ParentRef)
	if err != nil {
		return gen.Location{}, err
	}
	tenantID, err := s.resolveTenantRef(ctx, in.TenantRef)
	if err != nil {
		return gen.Location{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.Location{}, fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	loc, err := q.CreateLocation(ctx, gen.CreateLocationParams{
		ID:          id.New(),
		SiteID:      site.ID,
		ParentID:    parentID,
		Slug:        in.Slug,
		Name:        in.Name,
		Kind:        in.Kind,
		Status:      in.Status,
		TenantID:    tenantID,
		Description: in.Description,
	})
	if err != nil {
		return gen.Location{}, fault.FromDB(err, "location "+in.Slug)
	}
	if err := changelog.Record(ctx, q, changelog.ActionCreate, TypeLocation.Key, loc.ID, loc.Name, nil, locationSnapshot(loc)); err != nil {
		return gen.Location{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.Location{}, fmt.Errorf("dcim: commit: %w", err)
	}
	return loc, nil
}

// UpdateLocationByRef applies a partial update. Reparenting is checked
// against cycles by walking up from the new parent: if the walk passes
// through the location being moved, the move is rejected.
func (s *Service) UpdateLocationByRef(ctx context.Context, ref string, up LocationUpdate) (gen.Location, error) {
	cur, err := s.GetLocationByRef(ctx, ref)
	if err != nil {
		return gen.Location{}, err
	}

	next := LocationInput{
		Name: cur.Name, Slug: cur.Slug, Kind: cur.Kind,
		Status: cur.Status, Description: cur.Description,
	}
	if up.Name != nil {
		next.Name = *up.Name
	}
	if up.Slug != nil {
		next.Slug = *up.Slug
	}
	if up.Kind != nil {
		next.Kind = *up.Kind
	}
	if up.Status != nil {
		next.Status = *up.Status
	}
	if up.Description != nil {
		next.Description = *up.Description
	}
	next, err = normalizeLocation(next)
	if err != nil {
		return gen.Location{}, err
	}

	parentID := cur.ParentID
	if up.ParentRef != nil {
		parentID, err = s.resolveParentRef(ctx, cur.SiteID, *up.ParentRef)
		if err != nil {
			return gen.Location{}, err
		}
		if err := s.checkNoCycle(ctx, cur.ID, parentID); err != nil {
			return gen.Location{}, err
		}
	}

	tenantID := cur.TenantID
	if up.TenantRef != nil {
		tenantID, err = s.resolveTenantRef(ctx, *up.TenantRef)
		if err != nil {
			return gen.Location{}, err
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.Location{}, fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	loc, err := q.UpdateLocation(ctx, gen.UpdateLocationParams{
		ID:          cur.ID,
		ParentID:    parentID,
		Slug:        next.Slug,
		Name:        next.Name,
		Kind:        next.Kind,
		Status:      next.Status,
		TenantID:    tenantID,
		Description: next.Description,
	})
	if err != nil {
		return gen.Location{}, fault.FromDB(err, "location "+ref)
	}
	if err := changelog.Record(ctx, q, changelog.ActionUpdate, TypeLocation.Key, loc.ID, loc.Name, locationSnapshot(cur), locationSnapshot(loc)); err != nil {
		return gen.Location{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.Location{}, fmt.Errorf("dcim: commit: %w", err)
	}
	return loc, nil
}

// checkNoCycle rejects a reparent that would make moved its own ancestor.
// Location trees are shallow, so a parent-chain walk is plenty; the walk
// is bounded defensively all the same.
func (s *Service) checkNoCycle(ctx context.Context, moved id.ID, newParent *id.ID) error {
	const maxDepth = 100
	cursor := newParent
	for depth := 0; cursor != nil; depth++ {
		if depth > maxDepth {
			return fault.New(fault.Invalid, "location tree deeper than %d levels", maxDepth)
		}
		if *cursor == moved {
			return fault.New(fault.Invalid, "cannot move a location under its own descendant")
		}
		anc, err := s.q.GetLocation(ctx, *cursor)
		if err != nil {
			return fault.FromDB(err, "location ancestry")
		}
		cursor = anc.ParentID
	}
	return nil
}

// DeleteLocationByRef removes a location. Children block the delete via
// the self-referencing foreign key, surfaced as a Conflict.
func (s *Service) DeleteLocationByRef(ctx context.Context, ref string) error {
	cur, err := s.GetLocationByRef(ctx, ref)
	if err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	if _, err := q.DeleteLocation(ctx, cur.ID); err != nil {
		return fault.FromDB(err, "location "+ref)
	}
	if err := changelog.Record(ctx, q, changelog.ActionDelete, TypeLocation.Key, cur.ID, cur.Name, locationSnapshot(cur), nil); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("dcim: commit: %w", err)
	}
	return nil
}

// LocationChildren returns a location's direct children, name-ordered.
func (s *Service) LocationChildren(ctx context.Context, locationID id.ID) ([]gen.Location, error) {
	items, err := s.q.ListLocationChildren(ctx, &locationID)
	if err != nil {
		return nil, fmt.Errorf("dcim: list children: %w", err)
	}
	return items, nil
}

// LocationNode is a location with its children, for tree rendering.
type LocationNode struct {
	Location gen.Location
	// Path is the full slug path including the site slug.
	Path     string
	Children []*LocationNode
}

// LocationTree assembles the site's location tree (roots ordered by name).
func (s *Service) LocationTree(ctx context.Context, siteID id.ID) ([]*LocationNode, error) {
	site, err := s.q.GetSite(ctx, siteID)
	if err != nil {
		return nil, fault.FromDB(err, "site")
	}
	all, err := s.q.ListLocationsBySite(ctx, siteID)
	if err != nil {
		return nil, fmt.Errorf("dcim: list locations: %w", err)
	}

	nodes := make(map[id.ID]*LocationNode, len(all))
	for _, l := range all {
		nodes[l.ID] = &LocationNode{Location: l}
	}
	var roots []*LocationNode
	for _, l := range all {
		n := nodes[l.ID]
		if l.ParentID == nil {
			n.Path = site.Slug + "/" + l.Slug
			roots = append(roots, n)
		} else {
			nodes[*l.ParentID].Children = append(nodes[*l.ParentID].Children, n)
		}
	}
	// Children inherit their path from the parent; rows arrive name-sorted,
	// so a breadth-first fill keeps every level ordered.
	var fill func(n *LocationNode)
	fill = func(n *LocationNode) {
		for _, c := range n.Children {
			c.Path = n.Path + "/" + c.Location.Slug
			fill(c)
		}
	}
	for _, r := range roots {
		fill(r)
	}
	return roots, nil
}
