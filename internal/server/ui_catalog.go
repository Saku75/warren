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

func (u *uiHandler) catalogRoutes(r chi.Router) {
	r.Get("/dcim/manufacturers", u.manufacturersPage)
	r.Post("/dcim/manufacturers", u.createManufacturer)
	r.Get("/dcim/manufacturers/new", u.manufacturerFormPage)
	r.Get("/dcim/manufacturers/{ref}", u.manufacturerDetail)
	r.Post("/dcim/manufacturers/{ref}", u.updateManufacturer)
	r.Delete("/dcim/manufacturers/{ref}", u.deleteManufacturer)

	r.Get("/dcim/device-roles", u.deviceRolesPage)
	r.Post("/dcim/device-roles", u.createDeviceRole)
	r.Get("/dcim/device-roles/new", u.deviceRoleFormPage)
	r.Get("/dcim/device-roles/{ref}", u.deviceRoleDetail)
	r.Post("/dcim/device-roles/{ref}", u.updateDeviceRole)
	r.Delete("/dcim/device-roles/{ref}", u.deleteDeviceRole)

	// Type refs are manufacturer/slug paths; "<ref>/templates" POSTs are
	// template creation on that owner.
	r.Get("/dcim/device-types", u.deviceTypesPage)
	r.Post("/dcim/device-types", u.createDeviceType)
	r.Get("/dcim/device-types/new", u.deviceTypeFormPage)
	r.Get("/dcim/device-types/*", u.deviceTypeDetail)
	r.Post("/dcim/device-types/*", u.updateDeviceTypeOrAddTemplate)
	r.Delete("/dcim/device-types/*", u.deleteDeviceType)

	r.Get("/dcim/module-types", u.moduleTypesPage)
	r.Post("/dcim/module-types", u.createModuleType)
	r.Get("/dcim/module-types/new", u.moduleTypeFormPage)
	r.Get("/dcim/module-types/*", u.moduleTypeDetail)
	r.Post("/dcim/module-types/*", u.updateModuleTypeOrAddTemplate)
	r.Delete("/dcim/module-types/*", u.deleteModuleType)

	r.Delete("/dcim/component-templates/{id}", u.deleteTemplate)
}

// --- manufacturers ---

func (u *uiHandler) manufacturersPage(w http.ResponseWriter, r *http.Request) {
	items, _, err := u.dcim.ListManufacturers(r.Context(), uiListLimit, 0)
	if err != nil {
		st, msg := u.fail(r, err)
		u.render(w, r, st, web.ErrorPage(msg))
		return
	}
	u.render(w, r, http.StatusOK, web.ManufacturersPage(items, flashErr(r)))
}

func (u *uiHandler) manufacturerFormPage(w http.ResponseWriter, r *http.Request) {
	u.render(w, r, http.StatusOK, web.ManufacturerFormPage(dcim.ManufacturerInput{}, ""))
}

func (u *uiHandler) createManufacturer(w http.ResponseWriter, r *http.Request) {
	in := dcim.ManufacturerInput{
		Name: formValue(r, "name"), Slug: formValue(r, "slug"), Description: formValue(r, "description"),
	}
	m, err := u.dcim.CreateManufacturer(r.Context(), in)
	if err != nil {
		status, msg := u.fail(r, err)
		u.render(w, r, status, web.ManufacturerFormPage(in, msg))
		return
	}
	http.Redirect(w, r, "/dcim/manufacturers/"+m.Slug, http.StatusSeeOther)
}

func (u *uiHandler) renderManufacturerDetail(w http.ResponseWriter, r *http.Request, status int, m gen.Manufacturer, errMsg string) {
	deviceTypes, err := u.dcim.DeviceTypesOfManufacturer(r.Context(), m.ID)
	if err == nil {
		var moduleTypes []gen.ModuleType
		moduleTypes, err = u.dcim.ModuleTypesOfManufacturer(r.Context(), m.ID)
		if err == nil {
			u.render(w, r, status, web.ManufacturerDetailPage(m, deviceTypes, moduleTypes, errMsg))
			return
		}
	}
	st, msg := u.fail(r, err)
	u.render(w, r, st, web.ErrorPage(msg))
}

func (u *uiHandler) manufacturerDetail(w http.ResponseWriter, r *http.Request) {
	m, err := u.dcim.GetManufacturerByRef(r.Context(), chi.URLParam(r, "ref"))
	if err != nil {
		status, msg := u.fail(r, err)
		u.render(w, r, status, web.ErrorPage(msg))
		return
	}
	u.renderManufacturerDetail(w, r, http.StatusOK, m, flashErr(r))
}

func (u *uiHandler) updateManufacturer(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "ref")
	m, err := u.dcim.UpdateManufacturerByRef(r.Context(), ref, dcim.ManufacturerUpdate{
		Name: formPtr(r, "name"), Slug: formPtr(r, "slug"), Description: formPtr(r, "description"),
	})
	if err != nil {
		status, msg := u.fail(r, err)
		cur, gerr := u.dcim.GetManufacturerByRef(r.Context(), ref)
		if gerr != nil {
			status, msg = u.fail(r, gerr)
			u.render(w, r, status, web.ErrorPage(msg))
			return
		}
		u.renderManufacturerDetail(w, r, status, cur, msg)
		return
	}
	http.Redirect(w, r, "/dcim/manufacturers/"+m.Slug, http.StatusSeeOther)
}

func (u *uiHandler) deleteManufacturer(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "ref")
	err := u.dcim.DeleteManufacturerByRef(r.Context(), ref)
	if hxTarget(r) == "manufacturers-section" {
		msg := ""
		if err != nil {
			_, msg = u.fail(r, err)
		}
		items, _, lerr := u.dcim.ListManufacturers(r.Context(), uiListLimit, 0)
		if lerr != nil {
			st, m := u.fail(r, lerr)
			u.render(w, r, st, web.ErrorPage(m))
			return
		}
		u.render(w, r, http.StatusOK, web.ManufacturersPage(items, msg))
		return
	}
	if err != nil {
		_, msg := u.fail(r, err)
		hxRedirect(w, "/dcim/manufacturers/"+ref, msg)
		return
	}
	hxRedirect(w, "/dcim/manufacturers", "")
}

// --- device roles ---

func (u *uiHandler) deviceRolesPage(w http.ResponseWriter, r *http.Request) {
	items, _, err := u.dcim.ListDeviceRoles(r.Context(), uiListLimit, 0)
	if err != nil {
		st, msg := u.fail(r, err)
		u.render(w, r, st, web.ErrorPage(msg))
		return
	}
	u.render(w, r, http.StatusOK, web.DeviceRolesPage(items, flashErr(r)))
}

func (u *uiHandler) deviceRoleFormPage(w http.ResponseWriter, r *http.Request) {
	u.render(w, r, http.StatusOK, web.DeviceRoleFormPage(dcim.DeviceRoleInput{}, ""))
}

func (u *uiHandler) createDeviceRole(w http.ResponseWriter, r *http.Request) {
	in := dcim.DeviceRoleInput{
		Name: formValue(r, "name"), Slug: formValue(r, "slug"),
		Color: strings.ToLower(formValue(r, "color")), Description: formValue(r, "description"),
	}
	x, err := u.dcim.CreateDeviceRole(r.Context(), in)
	if err != nil {
		status, msg := u.fail(r, err)
		u.render(w, r, status, web.DeviceRoleFormPage(in, msg))
		return
	}
	http.Redirect(w, r, "/dcim/device-roles/"+x.Slug, http.StatusSeeOther)
}

func (u *uiHandler) deviceRoleDetail(w http.ResponseWriter, r *http.Request) {
	x, err := u.dcim.GetDeviceRoleByRef(r.Context(), chi.URLParam(r, "ref"))
	if err != nil {
		status, msg := u.fail(r, err)
		u.render(w, r, status, web.ErrorPage(msg))
		return
	}
	u.render(w, r, http.StatusOK, web.DeviceRoleDetailPage(x, flashErr(r)))
}

func (u *uiHandler) updateDeviceRole(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "ref")
	color := strings.ToLower(formValue(r, "color"))
	x, err := u.dcim.UpdateDeviceRoleByRef(r.Context(), ref, dcim.DeviceRoleUpdate{
		Name: formPtr(r, "name"), Slug: formPtr(r, "slug"), Color: &color, Description: formPtr(r, "description"),
	})
	if err != nil {
		status, msg := u.fail(r, err)
		cur, gerr := u.dcim.GetDeviceRoleByRef(r.Context(), ref)
		if gerr != nil {
			status, msg = u.fail(r, gerr)
			u.render(w, r, status, web.ErrorPage(msg))
			return
		}
		u.render(w, r, status, web.DeviceRoleDetailPage(cur, msg))
		return
	}
	http.Redirect(w, r, "/dcim/device-roles/"+x.Slug, http.StatusSeeOther)
}

func (u *uiHandler) deleteDeviceRole(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "ref")
	err := u.dcim.DeleteDeviceRoleByRef(r.Context(), ref)
	if hxTarget(r) == "device-roles-section" {
		msg := ""
		if err != nil {
			_, msg = u.fail(r, err)
		}
		items, _, lerr := u.dcim.ListDeviceRoles(r.Context(), uiListLimit, 0)
		if lerr != nil {
			st, m := u.fail(r, lerr)
			u.render(w, r, st, web.ErrorPage(m))
			return
		}
		u.render(w, r, http.StatusOK, web.DeviceRolesPage(items, msg))
		return
	}
	if err != nil {
		_, msg := u.fail(r, err)
		hxRedirect(w, "/dcim/device-roles/"+ref, msg)
		return
	}
	hxRedirect(w, "/dcim/device-roles", "")
}

// --- device types ---

func (u *uiHandler) deviceTypesPage(w http.ResponseWriter, r *http.Request) {
	items, _, err := u.dcim.ListDeviceTypes(r.Context(), uiListLimit, 0)
	if err != nil {
		st, msg := u.fail(r, err)
		u.render(w, r, st, web.ErrorPage(msg))
		return
	}
	u.render(w, r, http.StatusOK, web.DeviceTypesPage(items, flashErr(r)))
}

func (u *uiHandler) renderDeviceTypeForm(w http.ResponseWriter, r *http.Request, status int, in dcim.DeviceTypeInput, errMsg string) {
	manufacturers, _, err := u.dcim.ListManufacturers(r.Context(), uiListLimit, 0)
	if err != nil {
		st, msg := u.fail(r, err)
		u.render(w, r, st, web.ErrorPage(msg))
		return
	}
	u.render(w, r, status, web.DeviceTypeFormPage(in, manufacturers, errMsg))
}

func (u *uiHandler) deviceTypeFormPage(w http.ResponseWriter, r *http.Request) {
	u.renderDeviceTypeForm(w, r, http.StatusOK, dcim.DeviceTypeInput{UHeight: 1, IsFullDepth: true}, "")
}

func deviceTypeInputFromForm(r *http.Request) dcim.DeviceTypeInput {
	uh, _ := strconv.ParseFloat(formValue(r, "u_height"), 64)
	return dcim.DeviceTypeInput{
		ManufacturerRef: formValue(r, "manufacturer"),
		Model:           formValue(r, "model"),
		Slug:            formValue(r, "slug"),
		PartNumber:      formValue(r, "part_number"),
		UHeight:         uh,
		IsFullDepth:     formValue(r, "is_full_depth") == "true",
		Description:     formValue(r, "description"),
	}
}

func (u *uiHandler) createDeviceType(w http.ResponseWriter, r *http.Request) {
	in := deviceTypeInputFromForm(r)
	dt, err := u.dcim.CreateDeviceType(r.Context(), in)
	if err != nil {
		status, msg := u.fail(r, err)
		u.renderDeviceTypeForm(w, r, status, in, msg)
		return
	}
	http.Redirect(w, r, "/dcim/device-types/"+in.ManufacturerRef+"/"+dt.Slug, http.StatusSeeOther)
}

func (u *uiHandler) renderDeviceTypeDetail(w http.ResponseWriter, r *http.Request, status int, dt gen.DeviceType, errMsg string) {
	m, err := u.dcim.GetManufacturerByRef(r.Context(), dt.ManufacturerID.String())
	if err == nil {
		var templates []gen.ComponentTemplate
		templates, err = u.dcim.TemplatesOfDeviceType(r.Context(), dt.ID)
		if err == nil {
			u.render(w, r, status, web.DeviceTypeDetailPage(dt, m, templates, errMsg))
			return
		}
	}
	st, msg := u.fail(r, err)
	u.render(w, r, st, web.ErrorPage(msg))
}

func (u *uiHandler) deviceTypeDetail(w http.ResponseWriter, r *http.Request) {
	dt, err := u.dcim.GetDeviceTypeByRef(r.Context(), wildcardRef(r))
	if err != nil {
		status, msg := u.fail(r, err)
		u.render(w, r, status, web.ErrorPage(msg))
		return
	}
	u.renderDeviceTypeDetail(w, r, http.StatusOK, dt, flashErr(r))
}

// templateInputFromForm maps the shared add-template form.
func templateInputFromForm(r *http.Request) dcim.TemplateInput {
	return dcim.TemplateInput{
		Kind:  formValue(r, "kind"),
		Name:  formValue(r, "name"),
		Label: formValue(r, "label"),
		Attrs: dcim.ComponentAttrs{
			Type:     formValue(r, "attr_type"),
			Position: formValue(r, "attr_position"),
		},
		Description: formValue(r, "description"),
	}
}

func (u *uiHandler) updateDeviceTypeOrAddTemplate(w http.ResponseWriter, r *http.Request) {
	ref := wildcardRef(r)
	if owner, ok := strings.CutSuffix(ref, "/templates"); ok {
		dt, err := u.dcim.GetDeviceTypeByRef(r.Context(), owner)
		if err != nil {
			status, msg := u.fail(r, err)
			u.render(w, r, status, web.ErrorPage(msg))
			return
		}
		in := templateInputFromForm(r)
		in.DeviceTypeRef = owner
		if _, err := u.dcim.CreateTemplate(r.Context(), in); err != nil {
			status, msg := u.fail(r, err)
			u.renderDeviceTypeDetail(w, r, status, dt, msg)
			return
		}
		http.Redirect(w, r, "/dcim/device-types/"+owner, http.StatusSeeOther)
		return
	}

	uh := formValue(r, "u_height")
	uhf, _ := strconv.ParseFloat(uh, 64)
	fullDepth := formValue(r, "is_full_depth") == "true"
	dt, err := u.dcim.UpdateDeviceTypeByRef(r.Context(), ref, dcim.DeviceTypeUpdate{
		Model: formPtr(r, "model"), Slug: formPtr(r, "slug"), PartNumber: formPtr(r, "part_number"),
		UHeight: &uhf, IsFullDepth: &fullDepth, Description: formPtr(r, "description"),
	})
	if err != nil {
		status, msg := u.fail(r, err)
		cur, gerr := u.dcim.GetDeviceTypeByRef(r.Context(), ref)
		if gerr != nil {
			status, msg = u.fail(r, gerr)
			u.render(w, r, status, web.ErrorPage(msg))
			return
		}
		u.renderDeviceTypeDetail(w, r, status, cur, msg)
		return
	}
	m, _ := u.dcim.GetManufacturerByRef(r.Context(), dt.ManufacturerID.String())
	http.Redirect(w, r, "/dcim/device-types/"+m.Slug+"/"+dt.Slug, http.StatusSeeOther)
}

func (u *uiHandler) deleteDeviceType(w http.ResponseWriter, r *http.Request) {
	ref := wildcardRef(r)
	err := u.dcim.DeleteDeviceTypeByRef(r.Context(), ref)
	if hxTarget(r) == "device-types-section" {
		msg := ""
		if err != nil {
			_, msg = u.fail(r, err)
		}
		items, _, lerr := u.dcim.ListDeviceTypes(r.Context(), uiListLimit, 0)
		if lerr != nil {
			st, m := u.fail(r, lerr)
			u.render(w, r, st, web.ErrorPage(m))
			return
		}
		u.render(w, r, http.StatusOK, web.DeviceTypesPage(items, msg))
		return
	}
	if err != nil {
		_, msg := u.fail(r, err)
		hxRedirect(w, "/dcim/device-types/"+ref, msg)
		return
	}
	hxRedirect(w, "/dcim/device-types", "")
}

// --- module types ---

func (u *uiHandler) moduleTypesPage(w http.ResponseWriter, r *http.Request) {
	items, _, err := u.dcim.ListModuleTypes(r.Context(), uiListLimit, 0)
	if err != nil {
		st, msg := u.fail(r, err)
		u.render(w, r, st, web.ErrorPage(msg))
		return
	}
	u.render(w, r, http.StatusOK, web.ModuleTypesPage(items, flashErr(r)))
}

func (u *uiHandler) renderModuleTypeForm(w http.ResponseWriter, r *http.Request, status int, in dcim.ModuleTypeInput, errMsg string) {
	manufacturers, _, err := u.dcim.ListManufacturers(r.Context(), uiListLimit, 0)
	if err != nil {
		st, msg := u.fail(r, err)
		u.render(w, r, st, web.ErrorPage(msg))
		return
	}
	u.render(w, r, status, web.ModuleTypeFormPage(in, manufacturers, errMsg))
}

func (u *uiHandler) moduleTypeFormPage(w http.ResponseWriter, r *http.Request) {
	u.renderModuleTypeForm(w, r, http.StatusOK, dcim.ModuleTypeInput{}, "")
}

func (u *uiHandler) createModuleType(w http.ResponseWriter, r *http.Request) {
	in := dcim.ModuleTypeInput{
		ManufacturerRef: formValue(r, "manufacturer"), Model: formValue(r, "model"),
		Slug: formValue(r, "slug"), PartNumber: formValue(r, "part_number"),
		Description: formValue(r, "description"),
	}
	mt, err := u.dcim.CreateModuleType(r.Context(), in)
	if err != nil {
		status, msg := u.fail(r, err)
		u.renderModuleTypeForm(w, r, status, in, msg)
		return
	}
	http.Redirect(w, r, "/dcim/module-types/"+in.ManufacturerRef+"/"+mt.Slug, http.StatusSeeOther)
}

func (u *uiHandler) renderModuleTypeDetail(w http.ResponseWriter, r *http.Request, status int, mt gen.ModuleType, errMsg string) {
	m, err := u.dcim.GetManufacturerByRef(r.Context(), mt.ManufacturerID.String())
	if err == nil {
		var templates []gen.ComponentTemplate
		templates, err = u.dcim.TemplatesOfModuleType(r.Context(), mt.ID)
		if err == nil {
			u.render(w, r, status, web.ModuleTypeDetailPage(mt, m, templates, errMsg))
			return
		}
	}
	st, msg := u.fail(r, err)
	u.render(w, r, st, web.ErrorPage(msg))
}

func (u *uiHandler) moduleTypeDetail(w http.ResponseWriter, r *http.Request) {
	mt, err := u.dcim.GetModuleTypeByRef(r.Context(), wildcardRef(r))
	if err != nil {
		status, msg := u.fail(r, err)
		u.render(w, r, status, web.ErrorPage(msg))
		return
	}
	u.renderModuleTypeDetail(w, r, http.StatusOK, mt, flashErr(r))
}

func (u *uiHandler) updateModuleTypeOrAddTemplate(w http.ResponseWriter, r *http.Request) {
	ref := wildcardRef(r)
	if owner, ok := strings.CutSuffix(ref, "/templates"); ok {
		mt, err := u.dcim.GetModuleTypeByRef(r.Context(), owner)
		if err != nil {
			status, msg := u.fail(r, err)
			u.render(w, r, status, web.ErrorPage(msg))
			return
		}
		in := templateInputFromForm(r)
		in.ModuleTypeRef = owner
		if _, err := u.dcim.CreateTemplate(r.Context(), in); err != nil {
			status, msg := u.fail(r, err)
			u.renderModuleTypeDetail(w, r, status, mt, msg)
			return
		}
		http.Redirect(w, r, "/dcim/module-types/"+owner, http.StatusSeeOther)
		return
	}

	mt, err := u.dcim.UpdateModuleTypeByRef(r.Context(), ref, dcim.ModuleTypeUpdate{
		Model: formPtr(r, "model"), Slug: formPtr(r, "slug"),
		PartNumber: formPtr(r, "part_number"), Description: formPtr(r, "description"),
	})
	if err != nil {
		status, msg := u.fail(r, err)
		cur, gerr := u.dcim.GetModuleTypeByRef(r.Context(), ref)
		if gerr != nil {
			status, msg = u.fail(r, gerr)
			u.render(w, r, status, web.ErrorPage(msg))
			return
		}
		u.renderModuleTypeDetail(w, r, status, cur, msg)
		return
	}
	m, _ := u.dcim.GetManufacturerByRef(r.Context(), mt.ManufacturerID.String())
	http.Redirect(w, r, "/dcim/module-types/"+m.Slug+"/"+mt.Slug, http.StatusSeeOther)
}

func (u *uiHandler) deleteModuleType(w http.ResponseWriter, r *http.Request) {
	ref := wildcardRef(r)
	err := u.dcim.DeleteModuleTypeByRef(r.Context(), ref)
	if hxTarget(r) == "module-types-section" {
		msg := ""
		if err != nil {
			_, msg = u.fail(r, err)
		}
		items, _, lerr := u.dcim.ListModuleTypes(r.Context(), uiListLimit, 0)
		if lerr != nil {
			st, m := u.fail(r, lerr)
			u.render(w, r, st, web.ErrorPage(m))
			return
		}
		u.render(w, r, http.StatusOK, web.ModuleTypesPage(items, msg))
		return
	}
	if err != nil {
		_, msg := u.fail(r, err)
		hxRedirect(w, "/dcim/module-types/"+ref, msg)
		return
	}
	hxRedirect(w, "/dcim/module-types", "")
}

// deleteTemplate removes a template and returns to the owner page given
// in ?back=.
func (u *uiHandler) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	back := sanitizeNext(r.URL.Query().Get("back"))
	tid, err := id.Parse(chi.URLParam(r, "id"))
	if err != nil {
		hxRedirect(w, back, "invalid template id")
		return
	}
	if err := u.dcim.DeleteTemplate(r.Context(), tid); err != nil {
		_, msg := u.fail(r, err)
		hxRedirect(w, back, msg)
		return
	}
	hxRedirect(w, back, "")
}
