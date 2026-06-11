package auth

import (
	"context"
	"log/slog"
	"slices"
	"sync"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/saku75/warren/internal/config"
	"github.com/saku75/warren/internal/core/fault"
)

// OIDCClient is Warren's relying party for the authorization-code flow
// with PKCE (design doc 0002 §2). Issuer discovery is lazy and cached:
// the server must be able to boot (and serve local logins) while the
// IdP is down.
type OIDCClient struct {
	cfg config.OIDCConfig
	log *slog.Logger

	mu       sync.Mutex
	provider *oidc.Provider
}

func NewOIDCClient(cfg config.OIDCConfig, log *slog.Logger) *OIDCClient {
	return &OIDCClient{cfg: cfg, log: log}
}

// ButtonLabel names the IdP on the login page.
func (o *OIDCClient) ButtonLabel() string { return o.cfg.ButtonLabel }

func (o *OIDCClient) discover(ctx context.Context) (*oidc.Provider, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.provider != nil {
		return o.provider, nil
	}
	p, err := oidc.NewProvider(ctx, o.cfg.Issuer)
	if err != nil {
		o.log.Error("oidc: discovery", "issuer", o.cfg.Issuer, "err", err)
		return nil, fault.Wrap(fault.Internal, err, "identity provider unavailable")
	}
	o.provider = p
	return p, nil
}

func (o *OIDCClient) oauthConfig(p *oidc.Provider) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     o.cfg.ClientID,
		ClientSecret: o.cfg.ClientSecret,
		Endpoint:     p.Endpoint(),
		RedirectURL:  o.cfg.RedirectURL,
		Scopes:       append([]string{oidc.ScopeOpenID}, o.cfg.Scopes...),
	}
}

// AuthURL builds the IdP redirect for one login attempt. state, nonce,
// and the PKCE verifier are minted per attempt; the caller persists them
// in short-lived cookies for the callback to check.
func (o *OIDCClient) AuthURL(ctx context.Context) (authURL, state, nonce, pkceVerifier string, err error) {
	p, err := o.discover(ctx)
	if err != nil {
		return "", "", "", "", err
	}
	state, nonce, pkceVerifier = newSecret(), newSecret(), oauth2.GenerateVerifier()
	authURL = o.oauthConfig(p).AuthCodeURL(state,
		oidc.Nonce(nonce),
		oauth2.S256ChallengeOption(pkceVerifier),
	)
	return authURL, state, nonce, pkceVerifier, nil
}

// Exchange redeems the callback code, verifies the ID token (signature,
// issuer, audience, nonce), and maps the configured claims onto an
// Identity.
func (o *OIDCClient) Exchange(ctx context.Context, code, nonce, pkceVerifier string) (Identity, error) {
	p, err := o.discover(ctx)
	if err != nil {
		return Identity{}, err
	}

	token, err := o.oauthConfig(p).Exchange(ctx, code, oauth2.VerifierOption(pkceVerifier))
	if err != nil {
		o.log.Error("oidc: code exchange", "err", err)
		return Identity{}, fault.Wrap(fault.Invalid, err, "sign-in could not be completed")
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return Identity{}, fault.New(fault.Invalid, "identity provider returned no id_token")
	}

	idToken, err := p.Verifier(&oidc.Config{ClientID: o.cfg.ClientID}).Verify(ctx, rawIDToken)
	if err != nil {
		o.log.Error("oidc: id_token verify", "err", err)
		return Identity{}, fault.Wrap(fault.Invalid, err, "sign-in could not be completed")
	}
	if idToken.Nonce != nonce {
		return Identity{}, fault.New(fault.Invalid, "sign-in could not be completed (nonce mismatch)")
	}

	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		return Identity{}, fault.Wrap(fault.Invalid, err, "unreadable identity claims")
	}

	str := func(claim string) string {
		v, _ := claims[claim].(string)
		return v
	}
	ident := Identity{
		Username:    str(o.cfg.UsernameClaim),
		DisplayName: str(o.cfg.NameClaim),
		Email:       str(o.cfg.EmailClaim),
		ExternalID:  idToken.Subject,
	}
	if ident.Username == "" {
		return Identity{}, fault.New(fault.Invalid, "id_token has no %q claim", o.cfg.UsernameClaim)
	}
	if o.cfg.AdminGroup != "" {
		isAdmin := false
		if groups, ok := claims[o.cfg.GroupsClaim].([]any); ok {
			isAdmin = slices.ContainsFunc(groups, func(g any) bool {
				gs, _ := g.(string)
				return gs == o.cfg.AdminGroup
			})
		}
		ident.IsAdmin = &isAdmin
	}
	return ident, nil
}
