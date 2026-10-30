-- core.partnerships ganhou as colunas que o repositório do toca-core já
-- consulta. O repositório (toca-core/internal/repository/partnership_postgres.go)
-- filtra `deleted_at IS NULL` em List/GetByID/Delete e grava `updated_at` em
-- Update — mas a DDL original da tabela (20231026000000_init_jsonb_pilot.sql)
-- omitiu as duas colunas. Resultado: `GET /partnerships` falhava com
-- "column \"deleted_at\" does not exist" → HTTP 500, derrubando a página
-- /parceiros inteira.
--
-- Espelha o padrão de core.properties (criada na mesma migration de origem),
-- que já trazia updated_at + deleted_at para soft-delete e auditoria.

ALTER TABLE core.partnerships
    ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE core.partnerships
    ADD COLUMN IF NOT EXISTS deleted_at timestamptz;

-- Acelera os filtros `deleted_at IS NULL` presentes em todas as queries de leitura.
CREATE INDEX IF NOT EXISTS idx_core_partnerships_deleted_at
    ON core.partnerships (deleted_at);
