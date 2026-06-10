package tenancy

import (
	"context"
	"testing"

	"github.com/saku75/warren/internal/core/fault"
	"github.com/saku75/warren/internal/db/dbtest"
)

func TestTenantGroupTreeLifecycle(t *testing.T) {
	pool := dbtest.Pool(t)
	svc := NewService(pool)
	ctx := context.Background()

	// Root and child; the child's slug is scoped to the parent.
	rootSlug := uniq("emea")
	if _, err := svc.CreateGroup(ctx, GroupInput{Name: "EMEA " + rootSlug, Slug: rootSlug}); err != nil {
		t.Fatalf("create root: %v", err)
	}
	child, err := svc.CreateGroup(ctx, GroupInput{Name: "Denmark", Slug: "dk", ParentRef: rootSlug})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}

	// Same slug under a different scope is fine; duplicate in the same
	// scope conflicts.
	otherSlug := uniq("apac")
	if _, err := svc.CreateGroup(ctx, GroupInput{Name: "APAC " + otherSlug, Slug: otherSlug}); err != nil {
		t.Fatalf("create second root: %v", err)
	}
	if _, err := svc.CreateGroup(ctx, GroupInput{Name: "DK again", Slug: "dk", ParentRef: otherSlug}); err != nil {
		t.Fatalf("same slug other scope: %v", err)
	}
	if _, err := svc.CreateGroup(ctx, GroupInput{Name: "Dup", Slug: "dk", ParentRef: rootSlug}); fault.KindOf(err) != fault.Conflict {
		t.Fatalf("dup in scope: got %v, want Conflict", err)
	}

	// Path resolution and path rendering.
	got, err := svc.GetGroupByRef(ctx, rootSlug+"/dk")
	if err != nil || got.ID != child.ID {
		t.Fatalf("resolve path: %+v, %v", got, err)
	}
	path, err := svc.GroupPath(ctx, child.ID)
	if err != nil || path != rootSlug+"/dk" {
		t.Fatalf("path = %q, %v", path, err)
	}

	// Tenant membership by path ref.
	tenant, err := svc.Create(ctx, Input{Name: "T " + uniq("t"), GroupRef: rootSlug + "/dk"})
	if err != nil {
		t.Fatalf("tenant with group: %v", err)
	}
	if tenant.TenantGroupID == nil || *tenant.TenantGroupID != child.ID {
		t.Fatalf("tenant group not linked: %+v", tenant)
	}
	members, err := svc.TenantsInGroup(ctx, child.ID)
	if err != nil || len(members) != 1 {
		t.Fatalf("members = %d, %v", len(members), err)
	}

	// Group with members or children cannot be deleted.
	if err := svc.DeleteGroupByRef(ctx, rootSlug+"/dk"); fault.KindOf(err) != fault.Conflict {
		t.Fatalf("delete with members: got %v, want Conflict", err)
	}
	if err := svc.DeleteGroupByRef(ctx, rootSlug); fault.KindOf(err) != fault.Conflict {
		t.Fatalf("delete with children: got %v, want Conflict", err)
	}

	// Reparent cycle is rejected: root cannot move under its child.
	childPath := rootSlug + "/dk"
	if _, err := svc.UpdateGroupByRef(ctx, rootSlug, GroupUpdate{ParentRef: &childPath}); fault.KindOf(err) != fault.Invalid {
		t.Fatalf("cycle: got %v, want Invalid", err)
	}

	// Clearing the tenant's group frees the leaf for deletion.
	empty := ""
	if _, err := svc.UpdateByRef(ctx, tenant.Slug, Update{GroupRef: &empty}); err != nil {
		t.Fatalf("clear tenant group: %v", err)
	}
	if err := svc.DeleteGroupByRef(ctx, childPath); err != nil {
		t.Fatalf("delete leaf: %v", err)
	}
	if err := svc.DeleteGroupByRef(ctx, rootSlug); err != nil {
		t.Fatalf("delete root: %v", err)
	}

	// Tree assembly includes the remaining root.
	roots, err := svc.GroupTree(ctx)
	if err != nil {
		t.Fatalf("tree: %v", err)
	}
	for _, r := range roots {
		if r.Item.Slug == rootSlug {
			t.Fatalf("deleted root still in tree")
		}
		if r.Item.Slug == otherSlug && (len(r.Children) != 1 || r.Children[0].Path != otherSlug+"/dk") {
			t.Fatalf("other root children wrong: %+v", r.Children)
		}
	}
}
