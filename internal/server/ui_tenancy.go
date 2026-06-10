package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/saku75/warren/internal/tenancy"
	"github.com/saku75/warren/internal/web"
)

func (u *uiHandler) tenantsPage(w http.ResponseWriter, r *http.Request) {
	items, _, err := u.tenancy.List(r.Context(), uiListLimit, 0)
	if err != nil {
		status, msg := u.fail(r, err)
		u.render(w, r, status, web.ErrorPage(msg))
		return
	}
	u.render(w, r, http.StatusOK, web.TenantsPage(items, flashErr(r)))
}

// renderTenantsSection re-renders the swappable list section.
func (u *uiHandler) renderTenantsSection(w http.ResponseWriter, r *http.Request, errMsg string) {
	items, _, err := u.tenancy.List(r.Context(), uiListLimit, 0)
	if err != nil {
		status, msg := u.fail(r, err)
		u.render(w, r, status, web.ErrorPage(msg))
		return
	}
	u.render(w, r, http.StatusOK, web.TenantsSection(items, errMsg))
}

func (u *uiHandler) createTenant(w http.ResponseWriter, r *http.Request) {
	_, err := u.tenancy.Create(r.Context(), tenancy.Input{
		Name:        formValue(r, "name"),
		Slug:        formValue(r, "slug"),
		Description: formValue(r, "description"),
	})
	switch {
	case isHX(r) && err == nil:
		u.renderTenantsSection(w, r, "")
	case isHX(r):
		_, msg := u.fail(r, err)
		u.renderTenantsSection(w, r, msg)
	case err == nil:
		http.Redirect(w, r, "/tenancy/tenants", http.StatusSeeOther)
	default:
		status, msg := u.fail(r, err)
		items, _, lerr := u.tenancy.List(r.Context(), uiListLimit, 0)
		if lerr != nil {
			status, msg = u.fail(r, lerr)
			u.render(w, r, status, web.ErrorPage(msg))
			return
		}
		u.render(w, r, status, web.TenantsPage(items, msg))
	}
}

func (u *uiHandler) tenantDetail(w http.ResponseWriter, r *http.Request) {
	t, err := u.tenancy.GetByRef(r.Context(), chi.URLParam(r, "ref"))
	if err != nil {
		status, msg := u.fail(r, err)
		u.render(w, r, status, web.ErrorPage(msg))
		return
	}
	u.render(w, r, http.StatusOK, web.TenantDetailPage(t, flashErr(r)))
}

func (u *uiHandler) updateTenant(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "ref")
	t, err := u.tenancy.UpdateByRef(r.Context(), ref, tenancy.Update{
		Name:        formPtr(r, "name"),
		Slug:        formPtr(r, "slug"),
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
		u.render(w, r, status, web.TenantDetailPage(cur, msg))
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
		u.renderTenantsSection(w, r, msg)
		return
	}
	if err != nil {
		_, msg := u.fail(r, err)
		hxRedirect(w, "/tenancy/tenants/"+ref, msg)
		return
	}
	hxRedirect(w, "/tenancy/tenants", "")
}
