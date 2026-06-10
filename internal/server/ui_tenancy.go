package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/saku75/warren/internal/core/tree"
	"github.com/saku75/warren/internal/db/gen"
	"github.com/saku75/warren/internal/tenancy"
	"github.com/saku75/warren/internal/web"
)

func (u *uiHandler) tenantGroupsFlat(r *http.Request) ([]tree.Flat[gen.TenantGroup], error) {
	roots, err := u.tenancy.GroupTree(r.Context())
	if err != nil {
		return nil, err
	}
	return tree.Flatten(roots), nil
}

func (u *uiHandler) tenantsPage(w http.ResponseWriter, r *http.Request) {
	u.renderTenants(w, r, http.StatusOK, flashErr(r), true)
}

// renderTenants renders the tenants list as a full page or as the
// swappable section.
func (u *uiHandler) renderTenants(w http.ResponseWriter, r *http.Request, status int, errMsg string, fullPage bool) {
	items, _, err := u.tenancy.List(r.Context(), uiListLimit, 0)
	if err == nil {
		var groups []tree.Flat[gen.TenantGroup]
		groups, err = u.tenantGroupsFlat(r)
		if err == nil {
			if fullPage {
				u.render(w, r, status, web.TenantsPage(items, groups, errMsg))
			} else {
				u.render(w, r, status, web.TenantsSection(items, groups, errMsg))
			}
			return
		}
	}
	st, msg := u.fail(r, err)
	u.render(w, r, st, web.ErrorPage(msg))
}

func (u *uiHandler) createTenant(w http.ResponseWriter, r *http.Request) {
	_, err := u.tenancy.Create(r.Context(), tenancy.Input{
		Name:        formValue(r, "name"),
		Slug:        formValue(r, "slug"),
		Description: formValue(r, "description"),
		GroupRef:    formValue(r, "group"),
	})
	switch {
	case isHX(r) && err == nil:
		u.renderTenants(w, r, http.StatusOK, "", false)
	case isHX(r):
		_, msg := u.fail(r, err)
		u.renderTenants(w, r, http.StatusOK, msg, false)
	case err == nil:
		http.Redirect(w, r, "/tenancy/tenants", http.StatusSeeOther)
	default:
		status, msg := u.fail(r, err)
		u.renderTenants(w, r, status, msg, true)
	}
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
	groupPath, groupName := "", ""
	if t.TenantGroupID != nil {
		if g, err := u.tenancy.GetGroupByRef(r.Context(), t.TenantGroupID.String()); err == nil {
			groupName = g.Name
			groupPath, _ = u.tenancy.GroupPath(r.Context(), g.ID)
		}
	}
	groups, err := u.tenantGroupsFlat(r)
	if err != nil {
		st, msg := u.fail(r, err)
		u.render(w, r, st, web.ErrorPage(msg))
		return
	}
	u.render(w, r, status, web.TenantDetailPage(t, groupPath, groupName, groups, errMsg))
}

func (u *uiHandler) updateTenant(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "ref")
	t, err := u.tenancy.UpdateByRef(r.Context(), ref, tenancy.Update{
		Name:        formPtr(r, "name"),
		Slug:        formPtr(r, "slug"),
		Description: formPtr(r, "description"),
		GroupRef:    formPtr(r, "group"),
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

// --- tenant groups ---

func (u *uiHandler) tenantGroupsPage(w http.ResponseWriter, r *http.Request) {
	u.renderTenantGroups(w, r, http.StatusOK, flashErr(r), true)
}

func (u *uiHandler) renderTenantGroups(w http.ResponseWriter, r *http.Request, status int, errMsg string, fullPage bool) {
	roots, err := u.tenancy.GroupTree(r.Context())
	if err != nil {
		st, msg := u.fail(r, err)
		u.render(w, r, st, web.ErrorPage(msg))
		return
	}
	flat := tree.Flatten(roots)
	if fullPage {
		u.render(w, r, status, web.TenantGroupsPage(roots, flat, errMsg))
	} else {
		u.render(w, r, status, web.TenantGroupsSection(roots, flat, errMsg))
	}
}

func (u *uiHandler) createTenantGroup(w http.ResponseWriter, r *http.Request) {
	_, err := u.tenancy.CreateGroup(r.Context(), tenancy.GroupInput{
		Name:        formValue(r, "name"),
		Slug:        formValue(r, "slug"),
		ParentRef:   formValue(r, "parent"),
		Description: formValue(r, "description"),
	})
	switch {
	case isHX(r) && err == nil:
		u.renderTenantGroups(w, r, http.StatusOK, "", false)
	case isHX(r):
		_, msg := u.fail(r, err)
		u.renderTenantGroups(w, r, http.StatusOK, msg, false)
	case err == nil:
		http.Redirect(w, r, "/tenancy/tenant-groups", http.StatusSeeOther)
	default:
		status, msg := u.fail(r, err)
		u.renderTenantGroups(w, r, status, msg, true)
	}
}

func (u *uiHandler) tenantGroupDetail(w http.ResponseWriter, r *http.Request) {
	g, err := u.tenancy.GetGroupByRef(r.Context(), wildcardRef(r))
	if err != nil {
		status, msg := u.fail(r, err)
		u.render(w, r, status, web.ErrorPage(msg))
		return
	}
	u.renderTenantGroupDetail(w, r, http.StatusOK, g, flashErr(r))
}

func (u *uiHandler) renderTenantGroupDetail(w http.ResponseWriter, r *http.Request, status int, g gen.TenantGroup, errMsg string) {
	path, err := u.tenancy.GroupPath(r.Context(), g.ID)
	if err == nil {
		var children []gen.TenantGroup
		children, err = u.tenancy.GroupChildren(r.Context(), g.ID)
		if err == nil {
			var members []gen.Tenant
			members, err = u.tenancy.TenantsInGroup(r.Context(), g.ID)
			if err == nil {
				u.render(w, r, status, web.TenantGroupDetailPage(g, path, children, members, errMsg))
				return
			}
		}
	}
	st, msg := u.fail(r, err)
	u.render(w, r, st, web.ErrorPage(msg))
}

func (u *uiHandler) updateTenantGroup(w http.ResponseWriter, r *http.Request) {
	ref := wildcardRef(r)
	g, err := u.tenancy.UpdateGroupByRef(r.Context(), ref, tenancy.GroupUpdate{
		Name:        formPtr(r, "name"),
		Slug:        formPtr(r, "slug"),
		Description: formPtr(r, "description"),
	})
	if err != nil {
		status, msg := u.fail(r, err)
		cur, gerr := u.tenancy.GetGroupByRef(r.Context(), ref)
		if gerr != nil {
			status, msg = u.fail(r, gerr)
			u.render(w, r, status, web.ErrorPage(msg))
			return
		}
		u.renderTenantGroupDetail(w, r, status, cur, msg)
		return
	}
	path, err := u.tenancy.GroupPath(r.Context(), g.ID)
	if err != nil {
		http.Redirect(w, r, "/tenancy/tenant-groups", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/tenancy/tenant-groups/"+path, http.StatusSeeOther)
}

func (u *uiHandler) deleteTenantGroup(w http.ResponseWriter, r *http.Request) {
	ref := wildcardRef(r)
	if err := u.tenancy.DeleteGroupByRef(r.Context(), ref); err != nil {
		_, msg := u.fail(r, err)
		hxRedirect(w, "/tenancy/tenant-groups/"+ref, msg)
		return
	}
	hxRedirect(w, "/tenancy/tenant-groups", "")
}
