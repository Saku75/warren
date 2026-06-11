# 0003 — Devices, components, and modules

Status: **accepted** · 2026-06-12

Phase 3 design: the device model and the unified component machinery
that replaces NetBox's duplicated per-kind models (foundations §5.4),
while keeping — and nesting — the predefined-module workflow.

## 1. The unified component model

NetBox models each component kind (interface, console port, power port,
power outlet, front/rear port, module bay, …) as its own table, each
with a copy-pasted template twin. Warren has exactly two tables:

```
component_templates   on device types and module types (the catalog)
components            on devices (the inventory)         [next chunk]
```

Both carry a `kind` discriminator and a JSONB `attrs` column whose
schema is validated per kind in the service layer. Kinds in this phase:

| kind          | attrs                              |
|---------------|------------------------------------|
| interface     | type, mgmt_only                    |
| console-port  | type                               |
| power-port    | type, max_draw_w, allocated_draw_w |
| power-outlet  | type, feed_leg                     |
| module-bay    | position                           |

Front/rear ports join with cabling (Phase 4); device bays (blade
chassis) are deferred — module nesting covers the common cases. Adding
a kind is one enum value plus an attrs schema, not two new tables.

Components are not slugged: they are addressed by ID, or by name within
their device (high-cardinality machine names, foundations §4.2).

## 2. Catalog: device types and module types

- **manufacturers**: global slug.
- **device_types**: slug scoped to manufacturer (`cisco/c9300-48p`),
  model name, part number, u_height (numeric, half-U capable),
  full-depth flag. Owns component templates.
- **module_types**: slug scoped to manufacturer (`cisco/c9300-nm-8x`),
  model name, part number. Owns component templates — including
  module-bay templates, which is what makes modules nest.
- **device_roles**: global slug, display color.

A template row is (owner XOR: device_type | module_type, kind, name,
label, attrs). Names are unique per owner and kind and may contain the
`{module}` token (§3).

## 3. Modules and nesting (next chunk)

A **module bay** is a component (kind `module-bay`, attrs.position). A
**module** is an instance of a module type installed into one bay:

```
modules: id, device_id, bay_component_id (unique), module_type_id,
         serial, asset_tag, status
```

Installing a module instantiates the module type's templates as
components on the device with `module_id` set, substituting the
receiving bay's `position` for `{module}` in names ("Te{module}/1" in
bay position "1" → "Te1/1"). Module-bay templates instantiate as bays
owned by that module — into which further modules install (SFP into
NIC, NIC into switch). Removing a module cascades its components, and
through them any nested modules.

Components always carry `device_id` (the physical anchor) plus nullable
`module_id` (which module provided them), so "all interfaces of this
device" stays one indexed query regardless of module depth.

## 4. Racks and devices (next chunk)

- **racks**: slug scoped to site (`cph-dc1/r07`), u_height (default
  42), status, tenant; elevations rendered from mounted devices.
- **devices**: slug scoped to site (`cph-dc1/core-sw-01`), device type,
  role, status, optional location/rack/position/face, serial,
  asset tag, tenant. Creating a device stamps its device type's
  templates into components.

Template changes do not retroactively rewrite existing devices
(NetBox-compatible behavior); a later "sync from type" action can offer
that explicitly.
