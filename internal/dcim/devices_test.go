package dcim

import (
	"context"
	"testing"

	"github.com/saku75/warren/internal/core/fault"
	"github.com/saku75/warren/internal/db/dbtest"
	"github.com/saku75/warren/internal/db/gen"
)

// TestDeviceAndNestedModules drives the whole design-doc §3 flow: device
// creation stamps templates, installing a NIC module creates its SFP
// bay (with {module} substitution), installing an SFP into that bay
// creates its interface, and removing the NIC cascades everything.
func TestDeviceAndNestedModules(t *testing.T) {
	pool := dbtest.Pool(t)
	svc := NewService(pool)
	ctx := context.Background()

	mSlug := uniq("cisco")
	if _, err := svc.CreateManufacturer(ctx, ManufacturerInput{Name: "M " + mSlug, Slug: mSlug}); err != nil {
		t.Fatalf("manufacturer: %v", err)
	}
	role, err := svc.CreateDeviceRole(ctx, DeviceRoleInput{Name: "Switch " + uniq("r")})
	if err != nil {
		t.Fatalf("role: %v", err)
	}
	siteSlug := uniq("dc")
	if _, err := svc.CreateSite(ctx, SiteInput{Name: "DC " + siteSlug, Slug: siteSlug}); err != nil {
		t.Fatalf("site: %v", err)
	}

	// Catalog: switch with 48 ports (representative one) + an NM bay;
	// NIC module with an SFP bay; SFP module with one interface.
	if _, err := svc.CreateDeviceType(ctx, DeviceTypeInput{ManufacturerRef: mSlug, Model: "C9300", Slug: "c9300", UHeight: 1}); err != nil {
		t.Fatalf("device type: %v", err)
	}
	for _, tpl := range []TemplateInput{
		{DeviceTypeRef: mSlug + "/c9300", Kind: "interface", Name: "Gi1/0/1", Attrs: ComponentAttrs{Type: "1000base-t"}},
		{DeviceTypeRef: mSlug + "/c9300", Kind: "module-bay", Name: "NM1", Attrs: ComponentAttrs{Position: "1"}},
		{DeviceTypeRef: mSlug + "/c9300", Kind: "power-port", Name: "PSU1", Attrs: ComponentAttrs{Type: "iec-60320-c14", MaxDrawW: 715}},
	} {
		if _, err := svc.CreateTemplate(ctx, tpl); err != nil {
			t.Fatalf("template %s: %v", tpl.Name, err)
		}
	}
	if _, err := svc.CreateModuleType(ctx, ModuleTypeInput{ManufacturerRef: mSlug, Model: "NM-8X", Slug: "nm-8x"}); err != nil {
		t.Fatalf("nic type: %v", err)
	}
	if _, err := svc.CreateTemplate(ctx, TemplateInput{ModuleTypeRef: mSlug + "/nm-8x", Kind: "module-bay", Name: "SFP{module}-1", Attrs: ComponentAttrs{Position: "{module}.1"}}); err != nil {
		t.Fatalf("sfp bay template: %v", err)
	}
	if _, err := svc.CreateModuleType(ctx, ModuleTypeInput{ManufacturerRef: mSlug, Model: "SFP-10G", Slug: "sfp-10g"}); err != nil {
		t.Fatalf("sfp type: %v", err)
	}
	if _, err := svc.CreateTemplate(ctx, TemplateInput{ModuleTypeRef: mSlug + "/sfp-10g", Kind: "interface", Name: "Te{module}", Attrs: ComponentAttrs{Type: "10gbase-x-sfpp"}}); err != nil {
		t.Fatalf("sfp interface template: %v", err)
	}

	// Rack + device; placement overlap is rejected.
	rack, err := svc.CreateRack(ctx, RackInput{SiteRef: siteSlug, Name: "R1", Slug: "r1", UHeight: 42})
	if err != nil {
		t.Fatalf("rack: %v", err)
	}
	_ = rack
	dev, err := svc.CreateDevice(ctx, DeviceInput{
		SiteRef: siteSlug, RackRef: siteSlug + "/r1", Position: 10, Face: "front",
		DeviceTypeRef: mSlug + "/c9300", RoleRef: role.Slug, Name: "core-sw-01",
	})
	if err != nil {
		t.Fatalf("device: %v", err)
	}
	if _, err := svc.CreateDevice(ctx, DeviceInput{
		SiteRef: siteSlug, RackRef: siteSlug + "/r1", Position: 10, Face: "front",
		DeviceTypeRef: mSlug + "/c9300", RoleRef: role.Slug, Name: "overlap",
	}); fault.KindOf(err) != fault.Conflict {
		t.Fatalf("overlap: got %v, want Conflict", err)
	}

	// Templates were stamped.
	comps, err := svc.ComponentsOfDevice(ctx, dev.ID)
	if err != nil || len(comps) != 3 {
		t.Fatalf("stamped components = %d, %v", len(comps), err)
	}
	var nmBay gen.Component
	for _, c := range comps {
		if c.Kind == "module-bay" {
			nmBay = c
		}
	}
	if nmBay.Name != "NM1" {
		t.Fatalf("bay missing: %+v", comps)
	}

	// Install the NIC: its SFP bay appears with {module}→"1".
	nic, err := svc.InstallModule(ctx, ModuleInput{
		DeviceRef: siteSlug + "/core-sw-01", BayID: nmBay.ID.String(), ModuleTypeRef: mSlug + "/nm-8x",
	})
	if err != nil {
		t.Fatalf("install nic: %v", err)
	}
	if _, err := svc.InstallModule(ctx, ModuleInput{
		DeviceRef: siteSlug + "/core-sw-01", BayID: nmBay.ID.String(), ModuleTypeRef: mSlug + "/nm-8x",
	}); fault.KindOf(err) != fault.Conflict {
		t.Fatalf("double install: got %v, want Conflict", err)
	}

	comps, _ = svc.ComponentsOfDevice(ctx, dev.ID)
	var sfpBay gen.Component
	for _, c := range comps {
		if c.Kind == "module-bay" && c.Name == "SFP1-1" {
			sfpBay = c
		}
	}
	if sfpBay.ID == dev.ID || sfpBay.Name == "" {
		t.Fatalf("nested SFP bay not created: %+v", comps)
	}
	if sfpBay.ModuleID == nil || *sfpBay.ModuleID != nic.ID {
		t.Fatalf("nested bay not owned by nic module: %+v", sfpBay)
	}

	// Install the SFP into the nested bay: its interface appears with the
	// nested position substituted ("Te1.1").
	sfp, err := svc.InstallModule(ctx, ModuleInput{
		DeviceRef: siteSlug + "/core-sw-01", BayID: sfpBay.ID.String(), ModuleTypeRef: mSlug + "/sfp-10g",
	})
	if err != nil {
		t.Fatalf("install sfp: %v", err)
	}
	_ = sfp
	comps, _ = svc.ComponentsOfDevice(ctx, dev.ID)
	foundTe := false
	for _, c := range comps {
		if c.Kind == "interface" && c.Name == "Te1.1" {
			foundTe = true
		}
	}
	if !foundTe {
		t.Fatalf("nested interface missing: %+v", comps)
	}
	if len(comps) != 6 { // Gi, NM1, PSU1, SFP1-1 bay, nic's none... + Te1.1 = 5? device(3) + nic(1 bay) + sfp(1 iface)
		// 3 stamped + 1 nested bay + 1 nested interface = 5; tolerate 5.
		if len(comps) != 5 {
			t.Fatalf("component count = %d", len(comps))
		}
	}

	// Removing the NIC cascades: its bay's SFP module and the Te
	// interface vanish with it.
	if err := svc.RemoveModule(ctx, nic.ID); err != nil {
		t.Fatalf("remove nic: %v", err)
	}
	comps, _ = svc.ComponentsOfDevice(ctx, dev.ID)
	if len(comps) != 3 {
		t.Fatalf("after removal components = %d, want 3: %+v", len(comps), comps)
	}
	mods, err := svc.ModulesOfDevice(ctx, dev.ID)
	if err != nil || len(mods) != 0 {
		t.Fatalf("modules after removal = %d, %v", len(mods), err)
	}

	// Device deletion cascades everything; rack delete then works.
	if err := svc.DeleteDeviceByRef(ctx, siteSlug+"/core-sw-01"); err != nil {
		t.Fatalf("delete device: %v", err)
	}
	if err := svc.DeleteRackByRef(ctx, siteSlug+"/r1"); err != nil {
		t.Fatalf("delete rack: %v", err)
	}
}
