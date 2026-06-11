package dcim

import (
	"context"
	"fmt"
	"strings"

	"github.com/saku75/warren/internal/changelog"
	"github.com/saku75/warren/internal/core/fault"
	"github.com/saku75/warren/internal/core/id"
	"github.com/saku75/warren/internal/core/objtype"
	"github.com/saku75/warren/internal/core/slug"
	"github.com/saku75/warren/internal/db/gen"
)

var (
	// TypeDeviceType: slug scoped to manufacturer ("cisco/c9300-48p").
	TypeDeviceType = objtype.Register(objtype.Type{Key: "dcim.device-type", Name: "Device type", Plural: "Device types"})
	// TypeModuleType: slug scoped to manufacturer ("cisco/c9300-nm-8x").
	TypeModuleType = objtype.Register(objtype.Type{Key: "dcim.module-type", Name: "Module type", Plural: "Module types"})
	// TypeComponentTemplate: addressed by ID (sub-resource of a type).
	TypeComponentTemplate = objtype.Register(objtype.Type{Key: "dcim.component-template", Name: "Component template", Plural: "Component templates"})
)

// DeviceTypeInput are the writable device type fields. An empty Slug is
// derived from Model.
type DeviceTypeInput struct {
	ManufacturerRef string
	Model           string
	Slug            string
	PartNumber      string
	UHeight         float64
	IsFullDepth     bool
	Description     string
}

// DeviceTypeUpdate carries partial changes (manufacturer is fixed).
type DeviceTypeUpdate struct {
	Model       *string
	Slug        *string
	PartNumber  *string
	UHeight     *float64
	IsFullDepth *bool
	Description *string
}

func deviceTypeSnapshot(dt gen.DeviceType) map[string]any {
	return map[string]any{
		"manufacturer_id": dt.ManufacturerID.String(),
		"slug":            dt.Slug, "model": dt.Model, "part_number": dt.PartNumber,
		"u_height": dt.UHeight, "is_full_depth": dt.IsFullDepth, "description": dt.Description,
	}
}

// resolveTypeRef resolves "manufacturer/slug" (or a UUID via lookup) for
// the two catalog type models.
func splitTypeRef(ref, what string) (string, string, error) {
	parts := strings.Split(ref, "/")
	if len(parts) != 2 {
		return "", "", fault.New(fault.Invalid, "%s reference %q must be an ID or manufacturer/slug", what, ref)
	}
	for _, p := range parts {
		if err := slug.Validate(p); err != nil {
			return "", "", fault.Wrap(fault.Invalid, err, "invalid path segment %q", p)
		}
	}
	return parts[0], parts[1], nil
}

// CreateDeviceType makes a device type.
func (s *Service) CreateDeviceType(ctx context.Context, in DeviceTypeInput) (gen.DeviceType, error) {
	model, sl, err := normalizeNameSlug(in.Model, in.Slug)
	if err != nil {
		return gen.DeviceType{}, err
	}
	if in.UHeight < 0 {
		return gen.DeviceType{}, fault.New(fault.Invalid, "u_height must not be negative")
	}
	if in.ManufacturerRef == "" {
		return gen.DeviceType{}, fault.New(fault.Invalid, "manufacturer is required")
	}
	m, err := s.GetManufacturerByRef(ctx, in.ManufacturerRef)
	if err != nil {
		return gen.DeviceType{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.DeviceType{}, fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	dt, err := q.CreateDeviceType(ctx, gen.CreateDeviceTypeParams{
		ID: id.New(), ManufacturerID: m.ID, Slug: sl, Model: model,
		PartNumber: in.PartNumber, UHeight: in.UHeight, IsFullDepth: in.IsFullDepth,
		Description: in.Description,
	})
	if err != nil {
		return gen.DeviceType{}, fault.FromDB(err, "device type "+m.Slug+"/"+sl)
	}
	if err := changelog.Record(ctx, q, changelog.ActionCreate, TypeDeviceType.Key, dt.ID, dt.Model, nil, deviceTypeSnapshot(dt)); err != nil {
		return gen.DeviceType{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.DeviceType{}, fmt.Errorf("dcim: commit: %w", err)
	}
	return dt, nil
}

// GetDeviceTypeByRef resolves an ID or "manufacturer/slug" path.
func (s *Service) GetDeviceTypeByRef(ctx context.Context, ref string) (gen.DeviceType, error) {
	if tid, err := id.Parse(ref); err == nil {
		dt, err := s.q.GetDeviceType(ctx, tid)
		return dt, fault.FromDB(err, "device type "+ref)
	}
	mSlug, tSlug, err := splitTypeRef(ref, "device type")
	if err != nil {
		return gen.DeviceType{}, err
	}
	m, err := s.GetManufacturerByRef(ctx, mSlug)
	if err != nil {
		return gen.DeviceType{}, err
	}
	dt, err := s.q.GetDeviceTypeByScope(ctx, gen.GetDeviceTypeByScopeParams{ManufacturerID: m.ID, Slug: tSlug})
	return dt, fault.FromDB(err, "device type "+ref)
}

// ListDeviceTypes returns device types (with manufacturer display
// fields) plus the total count.
func (s *Service) ListDeviceTypes(ctx context.Context, limit, offset int32) ([]gen.ListDeviceTypesRow, int64, error) {
	total, err := s.q.CountDeviceTypes(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("dcim: count device types: %w", err)
	}
	items, err := s.q.ListDeviceTypes(ctx, gen.ListDeviceTypesParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, fmt.Errorf("dcim: list device types: %w", err)
	}
	return items, total, nil
}

// DeviceTypesOfManufacturer lists a manufacturer's device types.
func (s *Service) DeviceTypesOfManufacturer(ctx context.Context, manufacturerID id.ID) ([]gen.DeviceType, error) {
	items, err := s.q.ListDeviceTypesByManufacturer(ctx, manufacturerID)
	if err != nil {
		return nil, fmt.Errorf("dcim: list device types: %w", err)
	}
	return items, nil
}

// UpdateDeviceTypeByRef applies a partial update.
func (s *Service) UpdateDeviceTypeByRef(ctx context.Context, ref string, up DeviceTypeUpdate) (gen.DeviceType, error) {
	cur, err := s.GetDeviceTypeByRef(ctx, ref)
	if err != nil {
		return gen.DeviceType{}, err
	}
	next := gen.UpdateDeviceTypeParams{
		ID: cur.ID, Slug: cur.Slug, Model: cur.Model, PartNumber: cur.PartNumber,
		UHeight: cur.UHeight, IsFullDepth: cur.IsFullDepth, Description: cur.Description,
	}
	if up.Model != nil {
		next.Model = *up.Model
	}
	if up.Slug != nil {
		next.Slug = *up.Slug
	}
	if up.PartNumber != nil {
		next.PartNumber = *up.PartNumber
	}
	if up.UHeight != nil {
		next.UHeight = *up.UHeight
	}
	if up.IsFullDepth != nil {
		next.IsFullDepth = *up.IsFullDepth
	}
	if up.Description != nil {
		next.Description = *up.Description
	}
	if next.Model, next.Slug, err = normalizeNameSlug(next.Model, next.Slug); err != nil {
		return gen.DeviceType{}, err
	}
	if next.UHeight < 0 {
		return gen.DeviceType{}, fault.New(fault.Invalid, "u_height must not be negative")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.DeviceType{}, fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	dt, err := q.UpdateDeviceType(ctx, next)
	if err != nil {
		return gen.DeviceType{}, fault.FromDB(err, "device type "+ref)
	}
	if err := changelog.Record(ctx, q, changelog.ActionUpdate, TypeDeviceType.Key, dt.ID, dt.Model, deviceTypeSnapshot(cur), deviceTypeSnapshot(dt)); err != nil {
		return gen.DeviceType{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.DeviceType{}, fmt.Errorf("dcim: commit: %w", err)
	}
	return dt, nil
}

// DeleteDeviceTypeByRef removes a device type and its templates.
func (s *Service) DeleteDeviceTypeByRef(ctx context.Context, ref string) error {
	cur, err := s.GetDeviceTypeByRef(ctx, ref)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	if _, err := q.DeleteDeviceType(ctx, cur.ID); err != nil {
		return fault.FromDB(err, "device type "+ref)
	}
	if err := changelog.Record(ctx, q, changelog.ActionDelete, TypeDeviceType.Key, cur.ID, cur.Model, deviceTypeSnapshot(cur), nil); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("dcim: commit: %w", err)
	}
	return nil
}

// ModuleTypeInput are the writable module type fields.
type ModuleTypeInput struct {
	ManufacturerRef string
	Model           string
	Slug            string
	PartNumber      string
	Description     string
}

// ModuleTypeUpdate carries partial changes (manufacturer is fixed).
type ModuleTypeUpdate struct {
	Model       *string
	Slug        *string
	PartNumber  *string
	Description *string
}

func moduleTypeSnapshot(mt gen.ModuleType) map[string]any {
	return map[string]any{
		"manufacturer_id": mt.ManufacturerID.String(),
		"slug":            mt.Slug, "model": mt.Model, "part_number": mt.PartNumber,
		"description": mt.Description,
	}
}

// CreateModuleType makes a module type.
func (s *Service) CreateModuleType(ctx context.Context, in ModuleTypeInput) (gen.ModuleType, error) {
	model, sl, err := normalizeNameSlug(in.Model, in.Slug)
	if err != nil {
		return gen.ModuleType{}, err
	}
	if in.ManufacturerRef == "" {
		return gen.ModuleType{}, fault.New(fault.Invalid, "manufacturer is required")
	}
	m, err := s.GetManufacturerByRef(ctx, in.ManufacturerRef)
	if err != nil {
		return gen.ModuleType{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.ModuleType{}, fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	mt, err := q.CreateModuleType(ctx, gen.CreateModuleTypeParams{
		ID: id.New(), ManufacturerID: m.ID, Slug: sl, Model: model,
		PartNumber: in.PartNumber, Description: in.Description,
	})
	if err != nil {
		return gen.ModuleType{}, fault.FromDB(err, "module type "+m.Slug+"/"+sl)
	}
	if err := changelog.Record(ctx, q, changelog.ActionCreate, TypeModuleType.Key, mt.ID, mt.Model, nil, moduleTypeSnapshot(mt)); err != nil {
		return gen.ModuleType{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.ModuleType{}, fmt.Errorf("dcim: commit: %w", err)
	}
	return mt, nil
}

// GetModuleTypeByRef resolves an ID or "manufacturer/slug" path.
func (s *Service) GetModuleTypeByRef(ctx context.Context, ref string) (gen.ModuleType, error) {
	if tid, err := id.Parse(ref); err == nil {
		mt, err := s.q.GetModuleType(ctx, tid)
		return mt, fault.FromDB(err, "module type "+ref)
	}
	mSlug, tSlug, err := splitTypeRef(ref, "module type")
	if err != nil {
		return gen.ModuleType{}, err
	}
	m, err := s.GetManufacturerByRef(ctx, mSlug)
	if err != nil {
		return gen.ModuleType{}, err
	}
	mt, err := s.q.GetModuleTypeByScope(ctx, gen.GetModuleTypeByScopeParams{ManufacturerID: m.ID, Slug: tSlug})
	return mt, fault.FromDB(err, "module type "+ref)
}

// ListModuleTypes returns module types (with manufacturer display
// fields) plus the total count.
func (s *Service) ListModuleTypes(ctx context.Context, limit, offset int32) ([]gen.ListModuleTypesRow, int64, error) {
	total, err := s.q.CountModuleTypes(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("dcim: count module types: %w", err)
	}
	items, err := s.q.ListModuleTypes(ctx, gen.ListModuleTypesParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, fmt.Errorf("dcim: list module types: %w", err)
	}
	return items, total, nil
}

// ModuleTypesOfManufacturer lists a manufacturer's module types.
func (s *Service) ModuleTypesOfManufacturer(ctx context.Context, manufacturerID id.ID) ([]gen.ModuleType, error) {
	items, err := s.q.ListModuleTypesByManufacturer(ctx, manufacturerID)
	if err != nil {
		return nil, fmt.Errorf("dcim: list module types: %w", err)
	}
	return items, nil
}

// UpdateModuleTypeByRef applies a partial update.
func (s *Service) UpdateModuleTypeByRef(ctx context.Context, ref string, up ModuleTypeUpdate) (gen.ModuleType, error) {
	cur, err := s.GetModuleTypeByRef(ctx, ref)
	if err != nil {
		return gen.ModuleType{}, err
	}
	next := gen.UpdateModuleTypeParams{
		ID: cur.ID, Slug: cur.Slug, Model: cur.Model,
		PartNumber: cur.PartNumber, Description: cur.Description,
	}
	if up.Model != nil {
		next.Model = *up.Model
	}
	if up.Slug != nil {
		next.Slug = *up.Slug
	}
	if up.PartNumber != nil {
		next.PartNumber = *up.PartNumber
	}
	if up.Description != nil {
		next.Description = *up.Description
	}
	if next.Model, next.Slug, err = normalizeNameSlug(next.Model, next.Slug); err != nil {
		return gen.ModuleType{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.ModuleType{}, fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	mt, err := q.UpdateModuleType(ctx, next)
	if err != nil {
		return gen.ModuleType{}, fault.FromDB(err, "module type "+ref)
	}
	if err := changelog.Record(ctx, q, changelog.ActionUpdate, TypeModuleType.Key, mt.ID, mt.Model, moduleTypeSnapshot(cur), moduleTypeSnapshot(mt)); err != nil {
		return gen.ModuleType{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.ModuleType{}, fmt.Errorf("dcim: commit: %w", err)
	}
	return mt, nil
}

// DeleteModuleTypeByRef removes a module type and its templates.
func (s *Service) DeleteModuleTypeByRef(ctx context.Context, ref string) error {
	cur, err := s.GetModuleTypeByRef(ctx, ref)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	if _, err := q.DeleteModuleType(ctx, cur.ID); err != nil {
		return fault.FromDB(err, "module type "+ref)
	}
	if err := changelog.Record(ctx, q, changelog.ActionDelete, TypeModuleType.Key, cur.ID, cur.Model, moduleTypeSnapshot(cur), nil); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("dcim: commit: %w", err)
	}
	return nil
}

// TemplateInput are the writable component template fields. Exactly one
// of DeviceTypeRef / ModuleTypeRef must be set.
type TemplateInput struct {
	DeviceTypeRef string
	ModuleTypeRef string
	Kind          string
	Name          string
	Label         string
	Attrs         ComponentAttrs
	Description   string
}

// TemplateUpdate carries partial changes (owner and kind are fixed).
type TemplateUpdate struct {
	Name        *string
	Label       *string
	Attrs       *ComponentAttrs
	Description *string
}

func templateSnapshot(t gen.ComponentTemplate) map[string]any {
	snap := map[string]any{"kind": t.Kind, "name": t.Name, "label": t.Label, "description": t.Description}
	if attrs, err := UnmarshalAttrs(t.Attrs); err == nil {
		snap["attrs"] = attrs
	}
	if t.DeviceTypeID != nil {
		snap["device_type_id"] = t.DeviceTypeID.String()
	}
	if t.ModuleTypeID != nil {
		snap["module_type_id"] = t.ModuleTypeID.String()
	}
	return snap
}

// CreateTemplate adds a component template to a device or module type.
func (s *Service) CreateTemplate(ctx context.Context, in TemplateInput) (gen.ComponentTemplate, error) {
	if in.Name == "" {
		return gen.ComponentTemplate{}, fault.New(fault.Invalid, "name is required")
	}
	if !validComponentKind(in.Kind) {
		return gen.ComponentTemplate{}, fault.New(fault.Invalid, "invalid kind %q (allowed: %v)", in.Kind, ComponentKinds)
	}
	if err := in.Attrs.Validate(in.Kind); err != nil {
		return gen.ComponentTemplate{}, err
	}
	if (in.DeviceTypeRef == "") == (in.ModuleTypeRef == "") {
		return gen.ComponentTemplate{}, fault.New(fault.Invalid, "exactly one of device_type or module_type is required")
	}

	var deviceTypeID, moduleTypeID *id.ID
	var label string
	if in.DeviceTypeRef != "" {
		dt, err := s.GetDeviceTypeByRef(ctx, in.DeviceTypeRef)
		if err != nil {
			return gen.ComponentTemplate{}, err
		}
		deviceTypeID, label = &dt.ID, dt.Model+" / "+in.Name
	} else {
		mt, err := s.GetModuleTypeByRef(ctx, in.ModuleTypeRef)
		if err != nil {
			return gen.ComponentTemplate{}, err
		}
		moduleTypeID, label = &mt.ID, mt.Model+" / "+in.Name
	}

	attrs, err := in.Attrs.Marshal()
	if err != nil {
		return gen.ComponentTemplate{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.ComponentTemplate{}, fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	t, err := q.CreateComponentTemplate(ctx, gen.CreateComponentTemplateParams{
		ID: id.New(), DeviceTypeID: deviceTypeID, ModuleTypeID: moduleTypeID,
		Kind: in.Kind, Name: in.Name, Label: in.Label, Attrs: attrs, Description: in.Description,
	})
	if err != nil {
		return gen.ComponentTemplate{}, fault.FromDB(err, "template "+in.Name)
	}
	if err := changelog.Record(ctx, q, changelog.ActionCreate, TypeComponentTemplate.Key, t.ID, label, nil, templateSnapshot(t)); err != nil {
		return gen.ComponentTemplate{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.ComponentTemplate{}, fmt.Errorf("dcim: commit: %w", err)
	}
	return t, nil
}

// TemplatesOfDeviceType lists a device type's templates.
func (s *Service) TemplatesOfDeviceType(ctx context.Context, deviceTypeID id.ID) ([]gen.ComponentTemplate, error) {
	items, err := s.q.ListComponentTemplatesByDeviceType(ctx, &deviceTypeID)
	if err != nil {
		return nil, fmt.Errorf("dcim: list templates: %w", err)
	}
	return items, nil
}

// TemplatesOfModuleType lists a module type's templates.
func (s *Service) TemplatesOfModuleType(ctx context.Context, moduleTypeID id.ID) ([]gen.ComponentTemplate, error) {
	items, err := s.q.ListComponentTemplatesByModuleType(ctx, &moduleTypeID)
	if err != nil {
		return nil, fmt.Errorf("dcim: list templates: %w", err)
	}
	return items, nil
}

// GetTemplate fetches one template by ID.
func (s *Service) GetTemplate(ctx context.Context, templateID id.ID) (gen.ComponentTemplate, error) {
	t, err := s.q.GetComponentTemplate(ctx, templateID)
	return t, fault.FromDB(err, "template "+templateID.String())
}

// UpdateTemplate applies a partial update.
func (s *Service) UpdateTemplate(ctx context.Context, templateID id.ID, up TemplateUpdate) (gen.ComponentTemplate, error) {
	cur, err := s.GetTemplate(ctx, templateID)
	if err != nil {
		return gen.ComponentTemplate{}, err
	}
	curAttrs, err := UnmarshalAttrs(cur.Attrs)
	if err != nil {
		return gen.ComponentTemplate{}, err
	}
	name, label, attrs, desc := cur.Name, cur.Label, curAttrs, cur.Description
	if up.Name != nil {
		name = *up.Name
	}
	if up.Label != nil {
		label = *up.Label
	}
	if up.Attrs != nil {
		attrs = *up.Attrs
	}
	if up.Description != nil {
		desc = *up.Description
	}
	if name == "" {
		return gen.ComponentTemplate{}, fault.New(fault.Invalid, "name is required")
	}
	if err := attrs.Validate(cur.Kind); err != nil {
		return gen.ComponentTemplate{}, err
	}
	attrsJSON, err := attrs.Marshal()
	if err != nil {
		return gen.ComponentTemplate{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return gen.ComponentTemplate{}, fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	t, err := q.UpdateComponentTemplate(ctx, gen.UpdateComponentTemplateParams{
		ID: cur.ID, Name: name, Label: label, Attrs: attrsJSON, Description: desc,
	})
	if err != nil {
		return gen.ComponentTemplate{}, fault.FromDB(err, "template "+name)
	}
	if err := changelog.Record(ctx, q, changelog.ActionUpdate, TypeComponentTemplate.Key, t.ID, t.Name, templateSnapshot(cur), templateSnapshot(t)); err != nil {
		return gen.ComponentTemplate{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.ComponentTemplate{}, fmt.Errorf("dcim: commit: %w", err)
	}
	return t, nil
}

// DeleteTemplate removes a component template.
func (s *Service) DeleteTemplate(ctx context.Context, templateID id.ID) error {
	cur, err := s.GetTemplate(ctx, templateID)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("dcim: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	if _, err := q.DeleteComponentTemplate(ctx, cur.ID); err != nil {
		return fault.FromDB(err, "template "+cur.Name)
	}
	if err := changelog.Record(ctx, q, changelog.ActionDelete, TypeComponentTemplate.Key, cur.ID, cur.Name, templateSnapshot(cur), nil); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("dcim: commit: %w", err)
	}
	return nil
}
