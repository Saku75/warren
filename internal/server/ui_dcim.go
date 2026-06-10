package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/saku75/warren/internal/core/tree"
	"github.com/saku75/warren/internal/db/gen"
	"github.com/saku75/warren/internal/dcim"
	"github.com/saku75/warren/internal/web"
)

func (u *uiHandler) siteGroupsFlat(r *http.Request) ([]tree.Flat[gen.SiteGroup], error) {
	roots, err := u.dcim.SiteGroupTree(r.Context())
	if err != nil {
		return nil, err
	}
	return tree.Flatten(roots), nil
}

func (u *uiHandler) sitesPage(w http.ResponseWriter, r *http.Request) {
	u.renderSites(w, r, http.StatusOK, flashErr(r), true)
}

// renderSites renders the sites list as a full page or as the swappable
// section.
func (u *uiHandler) renderSites(w http.ResponseWriter, r *http.Request, status int, errMsg string, fullPage bool) {
	sites, _, err := u.dcim.ListSites(r.Context(), uiListLimit, 0)
	if err == nil {
		var tenants []gen.ListTenantsRow
		tenants, _, err = u.tenancy.List(r.Context(), uiListLimit, 0)
		if err == nil {
			var groups []tree.Flat[gen.SiteGroup]
			groups, err = u.siteGroupsFlat(r)
			if err == nil {
				if fullPage {
					u.render(w, r, status, web.SitesPage(sites, tenants, groups, errMsg))
				} else {
					u.render(w, r, status, web.SitesSection(sites, tenants, groups, errMsg))
				}
				return
			}
		}
	}
	st, msg := u.fail(r, err)
	u.render(w, r, st, web.ErrorPage(msg))
}

func siteInputFromForm(r *http.Request) dcim.SiteInput {
	return dcim.SiteInput{
		Name:        formValue(r, "name"),
		Slug:        formValue(r, "slug"),
		Status:      formValue(r, "status"),
		TenantRef:   formValue(r, "tenant"),
		GroupRef:    formValue(r, "group"),
		Facility:    formValue(r, "facility"),
		TimeZone:    formValue(r, "time_zone"),
		Description: formValue(r, "description"),
	}
}

func (u *uiHandler) createSite(w http.ResponseWriter, r *http.Request) {
	_, err := u.dcim.CreateSite(r.Context(), siteInputFromForm(r))
	switch {
	case isHX(r) && err == nil:
		u.renderSites(w, r, http.StatusOK, "", false)
	case isHX(r):
		_, msg := u.fail(r, err)
		u.renderSites(w, r, http.StatusOK, msg, false)
	case err == nil:
		http.Redirect(w, r, "/dcim/sites", http.StatusSeeOther)
	default:
		status, msg := u.fail(r, err)
		u.renderSites(w, r, status, msg, true)
	}
}

// renderSiteDetail gathers everything the site page needs.
func (u *uiHandler) renderSiteDetail(w http.ResponseWriter, r *http.Request, status int, site gen.Site, errMsg string) {
	tenantSlug := ""
	if site.TenantID != nil {
		if t, err := u.tenancy.GetByRef(r.Context(), site.TenantID.String()); err == nil {
			tenantSlug = t.Slug
		}
	}
	groupPath, groupName := "", ""
	if site.SiteGroupID != nil {
		if g, err := u.dcim.GetSiteGroupByRef(r.Context(), site.SiteGroupID.String()); err == nil {
			groupName = g.Name
			groupPath, _ = u.dcim.SiteGroupPath(r.Context(), g.ID)
		}
	}
	tenants, _, err := u.tenancy.List(r.Context(), uiListLimit, 0)
	if err == nil {
		var groups []tree.Flat[gen.SiteGroup]
		groups, err = u.siteGroupsFlat(r)
		if err == nil {
			var locTree []*tree.Node[gen.Location]
			locTree, err = u.dcim.LocationTree(r.Context(), site.ID)
			if err == nil {
				u.render(w, r, status, web.SiteDetailPage(site, tenantSlug, tenants, groupPath, groupName, groups, locTree, tree.Flatten(locTree), errMsg))
				return
			}
		}
	}
	st, msg := u.fail(r, err)
	u.render(w, r, st, web.ErrorPage(msg))
}

func (u *uiHandler) siteDetail(w http.ResponseWriter, r *http.Request) {
	site, err := u.dcim.GetSiteByRef(r.Context(), chi.URLParam(r, "ref"))
	if err != nil {
		status, msg := u.fail(r, err)
		u.render(w, r, status, web.ErrorPage(msg))
		return
	}
	u.renderSiteDetail(w, r, http.StatusOK, site, flashErr(r))
}

func (u *uiHandler) updateSite(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "ref")
	in := siteInputFromForm(r)
	site, err := u.dcim.UpdateSiteByRef(r.Context(), ref, dcim.SiteUpdate{
		Name:        &in.Name,
		Slug:        &in.Slug,
		Status:      &in.Status,
		TenantRef:   &in.TenantRef,
		GroupRef:    &in.GroupRef,
		Facility:    &in.Facility,
		TimeZone:    &in.TimeZone,
		Description: &in.Description,
	})
	if err != nil {
		status, msg := u.fail(r, err)
		cur, gerr := u.dcim.GetSiteByRef(r.Context(), ref)
		if gerr != nil {
			status, msg = u.fail(r, gerr)
			u.render(w, r, status, web.ErrorPage(msg))
			return
		}
		u.renderSiteDetail(w, r, status, cur, msg)
		return
	}
	http.Redirect(w, r, "/dcim/sites/"+site.Slug, http.StatusSeeOther)
}

func (u *uiHandler) deleteSite(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "ref")
	err := u.dcim.DeleteSiteByRef(r.Context(), ref)

	if hxTarget(r) == "sites-section" {
		msg := ""
		if err != nil {
			_, msg = u.fail(r, err)
		}
		u.renderSites(w, r, http.StatusOK, msg, false)
		return
	}
	if err != nil {
		_, msg := u.fail(r, err)
		hxRedirect(w, "/dcim/sites/"+ref, msg)
		return
	}
	hxRedirect(w, "/dcim/sites", "")
}

func (u *uiHandler) createLocation(w http.ResponseWriter, r *http.Request) {
	siteRef := chi.URLParam(r, "ref")
	_, err := u.dcim.CreateLocation(r.Context(), dcim.LocationInput{
		SiteRef:     siteRef,
		ParentRef:   formValue(r, "parent"),
		Name:        formValue(r, "name"),
		Slug:        formValue(r, "slug"),
		Kind:        formValue(r, "kind"),
		Status:      formValue(r, "status"),
		TenantRef:   formValue(r, "tenant"),
		Description: formValue(r, "description"),
	})
	if err != nil {
		status, msg := u.fail(r, err)
		site, gerr := u.dcim.GetSiteByRef(r.Context(), siteRef)
		if gerr != nil {
			status, msg = u.fail(r, gerr)
			u.render(w, r, status, web.ErrorPage(msg))
			return
		}
		u.renderSiteDetail(w, r, status, site, msg)
		return
	}
	http.Redirect(w, r, "/dcim/sites/"+siteRef, http.StatusSeeOther)
}

func (u *uiHandler) locationDetail(w http.ResponseWriter, r *http.Request) {
	loc, err := u.dcim.GetLocationByRef(r.Context(), wildcardRef(r))
	if err != nil {
		status, msg := u.fail(r, err)
		u.render(w, r, status, web.ErrorPage(msg))
		return
	}
	u.renderLocationDetail(w, r, http.StatusOK, loc, flashErr(r))
}

func (u *uiHandler) renderLocationDetail(w http.ResponseWriter, r *http.Request, status int, loc gen.Location, errMsg string) {
	path, err := u.dcim.LocationPath(r.Context(), loc.ID)
	if err == nil {
		var site gen.Site
		site, err = u.dcim.GetSiteByRef(r.Context(), loc.SiteID.String())
		if err == nil {
			var children []gen.Location
			children, err = u.dcim.LocationChildren(r.Context(), loc.ID)
			if err == nil {
				u.render(w, r, status, web.LocationDetailPage(loc, path, site, children, errMsg))
				return
			}
		}
	}
	st, msg := u.fail(r, err)
	u.render(w, r, st, web.ErrorPage(msg))
}

func (u *uiHandler) updateLocation(w http.ResponseWriter, r *http.Request) {
	ref := wildcardRef(r)
	loc, err := u.dcim.UpdateLocationByRef(r.Context(), ref, dcim.LocationUpdate{
		Name:        formPtr(r, "name"),
		Slug:        formPtr(r, "slug"),
		Kind:        formPtr(r, "kind"),
		Status:      formPtr(r, "status"),
		Description: formPtr(r, "description"),
	})
	if err != nil {
		status, msg := u.fail(r, err)
		cur, gerr := u.dcim.GetLocationByRef(r.Context(), ref)
		if gerr != nil {
			status, msg = u.fail(r, gerr)
			u.render(w, r, status, web.ErrorPage(msg))
			return
		}
		u.renderLocationDetail(w, r, status, cur, msg)
		return
	}
	path, err := u.dcim.LocationPath(r.Context(), loc.ID)
	if err != nil {
		http.Redirect(w, r, "/dcim/sites", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/dcim/locations/"+path, http.StatusSeeOther)
}

func (u *uiHandler) deleteLocation(w http.ResponseWriter, r *http.Request) {
	ref := wildcardRef(r)

	// Resolve first so the site to navigate back to is known.
	loc, err := u.dcim.GetLocationByRef(r.Context(), ref)
	if err != nil {
		_, msg := u.fail(r, err)
		hxRedirect(w, "/dcim/sites", msg)
		return
	}
	site, err := u.dcim.GetSiteByRef(r.Context(), loc.SiteID.String())
	if err != nil {
		_, msg := u.fail(r, err)
		hxRedirect(w, "/dcim/sites", msg)
		return
	}

	if err := u.dcim.DeleteLocationByRef(r.Context(), ref); err != nil {
		_, msg := u.fail(r, err)
		hxRedirect(w, "/dcim/locations/"+ref, msg)
		return
	}
	hxRedirect(w, "/dcim/sites/"+site.Slug, "")
}

// --- site groups ---

func (u *uiHandler) siteGroupsPage(w http.ResponseWriter, r *http.Request) {
	u.renderSiteGroups(w, r, http.StatusOK, flashErr(r), true)
}

func (u *uiHandler) renderSiteGroups(w http.ResponseWriter, r *http.Request, status int, errMsg string, fullPage bool) {
	roots, err := u.dcim.SiteGroupTree(r.Context())
	if err != nil {
		st, msg := u.fail(r, err)
		u.render(w, r, st, web.ErrorPage(msg))
		return
	}
	flat := tree.Flatten(roots)
	if fullPage {
		u.render(w, r, status, web.SiteGroupsPage(roots, flat, errMsg))
	} else {
		u.render(w, r, status, web.SiteGroupsSection(roots, flat, errMsg))
	}
}

func (u *uiHandler) createSiteGroup(w http.ResponseWriter, r *http.Request) {
	_, err := u.dcim.CreateSiteGroup(r.Context(), dcim.SiteGroupInput{
		Name:        formValue(r, "name"),
		Slug:        formValue(r, "slug"),
		Kind:        formValue(r, "kind"),
		ParentRef:   formValue(r, "parent"),
		Description: formValue(r, "description"),
	})
	switch {
	case isHX(r) && err == nil:
		u.renderSiteGroups(w, r, http.StatusOK, "", false)
	case isHX(r):
		_, msg := u.fail(r, err)
		u.renderSiteGroups(w, r, http.StatusOK, msg, false)
	case err == nil:
		http.Redirect(w, r, "/dcim/site-groups", http.StatusSeeOther)
	default:
		status, msg := u.fail(r, err)
		u.renderSiteGroups(w, r, status, msg, true)
	}
}

func (u *uiHandler) siteGroupDetail(w http.ResponseWriter, r *http.Request) {
	g, err := u.dcim.GetSiteGroupByRef(r.Context(), wildcardRef(r))
	if err != nil {
		status, msg := u.fail(r, err)
		u.render(w, r, status, web.ErrorPage(msg))
		return
	}
	u.renderSiteGroupDetail(w, r, http.StatusOK, g, flashErr(r))
}

func (u *uiHandler) renderSiteGroupDetail(w http.ResponseWriter, r *http.Request, status int, g gen.SiteGroup, errMsg string) {
	path, err := u.dcim.SiteGroupPath(r.Context(), g.ID)
	if err == nil {
		var children []gen.SiteGroup
		children, err = u.dcim.SiteGroupChildren(r.Context(), g.ID)
		if err == nil {
			var members []gen.Site
			members, err = u.dcim.SitesInGroup(r.Context(), g.ID)
			if err == nil {
				u.render(w, r, status, web.SiteGroupDetailPage(g, path, children, members, errMsg))
				return
			}
		}
	}
	st, msg := u.fail(r, err)
	u.render(w, r, st, web.ErrorPage(msg))
}

func (u *uiHandler) updateSiteGroup(w http.ResponseWriter, r *http.Request) {
	ref := wildcardRef(r)
	g, err := u.dcim.UpdateSiteGroupByRef(r.Context(), ref, dcim.SiteGroupUpdate{
		Name:        formPtr(r, "name"),
		Slug:        formPtr(r, "slug"),
		Kind:        formPtr(r, "kind"),
		Description: formPtr(r, "description"),
	})
	if err != nil {
		status, msg := u.fail(r, err)
		cur, gerr := u.dcim.GetSiteGroupByRef(r.Context(), ref)
		if gerr != nil {
			status, msg = u.fail(r, gerr)
			u.render(w, r, status, web.ErrorPage(msg))
			return
		}
		u.renderSiteGroupDetail(w, r, status, cur, msg)
		return
	}
	path, err := u.dcim.SiteGroupPath(r.Context(), g.ID)
	if err != nil {
		http.Redirect(w, r, "/dcim/site-groups", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/dcim/site-groups/"+path, http.StatusSeeOther)
}

func (u *uiHandler) deleteSiteGroup(w http.ResponseWriter, r *http.Request) {
	ref := wildcardRef(r)
	if err := u.dcim.DeleteSiteGroupByRef(r.Context(), ref); err != nil {
		_, msg := u.fail(r, err)
		hxRedirect(w, "/dcim/site-groups/"+ref, msg)
		return
	}
	hxRedirect(w, "/dcim/site-groups", "")
}
