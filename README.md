# Warren

Infrastructure resource modeling — datacenter infrastructure management
(DCIM) and IP address management (IPAM) — built with Go, HTMX, and
PostgreSQL.

Warren plays in the same space as [NetBox](https://netbox.dev) and aims for
feature parity over time, but deliberately redesigns the data model to fix
long-standing quirks (identifier sprawl, globally-unique slugs, duplicated
component models). It is built from day one to run as stateless container
replicas with PostgreSQL as the only stateful service.

**Status: Phase 2 — auth & access (complete).** Phase 1 shipped the
organization layer: hierarchical tenants (every level directly
assignable), sites and site groups (one nestable tree with region/group
kinds), nested location trees with scoped slugs and path addressing, a
transactional change log, and the REST API + HTMX UI for all of it.
Phase 2 adds authentication ([design](docs/design/0002-auth.md)): local
accounts (argon2id), LDAP (search-then-bind with attribute and
admin-group mapping), and OIDC SSO (code flow + PKCE with claim
mapping) — external users are JIT-provisioned and re-synced at every
login. DB-backed sessions keep replicas stateless; CSRF guards the UI;
the API takes bearer tokens only; the change log attributes every
mutation to the acting user. Next: Phase 3 — racks & devices. See the
[roadmap](docs/design/0001-foundations.md#7-roadmap).

## Design in one paragraph

Every object is identified by a **UUIDv7**, and human-addressable objects
also get exactly one **slug** — one format, one validator, scoped to the
object's natural parent rather than globally unique, addressed by slug
path (`cph-dc1/building-a/floor-2`). Generic references use a registered
**type key** plus ID (`dcim.site:0193a4f2-…`). One ID system, one slug
system, one reference syntax — nothing else. The full rules live in
[docs/design/0001-foundations.md](docs/design/0001-foundations.md).

## Stack

Go 1.26+ · chi · sqlc + pgx · templ · HTMX (vendored + embedded) ·
PostgreSQL 16+ supported, 18 shipped in the compose stack · `log/slog`
JSON logs. Generated code is committed, so a fresh clone builds with only
the Go toolchain.

## Quickstart

```sh
# everything in containers
docker compose up --build
# → http://localhost:8080  (probes: /healthz, /readyz)

# or from source against your own Postgres
export WARREN_DATABASE_URL=postgres://user:pass@localhost:5432/warren
go run ./cmd/warren serve     # auto-migrates by default
go run ./cmd/warren migrate   # or migrate explicitly (HA release step)

# bootstrap the first administrator (password on stdin), then sign in
go run ./cmd/warren user create -username admin -admin
```

Configuration is environment-only:

| Variable                | Default | Purpose                                  |
|-------------------------|---------|------------------------------------------|
| `WARREN_LISTEN`         | `:8080` | HTTP listen address                       |
| `WARREN_DATABASE_URL`   | —       | PostgreSQL DSN (required)                 |
| `WARREN_AUTO_MIGRATE`   | `true`  | apply migrations on boot (off for HA; run `warren migrate` instead) |
| `WARREN_COOKIE_SECURE`  | `false` | mark session cookies Secure (enable behind HTTPS) |
| `WARREN_SHUTDOWN_GRACE` | `15s`   | drain window after SIGTERM                |

LDAP login (optional — users are JIT-provisioned on first login and
re-synced on every login):

| Variable                   | Default     | Purpose                         |
|----------------------------|-------------|---------------------------------|
| `WARREN_LDAP_URL`          | —           | `ldap://host:389` or `ldaps://host:636` (enables LDAP) |
| `WARREN_LDAP_START_TLS`    | `false`     | upgrade plain connections before binding |
| `WARREN_LDAP_BIND_DN` / `WARREN_LDAP_BIND_PASSWORD` | — | service account for the user search (empty = anonymous) |
| `WARREN_LDAP_BASE_DN`      | —           | subtree searched for users (required) |
| `WARREN_LDAP_USER_FILTER`  | `(uid=%s)`  | user lookup filter (`(sAMAccountName=%s)` for AD) |
| `WARREN_LDAP_ATTR_USERNAME/_NAME/_EMAIL` | `uid`/`cn`/`mail` | attribute mapping |
| `WARREN_LDAP_ADMIN_GROUP`  | —           | group DN whose members get admin (synced per login) |

OIDC SSO (optional — adds a "Sign in with …" button):

| Variable                     | Default              | Purpose             |
|------------------------------|----------------------|---------------------|
| `WARREN_OIDC_ISSUER`         | —                    | IdP base URL (enables OIDC) |
| `WARREN_OIDC_CLIENT_ID` / `WARREN_OIDC_CLIENT_SECRET` | — | relying-party credentials |
| `WARREN_OIDC_REDIRECT_URL`   | —                    | `https://…/login/oidc/callback` (required) |
| `WARREN_OIDC_SCOPES`         | `profile email`      | scopes besides `openid` |
| `WARREN_OIDC_USERNAME_CLAIM` | `preferred_username` | claim mapping (also `_NAME_CLAIM`, `_EMAIL_CLAIM`) |
| `WARREN_OIDC_GROUPS_CLAIM` / `WARREN_OIDC_ADMIN_GROUP` | `groups` / — | group that grants admin |
| `WARREN_OIDC_NAME`           | `SSO`                | button label on the login page |

## API

`/api/v1` speaks JSON and authenticates with bearer tokens (create one
under *API tokens* in the UI). Reference segments accept an object ID
(UUIDv7) or the slug form — a slug path for parent-scoped types:

```sh
export TOKEN=wrt_…
curl -s -H "Authorization: Bearer $TOKEN" localhost:8080/api/v1/tenancy/tenants -d '{"name": "Lund Networks"}'
curl -s -H "Authorization: Bearer $TOKEN" localhost:8080/api/v1/dcim/sites -d '{"name": "Copenhagen DC 1", "tenant": "lund-networks"}'
curl -s -H "Authorization: Bearer $TOKEN" localhost:8080/api/v1/dcim/locations -d '{"site": "copenhagen-dc-1", "name": "Building A", "kind": "building"}'
curl -s -H "Authorization: Bearer $TOKEN" localhost:8080/api/v1/dcim/locations/copenhagen-dc-1/building-a
curl -s -H "Authorization: Bearer $TOKEN" localhost:8080/api/v1/changelog
```

Lists return `{"items": …, "total": …, "limit": …, "offset": …}`; errors
return `{"error": {"code": …, "message": …}}`.

## Development

```sh
make generate   # regenerate templ + sqlc output
make check      # build + vet + test (unit tests only without a database)
make run        # run the server locally
make image      # build the container image
```

Database-backed integration tests run when `WARREN_TEST_DATABASE_URL`
points at a disposable PostgreSQL 16+ database, and skip otherwise:

```sh
export WARREN_TEST_DATABASE_URL=postgres://user:pass@localhost:5432/warren_test
go test ./...
```

## Repository layout

```
cmd/warren/            entrypoint (serve | migrate | version)
internal/core/         identity kernel: id, slug, objtype, fault
internal/config/       env configuration
internal/db/           pool, embedded migrations, sqlc queries (gen/)
internal/changelog/    transactional change log
internal/tenancy/      tenants
internal/dcim/         sites, locations
internal/api/          REST API v1
internal/server/       router, middleware, probes, UI handlers, lifecycle
internal/web/          templ components + embedded assets
docs/design/           numbered design documents
```
