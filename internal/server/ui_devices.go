package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/saku75/warren/internal/core/id"
	"github.com/saku75/warren/internal/db/gen"
	"github.com/saku75/warren/internal/dcim"
	"github.com/saku75/warren/internal/web"
)

func (u *uiHandler) deviceRoutes(r chi.Router) {
	r.Get("/dcim/racks", u.racksPage)
	r.Post("/dcim/racks", u.createRack)
	r.Get("/dcim/racks/new", u.rackFormPage)
	r.Get("/dcim/racks/*", u.rackDetail)
	r.Post("/dcim/racks/*", u.updateRack)
	r.Delete("/dcim/racks/*", u.deleteRack)

	// "<ref>/modules" POSTs install a module on that device.
	r.Get("/dcim/devices", u.devicesPage)
	r.Post("/dcim/devices", u.createDevice)
	r.Get("/dcim/devices/new", u.deviceFormPage)
	r.Get("/dcim/devices/*", u.deviceDetail)
	r.Post("/dcim/devices/*", u.updateDeviceOrInstallModule)
	r.Delete("/dcim/devices/*", u.deleteDevice)

	r.Delete("/dcim/modules/{id}", u.removeModule)
}

// --- racks ---

func (u *uiHandler) racksPage(w http.ResponseWriter, r *http.Request) {
	items, _, err := u.dcim.ListRacks(r.Context(), uiListLimit, 0)
	if err != nil {
		st, msg := u.fail(r, err)
		u.render(w, r, st, web.ErrorPage(msg))
		return
	}
	u.render(w, r, http.StatusOK, web.RacksPage(items, flashErr(r)))
}

func (u *uiHandler) renderRackForm(w http.ResponseWriter, r *http.Request, status int, in dcim.RackInput, errMsg string) {
	sites, _, err := u.dcim.ListSites(r.Context(), uiListLimit, 0)
	if err != nil {
		st, msg := u.fail(r, err)
		u.render(w, r, st, web.ErrorPage(msg))
		return
	}
	u.render(w, r, status, web.RackFormPage(in, sites, errMsg))
}

func (u *uiHandler) rackFormPage(w http.ResponseWriter, r *http.Request) {
	u.renderRackForm(w, r, http.StatusOK, dcim.RackInput{Status: "active", UHeight: 42}, "")
}

func rackInputFromForm(r *http.Request) dcim.RackInput {
	uh, _ := strconv.ParseInt(formValue(r, "u_height"), 10, 32)
	return dcim.RackInput{
		SiteRef: formValue(r, "site"), Name: formValue(r, "name"), Slug: formValue(r, "slug"),
		Status: formValue(r, "status"), UHeight: int32(uh), Description: formValue(r, "description"),
	}
}

func (u *uiHandler) createRack(w http.ResponseWriter, r *http.Request) {
	in := rackInputFromForm(r)
	rack, err := u.dcim.CreateRack(r.Context(), in)
	if err != nil {
		status, msg := u.fail(r, err)
		u.renderRackForm(w, r, status, in, msg)
		return
	}
	http.Redirect(w, r, "/dcim/racks/"+in.SiteRef+"/"+rack.Slug, http.StatusSeeOther)
}

func (u *uiHandler) renderRackDetail(w http.ResponseWriter, r *http.Request, status int, rack gen.Rack, errMsg string) {
	site, err := u.dcim.GetSiteByRef(r.Context(), rack.SiteID.String())
	if err == nil {
		var mounted []gen.ListDevicesByRackRow
		mounted, err = u.dcim.DevicesInRack(r.Context(), rack.ID)
		if err == nil {
			u.render(w, r, status, web.RackDetailPage(rack, site, mounted, errMsg))
			return
		}
	}
	st, msg := u.fail(r, err)
	u.render(w, r, st, web.ErrorPage(msg))
}

func (u *uiHandler) rackDetail(w http.ResponseWriter, r *http.Request) {
	rack, err := u.dcim.GetRackByRef(r.Context(), wildcardRef(r))
	if err != nil {
		status, msg := u.fail(r, err)
		u.render(w, r, status, web.ErrorPage(msg))
		return
	}
	u.renderRackDetail(w, r, http.StatusOK, rack, flashErr(r))
}

func (u *uiHandler) updateRack(w http.ResponseWriter, r *http.Request) {
	ref := wildcardRef(r)
	uh64, _ := strconv.ParseInt(formValue(r, "u_height"), 10, 32)
	uh := int32(uh64)
	rack, err := u.dcim.UpdateRackByRef(r.Context(), ref, dcim.RackUpdate{
		Name: formPtr(r, "name"), Slug: formPtr(r, "slug"), Status: formPtr(r, "status"),
		UHeight: &uh, Description: formPtr(r, "description"),
	})
	if err != nil {
		status, msg := u.fail(r, err)
		cur, gerr := u.dcim.GetRackByRef(r.Context(), ref)
		if gerr != nil {
			status, msg = u.fail(r, gerr)
			u.render(w, r, status, web.ErrorPage(msg))
			return
		}
		u.renderRackDetail(w, r, status, cur, msg)
		return
	}
	site, _ := u.dcim.GetSiteByRef(r.Context(), rack.SiteID.String())
	http.Redirect(w, r, "/dcim/racks/"+site.Slug+"/"+rack.Slug, http.StatusSeeOther)
}

func (u *uiHandler) deleteRack(w http.ResponseWriter, r *http.Request) {
	ref := wildcardRef(r)
	err := u.dcim.DeleteRackByRef(r.Context(), ref)
	if hxTarget(r) == "racks-section" {
		msg := ""
		if err != nil {
			_, msg = u.fail(r, err)
		}
		items, _, lerr := u.dcim.ListRacks(r.Context(), uiListLimit, 0)
		if lerr != nil {
			st, m := u.fail(r, lerr)
			u.render(w, r, st, web.ErrorPage(m))
			return
		}
		u.render(w, r, http.StatusOK, web.RacksPage(items, msg))
		return
	}
	if err != nil {
		_, msg := u.fail(r, err)
		hxRedirect(w, "/dcim/racks/"+ref, msg)
		return
	}
	hxRedirect(w, "/dcim/racks", "")
}

// --- devices ---

func (u *uiHandler) devicesPage(w http.ResponseWriter, r *http.Request) {
	items, _, err := u.dcim.ListDevices(r.Context(), uiListLimit, 0)
	if err != nil {
		st, msg := u.fail(r, err)
		u.render(w, r, st, web.ErrorPage(msg))
		return
	}
	u.render(w, r, http.StatusOK, web.DevicesPage(items, flashErr(r)))
}

func (u *uiHandler) renderDeviceForm(w http.ResponseWriter, r *http.Request, status int, in dcim.DeviceInput, errMsg string) {
	sites, _, err := u.dcim.ListSites(r.Context(), uiListLimit, 0)
	if err == nil {
		var types []gen.ListDeviceTypesRow
		types, _, err = u.dcim.ListDeviceTypes(r.Context(), uiListLimit, 0)
		if err == nil {
			var roles []gen.DeviceRole
			roles, _, err = u.dcim.ListDeviceRoles(r.Context(), uiListLimit, 0)
			if err == nil {
				var racks []gen.ListRacksRow
				racks, _, err = u.dcim.ListRacks(r.Context(), uiListLimit, 0)
				if err == nil {
					u.render(w, r, status, web.DeviceFormPage(in, sites, types, roles, racks, errMsg))
					return
				}
			}
		}
	}
	st, msg := u.fail(r, err)
	u.render(w, r, st, web.ErrorPage(msg))
}

func (u *uiHandler) deviceFormPage(w http.ResponseWriter, r *http.Request) {
	u.renderDeviceForm(w, r, http.StatusOK, dcim.DeviceInput{Status: "active", Face: "front"}, "")
}

func deviceInputFromForm(r *http.Request) dcim.DeviceInput {
	pos, _ := strconv.ParseFloat(formValue(r, "position"), 64)
	return dcim.DeviceInput{
		SiteRef: formValue(r, "site"), RackRef: formValue(r, "rack"), Position: pos,
		Face: formValue(r, "face"), DeviceTypeRef: formValue(r, "device_type"),
		RoleRef: formValue(r, "role"), Name: formValue(r, "name"), Slug: formValue(r, "slug"),
		Status: formValue(r, "status"), Serial: formValue(r, "serial"),
		Description: formValue(r, "description"),
	}
}

func (u *uiHandler) createDevice(w http.ResponseWriter, r *http.Request) {
	in := deviceInputFromForm(r)
	d, err := u.dcim.CreateDevice(r.Context(), in)
	if err != nil {
		status, msg := u.fail(r, err)
		u.renderDeviceForm(w, r, status, in, msg)
		return
	}
	http.Redirect(w, r, "/dcim/devices/"+in.SiteRef+"/"+d.Slug, http.StatusSeeOther)
}

func (u *uiHandler) renderDeviceDetail(w http.ResponseWriter, r *http.Request, status int, d gen.Device, errMsg string) {
	site, err := u.dcim.GetSiteByRef(r.Context(), d.SiteID.String())
	if err != nil {
		st, msg := u.fail(r, err)
		u.render(w, r, st, web.ErrorPage(msg))
		return
	}
	typeLabel, roleLabel := "", ""
	if dt, err := u.dcim.GetDeviceTypeByRef(r.Context(), d.DeviceTypeID.String()); err == nil {
		typeLabel = dt.Model
	}
	if role, err := u.dcim.GetDeviceRoleByRef(r.Context(), d.RoleID.String()); err == nil {
		roleLabel = role.Name
	}
	comps, err := u.dcim.ComponentsOfDevice(r.Context(), d.ID)
	if err != nil {
		st, msg := u.fail(r, err)
		u.render(w, r, st, web.ErrorPage(msg))
		return
	}
	mods, err := u.dcim.ModulesOfDevice(r.Context(), d.ID)
	if err != nil {
		st, msg := u.fail(r, err)
		u.render(w, r, st, web.ErrorPage(msg))
		return
	}
	occupied := map[id.ID]bool{}
	for _, m := range mods {
		occupied[m.BayID] = true
	}
	var emptyBays []gen.Component
	for _, c := range comps {
		if c.Kind == "module-bay" && !occupied[c.ID] {
			emptyBays = append(emptyBays, c)
		}
	}
	moduleTypes, _, err := u.dcim.ListModuleTypes(r.Context(), uiListLimit, 0)
	if err != nil {
		st, msg := u.fail(r, err)
		u.render(w, r, st, web.ErrorPage(msg))
		return
	}
	u.render(w, r, status, web.DeviceDetailPage(d, site, typeLabel, roleLabel, comps, mods, emptyBays, moduleTypes, errMsg))
}

func (u *uiHandler) deviceDetail(w http.ResponseWriter, r *http.Request) {
	d, err := u.dcim.GetDeviceByRef(r.Context(), wildcardRef(r))
	if err != nil {
		status, msg := u.fail(r, err)
		u.render(w, r, status, web.ErrorPage(msg))
		return
	}
	u.renderDeviceDetail(w, r, http.StatusOK, d, flashErr(r))
}

func (u *uiHandler) updateDeviceOrInstallModule(w http.ResponseWriter, r *http.Request) {
	ref := wildcardRef(r)
	if owner, ok := strings.CutSuffix(ref, "/modules"); ok {
		d, err := u.dcim.GetDeviceByRef(r.Context(), owner)
		if err != nil {
			status, msg := u.fail(r, err)
			u.render(w, r, status, web.ErrorPage(msg))
			return
		}
		if _, err := u.dcim.InstallModule(r.Context(), dcim.ModuleInput{
			DeviceRef: owner, BayID: formValue(r, "bay"),
			ModuleTypeRef: formValue(r, "module_type"), Serial: formValue(r, "serial"),
		}); err != nil {
			status, msg := u.fail(r, err)
			u.renderDeviceDetail(w, r, status, d, msg)
			return
		}
		http.Redirect(w, r, "/dcim/devices/"+owner, http.StatusSeeOther)
		return
	}

	d, err := u.dcim.UpdateDeviceByRef(r.Context(), ref, dcim.DeviceUpdate{
		Name: formPtr(r, "name"), Slug: formPtr(r, "slug"), Status: formPtr(r, "status"),
		Serial: formPtr(r, "serial"), Description: formPtr(r, "description"),
	})
	if err != nil {
		status, msg := u.fail(r, err)
		cur, gerr := u.dcim.GetDeviceByRef(r.Context(), ref)
		if gerr != nil {
			status, msg = u.fail(r, gerr)
			u.render(w, r, status, web.ErrorPage(msg))
			return
		}
		u.renderDeviceDetail(w, r, status, cur, msg)
		return
	}
	site, _ := u.dcim.GetSiteByRef(r.Context(), d.SiteID.String())
	http.Redirect(w, r, "/dcim/devices/"+site.Slug+"/"+d.Slug, http.StatusSeeOther)
}

func (u *uiHandler) deleteDevice(w http.ResponseWriter, r *http.Request) {
	ref := wildcardRef(r)
	err := u.dcim.DeleteDeviceByRef(r.Context(), ref)
	if hxTarget(r) == "devices-section" {
		msg := ""
		if err != nil {
			_, msg = u.fail(r, err)
		}
		items, _, lerr := u.dcim.ListDevices(r.Context(), uiListLimit, 0)
		if lerr != nil {
			st, m := u.fail(r, lerr)
			u.render(w, r, st, web.ErrorPage(m))
			return
		}
		u.render(w, r, http.StatusOK, web.DevicesPage(items, msg))
		return
	}
	if err != nil {
		_, msg := u.fail(r, err)
		hxRedirect(w, "/dcim/devices/"+ref, msg)
		return
	}
	hxRedirect(w, "/dcim/devices", "")
}

// removeModule detaches a module and returns to ?back=.
func (u *uiHandler) removeModule(w http.ResponseWriter, r *http.Request) {
	back := sanitizeNext(r.URL.Query().Get("back"))
	mid, err := id.Parse(chi.URLParam(r, "id"))
	if err != nil {
		hxRedirect(w, back, "invalid module id")
		return
	}
	if err := u.dcim.RemoveModule(r.Context(), mid); err != nil {
		_, msg := u.fail(r, err)
		hxRedirect(w, back, msg)
		return
	}
	hxRedirect(w, back, "")
}
