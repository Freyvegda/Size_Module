-- 0004_assemblies.sql — products built from subparts (assemblies + components).
--
-- A *part* is a flat thing to cut (finished size + routings). An *assembly* is
-- a product the shop builds, e.g. a window: it has overall dimensions and an
-- ordered list of components (glass panel, frame beams, mullions). Components
-- are plain axis-aligned boxes positioned in the assembly's local frame with
-- the origin at the bottom-left-front corner (x right, y up, z out of the
-- wall), so the same data drives both the 2D elevation and the 3D view.

-- +goose Up

CREATE TABLE assemblies (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plant_id    UUID NOT NULL REFERENCES plants(id) ON DELETE CASCADE,
    code        TEXT NOT NULL,
    name        TEXT NOT NULL DEFAULT '',
    kind        TEXT NOT NULL DEFAULT 'window'
                CHECK (kind IN ('window', 'door', 'generic')),
    width_um    BIGINT NOT NULL DEFAULT 0 CHECK (width_um >= 0),
    height_um   BIGINT NOT NULL DEFAULT 0 CHECK (height_um >= 0),
    depth_um    BIGINT NOT NULL DEFAULT 0 CHECK (depth_um >= 0),
    attributes  JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (plant_id, code)
);

-- One row per subpart. Dimensions are micrometers; offsets are relative to the
-- assembly's bottom-left-front corner.
CREATE TABLE assembly_components (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    assembly_id      UUID NOT NULL REFERENCES assemblies(id) ON DELETE CASCADE,
    seq              INTEGER NOT NULL,
    role             TEXT NOT NULL DEFAULT 'part',
    kind             TEXT NOT NULL DEFAULT 'panel'
                     CHECK (kind IN ('beam', 'panel', 'custom')),
    name             TEXT NOT NULL DEFAULT '',
    material_spec_id UUID REFERENCES material_specs(id) ON DELETE SET NULL,
    width_um         BIGINT NOT NULL DEFAULT 0 CHECK (width_um >= 0),
    height_um        BIGINT NOT NULL DEFAULT 0 CHECK (height_um >= 0),
    depth_um         BIGINT NOT NULL DEFAULT 0 CHECK (depth_um >= 0),
    offset_x_um      BIGINT NOT NULL DEFAULT 0,
    offset_y_um      BIGINT NOT NULL DEFAULT 0,
    offset_z_um      BIGINT NOT NULL DEFAULT 0,
    attributes       JSONB NOT NULL DEFAULT '{}'::jsonb,
    UNIQUE (assembly_id, seq)
);

CREATE INDEX assembly_components_assembly_idx ON assembly_components (assembly_id);

-- +goose Down

DROP INDEX assembly_components_assembly_idx;
DROP TABLE assembly_components;
DROP TABLE assemblies;
