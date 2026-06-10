package dcim

import (
	"context"
	"fmt"
	"testing"

	"github.com/saku75/warren/internal/core/fault"
	"github.com/saku75/warren/internal/core/id"
	"github.com/saku75/warren/internal/db/dbtest"
	"github.com/saku75/warren/internal/tenancy"
)

func uniq(prefix string) string {
	u := id.New().String()
	return fmt.Sprintf("%s-%s", prefix, u[len(u)-12:])
}

func TestSiteLifecycleWithTenant(t *testing.T) {
	pool := dbtest.Pool(t)
	svc := NewService(pool)
	tsvc := tenancy.NewService(pool)
	ctx := context.Background()

	tenant, err := tsvc.Create(ctx, tenancy.Input{Name: "Tenant " + uniq("x")})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	siteSlug := uniq("cph-dc")
	site, err := svc.CreateSite(ctx, SiteInput{
		Name:      "Copenhagen " + siteSlug,
		Slug:      siteSlug,
		TenantRef: tenant.Slug,
	})
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if site.Status != "active" {
		t.Fatalf("default status = %q, want active", site.Status)
	}
	if site.TenantID == nil || *site.TenantID != tenant.ID {
		t.Fatalf("tenant not linked: %+v", site)
	}

	// Tenant deletion is blocked while the site references it.
	if err := tsvc.DeleteByRef(ctx, tenant.Slug); fault.KindOf(err) != fault.Conflict {
		t.Fatalf("tenant delete while referenced: got %v, want Conflict", err)
	}

	// Clear the tenant via partial update, then deletion works.
	empty := ""
	if _, err := svc.UpdateSiteByRef(ctx, siteSlug, SiteUpdate{TenantRef: &empty}); err != nil {
		t.Fatalf("clear tenant: %v", err)
	}
	if err := tsvc.DeleteByRef(ctx, tenant.Slug); err != nil {
		t.Fatalf("tenant delete after clearing: %v", err)
	}

	if _, err := svc.CreateSite(ctx, SiteInput{Name: "Bad", Slug: siteSlug}); fault.KindOf(err) != fault.Conflict {
		t.Fatalf("duplicate site slug: got %v, want Conflict", err)
	}
	if _, err := svc.CreateSite(ctx, SiteInput{Name: "Bad", Status: "bogus"}); fault.KindOf(err) != fault.Invalid {
		t.Fatalf("bogus status: got %v, want Invalid", err)
	}
}

func TestLocationTreeAndPaths(t *testing.T) {
	pool := dbtest.Pool(t)
	svc := NewService(pool)
	ctx := context.Background()

	siteA := uniq("site-a")
	siteB := uniq("site-b")
	for _, s := range []string{siteA, siteB} {
		if _, err := svc.CreateSite(ctx, SiteInput{Name: "Site " + s, Slug: s}); err != nil {
			t.Fatalf("create site %s: %v", s, err)
		}
	}

	// Build siteA: building-a > floor-1 > row-1
	bldg, err := svc.CreateLocation(ctx, LocationInput{
		SiteRef: siteA, Name: "Building A", Slug: "building-a", Kind: "building",
	})
	if err != nil {
		t.Fatalf("create building: %v", err)
	}
	floor, err := svc.CreateLocation(ctx, LocationInput{
		SiteRef: siteA, ParentRef: "building-a", Name: "Floor 1", Slug: "floor-1", Kind: "floor",
	})
	if err != nil {
		t.Fatalf("create floor: %v", err)
	}
	if _, err := svc.CreateLocation(ctx, LocationInput{
		SiteRef: siteA, ParentRef: bldg.ID.String(), Name: "Row 1", Slug: "row-1", Kind: "row",
	}); err != nil {
		t.Fatalf("create row under building by id: %v", err)
	}

	// THE scoped-slug payoff: same slug under a different scope is fine…
	if _, err := svc.CreateLocation(ctx, LocationInput{
		SiteRef: siteB, Name: "Building A", Slug: "building-a", Kind: "building",
	}); err != nil {
		t.Fatalf("same slug in other site should work: %v", err)
	}
	if _, err := svc.CreateLocation(ctx, LocationInput{
		SiteRef: siteA, ParentRef: "building-a/floor-1", Name: "Row 1", Slug: "row-1", Kind: "row",
	}); err != nil {
		t.Fatalf("same slug under different parent should work: %v", err)
	}
	// …but a duplicate in the same scope conflicts.
	if _, err := svc.CreateLocation(ctx, LocationInput{
		SiteRef: siteA, ParentRef: "building-a", Name: "Floor 1 again", Slug: "floor-1",
	}); fault.KindOf(err) != fault.Conflict {
		t.Fatalf("duplicate slug in same scope: got %v, want Conflict", err)
	}

	// Path resolution end to end.
	got, err := svc.GetLocationByRef(ctx, siteA+"/building-a/floor-1")
	if err != nil {
		t.Fatalf("resolve path: %v", err)
	}
	if got.ID != floor.ID {
		t.Fatal("path resolved to wrong location")
	}
	path, err := svc.LocationPath(ctx, floor.ID)
	if err != nil {
		t.Fatalf("location path: %v", err)
	}
	if want := siteA + "/building-a/floor-1"; path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	if _, err := svc.GetLocationByRef(ctx, siteA+"/building-a/missing"); fault.KindOf(err) != fault.NotFound {
		t.Fatalf("missing path: got %v, want NotFound", err)
	}

	// Cross-site parent is rejected.
	if _, err := svc.CreateLocation(ctx, LocationInput{
		SiteRef: siteB, ParentRef: floor.ID.String(), Name: "Bad", Slug: "bad",
	}); fault.KindOf(err) != fault.Invalid {
		t.Fatalf("cross-site parent: got %v, want Invalid", err)
	}

	// Cycle rejection: building-a cannot move under its own child row-1.
	newParent := "building-a/row-1"
	if _, err := svc.UpdateLocationByRef(ctx, siteA+"/building-a", LocationUpdate{ParentRef: &newParent}); fault.KindOf(err) != fault.Invalid {
		t.Fatalf("cycle: got %v, want Invalid", err)
	}

	// Delete with children blocks; leaf delete works.
	if err := svc.DeleteLocationByRef(ctx, siteA+"/building-a"); fault.KindOf(err) != fault.Conflict {
		t.Fatalf("delete with children: got %v, want Conflict", err)
	}
	if err := svc.DeleteLocationByRef(ctx, siteA+"/building-a/floor-1/row-1"); err != nil {
		t.Fatalf("delete leaf: %v", err)
	}

	// Tree assembly.
	site, err := svc.GetSiteByRef(ctx, siteA)
	if err != nil {
		t.Fatalf("get site: %v", err)
	}
	tree, err := svc.LocationTree(ctx, site.ID)
	if err != nil {
		t.Fatalf("tree: %v", err)
	}
	if len(tree) != 1 || tree[0].Location.Slug != "building-a" {
		t.Fatalf("tree roots wrong: %+v", tree)
	}
	if len(tree[0].Children) != 2 { // floor-1 and row-1 under building
		t.Fatalf("building children = %d, want 2", len(tree[0].Children))
	}
	if tree[0].Path != siteA+"/building-a" {
		t.Fatalf("tree path = %q", tree[0].Path)
	}

	// Site delete cascades the remaining locations.
	if err := svc.DeleteSiteByRef(ctx, siteA); err != nil {
		t.Fatalf("delete site: %v", err)
	}
	if _, err := svc.GetLocationByRef(ctx, siteA+"/building-a"); fault.KindOf(err) != fault.NotFound {
		t.Fatalf("locations should cascade with site: %v", err)
	}
}
