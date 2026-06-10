# 0001 — Foundations

Status: **accepted** · 2026-06-10

This document records Warren's founding decisions: what it is, the stack,
the identification system, the deployment model, and the order in which the
domains will be built. Domain-specific data model design (DCIM, IPAM, …)
gets its own numbered document when that domain is started; this document
only fixes the rules those designs must follow.

## 1. What Warren is

Warren is an infrastructure resource modeling application — datacenter
infrastructure management (DCIM) and IP address management (IPAM) — in the
same problem space as [NetBox](https://netbox.dev), with feature parity as
the long-term goal. Warren is **not** a NetBox fork or a schema-compatible
clone: the data model is redesigned where NetBox's has accumulated quirks,
and compatibility is provided through import tooling rather than schema
mirroring.

Target feature surface (long term): DCIM (sites, racks, devices, device
types, modules, cabling, power), IPAM (VRFs, prefixes, IP addresses,
VLANs, services), circuits, virtualization, wireless, VPN/tunnels, tenancy,
and the platform features that make NetBox sticky — REST API, change
logging, journaling, tags, custom fields, webhooks/event rules, RBAC,
reports/export.

## 2. Principles

1. **PostgreSQL is the only stateful component.** Application replicas are
   identical, stateless, and disposable. Anything that must survive a
   restart lives in Postgres — sessions, jobs, attachments, all of it.
2. **One way to identify anything.** Every object has a UUIDv7. Types that
   humans address by name additionally have exactly one slug under one
   uniform format and scoping rule. There is no third mechanism.
3. **Server-rendered UI.** HTML over the wire with HTMX for interactivity.
   No SPA, no client-side state store, no separate frontend build pipeline
   beyond templ.
4. **The API is a peer of the UI.** Every capability lands in the REST API;
   the UI consumes the same service layer. Automation is a first-class
   consumer, as it is for NetBox.
5. **Cross-cutting features are designed in, not bolted on.** Change
   logging, tenancy, and tagging hooks exist from the first domain model,
   because retrofitting them is what made some of NetBox's seams visible.
6. **Generated code is committed.** `templ generate` and `sqlc generate`
   outputs are checked in: `go build ./...` from a fresh clone must work
   with no tools installed.

## 3. Stack

| Concern    | Choice                                | Notes |
|------------|---------------------------------------|-------|
| Language   | Go (1.26+)                            | single static binary |
| Database   | PostgreSQL 16+ only (ship images track latest, currently 18) | native `inet`/`cidr` for IPAM, JSONB for custom fields, advisory locks, `SKIP LOCKED`, `LISTEN/NOTIFY` |
| Routing    | chi                                   | stdlib-compatible middleware |
| Data layer | sqlc over pgx                         | hand-written SQL, generated type-safe Go; no ORM |
| Templates  | templ                                 | compiled, type-checked components; pairs with HTMX |
| Frontend   | HTMX (vendored, embedded)             | no CDN dependency, works air-gapped |
| Migrations | plain SQL, embedded, run via `warren migrate` or on boot | see §6 |
| Config     | environment variables (`WARREN_*`)    | 12-factor; no config files |
| Logging    | `log/slog` JSON to stdout             | container-native |

Deliberately **not** chosen: ORMs (the gnarly queries — prefix containment,
cable tracing — want real SQL), external caches/queues (Postgres covers
both at Warren's scale; revisit only with evidence), Node-based asset
pipelines.

## 4. Identification: IDs, slugs, type keys, references

This section is the normative answer to "how do I point at a thing in
Warren" — for URLs, API payloads, change logs, webhooks, and import files.

### 4.1 IDs

- Every row of every object table has an `id uuid` primary key, generated
  as **UUIDv7** at creation, immutable forever.
- UUIDv7 is time-ordered (index-friendly, sorts by creation) and globally
  unique across instances, so exports, imports, restores, and
  staging→production promotions never renumber anything.
- The canonical text form is the lowercase 36-character hyphenated form.
  Nothing else (braces, URN prefix, bare hex) is accepted.

### 4.2 Slugs

- A slug is `[a-z0-9]` groups joined by single hyphens, 1–64 bytes:
  regex `^[a-z0-9]+(?:-[a-z0-9]+)*$`. No leading/trailing/double hyphens,
  no underscores, no uppercase, no unicode.
- A slug must **not** match the canonical UUID shape. This makes every
  reference unambiguous: parse as UUID → it's an ID; otherwise → slug.
- One generator (`slug.Make`) derives suggestions from display names
  (transliteration + NFKD fold: "Røde Æbler" → "rode-aebler"); users may
  edit the suggestion. One validator (`slug.Validate`) is used everywhere.
- Slugs are mutable (renames happen) but discouraged; the ID is the durable
  reference. A slug-history/redirect table is future work, not v1.
- Only human-addressed types get slugs (sites, tenants, roles,
  manufacturers, device types, …). High-cardinality machine-named objects
  (IP addresses, cables, interfaces) are addressed by ID or by their
  natural display form — they do not grow per-type identifier schemes.

### 4.3 Scoping and path addressing

NetBox makes every slug globally unique per type, which forces awkward
manual prefixing ("cph-dc1-row-1") the moment a name naturally repeats.
Warren instead gives every sluggable type exactly one declared **scope**:

- `global` — slug unique across the type (e.g. tenants, manufacturers).
- `parent` — slug unique within one named parent object (e.g. a location's
  slug is unique within its site; a device type's within its manufacturer).

The human-readable address of an object is its **slug path**: the slugs
from its scope root to itself, joined with `/`. Two sites can both contain
`row-1`; their addresses are `cph-dc1/row-1` and `aar-dc2/row-1`. For
nested trees (locations within locations) the path walks every level:
`cph-dc1/floor-2/row-1`.

Uniqueness is enforced in the schema: `UNIQUE (parent_id, slug)` for
parent-scoped types, `UNIQUE (slug)` for global ones. Resolution is a path
walk per segment — index-backed point lookups.

### 4.4 Object type keys

Every model registers a stable **type key** `domain.name`, both halves in
slug syntax: `dcim.site`, `dcim.device-type`, `ipam.prefix`. Type keys are
permanent once released — persisted data (change log entries, webhook
payloads, permission grants) depends on them.

The registry (`internal/core/objtype`) replaces Django's content-types
mechanism: anything generic — tags, custom fields, journal entries, change
log, webhooks, object permissions — refers to objects by **(type key,
ID)**, with the human form **(type key, slug path)** where useful:

```
dcim.location:0193a4f2-1c2e-7b3a-9f00-4d5e6a7b8c9d   (machine form)
dcim.location:cph-dc1/floor-2/row-1                   (human form)
```

### 4.5 URL conventions

```
UI:   /{domain}/{type-plural}/{ref}            /dcim/sites/cph-dc1
API:  /api/v1/{domain}/{type-plural}/{ref}     /api/v1/dcim/sites/0193a4f2-…
```

`{ref}` accepts an ID or a slug path (multi-segment for nested scopes) —
the UUID-shape exclusion in §4.2 makes this lossless. Both forms are
canonical; the API echoes both `id` and `slug_path` (where applicable) in
every representation.

## 5. Data model redesign directions

The quirks Warren fixes, and the direction for each. Details land in the
per-domain design docs; directions are fixed here so early schemas don't
paint over them.

1. **Identifier sprawl** → §4. One ID format, one slug system, one
   reference syntax, one type-key registry.
2. **Global slug uniqueness** → scoped slugs with path addressing (§4.3).
3. **Organizational sprawl** (Region, SiteGroup, Site, Location are four
   overlapping grouping mechanisms) → collapse to one nestable grouping
   tree above sites and one nestable location tree below them; "kind"
   labels (region/campus/building/floor/room/row) replace distinct models.
4. **Component model duplication** (8 near-identical component types × 8
   template types, each with copy-pasted machinery) → one unified
   `component` table with a `kind` discriminator plus kind-specific
   attribute tables, and a single template mechanism that instantiates any
   kind.
5. **Cable termination maze** (generic FKs to a dozen termination models)
   → terminations reference the unified component model; A/B ends are rows
   in one termination table, which also makes breakout cables natural.
6. **Django content-type generic FKs** → typed `(type key, ID)` references
   with referential integrity enforced by triggers against the registry.
7. **Custom field values in unvalidated JSON** → keep JSONB storage, add
   declared field schemas, server-side validation, and expression indexes
   for filterable fields.
8. **Inconsistent tenancy** → a uniform optional `tenant_id` on every
   asset-class object from day one, with one filtering semantic.
9. **Change log as a side effect** → an append-only event log written in
   the same transaction as the mutation; webhooks and the UI activity feed
   consume it via the outbox pattern (§6).
10. **IPAM containment confusion** (implicit, recomputed prefix hierarchy;
    duplicate-prefix ambiguity inside VRFs) → explicit, trigger-maintained
    containment links over native `cidr` ops; design doc due with the IPAM
    phase.

## 6. Deployment: containers and high availability

Warren is built to run as **N identical replicas of one container image
behind a load balancer, with PostgreSQL as the only stateful service**.
Two replicas on day one should be boring, not an achievement.

What that requires, concretely:

- **Stateless replicas.** No sticky sessions: session records live in
  Postgres, the cookie carries only a random token. No local disk: assets
  are embedded in the binary; uploads (device images, attachments) go to
  Postgres initially, with S3-compatible object storage as a later option.
- **Probes.** `/healthz` (liveness: process up) and `/readyz` (readiness:
  Postgres reachable **and** schema at the expected version — a replica
  built for schema N reports unready against schema N−1).
- **Graceful shutdown.** SIGTERM stops accepting, drains in-flight
  requests within `WARREN_SHUTDOWN_GRACE` (default 15s), exits. Rolling
  updates must not drop requests.
- **Safe migrations.** `warren migrate` runs embedded SQL migrations and
  takes a Postgres advisory lock, so concurrent replica startups
  serialize instead of racing. Recommended HA pattern: run migrations as a
  Job/init step, then roll replicas; auto-migrate-on-boot stays available
  for single-node convenience. Migrations must be backward-compatible one
  version back (expand → migrate → contract) so old and new replicas
  coexist mid-roll.
- **Background work without a queue service.** Jobs (webhook delivery,
  bulk imports, scheduled tasks) are rows claimed with
  `FOR UPDATE SKIP LOCKED`; every replica runs workers, so job throughput
  scales with replicas and a dead replica's jobs are simply re-claimed.
  Singleton schedules use advisory-lock leader election. Events that must
  not be lost follow the transactional outbox pattern: the mutation and
  its event row commit atomically, workers deliver after commit.
- **Cross-replica signaling.** `LISTEN/NOTIFY` for cache invalidation and
  (later) live UI updates via SSE. Any in-process cache must be
  correct-when-stale or invalidated by notification.
- **Container discipline.** Distroless, non-root, read-only-rootfs-safe
  image; config only via `WARREN_*` env vars; JSON logs to stdout;
  Prometheus `/metrics` in a later phase.

## 7. Roadmap

Each phase ships a usable vertical slice: schema + service layer + REST
API + HTMX UI, with change logging and tenancy hooks from Phase 1 onward.

- **Phase 0 — Foundations** *(this document)*: repo, stack, identity
  kernel (`id`, `slug`, `objtype`), HTTP skeleton with probes and graceful
  shutdown, container image, design doc.
- **Phase 1 — Platform spine + organization**: Postgres wiring (pgx +
  sqlc), migrations with advisory lock, sessions + local auth, the change
  log, tenants/tenant groups, site groups, sites, location tree. First
  real test of scoped slugs and path resolution.
- **Phase 2 — Racks & devices**: manufacturers, device types with the
  unified component-template model, racks (elevations), devices, modules,
  inventory.
- **Phase 3 — Connectivity**: the unified termination model, cables,
  end-to-end path tracing, power chains.
- **Phase 4 — IPAM**: VRFs/route targets, prefixes, IP addresses, VLANs,
  services; interface bindings; native-`cidr` containment design doc.
- **Phase 5 — Virtualization & circuits**: clusters, VMs, virtual
  interfaces; providers, circuits, terminations.
- **Phase 6 — Extensibility**: custom fields, tags (full UI), webhooks +
  event rules on the outbox, journaling, API tokens, RBAC, export
  templates, bulk import/export, NetBox import tooling.
- **Phase 7 — Long tail**: wireless, VPN/tunnels, Prometheus metrics,
  SSE live updates, plugin story, GraphQL (only if demand proves out).

## 8. Repository layout

```
cmd/warren/            main: serve | migrate (phase 1) | version
internal/config/       WARREN_* env config
internal/core/id/      UUIDv7 kernel
internal/core/slug/    slug validate/generate kernel
internal/core/objtype/ object type registry
internal/server/       chi router, middleware, probes, lifecycle
internal/web/          templ components + embedded static assets
internal/db/           (phase 1) pgx pool, sqlc queries, migrations
internal/<domain>/     (phase 1+) dcim, ipam, tenancy, …
docs/design/           numbered design documents (this series)
```

## 9. Open questions

Deferred deliberately; none block Phase 1.

- License (repo currently has none — decide before any public release).
- CSS approach: hand-rolled design system vs Tailwind standalone CLI.
- Auth providers beyond local accounts (OIDC is the likely first).
- Attachment storage: Postgres `bytea` first; S3-compatible when needed.
- GraphQL parity with NetBox: not planned unless a concrete need appears.
- Slug rename redirects/history.
