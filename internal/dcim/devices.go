package dcim

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/saku75/warren/internal/changelog"
	"github.com/saku75/warren/internal/core/fault"
	"github.com/saku75/warren/internal/core/id"
	"github.com/saku75/warren/internal/core/objtype"
	"github.com/saku75/warren/internal/db/gen"
)

var (
	// TypeRack / TypeDevice: slugs scoped to their site ("cph-dc1/r07").
	TypeRack      = objtype.Register(objtype.Type{Key: "dcim.rack", Name: "Rack", Plural: "Racks"})
	TypeDevice    = objtype.Register(objtype.Type{Key: "dcim.device", Name: "Device", Plural: "Devices"})
	TypeComponent = objtype.Register(objtype.Type{Key: "dcim.component", Name: "Component", Plural: "Components"})
	TypeModule    = objtype.Register(objtype.Type{Key: "dcim.module", Name: "Module", Plural: "Modules"})
)

// RackInput are the writable rack fields.
type RackInput struct {
	SiteRef     string
	LocationRef string
	Name        string
	Slug        string
	Status      string
	UHeight     int32
	TenantRef   string
	Description string
}

// RackUpdate carries partial changes (site is fixed).
type RackUpdate struct {
	Name        *string
	Slug        *string
	Status      *string
	UHeight     *int32
	LocationRef *string
	TenantRef   *string
	Description *string
}

func rackSnapshot(r gen.Rack) map[string]any {
	return map[string]any{"site_id": r.SiteID.String(), "slug": r.Slug, "name": r.Name,
		"status": r.Status, "u_height": r.UHeight, "description": r.Description}
}

// CreateRack makes a rack within a site.
func (s *Service) CreateRack(ctx context.Context, in RackInput) (gen.Rack, error) {
	name, sl, err := normalizeNameSlug(in.Name, in.Slug)
	if err != nil {
		return gen.Rack{}, err
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if !validStatus(in.Status) {
		return gen.Rack{}, fault.New(fault.Invalid, "invalid status %q", in.Status)
	}
	if in.UHeight == 0 {
		in.UHeight = 42
	}
	if in.SiteRef == "" {
		return gen.Rack{}, fault.New(fault.Invalid, "site is required")
	}
	site, err := s.GetSiteByRef(ctx, in.SiteRef)
	if err != nil {
		return gen.Rack{}, err
	}
	var locationID *id.ID
	if in.LocationRef != "" {
		loc, err := s.resolveParentRef(ctx, site.ID, in.LocationRef)
		if err != nil {
			return gen.Rack{}, err
		}
		locationID = loc
	}
	tenantID, err := s.resolveTenantRef(ctx, in.TenantRef)
	if err != nil {
		return gen.Rack{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.Rack{}, fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	rack, err := q.CreateRack(ctx, gen.CreateRackParams{
		ID: id.New(), SiteID: site.ID, LocationID: locationID, Slug: sl, Name: name,
		Status: in.Status, UHeight: in.UHeight, TenantID: tenantID, Description: in.Description,
	})
	if err != nil {
		return gen.Rack{}, fault.FromDB(err, "rack "+site.Slug+"/"+sl)
	}
	if err := changelog.Record(ctx, q, changelog.ActionCreate, TypeRack.Key, rack.ID, rack.Name, nil, rackSnapshot(rack)); err != nil {
		return gen.Rack{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.Rack{}, fmt.Errorf("dcim: commit: %w", err)
	}
	return rack, nil
}

// GetRackByRef resolves an ID or "site/rack" path.
func (s *Service) GetRackByRef(ctx context.Context, ref string) (gen.Rack, error) {
	if rid, err := id.Parse(ref); err == nil {
		rack, err := s.q.GetRack(ctx, rid)
		return rack, fault.FromDB(err, "rack "+ref)
	}
	siteSlug, rackSlug, err := splitTypeRef(ref, "rack")
	if err != nil {
		return gen.Rack{}, err
	}
	site, err := s.GetSiteByRef(ctx, siteSlug)
	if err != nil {
		return gen.Rack{}, err
	}
	rack, err := s.q.GetRackByScope(ctx, gen.GetRackByScopeParams{SiteID: site.ID, Slug: rackSlug})
	return rack, fault.FromDB(err, "rack "+ref)
}

// ListRacks returns racks (with site display fields) and the total count.
func (s *Service) ListRacks(ctx context.Context, limit, offset int32) ([]gen.ListRacksRow, int64, error) {
	total, err := s.q.CountRacks(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("dcim: count racks: %w", err)
	}
	items, err := s.q.ListRacks(ctx, gen.ListRacksParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, fmt.Errorf("dcim: list racks: %w", err)
	}
	return items, total, nil
}

// DevicesInRack lists mounted devices with their type heights, top-down.
func (s *Service) DevicesInRack(ctx context.Context, rackID id.ID) ([]gen.ListDevicesByRackRow, error) {
	items, err := s.q.ListDevicesByRack(ctx, &rackID)
	if err != nil {
		return nil, fmt.Errorf("dcim: rack devices: %w", err)
	}
	return items, nil
}

// UpdateRackByRef applies a partial update.
func (s *Service) UpdateRackByRef(ctx context.Context, ref string, up RackUpdate) (gen.Rack, error) {
	cur, err := s.GetRackByRef(ctx, ref)
	if err != nil {
		return gen.Rack{}, err
	}
	next := gen.UpdateRackParams{
		ID: cur.ID, Slug: cur.Slug, Name: cur.Name, Status: cur.Status,
		UHeight: cur.UHeight, LocationID: cur.LocationID, TenantID: cur.TenantID,
		Description: cur.Description,
	}
	if up.Name != nil {
		next.Name = *up.Name
	}
	if up.Slug != nil {
		next.Slug = *up.Slug
	}
	if up.Status != nil {
		next.Status = *up.Status
	}
	if up.UHeight != nil {
		next.UHeight = *up.UHeight
	}
	if up.Description != nil {
		next.Description = *up.Description
	}
	if up.LocationRef != nil {
		next.LocationID = nil
		if *up.LocationRef != "" {
			loc, err := s.resolveParentRef(ctx, cur.SiteID, *up.LocationRef)
			if err != nil {
				return gen.Rack{}, err
			}
			next.LocationID = loc
		}
	}
	if up.TenantRef != nil {
		next.TenantID, err = s.resolveTenantRef(ctx, *up.TenantRef)
		if err != nil {
			return gen.Rack{}, err
		}
	}
	if next.Name, next.Slug, err = normalizeNameSlug(next.Name, next.Slug); err != nil {
		return gen.Rack{}, err
	}
	if !validStatus(next.Status) {
		return gen.Rack{}, fault.New(fault.Invalid, "invalid status %q", next.Status)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.Rack{}, fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	rack, err := q.UpdateRack(ctx, next)
	if err != nil {
		return gen.Rack{}, fault.FromDB(err, "rack "+ref)
	}
	if err := changelog.Record(ctx, q, changelog.ActionUpdate, TypeRack.Key, rack.ID, rack.Name, rackSnapshot(cur), rackSnapshot(rack)); err != nil {
		return gen.Rack{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.Rack{}, fmt.Errorf("dcim: commit: %w", err)
	}
	return rack, nil
}

// DeleteRackByRef removes a rack; mounted devices block it.
func (s *Service) DeleteRackByRef(ctx context.Context, ref string) error {
	cur, err := s.GetRackByRef(ctx, ref)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	if _, err := q.DeleteRack(ctx, cur.ID); err != nil {
		return fault.FromDB(err, "rack "+ref)
	}
	if err := changelog.Record(ctx, q, changelog.ActionDelete, TypeRack.Key, cur.ID, cur.Name, rackSnapshot(cur), nil); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("dcim: commit: %w", err)
	}
	return nil
}

// DeviceInput are the writable device fields.
type DeviceInput struct {
	SiteRef       string
	LocationRef   string
	RackRef       string
	Position      float64 // 0 = unpositioned
	Face          string
	DeviceTypeRef string
	RoleRef       string
	TenantRef     string
	Name          string
	Slug          string
	Status        string
	Serial        string
	AssetTag      string
	Description   string
}

// DeviceUpdate carries partial changes (site and device type are fixed).
type DeviceUpdate struct {
	Name        *string
	Slug        *string
	Status      *string
	LocationRef *string
	RackRef     *string
	Position    *float64
	Face        *string
	RoleRef     *string
	TenantRef   *string
	Serial      *string
	AssetTag    *string
	Description *string
}

func deviceSnapshot(d gen.Device) map[string]any {
	snap := map[string]any{
		"site_id": d.SiteID.String(), "slug": d.Slug, "name": d.Name, "status": d.Status,
		"device_type_id": d.DeviceTypeID.String(), "role_id": d.RoleID.String(),
		"serial": d.Serial, "asset_tag": d.AssetTag, "description": d.Description,
	}
	if d.RackID != nil {
		snap["rack_id"] = d.RackID.String()
	}
	if d.Position != nil {
		snap["position"] = *d.Position
	}
	return snap
}

// checkRackPlacement validates position/face and rejects overlaps.
func (s *Service) checkRackPlacement(ctx context.Context, rack gen.Rack, deviceID id.ID, position float64, face string, height float64) error {
	if face != "front" && face != "rear" {
		return fault.New(fault.Invalid, "face must be front or rear for racked devices")
	}
	if position < 1 || position+height-1 > float64(rack.UHeight) {
		return fault.New(fault.Invalid, "device does not fit: position %g, height %gU, rack %dU", position, height, rack.UHeight)
	}
	mounted, err := s.DevicesInRack(ctx, rack.ID)
	if err != nil {
		return err
	}
	for _, m := range mounted {
		if m.ID == deviceID || m.Position == nil {
			continue
		}
		// Same-face overlap only; full-depth handling comes with the
		// elevations polish.
		if m.Face != nil && *m.Face != face {
			continue
		}
		lo, hi := *m.Position, *m.Position+m.TypeUHeight-1
		if position <= hi && position+height-1 >= lo {
			return fault.New(fault.Conflict, "position %g–%g overlaps %s (U%g–U%g)", position, position+height-1, m.Name, lo, hi)
		}
	}
	return nil
}

// CreateDevice makes a device and stamps its type's component templates.
func (s *Service) CreateDevice(ctx context.Context, in DeviceInput) (gen.Device, error) {
	name, sl, err := normalizeNameSlug(in.Name, in.Slug)
	if err != nil {
		return gen.Device{}, err
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if !validStatus(in.Status) {
		return gen.Device{}, fault.New(fault.Invalid, "invalid status %q", in.Status)
	}
	if in.SiteRef == "" || in.DeviceTypeRef == "" || in.RoleRef == "" {
		return gen.Device{}, fault.New(fault.Invalid, "site, device type, and role are required")
	}
	site, err := s.GetSiteByRef(ctx, in.SiteRef)
	if err != nil {
		return gen.Device{}, err
	}
	dt, err := s.GetDeviceTypeByRef(ctx, in.DeviceTypeRef)
	if err != nil {
		return gen.Device{}, err
	}
	role, err := s.GetDeviceRoleByRef(ctx, in.RoleRef)
	if err != nil {
		return gen.Device{}, err
	}
	tenantID, err := s.resolveTenantRef(ctx, in.TenantRef)
	if err != nil {
		return gen.Device{}, err
	}
	var locationID *id.ID
	if in.LocationRef != "" {
		if locationID, err = s.resolveParentRef(ctx, site.ID, in.LocationRef); err != nil {
			return gen.Device{}, err
		}
	}
	var rackID *id.ID
	var position *float64
	var face *string
	if in.RackRef != "" {
		rack, err := s.GetRackByRef(ctx, in.RackRef)
		if err != nil {
			return gen.Device{}, err
		}
		if rack.SiteID != site.ID {
			return gen.Device{}, fault.New(fault.Invalid, "rack belongs to a different site")
		}
		rackID = &rack.ID
		if in.Position > 0 {
			if err := s.checkRackPlacement(ctx, rack, id.Nil, in.Position, in.Face, dt.UHeight); err != nil {
				return gen.Device{}, err
			}
			position, face = &in.Position, &in.Face
		}
	}

	templates, err := s.TemplatesOfDeviceType(ctx, dt.ID)
	if err != nil {
		return gen.Device{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.Device{}, fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	d, err := q.CreateDevice(ctx, gen.CreateDeviceParams{
		ID: id.New(), SiteID: site.ID, LocationID: locationID, RackID: rackID,
		Position: position, Face: face, DeviceTypeID: dt.ID, RoleID: role.ID,
		TenantID: tenantID, Slug: sl, Name: name, Status: in.Status,
		Serial: in.Serial, AssetTag: in.AssetTag, Description: in.Description,
	})
	if err != nil {
		return gen.Device{}, fault.FromDB(err, "device "+site.Slug+"/"+sl)
	}
	if err := instantiateTemplates(ctx, q, d.ID, nil, templates, ""); err != nil {
		return gen.Device{}, err
	}
	if err := changelog.Record(ctx, q, changelog.ActionCreate, TypeDevice.Key, d.ID, d.Name, nil, deviceSnapshot(d)); err != nil {
		return gen.Device{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.Device{}, fmt.Errorf("dcim: commit: %w", err)
	}
	return d, nil
}

// instantiateTemplates stamps templates as components on a device,
// substituting {module} with the receiving bay position when installing
// a module (design doc 0003 §3).
func instantiateTemplates(ctx context.Context, q *gen.Queries, deviceID id.ID, moduleID *id.ID, templates []gen.ComponentTemplate, modulePosition string) error {
	for _, t := range templates {
		name := t.Name
		attrs := t.Attrs
		if modulePosition != "" {
			name = strings.ReplaceAll(name, "{module}", modulePosition)
			// Bay positions carry the token too, so nested bays resolve to
			// "1.1" and the next module level substitutes correctly.
			a, err := UnmarshalAttrs(t.Attrs)
			if err != nil {
				return err
			}
			if strings.Contains(a.Position, "{module}") {
				a.Position = strings.ReplaceAll(a.Position, "{module}", modulePosition)
				if attrs, err = a.Marshal(); err != nil {
					return err
				}
			}
		}
		if _, err := q.CreateComponent(ctx, gen.CreateComponentParams{
			ID: id.New(), DeviceID: deviceID, ModuleID: moduleID,
			Kind: t.Kind, Name: name, Label: t.Label, Attrs: attrs, Description: t.Description,
		}); err != nil {
			return fault.FromDB(err, "component "+name)
		}
	}
	return nil
}

// GetDeviceByRef resolves an ID or "site/device" path.
func (s *Service) GetDeviceByRef(ctx context.Context, ref string) (gen.Device, error) {
	if did, err := id.Parse(ref); err == nil {
		d, err := s.q.GetDevice(ctx, did)
		return d, fault.FromDB(err, "device "+ref)
	}
	siteSlug, devSlug, err := splitTypeRef(ref, "device")
	if err != nil {
		return gen.Device{}, err
	}
	site, err := s.GetSiteByRef(ctx, siteSlug)
	if err != nil {
		return gen.Device{}, err
	}
	d, err := s.q.GetDeviceByScope(ctx, gen.GetDeviceByScopeParams{SiteID: site.ID, Slug: devSlug})
	return d, fault.FromDB(err, "device "+ref)
}

// ListDevices returns devices (with display joins) and the total count.
func (s *Service) ListDevices(ctx context.Context, limit, offset int32) ([]gen.ListDevicesRow, int64, error) {
	total, err := s.q.CountDevices(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("dcim: count devices: %w", err)
	}
	items, err := s.q.ListDevices(ctx, gen.ListDevicesParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, fmt.Errorf("dcim: list devices: %w", err)
	}
	return items, total, nil
}

// ComponentsOfDevice lists every component on a device.
func (s *Service) ComponentsOfDevice(ctx context.Context, deviceID id.ID) ([]gen.Component, error) {
	items, err := s.q.ListComponentsByDevice(ctx, deviceID)
	if err != nil {
		return nil, fmt.Errorf("dcim: list components: %w", err)
	}
	return items, nil
}

// UpdateDeviceByRef applies a partial update.
func (s *Service) UpdateDeviceByRef(ctx context.Context, ref string, up DeviceUpdate) (gen.Device, error) {
	cur, err := s.GetDeviceByRef(ctx, ref)
	if err != nil {
		return gen.Device{}, err
	}
	dt, err := s.q.GetDeviceType(ctx, cur.DeviceTypeID)
	if err != nil {
		return gen.Device{}, fault.FromDB(err, "device type")
	}

	next := gen.UpdateDeviceParams{
		ID: cur.ID, Slug: cur.Slug, Name: cur.Name, Status: cur.Status,
		LocationID: cur.LocationID, RackID: cur.RackID, Position: cur.Position,
		Face: cur.Face, RoleID: cur.RoleID, TenantID: cur.TenantID,
		Serial: cur.Serial, AssetTag: cur.AssetTag, Description: cur.Description,
	}
	if up.Name != nil {
		next.Name = *up.Name
	}
	if up.Slug != nil {
		next.Slug = *up.Slug
	}
	if up.Status != nil {
		next.Status = *up.Status
	}
	if up.Serial != nil {
		next.Serial = *up.Serial
	}
	if up.AssetTag != nil {
		next.AssetTag = *up.AssetTag
	}
	if up.Description != nil {
		next.Description = *up.Description
	}
	if up.RoleRef != nil {
		role, err := s.GetDeviceRoleByRef(ctx, *up.RoleRef)
		if err != nil {
			return gen.Device{}, err
		}
		next.RoleID = role.ID
	}
	if up.TenantRef != nil {
		if next.TenantID, err = s.resolveTenantRef(ctx, *up.TenantRef); err != nil {
			return gen.Device{}, err
		}
	}
	if up.LocationRef != nil {
		next.LocationID = nil
		if *up.LocationRef != "" {
			if next.LocationID, err = s.resolveParentRef(ctx, cur.SiteID, *up.LocationRef); err != nil {
				return gen.Device{}, err
			}
		}
	}
	if up.RackRef != nil {
		next.RackID, next.Position, next.Face = nil, nil, nil
		if *up.RackRef != "" {
			rack, err := s.GetRackByRef(ctx, *up.RackRef)
			if err != nil {
				return gen.Device{}, err
			}
			if rack.SiteID != cur.SiteID {
				return gen.Device{}, fault.New(fault.Invalid, "rack belongs to a different site")
			}
			next.RackID = &rack.ID
		}
	}
	if up.Position != nil && next.RackID != nil {
		face := ""
		if up.Face != nil {
			face = *up.Face
		} else if cur.Face != nil {
			face = *cur.Face
		}
		rack, err := s.q.GetRack(ctx, *next.RackID)
		if err != nil {
			return gen.Device{}, fault.FromDB(err, "rack")
		}
		if *up.Position > 0 {
			if err := s.checkRackPlacement(ctx, rack, cur.ID, *up.Position, face, dt.UHeight); err != nil {
				return gen.Device{}, err
			}
			next.Position, next.Face = up.Position, &face
		} else {
			next.Position, next.Face = nil, nil
		}
	}
	if next.Name, next.Slug, err = normalizeNameSlug(next.Name, next.Slug); err != nil {
		return gen.Device{}, err
	}
	if !validStatus(next.Status) {
		return gen.Device{}, fault.New(fault.Invalid, "invalid status %q", next.Status)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.Device{}, fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	d, err := q.UpdateDevice(ctx, next)
	if err != nil {
		return gen.Device{}, fault.FromDB(err, "device "+ref)
	}
	if err := changelog.Record(ctx, q, changelog.ActionUpdate, TypeDevice.Key, d.ID, d.Name, deviceSnapshot(cur), deviceSnapshot(d)); err != nil {
		return gen.Device{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.Device{}, fmt.Errorf("dcim: commit: %w", err)
	}
	return d, nil
}

// DeleteDeviceByRef removes a device; components and modules cascade.
func (s *Service) DeleteDeviceByRef(ctx context.Context, ref string) error {
	cur, err := s.GetDeviceByRef(ctx, ref)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	if _, err := q.DeleteDevice(ctx, cur.ID); err != nil {
		return fault.FromDB(err, "device "+ref)
	}
	if err := changelog.Record(ctx, q, changelog.ActionDelete, TypeDevice.Key, cur.ID, cur.Name, deviceSnapshot(cur), nil); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("dcim: commit: %w", err)
	}
	return nil
}

// ModuleInput installs a module type into a bay on a device.
type ModuleInput struct {
	DeviceRef     string
	BayID         string // component id of an empty module-bay
	ModuleTypeRef string
	Serial        string
	Description   string
}

func moduleSnapshot(m gen.Module) map[string]any {
	return map[string]any{
		"device_id": m.DeviceID.String(), "bay_id": m.BayID.String(),
		"module_type_id": m.ModuleTypeID.String(), "serial": m.Serial,
	}
}

// InstallModule seats a module type in a bay and stamps its templates as
// components — including nested module-bay templates, which is how SFP
// bays appear on an installed NIC.
func (s *Service) InstallModule(ctx context.Context, in ModuleInput) (gen.Module, error) {
	d, err := s.GetDeviceByRef(ctx, in.DeviceRef)
	if err != nil {
		return gen.Module{}, err
	}
	mt, err := s.GetModuleTypeByRef(ctx, in.ModuleTypeRef)
	if err != nil {
		return gen.Module{}, err
	}
	bayID, err := id.Parse(in.BayID)
	if err != nil {
		return gen.Module{}, fault.New(fault.Invalid, "bay must be a component ID")
	}
	bay, err := s.q.GetComponent(ctx, bayID)
	if err != nil {
		return gen.Module{}, fault.FromDB(err, "bay "+in.BayID)
	}
	if bay.DeviceID != d.ID {
		return gen.Module{}, fault.New(fault.Invalid, "bay belongs to a different device")
	}
	if bay.Kind != "module-bay" {
		return gen.Module{}, fault.New(fault.Invalid, "component %s is a %s, not a module bay", bay.Name, bay.Kind)
	}
	if _, err := s.q.GetModuleByBay(ctx, bay.ID); err == nil {
		return gen.Module{}, fault.New(fault.Conflict, "bay %s is already occupied", bay.Name)
	} else if !isNoRows(err) {
		return gen.Module{}, fault.FromDB(err, "bay "+bay.Name)
	}

	bayAttrs, err := UnmarshalAttrs(bay.Attrs)
	if err != nil {
		return gen.Module{}, err
	}
	position := bayAttrs.Position
	if position == "" {
		position = bay.Name
	}
	templates, err := s.TemplatesOfModuleType(ctx, mt.ID)
	if err != nil {
		return gen.Module{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.Module{}, fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	m, err := q.CreateModule(ctx, gen.CreateModuleParams{
		ID: id.New(), DeviceID: d.ID, BayID: bay.ID, ModuleTypeID: mt.ID,
		Serial: in.Serial, Description: in.Description,
	})
	if err != nil {
		return gen.Module{}, fault.FromDB(err, "module in bay "+bay.Name)
	}
	if err := instantiateTemplates(ctx, q, d.ID, &m.ID, templates, position); err != nil {
		return gen.Module{}, err
	}
	if err := changelog.Record(ctx, q, changelog.ActionCreate, TypeModule.Key, m.ID, mt.Model+" in "+d.Name+"/"+bay.Name, nil, moduleSnapshot(m)); err != nil {
		return gen.Module{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.Module{}, fmt.Errorf("dcim: commit: %w", err)
	}
	return m, nil
}

func isNoRows(err error) bool {
	return err != nil && (err == pgx.ErrNoRows || strings.Contains(err.Error(), "no rows"))
}

// ModulesOfDevice lists a device's installed modules.
func (s *Service) ModulesOfDevice(ctx context.Context, deviceID id.ID) ([]gen.ListModulesByDeviceRow, error) {
	items, err := s.q.ListModulesByDevice(ctx, deviceID)
	if err != nil {
		return nil, fmt.Errorf("dcim: list modules: %w", err)
	}
	return items, nil
}

// RemoveModule deletes a module; its components — and recursively any
// modules seated in bays it provided — cascade away via foreign keys.
func (s *Service) RemoveModule(ctx context.Context, moduleID id.ID) error {
	m, err := s.q.GetModule(ctx, moduleID)
	if err != nil {
		return fault.FromDB(err, "module "+moduleID.String())
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	if _, err := q.DeleteModule(ctx, m.ID); err != nil {
		return fault.FromDB(err, "module")
	}
	if err := changelog.Record(ctx, q, changelog.ActionDelete, TypeModule.Key, m.ID, m.ID.String(), moduleSnapshot(m), nil); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("dcim: commit: %w", err)
	}
	return nil
}
