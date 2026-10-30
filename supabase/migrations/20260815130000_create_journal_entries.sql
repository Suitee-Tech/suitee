-- Migration: cria o ledger contábil de dupla entrada (financial.journal_entries).
--
-- FIN-E2E-33 substitui o modelo single-entry mutável de financial.transactions
-- por um journal append-only: cada entrada tem pernas de débito/crédito cuja
-- soma é zero e só pode ser corrigida por reversão (nunca por UPDATE/DELETE do
-- conteúdo econômico). A única mutação permitida é a transição de status para
-- REVERSED, feita de forma otimista sobre updated_at.

CREATE TABLE IF NOT EXISTS financial.journal_entries (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    metadata JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_financial_journal_entries_tenant_id
    ON financial.journal_entries(tenant_id);

CREATE INDEX IF NOT EXISTS idx_financial_journal_entries_active
    ON financial.journal_entries (tenant_id, created_at DESC)
    WHERE deleted_at IS NULL;

-- Idempotência por fato econômico: um mesmo source_event_id só materializa uma
-- entrada, inclusive em redelivery do evento.
CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_journal_entries_source_event_id
    ON financial.journal_entries (tenant_id, ((metadata->>'source_event_id')))
    WHERE deleted_at IS NULL AND (metadata->>'source_event_id') IS NOT NULL;

-- Isolamento por tenant, idêntico às demais tabelas financeiras.
DROP POLICY IF EXISTS tenant_isolation_select ON financial.journal_entries;
CREATE POLICY tenant_isolation_select ON financial.journal_entries
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON financial.journal_entries;
CREATE POLICY tenant_isolation_modify ON financial.journal_entries
    FOR ALL TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    )
    WITH CHECK (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

ALTER TABLE financial.journal_entries ENABLE ROW LEVEL SECURITY;
