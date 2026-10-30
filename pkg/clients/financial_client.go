package clients

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Charge struct {
	ID             string     `json:"id"`
	Description    string     `json:"description"`
	ReferenceMonth string     `json:"reference_month"`
	Amount         float64    `json:"amount"`
	TotalAmount    float64    `json:"total_amount"`
	DueDate        time.Time  `json:"due_date"`
	Status         string     `json:"status"`
	PaymentURL     string     `json:"payment_url,omitempty"`
	PaidAt         *time.Time `json:"paid_at,omitempty"`
	// Encargos discriminados da cobrança (G32 do plano de inadimplência).
	//
	// A rota interna do toca-financial devolve o JSON do domínio
	// `charge.Charge` direto (SELECT metadata → unmarshal → response.Success),
	// então `late_fee`, `interest` e `discount` chegam prontos: não há cálculo
	// de encargo do lado do toca-deal.
	LateFee  float64 `json:"late_fee"`
	Interest float64 `json:"interest"`
	Discount float64 `json:"discount"`
	// DaysOverdue NÃO é serializado pela origem hoje: o domínio do
	// toca-financial expõe apenas o método `Charge.DaysOverdue()`, e método não
	// entra no JSON. O campo fica aqui para capturar o valor no dia em que a
	// origem passar a expô-lo; enquanto isso o mapper do portal deriva o atraso
	// de DueDate + Status com a mesma régua (ver portalChargeDaysOverdue em
	// toca-deal/internal/services/deal_portal_service.go).
	DaysOverdue int `json:"days_overdue"`
}

// ErrFinancialTenantRequired é devolvido antes de sair da rede quando o chamador
// não informa o tenant.
//
// SECURITY (achado A3 da revisão de permissões de 2026-09-19): a rota interna do
// toca-financial resolve o tenant pela CREDENCIAL
// (`auth.ResolveTenantForInternalRequest`) e, para chamada de serviço, exige o
// valor explícito. Falhar aqui, com mensagem própria, evita o ciclo quebrado que
// existia: a chamada saía sem tenant, o serviço respondia 400 e o handler do
// portal traduzia QUALQUER erro em 403 — a tela Financeiro do cliente dizia
// "acesso negado" quando o defeito era o parâmetro ausente.
var ErrFinancialTenantRequired = errors.New("financial: tenant_id is required for internal calls")

type FinancialClient struct {
	BaseClient *BaseClient
}

func NewFinancialClient() *FinancialClient {
	return &FinancialClient{
		BaseClient: NewBaseClient(os.Getenv("FINANCIAL_SERVICE_URL")),
	}
}

// ListChargesByCustomer lists charges by dealID, customerAccountID and role.
//
// `tenantID` é o tenant do NEGÓCIO (o mesmo que o toca-deal já resolveu em
// `findMyDeal`), não um valor vindo do navegador: o serviço de destino o valida
// contra a credencial e recusa qualquer outro.
func (c *FinancialClient) ListChargesByCustomer(ctx context.Context, tenantID, dealID, accountID, role string) ([]*Charge, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, ErrFinancialTenantRequired
	}
	query := url.Values{"tenant_id": {tenantID}}
	if accountID != "" {
		query.Set("account_id", accountID)
	}
	if role != "" {
		query.Set("role", role)
	}
	return c.listCharges(ctx, "/api/v1/financial/internal/deals/"+url.PathEscape(dealID)+"/charges?"+query.Encode())
}

// ListChargesByDeal lists all charges for a deal (visão de operador, mesmo
// tenant do negócio).
func (c *FinancialClient) ListChargesByDeal(ctx context.Context, tenantID, dealID string) ([]*Charge, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, ErrFinancialTenantRequired
	}
	query := url.Values{"tenant_id": {tenantID}}
	return c.listCharges(ctx, "/api/v1/financial/internal/deals/"+url.PathEscape(dealID)+"/charges?"+query.Encode())
}

// SalePlanReadiness é a resposta de GET
// /financial/internal/deals/:id/sale-plan-readiness. Active=false significa
// "nenhum draft ativo" (o operador ainda não configurou a forma de
// recebimento); Active=true traz o total congelado e o modo para o chamador
// conferir contra o valor do contrato.
type SalePlanReadiness struct {
	Active           bool   `json:"active"`
	TotalAmountCents int64  `json:"total_amount_cents"`
	Mode             string `json:"mode"`
}

// GetSalePlanReadiness consulta o estado do plano SALE pré-contrato de um
// negócio. Consumido pelo toca-deal (T-01) para recusar, no servidor,
// contratos de venda gerados/enviados sem forma de recebimento compatível —
// o mesmo guard que a esteira aplica no frontend.
func (c *FinancialClient) GetSalePlanReadiness(ctx context.Context, tenantID, dealID string) (*SalePlanReadiness, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, ErrFinancialTenantRequired
	}
	query := url.Values{"tenant_id": {tenantID}}
	path := "/api/v1/financial/internal/deals/" + url.PathEscape(dealID) + "/sale-plan-readiness?" + query.Encode()

	resp, err := c.BaseClient.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("financial service returned status: %d", resp.StatusCode)
	}

	var body struct {
		Data SalePlanReadiness `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	return &body.Data, nil
}

func (c *FinancialClient) listCharges(ctx context.Context, path string) ([]*Charge, error) {
	resp, err := c.BaseClient.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("financial service returned status: %d", resp.StatusCode)
	}

	var response struct {
		Data []*Charge `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return response.Data, nil
}
