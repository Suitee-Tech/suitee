package service

import (
	"context"
	"testing"

	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

// P0 / C9 — isolamento de tenant na superfície de leads.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado C9) mostrou que o repositório de leads buscava e gravava por `id` sem
// `tenant_id`. Consequência: um usuário com `leads:write` do tenant A lia e
// sobrescrevia leads do tenant B conhecendo o UUID.
//
// Estes testes expressam a propriedade de segurança no nível do serviço, que é
// a fronteira chamada pelos handlers HTTP. Eles são escritos contra a API
// pública existente (mesma assinatura antes e depois da correção), de modo que
// a evidência vermelho→verde seja real e não um artefato de compilação.
//
// Chamador e vítima ficam em tenants diferentes; o contexto carrega o tenant do
// chamador (é o que o middleware de JWT faz em produção).

const (
	p0TenantAttacker = "tenant-atacante"
	p0TenantVictim   = "tenant-vitima"
	p0ForeignLeadID  = "lead-do-outro-tenant"
)

func p0ForeignLead() *lead.Lead {
	return &lead.Lead{
		ID:       p0ForeignLeadID,
		TenantID: p0TenantVictim,
		Name:     "Titular de outro tenant",
		Email:    "vitima@example.com",
		Phone:    "11999999999",
		Status:   lead.LeadStatusNew,
	}
}

// ctx do atacante: tenant diferente do dono do lead.
func p0AttackerCtx() context.Context {
	return pkgcontext.WithTenantID(context.Background(), p0TenantAttacker)
}

// TestLeadService_GetByID_DeniesForeignTenant prova que a leitura por id
// respeita o tenant do chamador.
func TestLeadService_GetByID_DeniesForeignTenant(t *testing.T) {
	repo := newAcceptRepo(p0ForeignLead())
	svc := newDisqualifyTestService(repo)

	got, err := svc.GetByID(p0AttackerCtx(), p0ForeignLeadID)

	if err == nil && got != nil {
		t.Fatalf("VAZAMENTO CROSS-TENANT: chamador de %q leu o lead %q do tenant %q (nome=%q, email=%q)",
			p0TenantAttacker, got.ID, got.TenantID, got.Name, got.Email)
	}
	if got != nil {
		t.Fatalf("lead devolvido junto com erro: %+v", got)
	}
}

// TestLeadService_GetByID_AllowsOwnTenant garante que a correção não quebra o
// caminho legítimo (mesmo tenant).
func TestLeadService_GetByID_AllowsOwnTenant(t *testing.T) {
	own := &lead.Lead{ID: "lead-proprio", TenantID: p0TenantAttacker, Name: "Próprio", Status: lead.LeadStatusNew}
	repo := newAcceptRepo(own)
	svc := newDisqualifyTestService(repo)

	got, err := svc.GetByID(pkgcontext.WithTenantID(context.Background(), p0TenantAttacker), "lead-proprio")
	if err != nil {
		t.Fatalf("leitura legítima falhou: %v", err)
	}
	if got == nil || got.ID != "lead-proprio" {
		t.Fatalf("esperava o lead do próprio tenant, veio %+v", got)
	}
}

// TestLeadService_Disqualify_DeniesForeignTenant prova que uma mutação em lead
// de outro tenant é recusada e não altera a linha da vítima.
func TestLeadService_Disqualify_DeniesForeignTenant(t *testing.T) {
	victim := p0ForeignLead()
	repo := newAcceptRepo(victim)
	svc := newDisqualifyTestService(repo)

	out, err := svc.Disqualify(p0AttackerCtx(), p0ForeignLeadID, "motivo do atacante", "atacante-1")

	if err == nil {
		t.Fatalf("ESCRITA CROSS-TENANT PERMITIDA: Disqualify aceitou lead do tenant %q (retorno=%+v)", p0TenantVictim, out)
	}
	if stored := repo.leads[p0ForeignLeadID]; stored.Status != lead.LeadStatusNew {
		t.Fatalf("ESCRITA CROSS-TENANT PERSISTIDA: status da vítima virou %q", stored.Status)
	}
}

// TestLeadService_SetRecommendedProperty_DeniesForeignTenant cobre outro
// caminho de escrita que carregava por id sem tenant.
func TestLeadService_SetRecommendedProperty_DeniesForeignTenant(t *testing.T) {
	victim := p0ForeignLead()
	repo := newAcceptRepo(victim)
	svc := newDisqualifyTestService(repo)

	err := svc.SetRecommendedProperty(p0AttackerCtx(), p0ForeignLeadID, "imovel-do-atacante")

	if err == nil {
		t.Fatalf("ESCRITA CROSS-TENANT PERMITIDA: SetRecommendedProperty aceitou lead do tenant %q", p0TenantVictim)
	}
	if stored := repo.leads[p0ForeignLeadID]; stored.RecommendedPropertyID != "" {
		t.Fatalf("ESCRITA CROSS-TENANT PERSISTIDA: RecommendedPropertyID=%q", stored.RecommendedPropertyID)
	}
}

// TestLeadService_RegisterFirstContact_DeniesForeignTenant cobre o caminho de
// primeiro contato (também carregava por id sem tenant).
func TestLeadService_RegisterFirstContact_DeniesForeignTenant(t *testing.T) {
	victim := p0ForeignLead()
	repo := newAcceptRepo(victim)
	svc := newDisqualifyTestService(repo)

	if _, _, err := svc.RegisterFirstContact(p0AttackerCtx(), p0ForeignLeadID); err == nil {
		t.Fatalf("ESCRITA CROSS-TENANT PERMITIDA: RegisterFirstContact aceitou lead do tenant %q", p0TenantVictim)
	}
	if stored := repo.leads[p0ForeignLeadID]; stored.FirstContactAt != nil {
		t.Fatal("ESCRITA CROSS-TENANT PERSISTIDA: FirstContactAt preenchido na vítima")
	}
}

// TestLeadService_Delete_DeniesForeignTenant prova que a exclusão respeita o
// tenant do chamador.
func TestLeadService_Delete_DeniesForeignTenant(t *testing.T) {
	repo := newAcceptRepo(p0ForeignLead())
	svc := newDisqualifyTestService(repo)

	if err := svc.Delete(p0AttackerCtx(), p0ForeignLeadID); err == nil {
		t.Fatalf("EXCLUSÃO CROSS-TENANT PERMITIDA para o tenant %q", p0TenantVictim)
	}
	if _, still := repo.leads[p0ForeignLeadID]; !still {
		t.Fatal("EXCLUSÃO CROSS-TENANT PERSISTIDA: lead da vítima removido")
	}
}
