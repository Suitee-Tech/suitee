package site

import (
	"fmt"
	"strings"
	"unicode"
)

// ReservedSlugs nunca podem ser usados como marca de uma imobiliária: são
// subdomínios de produto/infra ou colidem com a operação da Suitee.
var ReservedSlugs = map[string]struct{}{
	"www": {}, "app": {}, "api": {}, "admin": {}, "portal": {},
	"sites": {}, "mail": {}, "cdn": {}, "imoveis": {}, "suitee": {},
	"blog": {}, "docs": {}, "status": {}, "suporte": {}, "atendimento": {},
	"conta": {}, "login": {}, "painel": {}, "sistema": {}, "checkout": {},
	"assets": {}, "static": {}, "media": {}, "storage": {}, "webhook": {},
	"webhooks": {}, "root": {}, "null": {}, "undefined": {}, "test": {},
	"teste": {}, "demo": {}, "dev": {}, "staging": {}, "beta": {},
	"imobiliaria": {}, "imovel": {},
}

const (
	MinSlugLength = 3
	MaxSlugLength = 48
)

// accentFold mapeia os caracteres acentuados do português para ASCII. Um
// `golang.org/x/text/unicode/norm` resolveria qualquer alfabeto, mas o slug é
// uma marca comercial em português e a tabela explícita evita uma dependência
// (e uma surpresa de normalização) no caminho de criação do site.
var accentFold = map[rune]string{
	'á': "a", 'à': "a", 'â': "a", 'ã': "a", 'ä': "a", 'å': "a",
	'Á': "a", 'À': "a", 'Â': "a", 'Ã': "a", 'Ä': "a", 'Å': "a",
	'é': "e", 'è': "e", 'ê': "e", 'ë': "e",
	'É': "e", 'È': "e", 'Ê': "e", 'Ë': "e",
	'í': "i", 'ì': "i", 'î': "i", 'ï': "i",
	'Í': "i", 'Ì': "i", 'Î': "i", 'Ï': "i",
	'ó': "o", 'ò': "o", 'ô': "o", 'õ': "o", 'ö': "o",
	'Ó': "o", 'Ò': "o", 'Ô': "o", 'Õ': "o", 'Ö': "o",
	'ú': "u", 'ù': "u", 'û': "u", 'ü': "u",
	'Ú': "u", 'Ù': "u", 'Û': "u", 'Ü': "u",
	'ç': "c", 'Ç': "c",
	'ñ': "n", 'Ñ': "n",
}

// Slugify transforma um nome comercial em candidato a `{marca}`:
// minúsculas, ASCII, apenas `[a-z0-9-]`, sem hífens duplicados nem nas pontas.
// O resultado pode ficar fora dos limites (nome muito curto/longo); quem chama
// decide o fallback e a unicidade.
func Slugify(name string) string {
	var b strings.Builder
	lastHyphen := false
	for _, r := range strings.TrimSpace(name) {
		if folded, ok := accentFold[r]; ok {
			for _, fr := range folded {
				b.WriteRune(fr)
			}
			lastHyphen = false
			continue
		}
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(unicode.ToLower(r))
			lastHyphen = false
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastHyphen = false
		default:
			// Qualquer outro caractere vira separador, mas nunca um hífen
			// consecutivo nem um hífen na primeira posição.
			if b.Len() > 0 && !lastHyphen {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// HasLetter informa se o slug tem pelo menos uma letra. Um host formado só
// por números é sintaticamente válido, mas não é uma marca semântica.
func HasLetter(slug string) bool {
	for _, r := range slug {
		if r >= 'a' && r <= 'z' {
			return true
		}
	}
	return false
}

// ValidateSlug aplica o contrato do `{marca}` no host comercial. Slugs são
// UNIQUE no banco, mas validar antes devolve 400 (em vez de 500 de constraint)
// e mantém a regra num só lugar.
func ValidateSlug(slug string) error {
	if slug != rawSlug(slug) {
		return fmt.Errorf("%w: use apenas a-z, 0-9 e hífen", ErrSlugInvalid)
	}
	if len(slug) < MinSlugLength || len(slug) > MaxSlugLength {
		return fmt.Errorf("%w: deve ter entre %d e %d caracteres", ErrSlugInvalid, MinSlugLength, MaxSlugLength)
	}
	if strings.HasPrefix(slug, "-") || strings.HasSuffix(slug, "-") {
		return fmt.Errorf("%w: não pode começar nem terminar com hífen", ErrSlugInvalid)
	}
	if strings.Contains(slug, "--") {
		return fmt.Errorf("%w: não pode conter hífens consecutivos", ErrSlugInvalid)
	}
	if !HasLetter(slug) {
		return fmt.Errorf("%w: use pelo menos uma letra na marca", ErrSlugInvalid)
	}
	if _, reserved := ReservedSlugs[slug]; reserved {
		return fmt.Errorf("%w: %q é reservado", ErrSlugInvalid, slug)
	}
	return nil
}

// rawSlug normaliza o input somente para comparação: nada de trim silencioso,
// porque " marca" no host é um slug diferente e recusar é mais seguro.
func rawSlug(slug string) string {
	var b strings.Builder
	for _, r := range slug {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// BuildPublicBaseURL monta a URL pública do site a partir de um único sufixo
// configurado no servidor. O frontend NUNCA concatena esse host: recebe
// `public_base_url` pronto para não divergir de `PUBLIC_SITE_HOST_SUFFIX`.
func BuildPublicBaseURL(scheme, suffix, slug string) string {
	return BuildPublicBaseURLWithPort(scheme, suffix, slug, "")
}

// BuildPublicBaseURLWithPort é a mesma montagem, mas inclui a porta do site
// quando o ambiente local publica o Next em `{marca}.imoveis.localhost:3002`.
// Produção usa porta vazia (443 implícita).
func BuildPublicBaseURLWithPort(scheme, suffix, slug, port string) string {
	scheme = strings.TrimSpace(scheme)
	if scheme == "" {
		scheme = "https"
	}
	suffix = strings.Trim(strings.TrimSpace(suffix), ".")
	slug = strings.TrimSpace(slug)
	if slug == "" || suffix == "" {
		return ""
	}
	host := slug + "." + suffix
	if p := strings.TrimSpace(port); p != "" {
		host += ":" + p
	}
	return scheme + "://" + host
}

// SlugFromHost extrai o `{marca}` do Host comercial. É a ÚNICA entrada aceita
// para resolver tenant no site público — nunca querystring, header de tenant ou
// cookie. Aceita porta, caixa alta e ponto final (Host não é canônico); recusa
// host sem subdomínio, subdomínio multinível, sufixo diferente e slug inválido.
func SlugFromHost(host, suffix string) (string, error) {
	h := strings.ToLower(strings.TrimSpace(host))
	if i := strings.Index(h, "://"); i >= 0 {
		h = h[i+3:]
	}
	if i := strings.IndexAny(h, "/?#"); i >= 0 {
		h = h[:i]
	}
	if i := strings.LastIndex(h, ":"); i >= 0 && !strings.Contains(h[i:], "]") {
		// Host:porta — o IPv6 não é um Host comercial válido aqui, então o
		// corte simples basta.
		h = h[:i]
	}
	h = strings.TrimSuffix(h, ".")

	suffix = strings.ToLower(strings.Trim(strings.TrimSpace(suffix), "."))
	if h == "" || suffix == "" {
		return "", ErrSiteDisabled
	}
	if !strings.HasSuffix(h, "."+suffix) {
		return "", ErrSiteDisabled
	}
	label := strings.TrimSuffix(h, "."+suffix)
	if label == "" || strings.Contains(label, ".") {
		return "", ErrSiteDisabled
	}
	if err := ValidateSlug(label); err != nil {
		return "", ErrSiteDisabled
	}
	return label, nil
}

// SeoSlug gera o slug do anúncio a partir do título, preservando o formato
// canônico `apartamento-3-quartos-centro`. O caller garante unicidade por
// tenant (a constraint do banco é a última linha de defesa).
func SeoSlug(title, propertyID string, suffix int) string {
	const maxSeoSlug = 80
	base := Slugify(title)
	if base == "" || !HasLetter(base) || len(base) < 3 {
		// Título vazio, numérico ou só de símbolos: usa o id do imóvel como
		// fallback determinístico, sempre com uma parte semântica.
		id := strings.ReplaceAll(propertyID, "-", "")
		if len(id) > 8 {
			id = id[:8]
		}
		base = "imovel-" + id
	}
	if suffix > 1 {
		suffixPart := fmt.Sprintf("-%d", suffix)
		if len(base)+len(suffixPart) > maxSeoSlug {
			base = strings.Trim(base[:maxSeoSlug-len(suffixPart)], "-")
		}
		return base + suffixPart
	}
	if len(base) > maxSeoSlug {
		base = strings.Trim(base[:maxSeoSlug], "-")
	}
	return base
}
