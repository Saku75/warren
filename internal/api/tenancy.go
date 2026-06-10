package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/saku75/warren/internal/db/gen"
	"github.com/saku75/warren/internal/tenancy"
)

type tenantRep struct {
	ID          string      `json:"id"`
	Type        string      `json:"type"`
	Slug        string      `json:"slug"`
	Name        string      `json:"name"`
	Parent      *refSummary `json:"parent"`
	Description string      `json:"description"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

func (h *Handler) tenantToRep(r *http.Request, t gen.Tenant) tenantRep {
	rep := tenantRep{
		ID:          t.ID.String(),
		Type:        tenancy.TypeTenant.Key,
		Slug:        t.Slug,
		Name:        t.Name,
		Description: t.Description,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
	if t.ParentID != nil {
		rep.Parent = &refSummary{ID: t.ParentID.String()}
		if p, err := h.tenancy.GetByRef(r.Context(), t.ParentID.String()); err == nil {
			rep.Parent.Slug, rep.Parent.Name = p.Slug, p.Name
		}
	}
	return rep
}

type tenantWrite struct {
	Name        *string `json:"name"`
	Slug        *string `json:"slug"`
	Parent      *string `json:"parent"`
	Description *string `json:"description"`
}

func (h *Handler) listTenants(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	items, total, err := h.tenancy.List(r.Context(), limit, offset)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	reps := make([]tenantRep, len(items))
	for i, t := range items {
		rep := tenantRep{
			ID:          t.ID.String(),
			Type:        tenancy.TypeTenant.Key,
			Slug:        t.Slug,
			Name:        t.Name,
			Description: t.Description,
			CreatedAt:   t.CreatedAt,
			UpdatedAt:   t.UpdatedAt,
		}
		if t.ParentID != nil {
			rep.Parent = &refSummary{ID: t.ParentID.String(), Slug: derefOr(t.ParentSlug, ""), Name: derefOr(t.ParentName, "")}
		}
		reps[i] = rep
	}
	h.writeJSON(w, http.StatusOK, listEnvelope{Items: reps, Total: total, Limit: limit, Offset: offset})
}

func (h *Handler) createTenant(w http.ResponseWriter, r *http.Request) {
	var in tenantWrite
	if err := h.decode(r, &in); err != nil {
		h.writeError(w, r, err)
		return
	}
	t, err := h.tenancy.Create(r.Context(), tenancy.Input{
		Name:        deref(in.Name),
		Slug:        deref(in.Slug),
		ParentRef:   deref(in.Parent),
		Description: deref(in.Description),
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, h.tenantToRep(r, t))
}

func (h *Handler) getTenant(w http.ResponseWriter, r *http.Request) {
	t, err := h.tenancy.GetByRef(r.Context(), chi.URLParam(r, "ref"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, h.tenantToRep(r, t))
}

func (h *Handler) updateTenant(w http.ResponseWriter, r *http.Request) {
	var in tenantWrite
	if err := h.decode(r, &in); err != nil {
		h.writeError(w, r, err)
		return
	}
	t, err := h.tenancy.UpdateByRef(r.Context(), chi.URLParam(r, "ref"), tenancy.Update{
		Name:        in.Name,
		Slug:        in.Slug,
		ParentRef:   in.Parent,
		Description: in.Description,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, h.tenantToRep(r, t))
}

func (h *Handler) deleteTenant(w http.ResponseWriter, r *http.Request) {
	if err := h.tenancy.DeleteByRef(r.Context(), chi.URLParam(r, "ref")); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
