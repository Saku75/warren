package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/saku75/warren/internal/changelog"
	"github.com/saku75/warren/internal/core/fault"
	"github.com/saku75/warren/internal/core/id"
	"github.com/saku75/warren/internal/db/gen"
)

// Identity is what an external provider asserts about a person after a
// successful authentication.
type Identity struct {
	// Username is the provider's login name for the person; it becomes
	// the Warren username (lowercased) on JIT creation.
	Username    string
	DisplayName string
	Email       string
	// ExternalID is the provider's stable identifier (LDAP DN, OIDC
	// subject); JIT users are matched by it, so renames in the directory
	// don't create duplicates.
	ExternalID string
	// IsAdmin, when non-nil, is synchronized onto the user at every
	// login (group-mapped). Nil leaves Warren's flag untouched.
	IsAdmin *bool
}

// ExternalAuthenticator verifies a username+password against an external
// directory (LDAP). OIDC is redirect-based and uses LoginExternal
// directly.
type ExternalAuthenticator interface {
	// Provider returns the provider key stored on users ("ldap").
	Provider() string
	// Authenticate verifies the credentials and returns the directory's
	// view of the person. Failures must be indistinguishable to callers.
	Authenticate(ctx context.Context, username, password string) (Identity, error)
}

// SetExternalAuthenticator wires the optional password-style external
// provider (LDAP).
func (s *Service) SetExternalAuthenticator(a ExternalAuthenticator) {
	s.external = a
}

// normalizeExternalUsername maps a directory login name onto Warren's
// username rules.
func normalizeExternalUsername(name string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if err := validUsername(name); err != nil {
		return "", fault.New(fault.Invalid, "directory username %q does not fit Warren's username rules", name)
	}
	return name, nil
}

// loginExternalIdentity finds-or-creates the user for an asserted
// external identity and opens a session. Used by the LDAP password path
// and the OIDC callback.
func (s *Service) loginExternalIdentity(ctx context.Context, provider string, ident Identity, ip, userAgent string) (gen.Session, string, error) {
	if ident.ExternalID == "" {
		return gen.Session{}, "", fault.New(fault.Internal, "external provider returned an empty identifier")
	}

	u, err := s.q.GetUserByExternalID(ctx, gen.GetUserByExternalIDParams{Provider: provider, ExternalID: ident.ExternalID})
	switch {
	case err == nil:
		if u.Disabled {
			return gen.Session{}, "", errBadCredentials()
		}
		if u, err = s.syncExternalUser(ctx, u, ident); err != nil {
			return gen.Session{}, "", err
		}
	default:
		if u, err = s.createExternalUser(ctx, provider, ident); err != nil {
			return gen.Session{}, "", err
		}
	}

	return s.openSession(ctx, u, ip, userAgent)
}

// createExternalUser JIT-provisions a user row on first login.
func (s *Service) createExternalUser(ctx context.Context, provider string, ident Identity) (gen.User, error) {
	username, err := normalizeExternalUsername(ident.Username)
	if err != nil {
		return gen.User{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.User{}, fmt.Errorf("auth: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	u, err := q.CreateUser(ctx, gen.CreateUserParams{
		ID:           id.New(),
		Username:     username,
		DisplayName:  ident.DisplayName,
		Email:        ident.Email,
		Provider:     provider,
		ExternalID:   ident.ExternalID,
		PasswordHash: nil,
		IsAdmin:      ident.IsAdmin != nil && *ident.IsAdmin,
	})
	if err != nil {
		// Most likely a username collision with an existing account from
		// another provider; surface it clearly in the log, vaguely to the
		// person logging in.
		return gen.User{}, fault.Wrap(fault.Conflict, err, "cannot provision %s user %q (username already taken?)", provider, username)
	}
	if err := changelog.Record(ctx, q, changelog.ActionCreate, TypeUser.Key, u.ID, u.Username, nil, userSnapshot(u)); err != nil {
		return gen.User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.User{}, fmt.Errorf("auth: commit: %w", err)
	}
	return u, nil
}

// syncExternalUser refreshes directory-owned fields on login.
func (s *Service) syncExternalUser(ctx context.Context, u gen.User, ident Identity) (gen.User, error) {
	next := gen.UpdateUserParams{
		ID:          u.ID,
		Username:    u.Username,
		DisplayName: u.DisplayName,
		Email:       u.Email,
		IsAdmin:     u.IsAdmin,
		Disabled:    u.Disabled,
	}
	changed := false
	if ident.DisplayName != "" && ident.DisplayName != u.DisplayName {
		next.DisplayName, changed = ident.DisplayName, true
	}
	if ident.Email != "" && ident.Email != u.Email {
		next.Email, changed = ident.Email, true
	}
	if ident.IsAdmin != nil && *ident.IsAdmin != u.IsAdmin {
		next.IsAdmin, changed = *ident.IsAdmin, true
	}
	if !changed {
		return u, nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.User{}, fmt.Errorf("auth: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	updated, err := q.UpdateUser(ctx, next)
	if err != nil {
		return gen.User{}, fault.FromDB(err, "user "+u.Username)
	}
	if err := changelog.Record(ctx, q, changelog.ActionUpdate, TypeUser.Key, updated.ID, updated.Username, userSnapshot(u), userSnapshot(updated)); err != nil {
		return gen.User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.User{}, fmt.Errorf("auth: commit: %w", err)
	}
	return updated, nil
}

// openSession is the common tail of every successful login.
func (s *Service) openSession(ctx context.Context, u gen.User, ip, userAgent string) (gen.Session, string, error) {
	raw, hash := NewSessionToken()
	sess, err := s.q.CreateSession(ctx, gen.CreateSessionParams{
		ID:        id.New(),
		UserID:    u.ID,
		TokenHash: hash,
		CsrfToken: newSecret(),
		Ip:        ip,
		UserAgent: userAgent,
		ExpiresAt: time.Now().Add(SessionTTL),
	})
	if err != nil {
		return gen.Session{}, "", fault.FromDB(err, "session")
	}
	if err := s.q.TouchUserLogin(ctx, u.ID); err != nil {
		return gen.Session{}, "", fmt.Errorf("auth: touch login: %w", err)
	}
	return sess, raw, nil
}
