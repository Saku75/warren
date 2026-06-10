package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/saku75/warren/internal/core/fault"
	"github.com/saku75/warren/internal/db/gen"
	"github.com/saku75/warren/internal/dcim"
)

// refSummary is the uniform embedded representation of a referenced object.
type refSummary struct {
	ID   string `json:"id"`
	Slug string `json:"slug,omitempty"`
	Name string `json:"name,omitempty"`
}

type siteRep struct {
	ID          string      `json:"id"`
	Type        string      `json:"type"`
	Slug        string      `json:"slug"`
	Name        string      `json:"name"`
	Status      string      `json:"status"`
	Tenant      *refSummary `json:"tenant"`
	Facility    string      `json:"facility"`
	TimeZone    string      `json:"time_zone"`
	Description string      `json:"description"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

func (h *Handler) siteToRep(r *http.Request, s gen.Site) siteRep {
	rep := siteRep{
		ID:          s.ID.String(),
		Type:        dcim.TypeSite.Key,
		Slug:        s.Slug,
		Name:        s.Name,
		Status:      s.Status,
		Facility:    s.Facility,
		TimeZone:    s.TimeZone,
		Description: s.Description,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
	}
	if s.TenantID != nil {
		t, err := h.tenancy.GetByRef(r.Context(), s.TenantID.String())
		if err == nil {
			rep.Tenant = &refSummary{ID: t.ID.String(), Slug: t.Slug, Name: t.Name}
		} else {
			rep.Tenant = &refSummary{ID: s.TenantID.String()}
		}
	}
	return rep
}

type siteWrite struct {
	Name        *string `json:"name"`
	Slug        *string `json:"slug"`
	Status      *string `json:"status"`
	Tenant      *string `json:"tenant"`
	Facility    *string `json:"facility"`
	TimeZone    *string `json:"time_zone"`
	Description *string `json:"description"`
}

func (h *Handler) listSites(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	items, total, err := h.dcim.ListSites(r.Context(), limit, offset)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	reps := make([]siteRep, len(items))
	for i, row := range items {
		rep := siteRep{
			ID:          row.ID.String(),
			Type:        dcim.TypeSite.Key,
			Slug:        row.Slug,
			Name:        row.Name,
			Status:      row.Status,
			Facility:    row.Facility,
			TimeZone:    row.TimeZone,
			Description: row.Description,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
		}
		if row.TenantID != nil {
			rep.Tenant = &refSummary{ID: row.TenantID.String(), Slug: derefOr(row.TenantSlug, ""), Name: derefOr(row.TenantName, "")}
		}
		reps[i] = rep
	}
	h.writeJSON(w, http.StatusOK, listEnvelope{Items: reps, Total: total, Limit: limit, Offset: offset})
}

func (h *Handler) createSite(w http.ResponseWriter, r *http.Request) {
	var in siteWrite
	if err := h.decode(r, &in); err != nil {
		h.writeError(w, r, err)
		return
	}
	s, err := h.dcim.CreateSite(r.Context(), dcim.SiteInput{
		Name:        deref(in.Name),
		Slug:        deref(in.Slug),
		Status:      deref(in.Status),
		TenantRef:   deref(in.Tenant),
		Facility:    deref(in.Facility),
		TimeZone:    deref(in.TimeZone),
		Description: deref(in.Description),
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, h.siteToRep(r, s))
}

func (h *Handler) getSite(w http.ResponseWriter, r *http.Request) {
	s, err := h.dcim.GetSiteByRef(r.Context(), chi.URLParam(r, "ref"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, h.siteToRep(r, s))
}

func (h *Handler) updateSite(w http.ResponseWriter, r *http.Request) {
	var in siteWrite
	if err := h.decode(r, &in); err != nil {
		h.writeError(w, r, err)
		return
	}
	s, err := h.dcim.UpdateSiteByRef(r.Context(), chi.URLParam(r, "ref"), dcim.SiteUpdate{
		Name:        in.Name,
		Slug:        in.Slug,
		Status:      in.Status,
		TenantRef:   in.Tenant,
		Facility:    in.Facility,
		TimeZone:    in.TimeZone,
		Description: in.Description,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, h.siteToRep(r, s))
}

func (h *Handler) deleteSite(w http.ResponseWriter, r *http.Request) {
	if err := h.dcim.DeleteSiteByRef(r.Context(), chi.URLParam(r, "ref")); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type locationRep struct {
	ID          string      `json:"id"`
	Type        string      `json:"type"`
	Slug        string      `json:"slug"`
	SlugPath    string      `json:"slug_path"`
	Name        string      `json:"name"`
	Kind        string      `json:"kind"`
	Status      string      `json:"status"`
	Site        refSummary  `json:"site"`
	Parent      *refSummary `json:"parent"`
	Tenant      *refSummary `json:"tenant"`
	Description string      `json:"description"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

func (h *Handler) locationToRep(r *http.Request, l gen.Location) (locationRep, error) {
	path, err := h.dcim.LocationPath(r.Context(), l.ID)
	if err != nil {
		return locationRep{}, err
	}
	rep := locationRep{
		ID:          l.ID.String(),
		Type:        dcim.TypeLocation.Key,
		Slug:        l.Slug,
		SlugPath:    path,
		Name:        l.Name,
		Kind:        l.Kind,
		Status:      l.Status,
		Description: l.Description,
		CreatedAt:   l.CreatedAt,
		UpdatedAt:   l.UpdatedAt,
	}
	site, err := h.dcim.GetSiteByRef(r.Context(), l.SiteID.String())
	if err == nil {
		rep.Site = refSummary{ID: site.ID.String(), Slug: site.Slug, Name: site.Name}
	} else {
		rep.Site = refSummary{ID: l.SiteID.String()}
	}
	if l.ParentID != nil {
		rep.Parent = &refSummary{ID: l.ParentID.String()}
		// The parent's path is this location's path minus its last segment.
		if i := strings.LastIndexByte(path, '/'); i > 0 {
			rep.Parent.Slug = path[:i]
		}
	}
	if l.TenantID != nil {
		t, err := h.tenancy.GetByRef(r.Context(), l.TenantID.String())
		if err == nil {
			rep.Tenant = &refSummary{ID: t.ID.String(), Slug: t.Slug, Name: t.Name}
		} else {
			rep.Tenant = &refSummary{ID: l.TenantID.String()}
		}
	}
	return rep, nil
}

type locationWrite struct {
	Site        *string `json:"site"`
	Parent      *string `json:"parent"`
	Name        *string `json:"name"`
	Slug        *string `json:"slug"`
	Kind        *string `json:"kind"`
	Status      *string `json:"status"`
	Tenant      *string `json:"tenant"`
	Description *string `json:"description"`
}

func (h *Handler) createLocation(w http.ResponseWriter, r *http.Request) {
	var in locationWrite
	if err := h.decode(r, &in); err != nil {
		h.writeError(w, r, err)
		return
	}
	l, err := h.dcim.CreateLocation(r.Context(), dcim.LocationInput{
		SiteRef:     deref(in.Site),
		ParentRef:   deref(in.Parent),
		Name:        deref(in.Name),
		Slug:        deref(in.Slug),
		Kind:        deref(in.Kind),
		Status:      deref(in.Status),
		TenantRef:   deref(in.Tenant),
		Description: deref(in.Description),
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	rep, err := h.locationToRep(r, l)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, rep)
}

// locationRef pulls the wildcard remainder ("site/loc/…" or a UUID).
func locationRef(r *http.Request) string {
	return strings.Trim(chi.URLParam(r, "*"), "/")
}

func (h *Handler) getLocation(w http.ResponseWriter, r *http.Request) {
	l, err := h.dcim.GetLocationByRef(r.Context(), locationRef(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	rep, err := h.locationToRep(r, l)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, rep)
}

func (h *Handler) updateLocation(w http.ResponseWriter, r *http.Request) {
	var in locationWrite
	if err := h.decode(r, &in); err != nil {
		h.writeError(w, r, err)
		return
	}
	if in.Site != nil {
		h.writeError(w, r, fault.New(fault.Invalid, "a location cannot move between sites"))
		return
	}
	l, err := h.dcim.UpdateLocationByRef(r.Context(), locationRef(r), dcim.LocationUpdate{
		ParentRef:   in.Parent,
		Name:        in.Name,
		Slug:        in.Slug,
		Kind:        in.Kind,
		Status:      in.Status,
		TenantRef:   in.Tenant,
		Description: in.Description,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	rep, err := h.locationToRep(r, l)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, rep)
}

func (h *Handler) deleteLocation(w http.ResponseWriter, r *http.Request) {
	if err := h.dcim.DeleteLocationByRef(r.Context(), locationRef(r)); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// locationTreeRep mirrors dcim.LocationNode for JSON.
type locationTreeRep struct {
	ID       string            `json:"id"`
	Slug     string            `json:"slug"`
	SlugPath string            `json:"slug_path"`
	Name     string            `json:"name"`
	Kind     string            `json:"kind"`
	Status   string            `json:"status"`
	Children []locationTreeRep `json:"children"`
}

func treeToRep(nodes []*dcim.LocationNode) []locationTreeRep {
	out := make([]locationTreeRep, len(nodes))
	for i, n := range nodes {
		out[i] = locationTreeRep{
			ID:       n.Location.ID.String(),
			Slug:     n.Location.Slug,
			SlugPath: n.Path,
			Name:     n.Location.Name,
			Kind:     n.Location.Kind,
			Status:   n.Location.Status,
			Children: treeToRep(n.Children),
		}
	}
	return out
}

func (h *Handler) listSiteLocations(w http.ResponseWriter, r *http.Request) {
	site, err := h.dcim.GetSiteByRef(r.Context(), chi.URLParam(r, "ref"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	tree, err := h.dcim.LocationTree(r.Context(), site.ID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"items": treeToRep(tree)})
}

func derefOr(s *string, fallback string) string {
	if s == nil {
		return fallback
	}
	return *s
}
