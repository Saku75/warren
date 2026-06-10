package db

import (
	"context"
	"os"
	"testing"
)

func TestMigrationsWellFormed(t *testing.T) {
	ms := Migrations()
	if len(ms) == 0 {
		t.Fatal("no embedded migrations")
	}
	if ms[0].Version != 1 {
		t.Fatalf("first migration version = %d, want 1", ms[0].Version)
	}
	for i := 1; i < len(ms); i++ {
		if ms[i].Version != ms[i-1].Version+1 {
			t.Fatalf("migration versions not contiguous: %d then %d", ms[i-1].Version, ms[i].Version)
		}
	}
	if LatestVersion() != ms[len(ms)-1].Version {
		t.Fatalf("LatestVersion() = %d, want %d", LatestVersion(), ms[len(ms)-1].Version)
	}
}

func TestMigrateIsIdempotentAndReady(t *testing.T) {
	url := os.Getenv("WARREN_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("WARREN_TEST_DATABASE_URL not set; skipping database integration test")
	}
	ctx := context.Background()
	pool, err := Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	if _, err := Migrate(ctx, pool); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	again, err := Migrate(ctx, pool)
	if err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("second migrate applied %v, want nothing", again)
	}

	v, err := SchemaVersion(ctx, pool)
	if err != nil {
		t.Fatalf("schema version: %v", err)
	}
	if v != LatestVersion() {
		t.Fatalf("schema version = %d, want %d", v, LatestVersion())
	}
	if err := Ready(ctx, pool); err != nil {
		t.Fatalf("Ready: %v", err)
	}
}
