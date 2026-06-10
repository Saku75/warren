package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/saku75/warren/internal/core/tree"
	"github.com/saku75/warren/internal/db/gen"
	"github.com/saku75/warren/internal/tenancy"
)

type tenantRep struct {
	ID          string      `json:"id"`
	Type        string      `json:"type"`
	Slug        string      `json:"slug"`
	Name        string      `json:"name"`
	Group       *refSummary `json:"group"`
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
	if t.TenantGroupID != nil {
		rep.Group = &refSummary{ID: t.TenantGroupID.String()}
		if g, err := h.tenancy.GetGroupByRef(r.Context(), t.TenantGroupID.String()); err == nil {
			rep.Group.Slug, rep.Group.Name = g.Slug, g.Name
		}
	}
	return rep
}

type tenantWrite struct {
	Name        *string `json:"name"`
	Slug        *string `json:"slug"`
	Group       *string `json:"group"`
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
		if t.TenantGroupID != nil {
			rep.Group = &refSummary{ID: t.TenantGroupID.String(), Slug: derefOr(t.GroupSlug, ""), Name: derefOr(t.GroupName, "")}
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
		Description: deref(in.Description),
		GroupRef:    deref(in.Group),
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
		Description: in.Description,
		GroupRef:    in.Group,
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

// --- tenant groups ---

type tenantGroupRep struct {
	ID          string      `json:"id"`
	Type        string      `json:"type"`
	Slug        string      `json:"slug"`
	SlugPath    string      `json:"slug_path"`
	Name        string      `json:"name"`
	Parent      *refSummary `json:"parent"`
	Description string      `json:"description"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

func (h *Handler) tenantGroupToRep(r *http.Request, g gen.TenantGroup) (tenantGroupRep, error) {
	path, err := h.tenancy.GroupPath(r.Context(), g.ID)
	if err != nil {
		return tenantGroupRep{}, err
	}
	rep := tenantGroupRep{
		ID:          g.ID.String(),
		Type:        tenancy.TypeTenantGroup.Key,
		Slug:        g.Slug,
		SlugPath:    path,
		Name:        g.Name,
		Description: g.Description,
		CreatedAt:   g.CreatedAt,
		UpdatedAt:   g.UpdatedAt,
	}
	if g.ParentID != nil {
		rep.Parent = &refSummary{ID: g.ParentID.String()}
		if i := strings.LastIndexByte(path, '/'); i > 0 {
			rep.Parent.Slug = path[:i]
		}
	}
	return rep, nil
}

type tenantGroupWrite struct {
	Name        *string `json:"name"`
	Slug        *string `json:"slug"`
	Parent      *string `json:"parent"`
	Description *string `json:"description"`
}

// groupTreeRep is the nested tree shape shared by both group list
// endpoints.
type groupTreeRep struct {
	ID       string         `json:"id"`
	Slug     string         `json:"slug"`
	SlugPath string         `json:"slug_path"`
	Name     string         `json:"name"`
	Kind     string         `json:"kind,omitempty"`
	Children []groupTreeRep `json:"children"`
}

func tenantGroupTreeToRep(nodes []*tree.Node[gen.TenantGroup]) []groupTreeRep {
	out := make([]groupTreeRep, len(nodes))
	for i, n := range nodes {
		out[i] = groupTreeRep{
			ID:       n.Item.ID.String(),
			Slug:     n.Item.Slug,
			SlugPath: n.Path,
			Name:     n.Item.Name,
			Children: tenantGroupTreeToRep(n.Children),
		}
	}
	return out
}

func (h *Handler) listTenantGroups(w http.ResponseWriter, r *http.Request) {
	roots, err := h.tenancy.GroupTree(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"items": tenantGroupTreeToRep(roots)})
}

func (h *Handler) createTenantGroup(w http.ResponseWriter, r *http.Request) {
	var in tenantGroupWrite
	if err := h.decode(r, &in); err != nil {
		h.writeError(w, r, err)
		return
	}
	g, err := h.tenancy.CreateGroup(r.Context(), tenancy.GroupInput{
		Name:        deref(in.Name),
		Slug:        deref(in.Slug),
		ParentRef:   deref(in.Parent),
		Description: deref(in.Description),
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	rep, err := h.tenantGroupToRep(r, g)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, rep)
}

func (h *Handler) getTenantGroup(w http.ResponseWriter, r *http.Request) {
	g, err := h.tenancy.GetGroupByRef(r.Context(), wildcard(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	rep, err := h.tenantGroupToRep(r, g)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, rep)
}

func (h *Handler) updateTenantGroup(w http.ResponseWriter, r *http.Request) {
	var in tenantGroupWrite
	if err := h.decode(r, &in); err != nil {
		h.writeError(w, r, err)
		return
	}
	g, err := h.tenancy.UpdateGroupByRef(r.Context(), wildcard(r), tenancy.GroupUpdate{
		Name:        in.Name,
		Slug:        in.Slug,
		ParentRef:   in.Parent,
		Description: in.Description,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	rep, err := h.tenantGroupToRep(r, g)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, rep)
}

func (h *Handler) deleteTenantGroup(w http.ResponseWriter, r *http.Request) {
	if err := h.tenancy.DeleteGroupByRef(r.Context(), wildcard(r)); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
