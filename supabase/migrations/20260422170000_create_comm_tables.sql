CREATE TABLE IF NOT EXISTS comm.channels (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_comm_channels_tenant_deleted_at ON comm.channels (tenant_id, deleted_at);

CREATE TABLE IF NOT EXISTS comm.messages (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_comm_messages_tenant_deleted_at ON comm.messages (tenant_id, deleted_at);

CREATE TABLE IF NOT EXISTS comm.templates (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_comm_templates_tenant_deleted_at ON comm.templates (tenant_id, deleted_at);

CREATE TABLE IF NOT EXISTS comm.inapp_notifications (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_comm_inapp_notifications_tenant_deleted_at ON comm.inapp_notifications (tenant_id, deleted_at);

DO $$
BEGIN
  IF to_regproc('auth.jwt()') IS NULL THEN
    RETURN;
  END IF;

  ALTER TABLE comm.channels ENABLE ROW LEVEL SECURITY;
  DROP POLICY IF EXISTS tenant_isolation_select ON comm.channels;
  CREATE POLICY tenant_isolation_select ON comm.channels FOR SELECT
    USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
  DROP POLICY IF EXISTS tenant_isolation_modify ON comm.channels;
  CREATE POLICY tenant_isolation_modify ON comm.channels FOR ALL
    USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid))
    WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

  ALTER TABLE comm.messages ENABLE ROW LEVEL SECURITY;
  DROP POLICY IF EXISTS tenant_isolation_select ON comm.messages;
  CREATE POLICY tenant_isolation_select ON comm.messages FOR SELECT
    USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
  DROP POLICY IF EXISTS tenant_isolation_modify ON comm.messages;
  CREATE POLICY tenant_isolation_modify ON comm.messages FOR ALL
    USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid))
    WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

  ALTER TABLE comm.templates ENABLE ROW LEVEL SECURITY;
  DROP POLICY IF EXISTS tenant_isolation_select ON comm.templates;
  CREATE POLICY tenant_isolation_select ON comm.templates FOR SELECT
    USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
  DROP POLICY IF EXISTS tenant_isolation_modify ON comm.templates;
  CREATE POLICY tenant_isolation_modify ON comm.templates FOR ALL
    USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid))
    WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

  ALTER TABLE comm.inapp_notifications ENABLE ROW LEVEL SECURITY;
  DROP POLICY IF EXISTS tenant_isolation_select ON comm.inapp_notifications;
  CREATE POLICY tenant_isolation_select ON comm.inapp_notifications FOR SELECT
    USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
  DROP POLICY IF EXISTS tenant_isolation_modify ON comm.inapp_notifications;
  CREATE POLICY tenant_isolation_modify ON comm.inapp_notifications FOR ALL
    USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid))
    WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
END $$;

