package services_test

import (
	"context"
	"strings"
	"testing"

	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-deal/internal/domain/document"
	service "github.com/oginoino/toca/toca-deal/internal/services"
)

// P0 / C11 — isolamento de tenant no serviço de documentos.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado C11) mostrou que `GET /api/v1/documents/:id` era a única rota da
// família sem checagem de tenant: lia nome, notas, tags, signatários (nome +
// e-mail) e a storage key de documentos de qualquer tenant pelo UUID.

func documentForTenant(tenantID string) *service.DocumentService {
	repo := &fakeDocumentRepo{
		docs: []*document.Document{{
			ID:         "doc-da-vitima",
			TenantID:   tenantID,
			Name:       "Contrato confidencial",
			Notes:      "notas internas",
			Tags:       []string{"sigiloso"},
			StorageKey: tenantID + "/contracts/contrato.pdf",
			FileURL:    "https://storage.example/" + tenantID + "/contrato.pdf",
		}},
	}
	return service.NewDocumentService(repo, nil)
}

func TestDocumentService_GetByID_DeniesForeignTenant(t *testing.T) {
	svc := documentForTenant("tenant-vitima")

	got, err := svc.GetByID(context.Background(), "tenant-atacante", "doc-da-vitima")
	if err == nil {
		t.Fatalf("VAZAMENTO CROSS-TENANT: documento do tenant vitima devolvido para o atacante: %+v", got)
	}
	if got != nil {
		t.Fatalf("documento devolvido junto com erro: %+v", got)
	}
}

func TestDocumentService_GetByID_FailsClosedWithoutTenant(t *testing.T) {
	svc := documentForTenant("tenant-vitima")

	for _, tenantID := range []string{"", "   "} {
		if _, err := svc.GetByID(context.Background(), tenantID, "doc-da-vitima"); err == nil {
			t.Fatalf("tenant %q deveria ser recusado (fail-closed), não buscar globalmente", tenantID)
		}
	}
}

func TestDocumentService_GetByID_AllowsOwnTenant(t *testing.T) {
	svc := documentForTenant("tenant-vitima")

	got, err := svc.GetByID(context.Background(), "tenant-vitima", "doc-da-vitima")
	if err != nil {
		t.Fatalf("leitura legítima falhou: %v", err)
	}
	if got == nil || got.ID != "doc-da-vitima" {
		t.Fatalf("esperava o documento do próprio tenant, veio %+v", got)
	}
}

// TestDocumentService_GetFileContent_RequiresTenant cobre o caminho usado pela
// rota interna de conteúdo: sem tenant no contexto, falha fechado.
func TestDocumentService_GetFileContent_RequiresTenant(t *testing.T) {
	svc := documentForTenant("tenant-vitima")

	if _, _, err := svc.GetFileContent(context.Background(), "doc-da-vitima"); err == nil {
		t.Fatal("GetFileContent sem tenant deveria falhar (fail-closed)")
	}
	if !strings.Contains(strings.ToLower(errString(svc)), "tenant") {
		// mensagem apenas informativa; o contrato é o erro acima
		t.Log("mensagem de erro não menciona tenant (ok, não é o contrato)")
	}
}

func errString(svc *service.DocumentService) string {
	_, _, err := svc.GetFileContent(context.Background(), "doc-da-vitima")
	if err == nil {
		return ""
	}
	return err.Error()
}

// TestDocumentService_GetByIDWithContextTenant garante que o caminho que
// resolve o tenant pelo contexto (usado por GetFileURL/GetFileContent) recusa
// tenant diferente do dono.
func TestDocumentService_GetByIDWithContextTenant(t *testing.T) {
	svc := documentForTenant("tenant-vitima")

	attackerCtx := pkgcontext.WithTenantID(context.Background(), "tenant-atacante")
	if _, _, err := svc.GetFileContent(attackerCtx, "doc-da-vitima"); err == nil {
		t.Fatal("VAZAMENTO CROSS-TENANT: GetFileContent devolveu documento de outro tenant")
	}

	ownerCtx := pkgcontext.WithTenantID(context.Background(), "tenant-vitima")
	if _, _, err := svc.GetFileContent(ownerCtx, "doc-da-vitima"); err == nil {
		// Sem storage configurado o caminho pode falhar depois da leitura; o
		// que importa é NÃO ser erro de tenant/autorização.
		if strings.Contains(strings.ToLower(err.Error()), "tenant") {
			t.Fatalf("dono do documento foi recusado por tenant: %v", err)
		}
	}
}
