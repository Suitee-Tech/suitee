-- Um corretor por empresa: a identidade do corretor é (tenant_id, id).
--
-- `id` é o account_id da pessoa — é o valor que a roleta compara com o JWT para
-- aceitar/recusar atribuição — e a mesma pessoa pode ser corretora em mais de
-- uma imobiliária. Com a chave primária só em `id`, a segunda empresa esbarrava
-- na linha da primeira: o upsert do sync de membros (POST /brokers, disparado
-- ao abrir as telas que carregam corretores) caía na política de RLS com
-- "new row violates row-level security policy (USING expression)" e a rota
-- devolvia 500 a cada carregamento, com o cliente reportando erro de rede.
--
-- Todas as leituras do código já são tenant-scoped (`WHERE tenant_id = $1 AND
-- id = $2`), então passar a chave para (tenant_id, id) não cria ambiguidade:
-- apenas permite que a mesma pessoa tenha o registro próprio em cada empresa.
DO $$
DECLARE
    current_def text;
BEGIN
    SELECT pg_get_constraintdef(c.oid)
      INTO current_def
      FROM pg_constraint c
     WHERE c.conrelid = 'crm.brokers'::regclass
       AND c.contype = 'p';

    IF current_def IS DISTINCT FROM 'PRIMARY KEY (tenant_id, id)' THEN
        ALTER TABLE crm.brokers DROP CONSTRAINT IF EXISTS brokers_pkey;
        ALTER TABLE crm.brokers ADD CONSTRAINT brokers_pkey PRIMARY KEY (tenant_id, id);
    END IF;
END $$;
