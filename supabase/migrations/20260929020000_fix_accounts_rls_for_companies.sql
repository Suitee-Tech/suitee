-- P2/M1 (continuação) — corrige a política RLS de `toca_auth.accounts` para o
-- modelo de EMPRESA. A política original (`id = auth.uid()`) só atendia a conta
-- INDIVIDUAL; contas de empresa (id = id da empresa) são compartilhadas por vários
-- membros e ficavam deny-by-default para todos — criar/listar/ler/atualizar empresa
-- falhavam com "new row violates row-level security policy for table accounts".
--
-- O `metadata` é JSONB serializado pelo domínio Go: as chaves de `Company` saem com
-- o nome do campo (ex.: `OwnerID`) e os campos de cada Membro saem em snake_case
-- (tag `json:`): `account_id`, `profile`, `functional_role`, `is_active`.
--
-- Regras:
--   SELECT: a própria conta individual (id = auth.uid()) OU uma empresa em que o
--           chamador é dono ou membro ativo.
--   INSERT: a própria conta individual OU uma empresa criada com o chamador como
--           dono (`Company.OwnerID = auth.uid()`).
--   UPDATE: a própria conta OU uma empresa em que o chamador é dono/admin ativo.
--   DELETE: a própria conta OU uma empresa em que o chamador é dono.
--
-- Idempotente: DROP IF EXISTS + CREATE. As contas de empresa já existentes passam
-- a ser lidas por seus donos/membros na próxima requisição.

DROP POLICY IF EXISTS account_isolation ON toca_auth.accounts;
DROP POLICY IF EXISTS account_insert_owner ON toca_auth.accounts;
DROP POLICY IF EXISTS account_update_owner ON toca_auth.accounts;
DROP POLICY IF EXISTS account_delete_owner ON toca_auth.accounts;

-- SELECT
CREATE POLICY account_isolation ON toca_auth.accounts
  FOR SELECT TO authenticated
  USING (
    id = auth.uid()
    OR (
      metadata->>'Type' = 'company'
      AND (
        metadata->'Company'->>'OwnerID' = auth.uid()::text
        OR EXISTS (
          SELECT 1 FROM jsonb_array_elements(metadata->'Company'->'Members') AS m
          WHERE m->>'account_id' = auth.uid()::text
            AND COALESCE((m->>'is_active')::boolean, true)
        )
      )
    )
  );

-- INSERT
CREATE POLICY account_insert_owner ON toca_auth.accounts
  FOR INSERT TO authenticated
  WITH CHECK (
    id = auth.uid()
    OR (
      metadata->>'Type' = 'company'
      AND metadata->'Company'->>'OwnerID' = auth.uid()::text
    )
  );

-- UPDATE (USING = estado antigo; WITH CHECK = estado novo)
CREATE POLICY account_update_owner ON toca_auth.accounts
  FOR UPDATE TO authenticated
  USING (
    id = auth.uid()
    OR (
      metadata->>'Type' = 'company'
      AND (
        metadata->'Company'->>'OwnerID' = auth.uid()::text
        OR EXISTS (
          SELECT 1 FROM jsonb_array_elements(metadata->'Company'->'Members') AS m
          WHERE m->>'account_id' = auth.uid()::text
            AND m->>'profile' IN ('owner', 'admin')
            AND COALESCE((m->>'is_active')::boolean, true)
        )
      )
    )
  )
  WITH CHECK (
    id = auth.uid()
    OR (
      metadata->>'Type' = 'company'
      AND (
        metadata->'Company'->>'OwnerID' = auth.uid()::text
        OR EXISTS (
          SELECT 1 FROM jsonb_array_elements(metadata->'Company'->'Members') AS m
          WHERE m->>'account_id' = auth.uid()::text
            AND m->>'profile' IN ('owner', 'admin')
            AND COALESCE((m->>'is_active')::boolean, true)
        )
      )
    )
  );

-- DELETE (só o dono remove a própria conta ou a empresa)
CREATE POLICY account_delete_owner ON toca_auth.accounts
  FOR DELETE TO authenticated
  USING (
    id = auth.uid()
    OR (
      metadata->>'Type' = 'company'
      AND metadata->'Company'->>'OwnerID' = auth.uid()::text
    )
  );
