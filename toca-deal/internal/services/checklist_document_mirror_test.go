package services_test

import (
	"context"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-deal/internal/domain/document"
	service "github.com/oginoino/toca/toca-deal/internal/services"
)

// O arquivo que o cliente envia pela esteira digital vive no metadata do
// negócio (checklist[]), e o painel de documentos lê deal.documents. Este
// espelho é o que faz o documento existir para a imobiliária — com nome legível,
// categoria classificada, tags de organização e vínculo com o negócio.
func TestEnsureForChecklistItemCreatesClassifiedDocument(t *testing.T) {
	repo := &fakeDocumentRepo{}
	svc := service.NewDocumentService(repo, nil)

	const tenantID = "tenant-1"
	const dealID = "deal-1"
	const itemID = "66936966-63a5-4a18-be1e-63aa7e2bffd9"
	uploadedAt := time.Now().Add(-time.Hour)

	doc, err := svc.EnsureForChecklistItem(context.Background(), service.ChecklistDocumentInput{
		TenantID:             tenantID,
		DealID:               dealID,
		PartyID:              "party-1",
		ItemID:               itemID,
		ItemName:             "Comprovante de renda",
		ItemCategory:         "financeiro",
		RequestedFieldLabels: []string{"Comprovante de renda"},
		FileName:             "holerite-outubro.pdf",
		StorageKey:           tenantID + "/checklist/deal-1/abc_holerite.pdf",
		FileSize:             12345,
		ContentType:          "application/pdf",
		UploadedBy:           "contato@example.com",
		UploadedAt:           uploadedAt,
	})
	if err != nil {
		t.Fatalf("EnsureForChecklistItem: %v", err)
	}
	if doc == nil {
		t.Fatal("nenhum documento materializado")
	}
	if doc.ID != document.ChecklistDocumentID(itemID) {
		t.Errorf("id = %s, want id determinístico do item (%s)", doc.ID, document.ChecklistDocumentID(itemID))
	}
	if doc.Category != document.CategoryComprovante {
		t.Errorf("categoria = %s, want %s", doc.Category, document.CategoryComprovante)
	}
	if doc.Status != document.StatusPendente {
		t.Errorf("status = %s, want %s (aguardando conferência)", doc.Status, document.StatusPendente)
	}
	if doc.EntityType != document.EntityDeal || doc.EntityID != dealID {
		t.Errorf("vínculo = %s/%s, want deal/%s (organização por negócio)", doc.EntityType, doc.EntityID, dealID)
	}
	if doc.Name != "Comprovante de renda — holerite-outubro.pdf" {
		t.Errorf("nome = %q, want item + arquivo", doc.Name)
	}
	if doc.Source != document.SourcePortal {
		t.Errorf("source = %s, want %s", doc.Source, document.SourcePortal)
	}
	if doc.StorageKey == "" || doc.PersistenceStatus != document.PersistenceStored {
		t.Errorf("arquivo não ficou como armazenado: key=%q persistence=%s", doc.StorageKey, doc.PersistenceStatus)
	}
	if doc.FileType != "application/pdf" || doc.FileSize != 12345 {
		t.Errorf("arquivo = %s/%d bytes, want application/pdf/12345", doc.FileType, doc.FileSize)
	}
	if doc.UploadedBy == "" || !doc.UploadDate.Equal(uploadedAt) {
		t.Errorf("autoria = %q em %s, want o cliente e %s", doc.UploadedBy, doc.UploadDate, uploadedAt)
	}
	wantTags := map[string]bool{"esteira": true, "cliente": true, string(document.CategoryComprovante): true}
	for _, tag := range doc.Tags {
		delete(wantTags, tag)
	}
	if len(wantTags) != 0 {
		t.Errorf("tags = %v, faltaram %v (organização no painel)", doc.Tags, wantTags)
	}
}

// Reenvio do cliente (ou replay do evento) ATUALIZA a linha existente: o painel
// não pode ganhar um documento duplicado a cada tentativa.
func TestEnsureForChecklistItemIsIdempotentOnReupload(t *testing.T) {
	repo := &fakeDocumentRepo{}
	svc := service.NewDocumentService(repo, nil)
	ctx := context.Background()

	base := service.ChecklistDocumentInput{
		TenantID: "tenant-1", DealID: "deal-1", ItemID: "item-1",
		ItemName: "CPF", ItemCategory: "pessoal",
		FileName: "rg.pdf", StorageKey: "tenant-1/checklist/deal-1/a_rg.pdf",
		FileSize: 100, ContentType: "application/pdf", UploadedBy: "cliente",
	}
	first, err := svc.EnsureForChecklistItem(ctx, base)
	if err != nil {
		t.Fatalf("primeiro envio: %v", err)
	}
	if first.Category != document.CategoryIdentidade {
		t.Fatalf("categoria = %s, want identidade", first.Category)
	}

	second := base
	second.FileName = "rg-frente-e-verso.pdf"
	second.StorageKey = "tenant-1/checklist/deal-1/b_rg.pdf"
	second.FileSize = 200
	second.ItemName = "Documento de identidade"
	updated, err := svc.EnsureForChecklistItem(ctx, second)
	if err != nil {
		t.Fatalf("reenvio: %v", err)
	}
	if len(repo.creates) != 1 {
		t.Fatalf("documentos criados = %d, want 1 (reenvio não pode duplicar)", len(repo.creates))
	}
	if updated.ID != first.ID {
		t.Errorf("id mudou no reenvio: %s != %s", updated.ID, first.ID)
	}
	if updated.StorageKey != second.StorageKey || updated.FileSize != 200 {
		t.Errorf("arquivo não foi atualizado: key=%q size=%d", updated.StorageKey, updated.FileSize)
	}
	if updated.Name != "Documento de identidade — rg-frente-e-verso.pdf" {
		t.Errorf("nome = %q, want o do último arquivo", updated.Name)
	}
	if updated.Version < 2 {
		t.Errorf("versão = %d, want >= 2 (o painel mostra o histórico)", updated.Version)
	}
}

// Item cumprido só com preenchimento de formulário não tem arquivo: não pode
// virar documento vazio no painel.
func TestEnsureForChecklistItemSkipsFormOnlyItems(t *testing.T) {
	repo := &fakeDocumentRepo{}
	svc := service.NewDocumentService(repo, nil)
	doc, err := svc.EnsureForChecklistItem(context.Background(), service.ChecklistDocumentInput{
		TenantID: "tenant-1", DealID: "deal-1", ItemID: "item-form",
		ItemName: "Dados bancários", ItemCategory: "financeiro",
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if doc != nil || len(repo.creates) != 0 {
		t.Fatalf("formulário sem arquivo gerou documento: %+v", doc)
	}
}

// Sem tenant ou item não há o que materializar (fail-closed).
func TestEnsureForChecklistItemRequiresTenantAndItem(t *testing.T) {
	repo := &fakeDocumentRepo{}
	svc := service.NewDocumentService(repo, nil)
	doc, err := svc.EnsureForChecklistItem(context.Background(), service.ChecklistDocumentInput{
		DealID: "deal-1", ItemID: "", FileName: "x.pdf", StorageKey: "k",
	})
	if err != nil || doc != nil || len(repo.creates) != 0 {
		t.Fatalf("entrada incompleta gerou documento: doc=%+v err=%v", doc, err)
	}
}
