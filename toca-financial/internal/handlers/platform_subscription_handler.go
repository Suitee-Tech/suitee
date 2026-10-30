package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"

	"github.com/gin-gonic/gin"
	appctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	ps "github.com/oginoino/toca/toca-financial/internal/domain/platform_subscription"
	"github.com/oginoino/toca/toca-financial/internal/providers"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

const abacatePublicKey = "t9dXRhHHo3yDEj5pVDYz0frf7q6bMKyMRmxxCPIPp3RCplBfXRxqlC6ZpiWmOqj4L63qEaeUOtrCI8P0VMUgo6iIga2ri9ogaHFs0WIIywSMg0q7RmBfybe1E5XJcfC4IW3alNqym0tXoAKkzvfEjZxV6bE0oG2zJrNNYmUCKZyV0KZ3JS8Votf9EAWWYdiDkMkpbMdPggfh1EqHlVkMiTady6jOR3hyzGEHrIz2Ret0xHKMbiqkr9HS1JhNHDX9"

type PlatformSubscriptionHandler struct {
	Service    *service.PlatformSubscriptionService
	Secret     string
	AllowedIPs []string
}

func (h *PlatformSubscriptionHandler) Get(c *gin.Context) {
	tenant, err := appctx.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	state, err := h.Service.Refresh(c.Request.Context(), tenant)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	access, err := h.Service.Access(c.Request.Context(), tenant)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	state, err = h.Service.Repo.Get(c.Request.Context(), tenant)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	// A página mostra a oferta atual do catálogo (preço da primeira cobrança do
	// teste, limites do plano), não o retrato do dia da concessão.
	state = service.CatalogOffer(state)
	payments, err := h.Service.Repo.Payments(c.Request.Context(), tenant)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	usage, err := h.Service.Repo.Usage(c.Request.Context(), tenant)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	response.Success(c, "Assinatura consultada", gin.H{"subscription": state, "access": access, "plans": service.PlatformPlans(), "payments": payments, "usage": usage, "enabled": h.Service.Enabled, "access_control_enabled": h.Service.AccessControlEnabled, "dev_mode": h.Service.DevMode})
}

// Access is intentionally available to every authenticated staff member: the
// application loads it on entry so a new tenant starts its trial before
// reaching a premium action. Tenant identity still comes from verified auth.
func (h *PlatformSubscriptionHandler) Access(c *gin.Context) {
	tenant, err := appctx.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	access, err := h.Service.Access(c.Request.Context(), tenant)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	response.Success(c, "Acesso do plano consultado", access)
}
func (h *PlatformSubscriptionHandler) Select(c *gin.Context) {
	tenant, err := appctx.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req struct {
		Tier  string `json:"tier"`
		Cycle string `json:"cycle"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.ValidationError(c, "Informe o plano e o ciclo de cobrança")
		return
	}
	state, err := h.Service.Select(c.Request.Context(), tenant, req.Tier, req.Cycle)
	if err != nil {
		subscriptionError(c, err)
		return
	}
	response.Success(c, "Plano selecionado", state)
}
func (h *PlatformSubscriptionHandler) Cancel(c *gin.Context) {
	tenant, err := appctx.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	// The reason is optional: the confirmation flow may send one, and an empty
	// body keeps the previous behaviour.
	var req struct {
		Reason string `json:"reason"`
	}
	if c.Request.ContentLength > 0 && c.ShouldBindJSON(&req) != nil {
		response.ValidationError(c, "Motivo de cancelamento inválido")
		return
	}
	if req.Reason != "" && !service.ValidCancellationReason(req.Reason) {
		response.ValidationError(c, "Motivo de cancelamento não reconhecido")
		return
	}
	state, err := h.Service.Cancel(c.Request.Context(), tenant, req.Reason)
	if err != nil {
		subscriptionError(c, err)
		return
	}
	response.Success(c, "Renovação cancelada", state)
}
func (h *PlatformSubscriptionHandler) RevertChange(c *gin.Context) {
	tenant, err := appctx.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	state, err := h.Service.RevertChange(c.Request.Context(), tenant)
	if err != nil {
		subscriptionError(c, err)
		return
	}
	response.Success(c, "Alteração agendada cancelada", state)
}

func (h *PlatformSubscriptionHandler) ChangePaymentMethod(c *gin.Context) {
	tenant, err := appctx.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	state, err := h.Service.ChangePaymentMethodAndCheckout(c.Request.Context(), tenant)
	if err != nil {
		subscriptionError(c, err)
		return
	}
	response.Success(c, "Novo checkout para trocar o cartão", state)
}

// GetPaymentReceipt issues the receipt for one confirmed plan payment.
func (h *PlatformSubscriptionHandler) GetPaymentReceipt(c *gin.Context) {
	ctx := c.Request.Context()
	tenant, err := appctx.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	payment, err := h.Service.Repo.Payment(ctx, tenant, c.Param("id"))
	if err != nil {
		respondInternalError(c, err)
		return
	}
	// Another customer's payment is indistinguishable from a missing one.
	if payment == nil {
		response.NotFound(c, "payment not found")
		return
	}
	state, err := h.Service.Repo.Get(ctx, tenant)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	name, document, _ := h.Service.Repo.TenantIdentity(ctx, tenant)
	receipt := payment.ReceiptURL
	if receipt != "" && !providers.SafeAbacateURL(receipt) {
		receipt = ""
	}
	pdf, err := service.RenderPlatformSubscriptionReceiptPDF(service.PlatformSubscriptionReceiptContext{
		Payment: *payment, Plan: state.Plan, SubscriptionID: payment.SubscriptionID,
		PaymentMethod: state.PaymentMethod, TenantName: name, TenantDocument: document,
		ProviderReceiptURL: receipt, Sandbox: h.Service.DevMode,
	})
	if err != nil {
		respondInternalError(c, err)
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"comprovante-%s.pdf\"", payment.PaidAt.Format("2006-01-02")))
	c.Data(http.StatusOK, "application/pdf", pdf)
}

func subscriptionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ps.ErrInvalidPlan):
		response.ValidationError(c, "Plano ou ciclo inválido. Enterprise exige proposta comercial.")
	case errors.Is(err, ps.ErrConflict):
		// O motivo específico ("o checkout do plano Pro já foi pago", "uso acima
		// dos limites do plano") ia para o lixo e a tela mostrava só o texto
		// genérico — o operador ficava sem saber o que fazer. Quando o serviço
		// anexa um motivo, é ele que aparece.
		message := "A operação não está disponível no estado atual. Atualize a assinatura e verifique o checkout ou o período pago."
		if detalhe := subscriptionDetail(err, ps.ErrConflict); detalhe != "" {
			message = subscriptionSentence(detalhe)
		}
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": message})
	case errors.Is(err, ps.ErrNotFound):
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "O provedor não reconhece mais esta assinatura. Atualize a página; se o estado permanecer, informe o suporte."})
	case errors.Is(err, ps.ErrMethodUnavailable):
		// A loja do provedor não tem cobrança recorrente habilitada: quem resolve
		// é o responsável pela conta no AbacatePay, não o operador da imobiliária.
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "A conta do AbacatePay ainda não tem cobrança recorrente habilitada (cartão ou PIX Automático). Fale com o responsável pela conta para ativar a cobrança de assinaturas."})
	case errors.Is(err, ps.ErrUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Não foi possível confirmar a operação com o AbacatePay. Atualize para recuperar o checkout antes de tentar novamente."})
	default:
		respondInternalError(c, err)
	}
}

// subscriptionDetail extrai o motivo anexado ao erro sentinela pelo serviço
// ("<sentinela>: <motivo>"). Vazio quando o erro é só o sentinela.
func subscriptionDetail(err, sentinel error) string {
	raw := err.Error()
	base := sentinel.Error()
	if !strings.HasPrefix(raw, base) {
		return ""
	}
	return strings.TrimSpace(strings.TrimLeft(strings.TrimPrefix(raw, base), ": "))
}

// subscriptionSentence transforma o motivo interno em frase para a tela:
// primeira letra maiúscula e ponto final.
func subscriptionSentence(detalhe string) string {
	if detalhe == "" {
		return detalhe
	}
	primeira := strings.ToUpper(detalhe[:1])
	frase := primeira + detalhe[1:]
	if !strings.HasSuffix(frase, ".") && !strings.HasSuffix(frase, "!") && !strings.HasSuffix(frase, "?") {
		frase += "."
	}
	return frase
}

func (h *PlatformSubscriptionHandler) Webhook(c *gin.Context) {
	if !h.Service.Enabled || h.Secret == "" {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	if !hmac.Equal([]byte(c.Query("webhookSecret")), []byte(h.Secret)) {
		c.Status(http.StatusUnauthorized)
		return
	}
	if len(h.AllowedIPs) > 0 && !subscriptionIPAllowed(c.ClientIP(), h.AllowedIPs) {
		c.Status(http.StatusForbidden)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 256<<10))
	if err != nil {
		c.Status(http.StatusRequestEntityTooLarge)
		return
	}
	mac := hmac.New(sha256.New, []byte(abacatePublicKey))
	_, _ = mac.Write(raw)
	if !hmac.Equal([]byte(base64.StdEncoding.EncodeToString(mac.Sum(nil))), []byte(c.GetHeader("X-Webhook-Signature"))) {
		c.Status(http.StatusUnauthorized)
		return
	}
	var event ps.Event
	if json.Unmarshal(raw, &event) != nil || event.ID == "" || event.APIVersion != 2 || event.DevMode != h.Service.DevMode {
		c.Status(http.StatusBadRequest)
		return
	}
	if err = h.Service.HandleEvent(c.Request.Context(), event); err != nil {
		// Unknown subscriptions belong to other products in the merchant account.
		if errors.Is(err, ps.ErrNotFound) {
			c.Status(http.StatusOK)
			return
		}
		// Anything else stays retryable. The provider never repeats a 4xx: a
		// payment event dropped here would leave a paid subscription pending
		// forever, while retries are idempotent by event id.
		c.Status(http.StatusServiceUnavailable)
		return
	}
	c.Status(http.StatusOK)
}
func subscriptionIPAllowed(raw string, list []string) bool {
	ip, err := netip.ParseAddr(raw)
	if err != nil {
		return false
	}
	for _, entry := range list {
		entry = strings.TrimSpace(entry)
		if p, err := netip.ParsePrefix(entry); err == nil && p.Contains(ip) {
			return true
		}
		if a, err := netip.ParseAddr(entry); err == nil && a == ip {
			return true
		}
	}
	return false
}
