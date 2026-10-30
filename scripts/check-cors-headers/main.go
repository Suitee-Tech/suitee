// check-cors-headers garante que todo header customizado que o frontend envia
// esteja na lista que a borda pública (gateway) declara em
// Access-Control-Allow-Headers.
//
// Incidente de 2026-09-30: o modal "marcar cobrança como paga" falhava com
// "Request header field idempotency-key is not allowed by Access-Control-Allow-
// Headers in preflight response". O backend lia `Idempotency-Key` (grafia
// canônica, que o axios manda no fio), mas a lista de CORS só tinha
// `X-Idempotency-Key`. O preflight morre ANTES da requisição sair do browser:
// nenhum log de servidor, nenhum erro de aplicação — só um Network Error no
// console. Comparar as duas listas automaticamente é o único jeito barato de
// não repetir isso.
//
// Fontes:
//   - headers enviados: `headers: { ... }` em toka/src (chamadas axios) e os
//     defaults do httpClient (`headers.set("X-Request-ID", ...)`);
//   - headers permitidos: AllowedHeaders do gateway/cmd/gateway/router.go.
//
// Headers "safelisted" pelo próprio CORS (Accept, Accept-Language,
// Content-Language, Content-Type, Range) não precisam constar: o browser os
// envia sem preflight. `Content-Type` fica na lista por ser explicitado.
//
// Usage:
//
//	go run ./scripts/check-cors-headers -root ../..
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

// axiosHeadersRe casa o objeto de headers das chamadas axios.
var axiosHeadersRe = regexp.MustCompile(`(?s)headers\s*:\s*\{([^}]*)\}`)

// headerKeyRe extrai a chave de cada entrada do objeto (`"X-Foo": valor`).
var headerKeyRe = regexp.MustCompile(`"([A-Za-z][A-Za-z0-9-]*)"\s*:`)

// headersSetRe casa `headers.set("X-Foo"` / `headers["X-Foo"] =` no httpClient.
var headersSetRe = regexp.MustCompile(`headers(?:\.set\(|\[)\s*"([A-Za-z][A-Za-z0-9-]*)"`)

// allowedHeadersRe lê a lista do gateway. A fonte é a var
// `gatewayCORSAllowedHeaders` (usada pelo SetupCORS); a forma inline fica como
// fallback para não quebrar se alguém voltar a declarar no campo.
var allowedHeadersRe = regexp.MustCompile(`(?s)gatewayCORSAllowedHeaders\s*=\s*\[\]string\{([^}]*)\}`)
var allowedHeadersInlineRe = regexp.MustCompile(`(?s)AllowedHeaders:\s*\[\]string\{([^}]*)\}`)

// stringLiteralRe extrai cada literal da lista.
var stringLiteralRe = regexp.MustCompile(`"([^"]+)"`)

// safelisted: o browser envia sem preflight; não precisam estar na lista.
var safelisted = map[string]bool{
	"accept": true, "accept-language": true, "content-language": true, "range": true,
}

// allowlist: headers que o frontend envia para destinos que NÃO são a nossa
// API (provedores externos, Supabase Storage, etc.) — o CORS deles não é nosso.
var allowlist = map[string]bool{}

func main() {
	root := flag.String("root", ".", "raiz do repositório")
	flag.Parse()

	allowed, err := gatewayAllowedHeaders(filepath.Join(*root, "gateway", "cmd", "gateway", "router.go"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-cors-headers: %v\n", err)
		os.Exit(2)
	}
	if len(allowed) == 0 {
		fmt.Fprintln(os.Stderr, "check-cors-headers: não achei AllowedHeaders no gateway")
		os.Exit(2)
	}

	sent, err := frontendHeaders(filepath.Join(*root, "toka", "src"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-cors-headers: %v\n", err)
		os.Exit(2)
	}

	var violations []string
	for header, sources := range sent {
		key := strings.ToLower(strings.TrimSpace(header))
		if key == "" || safelisted[key] || allowlist[key] || allowed[key] {
			continue
		}
		sort.Strings(sources)
		violations = append(violations, fmt.Sprintf("%s (enviado em %s) não está em Access-Control-Allow-Headers do gateway — o preflight bloqueia a requisição antes de sair do browser", header, strings.Join(sources, ", ")))
	}
	sort.Strings(violations)
	for _, v := range violations {
		fmt.Println(v)
	}
	if len(violations) > 0 {
		fmt.Printf("check-cors-headers: %d violação(ões)\n", len(violations))
		os.Exit(1)
	}
	fmt.Printf("check-cors-headers: ok (%d headers enviados pelo frontend, todos permitidos na borda)\n", len(sent))
}

// gatewayAllowedHeaders devolve o conjunto (minúsculo) de headers permitidos.
func gatewayAllowedHeaders(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m := allowedHeadersRe.FindSubmatch(data)
	if m == nil {
		m = allowedHeadersInlineRe.FindSubmatch(data)
	}
	if m == nil {
		return nil, fmt.Errorf("lista de AllowedHeaders não encontrada em %s", path)
	}
	allowed := map[string]bool{}
	for _, lit := range stringLiteralRe.FindAllStringSubmatch(string(m[1]), -1) {
		allowed[strings.ToLower(strings.TrimSpace(lit[1]))] = true
	}
	return allowed, nil
}

// frontendHeaders varre o código do frontend atrás dos headers customizados.
func frontendHeaders(dir string) (map[string][]string, error) {
	sent := map[string][]string{}
	add := func(name, source string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		for _, existing := range sent[name] {
			if existing == source {
				return
			}
		}
		sent[name] = append(sent[name], source)
	}

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
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
		if strings.HasSuffix(path, ".test.ts") || strings.HasSuffix(path, ".test.tsx") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		rel, _ := filepath.Rel(filepath.Dir(dir), path)
		content := string(data)
		for _, block := range axiosHeadersRe.FindAllStringSubmatch(content, -1) {
			for _, key := range headerKeyRe.FindAllStringSubmatch(block[1], -1) {
				add(key[1], rel)
			}
		}
		for _, key := range headersSetRe.FindAllStringSubmatch(content, -1) {
			add(key[1], rel)
		}
		return nil
	})
	return sent, err
}
