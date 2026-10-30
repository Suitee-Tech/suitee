CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE SCHEMA IF NOT EXISTS toca_auth;
CREATE SCHEMA IF NOT EXISTS core;
CREATE SCHEMA IF NOT EXISTS crm;
CREATE SCHEMA IF NOT EXISTS deal;
CREATE SCHEMA IF NOT EXISTS financial;
CREATE SCHEMA IF NOT EXISTS ops;
CREATE SCHEMA IF NOT EXISTS portal;
CREATE SCHEMA IF NOT EXISTS comm;
CREATE SCHEMA IF NOT EXISTS calendar;
CREATE SCHEMA IF NOT EXISTS events;

DO $$
BEGIN
  IF to_regclass('core.appearance') IS NOT NULL AND to_regclass('core.appearance_settings') IS NULL THEN
    ALTER TABLE core.appearance RENAME TO appearance_settings;
  END IF;
  IF to_regclass('core.settings') IS NOT NULL AND to_regclass('core.company_settings') IS NULL THEN
    ALTER TABLE core.settings RENAME TO company_settings;
  END IF;
  IF to_regclass('core.permissions') IS NOT NULL AND to_regclass('core.role_permissions') IS NULL THEN
    ALTER TABLE core.permissions RENAME TO role_permissions;
  END IF;
  IF to_regclass('toca_auth.preferences') IS NOT NULL AND to_regclass('toca_auth.user_preferences') IS NULL THEN
    ALTER TABLE toca_auth.preferences RENAME TO user_preferences;
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS financial.nfse (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE TABLE IF NOT EXISTS financial.dimob (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE TABLE IF NOT EXISTS financial.informes (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE TABLE IF NOT EXISTS crm.roulette_configs (
  tenant_id UUID PRIMARY KEY,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE TABLE IF NOT EXISTS crm.roulette_history (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE TABLE IF NOT EXISTS portal.integrations (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE TABLE IF NOT EXISTS portal.sync_history (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE TABLE IF NOT EXISTS portal.leads (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE TABLE IF NOT EXISTS portal.feed_cache (
  id UUID PRIMARY KEY,
  tenant_id UUID NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_portal_feed_cache_tenant_id ON portal.feed_cache(tenant_id);

CREATE TABLE IF NOT EXISTS comm.channels (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_comm_channels_tenant_id ON comm.channels(tenant_id);
CREATE INDEX IF NOT EXISTS idx_comm_channels_participants ON comm.channels USING GIN ((metadata -> 'participants'));

CREATE TABLE IF NOT EXISTS comm.templates (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_comm_templates_tenant_id ON comm.templates(tenant_id);

ALTER TABLE IF EXISTS toca_auth.invitations
  ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS toca_auth.credentials
  ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

ALTER TABLE IF EXISTS toca_auth.accounts
  ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

ALTER TABLE IF EXISTS core.properties
  ALTER COLUMN title SET DEFAULT '',
  ALTER COLUMN status SET DEFAULT '',
  ALTER COLUMN type SET DEFAULT '';

DO $$
DECLARE r RECORD;
BEGIN
  FOR r IN
    SELECT n.nspname AS s, c.relname AS t
    FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
    WHERE c.relkind='r'
      AND n.nspname IN ('core','crm','deal','financial','ops','portal','comm','calendar')
      AND EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema=n.nspname AND table_name=c.relname AND column_name='tenant_id'
      )
  LOOP
    EXECUTE format('ALTER TABLE %I.%I ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL', r.s, r.t);
    EXECUTE format('ALTER TABLE %I.%I ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()', r.s, r.t);
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_%s_%s_tenant_deleted_at ON %I.%I (tenant_id, deleted_at)', r.s, r.t, r.s, r.t);
  END LOOP;
END $$;

DO $$
DECLARE r RECORD;
BEGIN
  IF to_regproc('auth.jwt()') IS NULL THEN
    RETURN;
  END IF;

  FOR r IN
    SELECT n.nspname AS s, c.relname AS t
    FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
    WHERE c.relkind='r'
      AND n.nspname IN ('core','crm','deal','financial','ops','portal','comm','calendar')
      AND EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema=n.nspname AND table_name=c.relname AND column_name='tenant_id'
      )
  LOOP
    EXECUTE format('ALTER TABLE %I.%I ENABLE ROW LEVEL SECURITY', r.s, r.t);
    EXECUTE format('DROP POLICY IF EXISTS tenant_isolation_select ON %I.%I', r.s, r.t);
    EXECUTE format(
      'CREATE POLICY tenant_isolation_select ON %I.%I FOR SELECT USING (tenant_id = ((auth.jwt() -> ''app_metadata'' ->> ''tenant_id'')::uuid))',
      r.s, r.t
    );
    EXECUTE format('DROP POLICY IF EXISTS tenant_isolation_modify ON %I.%I', r.s, r.t);
    EXECUTE format(
      'CREATE POLICY tenant_isolation_modify ON %I.%I FOR ALL USING (tenant_id = ((auth.jwt() -> ''app_metadata'' ->> ''tenant_id'')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> ''app_metadata'' ->> ''tenant_id'')::uuid))',
      r.s, r.t
    );
  END LOOP;
END $$;

DO $$
DECLARE storage_objects_owner name;
BEGIN
  IF to_regclass('storage.buckets') IS NULL OR to_regclass('storage.objects') IS NULL THEN
    RETURN;
  END IF;

  BEGIN
    INSERT INTO storage.buckets (id, name, public)
    VALUES
      ('toca-avatars', 'toca-avatars', true),
      ('toca-logos', 'toca-logos', true),
      ('toca-property-images', 'toca-property-images', true),
      ('toca-documents', 'toca-documents', false),
      ('toca-message-attachments', 'toca-message-attachments', false)
    ON CONFLICT (id) DO NOTHING;
  EXCEPTION
    WHEN insufficient_privilege THEN
      RETURN;
  END;

  SELECT r.rolname
  INTO storage_objects_owner
  FROM pg_class c
  JOIN pg_namespace n ON n.oid = c.relnamespace
  JOIN pg_roles r ON r.oid = c.relowner
  WHERE n.nspname = 'storage' AND c.relname = 'objects';

  IF storage_objects_owner IS NULL OR storage_objects_owner <> current_user THEN
    RETURN;
  END IF;

  BEGIN
    ALTER TABLE storage.objects ENABLE ROW LEVEL SECURITY;

    DROP POLICY IF EXISTS public_read_toca_avatars ON storage.objects;
    CREATE POLICY public_read_toca_avatars ON storage.objects FOR SELECT USING (bucket_id = 'toca-avatars');
    DROP POLICY IF EXISTS tenant_folder_toca_avatars ON storage.objects;
    CREATE POLICY tenant_folder_toca_avatars ON storage.objects FOR ALL USING (bucket_id = 'toca-avatars' AND (storage.foldername(name))[1] = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')) WITH CHECK (bucket_id = 'toca-avatars' AND (storage.foldername(name))[1] = (auth.jwt() -> 'app_metadata' ->> 'tenant_id'));

    DROP POLICY IF EXISTS public_read_toca_logos ON storage.objects;
    CREATE POLICY public_read_toca_logos ON storage.objects FOR SELECT USING (bucket_id = 'toca-logos');
    DROP POLICY IF EXISTS tenant_folder_toca_logos ON storage.objects;
    CREATE POLICY tenant_folder_toca_logos ON storage.objects FOR ALL USING (bucket_id = 'toca-logos' AND (storage.foldername(name))[1] = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')) WITH CHECK (bucket_id = 'toca-logos' AND (storage.foldername(name))[1] = (auth.jwt() -> 'app_metadata' ->> 'tenant_id'));

    DROP POLICY IF EXISTS public_read_toca_property_images ON storage.objects;
    CREATE POLICY public_read_toca_property_images ON storage.objects FOR SELECT USING (bucket_id = 'toca-property-images');
    DROP POLICY IF EXISTS tenant_folder_toca_property_images ON storage.objects;
    CREATE POLICY tenant_folder_toca_property_images ON storage.objects FOR ALL USING (bucket_id = 'toca-property-images' AND (storage.foldername(name))[1] = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')) WITH CHECK (bucket_id = 'toca-property-images' AND (storage.foldername(name))[1] = (auth.jwt() -> 'app_metadata' ->> 'tenant_id'));

    DROP POLICY IF EXISTS tenant_folder_toca_documents ON storage.objects;
    CREATE POLICY tenant_folder_toca_documents ON storage.objects FOR ALL USING (bucket_id = 'toca-documents' AND (storage.foldername(name))[1] = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')) WITH CHECK (bucket_id = 'toca-documents' AND (storage.foldername(name))[1] = (auth.jwt() -> 'app_metadata' ->> 'tenant_id'));

    DROP POLICY IF EXISTS tenant_folder_toca_message_attachments ON storage.objects;
    CREATE POLICY tenant_folder_toca_message_attachments ON storage.objects FOR ALL USING (bucket_id = 'toca-message-attachments' AND (storage.foldername(name))[1] = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')) WITH CHECK (bucket_id = 'toca-message-attachments' AND (storage.foldername(name))[1] = (auth.jwt() -> 'app_metadata' ->> 'tenant_id'));
  EXCEPTION
    WHEN insufficient_privilege THEN
      RETURN;
  END;
END $$;

-- This migration consolidates scripts/postgres/{reconcile,supabase_rls,supabase_buckets}.sql.
-- Do not edit those legacy scripts; future changes must be introduced via a new migration.
