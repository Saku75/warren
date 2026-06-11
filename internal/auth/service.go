package auth

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/saku75/warren/internal/changelog"
	"github.com/saku75/warren/internal/core/fault"
	"github.com/saku75/warren/internal/core/id"
	"github.com/saku75/warren/internal/core/objtype"
	"github.com/saku75/warren/internal/db/gen"
)

// TypeUser is the registered type key for user accounts.
var TypeUser = objtype.Register(objtype.Type{Key: "auth.user", Name: "User", Plural: "Users"})

// SessionTTL is the sliding browser-session lifetime.
const SessionTTL = 7 * 24 * time.Hour

// sessionExtendAfter throttles session-extension writes: a session row
// is updated at most once per this interval.
const sessionExtendAfter = time.Hour

// ProviderLocal is the built-in username/password provider. LDAP and
// OIDC join in later Phase 2 work (design doc 0002 §2).
const ProviderLocal = "local"

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// MinPasswordLen is the minimum local-account password length.
const MinPasswordLen = 10

// Service implements users, sessions, and API tokens.
type Service struct {
	pool *pgxpool.Pool
	q    *gen.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: gen.New(pool)}
}

func userSnapshot(u gen.User) map[string]any {
	return map[string]any{
		"username":     u.Username,
		"display_name": u.DisplayName,
		"email":        u.Email,
		"provider":     u.Provider,
		"is_admin":     u.IsAdmin,
		"disabled":     u.Disabled,
	}
}

// UserInput are the writable fields for creating a local user.
type UserInput struct {
	Username    string
	DisplayName string
	Email       string
	Password    string
	IsAdmin     bool
}

// UserUpdate carries partial changes; nil fields are untouched.
type UserUpdate struct {
	Username    *string
	DisplayName *string
	Email       *string
	IsAdmin     *bool
	Disabled    *bool
}

func validUsername(u string) error {
	if !usernamePattern.MatchString(u) {
		return fault.New(fault.Invalid, "username must be lowercase letters, digits, '.', '_' or '-', starting with a letter or digit (max 64)")
	}
	return nil
}

func validPassword(p string) error {
	if len(p) < MinPasswordLen {
		return fault.New(fault.Invalid, "password must be at least %d characters", MinPasswordLen)
	}
	return nil
}

// CreateLocalUser creates a local-provider user.
func (s *Service) CreateLocalUser(ctx context.Context, in UserInput) (gen.User, error) {
	if err := validUsername(in.Username); err != nil {
		return gen.User{}, err
	}
	if err := validPassword(in.Password); err != nil {
		return gen.User{}, err
	}
	hash, err := HashPassword(in.Password)
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
		Username:     in.Username,
		DisplayName:  in.DisplayName,
		Email:        in.Email,
		Provider:     ProviderLocal,
		ExternalID:   "",
		PasswordHash: &hash,
		IsAdmin:      in.IsAdmin,
	})
	if err != nil {
		return gen.User{}, fault.FromDB(err, "user "+in.Username)
	}
	if err := changelog.Record(ctx, q, changelog.ActionCreate, TypeUser.Key, u.ID, u.Username, nil, userSnapshot(u)); err != nil {
		return gen.User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.User{}, fmt.Errorf("auth: commit: %w", err)
	}
	return u, nil
}

// GetUserByRef resolves a user from an ID or username.
func (s *Service) GetUserByRef(ctx context.Context, ref string) (gen.User, error) {
	if uid, err := id.Parse(ref); err == nil {
		u, err := s.q.GetUser(ctx, uid)
		return u, fault.FromDB(err, "user "+ref)
	}
	if err := validUsername(ref); err != nil {
		return gen.User{}, err
	}
	u, err := s.q.GetUserByUsername(ctx, ref)
	return u, fault.FromDB(err, "user "+ref)
}

// ListUsers returns users plus the total count.
func (s *Service) ListUsers(ctx context.Context, limit, offset int32) ([]gen.User, int64, error) {
	total, err := s.q.CountUsers(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("auth: count users: %w", err)
	}
	items, err := s.q.ListUsers(ctx, gen.ListUsersParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, fmt.Errorf("auth: list users: %w", err)
	}
	return items, total, nil
}

// CountUsers reports how many users exist (bootstrap hint).
func (s *Service) CountUsers(ctx context.Context) (int64, error) {
	return s.q.CountUsers(ctx)
}

// UpdateUserByRef applies a partial update. Disabling a user destroys
// their sessions in the same transaction.
func (s *Service) UpdateUserByRef(ctx context.Context, ref string, up UserUpdate) (gen.User, error) {
	cur, err := s.GetUserByRef(ctx, ref)
	if err != nil {
		return gen.User{}, err
	}

	next := gen.UpdateUserParams{
		ID:          cur.ID,
		Username:    cur.Username,
		DisplayName: cur.DisplayName,
		Email:       cur.Email,
		IsAdmin:     cur.IsAdmin,
		Disabled:    cur.Disabled,
	}
	if up.Username != nil {
		if err := validUsername(*up.Username); err != nil {
			return gen.User{}, err
		}
		next.Username = *up.Username
	}
	if up.DisplayName != nil {
		next.DisplayName = *up.DisplayName
	}
	if up.Email != nil {
		next.Email = *up.Email
	}
	if up.IsAdmin != nil {
		next.IsAdmin = *up.IsAdmin
	}
	if up.Disabled != nil {
		next.Disabled = *up.Disabled
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.User{}, fmt.Errorf("auth: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	u, err := q.UpdateUser(ctx, next)
	if err != nil {
		return gen.User{}, fault.FromDB(err, "user "+ref)
	}
	if u.Disabled && !cur.Disabled {
		if err := q.DeleteUserSessions(ctx, u.ID); err != nil {
			return gen.User{}, fmt.Errorf("auth: kill sessions: %w", err)
		}
	}
	if err := changelog.Record(ctx, q, changelog.ActionUpdate, TypeUser.Key, u.ID, u.Username, userSnapshot(cur), userSnapshot(u)); err != nil {
		return gen.User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.User{}, fmt.Errorf("auth: commit: %w", err)
	}
	return u, nil
}

// SetPasswordByRef sets a new password for a local user.
func (s *Service) SetPasswordByRef(ctx context.Context, ref, password string) error {
	cur, err := s.GetUserByRef(ctx, ref)
	if err != nil {
		return err
	}
	if cur.Provider != ProviderLocal {
		return fault.New(fault.Invalid, "user %s authenticates via %s; no local password", cur.Username, cur.Provider)
	}
	if err := validPassword(password); err != nil {
		return err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	if err := s.q.SetUserPassword(ctx, gen.SetUserPasswordParams{ID: cur.ID, PasswordHash: &hash}); err != nil {
		return fault.FromDB(err, "user "+ref)
	}
	return nil
}

// DeleteUserByRef removes a user; their sessions and tokens cascade.
func (s *Service) DeleteUserByRef(ctx context.Context, ref string) error {
	cur, err := s.GetUserByRef(ctx, ref)
	if err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("auth: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	if _, err := q.DeleteUser(ctx, cur.ID); err != nil {
		return fault.FromDB(err, "user "+ref)
	}
	if err := changelog.Record(ctx, q, changelog.ActionDelete, TypeUser.Key, cur.ID, cur.Username, userSnapshot(cur), nil); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("auth: commit: %w", err)
	}
	return nil
}

// errBadCredentials is deliberately vague: it never reveals whether the
// username or the password was wrong, or whether the user exists.
func errBadCredentials() error {
	return fault.New(fault.Invalid, "invalid username or password")
}

// LoginPassword authenticates a username+password and opens a session,
// returning the raw cookie token (its only appearance in plaintext).
func (s *Service) LoginPassword(ctx context.Context, username, password, ip, userAgent string) (gen.Session, string, error) {
	if validUsername(username) != nil || password == "" {
		return gen.Session{}, "", errBadCredentials()
	}
	u, err := s.q.GetUserByUsername(ctx, username)
	if err != nil {
		// Burn comparable time so user existence is not observable.
		VerifyPassword("$argon2id$v=19$m=65536,t=1,p=4$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", password)
		return gen.Session{}, "", errBadCredentials()
	}
	if u.Disabled || u.Provider != ProviderLocal || u.PasswordHash == nil {
		return gen.Session{}, "", errBadCredentials()
	}
	if !VerifyPassword(*u.PasswordHash, password) {
		return gen.Session{}, "", errBadCredentials()
	}

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

// ValidateSession resolves a cookie token to its user and session,
// sliding the expiry forward. Expired sessions are deleted on sight.
func (s *Service) ValidateSession(ctx context.Context, rawToken string) (gen.User, gen.Session, error) {
	row, err := s.q.GetSessionByTokenHash(ctx, HashToken(rawToken))
	if err != nil {
		return gen.User{}, gen.Session{}, fault.FromDB(err, "session")
	}
	if time.Now().After(row.Session.ExpiresAt) {
		_ = s.q.DeleteSession(ctx, row.Session.ID)
		return gen.User{}, gen.Session{}, fault.New(fault.NotFound, "session expired")
	}
	if row.User.Disabled {
		return gen.User{}, gen.Session{}, fault.New(fault.NotFound, "user disabled")
	}
	if time.Since(row.Session.LastSeenAt) > sessionExtendAfter {
		_ = s.q.ExtendSession(ctx, gen.ExtendSessionParams{
			ID:        row.Session.ID,
			ExpiresAt: time.Now().Add(SessionTTL),
		})
	}
	return row.User, row.Session, nil
}

// Logout destroys a session.
func (s *Service) Logout(ctx context.Context, sessionID id.ID) error {
	return s.q.DeleteSession(ctx, sessionID)
}

// CreateAPIToken mints a token for the user, returning the plaintext
// exactly once.
func (s *Service) CreateAPIToken(ctx context.Context, userID id.ID, name string, expiresAt *time.Time) (gen.ApiToken, string, error) {
	if name == "" {
		return gen.ApiToken{}, "", fault.New(fault.Invalid, "token name is required")
	}
	raw, hash := NewAPIToken()
	t, err := s.q.CreateAPIToken(ctx, gen.CreateAPITokenParams{
		ID:        id.New(),
		UserID:    userID,
		Name:      name,
		TokenHash: hash,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return gen.ApiToken{}, "", fault.FromDB(err, "api token")
	}
	return t, raw, nil
}

// ValidateAPIToken resolves a presented bearer token to its user.
func (s *Service) ValidateAPIToken(ctx context.Context, raw string) (gen.User, error) {
	if !IsAPIToken(raw) {
		return gen.User{}, fault.New(fault.NotFound, "invalid token")
	}
	row, err := s.q.GetAPITokenByHash(ctx, HashToken(raw))
	if err != nil {
		return gen.User{}, fault.FromDB(err, "api token")
	}
	if row.ApiToken.ExpiresAt != nil && time.Now().After(*row.ApiToken.ExpiresAt) {
		return gen.User{}, fault.New(fault.NotFound, "token expired")
	}
	if row.User.Disabled {
		return gen.User{}, fault.New(fault.NotFound, "user disabled")
	}
	_ = s.q.TouchAPIToken(ctx, row.ApiToken.ID)
	return row.User, nil
}

// ListAPITokens returns the user's tokens, newest first.
func (s *Service) ListAPITokens(ctx context.Context, userID id.ID) ([]gen.ApiToken, error) {
	items, err := s.q.ListAPITokensByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("auth: list tokens: %w", err)
	}
	return items, nil
}

// DeleteAPIToken revokes one of the user's own tokens.
func (s *Service) DeleteAPIToken(ctx context.Context, userID, tokenID id.ID) error {
	n, err := s.q.DeleteAPIToken(ctx, gen.DeleteAPITokenParams{ID: tokenID, UserID: userID})
	if err != nil {
		return fault.FromDB(err, "api token")
	}
	if n == 0 {
		return fault.New(fault.NotFound, "api token not found")
	}
	return nil
}
