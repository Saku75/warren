package tenancy

import (
	"context"
	"testing"

	"github.com/saku75/warren/internal/core/fault"
	"github.com/saku75/warren/internal/db/dbtest"
)

func TestTenantHierarchy(t *testing.T) {
	pool := dbtest.Pool(t)
	svc := NewService(pool)
	ctx := context.Background()

	// Parent and child by slug ref; slugs stay globally unique.
	parentSlug := uniq("acme")
	parent, err := svc.Create(ctx, Input{Name: "Acme " + parentSlug, Slug: parentSlug})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	childSlug := uniq("acme-dk")
	child, err := svc.Create(ctx, Input{Name: "Acme DK", Slug: childSlug, ParentRef: parentSlug})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	if child.ParentID == nil || *child.ParentID != parent.ID {
		t.Fatalf("child parent not linked: %+v", child)
	}

	// Global slug uniqueness still holds across levels.
	if _, err := svc.Create(ctx, Input{Name: "Dup", Slug: childSlug, ParentRef: ""}); fault.KindOf(err) != fault.Conflict {
		t.Fatalf("dup slug: got %v, want Conflict", err)
	}

	// List rows carry parent display fields.
	rows, _, err := svc.List(ctx, 500, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, row := range rows {
		if row.ID == child.ID {
			if row.ParentSlug == nil || *row.ParentSlug != parentSlug {
				t.Fatalf("list row parent fields wrong: %+v", row)
			}
		}
	}

	// Children and tree assembly.
	children, err := svc.Children(ctx, parent.ID)
	if err != nil || len(children) != 1 || children[0].ID != child.ID {
		t.Fatalf("children wrong: %v %v", children, err)
	}
	roots, err := svc.Tree(ctx)
	if err != nil {
		t.Fatalf("tree: %v", err)
	}
	foundParent := false
	for _, r := range roots {
		if r.Item.ID == parent.ID {
			foundParent = true
			if len(r.Children) != 1 || r.Children[0].Item.ID != child.ID {
				t.Fatalf("tree children wrong: %+v", r.Children)
			}
		}
		if r.Item.ID == child.ID {
			t.Fatal("child must not be a root")
		}
	}
	if !foundParent {
		t.Fatal("parent missing from tree roots")
	}

	// Cycle rejection: parent cannot move under its own child.
	if _, err := svc.UpdateByRef(ctx, parentSlug, Update{ParentRef: &childSlug}); fault.KindOf(err) != fault.Invalid {
		t.Fatalf("cycle: got %v, want Invalid", err)
	}
	selfRef := parentSlug
	if _, err := svc.UpdateByRef(ctx, parentSlug, Update{ParentRef: &selfRef}); fault.KindOf(err) != fault.Invalid {
		t.Fatalf("self-parent: got %v, want Invalid", err)
	}

	// Delete blocked while children exist; clearing the parent unblocks.
	if err := svc.DeleteByRef(ctx, parentSlug); fault.KindOf(err) != fault.Conflict {
		t.Fatalf("delete with children: got %v, want Conflict", err)
	}
	empty := ""
	if _, err := svc.UpdateByRef(ctx, childSlug, Update{ParentRef: &empty}); err != nil {
		t.Fatalf("clear parent: %v", err)
	}
	if err := svc.DeleteByRef(ctx, parentSlug); err != nil {
		t.Fatalf("delete parent: %v", err)
	}
	if err := svc.DeleteByRef(ctx, childSlug); err != nil {
		t.Fatalf("delete child: %v", err)
	}
}
