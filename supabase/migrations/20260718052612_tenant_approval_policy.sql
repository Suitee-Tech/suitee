-- Versioned, tenant-scoped approval gate consumed by toca-deal.
-- Filename aligned with the version recorded by the production migration ledger.
-- schema_version versions the JSON contract; revision is incremented on every
-- update by toca-auth so caches/audit rows can identify the exact decision rule.

CREATE SCHEMA IF NOT EXISTS toca_auth;

CREATE TABLE IF NOT EXISTS toca_auth.tenant_settings (
  tenant_id UUID PRIMARY KEY,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_toca_auth_tenant_settings_tenant
  ON toca_auth.tenant_settings (tenant_id);

-- Defense in depth for any JWT/PostgREST access. Backend policy reads/writes
-- use DATABASE_URI_SERVICE and therefore do not depend on a user JWT.
ALTER TABLE toca_auth.tenant_settings ENABLE ROW LEVEL SECURITY;

DO $$
BEGIN
  -- Vanilla Postgres used by local drills has no Supabase auth.jwt(). RLS stays
  -- enabled (deny by default); policies are installed when Supabase is present.
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

ALTER TABLE toca_auth.tenant_settings
  ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

UPDATE toca_auth.tenant_settings
SET metadata = jsonb_set(
      CASE WHEN jsonb_typeof(metadata) = 'object' THEN metadata ELSE '{}'::jsonb END,
      '{approval_policy}',
      jsonb_build_object(
        'schema_version', 1,
        'revision', 1,
        'enabled', true,
        'threshold_brl', 500000,
        'target_statuses', jsonb_build_array('ANALYSIS', 'CONTRACT'),
        'required_permission', 'deals:approve',
        'updated_at', NOW(),
        'updated_by', 'migration:20260717120000'
      ),
      true
    ),
    updated_at = NOW()
WHERE jsonb_typeof(metadata) <> 'object'
   OR NOT (metadata ? 'approval_policy');

ALTER TABLE toca_auth.tenant_settings
  ALTER COLUMN metadata SET DEFAULT jsonb_build_object(
    'approval_policy', jsonb_build_object(
      'schema_version', 1,
      'revision', 1,
      'enabled', true,
      'threshold_brl', 500000,
      'target_statuses', jsonb_build_array('ANALYSIS', 'CONTRACT'),
      'required_permission', 'deals:approve',
      'updated_at', NOW(),
      'updated_by', 'database_default'
    )
  );

ALTER TABLE toca_auth.tenant_settings
  DROP CONSTRAINT IF EXISTS tenant_settings_approval_policy_v1_check;

ALTER TABLE toca_auth.tenant_settings
  ADD CONSTRAINT tenant_settings_approval_policy_v1_check CHECK (
    jsonb_typeof(metadata) = 'object'
    AND jsonb_typeof(metadata -> 'approval_policy') = 'object'
    AND metadata #>> '{approval_policy,schema_version}' = '1'
    AND (metadata #>> '{approval_policy,revision}') ~ '^[0-9]+$'
    AND (metadata #>> '{approval_policy,revision}')::bigint >= 1
    AND jsonb_typeof(metadata #> '{approval_policy,enabled}') = 'boolean'
    AND jsonb_typeof(metadata #> '{approval_policy,threshold_brl}') = 'number'
    AND (metadata #>> '{approval_policy,threshold_brl}')::numeric >= 0
    AND (metadata #>> '{approval_policy,threshold_brl}')::numeric <= 1000000000000
    AND metadata #> '{approval_policy,target_statuses}' = '["ANALYSIS", "CONTRACT"]'::jsonb
    AND metadata #>> '{approval_policy,required_permission}' = 'deals:approve'
  ) NOT VALID;

ALTER TABLE toca_auth.tenant_settings
  VALIDATE CONSTRAINT tenant_settings_approval_policy_v1_check;
