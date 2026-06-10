package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/saku75/warren/internal/db/gen"
	"github.com/saku75/warren/internal/dcim"
	"github.com/saku75/warren/internal/web"
)

func (u *uiHandler) sitesPage(w http.ResponseWriter, r *http.Request) {
	sites, tenants, err := u.sitesAndTenants(r)
	if err != nil {
		status, msg := u.fail(r, err)
		u.render(w, r, status, web.ErrorPage(msg))
		return
	}
	u.render(w, r, http.StatusOK, web.SitesPage(sites, tenants, flashErr(r)))
}

func (u *uiHandler) sitesAndTenants(r *http.Request) ([]gen.ListSitesRow, []gen.Tenant, error) {
	sites, _, err := u.dcim.ListSites(r.Context(), uiListLimit, 0)
	if err != nil {
		return nil, nil, err
	}
	tenants, _, err := u.tenancy.List(r.Context(), uiListLimit, 0)
	if err != nil {
		return nil, nil, err
	}
	return sites, tenants, nil
}

func (u *uiHandler) renderSitesSection(w http.ResponseWriter, r *http.Request, errMsg string) {
	sites, tenants, err := u.sitesAndTenants(r)
	if err != nil {
		status, msg := u.fail(r, err)
		u.render(w, r, status, web.ErrorPage(msg))
		return
	}
	u.render(w, r, http.StatusOK, web.SitesSection(sites, tenants, errMsg))
}

func siteInputFromForm(r *http.Request) dcim.SiteInput {
	return dcim.SiteInput{
		Name:        formValue(r, "name"),
		Slug:        formValue(r, "slug"),
		Status:      formValue(r, "status"),
		TenantRef:   formValue(r, "tenant"),
		Facility:    formValue(r, "facility"),
		TimeZone:    formValue(r, "time_zone"),
		Description: formValue(r, "description"),
	}
}

func (u *uiHandler) createSite(w http.ResponseWriter, r *http.Request) {
	_, err := u.dcim.CreateSite(r.Context(), siteInputFromForm(r))
	switch {
	case isHX(r) && err == nil:
		u.renderSitesSection(w, r, "")
	case isHX(r):
		_, msg := u.fail(r, err)
		u.renderSitesSection(w, r, msg)
	case err == nil:
		http.Redirect(w, r, "/dcim/sites", http.StatusSeeOther)
	default:
		status, msg := u.fail(r, err)
		sites, tenants, lerr := u.sitesAndTenants(r)
		if lerr != nil {
			status, msg = u.fail(r, lerr)
			u.render(w, r, status, web.ErrorPage(msg))
			return
		}
		u.render(w, r, status, web.SitesPage(sites, tenants, msg))
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
	tenants, _, err := u.tenancy.List(r.Context(), uiListLimit, 0)
	if err != nil {
		st, msg := u.fail(r, err)
		u.render(w, r, st, web.ErrorPage(msg))
		return
	}
	tree, err := u.dcim.LocationTree(r.Context(), site.ID)
	if err != nil {
		st, msg := u.fail(r, err)
		u.render(w, r, st, web.ErrorPage(msg))
		return
	}
	u.render(w, r, status, web.SiteDetailPage(site, tenantSlug, tenants, tree, dcim.FlattenTree(tree), errMsg))
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
		u.renderSitesSection(w, r, msg)
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
	if err != nil {
		st, msg := u.fail(r, err)
		u.render(w, r, st, web.ErrorPage(msg))
		return
	}
	site, err := u.dcim.GetSiteByRef(r.Context(), loc.SiteID.String())
	if err != nil {
		st, msg := u.fail(r, err)
		u.render(w, r, st, web.ErrorPage(msg))
		return
	}
	children, err := u.dcim.LocationChildren(r.Context(), loc.ID)
	if err != nil {
		st, msg := u.fail(r, err)
		u.render(w, r, st, web.ErrorPage(msg))
		return
	}
	u.render(w, r, status, web.LocationDetailPage(loc, path, site, children, errMsg))
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
