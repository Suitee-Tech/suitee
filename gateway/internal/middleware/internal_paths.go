package middleware

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/logger"
)

// internalPathMarker é o segmento que identifica uma rota service-to-service.
// O contrato dos serviços é `/api/v1/<serviço>/internal/...` (toca-deal,
// toca-signature, toca-crm, toca-financeiro, toca-ops, toca-calendar,
// toca-comm, toca-ai). Essas rotas autenticam por `X-Internal-Key` — um
// segredo estático compartilhado, sem vínculo de tenant — e recebem o tenant
// do chamador (`?tenant_id=` ou `X-Tenant-Id`).
const internalPathMarker = "/internal/"

// RejectInternalPaths impede que a borda pública sirva rotas internas.
//
// P0/C10 (revisão de permissões de 2026-09-19): o gateway proxia
// `/api/v1/deal/internal/*path` e `/api/v1/signature/*path` — este último
// casava `/api/v1/signature/internal/...`. Como o proxy repassa os headers do
// cliente sem filtrar, qualquer pessoa com a `INTERNAL_API_KEY` lia deals,
// contratos e os bytes de documentos de qualquer tenant informando o tenant
// desejado, pela internet.
//
// P0/C10 — SEGUNDA RODADA (reauditoria de 2026-09-20): a primeira correção
// comparava apenas `URL.EscapedPath()`, que preserva `%69nternal`. O gin roteia
// pelo path DECODIFICADO (`UseRawPath` é false) e o `httputil.ReverseProxy`
// reencaminha a forma ESCAPADA, que o serviço decodifica de novo — então
// `/api/v1/signature/%69nternal/...` passava pela borda e casava a rota interna
// no upstream. Provado em runtime: literal → 404, com `%69` → 200 com dados de
// outro tenant. A denylist agora avalia TODAS as formas que o caminho pode
// assumir no caminho até o roteador (decodificada, escapada e decodificações
// sucessivas), em vez de apostar em uma delas.
//
// O tráfego interno legítimo NÃO passa por aqui: os serviços conversam entre si
// pela rede do compose, usando `*_SERVICE_URL` direto no serviço
// (ex.: `DEAL_SERVICE_URL=http://toca-deal:8085`). A denylist é global e
// declarativa de propósito: cobre rotas internas futuras sem depender de
// alguém lembrar de não registrá-las.
//
// Respondemos 404 (não 403) para não confirmar a existência dessas rotas.
//
// O bloqueio é por segmento: qualquer caminho que tenha `internal` como
// segmento de URL é recusado, inclusive `/api/v1/internal` (que não é rota
// pública de nenhum serviço). Ser conservador aqui não custa funcionalidade e
// evita depender de o chamador acertar a forma exata do caminho — e vale
// também para o falso positivo de um caminho legítimo que contenha a sequência
// escapada: recusar é o lado seguro.
// retiredPathPrefixes lista caminhos públicos que foram RETIRADOS do produto.
// A borda responde 404 mesmo que alguém volte a registrá-los (proxy no gateway
// ou rota no serviço): sem isso, um `r.Any("/api/v1/x/*path", proxy)` esquecido
// reabre a superfície sem ninguém perceber.
//
// P1/A20 — `/api/v1/portal`: era a superfície por token do portal do cliente
// (token = base64("tenantID:dealID"), sem HMAC e sem TTL; o tenant vinha do
// token e o escopo de corretor era desligado). A capacidade vive na superfície
// autenticada `/api/v1/client-portal/*`.
var retiredPathPrefixes = []string{
	"/api/v1/portal/",
}

// maxPathDecodePasses limita a decodificação sucessiva. Duas passadas cobrem a
// codificação dupla; a terceira é folga para intermediários que normalizam o
// caminho. O limite evita laço em entrada construída para não convergir.
const maxPathDecodePasses = 3

// pathCandidates devolve todas as formas do caminho que podem chegar ao
// roteador do serviço de destino, para a denylist não depender de qual delas o
// chamador escolheu:
//
//   - `URL.Path` — já decodificado pelo `net/http` (é o que o gin usa para
//     rotear);
//   - `URL.EscapedPath()` — a forma crua que o `ReverseProxy` reencaminha;
//   - as decodificações sucessivas de ambas (`%2569` → `%69` → `i`).
func pathCandidates(u *url.URL) []string {
	seen := make(map[string]struct{}, 8)
	out := make([]string, 0, 8)
	add := func(p string) {
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}

	seeds := []string{u.Path, u.EscapedPath()}
	for _, seed := range seeds {
		add(seed)
		cur := seed
		for i := 0; i < maxPathDecodePasses; i++ {
			decoded, err := url.PathUnescape(cur)
			// Entrada malformada (`%zz`) não decodifica: a forma literal já
			// está na lista e é ela que o roteador veria.
			if err != nil || decoded == cur {
				break
			}
			cur = decoded
			add(cur)
		}
	}
	return out
}

// normalizePath reproduz a normalização que o roteador aplica: minúsculas e
// barras duplicadas colapsadas, com barra inicial.
func normalizePath(raw string) string {
	joined := strings.Join(strings.FieldsFunc(raw, func(r rune) bool { return r == '/' }), "/")
	return "/" + strings.ToLower(joined)
}

// blockedPathReason diz POR QUE um caminho já normalizado foi recusado. Os dois
// motivos têm eventos de auditoria distintos (`edge.blocked_retired_path` e
// `edge.blocked_internal_path`), e há alerta apoiado neles — colapsar os dois
// apagaria a distinção entre "alguém tentou uma superfície retirada" e "alguém
// tentou uma rota interna".
func blockedPathReason(normalizedPath string) string {
	for _, retired := range retiredPathPrefixes {
		if normalizedPath == strings.TrimSuffix(retired, "/") || strings.HasPrefix(normalizedPath, retired) {
			return "retired"
		}
	}
	if strings.Contains(normalizedPath+"/", internalPathMarker) {
		return "internal"
	}
	return ""
}

// RejectInternalPaths impede que a borda pública sirva rotas internas e
// caminhos retirados.
func RejectInternalPaths() gin.HandlerFunc {
	return func(c *gin.Context) {
		for _, candidate := range pathCandidates(c.Request.URL) {
			reason := blockedPathReason(normalizePath(candidate))
			if reason == "" {
				continue
			}
			event := "edge.blocked_internal_path"
			if reason == "retired" {
				event = "edge.blocked_retired_path"
			}
			// O evento registra a forma ORIGINAL e a que disparou o bloqueio:
			// sem as duas não dá para distinguir tráfego legítimo malformado de
			// tentativa de bypass por codificação.
			logger.Warn(event,
				"event", event,
				"ip", c.ClientIP(),
				"path", c.Request.URL.EscapedPath(),
				"matched", candidate,
				"method", c.Request.Method,
				"user_agent", c.Request.UserAgent(),
			)
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
				"success":    false,
				"message":    "not found",
				"statusCode": http.StatusNotFound,
				"data":       nil,
			})
			return
		}
		c.Next()
	}
}
