package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// G32 do plano de inadimplência: o portal do cliente mostrava só `total_amount`
// e o cliente não entendia por que o valor subiu depois do vencimento.
//
// A cobrança do portal passa a discriminar multa, juros e desconto — campos que
// a rota interna do toca-financial já devolve, porque serializa o JSON do
// domínio `charge.Charge` — e a trazer `days_overdue`, que a origem NÃO
// serializa (lá é o método `Charge.DaysOverdue()`), derivado aqui com a mesma
// régua do domínio.
func TestPortalDealChargesCarryDiscriminatedCharges(t *testing.T) {
	d := &deal.Deal{ID: "deal-charges", TenantID: "11111111-1111-4111-8111-111111111111"}
	dueDate := time.Now().Add(-72 * time.Hour).Truncate(time.Second)
	base := clients.NewBaseClient("http://financial.test")
	base.InternalAPIKey = "test-key"
	base.HTTPClient.Transport = portalVisitTransport(func(*http.Request) (*http.Response, error) {
		body := fmt.Sprintf(
			`{"data":[{"id":"charge-1","description":"Aluguel de outubro","status":"OVERDUE","amount":3200,"late_fee":64,"interest":12.8,"discount":0,"total_amount":3276.8,"due_date":%q}]}`,
			dueDate.Format(time.RFC3339),
		)
		return portalChargesResponse(http.StatusOK, body), nil
	})
	svc := &DealService{
		repo:            portalVisitDeals{newInMemoryDealRepo(d), d},
		financialClient: &clients.FinancialClient{BaseClient: base},
	}

	charges, err := svc.GetMyDealCharges(context.Background(), "account-1", "client@example.test", d.ID)
	if err != nil {
		t.Fatalf("charges: %v", err)
	}
	if len(charges) != 1 {
		t.Fatalf("charges = %d, want 1", len(charges))
	}
	got := charges[0]
	if got.Amount != 3200 || got.LateFee != 64 || got.Interest != 12.8 || got.TotalAmount != 3276.8 {
		t.Fatalf("encargos não propagados: %+v", got)
	}
	// O cliente não recebe o que é interno: só o DTO do portal é serializado.
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	payload := string(encoded)
	for _, key := range []string{`"late_fee":64`, `"interest":12.8`, `"discount":0`, `"total_amount":3276.8`, `"days_overdue":3`} {
		if !strings.Contains(payload, key) {
			t.Fatalf("payload do portal = %s, want conter %s", payload, key)
		}
	}
	for _, leaked := range []string{"tenant_id", "payments", "reminders_sent", "applied_event_ids"} {
		if strings.Contains(payload, leaked) {
			t.Fatalf("payload do portal não pode vazar %q: %s", leaked, payload)
		}
	}
}

func TestPortalChargeDaysOverdueMirrorsFinancialDomain(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name       string
		status     string
		dueDate    time.Time
		fromOrigin int
		want       int
	}{
		{name: "vencida há 3 dias", status: "OVERDUE", dueDate: now.Add(-72 * time.Hour), want: 3},
		{name: "parcialmente paga e vencida", status: "PARTIALLY_PAID", dueDate: now.Add(-30 * time.Hour), want: 1},
		{name: "PENDING já vencida (janela antes de marcar OVERDUE)", status: "PENDING", dueDate: now.Add(-30 * time.Hour), want: 1},
		{name: "paga não conta atraso", status: "PAID", dueDate: now.Add(-72 * time.Hour), want: 0},
		{name: "cancelada não conta atraso", status: "CANCELLED", dueDate: now.Add(-72 * time.Hour), want: 0},
		{name: "estornada não conta atraso", status: "REVERSED", dueDate: now.Add(-72 * time.Hour), want: 0},
		{name: "ainda não venceu", status: "PENDING", dueDate: now.Add(48 * time.Hour), want: 0},
		{name: "vence hoje", status: "PENDING", dueDate: now, want: 0},
		{name: "sem vencimento", status: "OVERDUE", dueDate: time.Time{}, want: 0},
		{
			name:       "days_overdue da origem vence a derivação",
			status:     "OVERDUE",
			dueDate:    now.Add(-72 * time.Hour),
			fromOrigin: 42,
			want:       42,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			charge := &clients.Charge{Status: tc.status, DueDate: tc.dueDate, DaysOverdue: tc.fromOrigin}
			if got := portalChargeDaysOverdue(charge, now); got != tc.want {
				t.Fatalf("portalChargeDaysOverdue = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestMapPortalChargesSkipsNilAndKeepsEmptySlice(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	mapped := mapPortalCharges([]*clients.Charge{nil, {ID: "charge-1", Status: "OVERDUE", DueDate: now.Add(-48 * time.Hour)}}, now)
	if len(mapped) != 1 || mapped[0].ID != "charge-1" || mapped[0].DaysOverdue != 2 {
		t.Fatalf("mapped = %+v", mapped)
	}

	empty := mapPortalCharges(nil, now)
	encoded, err := json.Marshal(empty)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(encoded) != "[]" {
		t.Fatalf("lista vazia = %s, want [] (o portal espera array)", encoded)
	}
}
