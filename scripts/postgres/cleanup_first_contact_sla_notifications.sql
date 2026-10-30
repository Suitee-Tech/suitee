-- ============================================================================
-- Limpeza: enxurrada de notificações "SLA primeiro contato violado" (toca-comm)
-- ============================================================================
--
-- CONTEXTO
--   O worker RunFirstContactSLAWorker (toca-crm) re-publicava o evento
--   `lead.first_contact_sla_violated` a cada minuto: a query
--   ListFirstContactSLABreaches filtrava idempotência por uma chave JSONB que
--   nunca existia, então o filtro virava no-op e o mesmo lead era re-detectado
--   a cada tick. O consumer no toca-comm (event_consumer.go) gravava UMA nova
--   linha em comm.inapp_notifications por minuto, por destinatário, por lead —
--   inundando a Central de Notificações, inclusive para leads recém-criados ou
--   nunca atribuídos (estes com "atraso" absurdo de anos).
--
--   Bug de CÓDIGO já corrigido no PR #595 (branch fix/sla-first-contact-flood,
--   merge em 2026-05-20T19:16:18Z). Este script limpa apenas as linhas que o
--   bug já acumulou no banco — não altera schema nem código.
--
-- TABELA ALVO  comm.inapp_notifications
--   id          UUID PRIMARY KEY
--   tenant_id   UUID NOT NULL
--   metadata    JSONB NOT NULL   -- InAppNotification serializado
--   created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
--   updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
--   deleted_at  TIMESTAMPTZ NULL -- soft delete; a aplicação filtra deleted_at IS NULL
--
-- FINGERPRINT da notificação de SLA de primeiro contato (event_consumer.go,
-- handler "comm_lead_first_contact_sla_violated"):
--   metadata->>'type'  = 'system'
--   metadata->>'title' = 'SLA primeiro contato violado'
--   metadata->'data' contém as chaves  sla_minutes  +  minutes_overdue  + lead_id
--
--   As chaves sla_minutes e minutes_overdue em metadata->'data' são EXCLUSIVAS
--   desse handler — nenhum outro tipo de notificação as grava (verificado em
--   todo o event_consumer.go). O fingerprint, portanto, não colide com:
--     - "SLA Violado — Escalação"  (escalação de SLA — usa data.action/level)
--     - nem com qualquer outra notificação do tipo 'system'.
--
-- O QUE ESTE SCRIPT *NÃO* TOCA
--   Qualquer linha que não bata 100% no fingerprint acima. Notificações de
--   outros tipos, de outros tenants não selecionados, e linhas já soft-deletadas
--   ficam intactas.
--
-- ESTRATÉGIA: SOFT DELETE (reversível)
--   Marca deleted_at = NOW(). A notificação some da Central e do unread-count
--   (List e GetUnreadCount filtram deleted_at IS NULL). Nada é apagado
--   fisicamente. A PARTE 4 (opcional) faz o DELETE físico depois, se desejado.
--
-- IDEMPOTÊNCIA
--   Todo UPDATE exige  deleted_at IS NULL. Re-rodar o script não tem efeito:
--   linhas já tratadas estão fora do filtro. Seguro rodar mais de uma vez.
--
-- ----------------------------------------------------------------------------
-- COMO RODAR  (decida ANTES de executar — rode apenas UMA das estratégias)
--
--   1. Rode a PARTE 0 (preview, somente leitura) e confira os números.
--   2. (Recomendado em produção) Rode o snapshot de backup da PARTE 1.
--   3. Rode a PARTE 1  ── OU ──  a PARTE 2.  NUNCA as duas.
--        PARTE 1  DEDUP (RECOMENDADO): mantém a notificação MAIS RECENTE por
--                 (tenant, destinatário, lead) e remove as demais. Preserva
--                 um sinal por lead/usuário — inclusive de um SLA real — e
--                 mata só a duplicação.
--        PARTE 2  CORTE POR DATA (ALTERNATIVA): remove TODAS as notificações
--                 de SLA de primeiro contato criadas antes do deploy do fix.
--                 Resultado mais "limpo", porém descarta também a 1ª
--                 notificação legítima de cada lead na janela do bug.
--      A PARTE 1 já vem ATIVA. Para usar a PARTE 2, comente a PARTE 1 e
--      descomente a PARTE 2 (instruções nos marcadores de cada parte).
--   4. Rode a PARTE 3 (verificação, somente leitura).
--
-- TENANT-AWARE
--   - O dedup particiona por tenant_id — nunca cruza tenants.
--   - Para limpar UM tenant por vez, descomente o filtro marcado com
--       -- >>> opcional: escopo de 1 tenant
--     em cada statement e preencha o UUID do tenant.
--   - RLS: comm.inapp_notifications tem Row Level Security por tenant
--     (auth.jwt()). Rode como OWNER da tabela / service_role / superuser para
--     enxergar todos os tenants; um papel comum só enxerga o tenant do JWT.
--
-- TRANSAÇÕES / DRY-RUN
--   As PARTES 1 e 2 são envolvidas em BEGIN; ... COMMIT;. Para um dry-run,
--   troque o COMMIT; final por ROLLBACK; — o UPDATE reporta a contagem de
--   linhas afetadas ("UPDATE <n>") sem persistir. Reverta para COMMIT; quando
--   os números estiverem corretos.
--
-- VOLUME
--   Para tabelas muito grandes (centenas de milhares de linhas-alvo) o UPDATE
--   pode ser longo; considere rodar por tenant (filtro acima) para fatiar.
-- ============================================================================


-- ============================================================================
-- PARTE 0 — PREVIEW  (somente leitura — seguro rodar a qualquer momento)
-- ============================================================================

-- 0.0 — Amostra de linhas. Confira visualmente que o fingerprint casa só com
--        a notificação de SLA de primeiro contato.
SELECT
    id,
    tenant_id,
    created_at,
    metadata->>'user_id'                  AS user_id,
    metadata->>'title'                    AS title,
    metadata->>'content'                  AS content,
    metadata->'data'->>'lead_id'          AS lead_id,
    metadata->'data'->>'sla_minutes'      AS sla_minutes,
    metadata->'data'->>'minutes_overdue'  AS minutes_overdue
FROM comm.inapp_notifications
WHERE deleted_at IS NULL
  AND metadata->>'type'  = 'system'
  AND metadata->>'title' = 'SLA primeiro contato violado'
  AND metadata->'data'->>'sla_minutes'     IS NOT NULL
  AND metadata->'data'->>'minutes_overdue' IS NOT NULL
  AND metadata->'data'->>'lead_id'         IS NOT NULL
ORDER BY created_at DESC
LIMIT 20;

-- 0.1 — Visão geral: total de notificações de SLA de primeiro contato,
--        separando ativas (deleted_at IS NULL) de já removidas.
SELECT
    count(*) FILTER (WHERE deleted_at IS NULL)     AS ativas,
    count(*) FILTER (WHERE deleted_at IS NOT NULL) AS ja_removidas,
    count(*)                                       AS total
FROM comm.inapp_notifications
WHERE metadata->>'type'  = 'system'
  AND metadata->>'title' = 'SLA primeiro contato violado'
  AND metadata->'data'->>'sla_minutes'     IS NOT NULL
  AND metadata->'data'->>'minutes_overdue' IS NOT NULL
  AND metadata->'data'->>'lead_id'         IS NOT NULL;

-- 0.2 — Distribuição por tenant (linhas ativas).
SELECT
    tenant_id,
    count(*) AS ativas
FROM comm.inapp_notifications
WHERE deleted_at IS NULL
  AND metadata->>'type'  = 'system'
  AND metadata->>'title' = 'SLA primeiro contato violado'
  AND metadata->'data'->>'sla_minutes'     IS NOT NULL
  AND metadata->'data'->>'minutes_overdue' IS NOT NULL
  AND metadata->'data'->>'lead_id'         IS NOT NULL
GROUP BY tenant_id
ORDER BY ativas DESC;

-- 0.3 — Piores ofensores: grupos (tenant, destinatário, lead) com mais
--        duplicatas. Expõe o tamanho da enxurrada por grupo.
SELECT
    tenant_id,
    metadata->>'user_id'          AS user_id,
    metadata->'data'->>'lead_id'  AS lead_id,
    count(*)                      AS notificacoes,
    min(created_at)               AS primeira,
    max(created_at)               AS ultima
FROM comm.inapp_notifications
WHERE deleted_at IS NULL
  AND metadata->>'type'  = 'system'
  AND metadata->>'title' = 'SLA primeiro contato violado'
  AND metadata->'data'->>'sla_minutes'     IS NOT NULL
  AND metadata->'data'->>'minutes_overdue' IS NOT NULL
  AND metadata->'data'->>'lead_id'         IS NOT NULL
GROUP BY 1, 2, 3
ORDER BY notificacoes DESC
LIMIT 50;

-- 0.4 — Quanto a PARTE 1 (dedup) removeria — preview, NÃO muta nada.
WITH ranked AS (
    SELECT
        id,
        ROW_NUMBER() OVER (
            PARTITION BY tenant_id,
                         metadata->>'user_id',
                         metadata->'data'->>'lead_id'
            ORDER BY created_at DESC, id DESC
        ) AS rn
    FROM comm.inapp_notifications
    WHERE deleted_at IS NULL
      AND metadata->>'type'  = 'system'
      AND metadata->>'title' = 'SLA primeiro contato violado'
      AND metadata->'data'->>'sla_minutes'     IS NOT NULL
      AND metadata->'data'->>'minutes_overdue' IS NOT NULL
      AND metadata->'data'->>'lead_id'         IS NOT NULL
)
SELECT
    count(*) FILTER (WHERE rn > 1) AS seriam_removidas,
    count(*) FILTER (WHERE rn = 1) AS seriam_mantidas
FROM ranked;

-- 0.5 — Quanto a PARTE 2 (corte por data) removeria — preview, NÃO muta nada.
--        Ajuste o timestamp para o horário REAL do deploy do PR #595.
SELECT count(*) AS seriam_removidas
FROM comm.inapp_notifications
WHERE deleted_at IS NULL
  AND metadata->>'type'  = 'system'
  AND metadata->>'title' = 'SLA primeiro contato violado'
  AND metadata->'data'->>'sla_minutes'     IS NOT NULL
  AND metadata->'data'->>'minutes_overdue' IS NOT NULL
  AND metadata->'data'->>'lead_id'         IS NOT NULL
  AND created_at < TIMESTAMPTZ '2026-05-20 19:16:18+00';  -- <<< EDITE (deploy do fix)


-- ============================================================================
-- PARTE 1 — DEDUP  (RECOMENDADO — esta parte vem ATIVA)
-- ----------------------------------------------------------------------------
-- Mantém a notificação MAIS RECENTE de cada (tenant_id, user_id, lead_id) e
-- soft-deleta as demais.
--
-- Por que particionar por (tenant, user_id, lead) e não só por lead?
--   Cada violação notifica vários destinatários (o corretor responsável e os
--   gestores do tenant), cada um com a SUA própria linha. A Central de
--   Notificações é por usuário (List filtra por user_id). Deduplicar só "por
--   lead" apagaria a notificação dos demais destinatários — então a unidade
--   correta de dedup é (tenant, destinatário, lead): cada usuário fica com no
--   máximo UMA notificação por lead.
--   (Se você realmente quiser no máximo 1 linha por lead no banco inteiro,
--    remova  metadata->>'user_id'  do PARTITION BY abaixo.)
-- ============================================================================

-- (Opcional, recomendado em produção) Snapshot de backup ANTES de mutar.
-- Permite reverter por completo:
--   UPDATE comm.inapp_notifications n
--   SET deleted_at = NULL, updated_at = NOW()
--   FROM comm.inapp_notifications_fc_sla_bkp b
--   WHERE n.id = b.id;
--
-- CREATE TABLE comm.inapp_notifications_fc_sla_bkp AS
-- SELECT *
-- FROM comm.inapp_notifications
-- WHERE deleted_at IS NULL
--   AND metadata->>'type'  = 'system'
--   AND metadata->>'title' = 'SLA primeiro contato violado'
--   AND metadata->'data'->>'sla_minutes'     IS NOT NULL
--   AND metadata->'data'->>'minutes_overdue' IS NOT NULL
--   AND metadata->'data'->>'lead_id'         IS NOT NULL;

BEGIN;

WITH ranked AS (
    SELECT
        id,
        ROW_NUMBER() OVER (
            PARTITION BY tenant_id,
                         metadata->>'user_id',
                         metadata->'data'->>'lead_id'
            ORDER BY created_at DESC, id DESC
        ) AS rn
    FROM comm.inapp_notifications
    WHERE deleted_at IS NULL
      AND metadata->>'type'  = 'system'
      AND metadata->>'title' = 'SLA primeiro contato violado'
      AND metadata->'data'->>'sla_minutes'     IS NOT NULL
      AND metadata->'data'->>'minutes_overdue' IS NOT NULL
      AND metadata->'data'->>'lead_id'         IS NOT NULL
      -- >>> opcional: escopo de 1 tenant — descomente e preencha:
      -- AND tenant_id = '00000000-0000-0000-0000-000000000000'
)
UPDATE comm.inapp_notifications n
SET deleted_at = NOW(),
    updated_at = NOW()
FROM ranked
WHERE n.id = ranked.id
  AND ranked.rn > 1          -- mantém rn = 1 (a mais recente do grupo)
  AND n.deleted_at IS NULL;  -- idempotência: não re-toca linhas já removidas

COMMIT;
-- Dry-run: troque o COMMIT; acima por ROLLBACK; e confira a contagem "UPDATE <n>".


-- ============================================================================
-- PARTE 2 — CORTE POR DATA  (ALTERNATIVA — vem DESATIVADA)
-- ----------------------------------------------------------------------------
-- Soft-deleta TODAS as notificações de SLA de primeiro contato criadas antes
-- do deploy do fix. Use no lugar da PARTE 1 quando quiser zerar a janela do
-- bug por completo (sem manter o "último" sinal de cada lead).
--
-- PARA ATIVAR:
--   1. Comente a PARTE 1 inteira (do BEGIN; ao COMMIT;).
--   2. Remova a linha  /*  e a linha  */  que cercam o bloco abaixo.
--   3. Ajuste o timestamp do deploy.
-- ============================================================================
/*
BEGIN;

UPDATE comm.inapp_notifications
SET deleted_at = NOW(),
    updated_at = NOW()
WHERE deleted_at IS NULL
  AND metadata->>'type'  = 'system'
  AND metadata->>'title' = 'SLA primeiro contato violado'
  AND metadata->'data'->>'sla_minutes'     IS NOT NULL
  AND metadata->'data'->>'minutes_overdue' IS NOT NULL
  AND metadata->'data'->>'lead_id'         IS NOT NULL
  AND created_at < TIMESTAMPTZ '2026-05-20 19:16:18+00'  -- <<< EDITE (deploy do fix)
  -- >>> opcional: escopo de 1 tenant — descomente e preencha:
  -- AND tenant_id = '00000000-0000-0000-0000-000000000000'
;

COMMIT;
-- Dry-run: troque o COMMIT; acima por ROLLBACK; e confira a contagem "UPDATE <n>".
*/


-- ============================================================================
-- PARTE 3 — VERIFICAÇÃO PÓS-LIMPEZA  (somente leitura)
-- ============================================================================

-- 3.1 — Não deve sobrar nenhum grupo (tenant, destinatário, lead) com mais de
--        uma notificação ativa. Se você rodou a PARTE 2, o esperado é 0 linhas.
SELECT
    tenant_id,
    metadata->>'user_id'         AS user_id,
    metadata->'data'->>'lead_id' AS lead_id,
    count(*)                     AS ativas
FROM comm.inapp_notifications
WHERE deleted_at IS NULL
  AND metadata->>'type'  = 'system'
  AND metadata->>'title' = 'SLA primeiro contato violado'
  AND metadata->'data'->>'sla_minutes'     IS NOT NULL
  AND metadata->'data'->>'minutes_overdue' IS NOT NULL
  AND metadata->'data'->>'lead_id'         IS NOT NULL
GROUP BY 1, 2, 3
HAVING count(*) > 1
ORDER BY ativas DESC;

-- 3.2 — Contagem final: ativas vs. removidas.
SELECT
    count(*) FILTER (WHERE deleted_at IS NULL)     AS ativas,
    count(*) FILTER (WHERE deleted_at IS NOT NULL) AS removidas
FROM comm.inapp_notifications
WHERE metadata->>'type'  = 'system'
  AND metadata->>'title' = 'SLA primeiro contato violado'
  AND metadata->'data'->>'sla_minutes'     IS NOT NULL
  AND metadata->'data'->>'minutes_overdue' IS NOT NULL
  AND metadata->'data'->>'lead_id'         IS NOT NULL;


-- ============================================================================
-- PARTE 4 — DELETE FÍSICO  (OPCIONAL — vem DESATIVADA)
-- ----------------------------------------------------------------------------
-- O soft delete já resolve o problema do usuário (some da Central). Rode isto
-- só depois de validar a limpeza e, de preferência, depois de descartar o
-- backup da PARTE 1. Remove fisicamente apenas as linhas de SLA de primeiro
-- contato JÁ soft-deletadas — não toca em nada que esteja ativo.
--
-- PARA ATIVAR: remova as linhas  /*  e  */  abaixo.
-- ============================================================================
/*
BEGIN;

DELETE FROM comm.inapp_notifications
WHERE deleted_at IS NOT NULL
  AND metadata->>'type'  = 'system'
  AND metadata->>'title' = 'SLA primeiro contato violado'
  AND metadata->'data'->>'sla_minutes'     IS NOT NULL
  AND metadata->'data'->>'minutes_overdue' IS NOT NULL
  AND metadata->'data'->>'lead_id'         IS NOT NULL;

COMMIT;
*/
