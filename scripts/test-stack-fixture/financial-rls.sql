-- Run only in the private Supabase project owned by test-migrated-financial.sh.
-- Rollback leaves no fixture data. SET ROLE exercises the actual policies,
-- rather than relying on the privileged connection's repository filters.
BEGIN;
-- The financial schema is private by default. Grant only this probe's DML
-- inside the rolled-back transaction so row policies can be observed.
GRANT USAGE ON SCHEMA financial TO authenticated;
GRANT SELECT, INSERT, UPDATE ON financial.transactions TO authenticated;
INSERT INTO financial.transactions (id, tenant_id, metadata) VALUES
 ('22222222-2222-4222-8222-222222222201', '22222222-2222-4222-8222-222222222202', '{}'::jsonb),
 ('22222222-2222-4222-8222-222222222203', '22222222-2222-4222-8222-222222222204', '{}'::jsonb);
SET LOCAL ROLE authenticated;
SELECT set_config('request.jwt.claims', '{"app_metadata":{"tenant_id":"22222222-2222-4222-8222-222222222202"}}', true);
DO $$ DECLARE visible integer; changed integer;
BEGIN
  SELECT count(*) INTO visible FROM financial.transactions
    WHERE id IN ('22222222-2222-4222-8222-222222222201', '22222222-2222-4222-8222-222222222203');
  IF visible <> 1 THEN RAISE EXCEPTION 'RLS exposed another agency or hid own row: %', visible; END IF;
  UPDATE financial.transactions SET metadata = '{"forged":true}'::jsonb
    WHERE id = '22222222-2222-4222-8222-222222222203';
  GET DIAGNOSTICS changed = ROW_COUNT;
  IF changed <> 0 THEN RAISE EXCEPTION 'RLS allowed changing another agency'; END IF;
  BEGIN
    INSERT INTO financial.transactions (id, tenant_id, metadata)
      VALUES ('22222222-2222-4222-8222-222222222205', '22222222-2222-4222-8222-222222222204', '{}'::jsonb);
    RAISE EXCEPTION 'RLS allowed inserting into another agency';
  EXCEPTION WHEN insufficient_privilege THEN NULL;
  END;
END $$;
ROLLBACK;
