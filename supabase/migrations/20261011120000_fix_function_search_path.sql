-- Fixa o search_path das funções de guarda do financeiro.
--
-- Lint do Supabase: `function_search_path_mutable` (10 ocorrências em produção,
-- 11/10/2026 — todas na financial). Sem search_path fixo, nomes NÃO qualificados
-- dentro da função são resolvidos pelo search_path de QUEM CHAMA: quem conseguir
-- criar objetos num schema que apareça antes na busca pode sombrear uma tabela ou
-- função usada pela guarda (vetor clássico de escalada via search_path). As funções
-- abaixo decidem o que pode ser lançado, alterado ou apagado no livro contábil —
-- exatamente as que não podem depender do ambiente do chamador.
--
-- Fixamos em `pg_catalog, financial`, e não em `''`: nada além do catálogo e do
-- próprio schema entra na busca (não há o que sombrear), e referências não
-- qualificadas a objetos do financeiro continuariam resolvendo. Verificado antes de
-- fixar: os corpos já qualificam tudo (`pg_catalog.pg_roles`, `financial.…`), então
-- o comportamento não muda — o que muda é a garantia.
--
-- Efeito no comportamento: nenhum. Estas são funções de trigger e continuam sendo
-- executadas pelo mesmo papel, com as mesmas regras; só a resolução de nomes deixa
-- de acompanhar o chamador.
DO $$
DECLARE
  assinaturas text[] := ARRAY[
    'financial.guard_journal_entry_delete()',
    'financial.guard_journal_entry_insert()',
    'financial.guard_journal_entry_insert_strict()',
    'financial.guard_journal_entry_truncate()',
    'financial.guard_journal_entry_update()',
    'financial.guard_journal_entry_update_timestamp()',
    'financial.guard_managed_bill_insert()',
    'financial.guard_managed_bill_update()',
    'financial.is_managed_bill_payment_reversal_witness(uuid,uuid,text,text,bigint,text,text,timestamp with time zone,timestamp with time zone,text,text,timestamp with time zone,text)',
    'financial.require_single_journal_reversal_at_commit()'
  ];
  assinatura text;
  alvo regprocedure;
BEGIN
  FOREACH assinatura IN ARRAY assinaturas LOOP
    alvo := to_regprocedure(assinatura);
    IF alvo IS NULL THEN
      -- Ambiente sem a função (ex.: banco local pela metade): não é erro.
      RAISE NOTICE 'check-function-search-path: % não existe neste ambiente; ignorando', assinatura;
      CONTINUE;
    END IF;
    EXECUTE format('ALTER FUNCTION %s SET search_path = pg_catalog, financial', alvo);
    RAISE NOTICE 'check-function-search-path: search_path fixado em %', assinatura;
  END LOOP;
END $$;

-- Autoteste da própria migração: se alguma função da financial continuar sem
-- search_path fixo, a migração FALHA em vez de deixar o lint em pé.
DO $$
DECLARE
  pendentes text;
BEGIN
  SELECT string_agg(p.proname, ', ' ORDER BY p.proname)
    INTO pendentes
    FROM pg_proc p
    JOIN pg_namespace n ON n.oid = p.pronamespace
   WHERE n.nspname = 'financial'
     AND p.proconfig IS NULL;

  IF pendentes IS NOT NULL THEN
    RAISE EXCEPTION 'funções da financial sem search_path fixo: %', pendentes;
  END IF;
END $$;
