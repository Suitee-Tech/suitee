package tlsconfig

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeLister struct{ slugs []string }

func (f fakeLister) ListEnabledSlugs(context.Context) ([]string, error) { return f.slugs, nil }

func TestRenderCreatesOneLiteralRouterPerSite(t *testing.T) {
	body, err := Render("imoveis.suitee.com.br", "http://toca-web:3000", []string{"Exemplo-imoveis", "lopes-premium"})
	if err != nil {
		t.Fatal(err)
	}
	out := string(body)
	for _, want := range []string{
		"site-tls-Exemplo-imoveis:",
		"rule: \"Host(`Exemplo-imoveis.imoveis.suitee.com.br`)\"",
		"site-tls-lopes-premium:",
		"certResolver: myresolver",
		"url: \"http://toca-web:3000\"",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("configuração sem %q:\n%s", want, out)
		}
	}
}

func TestRenderRejectsInvalidSlug(t *testing.T) {
	if _, err := Render("imoveis.suitee.com.br", "http://toca-web:3000", []string{"ok", "evil`host"}); err == nil {
		t.Fatal("slug inválido deveria falhar")
	}
}

func TestWriterUpdatesFileAtomically(t *testing.T) {
	dir := t.TempDir()
	writer := NewWriter(fakeLister{slugs: []string{"Exemplo-imoveis"}}, dir, "imoveis.suitee.com.br", "http://toca-web:3000")
	if err := writer.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, outputFile)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(before), "Exemplo-imoveis") {
		t.Fatalf("arquivo inesperado: %s", before)
	}
	writer.lister = fakeLister{slugs: []string{"Exemplo-imoveis", "lopes-premium"}}
	if err := writer.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "lopes-premium") {
		t.Fatalf("novo site não entrou: %s", after)
	}
}
