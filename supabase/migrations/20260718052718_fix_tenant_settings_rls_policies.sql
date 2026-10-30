-- Corrects the auth.jwt() capability check in
-- 20260718052612_tenant_approval_policy.sql for databases where that migration
-- already ran. RLS remains deny-by-default when Supabase Auth is unavailable.

DO $$
BEGIN
  IF to_regprocedure('auth.jwt()') IS NULL THEN
    RETURN;
  END IF;

  DROP POLICY IF EXISTS tenant_isolation_select ON toca_auth.tenant_settings;
  CREATE POLICY tenant_isolation_select ON toca_auth.tenant_settings
    FOR SELECT USING (
      tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)
    );

  DROP POLICY IF EXISTS tenant_isolation_modify ON toca_auth.tenant_settings;
  CREATE POLICY tenant_isolation_modify ON toca_auth.tenant_settings
    FOR ALL USING (
      tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)
    ) WITH CHECK (
      tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)
    );
END $$;
