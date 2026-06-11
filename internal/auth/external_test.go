package auth

import (
	"context"
	"testing"

	"github.com/saku75/warren/internal/core/fault"
	"github.com/saku75/warren/internal/db/dbtest"
)

// fakeDirectory is a password-style external provider for exercising the
// JIT-provisioning and sync logic without a real LDAP server.
type fakeDirectory struct {
	accounts map[string]struct {
		password string
		ident    Identity
	}
}

func (f *fakeDirectory) Provider() string { return ProviderLDAP }

func (f *fakeDirectory) Authenticate(_ context.Context, username, password string) (Identity, error) {
	acc, ok := f.accounts[username]
	if !ok || acc.password != password {
		return Identity{}, errBadCredentials()
	}
	return acc.ident, nil
}

func boolPtr(b bool) *bool { return &b }

func TestExternalJITAndSync(t *testing.T) {
	pool := dbtest.Pool(t)
	svc := NewService(pool)
	ctx := context.Background()

	username := uniqUser("carol")
	dn := "uid=" + username + ",ou=people,dc=example,dc=org"
	dir := &fakeDirectory{accounts: map[string]struct {
		password string
		ident    Identity
	}{
		username: {password: "directory-pw-1", ident: Identity{
			Username: username, DisplayName: "Carol", Email: "carol@example.org",
			ExternalID: dn, IsAdmin: boolPtr(false),
		}},
	}}
	svc.SetExternalAuthenticator(dir)

	// Wrong directory password never provisions anything.
	if _, _, err := svc.LoginPassword(ctx, username, "wrong", "", ""); fault.KindOf(err) != fault.Invalid {
		t.Fatalf("wrong external password: %v", err)
	}
	if _, err := svc.GetUserByRef(ctx, username); fault.KindOf(err) != fault.NotFound {
		t.Fatal("user provisioned despite failed login")
	}

	// First successful login JIT-creates the user.
	_, raw, err := svc.LoginPassword(ctx, username, "directory-pw-1", "", "")
	if err != nil {
		t.Fatalf("ldap login: %v", err)
	}
	u, _, err := svc.ValidateSession(ctx, raw)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	if u.Provider != ProviderLDAP || u.ExternalID != dn || u.IsAdmin || u.PasswordHash != nil {
		t.Fatalf("JIT user wrong: %+v", u)
	}

	// Directory changes sync on the next login, same user row.
	acc := dir.accounts[username]
	acc.ident.DisplayName = "Carol Promoted"
	acc.ident.IsAdmin = boolPtr(true)
	dir.accounts[username] = acc

	if _, _, err := svc.LoginPassword(ctx, username, "directory-pw-1", "", ""); err != nil {
		t.Fatalf("second login: %v", err)
	}
	u2, err := svc.GetUserByRef(ctx, username)
	if err != nil {
		t.Fatalf("get after sync: %v", err)
	}
	if u2.ID != u.ID || !u2.IsAdmin || u2.DisplayName != "Carol Promoted" {
		t.Fatalf("sync wrong: %+v", u2)
	}

	// Disabled in Warren wins over the directory.
	disabled := true
	if _, err := svc.UpdateUserByRef(ctx, username, UserUpdate{Disabled: &disabled}); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if _, _, err := svc.LoginPassword(ctx, username, "directory-pw-1", "", ""); fault.KindOf(err) != fault.Invalid {
		t.Fatalf("disabled external login: %v", err)
	}

	if err := svc.DeleteUserByRef(ctx, username); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
}

func TestExternalUsernameCollision(t *testing.T) {
	pool := dbtest.Pool(t)
	svc := NewService(pool)
	ctx := context.Background()

	// A local account exists; a directory person maps to the same
	// username but a different identity.
	username := uniqUser("dave")
	if _, err := svc.CreateLocalUser(ctx, UserInput{Username: username, Password: "local-password-1"}); err != nil {
		t.Fatalf("create local: %v", err)
	}
	dir := &fakeDirectory{accounts: map[string]struct {
		password string
		ident    Identity
	}{
		// Different login name, but the directory asserts the colliding
		// username.
		"someone.else": {password: "dir-pw", ident: Identity{
			Username: username, ExternalID: "uid=someone.else,dc=example,dc=org",
		}},
	}}
	svc.SetExternalAuthenticator(dir)

	// The local user keeps working, with the local password only — the
	// provider pinned on the row decides the verifier.
	if _, _, err := svc.LoginPassword(ctx, username, "local-password-1", "", ""); err != nil {
		t.Fatalf("local login: %v", err)
	}
	if _, _, err := svc.LoginPassword(ctx, username, "dir-pw", "", ""); fault.KindOf(err) != fault.Invalid {
		t.Fatalf("directory password on local account: %v", err)
	}

	// JIT provisioning onto a taken username is refused.
	if _, _, err := svc.LoginPassword(ctx, "someone.else", "dir-pw", "", ""); fault.KindOf(err) != fault.Conflict {
		t.Fatalf("collision: got %v, want Conflict", err)
	}

	if err := svc.DeleteUserByRef(ctx, username); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
}

func TestLoginOIDCJIT(t *testing.T) {
	pool := dbtest.Pool(t)
	svc := NewService(pool)
	ctx := context.Background()

	username := uniqUser("erin")
	ident := Identity{
		Username: username, DisplayName: "Erin", Email: "erin@example.org",
		ExternalID: "sub-" + username, IsAdmin: boolPtr(true),
	}
	if _, _, err := svc.LoginOIDC(ctx, ident, "", ""); err != nil {
		t.Fatalf("oidc login: %v", err)
	}
	u, err := svc.GetUserByRef(ctx, username)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if u.Provider != ProviderOIDC || !u.IsAdmin || u.ExternalID != ident.ExternalID {
		t.Fatalf("oidc JIT wrong: %+v", u)
	}

	// OIDC users cannot password-login.
	if _, _, err := svc.LoginPassword(ctx, username, "whatever-pw-12", "", ""); fault.KindOf(err) != fault.Invalid {
		t.Fatalf("password login for oidc user: %v", err)
	}

	if err := svc.DeleteUserByRef(ctx, username); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
}
