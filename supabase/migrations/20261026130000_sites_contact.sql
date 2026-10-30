-- Contato da imobiliária projetado na vitrine (ADR 0006).
-- A fonte continua sendo Configurações da empresa no Toka; o toca-sites
-- guarda o snapshot usado no rodapé, no card da imobiliária e nos CTAs.
ALTER TABLE sites.sites
    ADD COLUMN IF NOT EXISTS contact jsonb NOT NULL DEFAULT '{}'::jsonb;
