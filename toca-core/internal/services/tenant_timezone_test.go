package services

import (
	"context"
	"errors"
	"testing"

	"github.com/oginoino/toca-core/internal/domain/core/settings"
)

// fakeCompanyRepo devolve o documento que o teste montar (ou nil).
type fakeCompanyRepo struct {
	doc *settings.CompanySettings
}

func (r *fakeCompanyRepo) GetByTenantID(_ context.Context, tenantID string) (*settings.CompanySettings, error) {
	if r.doc == nil {
		return nil, nil
	}
	if r.doc.TenantID != tenantID {
		return nil, nil
	}
	return r.doc, nil
}
func (r *fakeCompanyRepo) Upsert(_ context.Context, s *settings.CompanySettings) error {
	r.doc = s
	return nil
}

// O fuso da imobiliária (Configurações → "Fuso horário") é o que os fluxos usam
// para montar dias civis: lançamento às 22h em Brasília pertence ao MESMO dia,
// não ao dia UTC seguinte.
func TestTenantTimezoneDefaultsAndPersistence(t *testing.T) {
	ctx := context.Background()

	// Sem documento: padrão do produto (Brasília), nunca vazio.
	svc := NewSettingsService(&fakeCompanyRepo{}, nil, nil, nil, nil, nil, nil)
	if got, err := svc.TenantTimezone(ctx, "tenant-1"); err != nil || got != DefaultTimezone {
		t.Fatalf("sem configuração: %q err=%v, want %s", got, err, DefaultTimezone)
	}

	// Com configuração do operador: devolve o que ele escolheu.
	repo := &fakeCompanyRepo{doc: &settings.CompanySettings{TenantID: "tenant-1", Timezone: "America/Manaus"}}
	svc = NewSettingsService(repo, nil, nil, nil, nil, nil, nil)
	if got, err := svc.TenantTimezone(ctx, "tenant-1"); err != nil || got != "America/Manaus" {
		t.Fatalf("configurado: %q err=%v, want America/Manaus", got, err)
	}
	// Outro tenant não herda a configuração alheia.
	if got, _ := svc.TenantTimezone(ctx, "tenant-2"); got != DefaultTimezone {
		t.Fatalf("tenant sem configuração = %q, want %s", got, DefaultTimezone)
	}
}

// Fuso inexistente é recusado na gravação: aceitar em silêncio faria o serviço
// cair no padrão e o operador achar que configurou.
func TestUpdateCompanySettingsRejectsUnknownTimezone(t *testing.T) {
	repo := &fakeCompanyRepo{}
	svc := NewSettingsService(repo, nil, nil, nil, nil, nil, nil)

	err := svc.UpdateCompanySettings(context.Background(), "tenant-1", &settings.CompanySettings{Timezone: "Marte/Olympus"})
	if !errors.Is(err, ErrInvalidTimezone) {
		t.Fatalf("err = %v, want ErrInvalidTimezone", err)
	}

	if err := svc.UpdateCompanySettings(context.Background(), "tenant-1", &settings.CompanySettings{Timezone: "America/Sao_Paulo"}); err != nil {
		t.Fatalf("fuso válido recusado: %v", err)
	}
	if repo.doc == nil || repo.doc.Timezone != "America/Sao_Paulo" {
		t.Fatalf("não persistiu o fuso: %+v", repo.doc)
	}

	// Vazio continua permitido (volta ao padrão do ambiente).
	if err := svc.UpdateCompanySettings(context.Background(), "tenant-1", &settings.CompanySettings{}); err != nil {
		t.Fatalf("vazio deveria ser aceito: %v", err)
	}
}
