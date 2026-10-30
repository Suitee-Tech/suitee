package repositories

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
	"testing"
)

// Guarda estrutural: as duas escritas do ledger precisam passar pelo escritor
// privilegiado.
//
// `financial.journal_entries` é append-only por decisão de schema — a migration
// 20260822123000 dropou as policies de escrita e revogou INSERT/UPDATE/DELETE de
// authenticated/anon, deixando só SELECT. Pelo pool de aplicação a escrita
// falha com SQLSTATE 42501 ("new row violates row-level security policy"), que
// foi o 500 de "marcar cobrança como paga" em 30/09/2026. Roda no CI sem banco.
func TestJournalWritesUsePrivilegedWriter(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "journal_postgres.go", nil, 0)
	if err != nil {
		t.Fatalf("parse journal_postgres.go: %v", err)
	}
	bodies := map[string]string{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		var b bytes.Buffer
		if err := printer.Fprint(&b, fset, fn); err != nil {
			t.Fatalf("render %s: %v", fn.Name.Name, err)
		}
		bodies[fn.Name.Name] = b.String()
	}

	for _, method := range []string{"Create", "UpdateStatus"} {
		body, ok := bodies[method]
		if !ok {
			t.Fatalf("%s não encontrada em journal_postgres.go", method)
		}
		if !strings.Contains(body, "serviceTx(ctx)") {
			t.Errorf("%s não considera a transação de serviço: o caminho do consumidor perderia a atomicidade", method)
		}
		wantPool := "QueryServiceRow"
		if method == "UpdateStatus" {
			wantPool = "ExecServiceRows"
		}
		if !strings.Contains(body, wantPool) {
			t.Errorf("%s precisa escrever via %s (pool de aplicação não tem política de escrita no ledger)", method, wantPool)
		}
		for _, forbidden := range []string{"r.db.QueryRow(ctx", "r.db.Exec(ctx", "r.db.QueryRowContext(ctx", "r.db.ExecContext(ctx"} {
			if strings.Contains(body, forbidden) {
				t.Errorf("%s usa %s: o pool de aplicação é negado pela RLS do ledger (SQLSTATE 42501)", method, forbidden)
			}
		}
	}
}
