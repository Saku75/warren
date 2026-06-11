package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/saku75/warren/internal/core/fault"
	"github.com/saku75/warren/internal/core/id"
	"github.com/saku75/warren/internal/db/gen"
	"github.com/saku75/warren/internal/dcim"
)

type manufacturerRep struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func manufacturerToRep(m gen.Manufacturer) manufacturerRep {
	return manufacturerRep{
		ID: m.ID.String(), Type: dcim.TypeManufacturer.Key, Slug: m.Slug,
		Name: m.Name, Description: m.Description, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

type manufacturerWrite struct {
	Name        *string `json:"name"`
	Slug        *string `json:"slug"`
	Description *string `json:"description"`
}

func (h *Handler) listManufacturers(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	items, total, err := h.dcim.ListManufacturers(r.Context(), limit, offset)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	reps := make([]manufacturerRep, len(items))
	for i, m := range items {
		reps[i] = manufacturerToRep(m)
	}
	h.writeJSON(w, http.StatusOK, listEnvelope{Items: reps, Total: total, Limit: limit, Offset: offset})
}

func (h *Handler) createManufacturer(w http.ResponseWriter, r *http.Request) {
	var in manufacturerWrite
	if err := h.decode(r, &in); err != nil {
		h.writeError(w, r, err)
		return
	}
	m, err := h.dcim.CreateManufacturer(r.Context(), dcim.ManufacturerInput{
		Name: deref(in.Name), Slug: deref(in.Slug), Description: deref(in.Description),
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, manufacturerToRep(m))
}

func (h *Handler) getManufacturer(w http.ResponseWriter, r *http.Request) {
	m, err := h.dcim.GetManufacturerByRef(r.Context(), chi.URLParam(r, "ref"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, manufacturerToRep(m))
}

func (h *Handler) updateManufacturer(w http.ResponseWriter, r *http.Request) {
	var in manufacturerWrite
	if err := h.decode(r, &in); err != nil {
		h.writeError(w, r, err)
		return
	}
	m, err := h.dcim.UpdateManufacturerByRef(r.Context(), chi.URLParam(r, "ref"), dcim.ManufacturerUpdate{
		Name: in.Name, Slug: in.Slug, Description: in.Description,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, manufacturerToRep(m))
}

func (h *Handler) deleteManufacturer(w http.ResponseWriter, r *http.Request) {
	if err := h.dcim.DeleteManufacturerByRef(r.Context(), chi.URLParam(r, "ref")); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type deviceRoleRep struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Color       string    `json:"color"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func deviceRoleToRep(x gen.DeviceRole) deviceRoleRep {
	return deviceRoleRep{
		ID: x.ID.String(), Type: dcim.TypeDeviceRole.Key, Slug: x.Slug, Name: x.Name,
		Color: x.Color, Description: x.Description, CreatedAt: x.CreatedAt, UpdatedAt: x.UpdatedAt,
	}
}

type deviceRoleWrite struct {
	Name        *string `json:"name"`
	Slug        *string `json:"slug"`
	Color       *string `json:"color"`
	Description *string `json:"description"`
}

func (h *Handler) listDeviceRoles(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	items, total, err := h.dcim.ListDeviceRoles(r.Context(), limit, offset)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	reps := make([]deviceRoleRep, len(items))
	for i, x := range items {
		reps[i] = deviceRoleToRep(x)
	}
	h.writeJSON(w, http.StatusOK, listEnvelope{Items: reps, Total: total, Limit: limit, Offset: offset})
}

func (h *Handler) createDeviceRole(w http.ResponseWriter, r *http.Request) {
	var in deviceRoleWrite
	if err := h.decode(r, &in); err != nil {
		h.writeError(w, r, err)
		return
	}
	x, err := h.dcim.CreateDeviceRole(r.Context(), dcim.DeviceRoleInput{
		Name: deref(in.Name), Slug: deref(in.Slug), Color: deref(in.Color), Description: deref(in.Description),
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, deviceRoleToRep(x))
}

func (h *Handler) getDeviceRole(w http.ResponseWriter, r *http.Request) {
	x, err := h.dcim.GetDeviceRoleByRef(r.Context(), chi.URLParam(r, "ref"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, deviceRoleToRep(x))
}

func (h *Handler) updateDeviceRole(w http.ResponseWriter, r *http.Request) {
	var in deviceRoleWrite
	if err := h.decode(r, &in); err != nil {
		h.writeError(w, r, err)
		return
	}
	x, err := h.dcim.UpdateDeviceRoleByRef(r.Context(), chi.URLParam(r, "ref"), dcim.DeviceRoleUpdate{
		Name: in.Name, Slug: in.Slug, Color: in.Color, Description: in.Description,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, deviceRoleToRep(x))
}

func (h *Handler) deleteDeviceRole(w http.ResponseWriter, r *http.Request) {
	if err := h.dcim.DeleteDeviceRoleByRef(r.Context(), chi.URLParam(r, "ref")); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type deviceTypeRep struct {
	ID           string     `json:"id"`
	Type         string     `json:"type"`
	Slug         string     `json:"slug"`
	SlugPath     string     `json:"slug_path"`
	Model        string     `json:"model"`
	Manufacturer refSummary `json:"manufacturer"`
	PartNumber   string     `json:"part_number"`
	UHeight      float64    `json:"u_height"`
	IsFullDepth  bool       `json:"is_full_depth"`
	Description  string     `json:"description"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (h *Handler) deviceTypeToRep(r *http.Request, dt gen.DeviceType) deviceTypeRep {
	rep := deviceTypeRep{
		ID: dt.ID.String(), Type: dcim.TypeDeviceType.Key, Slug: dt.Slug, Model: dt.Model,
		PartNumber: dt.PartNumber, UHeight: dt.UHeight, IsFullDepth: dt.IsFullDepth,
		Description: dt.Description, CreatedAt: dt.CreatedAt, UpdatedAt: dt.UpdatedAt,
		Manufacturer: refSummary{ID: dt.ManufacturerID.String()},
	}
	if m, err := h.dcim.GetManufacturerByRef(r.Context(), dt.ManufacturerID.String()); err == nil {
		rep.Manufacturer.Slug, rep.Manufacturer.Name = m.Slug, m.Name
		rep.SlugPath = m.Slug + "/" + dt.Slug
	}
	return rep
}

type deviceTypeWrite struct {
	Manufacturer *string  `json:"manufacturer"`
	Model        *string  `json:"model"`
	Slug         *string  `json:"slug"`
	PartNumber   *string  `json:"part_number"`
	UHeight      *float64 `json:"u_height"`
	IsFullDepth  *bool    `json:"is_full_depth"`
	Description  *string  `json:"description"`
}

func (h *Handler) listDeviceTypes(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	items, total, err := h.dcim.ListDeviceTypes(r.Context(), limit, offset)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	reps := make([]deviceTypeRep, len(items))
	for i, row := range items {
		reps[i] = deviceTypeRep{
			ID: row.ID.String(), Type: dcim.TypeDeviceType.Key, Slug: row.Slug,
			SlugPath: row.ManufacturerSlug + "/" + row.Slug, Model: row.Model,
			Manufacturer: refSummary{ID: row.ManufacturerID.String(), Slug: row.ManufacturerSlug, Name: row.ManufacturerName},
			PartNumber:   row.PartNumber, UHeight: row.UHeight, IsFullDepth: row.IsFullDepth,
			Description: row.Description, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		}
	}
	h.writeJSON(w, http.StatusOK, listEnvelope{Items: reps, Total: total, Limit: limit, Offset: offset})
}

func (h *Handler) createDeviceType(w http.ResponseWriter, r *http.Request) {
	var in deviceTypeWrite
	if err := h.decode(r, &in); err != nil {
		h.writeError(w, r, err)
		return
	}
	uh := 1.0
	if in.UHeight != nil {
		uh = *in.UHeight
	}
	fullDepth := true
	if in.IsFullDepth != nil {
		fullDepth = *in.IsFullDepth
	}
	dt, err := h.dcim.CreateDeviceType(r.Context(), dcim.DeviceTypeInput{
		ManufacturerRef: deref(in.Manufacturer), Model: deref(in.Model), Slug: deref(in.Slug),
		PartNumber: deref(in.PartNumber), UHeight: uh, IsFullDepth: fullDepth, Description: deref(in.Description),
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, h.deviceTypeToRep(r, dt))
}

func (h *Handler) getDeviceType(w http.ResponseWriter, r *http.Request) {
	dt, err := h.dcim.GetDeviceTypeByRef(r.Context(), wildcard(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, h.deviceTypeToRep(r, dt))
}

func (h *Handler) updateDeviceType(w http.ResponseWriter, r *http.Request) {
	var in deviceTypeWrite
	if err := h.decode(r, &in); err != nil {
		h.writeError(w, r, err)
		return
	}
	if in.Manufacturer != nil {
		h.writeError(w, r, fault.New(fault.Invalid, "a device type cannot move between manufacturers"))
		return
	}
	dt, err := h.dcim.UpdateDeviceTypeByRef(r.Context(), wildcard(r), dcim.DeviceTypeUpdate{
		Model: in.Model, Slug: in.Slug, PartNumber: in.PartNumber,
		UHeight: in.UHeight, IsFullDepth: in.IsFullDepth, Description: in.Description,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, h.deviceTypeToRep(r, dt))
}

func (h *Handler) deleteDeviceType(w http.ResponseWriter, r *http.Request) {
	if err := h.dcim.DeleteDeviceTypeByRef(r.Context(), wildcard(r)); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type moduleTypeRep struct {
	ID           string     `json:"id"`
	Type         string     `json:"type"`
	Slug         string     `json:"slug"`
	SlugPath     string     `json:"slug_path"`
	Model        string     `json:"model"`
	Manufacturer refSummary `json:"manufacturer"`
	PartNumber   string     `json:"part_number"`
	Description  string     `json:"description"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (h *Handler) moduleTypeToRep(r *http.Request, mt gen.ModuleType) moduleTypeRep {
	rep := moduleTypeRep{
		ID: mt.ID.String(), Type: dcim.TypeModuleType.Key, Slug: mt.Slug, Model: mt.Model,
		PartNumber: mt.PartNumber, Description: mt.Description, CreatedAt: mt.CreatedAt, UpdatedAt: mt.UpdatedAt,
		Manufacturer: refSummary{ID: mt.ManufacturerID.String()},
	}
	if m, err := h.dcim.GetManufacturerByRef(r.Context(), mt.ManufacturerID.String()); err == nil {
		rep.Manufacturer.Slug, rep.Manufacturer.Name = m.Slug, m.Name
		rep.SlugPath = m.Slug + "/" + mt.Slug
	}
	return rep
}

type moduleTypeWrite struct {
	Manufacturer *string `json:"manufacturer"`
	Model        *string `json:"model"`
	Slug         *string `json:"slug"`
	PartNumber   *string `json:"part_number"`
	Description  *string `json:"description"`
}

func (h *Handler) listModuleTypes(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	items, total, err := h.dcim.ListModuleTypes(r.Context(), limit, offset)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	reps := make([]moduleTypeRep, len(items))
	for i, row := range items {
		reps[i] = moduleTypeRep{
			ID: row.ID.String(), Type: dcim.TypeModuleType.Key, Slug: row.Slug,
			SlugPath: row.ManufacturerSlug + "/" + row.Slug, Model: row.Model,
			Manufacturer: refSummary{ID: row.ManufacturerID.String(), Slug: row.ManufacturerSlug, Name: row.ManufacturerName},
			PartNumber:   row.PartNumber, Description: row.Description,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		}
	}
	h.writeJSON(w, http.StatusOK, listEnvelope{Items: reps, Total: total, Limit: limit, Offset: offset})
}

func (h *Handler) createModuleType(w http.ResponseWriter, r *http.Request) {
	var in moduleTypeWrite
	if err := h.decode(r, &in); err != nil {
		h.writeError(w, r, err)
		return
	}
	mt, err := h.dcim.CreateModuleType(r.Context(), dcim.ModuleTypeInput{
		ManufacturerRef: deref(in.Manufacturer), Model: deref(in.Model), Slug: deref(in.Slug),
		PartNumber: deref(in.PartNumber), Description: deref(in.Description),
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, h.moduleTypeToRep(r, mt))
}

func (h *Handler) getModuleType(w http.ResponseWriter, r *http.Request) {
	mt, err := h.dcim.GetModuleTypeByRef(r.Context(), wildcard(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, h.moduleTypeToRep(r, mt))
}

func (h *Handler) updateModuleType(w http.ResponseWriter, r *http.Request) {
	var in moduleTypeWrite
	if err := h.decode(r, &in); err != nil {
		h.writeError(w, r, err)
		return
	}
	if in.Manufacturer != nil {
		h.writeError(w, r, fault.New(fault.Invalid, "a module type cannot move between manufacturers"))
		return
	}
	mt, err := h.dcim.UpdateModuleTypeByRef(r.Context(), wildcard(r), dcim.ModuleTypeUpdate{
		Model: in.Model, Slug: in.Slug, PartNumber: in.PartNumber, Description: in.Description,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, h.moduleTypeToRep(r, mt))
}

func (h *Handler) deleteModuleType(w http.ResponseWriter, r *http.Request) {
	if err := h.dcim.DeleteModuleTypeByRef(r.Context(), wildcard(r)); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type templateRep struct {
	ID          string              `json:"id"`
	Type        string              `json:"type"`
	DeviceType  *refSummary         `json:"device_type"`
	ModuleType  *refSummary         `json:"module_type"`
	Kind        string              `json:"kind"`
	Name        string              `json:"name"`
	Label       string              `json:"label"`
	Attrs       dcim.ComponentAttrs `json:"attrs"`
	Description string              `json:"description"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

func templateToRep(t gen.ComponentTemplate) templateRep {
	attrs, _ := dcim.UnmarshalAttrs(t.Attrs)
	rep := templateRep{
		ID: t.ID.String(), Type: dcim.TypeComponentTemplate.Key, Kind: t.Kind, Name: t.Name,
		Label: t.Label, Attrs: attrs, Description: t.Description,
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
	if t.DeviceTypeID != nil {
		rep.DeviceType = &refSummary{ID: t.DeviceTypeID.String()}
	}
	if t.ModuleTypeID != nil {
		rep.ModuleType = &refSummary{ID: t.ModuleTypeID.String()}
	}
	return rep
}

type templateWrite struct {
	DeviceType  *string              `json:"device_type"`
	ModuleType  *string              `json:"module_type"`
	Kind        *string              `json:"kind"`
	Name        *string              `json:"name"`
	Label       *string              `json:"label"`
	Attrs       *dcim.ComponentAttrs `json:"attrs"`
	Description *string              `json:"description"`
}

// listTemplates requires a ?device_type= or ?module_type= owner filter.
func (h *Handler) listTemplates(w http.ResponseWriter, r *http.Request) {
	dtRef, mtRef := r.URL.Query().Get("device_type"), r.URL.Query().Get("module_type")
	var items []gen.ComponentTemplate
	switch {
	case dtRef != "" && mtRef == "":
		dt, err := h.dcim.GetDeviceTypeByRef(r.Context(), dtRef)
		if err != nil {
			h.writeError(w, r, err)
			return
		}
		if items, err = h.dcim.TemplatesOfDeviceType(r.Context(), dt.ID); err != nil {
			h.writeError(w, r, err)
			return
		}
	case mtRef != "" && dtRef == "":
		mt, err := h.dcim.GetModuleTypeByRef(r.Context(), mtRef)
		if err != nil {
			h.writeError(w, r, err)
			return
		}
		if items, err = h.dcim.TemplatesOfModuleType(r.Context(), mt.ID); err != nil {
			h.writeError(w, r, err)
			return
		}
	default:
		h.writeError(w, r, fault.New(fault.Invalid, "exactly one of ?device_type= or ?module_type= is required"))
		return
	}
	reps := make([]templateRep, len(items))
	for i, t := range items {
		reps[i] = templateToRep(t)
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"items": reps})
}

func (h *Handler) createTemplate(w http.ResponseWriter, r *http.Request) {
	var in templateWrite
	if err := h.decode(r, &in); err != nil {
		h.writeError(w, r, err)
		return
	}
	attrs := dcim.ComponentAttrs{}
	if in.Attrs != nil {
		attrs = *in.Attrs
	}
	t, err := h.dcim.CreateTemplate(r.Context(), dcim.TemplateInput{
		DeviceTypeRef: deref(in.DeviceType), ModuleTypeRef: deref(in.ModuleType),
		Kind: deref(in.Kind), Name: deref(in.Name), Label: deref(in.Label),
		Attrs: attrs, Description: deref(in.Description),
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, templateToRep(t))
}

func (h *Handler) templateID(w http.ResponseWriter, r *http.Request) (id.ID, bool) {
	tid, err := id.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, r, fault.New(fault.Invalid, "templates are addressed by ID"))
		return id.Nil, false
	}
	return tid, true
}

func (h *Handler) getTemplate(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.templateID(w, r)
	if !ok {
		return
	}
	t, err := h.dcim.GetTemplate(r.Context(), tid)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, templateToRep(t))
}

func (h *Handler) updateTemplate(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.templateID(w, r)
	if !ok {
		return
	}
	var in templateWrite
	if err := h.decode(r, &in); err != nil {
		h.writeError(w, r, err)
		return
	}
	if in.DeviceType != nil || in.ModuleType != nil || in.Kind != nil {
		h.writeError(w, r, fault.New(fault.Invalid, "a template's owner and kind are fixed"))
		return
	}
	t, err := h.dcim.UpdateTemplate(r.Context(), tid, dcim.TemplateUpdate{
		Name: in.Name, Label: in.Label, Attrs: in.Attrs, Description: in.Description,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, templateToRep(t))
}

func (h *Handler) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.templateID(w, r)
	if !ok {
		return
	}
	if err := h.dcim.DeleteTemplate(r.Context(), tid); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
