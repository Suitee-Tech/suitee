// Package helpcorpus carrega a Central de Ajuda (docs/help-center) e fornece
// busca local para as tools search_help/get_help_topic do toca-mcp.
//
// O corpus é a MESMA pasta consumida pela página /ajuda do frontend: a fonte
// única da documentação de uso. O formato é um frontmatter mínimo (chave: valor
// por linha entre delimitadores ---) e um manifest.yaml com o índice.
package helpcorpus

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Topic é um tópico da Central de Ajuda.
type Topic struct {
	ID      string
	File    string
	Title   string
	Summary string
	Tags    []string
	Roles   []string
	Body    string // markdown sem frontmatter, com links já reescritos
	// Plain é o mesmo conteúdo sem marcação: indexa a busca e alimenta o
	// preview dos resultados (sem `##`, `**`, tabelas ou links crus).
	Plain string
}

// SearchHit é um resultado de busca.
type SearchHit struct {
	Topic   Topic
	Score   float64
	Excerpt string
}

// Corpus é o índice completo da Central de Ajuda.
type Corpus struct {
	Topics []Topic
	byID   map[string]int
}

var listItemRe = regexp.MustCompile(`^\s*-\s+(\w+):\s*(.*)$`)
var kvRe = regexp.MustCompile(`^([a-z_]+):\s*(.*)$`)
var bracketsRe = regexp.MustCompile(`\[(.*)\]`)

// splitList interpreta "a, b, c" ou "[a, b, c]".
func splitList(s string) []string {
	m := bracketsRe.FindStringSubmatch(s)
	if len(m) == 2 {
		s = m[1]
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.Trim(strings.TrimSpace(p), `"'`)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseFrontmatter extrai o bloco `--- ... ---` inicial de um arquivo markdown.
func parseFrontmatter(raw string) (map[string]string, string, error) {
	lines := strings.Split(raw, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, "", fmt.Errorf("frontmatter ausente")
	}
	meta := map[string]string{}
	i := 1
	for ; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			break
		}
		if kv := kvRe.FindStringSubmatch(lines[i]); kv != nil {
			meta[kv[1]] = strings.TrimSpace(kv[2])
		}
	}
	if i >= len(lines) {
		return nil, "", fmt.Errorf("frontmatter sem fechamento")
	}
	body := strings.Join(lines[i+1:], "\n")
	return meta, body, nil
}

// LoadManifest carrega o índice manifest.yaml (subconjunto YAML sem dependências).
func LoadManifest(dir string) ([]Topic, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.yaml"))
	if err != nil {
		return nil, fmt.Errorf("manifest.yaml: %w", err)
	}
	var topics []Topic
	var cur *Topic
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "topics:") {
			continue
		}
		m := listItemRe.FindStringSubmatch(line)
		if m == nil {
			// linhas de continuação do item corrente (file:, title:, ...)
			if cur != nil {
				if kv := kvRe.FindStringSubmatch(trimmed); kv != nil {
					applyManifestKey(cur, kv[1], strings.TrimSpace(kv[2]))
				}
			}
			continue
		}
		key, val := m[1], strings.TrimSpace(m[2])
		if key == "id" {
			topics = append(topics, Topic{})
			cur = &topics[len(topics)-1]
			cur.ID = val
			continue
		}
		if cur != nil {
			applyManifestKey(cur, key, val)
		}
	}
	if len(topics) == 0 {
		return nil, fmt.Errorf("manifest.yaml sem tópicos")
	}
	return topics, nil
}

func applyManifestKey(t *Topic, key, val string) {
	switch key {
	case "file":
		t.File = val
	case "title":
		t.Title = val
	case "summary":
		t.Summary = val
	case "tags":
		t.Tags = splitList(val)
	case "roles":
		t.Roles = splitList(val)
	}
}

// Load lê manifest.yaml e os arquivos de topicos/*.md do diretório indicado.
func Load(dir string) (*Corpus, error) {
	topics, err := LoadManifest(dir)
	if err != nil {
		return nil, err
	}
	// Mapa arquivo -> id: reescreve os links entre tópicos para a rota da
	// Central de Ajuda no app (o mesmo tratamento da página /ajuda), para que
	// a Suitee IA cite um link que funciona em vez de um caminho .md.
	fileToID := map[string]string{}
	for _, t := range topics {
		if t.File == "" || t.ID == "" {
			continue
		}
		base := strings.TrimPrefix(t.File, "topicos/")
		fileToID[t.File] = t.ID
		fileToID[base] = t.ID
		fileToID["./"+base] = t.ID
	}
	corpus := &Corpus{byID: map[string]int{}}
	for i := range topics {
		t := &topics[i]
		if t.File == "" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, t.File))
		if err != nil {
			return nil, fmt.Errorf("tema %s: %w", t.File, err)
		}
		meta, body, err := parseFrontmatter(string(raw))
		if err != nil {
			return nil, fmt.Errorf("tema %s: %w", t.File, err)
		}
		// O manifest é o índice canônico; o frontmatter do arquivo confirma.
		if v := meta["title"]; t.Title == "" && v != "" {
			t.Title = v
		}
		if v := meta["summary"]; t.Summary == "" && v != "" {
			t.Summary = v
		}
		if len(t.Tags) == 0 {
			t.Tags = splitList(meta["tags"])
		}
		if len(t.Roles) == 0 {
			t.Roles = splitList(meta["roles"])
		}
		t.Body = rewriteTopicLinks(body, fileToID)
		t.Plain = markdownToPlainText(t.Body)
		corpus.Topics = append(corpus.Topics, *t)
		corpus.byID[t.ID] = len(corpus.Topics) - 1
	}
	if len(corpus.Topics) == 0 {
		return nil, fmt.Errorf("corpus vazio em %s", dir)
	}
	return corpus, nil
}

var mdLinkRe = regexp.MustCompile(`\]\(([^)\s]+?\.md)(#[^)\s]*)?\)`)

var mdNoiseRes = []*regexp.Regexp{
	regexp.MustCompile("(?s)```.*?```"),                 // blocos de código
	regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`),          // imagens
	regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`),         // links -> texto
	regexp.MustCompile(`(?m)^\s{0,3}#{1,6}\s+`),         // títulos
	regexp.MustCompile(`(?m)^\s{0,3}>\s?`),              // citações
	regexp.MustCompile(`(?m)^\s*[-*+]\s+`),              // listas
	regexp.MustCompile(`(?m)^\s*\d+\.\s+`),              // listas numeradas
	regexp.MustCompile(`(?m)^\s*\|?[\s:|-]{3,}\|?\s*$`), // separador de tabela
	regexp.MustCompile(`\|`),                            // colunas de tabela
	regexp.MustCompile("`([^`]+)`"),                     // código inline
	regexp.MustCompile(`\*\*([^*]+)\*\*`),               // negrito
	regexp.MustCompile(`\*([^*]+)\*`),                   // itálico
	regexp.MustCompile(`__([^_]+)__`),                   // negrito alternativo
	// RE2 (Go) não tem backreference: alterna os três marcadores de régua.
	regexp.MustCompile(`(?m)^\s*(?:-{3,}|\*{3,}|_{3,})\s*$`), // régua horizontal
}

var mdLinkTextRe = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)

// markdownToPlainText remove a marcação do tópico para o preview da busca.
func markdownToPlainText(markdown string) string {
	out := markdown
	// Links primeiro: preserva o texto visível ([texto](url) -> texto).
	out = mdLinkTextRe.ReplaceAllString(out, "$1")
	out = mdNoiseRes[0].ReplaceAllString(out, " ")
	for _, re := range mdNoiseRes[1:] {
		out = re.ReplaceAllString(out, " ")
	}
	out = strings.Join(strings.Fields(out), " ")
	return strings.TrimSpace(out)
}

// rewriteTopicLinks converte links entre tópicos (`./03-leads.md`,
// `topicos/03-leads.md`) em `/ajuda?t=<id>`. Links desconhecidos ficam como
// estão (o teste do corpus garante que todos resolvem).
func rewriteTopicLinks(markdown string, fileToID map[string]string) string {
	return mdLinkRe.ReplaceAllStringFunc(markdown, func(match string) string {
		m := mdLinkRe.FindStringSubmatch(match)
		if m == nil {
			return match
		}
		target, hash := m[1], m[2]
		base := strings.TrimPrefix(strings.TrimPrefix(target, "./"), "topicos/")
		candidates := []string{target, base, "./" + base, "topicos/" + base}
		id := ""
		for _, c := range candidates {
			if v, ok := fileToID[c]; ok {
				id = v
				break
			}
		}
		if id == "" {
			return match
		}
		return "](/ajuda?t=" + id + hash + ")"
	})
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

var stopwords = map[string]bool{
	"a": true, "o": true, "e": true, "é": true, "de": true, "da": true, "do": true,
	"em": true, "no": true, "na": true, "um": true, "uma": true, "para": true,
	"com": true, "como": true, "que": true, "por": true, "os": true, "as": true,
	"ao": true, "à": true, "se": true, "eu": true, "meu": true, "minha": true,
	"fazer": true, "faz": true, "mais": true, "sobre": true, "qual": true,
	"quais": true, "onde": true, "quando": true, "tenho": true, "quero": true,
}

// tokenize separa em termos normalizados (min. 2 caracteres, sem stopwords).
func tokenize(s string) []string {
	fields := strings.FieldsFunc(normalize(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '-'
	})
	seen := map[string]bool{}
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.Trim(f, "-")
		if len(f) < 2 || stopwords[f] {
			continue
		}
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	return out
}

func countTokens(haystack string, terms []string) float64 {
	hay := " " + normalize(haystack) + " "
	score := 0.0
	for _, t := range terms {
		score += float64(strings.Count(hay, t))
	}
	return score
}

// Search ranqueia os tópicos pelos termos da consulta (título e tags pesam
// mais; resumo e corpo menos). Retorna até limit resultados com score > 0.
func (c *Corpus) Search(query string, limit int) []SearchHit {
	terms := tokenize(query)
	if len(terms) == 0 {
		return nil
	}
	phrase := strings.Join(terms, " ")
	hits := make([]SearchHit, 0, len(c.Topics))
	for _, t := range c.Topics {
		score := countTokens(t.Title, terms)*8 +
			countTokens(strings.Join(t.Tags, " "), terms)*6 +
			countTokens(t.Summary, terms)*3 +
			countTokens(t.Plain, terms)
		if score <= 0 {
			continue
		}
		if strings.Contains(" "+normalize(t.Title)+" ", phrase) {
			score += 4
		}
		hits = append(hits, SearchHit{Topic: t, Score: score, Excerpt: excerptFor(t, terms)})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].Topic.ID < hits[j].Topic.ID
	})
	if limit <= 0 {
		limit = 5
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

// excerptFor devolve um trecho do corpo em texto puro que contenha termos da
// busca, ou o resumo como fallback.
func excerptFor(t Topic, terms []string) string {
	plain := t.Plain
	if plain == "" {
		plain = markdownToPlainText(t.Body)
	}
	body := normalize(plain)
	best := -1
	for _, term := range terms {
		if i := strings.Index(body, term); i >= 0 && (best == -1 || i < best) {
			best = i
		}
	}
	if best < 0 {
		return strings.TrimSpace(t.Summary)
	}
	raw := []rune(plain)
	start := best - 60
	if start < 0 {
		start = 0
	}
	end := start + 220
	if end > len(raw) {
		end = len(raw)
	}
	if start > len(raw) {
		start = 0
	}
	out := strings.Join(strings.Fields(string(raw[start:end])), " ")
	if start > 0 {
		out = "…" + out
	}
	if end < len(raw) {
		out += "…"
	}
	return out
}

// Topic devolve o tópico pelo id do manifest (ex.: "leads").
func (c *Corpus) Topic(id string) (*Topic, bool) {
	idx, ok := c.byID[id]
	if !ok {
		return nil, false
	}
	t := c.Topics[idx]
	return &t, true
}
