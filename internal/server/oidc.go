package server

import (
	"crypto/subtle"
	"net/http"

	"github.com/saku75/warren/internal/auth"
	"github.com/saku75/warren/internal/core/fault"
	"github.com/saku75/warren/internal/web"
)

// Per-attempt OIDC cookies. Path-scoped to the callback, 10-minute TTL.
const (
	oidcStateCookie = "warren_oidc_state"
	oidcNonceCookie = "warren_oidc_nonce"
	oidcPKCECookie  = "warren_oidc_pkce"
	oidcNextCookie  = "warren_oidc_next"
)

func (a *authHandlers) setOIDCCookie(w http.ResponseWriter, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/login/oidc",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   a.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// beginOIDC starts one sign-in attempt and hands off to the IdP.
func (a *authHandlers) beginOIDC(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.UserFrom(r.Context()); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	authURL, state, nonce, pkce, err := a.oidc.AuthURL(r.Context())
	if err != nil {
		a.renderLoginError(w, r, fault.HTTPStatus(fault.KindOf(err)), fault.Message(err))
		return
	}
	a.setOIDCCookie(w, oidcStateCookie, state, 600)
	a.setOIDCCookie(w, oidcNonceCookie, nonce, 600)
	a.setOIDCCookie(w, oidcPKCECookie, pkce, 600)
	a.setOIDCCookie(w, oidcNextCookie, sanitizeNext(r.URL.Query().Get("next")), 600)
	http.Redirect(w, r, authURL, http.StatusSeeOther)
}

// callbackOIDC finishes the sign-in: state check, code exchange, ID
// token verification, JIT user provisioning, session.
func (a *authHandlers) callbackOIDC(w http.ResponseWriter, r *http.Request) {
	cookie := func(name string) string {
		c, err := r.Cookie(name)
		if err != nil {
			return ""
		}
		return c.Value
	}
	clear := func() {
		for _, n := range []string{oidcStateCookie, oidcNonceCookie, oidcPKCECookie, oidcNextCookie} {
			a.setOIDCCookie(w, n, "", -1)
		}
	}

	state, nonce, pkce := cookie(oidcStateCookie), cookie(oidcNonceCookie), cookie(oidcPKCECookie)
	if state == "" || subtle.ConstantTimeCompare([]byte(state), []byte(r.URL.Query().Get("state"))) != 1 {
		clear()
		a.renderLoginError(w, r, http.StatusForbidden, "The sign-in attempt expired — please try again.")
		return
	}
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		clear()
		a.log.Info("oidc: idp returned error", "error", errParam, "description", r.URL.Query().Get("error_description"))
		a.renderLoginError(w, r, http.StatusBadRequest, "The identity provider rejected the sign-in.")
		return
	}

	ident, err := a.oidc.Exchange(r.Context(), r.URL.Query().Get("code"), nonce, pkce)
	if err != nil {
		clear()
		a.renderLoginError(w, r, fault.HTTPStatus(fault.KindOf(err)), fault.Message(err))
		return
	}

	_, raw, err := a.auth.LoginOIDC(r.Context(), ident, r.RemoteAddr, r.UserAgent())
	if err != nil {
		clear()
		a.log.Warn("oidc: login rejected", "username", ident.Username, "err", err)
		a.renderLoginError(w, r, fault.HTTPStatus(fault.KindOf(err)), fault.Message(err))
		return
	}

	next := sanitizeNext(cookie(oidcNextCookie))
	clear()
	a.setSessionCookie(w, raw, auth.SessionTTL)
	http.Redirect(w, r, next, http.StatusSeeOther)
}

// renderLoginError shows the login page with a message and a fresh
// double-submit token.
func (a *authHandlers) renderLoginError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	csrf := auth.NewLoginCSRF()
	http.SetCookie(w, &http.Cookie{
		Name: loginCSRFCookie, Value: csrf, Path: "/login", MaxAge: 3600,
		HttpOnly: true, Secure: a.cookieSecure, SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = web.LoginPage("/", csrf, a.ssoLabel(), msg).Render(r.Context(), w)
}

func (a *authHandlers) ssoLabel() string {
	if a.oidc == nil {
		return ""
	}
	return a.oidc.ButtonLabel()
}
