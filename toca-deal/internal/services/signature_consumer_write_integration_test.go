package services_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/config"
	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	"github.com/oginoino/toca/toca-deal/internal/repositories"
	service "github.com/oginoino/toca/toca-deal/internal/services"
)

// Integração (roda só com RLS_PROBE_URI_APP/RLS_PROBE_URI_SERVICE definidos):
// prova o caminho de ESCRITA do consumidor de eventos de assinatura contra o
// banco real, com as mesmas credenciais do beta.
//
// Por que existe: o consumidor roda SEM JWT. A RLS de deal.contracts/deal.deals
// compara tenant_id com auth.jwt() — e o pool de aplicação injeta
// request.jwt.claims a partir do tenant do CONTEXTO (applyJWTClaimsHook). O
// handler de assinatura põe esse tenant no ctx (pkgctx.WithTenantID), então a
// escrita passa com isolamento por tenant. Este teste trava as duas metades:
//
//  1. sem tenant no contexto a leitura pelo pool de aplicação devolve "nenhuma
//     linha" (isolamento ligado);
//  2. com o tenant no contexto o handler REAL grava contrato e projeção do
//     negócio — é o caminho de produção do consumidor;
//  3. um evento com tenant ERRADO não toca o contrato de outro tenant.
func TestSignerSignedConsumerWritesUnderRLS(t *testing.T) {
	appURI := os.Getenv("RLS_PROBE_URI_APP")
	svcURI := os.Getenv("RLS_PROBE_URI_SERVICE")
	if appURI == "" || svcURI == "" {
		t.Skip("defina RLS_PROBE_URI_APP e RLS_PROBE_URI_SERVICE")
	}
	ctx := context.Background()
	pg, err := database.ConnectPostgres(ctx, config.DatabaseConfig{URI: appURI, ServiceURI: svcURI})
	if err != nil {
		t.Fatal(err)
	}
	defer pg.Close()

	// O tenant é sintético: o que importa é a RLS (o pool de app é negado sem
	// claims, independentemente do tenant) e o pool de serviço bypassar.
	tenantID := uuid.NewString()
	contractRepo := repositories.NewContractPostgresRepository(pg)
	dealRepo := repositories.NewDealPostgresRepository(pg)
	svc := service.NewContractService(contractRepo, dealRepo, nil, pg)
	handler := service.NewSignerSignedHandler(svc)

	// O caminho de produção: tenant no contexto → claims injetadas no checkout
	// da conexão → a RLS libera as linhas daquele tenant.
	tenantCtx := pkgctx.WithTenantID(ctx, tenantID)

	signerID := uuid.NewString()
	dealID, contractID := uuid.NewString(), uuid.NewString()
	controlDealID, controlContractID := uuid.NewString(), uuid.NewString()

	cleanup := func() {
		_ = pg.ExecService(ctx, `DELETE FROM deal.contracts WHERE id = ANY($1::uuid[])`,
			[]string{contractID, controlContractID})
		_ = pg.ExecService(ctx, `DELETE FROM deal.deals WHERE id = ANY($1::uuid[])`,
			[]string{dealID, controlDealID})
	}
	cleanup()
	defer cleanup()

	makeFixture := func(dID, cID string) {
		t.Helper()
		d, err := deal.New(deal.TypeSale, uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), 1000, tenantID)
		if err != nil {
			t.Fatalf("deal.New: %v", err)
		}
		d.ID = dID
		d.Signers = []deal.Signer{{
			ID: signerID, Name: "Signatário E2E", Email: "e2e-assinatura@example.invalid",
			Role: deal.SignatoryRoleComprador, Status: deal.SignerStatusPending,
		}}
		if err := dealRepo.Create(tenantCtx, d); err != nil {
			t.Fatalf("criar negócio de teste: %v", err)
		}

		sent := time.Now().Add(-time.Minute)
		c := &contract.Contract{
			ID:                 cID,
			TenantID:           tenantID,
			DealID:             dID,
			Status:             contract.StatusPending,
			Type:               contract.TypeVenda,
			ExternalEnvelopeID: "e2e-envelope-1",
			SignatureLockedAt:  &sent,
			SentForSignatureAt: &sent,
			Signatories: []contract.Signatory{
				{ID: signerID, Name: "Signatário E2E", Email: "e2e-assinatura@example.invalid", Role: contract.RoleComprador, Status: contract.SignatoryStatusPending},
				{ID: uuid.NewString(), Name: "Outro", Email: "e2e-outro@example.invalid", Role: contract.RoleVendedor, Status: contract.SignatoryStatusPending},
			},
		}
		if err := contractRepo.Create(tenantCtx, c); err != nil {
			t.Fatalf("criar contrato de teste: %v", err)
		}
	}
	makeFixture(dealID, contractID)
	makeFixture(controlDealID, controlContractID)

	signedAt := time.Now().UTC().Truncate(time.Second)
	// 1) Isolamento: o mesmo SELECT pelo pool de aplicação só enxerga a linha
	// quando o tenant do contexto vira claim no checkout da conexão.
	if _, err := contractRepo.GetByID(ctx, tenantID, contractID); err == nil {
		t.Error("SEM tenant no contexto a leitura deveria falhar (RLS); passou, então o isolamento não está valendo")
	}

	event := stubEvent{name: "signature.signer_signed", id: "evt-e2e", payload: map[string]interface{}{
		"tenant_id":            tenantID,
		"contract_id":          contractID,
		"signer_id":            signerID,
		"external_envelope_id": "e2e-envelope-1",
		"signed_at":            signedAt.Format(time.RFC3339),
	}}

	// 2) Com o tenant no contexto (como os subscribers reais): grava contrato e negócio.
	if err := handler(tenantCtx, event); err != nil {
		t.Fatalf("handler com tenant no contexto falhou (é o caminho de produção): %v", err)
	}
	gotContract, err := contractRepo.GetByID(tenantCtx, tenantID, contractID)
	if err != nil {
		t.Fatalf("relendo contrato: %v", err)
	}
	if gotContract.Signatories[0].Status != contract.SignatoryStatusSigned {
		t.Errorf("signatário = %s, want SIGNED (o consumidor não conseguiu gravar)", gotContract.Signatories[0].Status)
	}
	if gotContract.Signatories[0].SignedAt == nil {
		t.Error("signed_at do signatário ficou vazio")
	}
	if gotContract.Status != contract.StatusPartiallySigned {
		t.Errorf("status do contrato = %s, want PARTIALLY_SIGNED (ainda há um signatário pendente)", gotContract.Status)
	}
	if gotContract.ExternalEnvelopeID != "e2e-envelope-1" {
		t.Errorf("external_envelope_id = %q, want e2e-envelope-1", gotContract.ExternalEnvelopeID)
	}
	gotDeal, err := dealRepo.GetByID(tenantCtx, tenantID, dealID)
	if err != nil {
		t.Fatalf("relendo negócio: %v", err)
	}
	if len(gotDeal.Signers) == 0 || gotDeal.Signers[0].Status != deal.SignerStatusSigned {
		t.Errorf("signer do negócio = %+v, want SIGNED (a projeção do negócio não foi atualizada)", gotDeal.Signers)
	}

	// 3) Evento com tenant ERRADO: o handler usa o tenant do payload como claim,
	// então a RLS esconde o contrato do outro tenant e nada é gravado. É a
	// garantia de que o escopo por tenant continua valendo no consumidor.
	controlEvent := stubEvent{name: "signature.signer_signed", id: "evt-e2e-wrong-tenant", payload: map[string]interface{}{
		"tenant_id":            uuid.NewString(),
		"contract_id":          controlContractID,
		"signer_id":            signerID,
		"external_envelope_id": "e2e-envelope-1",
		"signed_at":            signedAt.Format(time.RFC3339),
	}}
	if err := handler(tenantCtx, controlEvent); err == nil {
		t.Error("evento com tenant errado deveria falhar: o contrato pertence a outro tenant")
	}
	untouched, err := contractRepo.GetByID(tenantCtx, tenantID, controlContractID)
	if err != nil {
		t.Fatalf("relendo contrato de controle: %v", err)
	}
	if untouched.Signatories[0].Status == contract.SignatoryStatusSigned {
		t.Error("o evento com tenant errado gravou no contrato: a RLS não estava isolando")
	}
}
