-- Roles de runtime do projeto.
--
-- Em beta/produção elas são criadas fora do repositório (SQL Editor), porque
-- carregam senha: ver docs/deployment/production-deploy.md. O Supabase CLI
-- aplica este arquivo antes das migrations em `supabase start` (e em
-- `db push --include-roles`), então o banco local precisa das roles para que
-- migrations que fazem REVOKE/GRANT nelas não falhem com "role does not exist".
--
-- Idempotente: no-op se as roles já existirem. Sem LOGIN/PASSWORD de propósito;
-- a stack local conecta como `postgres`, não como as roles de runtime.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'toca_app') THEN
        EXECUTE 'CREATE ROLE toca_app NOLOGIN';
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'toca_service') THEN
        EXECUTE 'CREATE ROLE toca_service NOLOGIN BYPASSRLS';
    END IF;
END
$$;
