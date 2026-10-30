package helpcorpus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// escreve um corpus mínimo em disco para os testes.
func writeTestCorpus(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	manifest := `topics:
  - id: leads
    file: topicos/03-leads.md
    title: CRM de Leads
    summary: Captar, qualificar e converter leads.
    tags: [leads, captação]
    roles: [admin, corretor]
  - id: fiscal
    file: topicos/13-fiscal.md
    title: Fiscal e Relatórios
    summary: DIMOB, NFS-e e informes.
    tags: [fiscal, dimob]
    roles: [admin, financeiro]
`
	if err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	leads := `---
title: CRM de Leads
summary: Captar, qualificar e converter leads.
tags: [leads, captação]
roles: [admin, corretor]
---
# CRM de Leads

## Passo a passo: cadastrar um lead
1. Clique em Novo lead e preencha o contato.
`
	fiscal := `---
title: Fiscal e Relatórios
summary: DIMOB, NFS-e e informes.
tags: [fiscal, dimob]
roles: [admin, financeiro]
---
# Fiscal e Relatórios

## Gerar DIMOB
1. Em Fiscal, clique em Gerar DIMOB.
`
	td := filepath.Join(dir, "topicos")
	if err := os.MkdirAll(td, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(td, "03-leads.md"), []byte(leads), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(td, "13-fiscal.md"), []byte(fiscal), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoad(t *testing.T) {
	c, err := Load(writeTestCorpus(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Topics) != 2 {
		t.Fatalf("esperava 2 tópicos, veio %d", len(c.Topics))
	}
	if c.Topics[0].ID != "leads" || c.Topics[0].Title != "CRM de Leads" {
		t.Fatalf("primeiro tópico inesperado: %+v", c.Topics[0])
	}
	if topic, ok := c.Topic("fiscal"); !ok || topic.Title != "Fiscal e Relatórios" {
		t.Fatalf("Topic(fiscal) inesperado: %+v ok=%v", topic, ok)
	}
	if _, ok := c.Topic("inexistente"); ok {
		t.Fatal("Topic(inexistente) deveria falhar")
	}
}

func TestSearchRanking(t *testing.T) {
	c, err := Load(writeTestCorpus(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// "gerar dimob" deve ranquear o tópico fiscal primeiro.
	hits := c.Search("como eu gero o dimob", 5)
	if len(hits) == 0 {
		t.Fatal("busca deveria retornar resultados")
	}
	if hits[0].Topic.ID != "fiscal" {
		t.Fatalf("esperava fiscal em primeiro, veio %s", hits[0].Topic.ID)
	}
	// "cadastrar lead" deve ranquear leads.
	hits = c.Search("cadastrar lead", 5)
	if len(hits) == 0 || hits[0].Topic.ID != "leads" {
		t.Fatalf("esperava leads em primeiro, veio %+v", hits)
	}
	// Sem correspondência: vazio.
	if hits = c.Search("zzz qqq", 5); len(hits) != 0 {
		t.Fatalf("busca sem correspondência deveria ser vazia, veio %+v", hits)
	}
}

func TestExcerpt(t *testing.T) {
	c, err := Load(writeTestCorpus(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	hits := c.Search("dimob", 5)
	if len(hits) == 0 || hits[0].Excerpt == "" {
		t.Fatal("excerpt deveria vir preenchido")
	}
}

// O preview da busca alimenta a resposta da IA: não pode trazer marcação crua.
func TestExcerptHasNoMarkdownNoise(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "toka", "src", "help", "docs")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("corpus não encontrado em %s: %v", dir, err)
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, q := range []string{"gerar dimob", "registrar pagamento", "roleta de leads", "assinar contrato"} {
		hits := c.Search(q, 3)
		if len(hits) == 0 {
			t.Fatalf("busca %q não retornou resultados", q)
		}
		for _, h := range hits {
			for _, bad := range []string{"##", "**", "](./", "](../", "```"} {
				if strings.Contains(h.Excerpt, bad) {
					t.Errorf("preview de %q (%q) contém marcação %q: %s", h.Topic.ID, q, bad, h.Excerpt)
				}
			}
		}
	}
}

// A Central de Ajuda alimenta as tools search_help/get_help_topic do MCP: o
// tópico de /site precisa estar carregado e encontrável pela pergunta do usuário.
func TestRealCorpusExposesSiteTopicToMCP(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "toka", "src", "help", "docs")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("corpus não encontrado em %s: %v", dir, err)
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	topic, ok := c.Topic("site")
	if !ok {
		t.Fatal("tópico site deveria existir no corpus do MCP")
	}
	if !strings.Contains(topic.Body, "/site") || !strings.Contains(topic.Body, "HTTPS") {
		t.Fatalf("tópico site não documenta rota e publicação segura: %q", topic.Title)
	}
	hits := c.Search("como configurar o site de imóveis e liberar o link", 3)
	found := false
	for _, hit := range hits {
		if hit.Topic.ID == "site" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("busca sobre site deveria retornar o tópico site entre os 3 primeiros, veio %+v", hits)
	}
}

// Os links entre tópicos precisam sair como rota do app (/ajuda?t=<id>) para a
// Suitee IA citar um caminho que funciona, e não um arquivo .md.
func TestLoadRewritesTopicLinks(t *testing.T) {
	dir := t.TempDir()
	manifest := `topics:
  - id: leads
    file: topicos/03-leads.md
    title: CRM de Leads
    summary: Leads.
    tags: [leads]
    roles: [admin]
  - id: roleta
    file: topicos/04-roleta.md
    title: Roleta
    summary: Roleta.
    tags: [roleta]
    roles: [admin]
`
	if err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	td := filepath.Join(dir, "topicos")
	if err := os.MkdirAll(td, 0o755); err != nil {
		t.Fatal(err)
	}
	leadBody := `---
title: CRM de Leads
summary: Leads.
tags: [leads]
roles: [admin]
---
# CRM de Leads

## Veja também
- [Roleta de Leads](./04-roleta.md)
- [Índice](../index.md)
- [Externo](https://example.com/doc.md)
`
	roletaBody := `---
title: Roleta
summary: Roleta.
tags: [roleta]
roles: [admin]
---
# Roleta

## Veja também
- [CRM de Leads](topicos/03-leads.md)
`
	if err := os.WriteFile(filepath.Join(td, "03-leads.md"), []byte(leadBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(td, "04-roleta.md"), []byte(roletaBody), 0o644); err != nil {
		t.Fatal(err)
	}

	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	leads, _ := c.Topic("leads")
	if !strings.Contains(leads.Body, "](/ajuda?t=roleta)") {
		t.Fatalf("link ./04-roleta.md deveria virar /ajuda?t=roleta:\n%s", leads.Body)
	}
	if strings.Contains(leads.Body, "(./04-roleta.md)") {
		t.Fatalf("link .md não deveria sobrar:\n%s", leads.Body)
	}
	// Alvo fora do manifest e link externo permanecem intactos.
	if !strings.Contains(leads.Body, "](../index.md)") {
		t.Fatalf("link não mapeado deveria permanecer:\n%s", leads.Body)
	}
	if !strings.Contains(leads.Body, "](https://example.com/doc.md)") {
		t.Fatalf("link externo deveria permanecer:\n%s", leads.Body)
	}
	roleta, _ := c.Topic("roleta")
	if !strings.Contains(roleta.Body, "](/ajuda?t=leads)") {
		t.Fatalf("forma topicos/03-leads.md deveria virar /ajuda?t=leads:\n%s", roleta.Body)
	}
}

// O corpus real (toka/src/help/docs) não pode ter link .md sobrando: é o mesmo
// conteúdo que a página /ajuda reescreve e que a IA cita.
func TestRealCorpusLinksAreRewritten(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "toka", "src", "help", "docs")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("corpus não encontrado em %s: %v", dir, err)
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Topics) < 20 {
		t.Fatalf("esperava ao menos 20 tópicos, veio %d", len(c.Topics))
	}
	for _, topic := range c.Topics {
		if strings.Contains(topic.Body, "](.") && strings.Contains(topic.Body, ".md)") {
			t.Errorf("tópico %s ainda tem link .md no corpo", topic.ID)
		}
		for _, m := range mdLinkRe.FindAllString(topic.Body, -1) {
			if !strings.Contains(m, "http") {
				t.Errorf("tópico %s: link não reescrito: %s", topic.ID, m)
			}
		}
	}
}
