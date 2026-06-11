package auth

import (
	"context"
	"log/slog"
	"slices"
	"strings"

	"github.com/go-ldap/ldap/v3"

	"github.com/saku75/warren/internal/config"
	"github.com/saku75/warren/internal/core/fault"
)

// LDAPAuthenticator implements search-then-bind against a directory:
// connect, optionally bind a service account, locate the user entry,
// then bind as that entry with the presented password (design doc 0002
// §2). Every login opens a fresh connection — stateless and
// replica-safe, at the cost of a dial per login.
type LDAPAuthenticator struct {
	cfg config.LDAPConfig
	log *slog.Logger
}

func NewLDAPAuthenticator(cfg config.LDAPConfig, log *slog.Logger) *LDAPAuthenticator {
	return &LDAPAuthenticator{cfg: cfg, log: log}
}

func (l *LDAPAuthenticator) Provider() string { return ProviderLDAP }

func (l *LDAPAuthenticator) Authenticate(ctx context.Context, username, password string) (Identity, error) {
	// LDAP simple binds with an empty password succeed as anonymous on
	// many servers; never let that pass for a login.
	if password == "" {
		return Identity{}, errBadCredentials()
	}

	conn, err := ldap.DialURL(l.cfg.URL)
	if err != nil {
		l.log.Error("ldap: dial", "url", l.cfg.URL, "err", err)
		return Identity{}, fault.Wrap(fault.Internal, err, "directory unavailable")
	}
	defer conn.Close()
	if l.cfg.StartTLS {
		if err := conn.StartTLS(nil); err != nil {
			l.log.Error("ldap: starttls", "err", err)
			return Identity{}, fault.Wrap(fault.Internal, err, "directory unavailable")
		}
	}

	if l.cfg.BindDN != "" {
		if err := conn.Bind(l.cfg.BindDN, l.cfg.BindPassword); err != nil {
			l.log.Error("ldap: service bind", "bind_dn", l.cfg.BindDN, "err", err)
			return Identity{}, fault.Wrap(fault.Internal, err, "directory unavailable")
		}
	}

	filter := strings.ReplaceAll(l.cfg.UserFilter, "%s", ldap.EscapeFilter(username))
	attrs := []string{l.cfg.AttrUsername, l.cfg.AttrName, l.cfg.AttrEmail}
	if l.cfg.AdminGroupDN != "" {
		attrs = append(attrs, "memberOf")
	}
	res, err := conn.Search(ldap.NewSearchRequest(
		l.cfg.BaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		2, 10, false, filter, attrs, nil,
	))
	if err != nil {
		l.log.Error("ldap: search", "filter", filter, "err", err)
		return Identity{}, fault.Wrap(fault.Internal, err, "directory unavailable")
	}
	if len(res.Entries) != 1 {
		// Zero matches is a normal failed login; multiple matches means
		// the filter is ambiguous — log that loudly either way.
		l.log.Info("ldap: user lookup", "username", username, "matches", len(res.Entries))
		return Identity{}, errBadCredentials()
	}
	entry := res.Entries[0]

	// The actual authentication: bind as the located entry.
	if err := conn.Bind(entry.DN, password); err != nil {
		return Identity{}, errBadCredentials()
	}

	ident := Identity{
		Username:    entry.GetAttributeValue(l.cfg.AttrUsername),
		DisplayName: entry.GetAttributeValue(l.cfg.AttrName),
		Email:       entry.GetAttributeValue(l.cfg.AttrEmail),
		ExternalID:  entry.DN,
	}
	if ident.Username == "" {
		return Identity{}, fault.New(fault.Internal, "ldap entry %s has no %q attribute", entry.DN, l.cfg.AttrUsername)
	}
	if l.cfg.AdminGroupDN != "" {
		isAdmin := slices.ContainsFunc(entry.GetAttributeValues("memberOf"), func(g string) bool {
			return strings.EqualFold(g, l.cfg.AdminGroupDN)
		})
		ident.IsAdmin = &isAdmin
	}
	return ident, nil
}
