-- Smoke tests for the baseline schema. Run with db/scripts/test.ps1.
-- Every check raises an exception when it fails, so a non-zero exit means the
-- database is not in the expected shape.

\set ON_ERROR_STOP on

\echo 'checking seed data...'
DO $$
DECLARE
    n INTEGER;
BEGIN
    SELECT count(*) INTO n FROM plants;
    IF n < 1 THEN RAISE EXCEPTION 'expected at least one plant, found %', n; END IF;

    SELECT count(*) INTO n FROM materials;
    IF n < 1 THEN RAISE EXCEPTION 'expected at least one material, found %', n; END IF;

    SELECT count(*) INTO n FROM stock_formats;
    IF n < 1 THEN RAISE EXCEPTION 'expected at least one stock format, found %', n; END IF;

    SELECT count(*) INTO n FROM parts;
    IF n < 1 THEN RAISE EXCEPTION 'expected at least one part, found %', n; END IF;

    SELECT count(*) INTO n FROM rules_profiles WHERE is_default;
    IF n < 1 THEN RAISE EXCEPTION 'expected a default rules profile'; END IF;
END $$;

\echo 'checking constraint on stock_formats sizes...'
DO $$
BEGIN
    BEGIN
        INSERT INTO stock_formats (plant_id, material_spec_id, code)
        SELECT p.id, ms.id, 'BAD-EMPTY-SIZE'
        FROM plants p, material_specs ms
        LIMIT 1;
        RAISE EXCEPTION 'stock_formats accepted a format with no dimensions';
    EXCEPTION WHEN check_violation THEN
        RAISE NOTICE 'ok: empty stock format rejected';
    END;
END $$;

\echo 'checking invalid dimension profile is rejected...'
DO $$
BEGIN
    BEGIN
        INSERT INTO materials (plant_id, code, name, dimension_profile)
        SELECT p.id, 'BAD-PROFILE', 'Bad', '9d' FROM plants p LIMIT 1;
        RAISE EXCEPTION 'materials accepted an invalid dimension profile';
    EXCEPTION WHEN check_violation THEN
        RAISE NOTICE 'ok: invalid dimension profile rejected';
    END;
END $$;

\echo 'checking stock_items (physical pieces and remnants)...'
DO $$
DECLARE
    n INTEGER;
BEGIN
    SELECT count(*) INTO n FROM stock_items WHERE is_remnant;
    IF n < 1 THEN RAISE EXCEPTION 'expected at least one seeded remnant, found %', n; END IF;

    SELECT count(*) INTO n
    FROM stock_items
    WHERE status = 'available'
      AND (length_um > 0 OR (width_um > 0 AND height_um > 0));
    IF n < 1 THEN RAISE EXCEPTION 'expected at least one available physical piece'; END IF;
END $$;

\echo 'checking a stock item cannot have no dimensions...'
DO $$
BEGIN
    BEGIN
        INSERT INTO stock_items (plant_id, label, is_remnant)
        SELECT p.id, 'BAD-EMPTY-PIECE', true FROM plants p LIMIT 1;
        RAISE EXCEPTION 'stock_items accepted a piece with no dimensions';
    EXCEPTION WHEN check_violation THEN
        RAISE NOTICE 'ok: empty physical piece rejected';
    END;
END $$;

\echo 'checking labels are unique in the pool and consumed pieces cannot be...'
BEGIN;
INSERT INTO stock_items (plant_id, label, width_um, height_um, is_remnant)
SELECT p.id, 'SMOKE-LABEL-1', 500000, 500000, true FROM plants p LIMIT 1;
DO $$
DECLARE
    n INTEGER;
BEGIN
    BEGIN
        INSERT INTO stock_items (plant_id, label, width_um, height_um, is_remnant)
        SELECT p.id, 'SMOKE-LABEL-1', 400000, 400000, true FROM plants p LIMIT 1;
        RAISE EXCEPTION 'stock_items accepted a duplicate label';
    EXCEPTION WHEN unique_violation THEN
        RAISE NOTICE 'ok: duplicate remnant label rejected';
    END;

    UPDATE stock_items SET status = 'consumed', consumed_at = now() WHERE label = 'SMOKE-LABEL-1';
    SELECT count(*) INTO n FROM stock_items WHERE label = 'SMOKE-LABEL-1' AND status = 'consumed';
    IF n <> 1 THEN RAISE EXCEPTION 'consuming a physical piece did not work'; END IF;
END $$;
ROLLBACK;

\echo 'checking write path inside a rolled back transaction...'
BEGIN;
INSERT INTO cut_jobs (plant_id, solver, input, seed, budget_ms)
SELECT p.id, 'shelf-2d', '{"parts":[],"stocks":[]}'::jsonb, 42, 5000
FROM plants p LIMIT 1;
DO $$
DECLARE
    n INTEGER;
BEGIN
    SELECT count(*) INTO n FROM cut_jobs WHERE solver = 'shelf-2d';
    IF n < 1 THEN RAISE EXCEPTION 'cut_jobs insert did not work'; END IF;
END $$;
ROLLBACK;

\echo 'all smoke tests passed'
