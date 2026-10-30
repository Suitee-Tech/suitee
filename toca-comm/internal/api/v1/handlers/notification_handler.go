package handlers

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-comm/internal/domain/notification"
	"github.com/oginoino/toca/toca-comm/internal/service"
)

// Force usage of notification package for Swagger
var _ = notification.Notification{}

type NotificationHandler struct {
	service *service.NotificationService
}

func NewNotificationHandler(service *service.NotificationService) *NotificationHandler {
	return &NotificationHandler{service: service}
}

// domainOnly extrai o domínio de um destinatário de e-mail (para o log de
// auditoria). Telefones e outros destinatários devolvem string vazia.
func domainOnly(recipient string) string {
	at := strings.LastIndex(recipient, "@")
	if at < 0 || at == len(recipient)-1 {
		return ""
	}
	return strings.ToLower(recipient[at+1:])
}

type SendNotificationRequest struct {
	Type      notification.Type `json:"type" binding:"required"`
	Recipient string            `json:"recipient" binding:"required"`
	Subject   string            `json:"subject"`
	Content   string            `json:"content" binding:"required"`
	// TenantID é a atribuição da mensagem. Opcional: notificações de sistema
	// (ex.: e-mail de recuperação de senha, pedido antes de haver tenant) não
	// têm tenant. Quando informado, passa pelo resolver — ver Send.
	TenantID string `json:"tenant_id,omitempty"`
}

// Send godoc
// @Summary      Send notification
// @Description  Send a notification to a recipient
// @Tags         notifications
// @Accept       json
// @Produce      json
// @Param        request body handlers.SendNotificationRequest true "Send Notification Request"
// @Success      200  {object}  response.Response{data=notification.Notification}
// @Failure      400  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /comm/internal/notifications/send [post]
//
// SECURITY (achado A3 da revisão de permissões de 2026-09-19): esta rota é um
// primitivo de envio arbitrário de e-mail/WhatsApp/SMS — quem tem a chave
// interna escolhe destinatário, assunto e corpo, ou seja, dispara phishing em
// nome da plataforma e queima crédito de SMS. Ela ficava em
// `POST /api/v1/notifications/send`, caminho que o gateway proxiava
// (`gateway/internal/handlers/comm.go:19-20`) sem nenhum gate de usuário:
// bastava a `INTERNAL_API_KEY` estática, compartilhada por ~10 serviços, para
// alcançá-la pela internet.
//
// Duas mudanças, nesta ordem de importância:
//
//  1. a rota passou para `/api/v1/comm/internal/notifications/send`. O prefixo
//     `/internal/` faz a denylist da borda (P0/C10,
//     `gateway/internal/middleware/internal_paths.go`) responder 404 antes do
//     proxy — a borda deixa de servir o primitivo, e o tráfego legítimo
//     continua indo direto em `COMM_SERVICE_URL` (rede do compose);
//  2. o tenant passa a viajar no corpo. Com JWT de usuário vale o tenant do
//     token; com credencial de serviço vale o `tenant_id` informado, restrito à
//     allowlist da credencial. Sem tenant a mensagem é classificada como
//     SISTEMA — caso do e-mail de recuperação de senha, que é pedido por um
//     usuário ainda sem tenant — e fica registrada como tal no log.
func (h *NotificationHandler) Send(c *gin.Context) {
	var req SendNotificationRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	// Só o domínio do destinatário vai para o log (o e-mail completo não).
	c.Set(ctxNotificationRecipientDomain, domainOnly(req.Recipient))

	// Atribuição da mensagem: resolvida pela credencial quando há tenant; sem
	// tenant só a credencial de serviço passa (sistema).
	_, ok := resolveSendNotificationTenant(c, req.TenantID)
	if !ok {
		return
	}

	n := notification.New(req.Type, req.Recipient, req.Subject, req.Content)

	// In a real system, we might enqueue this for background processing
	if err := h.service.Send(c.Request.Context(), n); err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Notification sent successfully", n)
}
