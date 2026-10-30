package main

import (
	"os"
	"path/filepath"
	"testing"
)

// escreve um diretório de migrações temporário e roda a checagem como o make roda.
func checar(t *testing.T, arquivos map[string]string) (faltando int) {
	t.Helper()
	dir := t.TempDir()
	for nome, conteudo := range arquivos {
		if err := os.WriteFile(filepath.Join(dir, nome), []byte(conteudo), 0o600); err != nil {
			t.Fatalf("escrever %s: %v", nome, err)
		}
	}
	files, err := listSQLFiles(dir)
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	allow, _ := loadAllowlist(filepath.Join(dir, "inexistente.txt"))
	criadas, ordem, pinadas, err := coletar(files)
	if err != nil {
		t.Fatalf("coletar: %v", err)
	}
	return len(funcoesSemPath(ordem, criadas, pinadas, allow))
}

func TestFuncaoComSearchPathNaDefinicaoPassa(t *testing.T) {
	if n := checar(t, map[string]string{"1.sql": `
CREATE OR REPLACE FUNCTION financial.f() RETURNS trigger LANGUAGE plpgsql
SET search_path = pg_catalog, financial AS $$ BEGIN RETURN NEW; END $$;`}); n != 0 {
		t.Fatalf("esperava 0 pendências, veio %d", n)
	}
}

func TestFuncaoSemSearchPathNenhumFalha(t *testing.T) {
	if n := checar(t, map[string]string{"1.sql": `
CREATE OR REPLACE FUNCTION financial.f() RETURNS trigger LANGUAGE plpgsql
AS $$ BEGIN RETURN NEW; END $$;`}); n != 1 {
		t.Fatalf("esperava 1 pendência, veio %d", n)
	}
}

// O backfill dinâmico (o que a migração 20261011120000 faz) precisa contar como
// cobertura, senão o check acusa funções já corrigidas.
func TestBackfillDinamicoCobreAsFuncoes(t *testing.T) {
	if n := checar(t, map[string]string{
		"1.sql": `CREATE OR REPLACE FUNCTION financial.f() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$;`,
		"2.sql": `DO $$ DECLARE a text[] := ARRAY['financial.f()']; BEGIN
		            FOREACH x IN ARRAY a LOOP
		              EXECUTE format('ALTER FUNCTION %s SET search_path = pg_catalog, financial', to_regprocedure(x));
		            END LOOP; END $$;`,
	}); n != 0 {
		t.Fatalf("esperava 0 pendências, veio %d", n)
	}
}

// Função de schema da plataforma não é nossa para fixar.
func TestSchemaDePlataformaIgnorado(t *testing.T) {
	if n := checar(t, map[string]string{"1.sql": `
CREATE OR REPLACE FUNCTION auth.hook() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$;`}); n != 0 {
		t.Fatalf("esperava 0 pendências, veio %d", n)
	}
}

// ALTER estático também cobre.
func TestAlterEstaticoCobre(t *testing.T) {
	if n := checar(t, map[string]string{
		"1.sql": `CREATE OR REPLACE FUNCTION financial.f() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$;`,
		"2.sql": `ALTER FUNCTION financial.f() SET search_path = pg_catalog, financial;`,
	}); n != 0 {
		t.Fatalf("esperava 0 pendências, veio %d", n)
	}
}
