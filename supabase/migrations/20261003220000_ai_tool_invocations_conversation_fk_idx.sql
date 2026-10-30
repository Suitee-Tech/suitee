-- P2-08 — advisor de performance: FK sem índice em ai.tool_invocations.
--
-- `conversation_id` referencia ai.conversations(id) com ON DELETE CASCADE:
-- o cascade de uma conversa (e joins conversation→invocations) varre a
-- tabela por conversation_id. O índice existente
-- (tenant_id, conversation_id, created_at DESC) começa por tenant_id e
-- NÃO atende essa busca — o Postgres faria seq scan por conversa.
CREATE INDEX IF NOT EXISTS idx_ai_tool_invocations_conversation_id
    ON ai.tool_invocations (conversation_id);
