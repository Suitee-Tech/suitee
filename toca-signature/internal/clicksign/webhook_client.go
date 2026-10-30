package clicksign

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Webhooks na Clicksign.
//
// O provedor só entrega eventos para uma URL registrada — e valida que ela
// responde antes de aceitar o cadastro (um POST com URL inalcançável devolve
// 400 "url não está disponível"). Sem o registro, nenhum webhook chega: foi o
// que aconteceu em produção, com `signature.webhook_events` vazio e um envelope
// já finalizado seguindo como RUNNING do nosso lado.
//
// O cadastro é feito por comando, não na subida do serviço: registrar é ato
// operacional, precisa de uma URL pública válida e não deve depender de o
// serviço conseguir alcançar o provedor para iniciar.

// WebhookResource é o formato devolvido pela API v3.
type WebhookResource struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"`
	Attributes WebhookAttributes `json:"attributes"`
}

type WebhookAttributes struct {
	URL       string   `json:"url"`
	Events    []string `json:"events,omitempty"`
	CreatedAt string   `json:"created_at,omitempty"`
}

// ListWebhooks devolve os webhooks já cadastrados para o tenant.
func (c *Client) ListWebhooks(ctx context.Context, tenantID string) ([]WebhookResource, error) {
	tc, err := c.httpFor(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	// A lista usa o mesmo envelope JSON:API das demais respostas, então o
	// decodificador genérico de `data` já serve.
	lista, err := doJSON[[]WebhookResource](ctx, tc.http, http.MethodGet, "/api/v3/webhooks", nil)
	if err != nil {
		return nil, err
	}
	return *lista, nil
}

// CreateWebhook cadastra a URL de notificação.
//
// A Clicksign recusa com 400 quando a URL não responde, então uma falha aqui
// costuma significar "o endpoint ainda não está publicado" e não "o payload
// está errado" — a mensagem é repassada para o operador distinguir os casos.
func (c *Client) CreateWebhook(ctx context.Context, tenantID, url string) (*WebhookResource, error) {
	tc, err := c.httpFor(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	body := wrap("webhooks", "", WebhookAttributes{URL: strings.TrimSpace(url)})
	return doJSON[WebhookResource](ctx, tc.http, http.MethodPost, "/api/v3/webhooks", body)
}

// EnsureWebhook garante que a URL esteja cadastrada e devolve se precisou criar.
//
// Idempotente por URL: rodar de novo não duplica o cadastro.
func (c *Client) EnsureWebhook(ctx context.Context, tenantID, url string) (created bool, existing *WebhookResource, err error) {
	target := strings.TrimSpace(url)
	if target == "" {
		return false, nil, errors.New("webhook url is required")
	}

	webhooks, err := c.ListWebhooks(ctx, tenantID)
	if err != nil {
		return false, nil, err
	}
	for i := range webhooks {
		if strings.EqualFold(strings.TrimSpace(webhooks[i].Attributes.URL), target) {
			return false, &webhooks[i], nil
		}
	}

	novo, err := c.CreateWebhook(ctx, tenantID, target)
	if err != nil {
		// A Clicksign valida que a URL responde antes de aceitar o cadastro.
		return false, nil, fmt.Errorf("cadastrar webhook %s: %w", target, err)
	}
	return true, novo, nil
}
