// Package dbtest provides PostgreSQL-backed test helpers. Integration
// tests are gated on WARREN_TEST_DATABASE_URL and skip without it, so the
// plain unit test run never needs a database.
package dbtest

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/saku75/warren/internal/db"
)

var migrateOnce sync.Once

// Pool connects to the test database, ensures migrations are applied, and
// returns a pool that closes with the test. Tests sharing the database
// must use randomized slugs rather than assuming emptiness.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("WARREN_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("WARREN_TEST_DATABASE_URL not set; skipping database integration test")
	}

	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		t.Fatalf("dbtest: connect: %v", err)
	}
	t.Cleanup(pool.Close)

	migrateOnce.Do(func() {
		if _, err := db.Migrate(ctx, pool); err != nil {
			t.Fatalf("dbtest: migrate: %v", err)
		}
	})
	return pool
}
