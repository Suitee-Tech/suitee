-- P2/M1 — concede à role de aplicação (`toca_app`) membership em `authenticated`
-- para que as políticas RLS (escritas `FOR ALL TO authenticated`) passem a valer
-- para ela.
--
-- As roles `toca_app`/`toca_service` são criadas FORA do repositório (SQL editor
-- do Supabase) porque carregam senha de login; esta migration corrige apenas a
-- membership que o setup manual deixou faltando. Sem ela, uma role de aplicação
-- SEM `BYPASSRLS` fica deny-by-default em TODA tabela com RLS: o PostgreSQL não
-- aplica política `TO authenticated` a uma role que não é membro de
-- `authenticated`. Isso quebrou register/login//auth/me e toda query
-- tenant-scoped (o backstop de RLS do achado M1 ficava inerte no sentido errado:
-- negava até o acesso legítimo).
--
-- Idempotente: no-op quando a role ainda não existe (fresh DB antes do setup
-- manual) ou quando a membership já existe.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'toca_app')
       AND NOT EXISTS (
           SELECT 1
           FROM pg_auth_members
           WHERE roleid = 'authenticated'::regrole
             AND member = 'toca_app'::regrole
       ) THEN
        GRANT authenticated TO toca_app;
    END IF;
END $$;
