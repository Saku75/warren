package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/saku75/warren/internal/core/tree"
	"github.com/saku75/warren/internal/db/gen"
	"github.com/saku75/warren/internal/tenancy"
	"github.com/saku75/warren/internal/web"
)

func (u *uiHandler) tenantsFlat(r *http.Request) ([]tree.Flat[gen.Tenant], error) {
	roots, err := u.tenancy.Tree(r.Context())
	if err != nil {
		return nil, err
	}
	return tree.Flatten(roots), nil
}

func (u *uiHandler) tenantsPage(w http.ResponseWriter, r *http.Request) {
	u.renderTenants(w, r, http.StatusOK, flashErr(r), true)
}

// renderTenants renders the tenant tree as a full page or as the
// swappable section.
func (u *uiHandler) renderTenants(w http.ResponseWriter, r *http.Request, status int, errMsg string, fullPage bool) {
	roots, err := u.tenancy.Tree(r.Context())
	if err != nil {
		st, msg := u.fail(r, err)
		u.render(w, r, st, web.ErrorPage(msg))
		return
	}
	if fullPage {
		u.render(w, r, status, web.TenantsPage(roots, errMsg))
	} else {
		u.render(w, r, status, web.TenantsSection(roots, errMsg))
	}
}

func tenantInputFromForm(r *http.Request) tenancy.Input {
	return tenancy.Input{
		Name:        formValue(r, "name"),
		Slug:        formValue(r, "slug"),
		ParentRef:   formValue(r, "parent"),
		Description: formValue(r, "description"),
	}
}

// renderTenantForm shows the create form, optionally with an error and
// the submitted values preserved.
func (u *uiHandler) renderTenantForm(w http.ResponseWriter, r *http.Request, status int, in tenancy.Input, errMsg string) {
	parents, err := u.tenantsFlat(r)
	if err != nil {
		st, msg := u.fail(r, err)
		u.render(w, r, st, web.ErrorPage(msg))
		return
	}
	u.render(w, r, status, web.TenantFormPage(in, parents, errMsg))
}

func (u *uiHandler) tenantFormPage(w http.ResponseWriter, r *http.Request) {
	u.renderTenantForm(w, r, http.StatusOK, tenancy.Input{}, "")
}

func (u *uiHandler) createTenant(w http.ResponseWriter, r *http.Request) {
	in := tenantInputFromForm(r)
	t, err := u.tenancy.Create(r.Context(), in)
	if err != nil {
		status, msg := u.fail(r, err)
		u.renderTenantForm(w, r, status, in, msg)
		return
	}
	http.Redirect(w, r, "/tenancy/tenants/"+t.Slug, http.StatusSeeOther)
}

func (u *uiHandler) tenantDetail(w http.ResponseWriter, r *http.Request) {
	t, err := u.tenancy.GetByRef(r.Context(), chi.URLParam(r, "ref"))
	if err != nil {
		status, msg := u.fail(r, err)
		u.render(w, r, status, web.ErrorPage(msg))
		return
	}
	u.renderTenantDetail(w, r, http.StatusOK, t, flashErr(r))
}

func (u *uiHandler) renderTenantDetail(w http.ResponseWriter, r *http.Request, status int, t gen.Tenant, errMsg string) {
	parentSlug, parentName := "", ""
	if t.ParentID != nil {
		if p, err := u.tenancy.GetByRef(r.Context(), t.ParentID.String()); err == nil {
			parentSlug, parentName = p.Slug, p.Name
		}
	}
	parents, err := u.tenantsFlat(r)
	if err == nil {
		var children []gen.Tenant
		children, err = u.tenancy.Children(r.Context(), t.ID)
		if err == nil {
			u.render(w, r, status, web.TenantDetailPage(t, parentSlug, parentName, parents, children, errMsg))
			return
		}
	}
	st, msg := u.fail(r, err)
	u.render(w, r, st, web.ErrorPage(msg))
}

func (u *uiHandler) updateTenant(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "ref")
	t, err := u.tenancy.UpdateByRef(r.Context(), ref, tenancy.Update{
		Name:        formPtr(r, "name"),
		Slug:        formPtr(r, "slug"),
		ParentRef:   formPtr(r, "parent"),
		Description: formPtr(r, "description"),
	})
	if err != nil {
		status, msg := u.fail(r, err)
		cur, gerr := u.tenancy.GetByRef(r.Context(), ref)
		if gerr != nil {
			status, msg = u.fail(r, gerr)
			u.render(w, r, status, web.ErrorPage(msg))
			return
		}
		u.renderTenantDetail(w, r, status, cur, msg)
		return
	}
	http.Redirect(w, r, "/tenancy/tenants/"+t.Slug, http.StatusSeeOther)
}

func (u *uiHandler) deleteTenant(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "ref")
	err := u.tenancy.DeleteByRef(r.Context(), ref)

	// Deletes arrive from the list section (re-render it) or from a detail
	// page (navigate back to the list, or to the detail with the error).
	if hxTarget(r) == "tenants-section" {
		msg := ""
		if err != nil {
			_, msg = u.fail(r, err)
		}
		u.renderTenants(w, r, http.StatusOK, msg, false)
		return
	}
	if err != nil {
		_, msg := u.fail(r, err)
		hxRedirect(w, "/tenancy/tenants/"+ref, msg)
		return
	}
	hxRedirect(w, "/tenancy/tenants", "")
}
