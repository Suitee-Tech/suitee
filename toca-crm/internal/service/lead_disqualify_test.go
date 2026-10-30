package service

import (
	"context"
	"testing"

	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

// newDisqualifyTestService monta um LeadService mínimo para os testes de
// Disqualify — só o repo e bootstrapping; eventBus nil (publishAsync é
// nil-safe). Reusa o inMemoryAcceptLeadRepo de lead_acceptance_service_test.go.
func newDisqualifyTestService(repo lead.Repository) *LeadService {
	return &LeadService{repo: repo, bootstrapping: map[string]bool{}}
}

func TestDisqualify_Success(t *testing.T) {
	ctx := pkgcontext.WithTenantID(context.Background(), "tenant-A")
	l := &lead.Lead{ID: "lead-1", TenantID: "tenant-A", Name: "Fulano", Status: lead.LeadStatusContacted}
	repo := newAcceptRepo(l)
	svc := newDisqualifyTestService(repo)

	out, err := svc.Disqualify(ctx, "lead-1", "sem renda", "broker-1")
	if err != nil {
		t.Fatalf("Disqualify: %v", err)
	}
	if out.Status != lead.LeadStatusDisqualified {
		t.Errorf("status = %q, want DISQUALIFIED", out.Status)
	}
	found := false
	for _, h := range out.History {
		if h.Action == lead.LeadActionDisqualified && h.Details == "sem renda" {
			found = true
		}
	}
	if !found {
		t.Error("histórico não registrou LeadActionDisqualified com o motivo")
	}
	got, _ := repo.GetByID(ctx, "tenant-A", "lead-1")
	if got == nil || got.Status != lead.LeadStatusDisqualified {
		t.Error("desqualificação não foi persistida no repositório")
	}
}

func TestDisqualify_Idempotent(t *testing.T) {
	ctx := pkgcontext.WithTenantID(context.Background(), "tenant-A")
	l := &lead.Lead{ID: "lead-2", TenantID: "tenant-A", Status: lead.LeadStatusDisqualified}
	svc := newDisqualifyTestService(newAcceptRepo(l))

	out, err := svc.Disqualify(ctx, "lead-2", "duplicado", "broker-1")
	if err != nil {
		t.Fatalf("Disqualify idempotente: %v", err)
	}
	if out.Status != lead.LeadStatusDisqualified {
		t.Errorf("status = %q, want DISQUALIFIED", out.Status)
	}
	if len(out.History) != 0 {
		t.Errorf("lead já desqualificado não deveria gerar novo histórico, veio %d", len(out.History))
	}
}

func TestDisqualify_NotFound(t *testing.T) {
	svc := newDisqualifyTestService(newAcceptRepo())
	if _, err := svc.Disqualify(pkgcontext.WithTenantID(context.Background(), "tenant-A"), "missing", "motivo", "broker-1"); err == nil {
		t.Error("esperava erro para lead inexistente")
	}
}
