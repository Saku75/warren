# Warren

Infrastructure resource modeling — datacenter infrastructure management
(DCIM) and IP address management (IPAM) — built with Go, HTMX, and
PostgreSQL.

Warren plays in the same space as [NetBox](https://netbox.dev) and aims for
feature parity over time, but deliberately redesigns the data model to fix
long-standing quirks (identifier sprawl, globally-unique slugs, duplicated
component models). It is built from day one to run as stateless container
replicas with PostgreSQL as the only stateful service.

**Status: Phase 0 — foundations.** The identity kernel, HTTP skeleton, and
design docs exist; no domain models yet. See the
[roadmap](docs/design/0001-foundations.md#7-roadmap).

## Design in one paragraph

Every object is identified by a **UUIDv7**, and human-addressable objects
also get exactly one **slug** — one format, one validator, scoped to the
object's natural parent rather than globally unique, addressed by slug
path (`cph-dc1/floor-2/row-1`). Generic references use a registered **type
key** plus ID (`dcim.site:0193a4f2-…`). One ID system, one slug system,
one reference syntax — nothing else. The full rules live in
[docs/design/0001-foundations.md](docs/design/0001-foundations.md).

## Stack

Go 1.24+ · chi · sqlc + pgx · templ · HTMX (vendored + embedded) ·
PostgreSQL 16+ · `log/slog` JSON logs. Generated code is committed, so a
fresh clone builds with only the Go toolchain.

## Quickstart

```sh
# run from source
go run ./cmd/warren serve
# → http://localhost:8080  (probes: /healthz, /readyz)

# or in a container
docker compose up --build
```

Configuration is environment-only:

| Variable                | Default | Purpose                          |
|-------------------------|---------|----------------------------------|
| `WARREN_LISTEN`         | `:8080` | HTTP listen address              |
| `WARREN_DATABASE_URL`   | —       | PostgreSQL DSN (phase 1)         |
| `WARREN_SHUTDOWN_GRACE` | `15s`   | drain window after SIGTERM       |

## Development

```sh
make generate   # regenerate templ (and later sqlc) output
make check      # build + vet + test
make run        # run the server locally
make image      # build the container image
```

## Repository layout

```
cmd/warren/            entrypoint (serve | version)
internal/core/         identity kernel: id, slug, objtype
internal/config/       env configuration
internal/server/       router, middleware, probes, lifecycle
internal/web/          templ components + embedded assets
docs/design/           numbered design documents
```
