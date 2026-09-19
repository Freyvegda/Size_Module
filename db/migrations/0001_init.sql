-- 0001_init.sql — baseline schema for the material optimization platform.
-- Unit rule: every length is micrometers (µm) stored as BIGINT.

-- +goose Up

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE plants (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    timezone    TEXT NOT NULL DEFAULT 'UTC',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plant_id      UUID NOT NULL REFERENCES plants(id) ON DELETE CASCADE,
    email         TEXT NOT NULL,
    display_name  TEXT NOT NULL DEFAULT '',
    password_hash TEXT NOT NULL DEFAULT '',
    role          TEXT NOT NULL DEFAULT 'planner'
                  CHECK (role IN ('admin', 'planner', 'operator', 'viewer')),
    is_active     BOOLEAN NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (plant_id, email)
);

-- Materials are families: Glass, Aluminium, MDF, Fabric...
CREATE TABLE materials (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plant_id          UUID NOT NULL REFERENCES plants(id) ON DELETE CASCADE,
    code              TEXT NOT NULL,
    name              TEXT NOT NULL,
    dimension_profile TEXT NOT NULL DEFAULT '2d'
                      CHECK (dimension_profile IN ('1d', '2d', '3d')),
    attributes        JSONB NOT NULL DEFAULT '{}'::jsonb,
    is_active         BOOLEAN NOT NULL DEFAULT true,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (plant_id, code)
);

-- MaterialSpec is a concrete variant: 6 mm clear float, 18 mm birch ply...
CREATE TABLE material_specs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    material_id  UUID NOT NULL REFERENCES materials(id) ON DELETE CASCADE,
    code         TEXT NOT NULL,
    name         TEXT NOT NULL DEFAULT '',
    thickness_um BIGINT NOT NULL DEFAULT 0 CHECK (thickness_um >= 0),
    finish       TEXT NOT NULL DEFAULT '',
    color        TEXT NOT NULL DEFAULT '',
    attributes   JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (material_id, code)
);

CREATE TABLE machines (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plant_id   UUID NOT NULL REFERENCES plants(id) ON DELETE CASCADE,
    code       TEXT NOT NULL,
    name       TEXT NOT NULL DEFAULT '',
    kind       TEXT NOT NULL DEFAULT 'panel_saw'
               CHECK (kind IN ('panel_saw', 'cutter', 'cnc', 'laser', 'waterjet', 'other')),
    attributes JSONB NOT NULL DEFAULT '{}'::jsonb,
    is_active  BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (plant_id, code)
);

-- Rules profiles are the only place vertical specifics live (kerf, trim,
-- guillotine vs free cutting, grain, offcut policy).
CREATE TABLE rules_profiles (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plant_id   UUID NOT NULL REFERENCES plants(id) ON DELETE CASCADE,
    code       TEXT NOT NULL,
    name       TEXT NOT NULL DEFAULT '',
    rules      JSONB NOT NULL DEFAULT '{}'::jsonb,
    is_default BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (plant_id, code)
);

-- Stock formats are catalog sizes: "Glass 6 mm clear, 3210x2250".
CREATE TABLE stock_formats (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plant_id         UUID NOT NULL REFERENCES plants(id) ON DELETE CASCADE,
    material_spec_id UUID NOT NULL REFERENCES material_specs(id) ON DELETE CASCADE,
    code             TEXT NOT NULL,
    length_um        BIGINT NOT NULL DEFAULT 0 CHECK (length_um >= 0),
    width_um         BIGINT NOT NULL DEFAULT 0 CHECK (width_um >= 0),
    height_um        BIGINT NOT NULL DEFAULT 0 CHECK (height_um >= 0),
    on_hand_qty      INTEGER NOT NULL DEFAULT 0 CHECK (on_hand_qty >= 0),
    cost_per_unit    NUMERIC(14, 4) NOT NULL DEFAULT 0 CHECK (cost_per_unit >= 0),
    is_active        BOOLEAN NOT NULL DEFAULT true,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (plant_id, code),
    -- 1d stock has a length, 2d/3d stock has width and height.
    CHECK (length_um > 0 OR (width_um > 0 AND height_um > 0))
);

-- Parts store finished sizes; cut sizes are derived from routings.
CREATE TABLE parts (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plant_id          UUID NOT NULL REFERENCES plants(id) ON DELETE CASCADE,
    material_spec_id  UUID REFERENCES material_specs(id) ON DELETE SET NULL,
    code              TEXT NOT NULL,
    name              TEXT NOT NULL DEFAULT '',
    finished_length_um BIGINT NOT NULL DEFAULT 0 CHECK (finished_length_um >= 0),
    finished_width_um  BIGINT NOT NULL DEFAULT 0 CHECK (finished_width_um >= 0),
    finished_height_um BIGINT NOT NULL DEFAULT 0 CHECK (finished_height_um >= 0),
    grain             TEXT NOT NULL DEFAULT 'none'
                      CHECK (grain IN ('none', 'along_x', 'along_y')),
    allow_rotate      BOOLEAN NOT NULL DEFAULT true,
    priority          INTEGER NOT NULL DEFAULT 100,
    attributes        JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (plant_id, code)
);

-- One row per manufacturing operation; the allowance calculator sums these to
-- turn a finished size into a cut size.
CREATE TABLE part_routings (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    part_id            UUID NOT NULL REFERENCES parts(id) ON DELETE CASCADE,
    seq                INTEGER NOT NULL,
    operation          TEXT NOT NULL,
    allowance_um       BIGINT NOT NULL DEFAULT 0 CHECK (allowance_um >= 0),
    allowance_per_edge BOOLEAN NOT NULL DEFAULT true,
    notes              TEXT NOT NULL DEFAULT '',
    UNIQUE (part_id, seq)
);

CREATE TABLE cut_jobs (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plant_id         UUID NOT NULL REFERENCES plants(id) ON DELETE CASCADE,
    status           TEXT NOT NULL DEFAULT 'queued'
                     CHECK (status IN ('queued', 'running', 'done', 'failed', 'cancelled')),
    solver           TEXT NOT NULL DEFAULT '',
    rules_profile_id UUID REFERENCES rules_profiles(id) ON DELETE SET NULL,
    input            JSONB NOT NULL,
    result           JSONB,
    error            TEXT NOT NULL DEFAULT '',
    seed             BIGINT NOT NULL DEFAULT 0,
    budget_ms        INTEGER NOT NULL DEFAULT 5000 CHECK (budget_ms > 0),
    created_by       UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at       TIMESTAMPTZ,
    finished_at      TIMESTAMPTZ
);

CREATE INDEX cut_jobs_plant_status_idx ON cut_jobs (plant_id, status, created_at DESC);

CREATE TABLE plans (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plant_id       UUID NOT NULL REFERENCES plants(id) ON DELETE CASCADE,
    job_id         UUID REFERENCES cut_jobs(id) ON DELETE SET NULL,
    version        INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    status         TEXT NOT NULL DEFAULT 'draft'
                   CHECK (status IN ('draft', 'approved', 'accepted', 'archived')),
    name           TEXT NOT NULL DEFAULT '',
    solver         TEXT NOT NULL DEFAULT '',
    solver_version TEXT NOT NULL DEFAULT '',
    seed           BIGINT NOT NULL DEFAULT 0,
    rules          JSONB NOT NULL DEFAULT '{}'::jsonb,
    metrics        JSONB NOT NULL DEFAULT '{}'::jsonb,
    notes          JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX plans_plant_created_idx ON plans (plant_id, created_at DESC);

CREATE TABLE plan_sheets (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_id     UUID NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
    sheet_index INTEGER NOT NULL CHECK (sheet_index >= 0),
    stock_code  TEXT NOT NULL DEFAULT '',
    label       TEXT NOT NULL DEFAULT '',
    width_um    BIGINT NOT NULL CHECK (width_um > 0),
    height_um   BIGINT NOT NULL CHECK (height_um > 0),
    offcuts     JSONB NOT NULL DEFAULT '[]'::jsonb,
    cut_steps   JSONB NOT NULL DEFAULT '[]'::jsonb,
    UNIQUE (plan_id, sheet_index)
);

CREATE TABLE placements (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_sheet_id UUID NOT NULL REFERENCES plan_sheets(id) ON DELETE CASCADE,
    part_id      UUID REFERENCES parts(id) ON DELETE SET NULL,
    part_code    TEXT NOT NULL DEFAULT '',
    x_um         BIGINT NOT NULL CHECK (x_um >= 0),
    y_um         BIGINT NOT NULL CHECK (y_um >= 0),
    w_um         BIGINT NOT NULL CHECK (w_um > 0),
    h_um         BIGINT NOT NULL CHECK (h_um > 0),
    rotated      BOOLEAN NOT NULL DEFAULT false,
    seq          INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX placements_sheet_idx ON placements (plan_sheet_id, seq);

CREATE TABLE audit_log (
    id         BIGSERIAL PRIMARY KEY,
    plant_id   UUID,
    actor      UUID,
    action     TEXT NOT NULL,
    entity     TEXT NOT NULL,
    entity_id  TEXT NOT NULL DEFAULT '',
    payload    JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX audit_log_created_idx ON audit_log (created_at DESC);

-- +goose Down

DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS placements;
DROP TABLE IF EXISTS plan_sheets;
DROP TABLE IF EXISTS plans;
DROP TABLE IF EXISTS cut_jobs;
DROP TABLE IF EXISTS part_routings;
DROP TABLE IF EXISTS parts;
DROP TABLE IF EXISTS stock_formats;
DROP TABLE IF EXISTS rules_profiles;
DROP TABLE IF EXISTS machines;
DROP TABLE IF EXISTS material_specs;
DROP TABLE IF EXISTS materials;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS plants;
