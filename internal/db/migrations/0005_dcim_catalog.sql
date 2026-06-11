-- 0005: DCIM catalog — manufacturers, device roles, device types,
-- module types, and the unified component template model (design doc
-- 0003 §1–2). Devices, racks, and modules build on this next.

CREATE TABLE manufacturers (
    id          uuid PRIMARY KEY,
    slug        text        NOT NULL UNIQUE,  -- scope: global
    name        text        NOT NULL,
    description text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE device_roles (
    id          uuid PRIMARY KEY,
    slug        text        NOT NULL UNIQUE,  -- scope: global
    name        text        NOT NULL,
    color       text        NOT NULL DEFAULT '#8a5a2b'
                CHECK (color ~ '^#[0-9a-f]{6}$'),
    description text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE device_types (
    id              uuid PRIMARY KEY,
    manufacturer_id uuid        NOT NULL REFERENCES manufacturers (id),
    slug            text        NOT NULL,  -- scope: manufacturer
    model           text        NOT NULL,
    part_number     text        NOT NULL DEFAULT '',
    u_height        numeric(4,1) NOT NULL DEFAULT 1 CHECK (u_height >= 0),
    is_full_depth   boolean     NOT NULL DEFAULT true,
    description     text        NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (manufacturer_id, slug)
);

CREATE TABLE module_types (
    id              uuid PRIMARY KEY,
    manufacturer_id uuid        NOT NULL REFERENCES manufacturers (id),
    slug            text        NOT NULL,  -- scope: manufacturer
    model           text        NOT NULL,
    part_number     text        NOT NULL DEFAULT '',
    description     text        NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (manufacturer_id, slug)
);

-- One template table for every component kind (design doc 0003 §1).
-- Exactly one owner: a device type or a module type. Names may contain
-- the {module} placeholder, substituted with the receiving bay position
-- when a module is installed.
CREATE TABLE component_templates (
    id             uuid PRIMARY KEY,
    device_type_id uuid REFERENCES device_types (id) ON DELETE CASCADE,
    module_type_id uuid REFERENCES module_types (id) ON DELETE CASCADE,
    kind           text  NOT NULL
                   CHECK (kind IN ('interface', 'console-port', 'power-port', 'power-outlet', 'module-bay')),
    name           text  NOT NULL,
    label          text  NOT NULL DEFAULT '',
    attrs          jsonb NOT NULL DEFAULT '{}',
    description    text  NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    CHECK ((device_type_id IS NULL) <> (module_type_id IS NULL)),
    UNIQUE NULLS NOT DISTINCT (device_type_id, module_type_id, kind, name)
);

CREATE INDEX component_templates_device_type_idx ON component_templates (device_type_id);
CREATE INDEX component_templates_module_type_idx ON component_templates (module_type_id);
