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

type rackRep struct {
	ID          string      `json:"id"`
	Type        string      `json:"type"`
	Slug        string      `json:"slug"`
	SlugPath    string      `json:"slug_path"`
	Name        string      `json:"name"`
	Status      string      `json:"status"`
	UHeight     int32       `json:"u_height"`
	Site        refSummary  `json:"site"`
	Tenant      *refSummary `json:"tenant"`
	Description string      `json:"description"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

func (h *Handler) rackToRep(r *http.Request, x gen.Rack) rackRep {
	rep := rackRep{
		ID: x.ID.String(), Type: dcim.TypeRack.Key, Slug: x.Slug, Name: x.Name,
		Status: x.Status, UHeight: x.UHeight, Description: x.Description,
		CreatedAt: x.CreatedAt, UpdatedAt: x.UpdatedAt,
		Site: refSummary{ID: x.SiteID.String()},
	}
	if s, err := h.dcim.GetSiteByRef(r.Context(), x.SiteID.String()); err == nil {
		rep.Site.Slug, rep.Site.Name = s.Slug, s.Name
		rep.SlugPath = s.Slug + "/" + x.Slug
	}
	if x.TenantID != nil {
		rep.Tenant = &refSummary{ID: x.TenantID.String()}
	}
	return rep
}

type rackWrite struct {
	Site        *string `json:"site"`
	Location    *string `json:"location"`
	Name        *string `json:"name"`
	Slug        *string `json:"slug"`
	Status      *string `json:"status"`
	UHeight     *int32  `json:"u_height"`
	Tenant      *string `json:"tenant"`
	Description *string `json:"description"`
}

func (h *Handler) listRacks(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	items, total, err := h.dcim.ListRacks(r.Context(), limit, offset)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	reps := make([]rackRep, len(items))
	for i, x := range items {
		rep := rackRep{
			ID: x.ID.String(), Type: dcim.TypeRack.Key, Slug: x.Slug, Name: x.Name,
			Status: x.Status, UHeight: x.UHeight, Description: x.Description,
			CreatedAt: x.CreatedAt, UpdatedAt: x.UpdatedAt,
			Site:     refSummary{ID: x.SiteID.String(), Slug: x.SiteSlug, Name: x.SiteName},
			SlugPath: x.SiteSlug + "/" + x.Slug,
		}
		if x.TenantID != nil {
			rep.Tenant = &refSummary{ID: x.TenantID.String()}
		}
		reps[i] = rep
	}
	h.writeJSON(w, http.StatusOK, listEnvelope{Items: reps, Total: total, Limit: limit, Offset: offset})
}

func (h *Handler) createRack(w http.ResponseWriter, r *http.Request) {
	var in rackWrite
	if err := h.decode(r, &in); err != nil {
		h.writeError(w, r, err)
		return
	}
	var uh int32
	if in.UHeight != nil {
		uh = *in.UHeight
	}
	x, err := h.dcim.CreateRack(r.Context(), dcim.RackInput{
		SiteRef: deref(in.Site), LocationRef: deref(in.Location), Name: deref(in.Name),
		Slug: deref(in.Slug), Status: deref(in.Status), UHeight: uh,
		TenantRef: deref(in.Tenant), Description: deref(in.Description),
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, h.rackToRep(r, x))
}

func (h *Handler) getRack(w http.ResponseWriter, r *http.Request) {
	x, err := h.dcim.GetRackByRef(r.Context(), wildcard(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, h.rackToRep(r, x))
}

func (h *Handler) updateRack(w http.ResponseWriter, r *http.Request) {
	var in rackWrite
	if err := h.decode(r, &in); err != nil {
		h.writeError(w, r, err)
		return
	}
	if in.Site != nil {
		h.writeError(w, r, fault.New(fault.Invalid, "a rack cannot move between sites"))
		return
	}
	x, err := h.dcim.UpdateRackByRef(r.Context(), wildcard(r), dcim.RackUpdate{
		Name: in.Name, Slug: in.Slug, Status: in.Status, UHeight: in.UHeight,
		LocationRef: in.Location, TenantRef: in.Tenant, Description: in.Description,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, h.rackToRep(r, x))
}

func (h *Handler) deleteRack(w http.ResponseWriter, r *http.Request) {
	if err := h.dcim.DeleteRackByRef(r.Context(), wildcard(r)); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type deviceRep struct {
	ID          string      `json:"id"`
	Type        string      `json:"type"`
	Slug        string      `json:"slug"`
	SlugPath    string      `json:"slug_path"`
	Name        string      `json:"name"`
	Status      string      `json:"status"`
	Site        refSummary  `json:"site"`
	DeviceType  refSummary  `json:"device_type"`
	Role        refSummary  `json:"role"`
	Rack        *refSummary `json:"rack"`
	Position    *float64    `json:"position"`
	Face        *string     `json:"face"`
	Tenant      *refSummary `json:"tenant"`
	Serial      string      `json:"serial"`
	AssetTag    string      `json:"asset_tag"`
	Description string      `json:"description"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

func (h *Handler) deviceToRep(r *http.Request, d gen.Device) deviceRep {
	rep := deviceRep{
		ID: d.ID.String(), Type: dcim.TypeDevice.Key, Slug: d.Slug, Name: d.Name,
		Status: d.Status, Serial: d.Serial, AssetTag: d.AssetTag, Description: d.Description,
		Position: d.Position, Face: d.Face,
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
		Site:       refSummary{ID: d.SiteID.String()},
		DeviceType: refSummary{ID: d.DeviceTypeID.String()},
		Role:       refSummary{ID: d.RoleID.String()},
	}
	if s, err := h.dcim.GetSiteByRef(r.Context(), d.SiteID.String()); err == nil {
		rep.Site.Slug, rep.Site.Name = s.Slug, s.Name
		rep.SlugPath = s.Slug + "/" + d.Slug
	}
	if dt, err := h.dcim.GetDeviceTypeByRef(r.Context(), d.DeviceTypeID.String()); err == nil {
		rep.DeviceType.Slug, rep.DeviceType.Name = dt.Slug, dt.Model
	}
	if role, err := h.dcim.GetDeviceRoleByRef(r.Context(), d.RoleID.String()); err == nil {
		rep.Role.Slug, rep.Role.Name = role.Slug, role.Name
	}
	if d.RackID != nil {
		rep.Rack = &refSummary{ID: d.RackID.String()}
	}
	if d.TenantID != nil {
		rep.Tenant = &refSummary{ID: d.TenantID.String()}
	}
	return rep
}

type deviceWrite struct {
	Site        *string  `json:"site"`
	Location    *string  `json:"location"`
	Rack        *string  `json:"rack"`
	Position    *float64 `json:"position"`
	Face        *string  `json:"face"`
	DeviceType  *string  `json:"device_type"`
	Role        *string  `json:"role"`
	Tenant      *string  `json:"tenant"`
	Name        *string  `json:"name"`
	Slug        *string  `json:"slug"`
	Status      *string  `json:"status"`
	Serial      *string  `json:"serial"`
	AssetTag    *string  `json:"asset_tag"`
	Description *string  `json:"description"`
}

func (h *Handler) listDevices(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	items, total, err := h.dcim.ListDevices(r.Context(), limit, offset)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	reps := make([]deviceRep, len(items))
	for i, d := range items {
		reps[i] = deviceRep{
			ID: d.ID.String(), Type: dcim.TypeDevice.Key, Slug: d.Slug, Name: d.Name,
			Status: d.Status, Serial: d.Serial, AssetTag: d.AssetTag, Description: d.Description,
			Position: d.Position, Face: d.Face,
			CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
			SlugPath:   d.SiteSlug + "/" + d.Slug,
			Site:       refSummary{ID: d.SiteID.String(), Slug: d.SiteSlug, Name: d.SiteName},
			DeviceType: refSummary{ID: d.DeviceTypeID.String(), Name: d.TypeModel},
			Role:       refSummary{ID: d.RoleID.String(), Name: d.RoleName},
		}
	}
	h.writeJSON(w, http.StatusOK, listEnvelope{Items: reps, Total: total, Limit: limit, Offset: offset})
}

func (h *Handler) createDevice(w http.ResponseWriter, r *http.Request) {
	var in deviceWrite
	if err := h.decode(r, &in); err != nil {
		h.writeError(w, r, err)
		return
	}
	var pos float64
	if in.Position != nil {
		pos = *in.Position
	}
	d, err := h.dcim.CreateDevice(r.Context(), dcim.DeviceInput{
		SiteRef: deref(in.Site), LocationRef: deref(in.Location), RackRef: deref(in.Rack),
		Position: pos, Face: deref(in.Face), DeviceTypeRef: deref(in.DeviceType),
		RoleRef: deref(in.Role), TenantRef: deref(in.Tenant), Name: deref(in.Name),
		Slug: deref(in.Slug), Status: deref(in.Status), Serial: deref(in.Serial),
		AssetTag: deref(in.AssetTag), Description: deref(in.Description),
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, h.deviceToRep(r, d))
}

func (h *Handler) getDevice(w http.ResponseWriter, r *http.Request) {
	d, err := h.dcim.GetDeviceByRef(r.Context(), wildcard(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, h.deviceToRep(r, d))
}

func (h *Handler) updateDevice(w http.ResponseWriter, r *http.Request) {
	var in deviceWrite
	if err := h.decode(r, &in); err != nil {
		h.writeError(w, r, err)
		return
	}
	if in.Site != nil || in.DeviceType != nil {
		h.writeError(w, r, fault.New(fault.Invalid, "a device's site and device type are fixed"))
		return
	}
	d, err := h.dcim.UpdateDeviceByRef(r.Context(), wildcard(r), dcim.DeviceUpdate{
		Name: in.Name, Slug: in.Slug, Status: in.Status, LocationRef: in.Location,
		RackRef: in.Rack, Position: in.Position, Face: in.Face, RoleRef: in.Role,
		TenantRef: in.Tenant, Serial: in.Serial, AssetTag: in.AssetTag, Description: in.Description,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, h.deviceToRep(r, d))
}

func (h *Handler) deleteDevice(w http.ResponseWriter, r *http.Request) {
	if err := h.dcim.DeleteDeviceByRef(r.Context(), wildcard(r)); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type componentRep struct {
	ID          string              `json:"id"`
	Type        string              `json:"type"`
	Kind        string              `json:"kind"`
	Name        string              `json:"name"`
	Label       string              `json:"label"`
	Attrs       dcim.ComponentAttrs `json:"attrs"`
	ModuleID    *string             `json:"module_id"`
	Description string              `json:"description"`
}

// listComponents requires ?device=<ref>.
func (h *Handler) listComponents(w http.ResponseWriter, r *http.Request) {
	devRef := r.URL.Query().Get("device")
	if devRef == "" {
		h.writeError(w, r, fault.New(fault.Invalid, "?device= is required"))
		return
	}
	d, err := h.dcim.GetDeviceByRef(r.Context(), devRef)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	items, err := h.dcim.ComponentsOfDevice(r.Context(), d.ID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	reps := make([]componentRep, len(items))
	for i, c := range items {
		attrs, _ := dcim.UnmarshalAttrs(c.Attrs)
		rep := componentRep{
			ID: c.ID.String(), Type: dcim.TypeComponent.Key, Kind: c.Kind, Name: c.Name,
			Label: c.Label, Attrs: attrs, Description: c.Description,
		}
		if c.ModuleID != nil {
			mid := c.ModuleID.String()
			rep.ModuleID = &mid
		}
		reps[i] = rep
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"items": reps})
}

type moduleRep struct {
	ID         string     `json:"id"`
	Type       string     `json:"type"`
	Bay        refSummary `json:"bay"`
	ModuleType refSummary `json:"module_type"`
	Serial     string     `json:"serial"`
	CreatedAt  time.Time  `json:"created_at"`
}

type moduleWrite struct {
	Device     *string `json:"device"`
	Bay        *string `json:"bay"`
	ModuleType *string `json:"module_type"`
	Serial     *string `json:"serial"`
}

// listModules requires ?device=<ref>.
func (h *Handler) listModules(w http.ResponseWriter, r *http.Request) {
	devRef := r.URL.Query().Get("device")
	if devRef == "" {
		h.writeError(w, r, fault.New(fault.Invalid, "?device= is required"))
		return
	}
	d, err := h.dcim.GetDeviceByRef(r.Context(), devRef)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	items, err := h.dcim.ModulesOfDevice(r.Context(), d.ID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	reps := make([]moduleRep, len(items))
	for i, m := range items {
		reps[i] = moduleRep{
			ID: m.ID.String(), Type: dcim.TypeModule.Key, Serial: m.Serial, CreatedAt: m.CreatedAt,
			Bay:        refSummary{ID: m.BayID.String(), Name: m.BayName},
			ModuleType: refSummary{ID: m.ModuleTypeID.String(), Name: m.TypeModel},
		}
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"items": reps})
}

func (h *Handler) installModule(w http.ResponseWriter, r *http.Request) {
	var in moduleWrite
	if err := h.decode(r, &in); err != nil {
		h.writeError(w, r, err)
		return
	}
	m, err := h.dcim.InstallModule(r.Context(), dcim.ModuleInput{
		DeviceRef: deref(in.Device), BayID: deref(in.Bay),
		ModuleTypeRef: deref(in.ModuleType), Serial: deref(in.Serial),
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, moduleRep{
		ID: m.ID.String(), Type: dcim.TypeModule.Key, Serial: m.Serial, CreatedAt: m.CreatedAt,
		Bay:        refSummary{ID: m.BayID.String()},
		ModuleType: refSummary{ID: m.ModuleTypeID.String()},
	})
}

func (h *Handler) removeModule(w http.ResponseWriter, r *http.Request) {
	mid, err := id.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, r, fault.New(fault.Invalid, "modules are addressed by ID"))
		return
	}
	if err := h.dcim.RemoveModule(r.Context(), mid); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
