package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// migrationLockKey is the advisory lock serializing migration runs, so any
// number of replicas can start (or `warren migrate`) concurrently and
// exactly one applies each pending migration. Arbitrary but fixed:
// "warren" interpreted as a number.
const migrationLockKey int64 = 0x77617272656e

var migrationName = regexp.MustCompile(`^(\d{4})_([a-z0-9_]+)\.sql$`)

// Migration is one embedded schema migration.
type Migration struct {
	Version int32
	Name    string
	SQL     string
}

// Migrations returns the embedded migrations sorted by version. It panics
// on malformed or duplicate filenames, which can only happen at
// development time.
func Migrations() []Migration {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		panic("db: reading embedded migrations: " + err.Error())
	}
	seen := map[int32]string{}
	var out []Migration
	for _, e := range entries {
		m := migrationName.FindStringSubmatch(e.Name())
		if m == nil {
			panic(fmt.Sprintf("db: migration %q does not match NNNN_name.sql", e.Name()))
		}
		v, err := strconv.ParseInt(m[1], 10, 32)
		if err != nil || v == 0 {
			panic(fmt.Sprintf("db: migration %q has invalid version", e.Name()))
		}
		if prev, dup := seen[int32(v)]; dup {
			panic(fmt.Sprintf("db: migrations %q and %q share version %d", prev, e.Name(), v))
		}
		seen[int32(v)] = e.Name()
		body, err := fs.ReadFile(migrationFS, "migrations/"+e.Name())
		if err != nil {
			panic("db: reading migration " + e.Name() + ": " + err.Error())
		}
		out = append(out, Migration{Version: int32(v), Name: m[2], SQL: string(body)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out
}

// LatestVersion is the schema version this binary expects.
func LatestVersion() int32 {
	ms := Migrations()
	if len(ms) == 0 {
		return 0
	}
	return ms[len(ms)-1].Version
}

// Migrate applies all pending migrations and reports the versions it
// applied. The whole run holds a Postgres advisory lock, and each
// migration runs in its own transaction together with its
// schema_migrations bookkeeping row.
func Migrate(ctx context.Context, pool *pgxpool.Pool) ([]int32, error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("db: acquire conn: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockKey); err != nil {
		return nil, fmt.Errorf("db: take migration lock: %w", err)
	}
	defer conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", migrationLockKey)

	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    integer PRIMARY KEY,
			name       text NOT NULL,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`); err != nil {
		return nil, fmt.Errorf("db: ensure schema_migrations: %w", err)
	}

	applied := map[int32]bool{}
	rows, err := conn.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("db: read schema_migrations: %w", err)
	}
	versions, err := pgx.CollectRows(rows, pgx.RowTo[int32])
	if err != nil {
		return nil, fmt.Errorf("db: read schema_migrations: %w", err)
	}
	for _, v := range versions {
		applied[v] = true
	}

	var ran []int32
	for _, m := range Migrations() {
		if applied[m.Version] {
			continue
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return ran, fmt.Errorf("db: begin migration %04d: %w", m.Version, err)
		}
		// Simple protocol via PgConn so a migration file may contain any
		// number of statements; they all run inside this transaction.
		if _, err := tx.Conn().PgConn().Exec(ctx, m.SQL).ReadAll(); err != nil {
			_ = tx.Rollback(ctx)
			return ran, fmt.Errorf("db: apply migration %04d_%s: %w", m.Version, m.Name, err)
		}
		if _, err := tx.Exec(ctx,
			"INSERT INTO schema_migrations (version, name) VALUES ($1, $2)",
			m.Version, m.Name); err != nil {
			_ = tx.Rollback(ctx)
			return ran, fmt.Errorf("db: record migration %04d: %w", m.Version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return ran, fmt.Errorf("db: commit migration %04d: %w", m.Version, err)
		}
		ran = append(ran, m.Version)
	}
	return ran, nil
}

// SchemaVersion reports the highest applied migration version, or 0 when
// the schema_migrations table does not exist yet.
func SchemaVersion(ctx context.Context, pool *pgxpool.Pool) (int32, error) {
	var v int32
	err := pool.QueryRow(ctx, `
		SELECT COALESCE(max(version), 0) FROM schema_migrations
	`).Scan(&v)
	if err != nil {
		var exists bool
		if e2 := pool.QueryRow(ctx,
			"SELECT to_regclass('schema_migrations') IS NOT NULL").Scan(&exists); e2 == nil && !exists {
			return 0, nil
		}
		return 0, fmt.Errorf("db: schema version: %w", err)
	}
	return v, nil
}

// Ready returns nil when the database is reachable and the schema matches
// what this binary expects; the readiness probe gates on it so replicas
// built for a newer schema stay out of rotation until migrations run.
func Ready(ctx context.Context, pool *pgxpool.Pool) error {
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("db: ping: %w", err)
	}
	v, err := SchemaVersion(ctx, pool)
	if err != nil {
		return err
	}
	if want := LatestVersion(); v != want {
		return fmt.Errorf("db: schema at version %d, binary expects %d", v, want)
	}
	return nil
}
