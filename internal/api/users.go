package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/saku75/warren/internal/auth"
	"github.com/saku75/warren/internal/db/gen"
)

// requireAdminAPI gates user management on the acting token's user.
func (h *Handler) requireAdminAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, ok := auth.UserFrom(r.Context()); !ok || !u.IsAdmin {
			h.writeJSON(w, http.StatusForbidden, map[string]any{
				"error": map[string]string{"code": "forbidden", "message": "administrator access required"},
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

type userRep struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	Username    string     `json:"username"`
	DisplayName string     `json:"display_name"`
	Email       string     `json:"email"`
	Provider    string     `json:"provider"`
	IsAdmin     bool       `json:"is_admin"`
	Disabled    bool       `json:"disabled"`
	LastLoginAt *time.Time `json:"last_login_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func userToRep(u gen.User) userRep {
	return userRep{
		ID:          u.ID.String(),
		Type:        auth.TypeUser.Key,
		Username:    u.Username,
		DisplayName: u.DisplayName,
		Email:       u.Email,
		Provider:    u.Provider,
		IsAdmin:     u.IsAdmin,
		Disabled:    u.Disabled,
		LastLoginAt: u.LastLoginAt,
		CreatedAt:   u.CreatedAt,
		UpdatedAt:   u.UpdatedAt,
	}
}

type userWrite struct {
	Username    *string `json:"username"`
	DisplayName *string `json:"display_name"`
	Email       *string `json:"email"`
	Password    *string `json:"password"`
	IsAdmin     *bool   `json:"is_admin"`
	Disabled    *bool   `json:"disabled"`
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	items, total, err := h.auth.ListUsers(r.Context(), limit, offset)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	reps := make([]userRep, len(items))
	for i, u := range items {
		reps[i] = userToRep(u)
	}
	h.writeJSON(w, http.StatusOK, listEnvelope{Items: reps, Total: total, Limit: limit, Offset: offset})
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	var in userWrite
	if err := h.decode(r, &in); err != nil {
		h.writeError(w, r, err)
		return
	}
	u, err := h.auth.CreateLocalUser(r.Context(), auth.UserInput{
		Username:    deref(in.Username),
		DisplayName: deref(in.DisplayName),
		Email:       deref(in.Email),
		Password:    deref(in.Password),
		IsAdmin:     in.IsAdmin != nil && *in.IsAdmin,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, userToRep(u))
}

func (h *Handler) getUser(w http.ResponseWriter, r *http.Request) {
	u, err := h.auth.GetUserByRef(r.Context(), chi.URLParam(r, "ref"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, userToRep(u))
}

func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request) {
	var in userWrite
	if err := h.decode(r, &in); err != nil {
		h.writeError(w, r, err)
		return
	}
	ref := chi.URLParam(r, "ref")
	u, err := h.auth.UpdateUserByRef(r.Context(), ref, auth.UserUpdate{
		Username:    in.Username,
		DisplayName: in.DisplayName,
		Email:       in.Email,
		IsAdmin:     in.IsAdmin,
		Disabled:    in.Disabled,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if in.Password != nil {
		if err := h.auth.SetPasswordByRef(r.Context(), u.Username, *in.Password); err != nil {
			h.writeError(w, r, err)
			return
		}
	}
	h.writeJSON(w, http.StatusOK, userToRep(u))
}

func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	if err := h.auth.DeleteUserByRef(r.Context(), chi.URLParam(r, "ref")); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
