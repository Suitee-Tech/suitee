// Package tlsconfig mantém os routers TLS da vitrine sincronizados com os
// sites habilitados no banco. O Traefik lê o arquivo por file provider e emite
// um certificado por Host literal; não existe lista manual de imobiliárias.
package tlsconfig

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	defaultInterval = 30 * time.Second
	defaultUpstream = "http://toca-web:3000"
	outputFile      = "sites.yml"
)

var (
	slugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{1,46}[a-z0-9])$`)
	hostPattern = regexp.MustCompile(`^[a-z0-9.-]+$`)
)

// SlugLister é a dependência mínima do sincronizador; o repositório Postgres
// implementa a consulta cross-tenant usando o pool de serviço.
type SlugLister interface {
	ListEnabledSlugs(ctx context.Context) ([]string, error)
}

// Writer observa os sites habilitados e materializa os routers no diretório
// compartilhado com o Traefik.
type Writer struct {
	lister   SlugLister
	dir      string
	suffix   string
	upstream string
	interval time.Duration
}

func NewWriter(lister SlugLister, dir, hostSuffix, upstream string) *Writer {
	if strings.TrimSpace(upstream) == "" {
		upstream = defaultUpstream
	}
	return &Writer{
		lister:   lister,
		dir:      dir,
		suffix:   strings.Trim(strings.TrimSpace(hostSuffix), "."),
		upstream: strings.TrimSpace(upstream),
		interval: defaultInterval,
	}
}

// Run faz a primeira sincronização imediatamente e depois a cada intervalo.
// Erros de banco/escrita são logados e não derrubam o serviço de vitrine.
func (w *Writer) Run(ctx context.Context) {
	w.syncAndLog(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.syncAndLog(ctx)
		}
	}
}

func (w *Writer) syncAndLog(ctx context.Context) {
	if err := w.Sync(ctx); err != nil {
		log.Printf("[tlsconfig] sincronização falhou: %v", err)
	}
}

// Sync consulta os slugs e escreve o arquivo de forma atômica. Se o conteúdo
// não mudou, preserva o arquivo e evita um reload desnecessário no Traefik.
func (w *Writer) Sync(ctx context.Context) error {
	if strings.TrimSpace(w.dir) == "" {
		return nil
	}
	slugs, err := w.lister.ListEnabledSlugs(ctx)
	if err != nil {
		return fmt.Errorf("listar sites habilitados: %w", err)
	}
	body, err := Render(w.suffix, w.upstream, slugs)
	if err != nil {
		return err
	}
	return writeIfChanged(w.dir, body)
}

// Render gera a configuração dinâmica do file provider. Slugs são validados
// antes de entrar no YAML; um valor inesperado falha fechado e mantém a última
// configuração boa.
func Render(hostSuffix, upstream string, slugs []string) ([]byte, error) {
	suffix := strings.Trim(strings.TrimSpace(hostSuffix), ".")
	if suffix == "" || !hostPattern.MatchString(suffix) || strings.Contains(suffix, "..") {
		return nil, fmt.Errorf("sufixo de host inválido: %q", hostSuffix)
	}
	upstream = strings.TrimSpace(upstream)
	if upstream == "" {
		upstream = defaultUpstream
	}

	var b strings.Builder
	b.WriteString("http:\n  routers:\n")
	if len(slugs) == 0 {
		b.WriteString("    {}\n")
	} else {
		for _, raw := range slugs {
			slug := strings.TrimSpace(raw)
			if !slugPattern.MatchString(slug) {
				return nil, fmt.Errorf("slug inválido para TLS: %q", raw)
			}
			host := slug + "." + suffix
			fmt.Fprintf(&b, "    site-tls-%s:\n", slug)
			fmt.Fprintf(&b, "      rule: %q\n", fmt.Sprintf("Host(`%s`)", host))
			b.WriteString("      priority: 100\n")
			b.WriteString("      entryPoints:\n        - websecure\n")
			b.WriteString("      service: site-public\n")
			b.WriteString("      tls:\n        certResolver: myresolver\n")
		}
	}
	b.WriteString("  services:\n")
	if len(slugs) == 0 {
		b.WriteString("    {}\n")
	} else {
		b.WriteString("    site-public:\n      loadBalancer:\n        servers:\n")
		fmt.Fprintf(&b, "          - url: %q\n", upstream)
	}
	return []byte(b.String()), nil
}

func writeIfChanged(dir string, body []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("criar diretório TLS: %w", err)
	}
	path := filepath.Join(dir, outputFile)
	if current, err := os.ReadFile(path); err == nil && string(current) == string(body) {
		return nil
	}

	tmp, err := os.CreateTemp(dir, ".sites-*.yml")
	if err != nil {
		return fmt.Errorf("criar arquivo TLS temporário: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("publicar configuração TLS: %w", err)
	}
	log.Printf("[tlsconfig] %d host(s) sincronizado(s) em %s", strings.Count(string(body), "site-tls-"), path)
	return nil
}
