package dcim

import (
	"context"
	"testing"

	"github.com/saku75/warren/internal/core/fault"
	"github.com/saku75/warren/internal/db/dbtest"
)

func TestCatalogAndTemplates(t *testing.T) {
	pool := dbtest.Pool(t)
	svc := NewService(pool)
	ctx := context.Background()

	mSlug := uniq("cisco")
	if _, err := svc.CreateManufacturer(ctx, ManufacturerInput{Name: "Cisco " + mSlug, Slug: mSlug}); err != nil {
		t.Fatalf("create manufacturer: %v", err)
	}

	if _, err := svc.CreateDeviceRole(ctx, DeviceRoleInput{Name: "Role " + uniq("r"), Color: "#22aa55"}); err != nil {
		t.Fatalf("create role: %v", err)
	}
	if _, err := svc.CreateDeviceRole(ctx, DeviceRoleInput{Name: "Bad", Color: "red"}); fault.KindOf(err) != fault.Invalid {
		t.Fatalf("bad color: %v", err)
	}

	// Device type scoped to manufacturer, resolved by path.
	dt, err := svc.CreateDeviceType(ctx, DeviceTypeInput{
		ManufacturerRef: mSlug, Model: "Catalyst 9300-48P", Slug: "c9300-48p", UHeight: 1, IsFullDepth: true,
	})
	if err != nil {
		t.Fatalf("create device type: %v", err)
	}
	got, err := svc.GetDeviceTypeByRef(ctx, mSlug+"/c9300-48p")
	if err != nil || got.ID != dt.ID {
		t.Fatalf("resolve device type path: %v", err)
	}
	// Same slug under another manufacturer is fine.
	m2Slug := uniq("arista")
	if _, err := svc.CreateManufacturer(ctx, ManufacturerInput{Name: "Arista " + m2Slug, Slug: m2Slug}); err != nil {
		t.Fatalf("create m2: %v", err)
	}
	if _, err := svc.CreateDeviceType(ctx, DeviceTypeInput{ManufacturerRef: m2Slug, Model: "X", Slug: "c9300-48p"}); err != nil {
		t.Fatalf("same slug other manufacturer: %v", err)
	}
	// Duplicate within the manufacturer conflicts.
	if _, err := svc.CreateDeviceType(ctx, DeviceTypeInput{ManufacturerRef: mSlug, Model: "Dup", Slug: "c9300-48p"}); fault.KindOf(err) != fault.Conflict {
		t.Fatalf("dup device type: %v", err)
	}
	// Manufacturer with types cannot be deleted.
	if err := svc.DeleteManufacturerByRef(ctx, mSlug); fault.KindOf(err) != fault.Conflict {
		t.Fatalf("delete manufacturer in use: %v", err)
	}

	// Module type with a nested module-bay template (SFP-in-NIC).
	nic, err := svc.CreateModuleType(ctx, ModuleTypeInput{ManufacturerRef: mSlug, Model: "C9300-NM-8X", Slug: "c9300-nm-8x"})
	if err != nil {
		t.Fatalf("create module type: %v", err)
	}

	// Templates: interface on the device type, module bays on both.
	if _, err := svc.CreateTemplate(ctx, TemplateInput{
		DeviceTypeRef: mSlug + "/c9300-48p", Kind: "interface", Name: "GigabitEthernet1/0/1",
		Attrs: ComponentAttrs{Type: "1000base-t"},
	}); err != nil {
		t.Fatalf("create interface template: %v", err)
	}
	if _, err := svc.CreateTemplate(ctx, TemplateInput{
		DeviceTypeRef: mSlug + "/c9300-48p", Kind: "module-bay", Name: "NM Slot 1",
		Attrs: ComponentAttrs{Position: "1"},
	}); err != nil {
		t.Fatalf("create bay template: %v", err)
	}
	// The nested bay: SFP slots on the NIC module type.
	if _, err := svc.CreateTemplate(ctx, TemplateInput{
		ModuleTypeRef: mSlug + "/c9300-nm-8x", Kind: "module-bay", Name: "SFP {module}/1",
		Attrs: ComponentAttrs{Position: "1"},
	}); err != nil {
		t.Fatalf("create nested bay template: %v", err)
	}

	// Attrs are validated per kind.
	if _, err := svc.CreateTemplate(ctx, TemplateInput{
		DeviceTypeRef: mSlug + "/c9300-48p", Kind: "interface", Name: "bad",
		Attrs: ComponentAttrs{Position: "9"},
	}); fault.KindOf(err) != fault.Invalid {
		t.Fatalf("interface with position: %v", err)
	}
	if _, err := svc.CreateTemplate(ctx, TemplateInput{
		ModuleTypeRef: mSlug + "/c9300-nm-8x", Kind: "power-outlet", Name: "out",
		Attrs: ComponentAttrs{FeedLeg: "D"},
	}); fault.KindOf(err) != fault.Invalid {
		t.Fatalf("bad feed leg: %v", err)
	}
	// Duplicate template name per owner+kind conflicts.
	if _, err := svc.CreateTemplate(ctx, TemplateInput{
		DeviceTypeRef: mSlug + "/c9300-48p", Kind: "interface", Name: "GigabitEthernet1/0/1",
	}); fault.KindOf(err) != fault.Conflict {
		t.Fatalf("dup template: %v", err)
	}

	dtTemplates, err := svc.TemplatesOfDeviceType(ctx, dt.ID)
	if err != nil || len(dtTemplates) != 2 {
		t.Fatalf("device type templates = %d, %v", len(dtTemplates), err)
	}
	nicTemplates, err := svc.TemplatesOfModuleType(ctx, nic.ID)
	if err != nil || len(nicTemplates) != 1 || nicTemplates[0].Kind != "module-bay" {
		t.Fatalf("module type templates wrong: %v, %v", nicTemplates, err)
	}

	// Deleting the device type cascades its templates.
	if err := svc.DeleteDeviceTypeByRef(ctx, mSlug+"/c9300-48p"); err != nil {
		t.Fatalf("delete device type: %v", err)
	}
	if _, err := svc.GetTemplate(ctx, dtTemplates[0].ID); fault.KindOf(err) != fault.NotFound {
		t.Fatalf("template survived owner delete: %v", err)
	}
}
