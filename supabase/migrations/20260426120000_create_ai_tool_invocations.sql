CREATE TABLE IF NOT EXISTS ai.tool_invocations (
  id UUID PRIMARY KEY,
  conversation_id UUID NOT NULL REFERENCES ai.conversations(id) ON DELETE CASCADE,
  tenant_id UUID NOT NULL,
  account_id UUID NOT NULL,
  tool_name TEXT NOT NULL,
  args JSONB NOT NULL DEFAULT '{}'::jsonb,
  result_preview TEXT NULL,
  success BOOL NOT NULL DEFAULT FALSE,
  has_error BOOL NOT NULL DEFAULT FALSE,
  duration_ms INT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ai_tool_invocations_tenant_conversation_created_at
  ON ai.tool_invocations (tenant_id, conversation_id, created_at DESC);

DO $$
BEGIN
  IF to_regproc('auth.jwt()') IS NULL THEN
    RETURN;
  END IF;

  ALTER TABLE ai.tool_invocations ENABLE ROW LEVEL SECURITY;
  DROP POLICY IF EXISTS tenant_isolation_select ON ai.tool_invocations;
  CREATE POLICY tenant_isolation_select ON ai.tool_invocations FOR SELECT
    USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
  DROP POLICY IF EXISTS tenant_isolation_modify ON ai.tool_invocations;
  CREATE POLICY tenant_isolation_modify ON ai.tool_invocations FOR ALL
    USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid))
    WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
END $$;

