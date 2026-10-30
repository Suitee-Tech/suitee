package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	"github.com/oginoino/toca/toca-deal/internal/repositories"
)

// N14 (reauditoria de 2026-09-20): a divisão de comissão direciona repasse
// financeiro e os ids vinham do corpo da requisição sem nenhuma conferência — a
// divisão podia apontar para corretor de OUTRA imobiliária ou para um id
// inexistente. Os corretores são do toca-crm e entram pelo caminho
// cross-schema que o serviço já usa para `crm.leads`.

// fakeCrossTenant implementa CrossTenantLookup inteiro para o teste.
type fakeCrossTenant struct {
	brokers map[string]struct{}
	err     error
	asked   []string
	tenants []string
}

func (f *fakeCrossTenant) PropertyOwnerTenant(context.Context, string) (string, error) {
	return "", nil
}

func (f *fakeCrossTenant) LeadOwnerTenant(context.Context, string) (string, error) {
	return "", nil
}

func (f *fakeCrossTenant) ActivePartnershipBetween(context.Context, string, string) (*repositories.CrossPartnership, error) {
	return nil, nil
}

func (f *fakeCrossTenant) BrokersInTenant(_ context.Context, tenantID string, brokerIDs []string) (map[string]struct{}, error) {
	f.tenants = append(f.tenants, tenantID)
	f.asked = append(f.asked, brokerIDs...)
	if f.err != nil {
		return nil, f.err
	}
	return f.brokers, nil
}

func splitService(d *deal.Deal, cross CrossTenantLookup) *DealService {
	return testService(d).WithCrossRepo(cross)
}

// Divisão com um corretor de fora do tenant é recusada, e nada é gravado.
func TestSetBrokerParticipations_RejectsBrokerOutsideTenant(t *testing.T) {
	d := dealWithSplit(nil)
	svc := splitService(d, &fakeCrossTenant{brokers: map[string]struct{}{"broker-cap": {}}})

	_, err := svc.SetBrokerParticipations(context.Background(), d.TenantID, d.ID, "", "actor-1", "Admin", nil, []deal.BrokerParticipation{
		{BrokerID: "broker-cap", Role: deal.BrokerRoleCaptador, Percentage: 60},
		{BrokerID: "broker-de-outro-tenant", Role: deal.BrokerRoleFechamento, Percentage: 40},
	})
	if !errors.Is(err, ErrBrokerNotInTenant) {
		t.Fatalf("corretor fora do tenant: err = %v, quer ErrBrokerNotInTenant", err)
	}
	if !strings.Contains(err.Error(), "broker-de-outro-tenant") {
		t.Fatalf("erro não nomeia o id recusado: %v", err)
	}
	if len(d.BrokerParticipations) != 0 {
		t.Fatalf("divisão recusada não pode ter sido gravada: %+v", d.BrokerParticipations)
	}
}

// Todos os ids no tenant: grava normalmente.
func TestSetBrokerParticipations_AcceptsBrokersInTenant(t *testing.T) {
	d := dealWithSplit(nil)
	cross := &fakeCrossTenant{brokers: map[string]struct{}{"broker-cap": {}, "broker-fec": {}}}
	svc := splitService(d, cross)

	got, err := svc.SetBrokerParticipations(context.Background(), d.TenantID, d.ID, "", "actor-1", "Admin", nil, []deal.BrokerParticipation{
		{BrokerID: "broker-cap", Role: deal.BrokerRoleCaptador, Percentage: 50},
		{BrokerID: "broker-fec", Role: deal.BrokerRoleFechamento, Percentage: 50},
	})
	if err != nil {
		t.Fatalf("corretores do tenant: err inesperado: %v", err)
	}
	if len(got.BrokerParticipations) != 2 {
		t.Fatalf("esperava 2 participações, veio %d", len(got.BrokerParticipations))
	}
	if len(cross.tenants) == 0 || cross.tenants[0] != d.TenantID {
		t.Fatalf("consulta feita com tenant errado: %+v", cross.tenants)
	}
}

// Divisão só com o papel `imobiliaria` (sem broker_id) não tem id a validar.
func TestSetBrokerParticipations_AllowsAgencyOnlySplit(t *testing.T) {
	d := dealWithSplit(nil)
	cross := &fakeCrossTenant{}
	svc := splitService(d, cross)

	if _, err := svc.SetBrokerParticipations(context.Background(), d.TenantID, d.ID, "", "actor-1", "Admin", nil, []deal.BrokerParticipation{
		{Role: deal.BrokerRoleImobiliaria, Percentage: 100},
	}); err != nil {
		t.Fatalf("divisão da imobiliária: err inesperado: %v", err)
	}
	if len(cross.asked) != 0 {
		t.Fatalf("não deveria consultar corretores sem id: %+v", cross.asked)
	}
}

// Falha de LEITURA propaga (não vira "corretor inválido"): sem saber se o
// corretor existe, não se grava destinação de repasse — e o chamador recebe
// erro de servidor, não 4xx.
func TestSetBrokerParticipations_ReadFailurePropagates(t *testing.T) {
	d := dealWithSplit(nil)
	svc := splitService(d, &fakeCrossTenant{err: errors.New("banco indisponível")})

	_, err := svc.SetBrokerParticipations(context.Background(), d.TenantID, d.ID, "", "actor-1", "Admin", nil, []deal.BrokerParticipation{
		{BrokerID: "broker-cap", Role: deal.BrokerRoleCaptador, Percentage: 100},
	})
	if err == nil {
		t.Fatal("falha de leitura deveria propagar")
	}
	if errors.Is(err, ErrBrokerNotInTenant) {
		t.Fatalf("falha de leitura não pode virar erro do chamador: %v", err)
	}
}

// Sem repositório cross injetado a checagem é pulada — mesmo contrato das
// outras checagens cross-tenant do serviço (e o que mantém os testes existentes
// válidos).
func TestSetBrokerParticipations_SkipsValidationWithoutCrossRepo(t *testing.T) {
	d := dealWithSplit(nil)
	svc := testService(d)

	if _, err := svc.SetBrokerParticipations(context.Background(), d.TenantID, d.ID, "", "actor-1", "Admin", nil, []deal.BrokerParticipation{
		{BrokerID: "qualquer-id", Role: deal.BrokerRoleCaptador, Percentage: 100},
	}); err != nil {
		t.Fatalf("sem crossRepo: err inesperado: %v", err)
	}
}
