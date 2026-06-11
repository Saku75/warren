package server

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/saku75/warren/internal/auth"
	"github.com/saku75/warren/internal/changelog"
	"github.com/saku75/warren/internal/core/fault"
	"github.com/saku75/warren/internal/web"
)

const (
	sessionCookie   = "warren_session"
	loginCSRFCookie = "warren_login_csrf"
)

// authHandlers owns authentication middleware and the login lifecycle.
type authHandlers struct {
	log          *slog.Logger
	auth         *auth.Service
	cookieSecure bool
}

// loadSession resolves the session cookie into request context (user,
// session, changelog actor). It never rejects; guards decide later.
func (a *authHandlers) loadSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
			if user, sess, err := a.auth.ValidateSession(r.Context(), c.Value); err == nil {
				ctx := auth.WithUser(r.Context(), user)
				ctx = auth.WithSession(ctx, sess)
				ctx = changelog.WithActor(ctx, user.Username)
				r = r.WithContext(ctx)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// requireUI gates browser routes: anonymous requests are sent to the
// login page, preserving the original destination.
func (a *authHandlers) requireUI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := auth.UserFrom(r.Context()); ok {
			next.ServeHTTP(w, r)
			return
		}
		target := "/login"
		if r.URL.Path != "/" {
			target += "?next=" + url.QueryEscape(r.URL.RequestURI())
		}
		// htmx requests get a client-side redirect so the full login page
		// replaces the document instead of swapping into a fragment.
		if isHX(r) {
			w.Header().Set("HX-Redirect", target)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, target, http.StatusSeeOther)
	})
}

// requireAdminUI gates admin-only browser routes.
func (a *authHandlers) requireAdminUI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, ok := auth.UserFrom(r.Context()); !ok || !u.IsAdmin {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusForbidden)
			_ = web.ErrorPage("This area requires administrator access.").Render(r.Context(), w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// csrfUI enforces the session CSRF token on mutating browser requests,
// accepting it from the X-CSRF-Token header (htmx) or the _csrf form
// field (plain forms).
func (a *authHandlers) csrfUI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		want := auth.CSRFFrom(r.Context())
		got := r.Header.Get("X-CSRF-Token")
		if got == "" {
			got = r.PostFormValue("_csrf")
		}
		if want == "" || subtle.ConstantTimeCompare([]byte(want), []byte(got)) != 1 {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusForbidden)
			_ = web.ErrorPage("The form has expired — go back, reload the page, and try again.").Render(r.Context(), w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireAPI gates the REST API on bearer tokens. Cookies are
// deliberately not accepted here, which keeps the API CSRF-immune.
func (a *authHandlers) requireAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if ok {
			if user, err := a.auth.ValidateAPIToken(r.Context(), strings.TrimSpace(raw)); err == nil {
				ctx := auth.WithUser(r.Context(), user)
				ctx = changelog.WithActor(ctx, user.Username)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{"code": "unauthorized", "message": "a valid API token is required (Authorization: Bearer wrt_…)"},
		})
	})
}

// sanitizeNext keeps post-login redirects on this site.
func sanitizeNext(next string) string {
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return "/"
	}
	return next
}

func (a *authHandlers) setSessionCookie(w http.ResponseWriter, value string, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    value,
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		HttpOnly: true,
		Secure:   a.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// getLogin renders the login form with a fresh double-submit CSRF
// cookie (the session CSRF token does not exist yet at login time).
func (a *authHandlers) getLogin(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.UserFrom(r.Context()); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	csrf := auth.NewLoginCSRF()
	http.SetCookie(w, &http.Cookie{
		Name:     loginCSRFCookie,
		Value:    csrf,
		Path:     "/login",
		MaxAge:   3600,
		HttpOnly: true,
		Secure:   a.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = web.LoginPage(sanitizeNext(r.URL.Query().Get("next")), csrf, "").Render(r.Context(), w)
}

func (a *authHandlers) postLogin(w http.ResponseWriter, r *http.Request) {
	fail := func(status int, msg string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		csrf := auth.NewLoginCSRF()
		http.SetCookie(w, &http.Cookie{
			Name: loginCSRFCookie, Value: csrf, Path: "/login", MaxAge: 3600,
			HttpOnly: true, Secure: a.cookieSecure, SameSite: http.SameSiteLaxMode,
		})
		_ = web.LoginPage(sanitizeNext(r.PostFormValue("next")), csrf, msg).Render(r.Context(), w)
	}

	cookie, err := r.Cookie(loginCSRFCookie)
	if err != nil || cookie.Value == "" ||
		subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(r.PostFormValue("_csrf"))) != 1 {
		fail(http.StatusForbidden, "The form has expired — please try again.")
		return
	}

	sess, raw, err := a.auth.LoginPassword(r.Context(),
		strings.TrimSpace(r.PostFormValue("username")),
		r.PostFormValue("password"),
		r.RemoteAddr,
		r.UserAgent(),
	)
	if err != nil {
		a.log.Info("login failed", "username", r.PostFormValue("username"), "ip", r.RemoteAddr)
		fail(fault.HTTPStatus(fault.KindOf(err)), fault.Message(err))
		return
	}
	_ = sess

	a.setSessionCookie(w, raw, auth.SessionTTL)
	http.SetCookie(w, &http.Cookie{Name: loginCSRFCookie, Path: "/login", MaxAge: -1})
	http.Redirect(w, r, sanitizeNext(r.PostFormValue("next")), http.StatusSeeOther)
}

func (a *authHandlers) postLogout(w http.ResponseWriter, r *http.Request) {
	if sess, ok := auth.SessionFrom(r.Context()); ok {
		if err := a.auth.Logout(r.Context(), sess.ID); err != nil {
			a.log.Error("logout", "err", err)
		}
	}
	a.setSessionCookie(w, "", -1)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
