// Package api implements Warren's REST API (v1). The API is a peer of the
// UI: both sit on the same service layer, and everything the UI can do the
// API can do.
//
// Reference segments ({ref}) accept an object ID (UUID) or the type's
// slug form — a bare slug for globally scoped types, a slug path for
// parent-scoped ones (design doc §4.5).
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/saku75/warren/internal/changelog"
	"github.com/saku75/warren/internal/core/fault"
	"github.com/saku75/warren/internal/dcim"
	"github.com/saku75/warren/internal/tenancy"
)

// Handler serves the v1 API.
type Handler struct {
	log       *slog.Logger
	tenancy   *tenancy.Service
	dcim      *dcim.Service
	changelog *changelog.Service
}

func New(log *slog.Logger, t *tenancy.Service, d *dcim.Service, c *changelog.Service) *Handler {
	return &Handler{log: log, tenancy: t, dcim: d, changelog: c}
}

// Routes returns the router for mounting at /api/v1.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Route("/tenancy/tenants", func(r chi.Router) {
		r.Get("/", h.listTenants)
		r.Post("/", h.createTenant)
		r.Get("/{ref}", h.getTenant)
		r.Patch("/{ref}", h.updateTenant)
		r.Delete("/{ref}", h.deleteTenant)
	})

	// Site group refs are slug paths ("emea/colo"), so these use wildcards.
	r.Route("/dcim/site-groups", func(r chi.Router) {
		r.Get("/", h.listSiteGroups)
		r.Post("/", h.createSiteGroup)
		r.Get("/*", h.getSiteGroup)
		r.Patch("/*", h.updateSiteGroup)
		r.Delete("/*", h.deleteSiteGroup)
	})

	r.Route("/dcim/sites", func(r chi.Router) {
		r.Get("/", h.listSites)
		r.Post("/", h.createSite)
		r.Get("/{ref}", h.getSite)
		r.Patch("/{ref}", h.updateSite)
		r.Delete("/{ref}", h.deleteSite)
		r.Get("/{ref}/locations", h.listSiteLocations)
	})

	r.Route("/dcim/locations", func(r chi.Router) {
		r.Post("/", h.createLocation)
		// Location refs are paths ("site/loc/…"), so these use a wildcard.
		r.Get("/*", h.getLocation)
		r.Patch("/*", h.updateLocation)
		r.Delete("/*", h.deleteLocation)
	})

	r.Get("/changelog", h.listChangelog)

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		h.writeError(w, r, fault.New(fault.NotFound, "no such endpoint"))
	})
	return r
}

// listEnvelope is the uniform shape of every list response.
type listEnvelope struct {
	Items  any   `json:"items"`
	Total  int64 `json:"total"`
	Limit  int32 `json:"limit"`
	Offset int32 `json:"offset"`
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		h.log.Error("api: encode response", "err", err)
	}
}

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var body errorBody
	status := http.StatusInternalServerError
	switch fault.KindOf(err) {
	case fault.NotFound:
		status, body.Error.Code = http.StatusNotFound, "not_found"
	case fault.Conflict:
		status, body.Error.Code = http.StatusConflict, "conflict"
	case fault.Invalid:
		status, body.Error.Code = http.StatusBadRequest, "invalid"
	default:
		body.Error.Code = "internal"
		h.log.Error("api: internal error", "method", r.Method, "path", r.URL.Path, "err", err)
		body.Error.Message = "internal error"
		h.writeJSON(w, status, body)
		return
	}
	var fe *fault.Error
	if errors.As(err, &fe) {
		body.Error.Message = fe.Message
	} else {
		body.Error.Message = err.Error()
	}
	h.writeJSON(w, status, body)
}

// decode parses a JSON request body into v.
func (h *Handler) decode(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fault.Wrap(fault.Invalid, err, "malformed JSON body")
	}
	return nil
}

// wildcard pulls the trailing path reference from a wildcard route
// (a UUID or a slug path).
func wildcard(r *http.Request) string {
	return strings.Trim(chi.URLParam(r, "*"), "/")
}

// pagination reads limit/offset with sane bounds.
func pagination(r *http.Request) (limit, offset int32) {
	limit, offset = 50, 0
	if v, err := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 32); err == nil && v > 0 && v <= 200 {
		limit = int32(v)
	}
	if v, err := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 32); err == nil && v >= 0 {
		offset = int32(v)
	}
	return limit, offset
}
