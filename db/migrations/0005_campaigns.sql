-- 0004_campaigns.sql — campaign planning: an ordered set of jobs that shares
-- one stock budget. Running an item consumes the sheets/remnants it uses and
-- returns its offcuts to the campaign pool for the items that follow.
--
-- A campaign is a planning sandbox: accepting a plan is still what changes the
-- plant's real stock. The budget is the "what do we still have to plan with"
-- view while the campaign runs.

-- +goose Up

CREATE TABLE campaigns (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plant_id      UUID NOT NULL REFERENCES plants(id) ON DELETE CASCADE,
    code          TEXT NOT NULL DEFAULT '',
    name          TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'draft'
                  CHECK (status IN ('draft', 'active', 'completed', 'cancelled')),
    rules         JSONB NOT NULL DEFAULT '{}'::jsonb,
    objective     JSONB NOT NULL DEFAULT '{}'::jsonb,
    budget_ms     INTEGER NOT NULL DEFAULT 5000 CHECK (budget_ms > 0),
    seed          BIGINT NOT NULL DEFAULT 0,
    -- Remaining shared stock budget and the snapshot it started from.
    stock         JSONB NOT NULL DEFAULT '[]'::jsonb,
    initial_stock JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at  TIMESTAMPTZ,
    UNIQUE (plant_id, code)
);

CREATE INDEX campaigns_plant_created_idx ON campaigns (plant_id, created_at DESC);

CREATE TABLE campaign_items (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    seq         INTEGER NOT NULL CHECK (seq > 0),
    name        TEXT NOT NULL DEFAULT '',
    parts       JSONB NOT NULL DEFAULT '[]'::jsonb,
    due_date    DATE,
    priority    INTEGER NOT NULL DEFAULT 100,
    status      TEXT NOT NULL DEFAULT 'pending'
                CHECK (status IN ('pending', 'planned', 'failed')),
    plan_id     UUID REFERENCES plans(id) ON DELETE SET NULL,
    job_id      UUID REFERENCES cut_jobs(id) ON DELETE SET NULL,
    error       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (campaign_id, seq)
);

CREATE INDEX campaign_items_campaign_idx ON campaign_items (campaign_id, seq);

-- +goose Down

DROP TABLE IF EXISTS campaign_items;
DROP TABLE IF EXISTS campaigns;
