package auth

import (
	"context"

	"github.com/saku75/warren/internal/db/gen"
)

type userKey struct{}
type sessionKey struct{}

// WithUser attaches the authenticated user to the context.
func WithUser(ctx context.Context, u gen.User) context.Context {
	return context.WithValue(ctx, userKey{}, u)
}

// UserFrom returns the authenticated user, if any.
func UserFrom(ctx context.Context) (gen.User, bool) {
	u, ok := ctx.Value(userKey{}).(gen.User)
	return u, ok
}

// WithSession attaches the active browser session to the context.
func WithSession(ctx context.Context, s gen.Session) context.Context {
	return context.WithValue(ctx, sessionKey{}, s)
}

// SessionFrom returns the active browser session, if any.
func SessionFrom(ctx context.Context) (gen.Session, bool) {
	s, ok := ctx.Value(sessionKey{}).(gen.Session)
	return s, ok
}

// CSRFFrom returns the CSRF token of the active session, or "" when
// anonymous. Templates embed it in forms; the middleware enforces it.
func CSRFFrom(ctx context.Context) string {
	if s, ok := SessionFrom(ctx); ok {
		return s.CsrfToken
	}
	return ""
}
