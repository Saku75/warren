package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/saku75/warren/internal/auth"
	"github.com/saku75/warren/internal/config"
	"github.com/saku75/warren/internal/core/id"
	"github.com/saku75/warren/internal/db/dbtest"
)

var csrfInput = regexp.MustCompile(`name="_csrf" value="([^"]+)"`)

func uniqUser(prefix string) string {
	u := id.New().String()
	return fmt.Sprintf("%s-%s", prefix, u[len(u)-12:])
}

// TestAuthEndToEnd drives the real router: login wall, CSRF, admin
// gating, and API bearer tokens.
func TestAuthEndToEnd(t *testing.T) {
	pool := dbtest.Pool(t)
	authSvc := auth.NewService(pool)
	ctx := context.Background()

	adminName := uniqUser("admin")
	admin, err := authSvc.CreateLocalUser(ctx, auth.UserInput{Username: adminName, Password: "admin-password-1", IsAdmin: true})
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	userName := uniqUser("user")
	if _, err := authSvc.CreateLocalUser(ctx, auth.UserInput{Username: userName, Password: "user-password-12"}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	srv := httptest.NewServer(New(config.Config{Listen: ":0"}, slog.New(slog.DiscardHandler), "test", pool).Handler())
	t.Cleanup(srv.Close)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // observe redirects, don't follow
		},
	}

	get := func(path string) *http.Response {
		t.Helper()
		resp, err := client.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		return resp
	}
	readBody := func(resp *http.Response) string {
		t.Helper()
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return string(b)
	}

	// Anonymous UI requests hit the login wall; the API wants a token.
	resp := get("/dcim/sites")
	if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(resp.Header.Get("Location"), "/login") {
		t.Fatalf("anonymous UI: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	readBody(resp)
	resp = get("/api/v1/dcim/sites")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous API: %d", resp.StatusCode)
	}
	readBody(resp)
	if resp := get("/healthz"); resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz must stay open: %d", resp.StatusCode)
	} else {
		readBody(resp)
	}

	// Login: fetch the form (sets the double-submit cookie), then post.
	login := func(username, password string) *http.Response {
		t.Helper()
		body := readBody(get("/login"))
		m := csrfInput.FindStringSubmatch(body)
		if m == nil {
			t.Fatal("login page missing CSRF field")
		}
		resp, err := client.PostForm(srv.URL+"/login", url.Values{
			"_csrf": {m[1]}, "username": {username}, "password": {password}, "next": {"/dcim/sites"},
		})
		if err != nil {
			t.Fatalf("POST /login: %v", err)
		}
		return resp
	}

	resp = login(adminName, "wrong-password-x")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad login: %d", resp.StatusCode)
	}
	readBody(resp)

	resp = login(adminName, "admin-password-1")
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/dcim/sites" {
		t.Fatalf("login: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	readBody(resp)

	// Authenticated UI works and shows the user box.
	body := readBody(get("/"))
	if !strings.Contains(body, adminName) || !strings.Contains(body, "Sign out") {
		t.Fatal("dashboard missing signed-in user box")
	}

	// Mutations without the session CSRF token are rejected; with it they
	// pass (extracted from a rendered form).
	resp, err = client.PostForm(srv.URL+"/tenancy/tenants", url.Values{"name": {"CSRF Probe"}})
	if err != nil {
		t.Fatalf("POST without csrf: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("csrf-less POST: %d", resp.StatusCode)
	}
	readBody(resp)

	body = readBody(get("/tenancy/tenants/new"))
	m := csrfInput.FindStringSubmatch(body)
	if m == nil {
		t.Fatal("create form missing CSRF field")
	}
	tenantName := uniqUser("tenant")
	resp, err = client.PostForm(srv.URL+"/tenancy/tenants", url.Values{
		"_csrf": {m[1]}, "name": {tenantName},
	})
	if err != nil {
		t.Fatalf("POST with csrf: %v", err)
	}
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("csrf POST: %d", resp.StatusCode)
	}
	readBody(resp)

	// Admin page allowed for the admin.
	if resp := get("/system/users"); resp.StatusCode != http.StatusOK {
		t.Fatalf("admin users page: %d", resp.StatusCode)
	} else {
		readBody(resp)
	}

	// Bearer tokens drive the API; the admin-only user endpoints check
	// the token's user.
	_, rawToken, err := authSvc.CreateAPIToken(ctx, admin.ID, "test", nil)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/auth/users", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("API with token: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("API with admin token: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// A fresh client (no cookies) as the non-admin: login works, admin
	// area is forbidden.
	jar2, _ := cookiejar.New(nil)
	client = &http.Client{Jar: jar2, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp = login(userName, "user-password-12")
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("user login: %d", resp.StatusCode)
	}
	readBody(resp)
	resp = get("/system/users")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin on users page: %d", resp.StatusCode)
	}
	readBody(resp)
}
