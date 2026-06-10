package tenancy

import (
	"context"
	"fmt"
	"testing"

	"github.com/saku75/warren/internal/changelog"
	"github.com/saku75/warren/internal/core/fault"
	"github.com/saku75/warren/internal/core/id"
	"github.com/saku75/warren/internal/db/dbtest"
	"github.com/saku75/warren/internal/db/gen"
)

// uniq makes slugs unique across test runs sharing one database.
func uniq(prefix string) string {
	u := id.New().String()
	return fmt.Sprintf("%s-%s", prefix, u[len(u)-12:])
}

func TestTenantLifecycle(t *testing.T) {
	pool := dbtest.Pool(t)
	svc := NewService(pool)
	ctx := context.Background()

	name := "Acme " + uniq("corp")

	// Create with derived slug.
	created, err := svc.Create(ctx, Input{Name: name, Description: "test tenant"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Slug == "" || created.ID == id.Nil {
		t.Fatalf("create returned incomplete tenant: %+v", created)
	}

	// Resolve by both reference forms.
	bySlug, err := svc.GetByRef(ctx, created.Slug)
	if err != nil {
		t.Fatalf("get by slug: %v", err)
	}
	byID, err := svc.GetByRef(ctx, created.ID.String())
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if bySlug.ID != created.ID || byID.ID != created.ID {
		t.Fatal("reference forms resolved different tenants")
	}

	// Duplicate slug conflicts.
	if _, err := svc.Create(ctx, Input{Name: "Other", Slug: created.Slug}); fault.KindOf(err) != fault.Conflict {
		t.Fatalf("duplicate slug: got %v, want Conflict", err)
	}

	// Partial update: description only; slug must survive.
	desc := "updated"
	updated, err := svc.UpdateByRef(ctx, created.Slug, Update{Description: &desc})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Description != "updated" || updated.Slug != created.Slug {
		t.Fatalf("update wrong result: %+v", updated)
	}

	// Changelog has create + update for this object.
	clog := changelog.NewService(gen.New(pool))
	entries, err := clog.ListForObject(ctx, TypeTenant.Key, created.ID, 10, 0)
	if err != nil {
		t.Fatalf("changelog: %v", err)
	}
	if len(entries) != 2 || entries[0].Action != changelog.ActionUpdate || entries[1].Action != changelog.ActionCreate {
		t.Fatalf("changelog entries wrong: %+v", entries)
	}
	if entries[1].DataAfter == nil || entries[1].DataBefore != nil {
		t.Fatalf("create entry snapshots wrong: %+v", entries[1])
	}

	// Delete, then it is gone.
	if err := svc.DeleteByRef(ctx, created.Slug); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.GetByRef(ctx, created.Slug); fault.KindOf(err) != fault.NotFound {
		t.Fatalf("after delete: got %v, want NotFound", err)
	}

	// Changelog survives the object.
	entries, err = clog.ListForObject(ctx, TypeTenant.Key, created.ID, 10, 0)
	if err != nil || len(entries) != 3 {
		t.Fatalf("changelog after delete: %v entries, err %v", len(entries), err)
	}
}

func TestTenantValidation(t *testing.T) {
	pool := dbtest.Pool(t)
	svc := NewService(pool)
	ctx := context.Background()

	if _, err := svc.Create(ctx, Input{Name: ""}); fault.KindOf(err) != fault.Invalid {
		t.Errorf("empty name: got %v, want Invalid", err)
	}
	if _, err := svc.Create(ctx, Input{Name: "X", Slug: "Bad_Slug"}); fault.KindOf(err) != fault.Invalid {
		t.Errorf("bad slug: got %v, want Invalid", err)
	}
	if _, err := svc.GetByRef(ctx, "no-such-"+uniq("t")); fault.KindOf(err) != fault.NotFound {
		t.Errorf("missing ref: want NotFound")
	}
}
