package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/saku75/warren/internal/db/gen"
	"github.com/saku75/warren/internal/tenancy"
)

type tenantRep struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func tenantToRep(t gen.Tenant) tenantRep {
	return tenantRep{
		ID:          t.ID.String(),
		Type:        tenancy.TypeTenant.Key,
		Slug:        t.Slug,
		Name:        t.Name,
		Description: t.Description,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}

type tenantWrite struct {
	Name        *string `json:"name"`
	Slug        *string `json:"slug"`
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
		reps[i] = tenantToRep(t)
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
		Description: deref(in.Description),
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, tenantToRep(t))
}

func (h *Handler) getTenant(w http.ResponseWriter, r *http.Request) {
	t, err := h.tenancy.GetByRef(r.Context(), chi.URLParam(r, "ref"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, tenantToRep(t))
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
		Description: in.Description,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, tenantToRep(t))
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
