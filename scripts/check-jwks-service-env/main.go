// check-jwks-service-env garante que todo serviço que inicializa JWKS receba a
// configuração do Supabase no stack local.
//
// Incidente de 2026-09-19 ("Servidor MCP desconectado" no painel de IA): o
// endurecimento do P0/C7 pôs as rotas `/mcp/*` atrás do `pkgAuth.Middleware`
// (verificação de JWT por JWKS), mas o `docker-compose.yml` nunca passou
// `SUPABASE_URL` ao `toca-mcp` — só o `docker-compose.prod.yml` passava. Sem a
// variável, `pkgAuth.InitJWKS("")` falha, o grupo inteiro responde 401 e a
// listagem de ferramentas do assistente fica vazia. O sintoma que chega ao
// usuário é "servidor desconectado", longe da causa de configuração.
//
// Regra: se o código do serviço chama `InitJWKS`, o bloco do mesmo serviço no
// `docker-compose.yml` precisa definir `SUPABASE_URL` ou `SUPABASE_JWKS_URL`
// (direto ou por âncora YAML herdada no mesmo arquivo).
//
// Usage:
//
//	go run ./scripts/check-jwks-service-env -root ../..
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

// jwksCallRe reconhece a inicialização do verificador de JWT no código Go.
var jwksCallRe = regexp.MustCompile(`InitJWKS\s*\(`)

// composeServiceRe delimita os blocos de serviço de um compose (dois espaços).
var composeServiceRe = regexp.MustCompile(`(?m)^  ([a-z0-9][a-z0-9-]*):\s*$`)

// composeEnvRe lê as variáveis declaradas em um bloco.
var composeEnvRe = regexp.MustCompile(`(?m)^\s+-?\s*([A-Z0-9_]+)=`)

// anchorRe encontra âncoras YAML reaproveitadas (`<<: *nome`).
var anchorRe = regexp.MustCompile(`(?m)^\s*-\s*<<:\s*\*([A-Za-z0-9_-]+)`)

func main() {
	root := flag.String("root", ".", "raiz do repositório")
	flag.Parse()

	modules := []string{
		"gateway", "toca-auth", "toca-core", "toca-crm", "toca-deal",
		"toca-comm", "toca-financial", "toca-signature", "toca-ops",
		"toca-calendar", "toca-portal", "toca-ai", "toca-mcp",
	}

	needing := map[string]string{} // serviço do compose → módulo que exige JWKS
	for _, module := range modules {
		uses, err := moduleCallsInitJWKS(filepath.Join(*root, module))
		if err != nil {
			fmt.Fprintf(os.Stderr, "check-jwks-service-env: %v\n", err)
			os.Exit(2)
		}
		if uses {
			needing[module] = module
		}
	}

	composePath := filepath.Join(*root, "docker-compose.yml")
	raw, err := os.ReadFile(composePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-jwks-service-env: %v\n", err)
		os.Exit(2)
	}
	blocks := splitServices(string(raw))
	anchors := anchorEnvs(string(raw))

	var violations []string
	names := make([]string, 0, len(needing))
	for name := range needing {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		env := blocks[name]
		if env == nil {
			violations = append(violations, fmt.Sprintf("docker-compose.yml: serviço %q não existe (o código chama InitJWKS)", name))
			continue
		}
		if hasSupabaseConfig(env, anchors) {
			continue
		}
		violations = append(violations, fmt.Sprintf("docker-compose.yml: serviço %q inicializa JWKS mas não recebe SUPABASE_URL/SUPABASE_JWKS_URL — as rotas autenticadas por JWT respondem 401", name))
	}

	if len(violations) == 0 {
		fmt.Printf("check-jwks-service-env: ok (%d serviços inicializam JWKS, todos com configuração do Supabase)\n", len(needing))
		return
	}
	for _, v := range violations {
		fmt.Println(v)
	}
	fmt.Printf("\ncheck-jwks-service-env: %d violação(ões)\n", len(violations))
	os.Exit(1)
}

func moduleCallsInitJWKS(dir string) (bool, error) {
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
		if jwksCallRe.Match(content) {
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

func hasSupabaseConfig(env map[string]bool, anchors []map[string]bool) bool {
	if env["SUPABASE_URL"] || env["SUPABASE_JWKS_URL"] {
		return true
	}
	// Herança por âncora: qualquer bloco `x-*` que declare a variável.
	for _, anchor := range anchors {
		if anchor["SUPABASE_URL"] || anchor["SUPABASE_JWKS_URL"] {
			return true
		}
	}
	return false
}
