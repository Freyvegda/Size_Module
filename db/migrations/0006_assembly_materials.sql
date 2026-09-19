-- 0006_assembly_materials.sql — products bind to a material spec and their
-- subparts carry a quantity, so a designed product (e.g. a table) can be
-- exploded into cut parts that the solver plans against that material's stock.

-- +goose Up

-- Product-level material: the default for every component unless a component
-- names its own spec (e.g. an oak top with pine legs).
ALTER TABLE assemblies
    ADD COLUMN material_spec_id UUID REFERENCES material_specs(id) ON DELETE SET NULL;

-- How many of this subpart the product needs (the preview draws it once).
ALTER TABLE assembly_components
    ADD COLUMN quantity INTEGER NOT NULL DEFAULT 1 CHECK (quantity > 0);

-- +goose Down

ALTER TABLE assembly_components DROP COLUMN quantity;
ALTER TABLE assemblies DROP COLUMN material_spec_id;
