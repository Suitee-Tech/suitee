-- Deduplicação de criação de negócios (esteira digital).
--
-- Um duplo envio do formulário de criação produzia dois negócios com as mesmas
-- partes, imóvel e valor (audit: dois `create` com ~1,1s de diferença, mesmo
-- ator). Duas proteções entram no esquema:
--
-- 1. Chave de idempotência por INTENÇÃO de criação: o mesmo formulário
--    reenviado (duplo clique, retry de rede) usa a mesma chave; a unicidade
--    parcial garante que só um negócio ATIVO pode carregar a chave. Um negócio
--    cancelado libera a chave para uma nova intenção legítima.
-- 2. Índice de suporte à guarda de janela (mesmo imóvel + cliente + tipo +
--    valor criado em segundos), usada quando o cliente não envia a chave.
CREATE UNIQUE INDEX IF NOT EXISTS uq_deal_deals_idempotency_key
    ON deal.deals (tenant_id, (metadata ->> 'idempotency_key'))
    WHERE deleted_at IS NULL
      AND (metadata ->> 'idempotency_key') IS NOT NULL
      AND COALESCE(metadata ->> 'status', '') <> 'CANCELLED';

CREATE INDEX IF NOT EXISTS idx_deal_deals_duplicate_window
    ON deal.deals (tenant_id, (metadata ->> 'property_id'), (metadata ->> 'customer_id'), (metadata ->> 'type'), created_at DESC)
    WHERE deleted_at IS NULL;
