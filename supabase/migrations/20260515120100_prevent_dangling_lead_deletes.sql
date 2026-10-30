-- Hardening Onda 1 (P1) — Prevenção de delete de Lead com Deal ativo.
--
-- Contexto: hoje crm.leads.owner_id / customer_id são apontados em
-- deal.deals.metadata->>'owner_id' e metadata->>'customer_id', mas NÃO
-- existe FK física (a relação é soft, atravessa schemas e os campos vivem
-- em JSONB). Resultado: nada impede um operador ou rotina de apagar um
-- lead que ainda lastreia um Deal aberto — a UI da esteira quebra na
-- próxima abertura do card.
--
-- A auditoria já tem detecção pós-fato (cron ReconcileDanglingLeadReferences
-- + gauge dangling_lead_refs em pkg/metrics). Este trigger fecha a porta:
-- previne o DELETE em vez de só registrar depois.
--
-- Escopo: hard DELETE em crm.leads + soft delete (UPDATE deleted_at de NULL
-- → NOT NULL). Ambos os caminhos são cobertos.
--
-- Status considerados "inativos" (delete permitido): CONVERTED, LOST,
-- CANCELLED. Tudo o mais é ativo e bloqueia o delete com mensagem
-- estruturada (deal_id + status) para o operador conseguir agir.
--
-- Performance: a query usa o índice idx_deals_status (criado em
-- 20260429120000_search_indexes.sql) e filtra também por deleted_at IS NULL,
-- então só varre deals vivos no tenant.

CREATE OR REPLACE FUNCTION crm.prevent_lead_delete_with_active_deal()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    blocking_deal RECORD;
BEGIN
    -- Hard DELETE branch
    IF TG_OP = 'DELETE' THEN
        SELECT
            d.id,
            d.metadata->>'status' AS status,
            CASE
                WHEN d.metadata->>'owner_id' = OLD.id::text THEN 'owner_id'
                WHEN d.metadata->>'customer_id' = OLD.id::text THEN 'customer_id'
                ELSE 'unknown'
            END AS reference_field
        INTO blocking_deal
        FROM deal.deals d
        WHERE d.tenant_id = OLD.tenant_id
          AND d.deleted_at IS NULL
          AND (d.metadata->>'status') NOT IN ('CONVERTED', 'LOST', 'CANCELLED')
          AND (
                d.metadata->>'owner_id' = OLD.id::text
                OR d.metadata->>'customer_id' = OLD.id::text
              )
        LIMIT 1;

        IF FOUND THEN
            RAISE EXCEPTION
                'cannot delete lead %: referenced by active deal % (status=%, field=%). Archive or reassign the deal first.',
                OLD.id, blocking_deal.id, blocking_deal.status, blocking_deal.reference_field
            USING ERRCODE = '23503'; -- foreign_key_violation
        END IF;
        RETURN OLD;
    END IF;

    -- Soft DELETE branch: deleted_at transitioning from NULL to NOT NULL
    IF TG_OP = 'UPDATE'
       AND OLD.deleted_at IS NULL
       AND NEW.deleted_at IS NOT NULL
    THEN
        SELECT
            d.id,
            d.metadata->>'status' AS status,
            CASE
                WHEN d.metadata->>'owner_id' = OLD.id::text THEN 'owner_id'
                WHEN d.metadata->>'customer_id' = OLD.id::text THEN 'customer_id'
                ELSE 'unknown'
            END AS reference_field
        INTO blocking_deal
        FROM deal.deals d
        WHERE d.tenant_id = OLD.tenant_id
          AND d.deleted_at IS NULL
          AND (d.metadata->>'status') NOT IN ('CONVERTED', 'LOST', 'CANCELLED')
          AND (
                d.metadata->>'owner_id' = OLD.id::text
                OR d.metadata->>'customer_id' = OLD.id::text
              )
        LIMIT 1;

        IF FOUND THEN
            RAISE EXCEPTION
                'cannot soft-delete lead %: referenced by active deal % (status=%, field=%). Archive or reassign the deal first.',
                OLD.id, blocking_deal.id, blocking_deal.status, blocking_deal.reference_field
            USING ERRCODE = '23503';
        END IF;
        RETURN NEW;
    END IF;

    RETURN COALESCE(NEW, OLD);
END;
$$;

DROP TRIGGER IF EXISTS trg_prevent_lead_delete_with_active_deal ON crm.leads;
CREATE TRIGGER trg_prevent_lead_delete_with_active_deal
    BEFORE DELETE OR UPDATE OF deleted_at ON crm.leads
    FOR EACH ROW
    EXECUTE FUNCTION crm.prevent_lead_delete_with_active_deal();

COMMENT ON FUNCTION crm.prevent_lead_delete_with_active_deal IS
    'Onda 1 P1 — blocks delete/soft-delete of crm.leads when any active deal still references the row via owner_id/customer_id. Pair with ReconcileDanglingLeadReferences for detection of pre-existing drift.';
