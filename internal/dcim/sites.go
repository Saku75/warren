package dcim

import (
	"context"
	"fmt"

	"github.com/saku75/warren/internal/changelog"
	"github.com/saku75/warren/internal/core/fault"
	"github.com/saku75/warren/internal/core/id"
	"github.com/saku75/warren/internal/core/slug"
	"github.com/saku75/warren/internal/db/gen"
)

// SiteInput are the writable site fields. Empty Slug derives from Name,
// empty Status defaults to active. TenantRef is an ID or slug; empty means
// no tenant.
type SiteInput struct {
	Name        string
	Slug        string
	Status      string
	TenantRef   string
	GroupRef    string
	Facility    string
	TimeZone    string
	Description string
}

// SiteUpdate carries partial changes; nil fields are untouched. Clearing
// the tenant is requested with a pointer to "".
type SiteUpdate struct {
	Name        *string
	Slug        *string
	Status      *string
	TenantRef   *string
	GroupRef    *string
	Facility    *string
	TimeZone    *string
	Description *string
}

func siteSnapshot(s gen.Site) map[string]any {
	snap := map[string]any{
		"slug":        s.Slug,
		"name":        s.Name,
		"status":      s.Status,
		"facility":    s.Facility,
		"time_zone":   s.TimeZone,
		"description": s.Description,
	}
	if s.TenantID != nil {
		snap["tenant_id"] = s.TenantID.String()
	}
	if s.SiteGroupID != nil {
		snap["site_group_id"] = s.SiteGroupID.String()
	}
	return snap
}

// resolveTenantRef turns an ID-or-slug into a tenant ID, or nil for "".
func (s *Service) resolveTenantRef(ctx context.Context, ref string) (*id.ID, error) {
	if ref == "" {
		return nil, nil
	}
	if tid, err := id.Parse(ref); err == nil {
		t, err := s.q.GetTenant(ctx, tid)
		if err != nil {
			return nil, fault.FromDB(err, "tenant "+ref)
		}
		return &t.ID, nil
	}
	if err := slug.Validate(ref); err != nil {
		return nil, fault.Wrap(fault.Invalid, err, "invalid tenant reference %q", ref)
	}
	t, err := s.q.GetTenantBySlug(ctx, ref)
	if err != nil {
		return nil, fault.FromDB(err, "tenant "+ref)
	}
	return &t.ID, nil
}

func normalizeSite(in SiteInput) (SiteInput, error) {
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
	if in.Status == "" {
		in.Status = "active"
	}
	if !validStatus(in.Status) {
		return in, fault.New(fault.Invalid, "invalid status %q (allowed: %v)", in.Status, Statuses)
	}
	return in, nil
}

// CreateSite makes a site and records it in the change log atomically.
func (s *Service) CreateSite(ctx context.Context, in SiteInput) (gen.Site, error) {
	in, err := normalizeSite(in)
	if err != nil {
		return gen.Site{}, err
	}
	tenantID, err := s.resolveTenantRef(ctx, in.TenantRef)
	if err != nil {
		return gen.Site{}, err
	}
	groupID, err := s.resolveSiteGroupRef(ctx, in.GroupRef)
	if err != nil {
		return gen.Site{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.Site{}, fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	site, err := q.CreateSite(ctx, gen.CreateSiteParams{
		ID:          id.New(),
		Slug:        in.Slug,
		Name:        in.Name,
		Status:      in.Status,
		TenantID:    tenantID,
		SiteGroupID: groupID,
		Facility:    in.Facility,
		TimeZone:    in.TimeZone,
		Description: in.Description,
	})
	if err != nil {
		return gen.Site{}, fault.FromDB(err, "site "+in.Slug)
	}
	if err := changelog.Record(ctx, q, changelog.ActionCreate, TypeSite.Key, site.ID, site.Name, nil, siteSnapshot(site)); err != nil {
		return gen.Site{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.Site{}, fmt.Errorf("dcim: commit: %w", err)
	}
	return site, nil
}

// GetSiteByRef resolves a site from an ID or its (global) slug.
func (s *Service) GetSiteByRef(ctx context.Context, ref string) (gen.Site, error) {
	if sid, err := id.Parse(ref); err == nil {
		site, err := s.q.GetSite(ctx, sid)
		return site, fault.FromDB(err, "site "+ref)
	}
	if err := slug.Validate(ref); err != nil {
		return gen.Site{}, fault.Wrap(fault.Invalid, err, "invalid site reference %q", ref)
	}
	site, err := s.q.GetSiteBySlug(ctx, ref)
	return site, fault.FromDB(err, "site "+ref)
}

// CountSites returns the total number of sites.
func (s *Service) CountSites(ctx context.Context) (int64, error) {
	return s.q.CountSites(ctx)
}

// CountLocations returns the total number of locations across all sites.
func (s *Service) CountLocations(ctx context.Context) (int64, error) {
	return s.q.CountLocations(ctx)
}

// ListSites returns sites (with tenant display fields) and the total count.
func (s *Service) ListSites(ctx context.Context, limit, offset int32) ([]gen.ListSitesRow, int64, error) {
	total, err := s.q.CountSites(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("dcim: count sites: %w", err)
	}
	items, err := s.q.ListSites(ctx, gen.ListSitesParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, fmt.Errorf("dcim: list sites: %w", err)
	}
	return items, total, nil
}

// UpdateSiteByRef applies a partial update and records before/after.
func (s *Service) UpdateSiteByRef(ctx context.Context, ref string, up SiteUpdate) (gen.Site, error) {
	cur, err := s.GetSiteByRef(ctx, ref)
	if err != nil {
		return gen.Site{}, err
	}

	next := SiteInput{
		Name: cur.Name, Slug: cur.Slug, Status: cur.Status,
		Facility: cur.Facility, TimeZone: cur.TimeZone, Description: cur.Description,
	}
	if up.Name != nil {
		next.Name = *up.Name
	}
	if up.Slug != nil {
		next.Slug = *up.Slug
	}
	if up.Status != nil {
		next.Status = *up.Status
	}
	if up.Facility != nil {
		next.Facility = *up.Facility
	}
	if up.TimeZone != nil {
		next.TimeZone = *up.TimeZone
	}
	if up.Description != nil {
		next.Description = *up.Description
	}
	next, err = normalizeSite(next)
	if err != nil {
		return gen.Site{}, err
	}

	tenantID := cur.TenantID
	if up.TenantRef != nil {
		tenantID, err = s.resolveTenantRef(ctx, *up.TenantRef)
		if err != nil {
			return gen.Site{}, err
		}
	}
	groupID := cur.SiteGroupID
	if up.GroupRef != nil {
		groupID, err = s.resolveSiteGroupRef(ctx, *up.GroupRef)
		if err != nil {
			return gen.Site{}, err
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.Site{}, fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	site, err := q.UpdateSite(ctx, gen.UpdateSiteParams{
		ID:          cur.ID,
		Slug:        next.Slug,
		Name:        next.Name,
		Status:      next.Status,
		TenantID:    tenantID,
		SiteGroupID: groupID,
		Facility:    next.Facility,
		TimeZone:    next.TimeZone,
		Description: next.Description,
	})
	if err != nil {
		return gen.Site{}, fault.FromDB(err, "site "+ref)
	}
	if err := changelog.Record(ctx, q, changelog.ActionUpdate, TypeSite.Key, site.ID, site.Name, siteSnapshot(cur), siteSnapshot(site)); err != nil {
		return gen.Site{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.Site{}, fmt.Errorf("dcim: commit: %w", err)
	}
	return site, nil
}

// DeleteSiteByRef removes a site; its location tree cascades with it.
func (s *Service) DeleteSiteByRef(ctx context.Context, ref string) error {
	cur, err := s.GetSiteByRef(ctx, ref)
	if err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	if _, err := q.DeleteSite(ctx, cur.ID); err != nil {
		return fault.FromDB(err, "site "+ref)
	}
	if err := changelog.Record(ctx, q, changelog.ActionDelete, TypeSite.Key, cur.ID, cur.Name, siteSnapshot(cur), nil); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("dcim: commit: %w", err)
	}
	return nil
}
