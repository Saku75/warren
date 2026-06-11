package dcim

import (
	"context"
	"fmt"
	"regexp"

	"github.com/saku75/warren/internal/changelog"
	"github.com/saku75/warren/internal/core/fault"
	"github.com/saku75/warren/internal/core/id"
	"github.com/saku75/warren/internal/core/objtype"
	"github.com/saku75/warren/internal/core/slug"
	"github.com/saku75/warren/internal/db/gen"
)

var (
	// TypeManufacturer: global slug.
	TypeManufacturer = objtype.Register(objtype.Type{Key: "dcim.manufacturer", Name: "Manufacturer", Plural: "Manufacturers"})
	// TypeDeviceRole: global slug.
	TypeDeviceRole = objtype.Register(objtype.Type{Key: "dcim.device-role", Name: "Device role", Plural: "Device roles"})
)

var colorPattern = regexp.MustCompile(`^#[0-9a-f]{6}$`)

// ManufacturerInput are the writable manufacturer fields.
type ManufacturerInput struct {
	Name        string
	Slug        string
	Description string
}

// ManufacturerUpdate carries partial changes.
type ManufacturerUpdate struct {
	Name        *string
	Slug        *string
	Description *string
}

func manufacturerSnapshot(m gen.Manufacturer) map[string]any {
	return map[string]any{"slug": m.Slug, "name": m.Name, "description": m.Description}
}

// normalizeNameSlug derives/validates the common name+slug pair.
func normalizeNameSlug(name, s string) (string, string, error) {
	if name == "" {
		return "", "", fault.New(fault.Invalid, "name is required")
	}
	if s == "" {
		derived, err := slug.Make(name)
		if err != nil {
			return "", "", fault.Wrap(fault.Invalid, err, "cannot derive slug from name; provide one")
		}
		return name, derived, nil
	}
	if err := slug.Validate(s); err != nil {
		return "", "", fault.Wrap(fault.Invalid, err, "invalid slug")
	}
	return name, s, nil
}

// CreateManufacturer makes a manufacturer.
func (s *Service) CreateManufacturer(ctx context.Context, in ManufacturerInput) (gen.Manufacturer, error) {
	name, sl, err := normalizeNameSlug(in.Name, in.Slug)
	if err != nil {
		return gen.Manufacturer{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.Manufacturer{}, fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	m, err := q.CreateManufacturer(ctx, gen.CreateManufacturerParams{
		ID: id.New(), Slug: sl, Name: name, Description: in.Description,
	})
	if err != nil {
		return gen.Manufacturer{}, fault.FromDB(err, "manufacturer "+sl)
	}
	if err := changelog.Record(ctx, q, changelog.ActionCreate, TypeManufacturer.Key, m.ID, m.Name, nil, manufacturerSnapshot(m)); err != nil {
		return gen.Manufacturer{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.Manufacturer{}, fmt.Errorf("dcim: commit: %w", err)
	}
	return m, nil
}

// GetManufacturerByRef resolves an ID or global slug.
func (s *Service) GetManufacturerByRef(ctx context.Context, ref string) (gen.Manufacturer, error) {
	if mid, err := id.Parse(ref); err == nil {
		m, err := s.q.GetManufacturer(ctx, mid)
		return m, fault.FromDB(err, "manufacturer "+ref)
	}
	if err := slug.Validate(ref); err != nil {
		return gen.Manufacturer{}, fault.Wrap(fault.Invalid, err, "invalid manufacturer reference %q", ref)
	}
	m, err := s.q.GetManufacturerBySlug(ctx, ref)
	return m, fault.FromDB(err, "manufacturer "+ref)
}

// ListManufacturers returns manufacturers plus the total count.
func (s *Service) ListManufacturers(ctx context.Context, limit, offset int32) ([]gen.Manufacturer, int64, error) {
	total, err := s.q.CountManufacturers(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("dcim: count manufacturers: %w", err)
	}
	items, err := s.q.ListManufacturers(ctx, gen.ListManufacturersParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, fmt.Errorf("dcim: list manufacturers: %w", err)
	}
	return items, total, nil
}

// UpdateManufacturerByRef applies a partial update.
func (s *Service) UpdateManufacturerByRef(ctx context.Context, ref string, up ManufacturerUpdate) (gen.Manufacturer, error) {
	cur, err := s.GetManufacturerByRef(ctx, ref)
	if err != nil {
		return gen.Manufacturer{}, err
	}
	name, sl, desc := cur.Name, cur.Slug, cur.Description
	if up.Name != nil {
		name = *up.Name
	}
	if up.Slug != nil {
		sl = *up.Slug
	}
	if up.Description != nil {
		desc = *up.Description
	}
	if name, sl, err = normalizeNameSlug(name, sl); err != nil {
		return gen.Manufacturer{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.Manufacturer{}, fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	m, err := q.UpdateManufacturer(ctx, gen.UpdateManufacturerParams{ID: cur.ID, Slug: sl, Name: name, Description: desc})
	if err != nil {
		return gen.Manufacturer{}, fault.FromDB(err, "manufacturer "+ref)
	}
	if err := changelog.Record(ctx, q, changelog.ActionUpdate, TypeManufacturer.Key, m.ID, m.Name, manufacturerSnapshot(cur), manufacturerSnapshot(m)); err != nil {
		return gen.Manufacturer{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.Manufacturer{}, fmt.Errorf("dcim: commit: %w", err)
	}
	return m, nil
}

// DeleteManufacturerByRef removes a manufacturer; device/module types
// referencing it block the delete.
func (s *Service) DeleteManufacturerByRef(ctx context.Context, ref string) error {
	cur, err := s.GetManufacturerByRef(ctx, ref)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	if _, err := q.DeleteManufacturer(ctx, cur.ID); err != nil {
		return fault.FromDB(err, "manufacturer "+ref)
	}
	if err := changelog.Record(ctx, q, changelog.ActionDelete, TypeManufacturer.Key, cur.ID, cur.Name, manufacturerSnapshot(cur), nil); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("dcim: commit: %w", err)
	}
	return nil
}

// DeviceRoleInput are the writable device role fields.
type DeviceRoleInput struct {
	Name        string
	Slug        string
	Color       string
	Description string
}

// DeviceRoleUpdate carries partial changes.
type DeviceRoleUpdate struct {
	Name        *string
	Slug        *string
	Color       *string
	Description *string
}

func deviceRoleSnapshot(r gen.DeviceRole) map[string]any {
	return map[string]any{"slug": r.Slug, "name": r.Name, "color": r.Color, "description": r.Description}
}

// CreateDeviceRole makes a device role.
func (s *Service) CreateDeviceRole(ctx context.Context, in DeviceRoleInput) (gen.DeviceRole, error) {
	name, sl, err := normalizeNameSlug(in.Name, in.Slug)
	if err != nil {
		return gen.DeviceRole{}, err
	}
	if in.Color == "" {
		in.Color = "#8a5a2b"
	}
	if !colorPattern.MatchString(in.Color) {
		return gen.DeviceRole{}, fault.New(fault.Invalid, "color must be #rrggbb lowercase hex")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.DeviceRole{}, fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	r, err := q.CreateDeviceRole(ctx, gen.CreateDeviceRoleParams{
		ID: id.New(), Slug: sl, Name: name, Color: in.Color, Description: in.Description,
	})
	if err != nil {
		return gen.DeviceRole{}, fault.FromDB(err, "device role "+sl)
	}
	if err := changelog.Record(ctx, q, changelog.ActionCreate, TypeDeviceRole.Key, r.ID, r.Name, nil, deviceRoleSnapshot(r)); err != nil {
		return gen.DeviceRole{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.DeviceRole{}, fmt.Errorf("dcim: commit: %w", err)
	}
	return r, nil
}

// GetDeviceRoleByRef resolves an ID or global slug.
func (s *Service) GetDeviceRoleByRef(ctx context.Context, ref string) (gen.DeviceRole, error) {
	if rid, err := id.Parse(ref); err == nil {
		r, err := s.q.GetDeviceRole(ctx, rid)
		return r, fault.FromDB(err, "device role "+ref)
	}
	if err := slug.Validate(ref); err != nil {
		return gen.DeviceRole{}, fault.Wrap(fault.Invalid, err, "invalid device role reference %q", ref)
	}
	r, err := s.q.GetDeviceRoleBySlug(ctx, ref)
	return r, fault.FromDB(err, "device role "+ref)
}

// ListDeviceRoles returns roles plus the total count.
func (s *Service) ListDeviceRoles(ctx context.Context, limit, offset int32) ([]gen.DeviceRole, int64, error) {
	total, err := s.q.CountDeviceRoles(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("dcim: count roles: %w", err)
	}
	items, err := s.q.ListDeviceRoles(ctx, gen.ListDeviceRolesParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, fmt.Errorf("dcim: list roles: %w", err)
	}
	return items, total, nil
}

// UpdateDeviceRoleByRef applies a partial update.
func (s *Service) UpdateDeviceRoleByRef(ctx context.Context, ref string, up DeviceRoleUpdate) (gen.DeviceRole, error) {
	cur, err := s.GetDeviceRoleByRef(ctx, ref)
	if err != nil {
		return gen.DeviceRole{}, err
	}
	name, sl, color, desc := cur.Name, cur.Slug, cur.Color, cur.Description
	if up.Name != nil {
		name = *up.Name
	}
	if up.Slug != nil {
		sl = *up.Slug
	}
	if up.Color != nil {
		color = *up.Color
	}
	if up.Description != nil {
		desc = *up.Description
	}
	if name, sl, err = normalizeNameSlug(name, sl); err != nil {
		return gen.DeviceRole{}, err
	}
	if !colorPattern.MatchString(color) {
		return gen.DeviceRole{}, fault.New(fault.Invalid, "color must be #rrggbb lowercase hex")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.DeviceRole{}, fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	r, err := q.UpdateDeviceRole(ctx, gen.UpdateDeviceRoleParams{ID: cur.ID, Slug: sl, Name: name, Color: color, Description: desc})
	if err != nil {
		return gen.DeviceRole{}, fault.FromDB(err, "device role "+ref)
	}
	if err := changelog.Record(ctx, q, changelog.ActionUpdate, TypeDeviceRole.Key, r.ID, r.Name, deviceRoleSnapshot(cur), deviceRoleSnapshot(r)); err != nil {
		return gen.DeviceRole{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.DeviceRole{}, fmt.Errorf("dcim: commit: %w", err)
	}
	return r, nil
}

// DeleteDeviceRoleByRef removes a device role.
func (s *Service) DeleteDeviceRoleByRef(ctx context.Context, ref string) error {
	cur, err := s.GetDeviceRoleByRef(ctx, ref)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	if _, err := q.DeleteDeviceRole(ctx, cur.ID); err != nil {
		return fault.FromDB(err, "device role "+ref)
	}
	if err := changelog.Record(ctx, q, changelog.ActionDelete, TypeDeviceRole.Key, cur.ID, cur.Name, deviceRoleSnapshot(cur), nil); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("dcim: commit: %w", err)
	}
	return nil
}
