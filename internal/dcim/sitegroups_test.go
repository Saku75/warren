package dcim

import (
	"context"
	"testing"

	"github.com/saku75/warren/internal/core/fault"
	"github.com/saku75/warren/internal/db/dbtest"
)

func TestSiteGroupsAndMembership(t *testing.T) {
	pool := dbtest.Pool(t)
	svc := NewService(pool)
	ctx := context.Background()

	// Region root with a nested group; kind validation.
	regionSlug := uniq("emea")
	region, err := svc.CreateSiteGroup(ctx, SiteGroupInput{Name: "EMEA " + regionSlug, Slug: regionSlug, Kind: "region"})
	if err != nil {
		t.Fatalf("create region: %v", err)
	}
	if _, err := svc.CreateSiteGroup(ctx, SiteGroupInput{Name: "Bad", Kind: "continent"}); fault.KindOf(err) != fault.Invalid {
		t.Fatalf("bad kind: got %v, want Invalid", err)
	}
	colo, err := svc.CreateSiteGroup(ctx, SiteGroupInput{Name: "Colo", Slug: "colo", ParentRef: regionSlug})
	if err != nil {
		t.Fatalf("create child group: %v", err)
	}
	if colo.Kind != "group" {
		t.Fatalf("default kind = %q, want group", colo.Kind)
	}

	// Site joins the group by path ref; list rows carry group display
	// fields.
	siteSlug := uniq("cph")
	site, err := svc.CreateSite(ctx, SiteInput{Name: "CPH " + siteSlug, Slug: siteSlug, GroupRef: regionSlug + "/colo"})
	if err != nil {
		t.Fatalf("create site with group: %v", err)
	}
	if site.SiteGroupID == nil || *site.SiteGroupID != colo.ID {
		t.Fatalf("site group not linked: %+v", site)
	}
	members, err := svc.SitesInGroup(ctx, colo.ID)
	if err != nil || len(members) != 1 || members[0].ID != site.ID {
		t.Fatalf("members wrong: %v, %v", members, err)
	}
	rows, _, err := svc.ListSites(ctx, 200, 0)
	if err != nil {
		t.Fatalf("list sites: %v", err)
	}
	found := false
	for _, row := range rows {
		if row.ID == site.ID {
			found = true
			if row.GroupName == nil || *row.GroupName != "Colo" {
				t.Fatalf("list row group fields wrong: %+v", row)
			}
		}
	}
	if !found {
		t.Fatal("site missing from list")
	}

	// Deletes blocked while referenced, then cascade of cleanup.
	if err := svc.DeleteSiteGroupByRef(ctx, regionSlug+"/colo"); fault.KindOf(err) != fault.Conflict {
		t.Fatalf("delete group with site: got %v, want Conflict", err)
	}
	empty := ""
	if _, err := svc.UpdateSiteByRef(ctx, siteSlug, SiteUpdate{GroupRef: &empty}); err != nil {
		t.Fatalf("clear site group: %v", err)
	}
	if err := svc.DeleteSiteGroupByRef(ctx, regionSlug+"/colo"); err != nil {
		t.Fatalf("delete leaf group: %v", err)
	}
	if err := svc.DeleteSiteGroupByRef(ctx, region.ID.String()); err != nil {
		t.Fatalf("delete region by id: %v", err)
	}
	if err := svc.DeleteSiteByRef(ctx, siteSlug); err != nil {
		t.Fatalf("delete site: %v", err)
	}
}
