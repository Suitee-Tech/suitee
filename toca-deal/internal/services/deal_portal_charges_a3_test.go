package services

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// Incidente 2026-09-19 — tela "Financeiro" do portal do cliente:
//
//	GET /api/v1/client-portal/deals/<id>/charges → 403
//
// A causa não era autorização: o toca-deal chamava a rota interna do
// toca-financial SEM `tenant_id`, o serviço de destino resolvia o tenant pela
// credencial (achado A3) e respondia 400; o handler do portal traduzia QUALQUER
// erro em 403 e a cliente lia "acesso negado".
//
// Estes testes travam as três pontas: o tenant do negócio vai na chamada, falha
// do destino NÃO é acesso negado e negócio de terceiro continua 403.
func TestPortalDealChargesForwardsDealTenantToFinancial(t *testing.T) {
	d := &deal.Deal{ID: "deal-charges", TenantID: "11111111-1111-4111-8111-111111111111"}
	base := clients.NewBaseClient("http://financial.test")
	base.InternalAPIKey = "test-key"
	var gotPath string
	var gotQuery url.Values
	base.HTTPClient.Transport = portalVisitTransport(func(r *http.Request) (*http.Response, error) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		return portalChargesResponse(200, `{"data":[{"id":"charge-1","status":"PENDING","amount":100}]}`), nil
	})
	svc := &DealService{
		repo:            portalVisitDeals{newInMemoryDealRepo(d), d},
		financialClient: &clients.FinancialClient{BaseClient: base},
	}

	charges, err := svc.GetMyDealCharges(context.Background(), "account-1", "client@example.test", d.ID)
	if err != nil {
		t.Fatalf("charges: %v", err)
	}
	if len(charges) != 1 || charges[0].ID != "charge-1" {
		t.Fatalf("charges = %+v", charges)
	}
	if gotPath != "/api/v1/financial/internal/deals/"+d.ID+"/charges" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotQuery.Get("tenant_id") != d.TenantID {
		t.Fatalf("tenant_id = %q, want o tenant do negócio (%q)", gotQuery.Get("tenant_id"), d.TenantID)
	}
	if gotQuery.Get("account_id") != "account-1" {
		t.Fatalf("account_id = %q", gotQuery.Get("account_id"))
	}
}

func TestPortalDealChargesUpstreamFailureIsNotAccessDenied(t *testing.T) {
	d := &deal.Deal{ID: "deal-charges", TenantID: "11111111-1111-4111-8111-111111111111"}
	base := clients.NewBaseClient("http://financial.test")
	base.InternalAPIKey = "test-key"
	// 400 é exatamente o que o serviço devolvia quando o tenant não ia na query.
	base.HTTPClient.Transport = portalVisitTransport(func(*http.Request) (*http.Response, error) {
		return portalChargesResponse(400, `{"message":"tenant id required"}`), nil
	})
	svc := &DealService{
		repo:            portalVisitDeals{newInMemoryDealRepo(d), d},
		financialClient: &clients.FinancialClient{BaseClient: base},
	}

	_, err := svc.GetMyDealCharges(context.Background(), "account-1", "client@example.test", d.ID)
	if !errors.Is(err, ErrPortalUpstreamUnavailable) {
		t.Fatalf("err = %v, want ErrPortalUpstreamUnavailable", err)
	}
	if errors.Is(err, ErrPortalAccessDenied) {
		t.Fatal("falha do serviço de destino não pode ser reportada como acesso negado")
	}
}

func TestPortalDealChargesForeignDealStaysAccessDenied(t *testing.T) {
	mine := &deal.Deal{ID: "deal-mine", TenantID: "tenant-1"}
	other := &deal.Deal{ID: "deal-other", TenantID: "tenant-2"}
	base := clients.NewBaseClient("http://financial.test")
	base.InternalAPIKey = "test-key"
	called := false
	base.HTTPClient.Transport = portalVisitTransport(func(*http.Request) (*http.Response, error) {
		called = true
		return portalChargesResponse(200, `{"data":[]}`), nil
	})
	svc := &DealService{
		repo:            portalVisitDeals{newInMemoryDealRepo(mine, other), other},
		financialClient: &clients.FinancialClient{BaseClient: base},
	}

	_, err := svc.GetMyDealCharges(context.Background(), "account-1", "client@example.test", mine.ID)
	if !errors.Is(err, ErrPortalAccessDenied) {
		t.Fatalf("err = %v, want ErrPortalAccessDenied", err)
	}
	if called {
		t.Fatal("negócio de terceiro não pode gerar chamada ao financeiro")
	}
}

// portalChargesResponse monta a resposta HTTP mínima do toca-financial.
func portalChargesResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{},
	}
}
