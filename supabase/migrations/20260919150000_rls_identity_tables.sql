-- P2/M1 — backstop de RLS nas tabelas de identidade e de plumbing.
--
-- A revisão de permissões de 2026-09-19 (achado M1) mediu o estado real do banco:
-- oito tabelas multi-tenant ficavam SEM RLS, e como o pool de aplicação conecta
-- hoje com um role que tem BYPASSRLS (`rolbypassrls = t`), o isolamento dependia
-- 100% do `WHERE tenant_id` escrito à mão em cada query. Um único predicado
-- esquecido virava vazamento cross-tenant, sem backstop nenhum.
--
-- Estado verificado antes desta migration (pg_class.relrowsecurity = false):
--   core.partnerships
--   events.processed_events
--   toca_auth.accounts
--   toca_auth.credentials
--   toca_auth.groups
--   toca_auth.invitations
--   toca_auth.password_resets
--   toca_auth.user_preferences
--
-- As políticas abaixo ainda não filtram nada em produção: com BYPASSRLS o role
-- de aplicação ignora RLS. Elas existem para que o backstop passe a valer no dia
-- em que o pool de aplicação usar um role sem BYPASSRLS — e para que esse dia
-- não seja uma queda: as tabelas que o app consulta pelo pool de aplicação
-- (pkg/database.DB) ganham política vinculada ao JWT, as que só o relay/serviço
-- toca ficam deny-by-default (mesmo padrão já documentado para
-- events.outbox_events/dlq_events em .no-rls-allowlist.txt).
--
-- O boot do serviço reporta o estado do backstop (`database.rls_backstop_disabled`
-- quando o role de aplicação bypassa RLS; `REQUIRE_RLS_BACKSTOP=true` transforma
-- isso em falha de boot). O role de serviço continua obrigatoriamente com
-- BYPASSRLS, verificado em pkg/database (ErrServiceRoleRequired).

-- 1. Liga RLS nas oito tabelas. Idempotente e independente de extensões.
ALTER TABLE IF EXISTS toca_auth.accounts ENABLE ROW LEVEL SECURITY;
ALTER TABLE IF EXISTS toca_auth.credentials ENABLE ROW LEVEL SECURITY;
ALTER TABLE IF EXISTS toca_auth.invitations ENABLE ROW LEVEL SECURITY;
ALTER TABLE IF EXISTS toca_auth.groups ENABLE ROW LEVEL SECURITY;
ALTER TABLE IF EXISTS toca_auth.user_preferences ENABLE ROW LEVEL SECURITY;
ALTER TABLE IF EXISTS toca_auth.password_resets ENABLE ROW LEVEL SECURITY;
ALTER TABLE IF EXISTS core.partnerships ENABLE ROW LEVEL SECURITY;
ALTER TABLE IF EXISTS events.processed_events ENABLE ROW LEVEL SECURITY;

-- 2. Políticas vinculadas ao JWT para o que o pool de aplicação lê/escreve.
--
-- Guarda: em Postgres "puro" (testes de integração, QA) `auth.jwt()`/`auth.uid()`
-- não existem; nesse caso as tabelas ficam com RLS ligado e sem política
-- (deny-by-default para qualquer role sujeito a RLS), exatamente o que já
-- acontece com as demais tabelas multi-tenant nesse ambiente.
DO $$
BEGIN
  -- Atenção: `to_regproc` recebe "schema.nome" (sem parênteses); usar
  -- `to_regproc('auth.jwt()')` devolve NULL mesmo com a função presente — foi
  -- assim que o bloco dinâmico de 20260421120000_finalize_jsonb_rls.sql virou
  -- no-op. Aqui a checagem é por `to_regprocedure` com a assinatura completa.
  IF to_regprocedure('auth.jwt()') IS NULL OR to_regprocedure('auth.uid()') IS NULL THEN
    RAISE NOTICE 'auth.jwt()/auth.uid() ausentes: RLS ligado sem política (deny-by-default) nas tabelas de identidade';
    RETURN;
  END IF;

  -- Conta: a identidade é o próprio `sub` do JWT (`toca_auth.accounts.id` é o
  -- account_id que aparece nos claims).
  EXECUTE 'DROP POLICY IF EXISTS account_isolation ON toca_auth.accounts';
  EXECUTE 'CREATE POLICY account_isolation ON toca_auth.accounts
             FOR ALL TO authenticated
             USING (id = (SELECT auth.uid()))
             WITH CHECK (id = (SELECT auth.uid()))';

  -- Credencial de login: pertence a uma conta.
  EXECUTE 'DROP POLICY IF EXISTS account_isolation ON toca_auth.credentials';
  EXECUTE 'CREATE POLICY account_isolation ON toca_auth.credentials
             FOR ALL TO authenticated
             USING (account_id = (SELECT auth.uid()))
             WITH CHECK (account_id = (SELECT auth.uid()))';

  -- Preferências: por conta.
  EXECUTE 'DROP POLICY IF EXISTS account_isolation ON toca_auth.user_preferences';
  EXECUTE 'CREATE POLICY account_isolation ON toca_auth.user_preferences
             FOR ALL TO authenticated
             USING (account_id = (SELECT auth.uid()))
             WITH CHECK (account_id = (SELECT auth.uid()))';

  -- Convites e grupos são da EMPRESA (= tenant do JWT).
  EXECUTE 'DROP POLICY IF EXISTS tenant_isolation ON toca_auth.invitations';
  EXECUTE 'CREATE POLICY tenant_isolation ON toca_auth.invitations
             FOR ALL TO authenticated
             USING (company_id = NULLIF((SELECT auth.jwt()) -> ''app_metadata'' ->> ''tenant_id'', '''')::uuid)
             WITH CHECK (company_id = NULLIF((SELECT auth.jwt()) -> ''app_metadata'' ->> ''tenant_id'', '''')::uuid)';

  EXECUTE 'DROP POLICY IF EXISTS tenant_isolation ON toca_auth.groups';
  EXECUTE 'CREATE POLICY tenant_isolation ON toca_auth.groups
             FOR ALL TO authenticated
             USING (company_id = NULLIF((SELECT auth.jwt()) -> ''app_metadata'' ->> ''tenant_id'', '''')::uuid)
             WITH CHECK (company_id = NULLIF((SELECT auth.jwt()) -> ''app_metadata'' ->> ''tenant_id'', '''')::uuid)';

  -- Parceria entre DUAS empresas: cada lado enxerga a linha. Caminhos de worker
  -- que precisam ler a parceria fora do tenant do chamador (split de comissão e
  -- royalty no toca-financial) usam o pool de serviço
  -- (pkg/database.RunInServiceContext), que é o dono legítimo do acesso
  -- cross-tenant.
  EXECUTE 'DROP POLICY IF EXISTS tenant_isolation ON core.partnerships';
  EXECUTE 'CREATE POLICY tenant_isolation ON core.partnerships
             FOR ALL TO authenticated
             USING (
               requester_id = NULLIF((SELECT auth.jwt()) -> ''app_metadata'' ->> ''tenant_id'', '''')::uuid
               OR target_id = NULLIF((SELECT auth.jwt()) -> ''app_metadata'' ->> ''tenant_id'', '''')::uuid
             )
             WITH CHECK (
               requester_id = NULLIF((SELECT auth.jwt()) -> ''app_metadata'' ->> ''tenant_id'', '''')::uuid
               OR target_id = NULLIF((SELECT auth.jwt()) -> ''app_metadata'' ->> ''tenant_id'', '''')::uuid
             )';
END $$;

-- Sem política, de propósito:
--   toca_auth.password_resets — fluxo de token local removido no P1/A14
--     (POST /auth/reset-password); a redefinição acontece no Supabase Auth. A
--     tabela continua existindo por compatibilidade e fica deny-by-default.
--   events.processed_events — dedup do relay de eventos
--     (pkg/events/rabbitmq.go), acessado só pelo pool de serviço.
--
-- CAMINHOS PRÉ-AUTENTICAÇÃO (registrado como continuação do M1, NÃO resolvido
-- aqui). O pool de aplicação monta `request.jwt.claims` a partir do CONTEXTO da
-- requisição (pkg/database.applyJWTClaimsHook). Requisições sem usuário
-- autenticado — login/registro (`toca_auth.credentials` por e-mail,
-- `toca_auth.accounts` por id), convite público por token
-- (`toca_auth.invitations`) e reset de senha — não têm claim nenhuma, então as
-- políticas acima NÃO as atendem. Enquanto `DATABASE_URI` for um role com
-- BYPASSRLS (estado atual, reportado no boot) isso é irrelevante; no dia em que
-- o role de aplicação deixar de bypassar, essas leituras precisam passar pelo
-- pool de serviço (`pkg/database.QueryServiceRow(s)`/`RunInServiceContext`,
-- que é o dono legítimo do acesso cross-tenant por token/e-mail) — a mesma
-- regra vale para o worker de royalty do toca-financial, que lê
-- `core.partnerships` fora do tenant do chamador. O boot reporta
-- `database.rls_backstop_disabled` justamente para que a troca de role seja uma
-- decisão consciente, e `REQUIRE_RLS_BACKSTOP=true` a torna bloqueante.
