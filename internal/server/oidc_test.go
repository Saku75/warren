package server

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"

	"github.com/saku75/warren/internal/config"
	"github.com/saku75/warren/internal/db/dbtest"
	"github.com/saku75/warren/internal/db/gen"
)

// fakeIDP is a minimal OIDC identity provider: discovery, JWKS, and a
// token endpoint that enforces PKCE. The test plays the "user approves"
// step itself by minting a code and calling Warren's callback.
type fakeIDP struct {
	issuer string
	key    *rsa.PrivateKey

	mu    sync.Mutex
	codes map[string]fakeGrant // code -> grant
}

type fakeGrant struct {
	nonce     string
	challenge string
	claims    map[string]any
}

func (f *fakeIDP) issueCode(nonce, challenge string, claims map[string]any) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	code := "code-" + base64.RawURLEncoding.EncodeToString([]byte(nonce[:8]))
	f.codes[code] = fakeGrant{nonce: nonce, challenge: challenge, claims: claims}
	return code
}

func (f *fakeIDP) mux(t *testing.T) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                f.issuer,
			"authorization_endpoint":                f.issuer + "/auth",
			"token_endpoint":                        f.issuer + "/token",
			"jwks_uri":                              f.issuer + "/keys",
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
		})
	})

	mux.HandleFunc("GET /keys", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
			Key: &f.key.PublicKey, KeyID: "test", Algorithm: "RS256", Use: "sig",
		}}})
	})

	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		grant, ok := f.codes[r.PostFormValue("code")]
		f.mu.Unlock()
		if !ok {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		// PKCE: the verifier must hash to the challenge from /auth.
		sum := sha256.Sum256([]byte(r.PostFormValue("code_verifier")))
		if base64.RawURLEncoding.EncodeToString(sum[:]) != grant.challenge {
			http.Error(w, `{"error":"invalid_grant","error_description":"pkce"}`, http.StatusBadRequest)
			return
		}

		claims := map[string]any{
			"iss":   f.issuer,
			"aud":   "warren-test",
			"exp":   time.Now().Add(5 * time.Minute).Unix(),
			"iat":   time.Now().Unix(),
			"nonce": grant.nonce,
		}
		for k, v := range grant.claims {
			claims[k] = v
		}
		payload, _ := json.Marshal(claims)
		signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: f.key},
			(&jose.SignerOptions{}).WithHeader("kid", "test"))
		if err != nil {
			t.Errorf("signer: %v", err)
			http.Error(w, "signer", http.StatusInternalServerError)
			return
		}
		jws, err := signer.Sign(payload)
		if err != nil {
			t.Errorf("sign: %v", err)
			http.Error(w, "sign", http.StatusInternalServerError)
			return
		}
		idToken, _ := jws.CompactSerialize()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at-test", "token_type": "Bearer", "id_token": idToken,
		})
	})

	return mux
}

func TestOIDCEndToEnd(t *testing.T) {
	pool := dbtest.Pool(t)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa: %v", err)
	}
	idp := &fakeIDP{key: key, codes: map[string]fakeGrant{}}
	idpSrv := httptest.NewServer(idp.mux(t))
	t.Cleanup(idpSrv.Close)
	idp.issuer = idpSrv.URL

	// Warren's URL must be known before its config; route through an
	// indirection so the handler can be assigned afterwards.
	var handler http.Handler
	warrenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(warrenSrv.Close)

	cfg := config.Config{
		Listen: ":0",
		OIDC: config.OIDCConfig{
			Issuer:        idpSrv.URL,
			ClientID:      "warren-test",
			ClientSecret:  "secret",
			RedirectURL:   warrenSrv.URL + "/login/oidc/callback",
			Scopes:        []string{"profile", "email"},
			UsernameClaim: "preferred_username",
			NameClaim:     "name",
			EmailClaim:    "email",
			GroupsClaim:   "groups",
			AdminGroup:    "warren-admins",
			ButtonLabel:   "TestIdP",
		},
	}
	handler = New(cfg, slog.New(slog.DiscardHandler), "test", pool).Handler()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	// The login page offers the IdP.
	resp, err := client.Get(warrenSrv.URL + "/login")
	if err != nil {
		t.Fatalf("GET /login: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "Sign in with TestIdP") {
		t.Fatal("login page missing SSO button")
	}

	// Begin: Warren redirects to the IdP with state, nonce, and a PKCE
	// challenge.
	resp, err = client.Get(warrenSrv.URL + "/login/oidc?next=/dcim/sites")
	if err != nil {
		t.Fatalf("GET /login/oidc: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("begin: %d", resp.StatusCode)
	}
	authURL, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || !strings.HasPrefix(authURL.String(), idpSrv.URL+"/auth") {
		t.Fatalf("begin redirect: %q", resp.Header.Get("Location"))
	}
	q := authURL.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" || q.Get("nonce") == "" || q.Get("state") == "" {
		t.Fatalf("auth url missing params: %v", q)
	}

	// "User approves" at the IdP: it would redirect back with a code.
	username := uniqUser("sso.alice")
	code := idp.issueCode(q.Get("nonce"), q.Get("code_challenge"), map[string]any{
		"sub":                "sub-" + username,
		"preferred_username": username,
		"name":               "Alice SSO",
		"email":              "alice@example.org",
		"groups":             []string{"warren-admins", "everyone"},
	})

	// Tampered state is rejected.
	resp, err = client.Get(warrenSrv.URL + "/login/oidc/callback?state=tampered&code=" + code)
	if err != nil {
		t.Fatalf("tampered callback: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("tampered state: %d", resp.StatusCode)
	}

	// The state cookie was cleared by the rejected attempt; run the flow
	// again for the real callback.
	resp, _ = client.Get(warrenSrv.URL + "/login/oidc?next=/dcim/sites")
	resp.Body.Close()
	authURL, _ = url.Parse(resp.Header.Get("Location"))
	q = authURL.Query()
	code = idp.issueCode(q.Get("nonce"), q.Get("code_challenge"), map[string]any{
		"sub":                "sub-" + username,
		"preferred_username": username,
		"name":               "Alice SSO",
		"email":              "alice@example.org",
		"groups":             []string{"warren-admins", "everyone"},
	})

	resp, err = client.Get(warrenSrv.URL + "/login/oidc/callback?state=" + url.QueryEscape(q.Get("state")) + "&code=" + code)
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/dcim/sites" {
		t.Fatalf("callback: %d -> %q", resp.StatusCode, resp.Header.Get("Location"))
	}

	// The session works and the user was JIT-provisioned with the
	// group-mapped admin flag.
	resp, err = client.Get(warrenSrv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), username) {
		t.Fatalf("authed dashboard: %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), ">Users<") {
		t.Fatal("admin nav missing — group mapping failed")
	}

	var u gen.User
	row := pool.QueryRow(t.Context(), "SELECT provider, external_id, is_admin FROM users WHERE username = $1", username)
	if err := row.Scan(&u.Provider, &u.ExternalID, &u.IsAdmin); err != nil {
		t.Fatalf("user row: %v", err)
	}
	if u.Provider != "oidc" || u.ExternalID != "sub-"+username || !u.IsAdmin {
		t.Fatalf("JIT user wrong: %+v", u)
	}
}
