// check-route-guards scans the route registration files of every service and
// enforces the boundary rules that the 2026-09-19 permissions review
// (docs/reviews/2026-09-19-permissoes-acesso-review.md) turned into invariants:
//
//  1. P0/C10 — the public edge must never register a path containing
//     "/internal": those routes authenticate with a static shared key and take
//     the tenant from the caller.
//  2. P0/A11 — the operator surfaces of toca-deal, toca-signature, toca-comm
//     and toca-auth must reject the `cliente` functional role, while the
//     portal groups must keep requiring it. A portal token reaching the
//     operator plane is a cross-plane escalation: the stored tenant matrix can
//     grant it `deals:read`/`documents:write`, and the only thing standing in
//     the way was the absence of a role gate. The rule is per GROUP
//     DECLARATION, not per path: every group declaring a policy prefix must
//     carry the strict guard or be listed in clienteGuardExceptions with its
//     reason (a sibling group with the same prefix — `onboarding` in toca-auth,
//     `channels` in toca-comm — used to escape silently), and a route
//     registered with an absolute path under a policy prefix counts too.
//  3. P1/A7+M3 — every permission required by a route must EXIST in the
//     catalog (pkg/auth/permission_catalog.go). A route gated by a permission
//     that does not exist is unreachable for every functional role (only root
//     profiles pass), which is exactly how `finance:delete` made
//     `DELETE /transactions/:id` dead for the whole tenant. This is the server
//     half of "a UI-guarded screen needs a server-guarded route".
//
// The check is intentionally textual and conservative: it fails when a rule is
// violated in an obvious way and stays quiet when the file does not match the
// expected shape, so it cannot block a refactor it does not understand.
//
//  5. P1/A2 — every service that REGISTERS an internal route must declare
//     `INTERNAL_SERVICE_KEYS` in the compose stacks. The rollout of per-service
//     keys is configuration (each caller presents its own key, each receiver
//     lists `nome:chave[:tenants]`); without the env in the stack the service
//     silently stays on the shared legacy key, and the migration never completes.
//
//  6. P2/M8 — no route file may compare `X-Internal-Key` by hand: internal
//     endpoints use `RequireInternalKey()` (constant-time compare, per-IP rate
//     limit, audit log, named keys). The hand-rolled version in toca-auth was
//     non-constant, silent and rejected the named keys.
//
//  7. P2/M16 — every Swagger registration must go through `apidocs.Register`,
//     the only place that knows the release gate. A raw `v1.GET("/swagger/...")`
//     ships the API map in production (and in signature it was registered
//     before the auth middleware, so auth depended on registration order).
//
//  8. P2/B1 — the gateway's PUBLIC engine must not register `/metrics`, and the
//     release stacks must not publish a Traefik router for it. Metrics live on
//     the internal listener (GATEWAY_METRICS_PORT), reached only inside the
//     compose network by the Prometheus scrape.
//
//  13. P2/M18 — no client code may persist the portal access token: it is a
//     `cliente` bearer that belongs in memory (the refresh token is an HttpOnly
//     cookie). `portal_access_token` in localStorage is the regression this rule
//     blocks.
//
//  12. P2/M12 — the 2FA preference flag must not come back without the feature:
//     `two_factor_enabled` was a preference with no effect (no enroll, no
//     step-up) while the UI claimed "2FA ativado". If MFA is really implemented,
//     update this rule together with the feature.
//
//  11. P2/M17 — a handler file must not answer 500 with the raw error text
//     (`InternalServerError(c, x.Error())`): SQL/driver detail belongs in the
//     log, not in the response. Use `response.InternalError(c, err)`.
//
//  10. P2/M19+M20 (client side) — every entry of the admin route registry must
//     DECLARE the permissions that open it (or be an explicitly universal route
//     such as the user's own profile page), the client catch-all must live
//     outside the authenticated Layout, and `/inteligencia` must be wrapped in
//     `RequirePermission` — a UI route with no declaration is a screen the
//     server has to deny request by request. Declaring only `module:*` does not
//     count: that permission governs client NAVIGATION and no backend route
//     ever reads the `module:` prefix, so the screen stays gated by navigation
//     alone. The screens that still have no action permission to declare are
//     listed in moduleOnlyExceptionRoutes with their reason.
//
//  9. P2/B8 — route files must not mount a raw directory
//     (`r.Static`/`StaticFile`): toca-deal served every document, including
//     contracts and identity files, without authentication. Local files are
//     served by the authenticated endpoints through `storage.LocalUploadPath`.
//
//  4. P1/A6+A7 (client side) — every permission the frontend DECLARES (route
//     guard, nav entry, `<Can>`/`can()`) must exist in the backend catalog. A
//     frontend-only permission (e.g. `properties:create`) hides buttons that the
//     server would allow (or shows buttons that 403) — the divergence M15 found.
//
// Usage:
//
//	go run ./scripts/check-route-guards            # repo root inferred from cwd
//	go run ./scripts/check-route-guards -root=..
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

// routeFile lists the files that register HTTP routes, per service.
var routeFiles = map[string][]string{
	"gateway":        {"gateway/internal/handlers/deal.go", "gateway/internal/handlers/signature.go"},
	"toca-deal":      {"toca-deal/internal/api/v1/routes/routes.go"},
	"toca-signature": {"toca-signature/internal/api/v1/routes/routes.go"},
	"toca-comm":      {"toca-comm/internal/api/v1/routes/routes.go"},
	"toca-auth":      {"toca-auth/internal/api/v1/routes/routes.go"},
	"toca-crm":       {"toca-crm/internal/api/v1/routes/routes.go"},
	"toca-core":      {"toca-core/internal/handlers/router.go"},
	"toca-portal":    {"toca-portal/internal/handlers/router.go"},
	"toca-ops":       {"toca-ops/internal/handlers/router.go"},
	"toca-financial": {"toca-financial/internal/handlers/router.go"},
	"toca-calendar":  {"toca-calendar/internal/handlers/router.go"},
	"toca-ai":        {"toca-ai/internal/handlers/router.go"},
	"toca-mcp":       {},
	"toca-ocr":       {},
}

// groupsThatMustRejectCliente maps service → route-group identifiers that are
// operator-only and must therefore not be reachable with a `cliente` token.
var groupsThatMustRejectCliente = map[string][]string{
	"toca-deal":      {"/deals", "/contracts", "/documents", "/contract-templates", "/checklist-templates"},
	"toca-signature": {"/api/v1"},
	"toca-comm":      {"/channels", "/templates"},
	"toca-auth":      {"/companies", "/customers"},
}

// groupsThatMustRequireCliente maps service → group identifiers that serve the
// client portal and must keep requiring the `cliente` role. Guards against the
// fix above accidentally closing the portal.
var groupsThatMustRequireCliente = map[string][]string{
	"toca-deal":      {"/client-portal"},
	"toca-crm":       {"/client-portal"},
	"toca-financial": {"/client-portal"},
}

// clienteStrictGuard é a guarda estrita de operador: barra o papel `cliente` em
// qualquer vínculo. `RejectFunctionalRoleAllowUnassigned("cliente")` NÃO conta
// como guarda — ele deixa passar quem está sem vínculo, então é exceção
// explícita, não regra.
const clienteStrictGuard = `RejectFunctionalRole("cliente"`

// clienteGuardExceptions lista os grupos que declaram um caminho de
// `groupsThatMustRejectCliente` SEM a guarda estrita, de propósito — a chave é o
// nome da VARIÁVEL do grupo (o identificador antes do `:=` em
// `onboarding := v1.Group("/companies")`) e o valor é o motivo.
//
// Achado P0/A11: o check olhava só a primeira declaração do caminho, então um
// grupo IRMÃO com o mesmo prefixo escapava em silêncio (`onboarding` em
// toca-auth, `channels` em toca-comm). Agora toda declaração daquele caminho
// tem que aparecer: ou leva a guarda estrita, ou está aqui com o motivo. A
// chave não inclui o serviço, então um grupo novo com o mesmo nome em outro
// serviço precisa ser revisado (o motivo do valor diz de qual se trata).
var clienteGuardExceptions = map[string]string{
	"onboarding": "bootstrap de onboarding (toca-auth): conta nova não tem vínculo, então functional_role chega vazio e o guard estrito (fail-closed) travaria a criação da primeira empresa. Usa RejectFunctionalRoleAllowUnassigned(\"cliente\"), que continua barrando `cliente`.",
	"channels":   "superfície do CANAL (toca-comm): quem autoriza é a PARTICIPAÇÃO no canal (A9), validada fail-closed no serviço, e o `cliente` do portal é participante do canal do próprio negócio — o chat do portal usa exatamente estas rotas; rejeitar o papel no grupo inteiro apagaria o chat sem ganho de segurança. O grupo irmão `tenantChannels` (criar/listar canais, participantes, status) é que leva a guarda estrita.",
}

var (
	routeRe = regexp.MustCompile(`\.(GET|POST|PUT|PATCH|DELETE|Any|HEAD|OPTIONS)\(`)
	guardRe = regexp.MustCompile(`RequirePermission|RequireAnyPermission|RequireFunctionalRole|RequireRole|RequireInternalKey|InternalOrMiddleware|RejectFunctionalRole|RequirePermissionOrInternalKey`)

	// groupDeclRe casa a declaração de um grupo de rotas e captura a variável, o
	// grupo pai e o caminho: `companies := v1.Group("/companies")`.
	groupDeclRe = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\s*:?=\s*([A-Za-z_][A-Za-z0-9_]*)\.Group\(\s*"([^"]*)"`)

	// absoluteRouteRe casa uma rota registrada com caminho no receptor:
	// `v1.POST("/companies", ...)`.
	absoluteRouteRe = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\.(?:GET|POST|PUT|PATCH|DELETE|Any|HEAD|OPTIONS)\(\s*"([^"]*)"`)
)

type violation struct {
	File   string
	Line   int
	Rule   string
	Detail string
}

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()

	if _, err := os.Stat(filepath.Join(*root, "go.work")); err != nil {
		fmt.Fprintf(os.Stderr, "check-route-guards: %s does not look like the repo root (no go.work)\n", *root)
		os.Exit(2)
	}

	catalog, err := loadCatalog(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-route-guards: %v\n", err)
		os.Exit(2)
	}

	var violations []violation

	feViolations, err := checkFrontendPermissions(*root, catalog)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-route-guards: %v\n", err)
		os.Exit(2)
	}
	violations = append(violations, feViolations...)

	for svc, files := range routeFiles {
		for _, rel := range files {
			path := filepath.Join(*root, rel)
			src, err := os.ReadFile(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "check-route-guards: %v\n", err)
				os.Exit(2)
			}
			lines := strings.Split(string(src), "\n")

			if registersInternalRoute(lines) {
				violations = append(violations, checkInternalServiceKeys(*root, svc)...)
			}

			if svc == "gateway" {
				violations = append(violations, checkEdgeInternalPaths(rel, lines)...)
			}
			violations = append(violations, checkClienteBoundaries(svc, rel, lines)...)
			violations = append(violations, checkPermissionsExist(*root, rel, lines, catalog)...)
			violations = append(violations, checkInternalKeyGuard(rel, lines)...)
			violations = append(violations, checkSwaggerGate(rel, lines)...)
			violations = append(violations, checkNoRawStaticMount(rel, lines)...)
		}
	}

	violations = append(violations, checkFrontendRouteDeclarations(*root)...)
	violations = append(violations, checkInternalErrorLeaks(*root)...)
	violations = append(violations, checkTwoFactorFlag(*root)...)
	violations = append(violations, checkPortalTokenStorage(*root)...)
	violations = append(violations, checkGatewayMetricsSurface(*root)...)
	violations = append(violations, checkNoPublicMetricsRouter(*root)...)

	sort.Slice(violations, func(i, j int) bool {
		if violations[i].File != violations[j].File {
			return violations[i].File < violations[j].File
		}
		return violations[i].Line < violations[j].Line
	})

	for _, v := range violations {
		fmt.Printf("%s:%d: [%s] %s\n", v.File, v.Line, v.Rule, v.Detail)
	}
	if len(violations) > 0 {
		fmt.Printf("\ncheck-route-guards: %d violação(ões)\n", len(violations))
		os.Exit(1)
	}
	fmt.Println("check-route-guards: ok")
}

// checkEdgeInternalPaths (P0/C10): the gateway must not register an internal path.
func checkEdgeInternalPaths(file string, lines []string) []violation {
	var out []violation
	for i, line := range lines {
		if !routeRe.MatchString(line) {
			continue
		}
		normalized := strings.ToLower(strings.Join(strings.FieldsFunc(line, func(r rune) bool { return r == '/' || r == '"' || r == ' ' }), "/"))
		if strings.Contains("/"+normalized+"/", "/internal/") {
			out = append(out, violation{
				File: file, Line: i + 1, Rule: "C10",
				Detail: "a borda pública não pode registrar caminho /internal (tráfego interno usa *_SERVICE_URL direto no serviço)",
			})
		}
	}
	return out
}

// checkClienteBoundaries (P0/A11) verifies the role gate on both planes.
//
// No plano de operador a regra é por DECLARAÇÃO DE GRUPO, não por caminho: um
// caminho de política pode ter mais de um grupo (o operador `companies` e o
// `onboarding` em toca-auth; o `tenantChannels` e o `channels` em toca-comm), e
// olhar só a primeira declaração deixava o irmão sem guarda passar batido.
func checkClienteBoundaries(svc, file string, lines []string) []violation {
	var out []violation

	decls := parseGroupDecls(lines)
	full := groupFullPaths(decls)
	bases := policyBases(groupsThatMustRejectCliente[svc], decls, full)

	for _, group := range groupsThatMustRejectCliente[svc] {
		// Caminho sem nenhuma declaração de grupo é tolerado: ele pode ter sido
		// renomeado, e o check é conservador. O que existe, porém, tem que estar
		// coberto — inclusive o irmão que o check antigo não enxergava.
		for _, d := range decls {
			if d.Path != group {
				continue
			}
			if d.GuardLine > 0 {
				continue
			}
			if _, excepted := clienteGuardExceptions[d.Var]; excepted {
				continue
			}
			out = append(out, violation{
				File: file, Line: d.Line, Rule: "A11",
				Detail: fmt.Sprintf("grupo %q (%s) declara o caminho de operador %q sem rejeitar o papel cliente: toda declaração desse caminho precisa de auth.RejectFunctionalRole(\"cliente\") ou estar em clienteGuardExceptions com o motivo", d.Var, svc, group),
			})
		}

		out = append(out, checkAbsoluteClienteRoutes(file, svc, group, lines, decls, full, bases)...)
	}

	for _, group := range groupsThatMustRequireCliente[svc] {
		if !groupDeclared(lines, group) {
			continue
		}
		if !groupHasMiddleware(lines, group, `RequireFunctionalRole("cliente"`) {
			out = append(out, violation{
				File: file, Line: groupLine(lines, group), Rule: "A11-portal",
				Detail: fmt.Sprintf("grupo de portal %q (%s) deve continuar exigindo o papel cliente", group, svc),
			})
		}
	}

	return out
}

// groupDecl é uma declaração de grupo de rotas do arquivo.
type groupDecl struct {
	Var    string // identificador antes do `:=`
	Parent string // receptor do `.Group(` — a engine, quando o grupo é raiz
	Path   string // caminho declarado no Group(...)
	Line   int
	// GuardLine é a linha da guarda estrita: 0 quando não há. Guardar a linha
	// (e não um booleano) diz se a guarda veio ANTES da rota — o gin congela a
	// cadeia no registro, então um `.Use(` depois da rota não vale para ela.
	GuardLine int
}

// parseGroupDecls devolve as declarações de grupo na ordem em que aparecem, com
// a guarda estrita já resolvida. Ela conta na própria linha do `Group(` (como
// toca-deal e toca-comm registram) ou no `.Use(` logo abaixo (como toca-auth e
// toca-signature registram), com a mesma janela curta do check original. Linha
// comentada não registra grupo: código desligado não é violação.
func parseGroupDecls(lines []string) []groupDecl {
	var out []groupDecl
	for i, line := range lines {
		if isCommentLine(line) {
			continue
		}
		m := groupDeclRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		d := groupDecl{Var: m[1], Parent: m[2], Path: m[3], Line: i + 1}
		if strings.Contains(line, clienteStrictGuard) {
			d.GuardLine = i + 1
		} else {
			limit := min(i+6, len(lines))
			for j := i + 1; j < limit; j++ {
				if isCommentLine(lines[j]) {
					continue
				}
				if routeRe.MatchString(lines[j]) || strings.Contains(lines[j], "Group(") {
					break // fim do bloco de declaração do grupo
				}
				if strings.Contains(lines[j], ".Use(") && strings.Contains(lines[j], clienteStrictGuard) {
					d.GuardLine = j + 1
					break
				}
			}
		}
		out = append(out, d)
	}
	return out
}

// isCommentLine informa se a linha é comentário (`// ...`).
func isCommentLine(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "//")
}

// groupFullPaths devolve o caminho COMPLETO de cada grupo declarado no arquivo
// (`v1 := r.Group("/api/v1")` + `auth := v1.Group("/auth")` = `/api/v1/auth`). O
// pai é declarado antes (Go exige), então a ordem do arquivo basta para
// acumular; um receptor que não é grupo conhecido (a engine `r`, ou um grupo
// criado em outro arquivo) fica com o caminho que o próprio literal declara.
func groupFullPaths(decls []groupDecl) map[string]string {
	full := map[string]string{}
	for _, d := range decls {
		full[d.Var] = full[d.Parent] + d.Path
	}
	return full
}

// policyBases devolve as BASES em que os caminhos de política são declarados
// neste serviço — o caminho completo do pai na declaração
// (`companies := v1.Group("/companies")` declara `/companies` sobre a base
// `/api/v1`). É a base que transforma uma rota de caminho absoluto em rota
// daquele prefixo: `v1.POST("/companies", ...)` cai em `/api/v1/companies`,
// enquanto `internal.GET("/contracts/:id")` tem base `/api/v1/deal/internal` —
// outra árvore, nada a ver com `/contracts`. Sem grupo de política declarado não
// há base e o check fica quieto, a mesma tolerância do caminho renomeado.
func policyBases(prefixes []string, decls []groupDecl, full map[string]string) map[string]bool {
	wanted := map[string]bool{}
	for _, p := range prefixes {
		wanted[p] = true
	}
	bases := map[string]bool{}
	for _, d := range decls {
		if wanted[d.Path] {
			bases[full[d.Parent]] = true
		}
	}
	return bases
}

// checkAbsoluteClienteRoutes cobre a rota registrada com caminho absoluto sob o
// prefixo de política (`v1.POST("/companies", ...)`): ela não pertence a nenhum
// grupo daquele caminho, então escapava das duas checagens. Só olha rotas cujo
// receptor está na base do prefixo (a engine, quando o grupo foi declarado nela,
// ou o grupo que serve de base) e cujo literal já começa pelo prefixo — num
// subgrupo o literal é relativo (`companies.POST("/:id/users")`) e não é
// caminho absoluto.
func checkAbsoluteClienteRoutes(file, svc, prefix string, lines []string, decls []groupDecl, full map[string]string, bases map[string]bool) []violation {
	var out []violation
	byVar := map[string]groupDecl{}
	for _, d := range decls {
		byVar[d.Var] = d
	}
	for i, line := range lines {
		if isCommentLine(line) {
			continue
		}
		m := absoluteRouteRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		receiver, literal := m[1], m[2]
		if literal != prefix && !strings.HasPrefix(literal, prefix+"/") {
			continue
		}
		if !bases[full[receiver]] {
			continue // receptor fora da base do prefixo (outra árvore da API)
		}
		if d, known := byVar[receiver]; known && d.GuardLine > 0 && d.GuardLine < i+1 {
			continue // a guarda estrita do próprio receptor já vale para esta rota
		}
		if strings.Contains(line, clienteStrictGuard) {
			continue
		}
		out = append(out, violation{
			File: file, Line: i + 1, Rule: "A11",
			Detail: fmt.Sprintf("rota %q registrada com caminho absoluto sob o prefixo de operador %q (%s) sem rejeitar o papel cliente: ponha auth.RejectFunctionalRole(\"cliente\") na própria rota ou registre-a em um grupo, que é a forma que as exceções documentadas usam", literal, prefix, svc),
		})
	}
	return out
}

// groupDeclared reports whether the file mentions the group path at all.
func groupDeclared(lines []string, group string) bool {
	for _, line := range lines {
		if strings.Contains(line, `"`+group+`"`) {
			return true
		}
	}
	return false
}

func groupLine(lines []string, group string) int {
	for i, line := range lines {
		if strings.Contains(line, `"`+group+`"`) {
			return i + 1
		}
	}
	return 1
}

// groupHasMiddleware reports whether the group declaration or a `.Use(` within
// the following lines carries the wanted middleware. It looks ahead a bounded
// number of lines to cover `x := v1.Group(...)` followed by `x.Use(...)`.
func groupHasMiddleware(lines []string, group, middleware string) bool {
	for i, line := range lines {
		if !strings.Contains(line, `"`+group+`"`) {
			continue
		}
		if strings.Contains(line, middleware) {
			return true
		}
		limit := i + 6
		if limit > len(lines) {
			limit = len(lines)
		}
		for j := i + 1; j < limit; j++ {
			switch {
			case strings.Contains(lines[j], ".Use("):
				if strings.Contains(lines[j], middleware) {
					return true
				}
				// `.Use(` multilinha (um middleware por linha, parêntese fechando
				// depois): segue dentro da MESMA chamada até fechar. Sem isso a
				// guarda obrigatória era reportada como ausente só por estar numa
				// linha diferente do `.Use(`.
				depth := strings.Count(lines[j], "(") - strings.Count(lines[j], ")")
				for k := j + 1; k < len(lines) && depth > 0; k++ {
					if strings.Contains(lines[k], middleware) {
						return true
					}
					depth += strings.Count(lines[k], "(") - strings.Count(lines[k], ")")
				}
			case routeRe.MatchString(lines[j]), strings.Contains(lines[j], "Group("):
				// fim do bloco de declaração do grupo
				j = limit
			}
		}
	}
	return false
}

// catalogPath é a fonte única do catálogo de permissões (P1/A6).
const catalogPath = "pkg/auth/permission_catalog.go"

var catalogIDRe = regexp.MustCompile(`\{ID: "([^"]+)"`)

// loadCatalog lê os IDs do catálogo do backend.
func loadCatalog(root string) (map[string]bool, error) {
	src, err := os.ReadFile(filepath.Join(root, catalogPath))
	if err != nil {
		return nil, fmt.Errorf("ler catálogo: %w", err)
	}
	out := map[string]bool{}
	for _, m := range catalogIDRe.FindAllStringSubmatch(string(src), -1) {
		out[m[1]] = true
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("catálogo %s não devolveu nenhuma permissão", catalogPath)
	}
	return out, nil
}

// permissionArgRe captura os literais passados às guardas de permissão.
var permissionArgRe = regexp.MustCompile(`Require(?:Any)?Permission(?:OrInternalKey)?\([^)]*?\)`)

// permissionLiteralRe captura strings com forma de permissão ("modulo:acao").
var permissionLiteralRe = regexp.MustCompile(`"([a-z_]+:[a-z_]+)"`)

// checkPermissionsExist (P1/A7 + M3) falha quando uma rota exige uma permissão
// que não existe no catálogo. O check é textual e conservador: só olha os
// literais dentro de chamadas de guarda, e ignora `""` (guarda vazia = só
// autenticação, decisão consciente em algumas rotas).
func checkPermissionsExist(root, file string, lines []string, catalog map[string]bool) []violation {
	var out []violation
	for i, line := range lines {
		if !strings.Contains(line, "Permission(") {
			continue
		}
		for _, call := range permissionArgRe.FindAllString(line, -1) {
			for _, m := range permissionLiteralRe.FindAllStringSubmatch(call, -1) {
				id := m[1]
				if id == "" {
					continue
				}
				if !catalog[id] {
					out = append(out, violation{
						File: file, Line: i + 1, Rule: "A7",
						Detail: fmt.Sprintf("a rota exige a permissão %q, que não existe em %s — a rota fica inalcançável para todo papel funcional (só perfil root passa); use uma permissão do catálogo ou crie-a", id, catalogPath),
					})
				}
			}
		}
	}
	return out
}

// frontendPermissionContextRe isola os trechos do frontend onde uma permissão é
// DECLARADA (e não qualquer string parecida: `sm:hidden`, `blob:failed` e
// chaves de coleção aparecem em todo lugar).
var frontendPermissionContextRe = regexp.MustCompile(`(?:permissions?\s*[:=]\s*(?:\[[^\]]*\]|"[^"]*")|anyOf\s*[:=]\s*\[[^\]]*\]|can\(\s*"[^"]*")`)

// checkFrontendPermissions (P1/A6+A7) garante que o frontend não declare uma
// permissão que o catálogo do backend não conhece.
func checkFrontendPermissions(root string, catalog map[string]bool) ([]violation, error) {
	srcDir := filepath.Join(root, "toka", "src")
	var out []violation
	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		name := info.Name()
		if !strings.HasSuffix(name, ".ts") && !strings.HasSuffix(name, ".tsx") {
			return nil
		}
		if name == "catalog.generated.ts" || strings.Contains(name, ".test.") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(content)
		rel, _ := filepath.Rel(root, path)
		for _, ctx := range frontendPermissionContextRe.FindAllString(text, -1) {
			for _, m := range permissionLiteralRe.FindAllStringSubmatch(ctx, -1) {
				id := m[1]
				if strings.HasPrefix(id, "module:") {
					// `module:*` governa navegação no cliente e é validado pelo
					// próprio gerador do catálogo.
					continue
				}
				if catalog[id] {
					continue
				}
				out = append(out, violation{
					File: rel, Line: 1, Rule: "A6-fe",
					Detail: fmt.Sprintf("o frontend declara a permissão %q, que não existe em %s — guarda de UI que não corresponde a nenhuma autorização do servidor", id, catalogPath),
				})
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// internalRouteRe casa a declaração de um grupo de rota interna
// (`/comm/internal`, `/ops/internal`, `/api/v1/signature/internal`, ...).
var internalRouteRe = regexp.MustCompile(`"/[a-z0-9/]*internal"`)

// registersInternalRoute informa se o arquivo registra rota interna.
func registersInternalRoute(lines []string) bool {
	for _, line := range lines {
		if internalRouteRe.MatchString(line) && strings.Contains(line, "Group(") {
			return true
		}
		if strings.Contains(line, "/internal") && routeRe.MatchString(line) {
			return true
		}
	}
	return false
}

// composeStacks são os arquivos onde a configuração de credencial interna precisa
// existir. docker-compose.beta.yml é override do local, então não entra.
var composeStacks = []string{"docker-compose.yml", "docker-compose.test.yml", "docker-compose.prod.yml"}

// checkInternalServiceKeys (P1/A2) exige `INTERNAL_SERVICE_KEYS` no bloco do
// serviço em cada stack.
func checkInternalServiceKeys(root, svc string) []violation {
	var out []violation
	for _, stack := range composeStacks {
		src, err := os.ReadFile(filepath.Join(root, stack))
		if err != nil {
			continue // stack ausente no checkout: o check é conservador
		}
		block := composeServiceBlock(string(src), svc)
		if block == "" {
			continue // serviço não existe neste stack
		}
		if !strings.Contains(block, "INTERNAL_SERVICE_KEYS") {
			out = append(out, violation{
				File: stack, Line: 1, Rule: "A2",
				Detail: fmt.Sprintf("o serviço %q registra rota interna e não declara INTERNAL_SERVICE_KEYS neste stack — sem a variável ele fica no modelo antigo (chave compartilhada, sem allowlist de tenant)", svc),
			})
		}
	}
	return out
}

// composeServiceBlock devolve o texto do bloco do serviço: da linha `  <svc>:`
// até a próxima linha de SERVIÇO (exatamente dois espaços + nome + `:`), sem
// cortar nas chaves aninhadas (4+ espaços).
func composeServiceBlock(src, svc string) string {
	lines := strings.Split(src, "\n")
	serviceLine := regexp.MustCompile(`^  [a-z0-9-]+:\s*$`)
	out := make([]string, 0, 32)
	inBlock := false
	for _, line := range lines {
		if !inBlock {
			if line == "  "+svc+":" {
				inBlock = true
				out = append(out, line)
			}
			continue
		}
		if serviceLine.MatchString(line) {
			break
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// checkInternalKeyGuard (P2/M8) proíbe a comparação caseira de X-Internal-Key em
// arquivo de rotas. A guarda única é auth.RequireInternalKey() — comparação em
// tempo constante, rate limit por IP, auditoria e chaves nomeadas.
func checkInternalKeyGuard(file string, lines []string) []violation {
	var out []violation
	for i, line := range lines {
		if !strings.Contains(line, `GetHeader("X-Internal-Key")`) {
			continue
		}
		out = append(out, violation{
			File: file, Line: i + 1, Rule: "M8",
			Detail: "rota interna comparando X-Internal-Key à mão — use auth.RequireInternalKey() (comparação em tempo constante, rate limit, auditoria e chaves nomeadas)",
		})
	}
	return out
}

// swaggerHandlerMarker identifica o handler do Swagger UI; a única forma aceita
// de montá-lo é via apidocs.Register (gate de release).
const swaggerHandlerMarker = "ginSwagger.WrapHandler"

// swaggerGateMarker é a chamada autorizada.
const swaggerGateMarker = "apidocs.Register("

// checkSwaggerGate (P2/M16) exige que todo registro de Swagger passe pelo gate
// de ambiente. O check é textual e conservador: só olha linhas que constroem o
// handler.
func checkSwaggerGate(file string, lines []string) []violation {
	var out []violation
	for i, line := range lines {
		if !strings.Contains(line, swaggerHandlerMarker) {
			continue
		}
		if strings.Contains(line, swaggerGateMarker) {
			continue
		}
		out = append(out, violation{
			File: file, Line: i + 1, Rule: "M16",
			Detail: "Swagger registrado fora de apidocs.Register — o gate de release (achado M16) não é aplicado e o mapa da API fica público em produção",
		})
	}
	return out
}

// rawStaticRe casa mounts de diretório: `r.Static(...)` e `r.StaticFile(...)`.
// `StaticFS` continua permitido porque o toca-comm usa um FS que filtra o bucket
// de comprovantes financeiros (protectedUploadFS).
var rawStaticRe = regexp.MustCompile(`\.(Static|StaticFile)\(`)

// checkNoRawStaticMount (P2/B8) proíbe servir um diretório inteiro sem
// autenticação a partir de um arquivo de rotas.
func checkNoRawStaticMount(file string, lines []string) []violation {
	var out []violation
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		if !rawStaticRe.MatchString(line) {
			continue
		}
		out = append(out, violation{
			File: file, Line: i + 1, Rule: "B8",
			Detail: "mount de diretório sem autenticação — sirva o arquivo por endpoint autenticado (storage.LocalUploadPath) ou por URL assinada",
		})
	}
	return out
}

// gatewayPublicEngineFile é onde a engine pública do gateway é montada.
const gatewayPublicEngineFile = "gateway/cmd/gateway/router.go"

// metricsPathMarker identifica o registro de /metrics.
const metricsPathMarker = `"/metrics"`

// checkGatewayMetricsSurface (P2/B1) garante que a engine PÚBLICA do gateway não
// registre /metrics. O scrape é do listener interno (buildMetricsEngine), que
// não é publicado pela borda.
func checkGatewayMetricsSurface(root string) []violation {
	src, err := os.ReadFile(filepath.Join(root, gatewayPublicEngineFile))
	if err != nil {
		return nil // arquivo renomeado/movido: o check é conservador
	}
	body := functionBody(string(src), "buildPublicEngine")
	if body == "" {
		return nil
	}
	var out []violation
	for i, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		if strings.Contains(line, metricsPathMarker) || strings.Contains(line, "RegisterMetrics(") {
			out = append(out, violation{
				File: gatewayPublicEngineFile, Line: i + 1, Rule: "B1",
				Detail: "a engine pública do gateway não pode montar /metrics — use buildMetricsEngine (listener interno, gateway:" + defaultMetricsPortHint + ")",
			})
		}
	}
	return out
}

// defaultMetricsPortHint documenta a porta do listener interno no texto do check.
const defaultMetricsPortHint = "9091"

// metricsRouterRe casa um router do Traefik publicado para /metrics.
var metricsRouterRe = regexp.MustCompile(regexp.QuoteMeta("PathPrefix(`/metrics`)"))

// releaseStacks são os stacks de release onde a borda não pode publicar /metrics.
var releaseStacks = []string{"docker-compose.prod.yml", "docker-compose.beta.yml"}

// checkNoPublicMetricsRouter (P2/B1) exige que nenhum stack de release publique
// um router do Traefik para /metrics.
func checkNoPublicMetricsRouter(root string) []violation {
	var out []violation
	for _, stack := range releaseStacks {
		src, err := os.ReadFile(filepath.Join(root, stack))
		if err != nil {
			continue
		}
		for i, line := range strings.Split(string(src), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "#") {
				continue
			}
			if metricsRouterRe.MatchString(line) && !strings.Contains(line, "!PathPrefix") {
				out = append(out, violation{
					File: stack, Line: i + 1, Rule: "B1",
					Detail: "a borda não pode publicar /metrics em release — o scrape é interno (gateway:" + defaultMetricsPortHint + ", sem router no Traefik)",
				})
			}
		}
	}
	return out
}

// functionBody devolve o corpo da função `func <name>(` do arquivo, da linha da
// assinatura até a primeira linha `}` na coluna 0. Devolve "" quando a função
// não existe (o check não bloqueia um refactor que não entende).
func functionBody(src, name string) string {
	lines := strings.Split(src, "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "func "+name+"(") {
			start = i
			break
		}
	}
	if start < 0 {
		return ""
	}
	for j := start; j < len(lines); j++ {
		if strings.TrimSpace(lines[j]) == "}" && !strings.HasPrefix(lines[j], "\t") {
			return strings.Join(lines[start:j+1], "\n")
		}
	}
	return ""
}

// ───────────── fronteira do cliente (P2/M19 + M20) ─────────────

const adminRegistryPath = "toka/src/apps/admin/adminRouteRegistry.tsx"

const appRoutesPath = "toka/src/app/routes.tsx"

// universalClientRoutes são as rotas do shell administrativo que NÃO declaram
// permissão de propósito — cada uma com o motivo que justifica a exceção.
var universalClientRoutes = map[string]string{
	"(index)":       "dashboard é a página de entrada do app autenticado: agrega dados de módulos distintos e cada API por trás dela é guardada pelo próprio módulo",
	"configuracoes": "página de perfil do usuário: a aba \"Meu Perfil\" é de todos e as demais abas são gateadas individualmente (e no servidor)",
	"notificacoes":  "preferências pessoais de notificação",
	"access-denied": "destino da própria negação de acesso",
	"ajuda":         "Central de Ajuda: conteúdo estático empacotado no bundle (sem API de dados além do boot de sessão), aberta a qualquer usuário autenticado",
}

// moduleOnlyExceptionRoutes são as rotas do cliente cuja ÚNICA declaração é uma
// permissão de NAVEGAÇÃO (`module:*`) — cada uma com o motivo.
//
// `module:*` governa o menu do cliente: nenhuma rota de backend lê esse prefixo
// (o catálogo e este check são os únicos lugares que o conhecem), então declarar
// só ele deixa a tela guardada apenas pela navegação. As rotas daqui são as que
// ainda não têm permissão de AÇÃO para declarar; quando tiverem, a entrada sai.
var moduleOnlyExceptionRoutes = map[string]string{
	"assinatura": "tela de plano e consumo (`Assinatura & Consumo`): PLANS, INVOICES e as ações são constantes/estado local do componente e o consumo sai dos contexts já carregados (leads, imóveis, usuários). Não existe endpoint de assinatura/fatura de PLANO no backend — as `/invoices` do toca-financial são fatura de VENDA, outra tela, sob `finance:read`. Sem rota de servidor não há permissão de ação para declarar, e declarar `finance:read` esconderia a tela de `operacoes` sem guarda correspondente. Quando a API de assinatura existir, esta entrada sai e a rota declara a permissão dela.",
}

// adminRouteEntryRe casa uma entrada da registry: `{ path: "x", Component: Y ... }`.
var adminRouteEntryRe = regexp.MustCompile(`\{\s*(index:\s*true|path:\s*"([^"]+)")`)

// adminRoutePermissionsRe captura o array declarado em `permissions: [...]`.
var adminRoutePermissionsRe = regexp.MustCompile(`permissions:\s*\[([^\]]*)\]`)

// adminRoutePermissions devolve as permissões declaradas na linha. O segundo
// valor é false quando a linha não tem um array que o check consiga ler (uma
// declaração quebrada em várias linhas, por exemplo): aí o check fica quieto,
// como manda a regra de não bloquear o que ele não entende.
func adminRoutePermissions(line string) ([]string, bool) {
	m := adminRoutePermissionsRe.FindStringSubmatch(line)
	if m == nil {
		return nil, false
	}
	var out []string
	for _, lit := range permissionLiteralRe.FindAllStringSubmatch(m[1], -1) {
		out = append(out, lit[1])
	}
	return out, true
}

// hasActionPermission informa se alguma permissão declarada é de AÇÃO — isto é,
// qualquer coisa que não seja a navegação `module:*`.
func hasActionPermission(perms []string) bool {
	for _, p := range perms {
		if !strings.HasPrefix(p, "module:") {
			return true
		}
	}
	return false
}

// checkFrontendRouteDeclarations (P2/M19+M20) exige declaração de permissão nas
// rotas do cliente, catch-all fora do Layout e guarda em `/inteligencia`.
func checkFrontendRouteDeclarations(root string) []violation {
	var out []violation

	registry := filepath.Join(root, adminRegistryPath)
	if src, err := os.ReadFile(registry); err == nil {
		for i, line := range strings.Split(string(src), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			m := adminRouteEntryRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			path := m[2]
			key := path
			if key == "" {
				key = "(index)"
			}
			if _, universal := universalClientRoutes[key]; universal {
				continue
			}
			if !strings.Contains(line, "permissions:") {
				label := path
				if label == "" {
					label = "(index)"
				}
				out = append(out, violation{
					File: adminRegistryPath, Line: i + 1, Rule: "M19",
					Detail: fmt.Sprintf("rota do cliente %q sem `permissions`: a tela abre para qualquer autenticado e o servidor tem que negar requisição por requisição; declare a permissão ou inclua o caminho em universalClientRoutes com o motivo", label),
				})
				continue
			}
			perms, parsed := adminRoutePermissions(line)
			if !parsed {
				continue // declaração que o check não lê: não bloqueia o que não entende
			}
			if hasActionPermission(perms) {
				continue
			}
			if _, excepted := moduleOnlyExceptionRoutes[key]; excepted {
				continue
			}
			out = append(out, violation{
				File: adminRegistryPath, Line: i + 1, Rule: "M19",
				Detail: fmt.Sprintf("rota do cliente %q declara só permissões de navegação (`module:*`): nenhuma rota de backend lê esse prefixo, então a tela fica guardada apenas pela navegação. Declare a permissão da AÇÃO que a tela executa (a mesma que o endpoint por trás dela exige) ou inclua o caminho em moduleOnlyExceptionRoutes com o motivo", key),
			})
		}

		// Catch-all não pode ficar dentro do Layout autenticado.
		if body := functionBody(string(src), "getAdminLayoutRouteObject"); body != "" && strings.Contains(body, `path: "*"`) {
			out = append(out, violation{
				File: adminRegistryPath, Line: 1, Rule: "M19",
				Detail: "catch-all dentro do Layout: URL desconhecida renderiza a casca do app inteira; mova para fora (NotFoundRoute no nível do ProtectedRoute)",
			})
		}
	}

	// /inteligencia precisa da guarda de rota declarada (`ai:use`).
	if src, err := os.ReadFile(filepath.Join(root, appRoutesPath)); err == nil {
		lines := strings.Split(string(src), "\n")
		for i, line := range lines {
			if !strings.Contains(line, `path: "inteligencia"`) {
				continue
			}
			window := strings.Join(lines[i:min(i+6, len(lines))], "\n")
			// A guarda de rota no cliente é um componente JSX (`<RequirePermission ...>`),
			// não uma chamada de função.
			if !strings.Contains(window, "<RequirePermission") {
				out = append(out, violation{
					File: appRoutesPath, Line: i + 1, Rule: "M19",
					Detail: "/inteligencia sem RequirePermission(ai:use) na rota — a página só avisa depois de abrir",
				})
			}
			break
		}
	}

	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// internalErrorLeakRe casa 500 com o texto do erro na resposta.
var internalErrorLeakRe = regexp.MustCompile(`InternalServerError\(c,\s*[^)]*\.Error\(\)`)

// checkInternalErrorLeaks (P2/M17) percorre os handlers de todos os módulos e
// recusa devolver `err.Error()` num 500 — é assim que nome de tabela, constraint
// e mensagem do driver vazam para o cliente.
func checkInternalErrorLeaks(root string) []violation {
	var out []violation
	modules := []string{"gateway", "toca-auth", "toca-core", "toca-crm", "toca-deal", "toca-comm", "toca-financial", "toca-signature", "toca-ops", "toca-calendar", "toca-portal", "toca-ai", "toca-mcp"}
	for _, module := range modules {
		base := filepath.Join(root, module)
		_ = filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
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
			rel, _ := filepath.Rel(root, path)
			for i, line := range strings.Split(string(src), "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "//") {
					continue
				}
				if internalErrorLeakRe.MatchString(line) {
					out = append(out, violation{
						File: rel, Line: i + 1, Rule: "M17",
						Detail: "500 devolvendo o texto do erro ao cliente (vaza detalhe de SQL/driver); use response.InternalError(c, err) — o detalhe vai para o log",
					})
				}
			}
			return nil
		})
	}
	return out
}

// checkTwoFactorFlag (P2/M12) recusa o retorno da preferência de 2FA sem
// implementação. O escopo é o backend de identidade (toca-auth) e o serviço de
// preferências do cliente.
func checkTwoFactorFlag(root string) []violation {
	var out []violation
	// `toca-auth/internal/service/preferences_service.go` fica FORA de propósito:
	// ali a chave legada é reconhecida apenas para ser descartada com log
	// ("nunca grave a afirmação de 2FA"), e isso é o comportamento correto.
	targets := []string{
		"toca-auth/internal/domain/preferences/preferences.go",
		"toka/src/shared/services/settings.service.ts",
		"toka/src/shared/pages/Settings/GeneralTab.tsx",
		"toka/src/shared/pages/Settings/_constants.ts",
	}
	for _, rel := range targets {
		src, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			continue
		}
		if !strings.Contains(string(src), "two_factor_enabled") && !strings.Contains(string(src), "twoFactorEnabled") {
			continue
		}
		out = append(out, violation{
			File: rel, Line: 1, Rule: "M12",
			Detail: "flag de 2FA de volta no código sem implementação (era preferência sem efeito e a UI afirmava '2FA ativado'); se o MFA foi implementado de verdade, ajuste a regra M12 junto com a feature",
		})
	}
	return out
}

// portalTokenStorageRe casa persistência do access token do portal no browser.
var portalTokenStorageRe = regexp.MustCompile(`portal_access_token`)

// checkPortalTokenStorage (P2/M18) recusa qualquer referência à chave do token
// do portal no cliente — o token vive em memória e o refresh é cookie HttpOnly.
func checkPortalTokenStorage(root string) []violation {
	var out []violation
	base := filepath.Join(root, "toka", "src")
	_ = filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		name := info.Name()
		if (!strings.HasSuffix(name, ".ts") && !strings.HasSuffix(name, ".tsx")) || strings.HasSuffix(name, ".test.ts") || strings.HasSuffix(name, ".test.tsx") {
			return nil
		}
		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		for i, line := range strings.Split(string(src), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			if portalTokenStorageRe.MatchString(line) {
				out = append(out, violation{
					File: rel, Line: i + 1, Rule: "M18",
					Detail: "access token do portal persistido no browser: mantenha em memória (o refresh já é cookie HttpOnly) — bearer de `cliente` em localStorage é colhível por XSS/extensão",
				})
			}
		}
		return nil
	})
	return out
}
