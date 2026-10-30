package database

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Guarda estrutural: `PostgresDB` embute `*sqlx.DB`, então se o pacote deixar de
// declarar `QueryContext`/`QueryRowContext`/`ExecContext`, os métodos PROMOVIDOS
// do pool de aplicação voltam a ser usados — e os repositórios (que escrevem
// `r.db.QueryRowContext(ctx, ...)`) passariam a ignorar a transação do ctx,
// fazendo `RunInServiceContext` virar decoração silenciosa. Foi exatamente o
// defeito medido no beta em 2026-09-30 (ver o teste de integração).
func TestContextHelpersAreDeclaredOnPostgresDB(t *testing.T) {
	source, err := os.ReadFile("postgres.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"QueryContext", "QueryRowContext", "ExecContext"} {
		pattern := regexp.MustCompile(`func \(p \*PostgresDB\) ` + method + `\(`)
		if !pattern.Match(source) {
			t.Errorf("pkg/database não declara %s em PostgresDB: o método promovido do *sqlx.DB (pool de aplicação) voltaria a valer e a transação do ctx seria ignorada", method)
		}
	}
	if !strings.Contains(string(source), "func (p *PostgresDB) QueryRowContext(ctx context.Context") {
		t.Error("assinatura de QueryRowContext mudou; confirme se continua tx-aware")
	}
	// Depois desta rodada os helpers não escolhem o pool inline: passam pelo
	// `executor`, que decide transação do ctx → escopo de serviço → pool de
	// aplicação. Se alguém voltar a chamar p.DB direto, os dois caminhos
	// (RunInServiceContext e WithServiceScope) morrem em silêncio.
	for _, method := range []string{"QueryContext", "QueryRowContext", "ExecContext"} {
		pattern := regexp.MustCompile(`func \(p \*PostgresDB\) ` + method + `\(ctx context.Context[^\{]*\{\s*return p\.executor\(ctx\)`)
		if !pattern.Match(source) {
			t.Errorf("%s não delega para p.executor(ctx): a escolha do pool (tx / escopo de serviço / app) precisa ficar num lugar só", method)
		}
	}
}
