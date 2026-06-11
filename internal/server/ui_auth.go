package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/saku75/warren/internal/auth"
	"github.com/saku75/warren/internal/core/fault"
	"github.com/saku75/warren/internal/core/id"
	"github.com/saku75/warren/internal/web"
)

// userRoutes are admin-only (mounted behind requireAdminUI).
func (u *uiHandler) userRoutes(r chi.Router) {
	r.Get("/system/users", u.usersPage)
	r.Post("/system/users", u.createUser)
	r.Get("/system/users/new", u.userFormPage)
	r.Get("/system/users/{ref}", u.userDetail)
	r.Post("/system/users/{ref}", u.updateUser)
	r.Post("/system/users/{ref}/password", u.setUserPassword)
	r.Delete("/system/users/{ref}", u.deleteUser)
}

// profileRoutes serve the signed-in user.
func (u *uiHandler) profileRoutes(r chi.Router) {
	r.Get("/profile/tokens", u.tokensPage)
	r.Post("/profile/tokens", u.createToken)
	r.Delete("/profile/tokens/{id}", u.deleteToken)
}

func (u *uiHandler) renderUsers(w http.ResponseWriter, r *http.Request, status int, errMsg string, fullPage bool) {
	items, _, err := u.auth.ListUsers(r.Context(), uiListLimit, 0)
	if err != nil {
		st, msg := u.fail(r, err)
		u.render(w, r, st, web.ErrorPage(msg))
		return
	}
	if fullPage {
		u.render(w, r, status, web.UsersPage(items, errMsg))
	} else {
		u.render(w, r, status, web.UsersSection(items, errMsg))
	}
}

func (u *uiHandler) usersPage(w http.ResponseWriter, r *http.Request) {
	u.renderUsers(w, r, http.StatusOK, flashErr(r), true)
}

func userInputFromForm(r *http.Request) auth.UserInput {
	return auth.UserInput{
		Username:    formValue(r, "username"),
		DisplayName: formValue(r, "display_name"),
		Email:       formValue(r, "email"),
		Password:    r.PostFormValue("password"),
		IsAdmin:     formValue(r, "is_admin") == "true",
	}
}

func (u *uiHandler) userFormPage(w http.ResponseWriter, r *http.Request) {
	u.render(w, r, http.StatusOK, web.UserFormPage(auth.UserInput{}, ""))
}

func (u *uiHandler) createUser(w http.ResponseWriter, r *http.Request) {
	in := userInputFromForm(r)
	usr, err := u.auth.CreateLocalUser(r.Context(), in)
	if err != nil {
		status, msg := u.fail(r, err)
		in.Password = ""
		u.render(w, r, status, web.UserFormPage(in, msg))
		return
	}
	http.Redirect(w, r, "/system/users/"+usr.Username, http.StatusSeeOther)
}

func (u *uiHandler) userDetail(w http.ResponseWriter, r *http.Request) {
	usr, err := u.auth.GetUserByRef(r.Context(), chi.URLParam(r, "ref"))
	if err != nil {
		status, msg := u.fail(r, err)
		u.render(w, r, status, web.ErrorPage(msg))
		return
	}
	u.render(w, r, http.StatusOK, web.UserDetailPage(usr, flashErr(r)))
}

func (u *uiHandler) updateUser(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "ref")
	isAdmin := formValue(r, "is_admin") == "true"
	disabled := formValue(r, "disabled") == "true"
	usr, err := u.auth.UpdateUserByRef(r.Context(), ref, auth.UserUpdate{
		Username:    formPtr(r, "username"),
		DisplayName: formPtr(r, "display_name"),
		Email:       formPtr(r, "email"),
		IsAdmin:     &isAdmin,
		Disabled:    &disabled,
	})
	if err != nil {
		status, msg := u.fail(r, err)
		cur, gerr := u.auth.GetUserByRef(r.Context(), ref)
		if gerr != nil {
			status, msg = u.fail(r, gerr)
			u.render(w, r, status, web.ErrorPage(msg))
			return
		}
		u.render(w, r, status, web.UserDetailPage(cur, msg))
		return
	}
	http.Redirect(w, r, "/system/users/"+usr.Username, http.StatusSeeOther)
}

func (u *uiHandler) setUserPassword(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "ref")
	if err := u.auth.SetPasswordByRef(r.Context(), ref, r.PostFormValue("password")); err != nil {
		status, msg := u.fail(r, err)
		cur, gerr := u.auth.GetUserByRef(r.Context(), ref)
		if gerr != nil {
			status, msg = u.fail(r, gerr)
			u.render(w, r, status, web.ErrorPage(msg))
			return
		}
		u.render(w, r, status, web.UserDetailPage(cur, msg))
		return
	}
	http.Redirect(w, r, "/system/users/"+ref, http.StatusSeeOther)
}

func (u *uiHandler) deleteUser(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "ref")

	// Self-deletion is a foot-gun; require another admin to do it.
	if self, ok := auth.UserFrom(r.Context()); ok && (self.Username == ref || self.ID.String() == ref) {
		_, msg := u.fail(r, fault.New(fault.Invalid, "you cannot delete your own account"))
		if hxTarget(r) == "users-section" {
			u.renderUsers(w, r, http.StatusOK, msg, false)
		} else {
			hxRedirect(w, "/system/users/"+ref, msg)
		}
		return
	}

	err := u.auth.DeleteUserByRef(r.Context(), ref)
	if hxTarget(r) == "users-section" {
		msg := ""
		if err != nil {
			_, msg = u.fail(r, err)
		}
		u.renderUsers(w, r, http.StatusOK, msg, false)
		return
	}
	if err != nil {
		_, msg := u.fail(r, err)
		hxRedirect(w, "/system/users/"+ref, msg)
		return
	}
	hxRedirect(w, "/system/users", "")
}

// --- API tokens (own profile) ---

func (u *uiHandler) tokensPage(w http.ResponseWriter, r *http.Request) {
	u.renderTokens(w, r, http.StatusOK, "", flashErr(r))
}

func (u *uiHandler) renderTokens(w http.ResponseWriter, r *http.Request, status int, newToken, errMsg string) {
	self, _ := auth.UserFrom(r.Context())
	items, err := u.auth.ListAPITokens(r.Context(), self.ID)
	if err != nil {
		st, msg := u.fail(r, err)
		u.render(w, r, st, web.ErrorPage(msg))
		return
	}
	u.render(w, r, status, web.TokensPage(items, newToken, errMsg))
}

// createToken renders the page directly (no redirect) so the plaintext
// token can be shown exactly once without ever entering a URL.
func (u *uiHandler) createToken(w http.ResponseWriter, r *http.Request) {
	self, _ := auth.UserFrom(r.Context())

	var expiresAt *time.Time
	if v := formValue(r, "expires_days"); v != "" {
		days, err := strconv.Atoi(v)
		if err != nil || days < 1 {
			u.renderTokens(w, r, http.StatusBadRequest, "", "expiry must be a positive number of days")
			return
		}
		t := time.Now().AddDate(0, 0, days)
		expiresAt = &t
	}

	_, raw, err := u.auth.CreateAPIToken(r.Context(), self.ID, formValue(r, "name"), expiresAt)
	if err != nil {
		status, msg := u.fail(r, err)
		u.renderTokens(w, r, status, "", msg)
		return
	}
	u.renderTokens(w, r, http.StatusOK, raw, "")
}

func (u *uiHandler) deleteToken(w http.ResponseWriter, r *http.Request) {
	self, _ := auth.UserFrom(r.Context())
	tokenID, err := id.Parse(chi.URLParam(r, "id"))
	if err != nil {
		hxRedirect(w, "/profile/tokens", "invalid token id")
		return
	}
	if err := u.auth.DeleteAPIToken(r.Context(), self.ID, tokenID); err != nil {
		_, msg := u.fail(r, err)
		hxRedirect(w, "/profile/tokens", msg)
		return
	}
	hxRedirect(w, "/profile/tokens", "")
}
