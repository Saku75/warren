package server

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/saku75/warren/internal/changelog"
	"github.com/saku75/warren/internal/core/fault"
	"github.com/saku75/warren/internal/dcim"
	"github.com/saku75/warren/internal/tenancy"
)

// uiHandler serves the HTMX UI. Pages are server-rendered templ
// components; mutating forms post back and either swap a section (htmx)
// or redirect (plain form fallback).
//
// Note on error responses to htmx requests: htmx does not swap non-2xx
// responses by default, so section re-renders that carry a validation or
// conflict banner are sent as 200 — the "error" is application state, the
// HTTP exchange succeeded. Full-page renders keep real status codes.
type uiHandler struct {
	log       *slog.Logger
	tenancy   *tenancy.Service
	dcim      *dcim.Service
	changelog *changelog.Service
}

// uiListLimit caps UI lists until list pagination ships.
const uiListLimit = 200

func (u *uiHandler) routes(r chi.Router) {
	r.Get("/tenancy/tenants", u.tenantsPage)
	r.Post("/tenancy/tenants", u.createTenant)
	r.Get("/tenancy/tenants/{ref}", u.tenantDetail)
	r.Post("/tenancy/tenants/{ref}", u.updateTenant)
	r.Delete("/tenancy/tenants/{ref}", u.deleteTenant)

	r.Get("/dcim/sites", u.sitesPage)
	r.Post("/dcim/sites", u.createSite)
	r.Get("/dcim/sites/{ref}", u.siteDetail)
	r.Post("/dcim/sites/{ref}", u.updateSite)
	r.Delete("/dcim/sites/{ref}", u.deleteSite)
	r.Post("/dcim/sites/{ref}/locations", u.createLocation)

	r.Get("/dcim/locations/*", u.locationDetail)
	r.Post("/dcim/locations/*", u.updateLocation)
	r.Delete("/dcim/locations/*", u.deleteLocation)

	r.Get("/changelog", u.changelogPage)
}

func (u *uiHandler) render(w http.ResponseWriter, r *http.Request, status int, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := c.Render(r.Context(), w); err != nil {
		u.log.Error("ui: render", "path", r.URL.Path, "err", err)
	}
}

// fail logs internal errors and returns the user-facing message and status
// for any service error.
func (u *uiHandler) fail(r *http.Request, err error) (int, string) {
	if fault.KindOf(err) == fault.Internal {
		u.log.Error("ui: internal error", "method", r.Method, "path", r.URL.Path, "err", err)
	}
	return fault.HTTPStatus(fault.KindOf(err)), fault.Message(err)
}

func isHX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// hxTarget reports the id of the element an htmx request will swap.
func hxTarget(r *http.Request) string {
	return r.Header.Get("HX-Target")
}

// hxRedirect tells htmx to do a client-side navigation, optionally
// carrying a flash error in the query string.
func hxRedirect(w http.ResponseWriter, to string, errMsg string) {
	if errMsg != "" {
		to += "?err=" + url.QueryEscape(errMsg)
	}
	w.Header().Set("HX-Redirect", to)
	w.WriteHeader(http.StatusOK)
}

// flashErr reads a flash error from the query string (set by hxRedirect).
func flashErr(r *http.Request) string {
	msg := r.URL.Query().Get("err")
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return msg
}

func formValue(r *http.Request, name string) string {
	return strings.TrimSpace(r.PostFormValue(name))
}

func formPtr(r *http.Request, name string) *string {
	v := formValue(r, name)
	return &v
}

// wildcardRef pulls the trailing path reference from a wildcard route.
func wildcardRef(r *http.Request) string {
	return strings.Trim(chi.URLParam(r, "*"), "/")
}
