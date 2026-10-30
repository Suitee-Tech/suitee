package services

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-portal/internal/domain/integration"
	"github.com/oginoino/toca/toca-portal/internal/domain/lead"
)

// Sentinela do webhook público (C5). O handler traduz cada um para o status
// HTTP correto; o detalhe do erro nunca vai para o chamador anônimo.
var (
	// ErrSignatureRequired: header X-Hub-Signature ausente → 401.
	ErrSignatureRequired = errors.New("webhook signature header is required")
	// ErrSignatureInvalid: assinatura presente e inválida → 401.
	ErrSignatureInvalid = errors.New("invalid webhook signature")
	// ErrWebhookSecretMissing: integração sem webhook_secret configurado.
	// Falha FECHADO: sem segredo não existe requisição válida → 500.
	ErrWebhookSecretMissing = errors.New("integration has no webhook secret configured")
	// ErrIntegrationInactive: integração desconectada/em erro não ingere lead → 403.
	ErrIntegrationInactive = errors.New("integration is not active")
	// ErrWebhookRateLimited: rajada acima do limite por integração → 429.
	ErrWebhookRateLimited = errors.New("webhook rate limit exceeded for integration")
	// ErrInvalidPayload: corpo não é JSON do payload de lead, ou sem
	// email e telefone → 400.
	ErrInvalidPayload = errors.New("invalid webhook payload")
)

// defaultWebhookRatePerMin é o limite por integração (eventos/minuto).
const defaultWebhookRatePerMin = 60.0

type WebhookService struct {
	leadRepo      lead.Repository
	integRepo     integration.Repository
	eventBus      events.EventBus
	crmServiceURL string
	internalKey   string
	httpClient    *http.Client
	limiter       *webhookRateLimiter
}

func NewWebhookService(leadRepo lead.Repository, integRepo integration.Repository, eventBus events.EventBus, crmServiceURL, internalKey string) *WebhookService {
	return &WebhookService{
		leadRepo:      leadRepo,
		integRepo:     integRepo,
		eventBus:      eventBus,
		crmServiceURL: crmServiceURL,
		internalKey:   internalKey,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		limiter: newWebhookRateLimiter(),
	}
}

// webhookRateLimiter é um token bucket por integração, em processo (mesmo
// desenho do adminRateLimiter de pkg/auth). O limite é defesa em profundidade
// sobre a assinatura HMAC: protege o Postgres e o toca-crm de rajadas com
// assinatura válida. Default 60/min por integração, ajustável por
// PORTAL_WEBHOOK_RATE_LIMIT_PER_MIN.
type webhookRateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*webhookBucket
	capacity float64
	refill   float64 // tokens por segundo
}

type webhookBucket struct {
	tokens     float64
	lastRefill time.Time
}

func newWebhookRateLimiter() *webhookRateLimiter {
	perMin := defaultWebhookRatePerMin
	if v := strings.TrimSpace(os.Getenv("PORTAL_WEBHOOK_RATE_LIMIT_PER_MIN")); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil && n > 0 {
			perMin = n
		}
	}
	return &webhookRateLimiter{
		buckets:  map[string]*webhookBucket{},
		capacity: perMin,
		refill:   perMin / 60.0,
	}
}

// allow devolve true enquanto a integração tiver tokens.
func (r *webhookRateLimiter) allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	b, ok := r.buckets[key]
	if !ok {
		b = &webhookBucket{tokens: r.capacity, lastRefill: now}
		r.buckets[key] = b
	}
	if elapsed := now.Sub(b.lastRefill).Seconds(); elapsed > 0 {
		b.tokens += elapsed * r.refill
		if b.tokens > r.capacity {
			b.tokens = r.capacity
		}
		b.lastRefill = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// WebhookLeadPayload represents the expected payload from portal webhooks
type WebhookLeadPayload struct {
	Name            string `json:"name"`
	Email           string `json:"email"`
	Phone           string `json:"phone"`
	Message         string `json:"message,omitempty"`
	ListingID       string `json:"listing_id,omitempty"`
	PropertyID      string `json:"property_id,omitempty"`
	PortalListingID string `json:"portal_listing_id,omitempty"`
}

// ValidateHMAC validates the webhook HMAC signature
func ValidateHMAC(body []byte, signature, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expectedMAC := hex.EncodeToString(mac.Sum(nil))
	expectedSig := "sha256=" + expectedMAC
	return hmac.Equal([]byte(expectedSig), []byte(signature))
}

// ProcessWebhook handles an incoming lead from a portal webhook.
//
// Endpoint público (sem JWT): a assinatura HMAC é OBRIGATÓRIA (C5). Ordem das
// checagens — assinatura presente → integração → segredo configurado →
// assinatura válida → status ativo → limite por integração → ingestão. Tudo que
// não passa devolve um erro sentinela e NADA é persistido; o detalhe do erro
// fica no log, nunca na resposta.
func (s *WebhookService) ProcessWebhook(ctx context.Context, integrationID string, rawBody []byte, signature string) (*lead.PortalLead, error) {
	// 1. Header obrigatório ANTES de qualquer trabalho: requisição sem
	// assinatura não é autenticável, ponto. (Antes: `signature != ""` pulava a
	// verificação inteira e o lead era gravado.)
	if strings.TrimSpace(signature) == "" {
		return nil, ErrSignatureRequired
	}

	// 2. Get integration — caminho público, sem tenant no contexto: o tenant
	// sai do próprio registro (escotilha explícita GetByIDAnyTenant).
	integ, err := s.integRepo.GetByIDAnyTenant(ctx, integrationID)
	if err != nil {
		if errors.Is(err, integration.ErrNotFound) {
			return nil, integration.ErrNotFound
		}
		return nil, fmt.Errorf("load integration: %w", err)
	}

	// 3. Segredo ausente falha FECHADO: sem segredo não existe assinatura
	// válida. (Antes, segredo vazio + assinatura vazia aceitava tudo; e uma
	// assinatura calculada com chave vazia é trivial de forjar.)
	if strings.TrimSpace(integ.WebhookSecret) == "" {
		log.Printf("[WebhookService] SECURITY: integration %s (tenant %s) has no webhook_secret; rejecting lead (fail-closed)", integ.ID, integ.TenantID)
		return nil, ErrWebhookSecretMissing
	}

	// 4. Assinatura sempre validada.
	if !ValidateHMAC(rawBody, signature, integ.WebhookSecret) {
		return nil, ErrSignatureInvalid
	}

	// 5. Integração precisa estar ativa para ingerir lead.
	if integ.Status != integration.IntegrationActive {
		return nil, fmt.Errorf("%w (status=%s)", ErrIntegrationInactive, integ.Status)
	}

	// 6. Limite por integração, depois da assinatura: rajada anônima não
	// consome o bucket (não dá para derrubar o portal legítimo) e rajada
	// autenticada não chega ao Postgres/CRM.
	if s.limiter != nil && !s.limiter.allow(integ.ID) {
		return nil, ErrWebhookRateLimited
	}

	// 7. Parse payload
	var payload WebhookLeadPayload
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPayload, err)
	}

	if payload.Email == "" && payload.Phone == "" {
		return nil, fmt.Errorf("%w: lead must have at least email or phone", ErrInvalidPayload)
	}

	// 8. Create portal lead record
	portalLead := lead.NewPortalLead(
		integ.TenantID,
		integ.ID,
		integ.Portal,
		string(rawBody),
		payload.Name,
		payload.Email,
		payload.Phone,
		payload.Message,
		payload.PortalListingID,
	)

	// Set property ID from listing ID or property_id
	if payload.PropertyID != "" {
		portalLead.PropertyID = payload.PropertyID
	} else if payload.ListingID != "" {
		portalLead.PropertyID = payload.ListingID
	}

	if err := s.leadRepo.Create(ctx, portalLead); err != nil {
		return nil, fmt.Errorf("failed to store portal lead: %w", err)
	}

	// 9. Check for duplicates
	if payload.Email != "" {
		existing, err := s.leadRepo.FindDuplicate(ctx, integ.TenantID, string(integ.Portal), payload.Email, payload.Phone, portalLead.PropertyID)
		if err == nil && existing != nil && existing.ID != portalLead.ID {
			portalLead.MarkDuplicate()
			_ = s.leadRepo.Update(ctx, portalLead)
			log.Printf("[WebhookService] Duplicate lead detected for email %s, tenant %s", payload.Email, integ.TenantID)
			return portalLead, nil
		}
	}

	// 10. Create lead in toca-crm via HTTP
	crmLeadID, err := s.createCRMLead(ctx, integ, payload)
	if err != nil {
		portalLead.MarkFailed(err.Error())
		_ = s.leadRepo.Update(ctx, portalLead)
		log.Printf("[WebhookService] Failed to create CRM lead: %v", err)
		return portalLead, nil // Don't return error, lead is stored
	}

	portalLead.MarkProcessed(crmLeadID)
	_ = s.leadRepo.Update(ctx, portalLead)

	// 11. Publish portal.lead_received event for real-time notification
	if s.eventBus != nil {
		evt := PortalLeadReceivedEvent{
			PortalLeadID: portalLead.ID,
			CRMLeadID:    crmLeadID,
			TenantID:     integ.TenantID,
			Portal:       string(integ.Portal),
			LeadName:     payload.Name,
			LeadEmail:    payload.Email,
			LeadPhone:    payload.Phone,
			PropertyID:   portalLead.PropertyID,
			Timestamp:    time.Now(),
		}
		if err := s.eventBus.Publish(ctx, evt); err != nil {
			log.Printf("[WebhookService] Failed to publish portal.lead_received event: %v", err)
		}
	}

	log.Printf("[WebhookService] Successfully processed portal lead %s -> CRM lead %s", portalLead.ID, crmLeadID)
	return portalLead, nil
}

// createCRMLead calls toca-crm to create a lead
func (s *WebhookService) createCRMLead(ctx context.Context, integ *integration.PortalIntegration, payload WebhookLeadPayload) (string, error) {
	// Map portal type to lead source
	source := mapPortalToLeadSource(integ.Portal)

	crmPayload := map[string]interface{}{
		"name":   payload.Name,
		"email":  payload.Email,
		"phone":  payload.Phone,
		"source": source,
	}

	body, err := json.Marshal(crmPayload)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/api/v1/leads", s.crmServiceURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("X-Internal-Key", s.internalKey)
	req.Header.Set("X-Tenant-Id", integ.TenantID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call toca-crm: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("toca-crm returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode CRM response: %w", err)
	}

	return result.Data.ID, nil
}

func mapPortalToLeadSource(portal integration.PortalType) string {
	switch portal {
	case integration.PortalZAP:
		return "ZAP_IMOVEIS"
	case integration.PortalOLX:
		return "OLX_GRUPO_ZAP"
	case integration.PortalVivaReal:
		return "VIVAREAL"
	default:
		return "OTHER"
	}
}
