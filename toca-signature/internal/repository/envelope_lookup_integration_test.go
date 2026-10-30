package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/database"
)

// Integração (roda só com RLS_PROBE_URI_SERVICE definido): reproduz o banco do
// beta que derrubou todos os webhooks da Clicksign em 30/09/2026.
//
// Existiam envelopes antigos com `metadata->'documents'` = JSON `null`. A query
// do fallback por document key fazia
// `jsonb_array_elements(COALESCE(metadata->'documents', '[]'::jsonb))` — e
// jsonb null NÃO é SQL NULL, então o COALESCE não protegia: a query inteira
// falhava com "cannot extract elements from a scalar". Como o repositório
// tratava qualquer erro de Scan como "não encontrado", o handler acksava 200
// "envelope not found locally" e o evento era descartado.
//
// O teste insere um envelope legado (documents = null) na frente do envelope
// real e exige que o lookup ache o envelope certo em vez de estourar.
func TestGetByDocumentKeySurvivesLegacyNullDocuments(t *testing.T) {
	svcURI := os.Getenv("RLS_PROBE_URI_SERVICE")
	appURI := os.Getenv("RLS_PROBE_URI_APP")
	if svcURI == "" || appURI == "" {
		t.Skip("defina RLS_PROBE_URI_APP e RLS_PROBE_URI_SERVICE")
	}
	ctx := context.Background()
	pg, err := database.ConnectPostgres(ctx, config.DatabaseConfig{URI: appURI, ServiceURI: svcURI})
	if err != nil {
		t.Fatal(err)
	}
	defer pg.Close()

	const (
		contractID = "00000000-0000-4000-8000-00000000c0de"
		tenantID   = "bc46e19a-7750-4ed8-a5fb-ebe48ee99a91"
		docKey     = "00000000-0000-4000-8000-00000000d0c0"
		legacyID   = "00000000-0000-4000-8000-000000000001"
		modernID   = "00000000-0000-4000-8000-000000000002"
	)
	cleanup := func() {
		_ = pg.ExecService(ctx, `DELETE FROM signature.envelopes WHERE id IN ($1, $2)`, legacyID, modernID)
	}
	cleanup()
	defer cleanup()

	// Envelope legado: `documents` explicitamente null (o formato que quebrava a
	// query) e mais antigo, para ser avaliado antes do envelope moderno.
	if err := pg.ExecService(ctx, `
		INSERT INTO signature.envelopes (id, tenant_id, contract_id, external_envelope_id, environment, status, metadata, created_at, updated_at, kind)
		VALUES ($1::uuid, $2::uuid, '00000000-0000-4000-8000-0000000000c1'::uuid, 'legacy-envelope-x', 'sandbox', 'FINISHED',
		        jsonb_build_object('id', $1::text, 'tenant_id', $2::text, 'status', 'FINISHED', 'documents', 'null'::jsonb),
		        now() - interval '2 days', now() - interval '2 days', 'CONTRACT')`,
		legacyID, tenantID); err != nil {
		t.Fatal(err)
	}

	if err := pg.ExecService(ctx, `
		INSERT INTO signature.envelopes (id, tenant_id, contract_id, external_envelope_id, environment, status, metadata, created_at, updated_at, kind)
		VALUES ($1::uuid, $2::uuid, $3::uuid, 'modern-envelope-x', 'sandbox', 'RUNNING',
		        jsonb_build_object('id', $1::text, 'tenant_id', $2::text, 'contract_id', $3::text, 'status', 'RUNNING',
		                           'documents', jsonb_build_array(jsonb_build_object('clicksign_document_key', $4::text))),
		        now(), now(), 'CONTRACT')`,
		modernID, tenantID, contractID, docKey); err != nil {
		t.Fatal(err)
	}

	repo := NewEnvelopePostgresRepository(pg)

	// 1) A query não pode estourar por causa da linha legada.
	env, err := repo.GetByDocumentKey(ctx, docKey)
	if err != nil {
		t.Fatalf("GetByDocumentKey devolveu erro (a linha legada com documents=null quebra a query): %v", err)
	}
	if env == nil {
		t.Fatal("GetByDocumentKey não achou o envelope pelo document key")
	}
	if env.ID != modernID {
		t.Errorf("envelope encontrado = %s, quero %s", env.ID, modernID)
	}
	if env.TenantID != tenantID {
		t.Errorf("tenant = %s, quero %s", env.TenantID, tenantID)
	}

	// 2) Chave inexistente continua sendo "não encontrado" (nil, nil), não erro.
	missing, err := repo.GetByDocumentKey(ctx, "00000000-0000-4000-8000-00000000dead")
	if err != nil {
		t.Fatalf("GetByDocumentKey para chave inexistente devolveu erro: %v", err)
	}
	if missing != nil {
		t.Errorf("chave inexistente devolveu envelope %s", missing.ID)
	}

	// 3) O lookup por id externo (caminho principal) segue funcionando.
	byExternal, err := repo.GetByExternalID(ctx, "modern-envelope-x")
	if err != nil {
		t.Fatalf("GetByExternalID: %v", err)
	}
	if byExternal == nil || byExternal.ID != modernID {
		t.Errorf("GetByExternalID devolveu %v, quero %s", byExternal, modernID)
	}

	// 4) A varredura de reconciliação precisa ENXERGAR o envelope pelo pool de
	// serviço. Rodando sem JWT, a mesma query no pool de aplicação devolve zero
	// linhas (medido no beta: 0 x 2) e a rede de segurança nunca dispara.
	if err := pg.ExecService(ctx, `UPDATE signature.envelopes SET status = 'RUNNING', updated_at = now() - interval '1 hour' WHERE id = $1`, modernID); err != nil {
		t.Fatal(err)
	}
	stale, err := repo.ListStaleActive(ctx, time.Now().Add(-10*time.Minute), 50)
	if err != nil {
		t.Fatalf("ListStaleActive: %v", err)
	}
	found := false
	for _, e := range stale {
		if e.ID == modernID {
			found = true
		}
	}
	if !found {
		t.Errorf("ListStaleActive não devolveu o envelope %s (%d devolvidos) — a varredura roda sem JWT e precisa do pool de serviço", modernID, len(stale))
	}

	// 5) A leitura service-scope por id também precisa funcionar sem JWT.
	fetched, err := repo.GetByIDService(ctx, modernID)
	if err != nil {
		t.Fatalf("GetByIDService: %v", err)
	}
	if fetched == nil || fetched.ID != modernID {
		t.Errorf("GetByIDService devolveu %v, quero %s", fetched, modernID)
	}
}
