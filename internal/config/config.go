// Package config loads Warren's runtime configuration.
//
// Warren is configured entirely through environment variables (12-factor
// style) so that the same container image runs unchanged across replicas
// and environments. No configuration files, no local state.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime settings.
type Config struct {
	// Listen is the address the HTTP server binds, e.g. ":8080".
	Listen string
	// DatabaseURL is the PostgreSQL connection string. Required: Postgres
	// is Warren's only stateful component.
	DatabaseURL string
	// AutoMigrate applies pending schema migrations on startup. Handy for
	// single-node deployments; HA deployments should set it to false and
	// run `warren migrate` as a release step instead (the advisory lock
	// makes either safe).
	AutoMigrate bool
	// CookieSecure marks session cookies Secure. Enable whenever Warren
	// is served over HTTPS (directly or behind a TLS-terminating proxy).
	CookieSecure bool
	// LDAP configures the optional LDAP password provider.
	LDAP LDAPConfig
	// OIDC configures the optional OIDC SSO provider.
	OIDC OIDCConfig
	// ShutdownGrace is how long in-flight requests get to finish after
	// SIGTERM before the server exits. Keep it below the orchestrator's
	// termination grace period.
	ShutdownGrace time.Duration
}

// LDAPConfig holds the search-then-bind LDAP settings (design doc 0002
// §2). Enabled when URL is set.
type LDAPConfig struct {
	// URL is the directory address, ldap://host:389 or ldaps://host:636.
	URL string
	// StartTLS upgrades a plain ldap:// connection before any bind.
	StartTLS bool
	// BindDN/BindPassword are the service account used to search for the
	// login user. Empty BindDN means anonymous search.
	BindDN       string
	BindPassword string
	// BaseDN is the subtree searched for users.
	BaseDN string
	// UserFilter locates the login user; %s is replaced with the
	// (escaped) login name, e.g. "(uid=%s)" or "(sAMAccountName=%s)".
	UserFilter string
	// Attribute mapping from directory entries onto Warren users.
	AttrUsername string
	AttrName     string
	AttrEmail    string
	// AdminGroupDN grants is_admin to members (matched against the
	// entry's memberOf values) when set. Synced on every login.
	AdminGroupDN string
}

// Enabled reports whether LDAP login is configured.
func (c LDAPConfig) Enabled() bool { return c.URL != "" }

// OIDCConfig holds the OIDC code-flow settings (design doc 0002 §2).
// Enabled when Issuer is set.
type OIDCConfig struct {
	// Issuer is the IdP base URL; discovery is fetched from it.
	Issuer       string
	ClientID     string
	ClientSecret string
	// RedirectURL is this Warren's externally reachable callback,
	// e.g. https://warren.example.com/login/oidc/callback.
	RedirectURL string
	// Scopes requested in addition to "openid".
	Scopes []string
	// Claim mapping from the ID token onto Warren users.
	UsernameClaim string
	NameClaim     string
	EmailClaim    string
	// AdminGroup grants is_admin when present in GroupsClaim. Synced on
	// every login.
	GroupsClaim string
	AdminGroup  string
	// ButtonLabel names the IdP on the login page.
	ButtonLabel string
}

// Enabled reports whether OIDC login is configured.
func (c OIDCConfig) Enabled() bool { return c.Issuer != "" }

// FromEnv builds a Config from WARREN_* environment variables, applying
// defaults for anything unset.
func FromEnv() (Config, error) {
	cfg := Config{
		Listen:        getenv("WARREN_LISTEN", ":8080"),
		DatabaseURL:   os.Getenv("WARREN_DATABASE_URL"),
		AutoMigrate:   true,
		ShutdownGrace: 15 * time.Second,
	}

	if v := os.Getenv("WARREN_AUTO_MIGRATE"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("config: WARREN_AUTO_MIGRATE: %w", err)
		}
		cfg.AutoMigrate = b
	}

	if v := os.Getenv("WARREN_COOKIE_SECURE"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("config: WARREN_COOKIE_SECURE: %w", err)
		}
		cfg.CookieSecure = b
	}

	cfg.LDAP = LDAPConfig{
		URL:          os.Getenv("WARREN_LDAP_URL"),
		BindDN:       os.Getenv("WARREN_LDAP_BIND_DN"),
		BindPassword: os.Getenv("WARREN_LDAP_BIND_PASSWORD"),
		BaseDN:       os.Getenv("WARREN_LDAP_BASE_DN"),
		UserFilter:   getenv("WARREN_LDAP_USER_FILTER", "(uid=%s)"),
		AttrUsername: getenv("WARREN_LDAP_ATTR_USERNAME", "uid"),
		AttrName:     getenv("WARREN_LDAP_ATTR_NAME", "cn"),
		AttrEmail:    getenv("WARREN_LDAP_ATTR_EMAIL", "mail"),
		AdminGroupDN: os.Getenv("WARREN_LDAP_ADMIN_GROUP"),
	}
	if v := os.Getenv("WARREN_LDAP_START_TLS"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("config: WARREN_LDAP_START_TLS: %w", err)
		}
		cfg.LDAP.StartTLS = b
	}
	if cfg.LDAP.Enabled() && cfg.LDAP.BaseDN == "" {
		return Config{}, fmt.Errorf("config: WARREN_LDAP_BASE_DN is required when WARREN_LDAP_URL is set")
	}

	cfg.OIDC = OIDCConfig{
		Issuer:        os.Getenv("WARREN_OIDC_ISSUER"),
		ClientID:      os.Getenv("WARREN_OIDC_CLIENT_ID"),
		ClientSecret:  os.Getenv("WARREN_OIDC_CLIENT_SECRET"),
		RedirectURL:   os.Getenv("WARREN_OIDC_REDIRECT_URL"),
		Scopes:        strings.Fields(getenv("WARREN_OIDC_SCOPES", "profile email")),
		UsernameClaim: getenv("WARREN_OIDC_USERNAME_CLAIM", "preferred_username"),
		NameClaim:     getenv("WARREN_OIDC_NAME_CLAIM", "name"),
		EmailClaim:    getenv("WARREN_OIDC_EMAIL_CLAIM", "email"),
		GroupsClaim:   getenv("WARREN_OIDC_GROUPS_CLAIM", "groups"),
		AdminGroup:    os.Getenv("WARREN_OIDC_ADMIN_GROUP"),
		ButtonLabel:   getenv("WARREN_OIDC_NAME", "SSO"),
	}
	if cfg.OIDC.Enabled() && (cfg.OIDC.ClientID == "" || cfg.OIDC.RedirectURL == "") {
		return Config{}, fmt.Errorf("config: WARREN_OIDC_CLIENT_ID and WARREN_OIDC_REDIRECT_URL are required when WARREN_OIDC_ISSUER is set")
	}

	if v := os.Getenv("WARREN_SHUTDOWN_GRACE"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("config: WARREN_SHUTDOWN_GRACE: %w", err)
		}
		cfg.ShutdownGrace = d
	}

	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
