-- O schema sites é criado pela role de migration; o toca-sites conecta com as
-- roles de runtime (toca_app/toca_service). Sem USAGE e grants de tabela o
-- painel recebe 500 em /sites/me mesmo com o schema aplicado.
--
-- Idempotente: em bancos onde as roles não existem (testes locais), não faz nada.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'toca_app') THEN
    GRANT USAGE ON SCHEMA sites TO toca_app;
    GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA sites TO toca_app;
    GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA sites TO toca_app;
    ALTER DEFAULT PRIVILEGES IN SCHEMA sites
      GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO toca_app;
    ALTER DEFAULT PRIVILEGES IN SCHEMA sites
      GRANT USAGE, SELECT ON SEQUENCES TO toca_app;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'toca_service') THEN
    GRANT USAGE ON SCHEMA sites TO toca_service;
    GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA sites TO toca_service;
    GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA sites TO toca_service;
    ALTER DEFAULT PRIVILEGES IN SCHEMA sites
      GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO toca_service;
    ALTER DEFAULT PRIVILEGES IN SCHEMA sites
      GRANT USAGE, SELECT ON SEQUENCES TO toca_service;
  END IF;
END
$$;
