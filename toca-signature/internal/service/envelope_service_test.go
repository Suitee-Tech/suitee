package service_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/clients"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-signature/internal/clicksign"
	"github.com/oginoino/toca/toca-signature/internal/domain/credential"
	"github.com/oginoino/toca/toca-signature/internal/domain/envelope"
	"github.com/oginoino/toca/toca-signature/internal/service"
)

// ──────────────────────────── Fakes ──────────────────────────────

type fakeRepo struct {
	created       []*envelope.SignatureEnvelope
	updated       []*envelope.SignatureEnvelope
	byContract    map[string]*envelope.SignatureEnvelope
	byID          map[string]*envelope.SignatureEnvelope
	webhookEvents []envelope.WebhookEventRow
	reassociated  []webhookRelink
	// webhookID é o id devolvido por AppendWebhookEvent; vazio reproduz o
	// comportamento antigo dos testes que não olham auditoria.
	webhookID string
	// reassociateErr força falha no relink da auditoria (webhook).
	reassociateErr error
	// byDocumentKey/byExternalID alimentam os lookups service-scope do webhook.
	byDocumentKey map[string]*envelope.SignatureEnvelope
	byExternalID  map[string]*envelope.SignatureEnvelope
	createCalls   int32
	updateCalls   int32
	serviceReads  int32
	createErr     error
	updateErr     error
	// staleActive alimenta ListStaleActive — a varredura de reconciliação.
	staleActive []*envelope.SignatureEnvelope
	// writeTenants registra o tenant presente no contexto de cada escrita.
	// Existe para os testes provarem que o caminho sem JWT (webhook da
	// Clicksign) escreve com o tenant do envelope no contexto — sem ele o pool
	// de app instala claims vazios e a RLS esconde a linha.
	writeTenants []string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		byContract:    map[string]*envelope.SignatureEnvelope{},
		byID:          map[string]*envelope.SignatureEnvelope{},
		byDocumentKey: map[string]*envelope.SignatureEnvelope{},
		byExternalID:  map[string]*envelope.SignatureEnvelope{},
	}
}

func (f *fakeRepo) Create(ctx context.Context, e *envelope.SignatureEnvelope) error {
	atomic.AddInt32(&f.createCalls, 1)
	if f.createErr != nil {
		return f.createErr
	}
	f.created = append(f.created, copyEnv(e))
	f.byContract[e.ContractID] = e
	f.byID[e.ID] = e
	return nil
}

func (f *fakeRepo) Update(ctx context.Context, e *envelope.SignatureEnvelope) error {
	atomic.AddInt32(&f.updateCalls, 1)
	if tenantID, err := pkgcontext.GetTenantID(ctx); err == nil {
		f.writeTenants = append(f.writeTenants, tenantID)
	}
	if f.updateErr != nil {
		return f.updateErr
	}
	f.updated = append(f.updated, copyEnv(e))
	f.byContract[e.ContractID] = e
	f.byID[e.ID] = e
	return nil
}

// UpdateInTx delegates to Update for the fake — the test setup does not
// stand up a real Postgres transaction, so the tx argument is unused
// (just present to satisfy the interface). Production behaviour is
// exercised by the Postgres impl + integration tests.
func (f *fakeRepo) UpdateInTx(ctx context.Context, _ *sqlx.Tx, e *envelope.SignatureEnvelope) error {
	return f.Update(ctx, e)
}

func (f *fakeRepo) GetByID(_ context.Context, _, id string) (*envelope.SignatureEnvelope, error) {
	return f.byID[id], nil
}

// GetByIDService é a leitura do caminho de background (sem JWT). O contador
// serviceReads existe para os testes provarem que a varredura usa esse caminho.
func (f *fakeRepo) GetByIDService(_ context.Context, id string) (*envelope.SignatureEnvelope, error) {
	atomic.AddInt32(&f.serviceReads, 1)
	return f.byID[id], nil
}

func (f *fakeRepo) UpdateService(ctx context.Context, e *envelope.SignatureEnvelope) error {
	return f.Update(ctx, e)
}

func (f *fakeRepo) GetByContractID(_ context.Context, _, contractID string) (*envelope.SignatureEnvelope, error) {
	return f.byContract[contractID], nil
}

func (f *fakeRepo) GetByExternalID(_ context.Context, externalID string) (*envelope.SignatureEnvelope, error) {
	return f.byExternalID[externalID], nil
}

func (f *fakeRepo) GetByDocumentKey(_ context.Context, documentKey string) (*envelope.SignatureEnvelope, error) {
	return f.byDocumentKey[documentKey], nil
}

func (f *fakeRepo) ListStaleActive(_ context.Context, _ time.Time, _ int) ([]*envelope.SignatureEnvelope, error) {
	return f.staleActive, nil
}
func (f *fakeRepo) ListByTenant(_ context.Context, _ string, _ *envelope.Status) ([]*envelope.SignatureEnvelope, error) {
	return nil, nil
}

func (f *fakeRepo) AppendWebhookEvent(_ context.Context, _, _, _ string, _ []byte, _, _ string) (string, error) {
	return f.webhookID, nil
}

func (f *fakeRepo) MarkWebhookProcessed(_ context.Context, _ string, _ string) error {
	return nil
}

// reassociated registra os relinks de auditoria feitos pelo webhook — usado no
// teste do fallback por document.key.
func (f *fakeRepo) ReassociateWebhookEvent(_ context.Context, webhookID, externalEnvelopeID, tenantID string) error {
	if f.reassociateErr != nil {
		return f.reassociateErr
	}
	f.reassociated = append(f.reassociated, webhookRelink{webhookID, externalEnvelopeID, tenantID})
	return nil
}

type webhookRelink struct{ webhookID, externalEnvelopeID, tenantID string }

func (f *fakeRepo) ListWebhookEvents(_ context.Context, _ string) ([]envelope.WebhookEventRow, error) {
	return f.webhookEvents, nil
}

func copyEnv(e *envelope.SignatureEnvelope) *envelope.SignatureEnvelope {
	c := *e
	c.Signers = append([]envelope.Signer(nil), e.Signers...)
	return &c
}

type fakeClicksign struct {
	createCalls   int
	uploadCalls   int
	signerCalls   int
	reqCalls      int
	activateCalls int
	cancelCalls   int
	getCalls      int
	resendCalls   int
	listDocsCalls int

	createErr   error
	uploadErr   error
	signerErr   error
	reqErr      error
	activateErr error
	getErr      error
	resendErr   error
	listDocsErr error
	cancelErr   error
	// getStatus é o status que GetEnvelope devolve; vazio usa "running".
	getStatus string

	// cancelEnvelopeIDs registra o que o serviço pediu para cancelar.
	cancelEnvelopeIDs []string

	signerKeys []string // returned in order from AddSigner

	lastResendEnvelopeID string
	lastResendSignerKey  string

	// listDocsResponse lets tests stage the signed-file URL recovery
	// path exercised by Reconcile + the refresh endpoint (Phase 3
	// reconcile gap fix). nil → ListDocuments returns an empty list.
	listDocsResponse []clicksign.DocumentResource
}

func (f *fakeClicksign) CreateEnvelope(_ context.Context, _, _ string) (*clicksign.EnvelopeResource, error) {
	f.createCalls++
	if f.createErr != nil {
		return nil, f.createErr
	}
	return &clicksign.EnvelopeResource{ID: "cs-env-1", Type: "envelopes"}, nil
}

func (f *fakeClicksign) UploadDocument(_ context.Context, _, _ string, _ clicksign.DocumentRequest) (*clicksign.DocumentResource, error) {
	f.uploadCalls++
	if f.uploadErr != nil {
		return nil, f.uploadErr
	}
	return &clicksign.DocumentResource{ID: "cs-doc-1", Type: "documents"}, nil
}

func (f *fakeClicksign) UpdateSigner(_ context.Context, _, _, _ string, req clicksign.SignerRequest) (*clicksign.SignerResource, error) {
	return &clicksign.SignerResource{
		ID:   "cs-signer-default",
		Type: "signers",
		Attributes: clicksign.SignerAttributes{
			Name:  req.Name,
			Email: req.Email,
		},
	}, nil
}

func (f *fakeClicksign) AddSigner(_ context.Context, _, _ string, req clicksign.SignerRequest) (*clicksign.SignerResource, error) {
	f.signerCalls++
	if f.signerErr != nil {
		return nil, f.signerErr
	}
	id := "cs-signer-default"
	if len(f.signerKeys) >= f.signerCalls {
		id = f.signerKeys[f.signerCalls-1]
	}
	return &clicksign.SignerResource{
		ID:   id,
		Type: "signers",
		Attributes: clicksign.SignerAttributes{
			Name:                req.Name,
			Email:               req.Email,
			RequestSignatureKey: "rsk-" + id,
		},
	}, nil
}

func (f *fakeClicksign) AddRequirement(_ context.Context, _, _ string, _ clicksign.RequirementRequest, _ clicksign.RequirementRelationships) (*clicksign.RequirementResource, error) {
	f.reqCalls++
	if f.reqErr != nil {
		return nil, f.reqErr
	}
	return &clicksign.RequirementResource{ID: "cs-req", Type: "requirements"}, nil
}

func (f *fakeClicksign) ActivateEnvelope(_ context.Context, _, _ string) (*clicksign.EnvelopeResource, error) {
	f.activateCalls++
	if f.activateErr != nil {
		return nil, f.activateErr
	}
	return &clicksign.EnvelopeResource{ID: "cs-env-1", Type: "envelopes"}, nil
}

func (f *fakeClicksign) CancelEnvelope(_ context.Context, _, envelopeID string) error {
	f.cancelCalls++
	f.cancelEnvelopeIDs = append(f.cancelEnvelopeIDs, envelopeID)
	return f.cancelErr
}

// ListSigners returns an empty list in the fake — tests that need to
// exercise the per-signer drift reconcile path can override via a
// dedicated fake. Default behaviour is "no drift" so existing tests
// continue passing without modifications.
func (f *fakeClicksign) ListSigners(_ context.Context, _, _ string) ([]clicksign.SignerResource, error) {
	return nil, nil
}

func (f *fakeClicksign) ListDocuments(_ context.Context, _, _ string) ([]clicksign.DocumentResource, error) {
	f.listDocsCalls++
	if f.listDocsErr != nil {
		return nil, f.listDocsErr
	}
	if f.listDocsResponse == nil {
		return []clicksign.DocumentResource{}, nil
	}
	return f.listDocsResponse, nil
}

func (f *fakeClicksign) GetEnvelope(_ context.Context, _, envelopeID string) (*clicksign.EnvelopeResource, error) {
	f.getCalls++
	if f.getErr != nil {
		return nil, f.getErr
	}
	status := f.getStatus
	if status == "" {
		status = "running"
	}
	return &clicksign.EnvelopeResource{
		ID:         envelopeID,
		Type:       "envelopes",
		Attributes: clicksign.EnvelopeAttributes{Status: status},
	}, nil
}

func (f *fakeClicksign) ResendNotification(_ context.Context, _, envelopeID, signerKey string) error {
	f.resendCalls++
	f.lastResendEnvelopeID = envelopeID
	f.lastResendSignerKey = signerKey
	return f.resendErr
}

type fakeDeal struct {
	contract *clients.ContractDTO
	err      error
	// Attachment content keyed by document ID — tests opt into this only
	// when exercising the multi-document path.
	attachments map[string]fakeAttachment
}

type fakeAttachment struct {
	filename string
	content  []byte
	err      error
}

func (f *fakeDeal) GetContract(_ context.Context, _, _ string) (*clients.ContractDTO, error) {
	return f.contract, f.err
}

func (f *fakeDeal) FetchDocumentContent(_ context.Context, _, documentID string) (string, []byte, error) {
	att, ok := f.attachments[documentID]
	if !ok {
		return "", nil, fmt.Errorf("fake deal: no attachment for %s", documentID)
	}
	return att.filename, att.content, att.err
}

type stubCreds struct{}

func (s *stubCreds) Get(_ context.Context, _ string) (*credential.Credentials, error) {
	return &credential.Credentials{
		BaseURL:       "https://sandbox.clicksign.com",
		AccessToken:   "test",
		WebhookSecret: "secret",
		Environment:   "sandbox",
	}, nil
}

type stubPDF struct{}

func (s *stubPDF) Render(_ context.Context, _, _ string) ([]byte, error) {
	return []byte("%PDF-1.4\nfake-pdf-bytes"), nil
}

func sampleContract() *clients.ContractDTO {
	return &clients.ContractDTO{
		ID:        "contract-1",
		DisplayID: "CONT-001",
		Status:    "PENDING_SIGNATURE",
		DealID:    "deal-1",
		Type:      "locacao",
		Content:   "%PDF-fake",
		Signatories: []clients.ContractSignatoryDTO{
			{ID: "sig-1", Name: "Inquilino Teste", Email: "inq@example.com", Role: "inquilino"},
			{ID: "sig-2", Name: "Proprietario Teste", Email: "prop@example.com", Role: "proprietario"},
		},
	}
}

// ──────────────────────────── Tests ──────────────────────────────

func TestCreateForContract_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	cs := &fakeClicksign{signerKeys: []string{"cs-sig-1", "cs-sig-2"}}
	deal := &fakeDeal{contract: sampleContract()}

	svc := service.NewEnvelopeService(repo, cs, &stubCreds{}, deal, &stubPDF{}, nil)

	env, err := svc.CreateForContract(context.Background(), "tenant-1", "contract-1", service.CreateOptions{
		SignerAuth: map[string][]envelope.AuthMethod{
			"sig-1": {envelope.AuthEmailToken},
			"sig-2": {envelope.AuthEmailToken, envelope.AuthWhatsApp},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.Status != envelope.StatusRunning {
		t.Errorf("status = %s, want RUNNING", env.Status)
	}
	if env.ExternalEnvelopeID != "cs-env-1" {
		t.Errorf("external id = %s, want cs-env-1", env.ExternalEnvelopeID)
	}
	if len(env.Signers) != 2 {
		t.Fatalf("got %d signers, want 2", len(env.Signers))
	}
	if env.Signers[0].ClicksignSignerKey != "cs-sig-1" || env.Signers[1].ClicksignSignerKey != "cs-sig-2" {
		t.Errorf("signer keys not propagated: %+v", env.Signers)
	}
	if len(env.Documents) != 1 || env.Documents[0].ClicksignDocumentKey != "cs-doc-1" {
		t.Errorf("documents not persisted: %+v", env.Documents)
	}
	if cs.createCalls != 1 || cs.uploadCalls != 1 || cs.signerCalls != 2 || cs.activateCalls != 1 || cs.resendCalls != 1 {
		t.Errorf("clicksign call counts unexpected: %+v", cs)
	}
	if cs.lastResendEnvelopeID != "cs-env-1" || cs.lastResendSignerKey != "" {
		t.Errorf("notification target = envelope %q signer %q, want envelope-wide cs-env-1", cs.lastResendEnvelopeID, cs.lastResendSignerKey)
	}
	// sig-1: 1 qualification + 1 auth (email) = 2.
	// sig-2: 1 qualification + 2 auths (email, whatsapp) = 3.
	// Total: 5 requirement calls.
	if cs.reqCalls != 5 {
		t.Errorf("requirement calls = %d, want 5 (2 qualifications + 3 auths)", cs.reqCalls)
	}
}

func TestCreateForContract_Idempotent(t *testing.T) {
	repo := newFakeRepo()
	existing := mustNewEnvelope(t, "tenant-1", "contract-1", []envelope.Signer{{SignerID: "x", Name: "x", Email: "x@x", Role: envelope.SignerRoleSign, Auth: []envelope.AuthMethod{envelope.AuthEmailToken}}})
	existing.Status = envelope.StatusRunning
	existing.ExternalEnvelopeID = "already-created"
	repo.byContract["contract-1"] = existing

	cs := &fakeClicksign{}
	svc := service.NewEnvelopeService(repo, cs, &stubCreds{}, &fakeDeal{contract: sampleContract()}, &stubPDF{}, nil)

	got, err := svc.CreateForContract(context.Background(), "tenant-1", "contract-1", service.CreateOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != existing.ID {
		t.Errorf("expected existing envelope, got new one")
	}
	if cs.createCalls != 0 {
		t.Errorf("clicksign should not have been called, but createCalls = %d", cs.createCalls)
	}
}

func TestCreateForContract_AllowsRetryAfterError(t *testing.T) {
	repo := newFakeRepo()
	previous := mustNewEnvelope(t, "tenant-1", "contract-1", []envelope.Signer{{SignerID: "x", Name: "x", Email: "x@x", Role: envelope.SignerRoleSign, Auth: []envelope.AuthMethod{envelope.AuthEmailToken}}})
	previous.MarkError("prior failure")
	repo.byContract["contract-1"] = previous

	cs := &fakeClicksign{signerKeys: []string{"cs-sig-1", "cs-sig-2"}}
	svc := service.NewEnvelopeService(repo, cs, &stubCreds{}, &fakeDeal{contract: sampleContract()}, &stubPDF{}, nil)

	got, err := svc.CreateForContract(context.Background(), "tenant-1", "contract-1", service.CreateOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != envelope.StatusRunning {
		t.Errorf("status = %s, want RUNNING", got.Status)
	}
	if cs.createCalls == 0 {
		t.Errorf("clicksign should have been called for retry")
	}
}

func TestCreateForContract_DoesNotDuplicateFailedExternalEnvelope(t *testing.T) {
	repo := newFakeRepo()
	previous := mustNewEnvelope(t, "tenant-1", "contract-1", []envelope.Signer{{SignerID: "x", Name: "Test Name", Email: "x@x", Role: envelope.SignerRoleSign, Auth: []envelope.AuthMethod{envelope.AuthEmailToken}}})
	previous.MarkError("notification failed")
	previous.ExternalEnvelopeID = "already-created"
	repo.byContract["contract-1"] = previous
	cs := &fakeClicksign{}
	svc := service.NewEnvelopeService(repo, cs, &stubCreds{}, &fakeDeal{contract: sampleContract()}, &stubPDF{}, nil)
	if _, err := svc.CreateForContract(context.Background(), "tenant-1", "contract-1", service.CreateOptions{}); err == nil {
		t.Fatal("expected external envelope conflict")
	}
	if cs.createCalls != 0 {
		t.Fatal("duplicate external envelope created")
	}
}

func TestCreateForContract_DoesNotSendRegeneratedDraft(t *testing.T) {
	c := sampleContract()
	c.Status = "DRAFT"
	cs := &fakeClicksign{}
	repo := newFakeRepo()
	svc := service.NewEnvelopeService(repo, cs, &stubCreds{}, &fakeDeal{contract: c}, &stubPDF{}, nil)
	_, err := svc.CreateForContract(context.Background(), "tenant-1", "contract-1", service.CreateOptions{})
	if !errors.Is(err, service.ErrContractNotReadyForSignature) {
		t.Fatalf("expected draft guard, got %v", err)
	}
	if cs.createCalls != 0 {
		t.Fatal("draft sent automatically")
	}
}

func TestCreateForContract_PDFFailureMarksError(t *testing.T) {
	repo := newFakeRepo()
	cs := &fakeClicksign{}
	svc := service.NewEnvelopeService(repo, cs, &stubCreds{}, &fakeDeal{contract: sampleContract()}, errPDF{}, nil)

	env, err := svc.CreateForContract(context.Background(), "tenant-1", "contract-1", service.CreateOptions{})
	if err == nil {
		t.Fatalf("expected error from pdf fetch")
	}
	if env == nil || env.Status != envelope.StatusError {
		t.Errorf("expected envelope with status ERROR, got %+v", env)
	}
	if cs.createCalls != 0 {
		t.Errorf("clicksign should NOT have been called when PDF fails (got %d calls)", cs.createCalls)
	}
	if atomic.LoadInt32(&repo.createCalls) != 1 {
		t.Errorf("envelope should be persisted with ERROR status")
	}
}

func TestCreateForContract_ClicksignFailureLeavesError(t *testing.T) {
	repo := newFakeRepo()
	cs := &fakeClicksign{uploadErr: errors.New("boom")}
	svc := service.NewEnvelopeService(repo, cs, &stubCreds{}, &fakeDeal{contract: sampleContract()}, &stubPDF{}, nil)

	env, err := svc.CreateForContract(context.Background(), "tenant-1", "contract-1", service.CreateOptions{})
	if err == nil {
		t.Fatalf("expected error from upload")
	}
	if env.Status != envelope.StatusError {
		t.Errorf("status = %s, want ERROR", env.Status)
	}
	if env.LastError == "" {
		t.Errorf("last_error should be populated")
	}
	if env.ExternalEnvelopeID != "cs-env-1" {
		t.Errorf("external id should still be set after partial failure")
	}
}

func TestCreateForContract_NotificationFailureLeavesError(t *testing.T) {
	repo := newFakeRepo()
	cs := &fakeClicksign{resendErr: errors.New("notification endpoint down")}
	svc := service.NewEnvelopeService(repo, cs, &stubCreds{}, &fakeDeal{contract: sampleContract()}, &stubPDF{}, nil)

	env, err := svc.CreateForContract(context.Background(), "tenant-1", "contract-1", service.CreateOptions{})
	if err == nil {
		t.Fatalf("expected error from notification send")
	}
	if env.Status != envelope.StatusError {
		t.Errorf("status = %s, want ERROR", env.Status)
	}
	if env.LastError == "" {
		t.Errorf("last_error should be populated")
	}
	if cs.activateCalls != 1 || cs.resendCalls != 1 {
		t.Errorf("activate/resend calls = %d/%d, want 1/1", cs.activateCalls, cs.resendCalls)
	}
}

func TestCreateForContract_NoSignatoriesRejected(t *testing.T) {
	repo := newFakeRepo()
	cs := &fakeClicksign{}
	deal := &fakeDeal{contract: &clients.ContractDTO{ID: "x", Signatories: nil}}
	svc := service.NewEnvelopeService(repo, cs, &stubCreds{}, deal, &stubPDF{}, nil)

	_, err := svc.CreateForContract(context.Background(), "tenant-1", "contract-1", service.CreateOptions{})
	if err == nil {
		t.Fatalf("expected error for empty signatories")
	}
}

func TestResendNotification_EnvelopeWide(t *testing.T) {
	repo := newFakeRepo()
	env := mustNewEnvelope(t, "tenant-1", "contract-1", []envelope.Signer{
		{SignerID: "sig-1", Name: "n", Email: "e@x", Role: envelope.SignerRoleSign, Auth: []envelope.AuthMethod{envelope.AuthEmailToken}, ClicksignSignerKey: "cs-sig-1"},
	})
	env.Status = envelope.StatusRunning
	env.ExternalEnvelopeID = "cs-env-1"
	repo.byID[env.ID] = env

	cs := &fakeClicksign{}
	svc := service.NewEnvelopeService(repo, cs, &stubCreds{}, &fakeDeal{contract: sampleContract()}, &stubPDF{}, nil)

	if _, err := svc.ResendNotification(context.Background(), "tenant-1", env.ID, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cs.resendCalls != 1 {
		t.Errorf("resendCalls = %d, want 1", cs.resendCalls)
	}
	if cs.lastResendSignerKey != "" {
		t.Errorf("expected envelope-wide resend, got signer key %q", cs.lastResendSignerKey)
	}
	if cs.lastResendEnvelopeID != "cs-env-1" {
		t.Errorf("envelopeID = %q, want cs-env-1", cs.lastResendEnvelopeID)
	}
}

func TestResendNotification_PerSigner(t *testing.T) {
	repo := newFakeRepo()
	env := mustNewEnvelope(t, "tenant-1", "contract-1", []envelope.Signer{
		{SignerID: "sig-1", Name: "n", Email: "e@x", Role: envelope.SignerRoleSign, Auth: []envelope.AuthMethod{envelope.AuthEmailToken}, ClicksignSignerKey: "cs-sig-1"},
		{SignerID: "sig-2", Name: "n2", Email: "e2@x", Role: envelope.SignerRoleSign, Auth: []envelope.AuthMethod{envelope.AuthEmailToken}, ClicksignSignerKey: "cs-sig-2"},
	})
	env.Status = envelope.StatusPartiallySigned
	env.ExternalEnvelopeID = "cs-env-1"
	repo.byID[env.ID] = env

	cs := &fakeClicksign{}
	svc := service.NewEnvelopeService(repo, cs, &stubCreds{}, &fakeDeal{contract: sampleContract()}, &stubPDF{}, nil)

	if _, err := svc.ResendNotification(context.Background(), "tenant-1", env.ID, "sig-2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cs.lastResendSignerKey != "cs-sig-2" {
		t.Errorf("signer key = %q, want cs-sig-2", cs.lastResendSignerKey)
	}
}

func TestResendNotification_EnvelopeNotFound(t *testing.T) {
	repo := newFakeRepo()
	cs := &fakeClicksign{}
	svc := service.NewEnvelopeService(repo, cs, &stubCreds{}, &fakeDeal{contract: sampleContract()}, &stubPDF{}, nil)

	_, err := svc.ResendNotification(context.Background(), "tenant-1", "missing", "")
	if !errors.Is(err, service.ErrEnvelopeNotFound) {
		t.Errorf("err = %v, want ErrEnvelopeNotFound", err)
	}
	if cs.resendCalls != 0 {
		t.Errorf("clicksign should not be called when envelope is missing")
	}
}

func TestResendNotification_SignerNotFound(t *testing.T) {
	repo := newFakeRepo()
	env := mustNewEnvelope(t, "tenant-1", "contract-1", []envelope.Signer{
		{SignerID: "sig-1", Name: "n", Email: "e@x", Role: envelope.SignerRoleSign, Auth: []envelope.AuthMethod{envelope.AuthEmailToken}, ClicksignSignerKey: "cs-sig-1"},
	})
	env.Status = envelope.StatusRunning
	env.ExternalEnvelopeID = "cs-env-1"
	repo.byID[env.ID] = env

	cs := &fakeClicksign{}
	svc := service.NewEnvelopeService(repo, cs, &stubCreds{}, &fakeDeal{contract: sampleContract()}, &stubPDF{}, nil)

	_, err := svc.ResendNotification(context.Background(), "tenant-1", env.ID, "ghost-signer")
	if !errors.Is(err, service.ErrSignerNotFound) {
		t.Errorf("err = %v, want ErrSignerNotFound", err)
	}
}

func TestResendNotification_RejectsTerminalStatus(t *testing.T) {
	terminal := []envelope.Status{
		envelope.StatusFinished,
		envelope.StatusCanceled,
		envelope.StatusRefused,
		envelope.StatusExpired,
		envelope.StatusError,
	}
	for _, status := range terminal {
		t.Run(string(status), func(t *testing.T) {
			repo := newFakeRepo()
			env := mustNewEnvelope(t, "tenant-1", "contract-1", []envelope.Signer{
				{SignerID: "sig-1", Name: "n", Email: "e@x", Role: envelope.SignerRoleSign, Auth: []envelope.AuthMethod{envelope.AuthEmailToken}, ClicksignSignerKey: "cs-sig-1"},
			})
			env.Status = status
			env.ExternalEnvelopeID = "cs-env-1"
			repo.byID[env.ID] = env

			cs := &fakeClicksign{}
			svc := service.NewEnvelopeService(repo, cs, &stubCreds{}, &fakeDeal{contract: sampleContract()}, &stubPDF{}, nil)

			_, err := svc.ResendNotification(context.Background(), "tenant-1", env.ID, "")
			if err == nil {
				t.Fatalf("expected error for terminal status %s", status)
			}
			if cs.resendCalls != 0 {
				t.Errorf("clicksign should not be called for terminal status %s", status)
			}
		})
	}
}

func TestCancel_TerminatesEnvelope(t *testing.T) {
	repo := newFakeRepo()
	env := mustNewEnvelope(t, "tenant-1", "contract-1", []envelope.Signer{
		{SignerID: "sig-1", Name: "n", Email: "e@x", Role: envelope.SignerRoleSign, Auth: []envelope.AuthMethod{envelope.AuthEmailToken}},
	})
	env.Status = envelope.StatusRunning
	env.ExternalEnvelopeID = "cs-env-1"
	repo.byID[env.ID] = env

	cs := &fakeClicksign{}
	svc := service.NewEnvelopeService(repo, cs, &stubCreds{}, &fakeDeal{contract: sampleContract()}, &stubPDF{}, nil)

	got, err := svc.Cancel(context.Background(), "tenant-1", env.ID)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if got.Status != envelope.StatusCanceled {
		t.Errorf("status = %s, want CANCELED", got.Status)
	}
	if cs.cancelCalls != 1 {
		t.Errorf("clicksign Cancel calls = %d, want 1", cs.cancelCalls)
	}
}

// ──────────────────────────── helpers ──────────────────────────────

type errPDF struct{}

func (errPDF) Render(_ context.Context, _, _ string) ([]byte, error) {
	return nil, errors.New("pdf rendering not configured")
}

func mustNewEnvelope(t *testing.T, tenantID, contractID string, signers []envelope.Signer) *envelope.SignatureEnvelope {
	t.Helper()
	e, err := envelope.New(tenantID, contractID, "deal-1", "sandbox", signers)
	if err != nil {
		t.Fatalf("envelope.New: %v", err)
	}
	e.CreatedAt = time.Now().Add(-time.Hour)
	return e
}
