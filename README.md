# Warren

Infrastructure resource modeling — datacenter infrastructure management
(DCIM) and IP address management (IPAM) — built with Go, HTMX, and
PostgreSQL.

Warren plays in the same space as [NetBox](https://netbox.dev) and aims for
feature parity over time, but deliberately redesigns the data model to fix
long-standing quirks (identifier sprawl, globally-unique slugs, duplicated
component models). It is built from day one to run as stateless container
replicas with PostgreSQL as the only stateful service.

**Status: Phase 2 — auth & access (in progress).** Phase 1 shipped the
organization layer: hierarchical tenants (every level directly
assignable), sites and site groups (one nestable tree with region/group
kinds), nested location trees with scoped slugs and path addressing, a
transactional change log, and the REST API + HTMX UI for all of it.
Phase 2 has landed local authentication: argon2id passwords, DB-backed
sessions (replica-safe), CSRF protection, a login wall over the whole
app, bearer-token API auth, user management, and per-user API tokens —
with the change log now attributing every mutation to the acting user.
Still to come in Phase 2: LDAP and OIDC SSO providers (designed in
[docs/design/0002-auth.md](docs/design/0002-auth.md)). See the
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
