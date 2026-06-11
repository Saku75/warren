package auth

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/saku75/warren/internal/core/fault"
	"github.com/saku75/warren/internal/core/id"
	"github.com/saku75/warren/internal/db/dbtest"
)

func uniqUser(prefix string) string {
	u := id.New().String()
	return fmt.Sprintf("%s-%s", prefix, u[len(u)-12:])
}

func TestPasswordHashing(t *testing.T) {
	hash, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("hash format: %q", hash)
	}
	if !VerifyPassword(hash, "correct horse battery") {
		t.Fatal("correct password rejected")
	}
	if VerifyPassword(hash, "wrong") {
		t.Fatal("wrong password accepted")
	}
	if VerifyPassword("garbage", "x") {
		t.Fatal("garbage hash accepted")
	}
}

func TestUserAndSessionLifecycle(t *testing.T) {
	pool := dbtest.Pool(t)
	svc := NewService(pool)
	ctx := context.Background()

	username := uniqUser("alice")
	if _, err := svc.CreateLocalUser(ctx, UserInput{Username: username, Password: "short"}); fault.KindOf(err) != fault.Invalid {
		t.Fatalf("short password: got %v, want Invalid", err)
	}
	if _, err := svc.CreateLocalUser(ctx, UserInput{Username: "Bad User!", Password: "long-enough-pw"}); fault.KindOf(err) != fault.Invalid {
		t.Fatalf("bad username: got %v, want Invalid", err)
	}

	u, err := svc.CreateLocalUser(ctx, UserInput{Username: username, DisplayName: "Alice", Password: "a-decent-password", IsAdmin: true})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Wrong credentials are rejected with one indistinguishable error.
	if _, _, err := svc.LoginPassword(ctx, username, "wrong-password!", "", ""); fault.KindOf(err) != fault.Invalid {
		t.Fatalf("wrong password: got %v", err)
	}
	if _, _, err := svc.LoginPassword(ctx, uniqUser("ghost"), "a-decent-password", "", ""); fault.KindOf(err) != fault.Invalid {
		t.Fatalf("unknown user: got %v", err)
	}

	// Login opens a session that validates and carries a CSRF token.
	sess, raw, err := svc.LoginPassword(ctx, username, "a-decent-password", "127.0.0.1", "go-test")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if sess.CsrfToken == "" {
		t.Fatal("session missing CSRF token")
	}
	gotUser, gotSess, err := svc.ValidateSession(ctx, raw)
	if err != nil || gotUser.ID != u.ID || gotSess.ID != sess.ID {
		t.Fatalf("validate session: %v", err)
	}
	if _, _, err := svc.ValidateSession(ctx, "bogus-token"); fault.KindOf(err) != fault.NotFound {
		t.Fatalf("bogus session token: got %v", err)
	}

	// Disabling the user kills the session.
	disabled := true
	if _, err := svc.UpdateUserByRef(ctx, username, UserUpdate{Disabled: &disabled}); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if _, _, err := svc.ValidateSession(ctx, raw); err == nil {
		t.Fatal("session survived user disable")
	}
	if _, _, err := svc.LoginPassword(ctx, username, "a-decent-password", "", ""); fault.KindOf(err) != fault.Invalid {
		t.Fatalf("disabled login: got %v", err)
	}

	// Re-enable, password change, logout.
	enabled := false
	if _, err := svc.UpdateUserByRef(ctx, username, UserUpdate{Disabled: &enabled}); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if err := svc.SetPasswordByRef(ctx, username, "another-good-password"); err != nil {
		t.Fatalf("set password: %v", err)
	}
	sess2, raw2, err := svc.LoginPassword(ctx, username, "another-good-password", "", "")
	if err != nil {
		t.Fatalf("login after password change: %v", err)
	}
	if err := svc.Logout(ctx, sess2.ID); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, _, err := svc.ValidateSession(ctx, raw2); err == nil {
		t.Fatal("session survived logout")
	}

	if err := svc.DeleteUserByRef(ctx, username); err != nil {
		t.Fatalf("delete user: %v", err)
	}
}

func TestAPITokens(t *testing.T) {
	pool := dbtest.Pool(t)
	svc := NewService(pool)
	ctx := context.Background()

	username := uniqUser("bot")
	u, err := svc.CreateLocalUser(ctx, UserInput{Username: username, Password: "automation-pw-1"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	tok, raw, err := svc.CreateAPIToken(ctx, u.ID, "ci", nil)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	if !strings.HasPrefix(raw, APITokenPrefix) {
		t.Fatalf("token format: %q", raw)
	}
	got, err := svc.ValidateAPIToken(ctx, raw)
	if err != nil || got.ID != u.ID {
		t.Fatalf("validate token: %v", err)
	}
	if _, err := svc.ValidateAPIToken(ctx, APITokenPrefix+"nope"); err == nil {
		t.Fatal("bogus token accepted")
	}

	// Expired tokens are rejected.
	past := time.Now().Add(-time.Hour)
	_, rawExpired, err := svc.CreateAPIToken(ctx, u.ID, "old", &past)
	if err != nil {
		t.Fatalf("create expired token: %v", err)
	}
	if _, err := svc.ValidateAPIToken(ctx, rawExpired); err == nil {
		t.Fatal("expired token accepted")
	}

	// Revocation.
	if err := svc.DeleteAPIToken(ctx, u.ID, tok.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := svc.ValidateAPIToken(ctx, raw); err == nil {
		t.Fatal("revoked token accepted")
	}

	if err := svc.DeleteUserByRef(ctx, username); err != nil {
		t.Fatalf("delete user: %v", err)
	}
}
