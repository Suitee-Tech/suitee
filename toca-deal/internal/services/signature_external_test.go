package services_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/events"
	pkgevents "github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	service "github.com/oginoino/toca/toca-deal/internal/services"
)

// recordingBus captures published events so tests can assert on the wire shape.
// Reuses no other mocks because mockEventBus in the existing file is shared.
type recordingBus struct {
	mu        sync.Mutex
	published []events.Event
	err       error
}

func (b *recordingBus) Publish(ctx context.Context, e events.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	// Um bus real não publica com contexto cancelado. Sem isso o mock aceitava a
	// publicação sempre, e um teste que devia provar que o evento sai apesar do
	// cancelamento passava mesmo com o defeito presente.
	if err := ctx.Err(); err != nil {
		return err
	}
	if b.err != nil {
		return b.err
	}
	b.published = append(b.published, e)
	return nil
}
func (b *recordingBus) PublishReliable(ctx context.Context, e events.Event) error {
	return b.Publish(ctx, e)
}
func (b *recordingBus) Subscribe(_ string, _ events.EventHandler) error { return nil }
func (b *recordingBus) SubscribeWithQueue(_ string, _ string, _ events.EventHandler) error {
	return nil
}

// stubEvent satisfies events.Event with the literal name/payload supplied by
// the test, so we can drive subscribers directly without an actual bus.
type stubEvent struct {
	name    string
	payload interface{}
	id      string
}

func (e stubEvent) EventID() string {
	if e.id == "" {
		return "evt-" + e.name
	}
	return e.id
}
func (e stubEvent) Name() string         { return e.name }
func (e stubEvent) Payload() interface{} { return e.payload }

type fakeSignatureClient struct {
	calls int
	err   error
	req   clients.CreateSignatureEnvelopeRequest
	// Chamado quando a criação falha, antes de devolver o erro: permite
	// reproduzir o timeout do provedor que cancela o contexto da requisição.
	onFail func()
}

func (f *fakeSignatureClient) CreateForContract(_ context.Context, _, _ string, req clients.CreateSignatureEnvelopeRequest) (*clients.SignatureEnvelopeDTO, error) {
	f.calls++
	f.req = req
	if f.err != nil {
		if f.onFail != nil {
			f.onFail()
		}
		return nil, f.err
	}
	return &clients.SignatureEnvelopeDTO{ID: "env-1", Status: "RUNNING"}, nil
}

// GetByContract satisfies SignatureEnvelopeCreator. Tests that need a
// specific envelope payload should populate fakeSignatureClient.envelope
// (added on-demand when a test exercises GetPortalSignatureView).
func (f *fakeSignatureClient) GetByContract(_ context.Context, _, _ string) (*clients.SignatureEnvelopeDTO, error) {
	return nil, nil
}

// RefreshSignedDocumentURL satisfies SignatureEnvelopeCreator. Default
// returns "" — tests exercising the Phase 3 reconcile gap recovery
// override the embedding struct with their own implementation.
func (f *fakeSignatureClient) RefreshSignedDocumentURL(_ context.Context, _, _ string) (string, error) {
	return "", nil
}

func sampleSignedContract() *contract.Contract {
	return &contract.Contract{
		ID:        "contract-1",
		DealID:    "deal-1",
		TenantID:  "tenant-1",
		Status:    contract.StatusDraft,
		Type:      contract.TypeLocacao,
		StartDate: time.Now(),
		Signatories: []contract.Signatory{
			{ID: "sig-1", Name: "Inquilino", Email: "inq@x.com", Role: contract.RoleInquilino},
			{ID: "sig-2", Name: "Proprietario", Email: "prop@x.com", Role: contract.RoleProprietario},
		},
	}
}

func newSvc(t *testing.T, c *contract.Contract, bus events.EventBus) (*service.ContractService, *mockContractRepo) {
	t.Helper()
	d, _ := deal.New(deal.TypeRent, "prop-1", "owner-1", "cust-1", "broker-1", 2500, "tenant-1")
	dealRepo := &mockDealRepo{d: d}
	repo := &mockContractRepo{c: c}
	return service.NewContractService(repo, dealRepo, bus, nil), repo
}

func TestSendForSignature_RefreshesEmailFromDealBeforeEnvelope(t *testing.T) {
	bus := &recordingBus{}
	c := sampleSignedContract()
	d, _ := deal.New(deal.TypeRent, "prop-1", "owner-1", "cust-1", "broker-1", 2500, "tenant-1")
	d.Signers = []deal.Signer{{
		ID: "sig-1", Name: "Inquilino Corrigido", Email: "email-corrigido@example.com", Role: deal.SignatoryRole(contract.RoleInquilino),
	}}
	repo := &mockContractRepo{c: c}
	svc := service.NewContractService(repo, &mockDealRepo{d: d}, bus, nil)

	out, err := svc.SendForSignature(context.Background(), "tenant-1", "contract-1", service.SendForSignatureRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Signatories[0].Email != "email-corrigido@example.com" || out.Signatories[0].Name != "Inquilino Corrigido" {
		t.Fatalf("envio usou o snapshot antigo do contrato: %+v", out.Signatories[0])
	}
	if repo.c.Signatories[0].Email != "email-corrigido@example.com" {
		t.Fatalf("o contrato persistido não recebeu o e-mail corrigido: %+v", repo.c.Signatories[0])
	}
}

func TestSendForSignature_PublishesAndLocks(t *testing.T) {
	bus := &recordingBus{}
	svc, repo := newSvc(t, sampleSignedContract(), bus)

	out, err := svc.SendForSignature(context.Background(), "tenant-1", "contract-1", service.SendForSignatureRequest{
		SignerAuth: map[string][]string{
			"sig-1": {"email", "whatsapp"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != contract.StatusPending {
		t.Errorf("status = %s, want PENDING_SIGNATURE", out.Status)
	}
	if out.SignatureLockedAt == nil {
		t.Errorf("expected SignatureLockedAt to be set")
	}
	if out.SignatureProvider != "clicksign" {
		t.Errorf("provider = %s, want clicksign", out.SignatureProvider)
	}
	if out.Signatories[0].AuthMethods == nil || out.Signatories[0].AuthMethods[0] != "email" {
		t.Errorf("auth methods not persisted on signer: %+v", out.Signatories[0])
	}
	if len(bus.published) != 1 {
		t.Fatalf("expected 1 event, got %d", len(bus.published))
	}
	evt, ok := bus.published[0].(pkgevents.SignatureRequestedEvent)
	if !ok {
		t.Fatalf("expected SignatureRequestedEvent, got %T", bus.published[0])
	}
	if evt.Data.ContractID != "contract-1" {
		t.Errorf("contract id mismatch: %+v", evt.Data)
	}
	if len(evt.Data.Signers) != 2 {
		t.Errorf("expected 2 signers in payload, got %d", len(evt.Data.Signers))
	}
	// Verify per-signer auth was applied; the second signer should fall back
	// to email since SignerAuth didn't include them.
	if len(evt.Data.Signers[1].Auth) != 1 || evt.Data.Signers[1].Auth[0] != "email" {
		t.Errorf("expected fallback to ['email'] for sig-2, got %+v", evt.Data.Signers[1].Auth)
	}
	_ = repo
}

// O envio NÃO cria o envelope na requisição.
//
// Renderizar o contrato em PDF leva ~27s (medido: Gotenberg/Chromium), muito
// acima do tempo de um request — o cliente interno cortava em 15s e o gateway
// devolvia 504. O envio publica o evento e o toca-signature cria o envelope com
// o tempo que precisar.
func TestSendForSignature_DelegatesEnvelopeCreationToWorker(t *testing.T) {
	bus := &recordingBus{}
	sig := &fakeSignatureClient{}
	c := sampleSignedContract()
	d, _ := deal.New(deal.TypeRent, "prop-1", "owner-1", "cust-1", "broker-1", 2500, "tenant-1")
	repo := &mockContractRepo{c: c}
	svc := service.NewContractService(repo, &mockDealRepo{d: d}, bus, nil, sig)

	out, err := svc.SendForSignature(context.Background(), "tenant-1", "contract-1", service.SendForSignatureRequest{
		SignerAuth: map[string][]string{"sig-1": {"email"}},
		Locale:     "pt-BR",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != contract.StatusPending {
		t.Errorf("status = %s, want PENDING_SIGNATURE", out.Status)
	}
	if out.SignatureLockedAt == nil {
		t.Errorf("o envio deve travar o contrato")
	}
	// A requisição não pode renderizar o PDF: nada de chamada ao provedor aqui.
	if sig.calls != 0 {
		t.Errorf("o envio não deve chamar o provedor na requisição, calls = %d", sig.calls)
	}
	if len(bus.published) != 1 {
		t.Fatalf("o evento precisa sair para o worker criar o envelope, published = %d", len(bus.published))
	}
}

// O evento precisa sair mesmo quando o gateway cancela a requisição.
//
// Um timeout de gateway cancela o contexto. Se a publicação usasse esse
// contexto, o evento não sairia — e como o worker é quem GARANTE a criação do
// envelope, o contrato ficaria marcado como enviado sem ninguém criar nada:
// exatamente o estado sem saída que produziu os 409 em produção.
func TestSendForSignature_PublishesEnvelopeRequestAfterContextCanceled(t *testing.T) {
	bus := &recordingBus{}
	// A tentativa síncrona falha E cancela o contexto, como o timeout do gateway.
	sig := &fakeSignatureClient{err: errors.New("gateway timeout")}
	c := sampleSignedContract()
	d, _ := deal.New(deal.TypeRent, "prop-1", "owner-1", "cust-1", "broker-1", 2500, "tenant-1")
	repo := &mockContractRepo{c: c}
	svc := service.NewContractService(repo, &mockDealRepo{d: d}, bus, nil, sig)

	ctx, cancel := context.WithCancel(context.Background())
	sig.onFail = cancel

	out, err := svc.SendForSignature(ctx, "tenant-1", "contract-1", service.SendForSignatureRequest{})
	if err != nil {
		t.Fatalf("envio não pode falhar por lentidão do provedor: %v", err)
	}
	if out.Status != contract.StatusPending {
		t.Errorf("status = %s, want PENDING_SIGNATURE", out.Status)
	}
	// O essencial: o worker foi acionado.
	if len(bus.published) != 1 {
		t.Fatalf("o evento de envio PRECISA sair mesmo com o contexto cancelado (published = %d); "+
			"sem ele o contrato fica marcado como enviado e nenhum envelope é criado", len(bus.published))
	}
}

// Sem o evento E sem criação síncrona não há como o envelope existir: o envio
// precisa falhar para o operador poder tentar de novo, em vez de devolver
// sucesso e deixar o contrato marcado como enviado sem envelope.
func TestSendForSignature_FailsWhenWorkerCannotBeReached(t *testing.T) {
	bus := &recordingBus{err: errors.New("rabbitmq down")}
	sig := &fakeSignatureClient{}
	c := sampleSignedContract()
	d, _ := deal.New(deal.TypeRent, "prop-1", "owner-1", "cust-1", "broker-1", 2500, "tenant-1")
	repo := &mockContractRepo{c: c}
	svc := service.NewContractService(repo, &mockDealRepo{d: d}, bus, nil, sig)

	if _, err := svc.SendForSignature(context.Background(), "tenant-1", "contract-1", service.SendForSignatureRequest{}); err == nil {
		t.Fatalf("o envio precisa falhar quando o worker não pode ser acionado")
	}
}

func TestSendForSignature_LockedContractRejected(t *testing.T) {
	c := sampleSignedContract()
	c.Status = contract.StatusPending
	now := time.Now()
	c.SignatureLockedAt = &now
	bus := &recordingBus{}
	svc, _ := newSvc(t, c, bus)

	_, err := svc.SendForSignature(context.Background(), "tenant-1", "contract-1", service.SendForSignatureRequest{})
	if !errors.Is(err, service.ErrContractLockedForSignature) {
		t.Fatalf("expected ErrContractLockedForSignature, got %v", err)
	}
	if len(bus.published) != 0 {
		t.Errorf("no event should be published when locked")
	}
}

func TestUpdateContract_LockedRejected(t *testing.T) {
	c := sampleSignedContract()
	c.Status = contract.StatusPending
	now := time.Now()
	c.SignatureLockedAt = &now
	bus := &recordingBus{}
	svc, _ := newSvc(t, c, bus)

	_, err := svc.UpdateContract(context.Background(), "tenant-1", "contract-1", &contract.CreateContractRequest{
		Status: contract.StatusActive,
	})
	if !errors.Is(err, service.ErrContractLockedForSignature) {
		t.Fatalf("expected lock error, got %v", err)
	}
}

func TestMarkSignerSignedExternally_StampsSigner(t *testing.T) {
	c := sampleSignedContract()
	c.Status = contract.StatusPending
	c.ExternalEnvelopeID = "ext-env-123"
	bus := &recordingBus{}
	svc, repo := newSvc(t, c, bus)

	signed := time.Date(2026, 5, 10, 18, 30, 0, 0, time.UTC)
	if err := svc.MarkSignerSignedExternally(context.Background(), "tenant-1", "contract-1", "sig-1", signed, "https://evidence/abc", "ext-env-123"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := repo.c.Signatories[0]
	if got.SignedAt == nil || !got.SignedAt.Equal(signed) {
		t.Errorf("signer signed_at = %v, want %v", got.SignedAt, signed)
	}
	if got.EvidenceURL != "https://evidence/abc" {
		t.Errorf("evidence URL not propagated: %s", got.EvidenceURL)
	}
	if got.Signature == "" {
		t.Errorf("signature should be set")
	}
}

func TestMarkSignedExternally_FlipsAndPublishesContractSigned(t *testing.T) {
	c := sampleSignedContract()
	c.Status = contract.StatusPending
	c.ExternalEnvelopeID = "ext-env-123"
	bus := &recordingBus{}
	svc, repo := newSvc(t, c, bus)

	finished := time.Date(2026, 5, 10, 19, 0, 0, 0, time.UTC)
	out, err := svc.MarkSignedExternally(context.Background(), "tenant-1", "contract-1", "ext-env-123", finished, "https://docs/signed.pdf")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != contract.StatusSigned {
		t.Errorf("status = %s, want SIGNED", out.Status)
	}
	if out.ExternalEnvelopeID != "ext-env-123" {
		t.Errorf("external_envelope_id = %s, want ext-env-123", out.ExternalEnvelopeID)
	}
	if out.SignedDocumentURL != "https://docs/signed.pdf" {
		t.Errorf("signed doc url not propagated: %s", out.SignedDocumentURL)
	}
	for i, sig := range out.Signatories {
		if sig.SignedAt == nil {
			t.Errorf("signatory %d not stamped", i)
		}
	}
	if len(bus.published) != 1 {
		t.Fatalf("expected 1 ContractSignedEvent, got %d", len(bus.published))
	}
	evt, ok := bus.published[0].(contract.ContractSignedEvent)
	if !ok {
		t.Fatalf("expected ContractSignedEvent, got %T", bus.published[0])
	}
	if evt.Data.ContractID != "contract-1" || evt.Data.DealID != "deal-1" {
		t.Errorf("event payload mismatch: %+v", evt.Data)
	}
	_ = repo
}

func TestMarkSignedExternally_Idempotent(t *testing.T) {
	c := sampleSignedContract()
	c.Status = contract.StatusSigned
	c.ExternalEnvelopeID = "ext-env-123"
	bus := &recordingBus{}
	svc, _ := newSvc(t, c, bus)

	out, err := svc.MarkSignedExternally(context.Background(), "tenant-1", "contract-1", "ext-env-123", time.Now(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != contract.StatusSigned {
		t.Errorf("status changed unexpectedly: %s", out.Status)
	}
	if len(bus.published) != 0 {
		t.Errorf("idempotent path should not republish events")
	}
}

func TestHandleSignatureRefused_UnlocksContract(t *testing.T) {
	c := sampleSignedContract()
	c.Status = contract.StatusPending
	c.ExternalEnvelopeID = "ext-env-123"
	now := time.Now()
	c.SignatureLockedAt = &now
	bus := &recordingBus{}
	svc, repo := newSvc(t, c, bus)

	if err := svc.HandleSignatureRefused(context.Background(), "tenant-1", "contract-1", "ext-env-123", "sig-1", "wrong document"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.c.Status != contract.StatusDraft {
		t.Errorf("status = %s, want DRAFT", repo.c.Status)
	}
	if repo.c.SignatureLockedAt != nil {
		t.Errorf("lock should be cleared after refusal")
	}
}

func TestNewSignatureExpiredHandler_UnlocksContract(t *testing.T) {
	c := sampleSignedContract()
	c.Status = contract.StatusPending
	c.ExternalEnvelopeID = "ext-env-123"
	now := time.Now()
	c.SignatureLockedAt = &now
	bus := &recordingBus{}
	svc, repo := newSvc(t, c, bus)

	handler := service.NewSignatureExpiredHandler(svc)
	evt := stubEvent{
		name: "signature.expired",
		payload: map[string]interface{}{
			"tenant_id":            "tenant-1",
			"contract_id":          "contract-1",
			"envelope_id":          "local-env-1",
			"external_envelope_id": "ext-env-123",
			"expired_at":           time.Now().UTC().Format(time.RFC3339),
			"timestamp":            time.Now().UTC().Format(time.RFC3339),
		},
	}
	if err := handler(context.Background(), evt); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if repo.c.Status != contract.StatusDraft {
		t.Errorf("status = %s, want DRAFT after expired", repo.c.Status)
	}
	if repo.c.SignatureLockedAt != nil {
		t.Errorf("lock should be cleared after expiration")
	}
}

func TestNewSignatureExpiredHandler_IgnoresMissingFields(t *testing.T) {
	c := sampleSignedContract()
	bus := &recordingBus{}
	svc, _ := newSvc(t, c, bus)

	handler := service.NewSignatureExpiredHandler(svc)
	// Missing contract_id → handler must log + return nil (no DLQ poisoning).
	evt := stubEvent{
		name:    "signature.expired",
		payload: map[string]interface{}{"tenant_id": "tenant-1"},
	}
	if err := handler(context.Background(), evt); err != nil {
		t.Fatalf("handler should not error on incomplete payload, got %v", err)
	}
}

// envelopeReturningSignatureClient responde GetByContract com um envelope
// concreto — necessário para correlacionar o evento quando o contrato ainda não
// tem `external_envelope_id`.
type envelopeReturningSignatureClient struct {
	fakeSignatureClient
	env *clients.SignatureEnvelopeDTO
}

func (f *envelopeReturningSignatureClient) GetByContract(_ context.Context, _, _ string) (*clients.SignatureEnvelopeDTO, error) {
	return f.env, nil
}

// Regressão do contrato 277b965e (beta, 30/09/2026): o contrato já estava
// SIGNED quando o envelope_finished foi reprocessado (depois de corrigir a RLS),
// e o caminho idempotente devolvia sem gravar o vínculo — o contrato ficava sem
// `external_envelope_id` mesmo com o envelope existindo.
func TestMarkSignedExternally_BackfillsExternalEnvelopeIDWhenAlreadySigned(t *testing.T) {
	c := sampleSignedContract()
	c.Status = contract.StatusSigned
	c.ExternalEnvelopeID = "" // assinado por outro caminho antes deste evento
	bus := &recordingBus{}
	d, _ := deal.New(deal.TypeRent, "prop-1", "owner-1", "cust-1", "broker-1", 2500, "tenant-1")
	repo := &mockContractRepo{c: c}
	sc := &envelopeReturningSignatureClient{env: &clients.SignatureEnvelopeDTO{
		ID:                 "env-1",
		ContractID:         "contract-1",
		ExternalEnvelopeID: "ext-env-123",
		Status:             "FINISHED",
		CreatedAt:          time.Now(),
	}}
	svc := service.NewContractService(repo, &mockDealRepo{d: d}, bus, nil, sc)

	out, err := svc.MarkSignedExternally(context.Background(), "tenant-1", "contract-1", "ext-env-123", time.Now(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ExternalEnvelopeID != "ext-env-123" {
		t.Errorf("external_envelope_id = %q, want ext-env-123 (vínculo não gravado)", out.ExternalEnvelopeID)
	}
	if repo.c.ExternalEnvelopeID != "ext-env-123" {
		t.Errorf("repositório não recebeu o vínculo: %q", repo.c.ExternalEnvelopeID)
	}
	if len(bus.published) != 0 {
		t.Errorf("caminho idempotente não pode republicar eventos, publicou %d", len(bus.published))
	}
}
