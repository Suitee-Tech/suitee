package handlers

import (
	"log"
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-comm/internal/service"
)

// buildUpgrader cria o websocket.Upgrader com CheckOrigin validando
// contra a lista de origins permitidos. Aceita "*" como wildcard apenas
// em desenvolvimento (lista vazia ou contendo "*").
//
// Em produção, allowedOrigins deve listar explicitamente os hosts do
// app/portal (ex.: https://app.example.com, https://portal.example.com).
// Sem essa validação o handler aceita conexões WebSocket de qualquer
// origem, permitindo CSRF — site malicioso abre WS com cookies/JWT da
// vítima e recebe eventos sensíveis em tempo real.
func buildUpgrader(allowedOrigins []string) websocket.Upgrader {
	// Modo desenvolvimento: lista vazia ou wildcard libera tudo.
	allowAll := len(allowedOrigins) == 0 || slices.Contains(allowedOrigins, "*")

	return websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			if allowAll {
				return true
			}
			origin := r.Header.Get("Origin")
			if origin == "" {
				// Non-browser client (curl, server-to-server). Aceita —
				// autenticação JWT continua exigida no handler.
				return true
			}
			if slices.Contains(allowedOrigins, origin) {
				return true
			}
			log.Printf("websocket: rejected connection from disallowed origin %q (allowed=%v)", origin, allowedOrigins)
			return false
		},
	}
}

type WebSocketHandler struct {
	hub      *service.Hub
	upgrader websocket.Upgrader
}

// NewWebSocketHandler cria o handler com o upgrader configurado para
// validar Origin contra allowedOrigins. Passe cfg.CORS.AllowedOrigins.
func NewWebSocketHandler(hub *service.Hub, allowedOrigins []string) *WebSocketHandler {
	return &WebSocketHandler{
		hub:      hub,
		upgrader: buildUpgrader(allowedOrigins),
	}
}

// ServeWS handles websocket requests from the peer.
func (h *WebSocketHandler) ServeWS(c *gin.Context) {
	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Tenant ID is required"})
		return
	}

	userID, err := pkgcontext.GetAccountID(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Account ID is required"})
		return
	}

	role, _ := pkgcontext.GetFunctionalRole(c.Request.Context())

	// O middleware injeta tenant_id no contexto mesmo quando vazio (quando o JWT não tem tenant_id).
	// Para manter consistência entre serviços, fazemos fallback para account_id.
	if tenantID == "" {
		tenantID = userID
	}

	// If the auth middleware accepted a Sec-WebSocket-Protocol token
	// ("Bearer.<jwt>"), gorilla/websocket requires us to echo that
	// subprotocol in the upgrade response — otherwise the client aborts.
	var upgradeHeaders http.Header
	if proto, ok := c.Get("ws_subprotocol"); ok {
		if s, ok := proto.(string); ok && s != "" {
			upgradeHeaders = http.Header{"Sec-WebSocket-Protocol": []string{s}}
		}
	}
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, upgradeHeaders)
	if err != nil {
		log.Println("upgrade error:", err)
		return
	}

	client := &service.Client{
		Hub:      h.hub,
		Conn:     conn,
		Send:     make(chan []byte, 256),
		TenantID: tenantID,
		UserID:   userID,
		Role:     role,
	}

	client.Hub.Register <- client

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.WritePump()
	go client.ReadPump()
}
