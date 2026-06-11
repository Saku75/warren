-- 0006: racks, devices, components, modules (design doc 0003 §3–4).
--
-- Components and modules are mutually referential (a module sits in a
-- bay component; a component may be provided by a module), so the
-- module FK on components is added after both exist. Removal cascades
-- recursively through the FKs alone: module → its components → modules
-- seated in those bays → their components → …

CREATE TABLE racks (
    id          uuid PRIMARY KEY,
    site_id     uuid NOT NULL REFERENCES sites (id) ON DELETE CASCADE,
    location_id uuid REFERENCES locations (id),
    slug        text NOT NULL,
    name        text NOT NULL,
    status      text NOT NULL DEFAULT 'active'
                CHECK (status IN ('planned', 'staging', 'active', 'decommissioning', 'retired')),
    u_height    integer NOT NULL DEFAULT 42 CHECK (u_height BETWEEN 1 AND 100),
    tenant_id   uuid REFERENCES tenants (id),
    description text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (site_id, slug)
);

CREATE INDEX racks_site_idx ON racks (site_id);

CREATE TABLE devices (
    id             uuid PRIMARY KEY,
    site_id        uuid NOT NULL REFERENCES sites (id),
    location_id    uuid REFERENCES locations (id),
    rack_id        uuid REFERENCES racks (id),
    position       numeric(4,1) CHECK (position >= 1),
    face           text CHECK (face IN ('front', 'rear')),
    device_type_id uuid NOT NULL REFERENCES device_types (id),
    role_id        uuid NOT NULL REFERENCES device_roles (id),
    tenant_id      uuid REFERENCES tenants (id),
    slug           text NOT NULL,
    name           text NOT NULL,
    status         text NOT NULL DEFAULT 'active'
                   CHECK (status IN ('planned', 'staging', 'active', 'decommissioning', 'retired')),
    serial         text NOT NULL DEFAULT '',
    asset_tag      text NOT NULL DEFAULT '',
    description    text NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (site_id, slug)
);

CREATE INDEX devices_site_idx ON devices (site_id);
CREATE INDEX devices_rack_idx ON devices (rack_id);
CREATE INDEX devices_type_idx ON devices (device_type_id);

CREATE TABLE components (
    id          uuid PRIMARY KEY,
    device_id   uuid NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    kind        text NOT NULL
                CHECK (kind IN ('interface', 'console-port', 'power-port', 'power-outlet', 'module-bay')),
    name        text NOT NULL,
    label       text NOT NULL DEFAULT '',
    attrs       jsonb NOT NULL DEFAULT '{}',
    description text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (device_id, kind, name)
);

CREATE INDEX components_device_idx ON components (device_id);

CREATE TABLE modules (
    id             uuid PRIMARY KEY,
    device_id      uuid NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    bay_id         uuid NOT NULL UNIQUE REFERENCES components (id) ON DELETE CASCADE,
    module_type_id uuid NOT NULL REFERENCES module_types (id),
    serial         text NOT NULL DEFAULT '',
    description    text NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX modules_device_idx ON modules (device_id);

ALTER TABLE components
    ADD COLUMN module_id uuid REFERENCES modules (id) ON DELETE CASCADE;

CREATE INDEX components_module_idx ON components (module_id);
