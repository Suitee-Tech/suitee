CREATE SCHEMA IF NOT EXISTS ai;

CREATE TABLE IF NOT EXISTS ai.conversations (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  account_id UUID NOT NULL,
  title TEXT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_ai_conversations_tenant_account_deleted_updated ON ai.conversations (tenant_id, account_id, deleted_at, updated_at DESC);

CREATE TABLE IF NOT EXISTS ai.messages (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  conversation_id UUID NOT NULL REFERENCES ai.conversations(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK (role IN ('system','user','assistant','tool')),
  content TEXT NOT NULL,
  tool_calls JSONB NULL,
  tool_call_id TEXT NULL,
  tool_name TEXT NULL,
  tokens_in INT NULL,
  tokens_out INT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ai_messages_conversation_created_at ON ai.messages (conversation_id, created_at);

DO $$
BEGIN
  IF to_regproc('auth.jwt()') IS NULL THEN
    RETURN;
  END IF;

  ALTER TABLE ai.conversations ENABLE ROW LEVEL SECURITY;
  DROP POLICY IF EXISTS tenant_isolation_select ON ai.conversations;
  CREATE POLICY tenant_isolation_select ON ai.conversations FOR SELECT
    USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
  DROP POLICY IF EXISTS tenant_isolation_modify ON ai.conversations;
  CREATE POLICY tenant_isolation_modify ON ai.conversations FOR ALL
    USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid))
    WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

  ALTER TABLE ai.messages ENABLE ROW LEVEL SECURITY;
  DROP POLICY IF EXISTS tenant_isolation_select ON ai.messages;
  CREATE POLICY tenant_isolation_select ON ai.messages FOR SELECT
    USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
  DROP POLICY IF EXISTS tenant_isolation_modify ON ai.messages;
  CREATE POLICY tenant_isolation_modify ON ai.messages FOR ALL
    USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid))
    WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
END $$;

