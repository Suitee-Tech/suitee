package services_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/config"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	"github.com/oginoino/toca/toca-deal/internal/domain/document"
	"github.com/oginoino/toca/toca-deal/internal/repositories"
	service "github.com/oginoino/toca/toca-deal/internal/services"
)

// Integração (roda só com RLS_PROBE_URI_APP/RLS_PROBE_URI_SERVICE definidos):
// prova, contra o banco real, que o arquivo enviado pelo cliente na esteira
// digital passa a existir no painel de documentos da imobiliária — com o
// vínculo, a categoria e as tags que o painel usa para organizar.
//
// Roda também o backfill quando BACKFILL_TENANT_ID é informado: é a mesma
// operação exposta em POST /api/v1/deals/backfill-checklist-documents, usada
// para materializar o que foi enviado antes deste recurso existir.
func TestChecklistDocumentsReachTheAgencyPanel(t *testing.T) {
	appURI := os.Getenv("RLS_PROBE_URI_APP")
	svcURI := os.Getenv("RLS_PROBE_URI_SERVICE")
	if appURI == "" || svcURI == "" {
		t.Skip("defina RLS_PROBE_URI_APP e RLS_PROBE_URI_SERVICE")
	}
	ctx := context.Background()
	db, err := database.ConnectPostgres(ctx, config.DatabaseConfig{URI: appURI, ServiceURI: svcURI})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	dealRepo := repositories.NewDealPostgresRepository(db)
	docRepo := repositories.NewDocumentPostgresRepository(db)
	docService := service.NewDocumentService(docRepo, nil)
	dealService := service.NewDealService(dealRepo, nil, nil, nil, nil).WithChecklistDocumentSync(docService)

	tenantID := uuid.NewString()
	// As escritas passam pelo pool de aplicação com as claims do tenant no
	// contexto — exatamente como no caminho real (requisição do cliente).
	tctx := pkgcontext.WithTenantID(ctx, tenantID)
	dealID := uuid.NewString()
	itemID := uuid.NewString()
	signerless := func() { _ = db.ExecService(ctx, `DELETE FROM deal.deals WHERE id = $1`, dealID) }
	defer func() {
		_ = db.ExecService(ctx, `DELETE FROM deal.documents WHERE tenant_id = $1`, tenantID)
		signerless()
	}()

	d, err := deal.New(deal.TypeSale, uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), 1000, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	d.ID = dealID
	uploadedAt := time.Now().Add(-30 * time.Minute)
	d.Checklist = []deal.ChecklistItem{{
		ID:          itemID,
		Name:        "Comprovante de renda",
		Category:    deal.CategoryFinanceiro,
		Status:      deal.ChecklistUploaded,
		Required:    true,
		UploadedAt:  &uploadedAt,
		UploadedBy:  "cliente@example.invalid",
		FileName:    "holerite.pdf",
		ContentType: "application/pdf",
		FileSize:    4242,
		StorageKey:  tenantID + "/checklist/" + dealID + "/abc_holerite.pdf",
	}}
	if err := dealRepo.Create(tctx, d); err != nil {
		t.Fatalf("criar negócio sintético: %v", err)
	}

	// Caminho do portal: item já enviado (arquivo no bucket) → espelho no painel.
	if _, err := docService.EnsureForChecklistItem(tctx, service.ChecklistDocumentInput{
		TenantID:     tenantID,
		DealID:       dealID,
		ItemID:       itemID,
		ItemName:     d.Checklist[0].Name,
		ItemCategory: string(d.Checklist[0].Category),
		FileName:     d.Checklist[0].FileName,
		StorageKey:   d.Checklist[0].StorageKey,
		FileSize:     d.Checklist[0].FileSize,
		ContentType:  d.Checklist[0].ContentType,
		UploadedBy:   d.Checklist[0].UploadedBy,
		UploadedAt:   uploadedAt,
	}); err != nil {
		t.Fatalf("espelhar documento da esteira: %v", err)
	}

	// O painel lê por tenant (List sem filtros) — é assim que ele precisa aparecer.
	docs, total, err := docRepo.List(tctx, tenantID, document.ListFilters{Limit: 50})
	if err != nil {
		t.Fatalf("listar documentos do painel: %v", err)
	}
	if total != 1 || len(docs) != 1 {
		t.Fatalf("painel devolveu %d documentos (total=%d), want 1", len(docs), total)
	}
	got := docs[0]
	if got.Category != document.CategoryComprovante {
		t.Errorf("categoria = %s, want comprovante", got.Category)
	}
	if got.EntityType != document.EntityDeal || got.EntityID != dealID {
		t.Errorf("vínculo = %s/%s, want deal/%s", got.EntityType, got.EntityID, dealID)
	}
	if got.StorageKey != d.Checklist[0].StorageKey {
		t.Errorf("storage key = %q, want %q", got.StorageKey, d.Checklist[0].StorageKey)
	}
	hasEsteira := false
	for _, tag := range got.Tags {
		if tag == "esteira" {
			hasEsteira = true
		}
	}
	if !hasEsteira {
		t.Errorf("tags = %v, want incluir 'esteira' para o painel filtrar a esteira", got.Tags)
	}

	// Backfill (uma vez): a operação exposta pela rota administrativa precisa
	// ser idempotente sobre o que já está espelhado.
	stats, err := dealService.BackfillChecklistDocuments(tctx, tenantID)
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if stats.ItemsScanned < 1 || stats.Documents < 1 {
		t.Errorf("backfill = %+v, want ao menos 1 item/documento do negócio sintético", stats)
	}
	if _, totalAfter, err := docRepo.List(tctx, tenantID, document.ListFilters{Limit: 50}); err != nil || totalAfter != 1 {
		t.Errorf("backfill duplicou documento: total=%d err=%v", totalAfter, err)
	}

	// Reparo opcional do tenant real informado pelo operador.
	if tenant := os.Getenv("BACKFILL_TENANT_ID"); tenant != "" {
		real, err := dealService.BackfillChecklistDocuments(pkgcontext.WithTenantID(ctx, tenant), tenant)
		if err != nil {
			t.Fatalf("backfill do tenant %s: %v", tenant, err)
		}
		t.Logf("backfill do tenant real: %+v", real)
	}
}
