package server

import (
	"net/http"

	"github.com/saku75/warren/internal/web"
)

func (u *uiHandler) dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var stats web.DashboardStats
	var err error

	if stats.Sites, err = u.dcim.CountSites(ctx); err == nil {
		if stats.Locations, err = u.dcim.CountLocations(ctx); err == nil {
			stats.Tenants, err = u.tenancy.Count(ctx)
		}
	}
	if err != nil {
		status, msg := u.fail(r, err)
		u.render(w, r, status, web.ErrorPage(msg))
		return
	}

	recent, total, err := u.changelog.List(ctx, 8, 0)
	if err != nil {
		status, msg := u.fail(r, err)
		u.render(w, r, status, web.ErrorPage(msg))
		return
	}
	stats.Changes = total

	u.render(w, r, http.StatusOK, web.Dashboard(u.version, stats, recent))
}
