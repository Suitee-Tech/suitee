package service

import (
	"context"
	"errors"
	"testing"

	"github.com/oginoino/toca/pkg/clients"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

// Revisão de acesso a imóveis (2026-09-19): recomendar um imóvel que o operador
// NÃO consegue ler precisa falhar. O `GetPropertyByID` vai ao core com o token
// dele — id inexistente ou de outra imobiliária devolve erro. Antes o erro era
// ignorado e o id ficava gravado no lead, deixando o cliente com uma sugestão
// que o portal não conseguia descrever.
type propertyLookupStub struct {
	property *clients.PropertyDTO
	err      error
	calls    int
}

func (s *propertyLookupStub) GetPropertyByID(_ context.Context, _ string) (*clients.PropertyDTO, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.property, nil
}

// SearchProperties completa a interface de leitura de imóveis do serviço; a
// recomendação usa apenas GetPropertyByID.
func (s *propertyLookupStub) SearchProperties(_ context.Context, _ clients.PropertyFilterDTO) ([]clients.PropertyDTO, error) {
	return nil, nil
}

func pkgCtxWithTenant(tenant string) context.Context {
	return pkgcontext.WithTenantID(context.Background(), tenant)
}

func TestSetRecommendedPropertyFailsClosedWhenPropertyUnreadable(t *testing.T) {
	repo := newAcceptRepo(&lead.Lead{ID: "lead-1", TenantID: "tenant-A", Name: "Cliente"})
	// O repositório de aceite do harness resolve o tenant do contexto de teste.
	ctx := pkgCtxWithTenant("tenant-A")

	for _, tc := range []struct {
		name string
		stub *propertyLookupStub
	}{
		{name: "imóvel inexistente ou de outro tenant", stub: &propertyLookupStub{err: errors.New("property not found or access denied")}},
		{name: "sem cliente de imóveis configurado", stub: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &LeadService{repo: repo, bootstrapping: map[string]bool{}}
			if tc.stub != nil {
				svc.propertyClient = tc.stub
			}
			err := svc.SetRecommendedProperty(ctx, "lead-1", "imovel-de-outra-imobiliaria")
			if !errors.Is(err, ErrRecommendedPropertyUnreadable) && tc.stub != nil {
				t.Fatalf("err = %v, want ErrRecommendedPropertyUnreadable", err)
			}
			if err == nil {
				t.Fatal("recomendação de imóvel ilegível foi aceita")
			}
			stored := repo.leads["lead-1"]
			if stored == nil {
				t.Fatal("lead sumiu do repositório")
			}
			if stored.RecommendedPropertyID != "" || stored.RecommendedPropertySnapshot != nil {
				t.Fatalf("id/snapshot gravados apesar da falha: id=%q snapshot=%+v", stored.RecommendedPropertyID, stored.RecommendedPropertySnapshot)
			}
		})
	}
}

func TestSetRecommendedPropertyStoresSnapshotOfReadableProperty(t *testing.T) {
	repo := newAcceptRepo(&lead.Lead{ID: "lead-2", TenantID: "tenant-A", Name: "Cliente"})
	stub := &propertyLookupStub{property: &clients.PropertyDTO{
		ID: "imovel-1", Title: "Apartamento", Neighborhood: "Centro", City: "Curitiba", State: "PR",
		ListingType: "sale", SalePrice: 450000,
	}}
	svc := &LeadService{repo: repo, bootstrapping: map[string]bool{}, propertyClient: stub}

	if err := svc.SetRecommendedProperty(pkgCtxWithTenant("tenant-A"), "lead-2", "imovel-1"); err != nil {
		t.Fatalf("SetRecommendedProperty: %v", err)
	}
	stored := repo.leads["lead-2"]
	if stored.RecommendedPropertyID != "imovel-1" || stored.RecommendedPropertySnapshot == nil {
		t.Fatalf("recomendação não persistida: %+v", stored)
	}
	if stored.RecommendedPropertySnapshot.SalePrice != 450000 || stored.RecommendedPropertySnapshot.ListingType != "sale" {
		t.Fatalf("snapshot sem o valor/finalidade do imóvel: %+v", stored.RecommendedPropertySnapshot)
	}
}
