// check-tenant-scoped-writes enforces the rule that the 2026-09-19 permissions
// review (item 36 / findings A20+B12) turned into an invariant: every
// `UPDATE <schema>.<table>` / `DELETE FROM <schema>.<table>` on a tenant-scoped
// aggregate must carry the tenant predicate — either in the statement itself or
// in the function that builds it (several repositories concatenate a `where`
// built with `tenant_id`).
//
// Why: RLS is inert for the application role (services connect with a role that
// bypasses it), so `WHERE id = $1` alone lets a body-supplied aggregate
// overwrite another tenant's row. The failure mode already happened (finding
// C9). A caller that did a tenant-scoped GetByID first is not enough: the write
// itself has to be scoped.
//
// Tables with no tenant column (identity rows keyed by id/email/token, system
// event plumbing) are listed in `exemptTables` with the reason, mirroring the
// RLS allowlist.
//
// Usage:
//
//	go run ./scripts/check-tenant-scoped-writes -root .
//
// Exit codes: 0 ok, 1 violation, 2 usage/IO error.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// modules são os diretórios varridos.
var modules = []string{
	"pkg", "gateway", "toca-auth", "toca-core", "toca-crm", "toca-deal",
	"toca-comm", "toca-financial", "toca-signature", "toca-ops",
	"toca-calendar", "toca-portal", "toca-ai", "toca-mcp",
}

// tenantColumns são as colunas aceitas como predicado de tenant.
//
// `requester_id`/`target_id` entram porque uma parceria liga DOIS tenants: o
// predicado correto é "o ator é uma das pontas" (core.partnerships).
// `company_id` é o tenant nas tabelas de identidade do toca_auth.
var tenantColumns = []string{"tenant_id", "company_id", "requester_id", "target_id"}

// exemptTables são tabelas cujo UPDATE/DELETE não é escopado por tenant, com o
// motivo — o mesmo critério do allowlist de RLS.
var exemptTables = map[string]string{
	"toca_auth.accounts":         "linha de identidade: chave é o próprio account id (não há coluna de tenant)",
	"toca_auth.credentials":      "credencial de login: chave é email/account_id (pré-autenticação)",
	"toca_auth.password_resets":  "token de reset: chave é o token (pré-autenticação)",
	"events.outbox_events":       "relay do outbox: linha de sistema, tenant_id é nullable e o worker usa o pool de serviço",
	"events.dlq_events":          "dead-letter: linha de sistema, replay pelo pool de serviço",
	"events.processed_events":    "dedup do relay: chave é (consumer, message_id)",
	"signature.webhook_events":   "evento de webhook: chave é o id do evento entregue pelo provedor",
	"toca_auth.tenant_settings":  "configuração por tenant chaveada por tenant_id (uso explícito na chave)",
	"core.role_permissions":      "matriz de papéis: chave é (tenant_id, role) e o tenant está na chave",
	"core.company_settings":      "configuração por tenant chaveada por tenant_id",
	"core.appearance_settings":   "configuração por tenant chaveada por tenant_id",
	"core.notification_settings": "configuração por tenant chaveada por tenant_id",
	"core.api_keys":              "chave de API: revogação por id com tenant na chave da consulta",
	"core.webhooks":              "webhook de integração: chave é o id do endpoint",
	"financial.idempotency_keys": "controle de idempotência: chave é a própria chave",
	"financial.webhook_events":   "evento de provedor: chave é o id do evento",
	"ops.suppliers":              "fornecedor já recebe tenant na assinatura do repositório (Delete(ctx, tenantID, id))",
	"exports.jobs":               "fila GLOBAL do worker de exportação: o claim pega o próximo job de QUALQUER tenant (o tenant é preservado na linha e as rotas de leitura são escopadas) — filtrar por tenant aqui pararia a fila dos outros",
	"crm.leads":                  "leads usam where dinâmico com tenant_id (patchWithSeq/patchWithoutSeq) — caso coberto pela heurística de função, listado aqui só quando o literal é concatenado",
}

var (
	writeRe    = regexp.MustCompile(`(?is)\b(UPDATE|DELETE\s+FROM)\s+([a-z_][a-z0-9_]*\.[a-z_][a-z0-9_]*)`)
	rawStrRe   = regexp.MustCompile("(?s)`([^`]*)`")
	plainStrRe = regexp.MustCompile(`"([^"\n]*)"`)
	funcRe     = regexp.MustCompile(`(?m)^func\s`)
)

type violation struct {
	File   string
	Line   int
	Table  string
	Detail string
}

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()

	if _, err := os.Stat(filepath.Join(*root, "go.work")); err != nil {
		fmt.Fprintf(os.Stderr, "check-tenant-scoped-writes: %s does not look like the repo root (no go.work)\n", *root)
		os.Exit(2)
	}

	var violations []violation
	scanned := 0
	for _, module := range modules {
		base := filepath.Join(*root, module)
		err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			name := info.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				return nil
			}
			src, rerr := os.ReadFile(path)
			if rerr != nil {
				return nil
			}
			rel, _ := filepath.Rel(*root, path)
			scanned++
			violations = append(violations, scanFile(rel, string(src))...)
			return nil
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "check-tenant-scoped-writes: %v\n", err)
			os.Exit(2)
		}
	}

	sort.Slice(violations, func(i, j int) bool {
		if violations[i].File != violations[j].File {
			return violations[i].File < violations[j].File
		}
		return violations[i].Line < violations[j].Line
	})

	for _, v := range violations {
		fmt.Printf("%s:%d: [item36] %s — %s\n", v.File, v.Line, v.Table, v.Detail)
	}
	if len(violations) > 0 {
		fmt.Printf("\ncheck-tenant-scoped-writes: %d statement(s) sem predicado de tenant\n", len(violations))
		os.Exit(1)
	}
	fmt.Printf("check-tenant-scoped-writes: ok (%d arquivos varridos)\n", scanned)
}

// scanFile devolve as violações de um arquivo.
func scanFile(rel, src string) []violation {
	var out []violation

	// Limites das funções, para a heurística "o predicado é montado na função".
	type fnRange struct{ start, end int }
	var fns []fnRange
	indexes := funcRe.FindAllStringIndex(src, -1)
	for i, idx := range indexes {
		end := len(src)
		if i+1 < len(indexes) {
			end = indexes[i+1][0]
		}
		fns = append(fns, fnRange{start: idx[0], end: end})
	}
	fnBody := func(pos int) string {
		for _, fn := range fns {
			if pos >= fn.start && pos < fn.end {
				return src[fn.start:fn.end]
			}
		}
		return ""
	}

	matches := append(rawStrRe.FindAllStringSubmatchIndex(src, -1), plainStrRe.FindAllStringSubmatchIndex(src, -1)...)
	for _, m := range matches {
		if len(m) < 4 || m[2] < 0 {
			continue
		}
		literal := src[m[2]:m[3]]
		for _, w := range writeRe.FindAllStringSubmatchIndex(literal, -1) {
			table := strings.ToLower(literal[w[4]:w[5]])
			stmt := literal[w[0]:]
			if _, exempt := exemptTables[table]; exempt {
				continue
			}
			if hasTenantPredicate(stmt) {
				continue
			}
			body := fnBody(m[2])
			if body != "" && hasTenantPredicate(body) {
				continue
			}
			line := strings.Count(src[:m[2]], "\n") + 1
			out = append(out, violation{
				File:   rel,
				Line:   line,
				Table:  table,
				Detail: "UPDATE/DELETE de agregado sem predicado de tenant (use AND tenant_id = $n na própria escrita, não só um GetByID escopado antes)",
			})
		}
	}
	return out
}

// hasTenantPredicate informa se o texto menciona uma coluna de tenant.
func hasTenantPredicate(text string) bool {
	lower := strings.ToLower(text)
	for _, column := range tenantColumns {
		if strings.Contains(lower, column) {
			return true
		}
	}
	return false
}
