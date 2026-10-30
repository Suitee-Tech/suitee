// check-frontend-env garante que o runtime env do frontend (env.js) não exponha
// NENHUMA credencial ao navegador.
//
// Contexto: o docker-entrypoint do frontend reescreve /env.js em runtime com um
// allowlist de variáveis VITE_*. Depois que o reset de senha passou a ser
// mediado pelo backend (POST /auth/reset-password), o navegador deixou de falar
// com o Supabase — então a anon key saiu do bundle e o arquivo passou a conter
// apenas URLs, flags de UI e a versão. O segredo real (service_role) nunca
// esteve ali e continua sem poder entrar.
//
// O check valida as camadas de defesa:
//  1. o allowlist do entrypoint não contém nada com cara de credencial
//     (key/secret/token/password/service_role/internal) — nem a anon key;
//  2. o entrypoint mantém o fail-fast das variáveis obrigatórias (sem ele, a
//     imagem serve env.js vazio e o app cai nos defaults de build — erro
//     silencioso);
//  3. nenhum serviço de frontend dos compose injeta variável com nome de
//     segredo;
//  4. o source do frontend não referencia service_role nem traz o supabase-js
//     de volta para o browser (o cliente Supabase no navegador é o que exige
//     uma chave pública no bundle).
//
// Usage:
//
//	go run ./scripts/check-frontend-env -root ../..
//
// Exit codes: 0 ok, 1 violação, 2 erro de uso/IO.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// keyListRe extrai a lista do allowlist (`keys="..."`) do entrypoint.
var keyListRe = regexp.MustCompile(`keys="([^"]+)"`)

// requiredListRe extrai a lista das variáveis obrigatórias.
var requiredListRe = regexp.MustCompile(`required_keys="([^"]+)"`)

// credentialNameRe é o padrão que nenhuma chave do allowlist pode casar. Depois
// da saída da anon key, nada aqui pode sequer parecer credencial.
var credentialNameRe = regexp.MustCompile(`(?i)(key|secret|token|password|passwd|credential|service_role|internal|private)`)

// guardRequiredRe exige o fail-fast de variáveis obrigatórias no entrypoint.
var guardRequiredRe = regexp.MustCompile(`recusando subir`)

// composeServiceRe captura o início dos blocos de serviço de frontend.
var composeServiceRe = regexp.MustCompile(`(?m)^  (?:toka-frontend|toca-frontend)(?:-next)?:`)

// composeNextServiceRe marca o começo do próximo serviço (2 espaços + nome).
var composeNextServiceRe = regexp.MustCompile(`(?m)^  [a-z][a-z0-9_-]*:`)

// envEntryRe casa as variáveis injetadas via `environment:`.
var envEntryRe = regexp.MustCompile(`(?m)^\s*-\s*([A-Z][A-Z0-9_]*)\s*=.*$`)

// supabaseClientImportRe detecta o cliente Supabase voltando ao navegador.
var supabaseClientImportRe = regexp.MustCompile(`@supabase/supabase-js`)

// frontendServiceEnvNames devolve os nomes de variáveis injetadas nos blocos
// `environment:` dos serviços de frontend de um arquivo compose.
func frontendServiceEnvNames(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := string(data)
	out := map[string]string{}
	for _, m := range composeServiceRe.FindAllStringIndex(content, -1) {
		start := m[0]
		rest := content[start:]
		end := len(rest)
		if n := composeNextServiceRe.FindStringIndex(rest[3:]); n != nil {
			end = 3 + n[0]
		}
		block := rest[:end]
		envIdx := strings.Index(block, "environment:")
		if envIdx < 0 {
			continue
		}
		envBlock := block[envIdx:]
		if n := composeNextServiceRe.FindStringIndex(envBlock[12:]); n != nil {
			envBlock = envBlock[:12+n[0]]
		}
		for _, em := range envEntryRe.FindAllStringSubmatch(envBlock, -1) {
			out[em[1]] = path
		}
	}
	return out, nil
}

func main() {
	root := flag.String("root", ".", "raiz do repositório")
	flag.Parse()

	violations := []string{}

	entrypoint := filepath.Join(*root, "toka", "docker-entrypoint.sh")
	data, err := os.ReadFile(entrypoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "entrypoint não legível: %v\n", err)
		os.Exit(2)
	}
	content := string(data)

	// 1. Allowlist: nenhuma credencial.
	if m := keyListRe.FindStringSubmatch(content); m != nil {
		for _, key := range strings.Fields(m[1]) {
			if credentialNameRe.MatchString(key) {
				violations = append(violations, fmt.Sprintf("entrypoint keys: %q parece credencial (padrão %s)", key, credentialNameRe))
			}
		}
	} else {
		violations = append(violations, "entrypoint: allowlist `keys=\"...\"` não encontrada")
	}

	// 2. Fail-fast das obrigatórias.
	if m := requiredListRe.FindStringSubmatch(content); m != nil {
		required := strings.Fields(m[1])
		if len(required) == 0 {
			violations = append(violations, "entrypoint: required_keys vazio — o container subiria com env.js incompleto")
		}
		for _, key := range required {
			if credentialNameRe.MatchString(key) {
				violations = append(violations, fmt.Sprintf("entrypoint required_keys: %q parece credencial", key))
			}
		}
	} else {
		violations = append(violations, "entrypoint: required_keys não encontrado")
	}
	if !guardRequiredRe.MatchString(content) {
		violations = append(violations, "entrypoint: guard fail-fast de variáveis obrigatórias não encontrado")
	}

	// 3. Source do frontend.
	serviceRoleRe := regexp.MustCompile(`(?i)service_role`)
	for _, dir := range []string{
		filepath.Join(*root, "toka", "src"),
		filepath.Join(*root, "toka", "public"),
		filepath.Join(*root, "toka", "index.html"),
	} {
		info, statErr := os.Stat(dir)
		if statErr != nil {
			continue
		}
		checkFile := func(path string) {
			b, readErr := os.ReadFile(path)
			if readErr != nil {
				return
			}
			if serviceRoleRe.Match(b) {
				violations = append(violations, fmt.Sprintf("%s: referência a service_role no bundle do frontend", path))
			}
			if supabaseClientImportRe.Match(b) {
				violations = append(violations, fmt.Sprintf("%s: importa @supabase/supabase-js — cliente Supabase no navegador exige credencial pública em /env.js", path))
			}
		}
		if !info.IsDir() {
			checkFile(dir)
			continue
		}
		_ = filepath.Walk(dir, func(path string, d os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if d.IsDir() {
				switch d.Name() {
				case "node_modules", "dist", ".git":
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".tsx") {
				return nil
			}
			checkFile(path)
			return nil
		})
	}

	// 4. Compose: serviço de frontend não injeta credencial.
	composeFiles := []string{
		filepath.Join(*root, "docker-compose.yml"),
		filepath.Join(*root, "docker-compose.prod.yml"),
		filepath.Join(*root, "docker-compose.beta.yml"),
		filepath.Join(*root, "docker-compose.test.yml"),
		filepath.Join(*root, "docker-compose.hostinger-build.yml"),
	}
	for _, f := range composeFiles {
		names, envErr := frontendServiceEnvNames(f)
		if envErr != nil {
			fmt.Fprintf(os.Stderr, "compose não legível (%s): %v\n", f, envErr)
			os.Exit(2)
		}
		for name, where := range names {
			if credentialNameRe.MatchString(name) {
				violations = append(violations, fmt.Sprintf("%s: variável %q injetada no frontend parece credencial", where, name))
			}
		}
	}

	if len(violations) > 0 {
		fmt.Println("check-frontend-env: violações")
		for _, v := range violations {
			fmt.Println("  -", v)
		}
		os.Exit(1)
	}
	fmt.Println("check-frontend-env: ok (env.js sem credencial, fail-fast presente, sem supabase-js no navegador)")
}
