// check-core-service-url garante que todo serviço que CHECA PERMISSÃO receba o
// endereço do toca-core (dono da matriz) no stack.
//
// Incidente de 2026-09-21 (agenda): o corretor abria a visita e recebia
// "permissão negada" ao salvar. O `toca-calendar` protege as rotas de
// `/appointments` com `RequirePermission("deals:read"/"deals:write")`, e o
// provider consulta `GET <core>/api/v1/me/permissions` — mas o bloco do serviço
// no `docker-compose.yml` NUNCA teve `CORE_SERVICE_URL`. Sem a variável o cliente
// falha antes de sair do processo, o middleware nega (e, antes da correção do
// mesmo dia, negava com **403 genérico**, escondendo que a causa era
// configuração). O mesmo buraco existia para `toca-ai`, `toca-auth`,
// `toca-comm` e `toca-ops` — e passou despercebido porque todo teste manual
// usava perfil root (owner/admin), que faz bypass do provider.
//
// Regra: se o código Go do serviço constrói um `CoreClient` / provider de
// permissões, o bloco do MESMO serviço precisa declarar `CORE_SERVICE_URL` em
// cada compose que roda sozinho (local e produção). `docker-compose.test.yml` /
// `docker-compose.beta.yml` / `docker-compose.hostinger-build.yml` são overlays:
// sempre entram junto do base ou do prod (ver `scripts/stack.sh`), então herdam
// a variável e não são cobrados aqui.
//
// Usage:
//
//	go run ./scripts/check-core-service-url -root ../..
//
// Exit codes: 0 ok, 1 violação, 2 erro de uso/IO.
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

// coreClientCallRe reconhece a construção do cliente/provider de permissões.
// Cobre `clients.NewCoreClient()` (usado direto ou dentro de
// `auth.NewCorePermissionProvider`) e `auth.NewCorePermissionProvider`.
var coreClientCallRe = regexp.MustCompile(`NewCoreClient\s*\(|NewCorePermissionProvider\s*\(`)

// composeServiceRe delimita os blocos de serviço de um compose (dois espaços).
var composeServiceRe = regexp.MustCompile(`(?m)^  ([a-z0-9][a-z0-9-]*):\s*$`)

// composeEnvRe lê as variáveis declaradas em um bloco.
var composeEnvRe = regexp.MustCompile(`(?m)^\s+-?\s*([A-Z0-9_]+)=`)

// anchorRe encontra âncoras YAML reaproveitadas (`<<: *nome`).
var anchorRe = regexp.MustCompile(`(?m)^\s*-\s*<<:\s*\*([A-Za-z0-9_-]+)`)

// composes verificados: os que rodam sem depender de outro arquivo.
var standaloneComposes = []string{"docker-compose.yml", "docker-compose.prod.yml"}

// modules são os serviços cujo código pode exigir a variável.
var modules = []string{
	"gateway", "toca-auth", "toca-core", "toca-crm", "toca-deal",
	"toca-comm", "toca-financial", "toca-signature", "toca-ops",
	"toca-calendar", "toca-portal", "toca-ai", "toca-mcp",
}

func main() {
	root := flag.String("root", ".", "raiz do repositório")
	flag.Parse()

	needing := map[string]bool{}
	for _, module := range modules {
		uses, err := moduleBuildsCoreClient(filepath.Join(*root, module))
		if err != nil {
			fmt.Fprintf(os.Stderr, "check-core-service-url: %v\n", err)
			os.Exit(2)
		}
		if uses {
			needing[module] = true
		}
	}

	var violations []string
	checked := 0
	names := make([]string, 0, len(needing))
	for name := range needing {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, compose := range standaloneComposes {
		path := filepath.Join(*root, compose)
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			fmt.Fprintf(os.Stderr, "check-core-service-url: %v\n", err)
			os.Exit(2)
		}
		blocks := splitServices(string(raw))
		anchors := anchorEnvs(string(raw))
		checked++

		for _, name := range names {
			block, ok := blocks[name]
			if !ok {
				// Nem todo compose roda todo serviço.
				continue
			}
			if hasCoreServiceURL(block, anchors) {
				continue
			}
			violations = append(violations, fmt.Sprintf(
				"%s: serviço %q constrói o cliente do core (checagem de permissão) mas não recebe CORE_SERVICE_URL — toda rota com RequirePermission nega para quem não é owner/admin",
				compose, name))
		}
	}

	if len(violations) == 0 {
		fmt.Printf("check-core-service-url: ok (%d serviços checam permissão no core, presentes em %d compose(s))\n", len(needing), checked)
		return
	}
	for _, v := range violations {
		fmt.Println(v)
	}
	fmt.Printf("\ncheck-core-service-url: %d violação(ões)\n", len(violations))
	os.Exit(1)
}

func moduleBuildsCoreClient(dir string) (bool, error) {
	found := false
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || found {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if base == "vendor" || base == "node_modules" || base == "docs" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if coreClientCallRe.Match(content) {
			found = true
		}
		return nil
	})
	return found, err
}

// splitServices devolve, por nome, as variáveis declaradas no bloco do serviço.
func splitServices(raw string) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	locs := composeServiceRe.FindAllStringSubmatchIndex(raw, -1)
	for i, loc := range locs {
		name := raw[loc[2]:loc[3]]
		end := len(raw)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		block := raw[loc[1]:end]
		env := map[string]bool{}
		for _, m := range composeEnvRe.FindAllStringSubmatch(block, -1) {
			env[m[1]] = true
		}
		out[name] = env
	}
	return out
}

// anchorEnvs coleta as variáveis declaradas nos blocos ancorados (`x-*`).
func anchorEnvs(raw string) []map[string]bool {
	var out []map[string]bool
	anchorDefRe := regexp.MustCompile(`(?m)^x-[a-z0-9-]+:.*$`)
	locs := anchorDefRe.FindAllStringIndex(raw, -1)
	for i, loc := range locs {
		end := len(raw)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		block := raw[loc[0]:end]
		env := map[string]bool{}
		for _, m := range composeEnvRe.FindAllStringSubmatch(block, -1) {
			env[m[1]] = true
		}
		if len(env) > 0 {
			out = append(out, env)
		}
	}
	return out
}

// hasCoreServiceURL aceita a variável declarada no bloco ou herdada por âncora.
func hasCoreServiceURL(env map[string]bool, anchors []map[string]bool) bool {
	if env["CORE_SERVICE_URL"] {
		return true
	}
	for _, anchor := range anchors {
		if anchor["CORE_SERVICE_URL"] {
			return true
		}
	}
	return false
}
