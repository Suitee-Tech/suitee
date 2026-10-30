package handlers_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/events"
	pkgevents "github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-signature/internal/api/v1/handlers"
	"github.com/oginoino/toca/toca-signature/internal/clicksign"
	"github.com/oginoino/toca/toca-signature/internal/domain/credential"
	"github.com/oginoino/toca/toca-signature/internal/domain/envelope"
	"github.com/oginoino/toca/toca-signature/internal/service"
)

func init() {
	gin.SetMode(gin.TestMode)
}

const testSecret = "shared-test-secret"

// ──────────────────────────── Fakes ──────────────────────────────

type stubCreds struct{}

func (stubCreds) Get(_ context.Context, _ string) (*credential.Credentials, error) {
	return &credential.Credentials{
		BaseURL:       "https://sandbox.clicksign.com",
		AccessToken:   "tok",
		WebhookSecret: testSecret,
		Environment:   "sandbox",
	}, nil
}

type stubBus struct {
	mu        sync.Mutex
	published []events.Event
}

func (b *stubBus) Publish(_ context.Context, e events.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = append(b.published, e)
	return nil
}

func (b *stubBus) PublishReliable(ctx context.Context, e events.Event) error {
	return b.Publish(ctx, e)
}

func (b *stubBus) Subscribe(_ string, _ events.EventHandler) error { return nil }

func (b *stubBus) SubscribeWithQueue(_ string, _ string, _ events.EventHandler) error {
	return nil
}

func (b *stubBus) names() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, 0, len(b.published))
	for _, e := range b.published {
		out = append(out, e.Name())
	}
	return out
}

type fakeRepo struct {
	envByExternal map[string]*envelope.SignatureEnvelope
	updates       []*envelope.SignatureEnvelope
	webhooks      []string
	// relinked guarda (webhookID → envelope externo/tenant) depois do fallback
	// por document key — a linha de auditoria precisa apontar para o envelope
	// real, senão some da tela de webhooks (tenant_id NULL / doc key).
	relinked []string
}

func (f *fakeRepo) Create(_ context.Context, _ *envelope.SignatureEnvelope) error { return nil }
func (f *fakeRepo) Update(_ context.Context, e *envelope.SignatureEnvelope) error {
	f.updates = append(f.updates, e)
	f.envByExternal[e.ExternalEnvelopeID] = e
	return nil
}

// UpdateInTx delegates to Update for the fake. The test setup does not
// spin up Postgres so the tx pointer is unused; production behaviour is
// covered by the Postgres impl and integration tests.
func (f *fakeRepo) UpdateInTx(ctx context.Context, _ *sqlx.Tx, e *envelope.SignatureEnvelope) error {
	return f.Update(ctx, e)
}
func (f *fakeRepo) GetByID(_ context.Context, _, _ string) (*envelope.SignatureEnvelope, error) {
	return nil, nil
}
func (f *fakeRepo) GetByContractID(_ context.Context, _, _ string) (*envelope.SignatureEnvelope, error) {
	return nil, nil
}

// GetByIDService é o caminho sem JWT (varredura); o fake serve os mesmos
// envelopes, ignorando tenant como o pool service-role faz.
func (f *fakeRepo) GetByIDService(_ context.Context, id string) (*envelope.SignatureEnvelope, error) {
	for _, e := range f.envByExternal {
		if e.ID == id {
			return e, nil
		}
	}
	return nil, nil
}

func (f *fakeRepo) UpdateService(ctx context.Context, e *envelope.SignatureEnvelope) error {
	return f.Update(ctx, e)
}
func (f *fakeRepo) GetByExternalID(_ context.Context, externalID string) (*envelope.SignatureEnvelope, error) {
	return f.envByExternal[externalID], nil
}
func (f *fakeRepo) GetByDocumentKey(_ context.Context, key string) (*envelope.SignatureEnvelope, error) {
	for _, e := range f.envByExternal {
		for i := range e.Documents {
			if e.Documents[i].ClicksignDocumentKey == key {
				return e, nil
			}
		}
		if e.Document != nil && e.Document.ClicksignDocumentKey == key {
			return e, nil
		}
	}
	return nil, nil
}
func (f *fakeRepo) ListStaleActive(_ context.Context, _ time.Time, _ int) ([]*envelope.SignatureEnvelope, error) {
	return nil, nil
}
func (f *fakeRepo) ListByTenant(_ context.Context, _ string, _ *envelope.Status) ([]*envelope.SignatureEnvelope, error) {
	return nil, nil
}
func (f *fakeRepo) AppendWebhookEvent(_ context.Context, externalID, eventType, deliveryID string, _ []byte, _, _ string) (string, error) {
	id := externalID + ":" + eventType + ":" + deliveryID
	f.webhooks = append(f.webhooks, id)
	return id, nil
}
func (f *fakeRepo) MarkWebhookProcessed(_ context.Context, _ string, _ string) error { return nil }

func (f *fakeRepo) ReassociateWebhookEvent(_ context.Context, webhookID, externalEnvelopeID, tenantID string) error {
	f.relinked = append(f.relinked, webhookID+"→"+externalEnvelopeID+"/"+tenantID)
	return nil
}
func (f *fakeRepo) ListWebhookEvents(_ context.Context, _ string) ([]envelope.WebhookEventRow, error) {
	return nil, nil
}

// ──────────────────────────── Helpers ──────────────────────────────

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	wd, _ := os.Getwd()
	// handlers package lives at toca-signature/internal/api/v1/handlers — go up four levels.
	root := filepath.Join(wd, "..", "..", "..", "..")
	body, err := os.ReadFile(filepath.Join(root, "testdata", "clicksign-webhooks", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return body
}

func newRouter(t *testing.T, repo *fakeRepo, bus events.EventBus, allowedIPs []string) *gin.Engine {
	t.Helper()
	svc := service.NewWebhookService(repo, bus)
	h := handlers.NewWebhookHandler(svc, stubCreds{}, allowedIPs)
	r := gin.New()
	r.POST("/webhooks/clicksign", h.Receive)
	return r
}

func envelopeWithSigners(externalID string, signerKeys ...string) *envelope.SignatureEnvelope {
	e := &envelope.SignatureEnvelope{
		ID:                 "local-env-1",
		TenantID:           "tenant-1",
		ContractID:         "contract-1",
		DealID:             "deal-1",
		ExternalEnvelopeID: externalID,
		Environment:        "sandbox",
		Status:             envelope.StatusRunning,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
	for _, k := range signerKeys {
		e.Signers = append(e.Signers, envelope.Signer{
			SignerID:           "internal-" + k,
			ClicksignSignerKey: k,
			Email:              k + "@example.com",
			Status:             envelope.SignerStatusPending,
			Auth:               []envelope.AuthMethod{envelope.AuthEmailToken},
			Role:               envelope.SignerRoleSign,
		})
	}
	return e
}

func doWebhookRequest(t *testing.T, r *gin.Engine, body []byte, opts ...func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/clicksign", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Hmac", clicksign.SignBody(body, testSecret))
	for _, opt := range opts {
		opt(req)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ──────────────────────────── Tests ──────────────────────────────

func TestWebhook_SignEvent_PublishesSignerSigned(t *testing.T) {
	repo := &fakeRepo{envByExternal: map[string]*envelope.SignatureEnvelope{
		"cs-env-1": envelopeWithSigners("cs-env-1", "cs-sig-1", "cs-sig-2"),
	}}
	bus := &stubBus{}
	r := newRouter(t, repo, bus, nil)

	w := doWebhookRequest(t, r, loadFixture(t, "sign.json"))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if got := bus.names(); len(got) != 1 || got[0] != "signature.signer_signed" {
		t.Errorf("expected single signature.signer_signed event, got %v", got)
	}
}

// TestWebhook_SignEvent_V3PayloadShape covers the real Clicksign v3 wire
// format: the envelope identifier arrives as document.key and the event name
// uses the "Event::Sign" Ruby-style notation. Before the audit-first refactor
// in webhook_service this combination produced a silent 200 with no row in
// webhook_events — which is what we saw in production (envelope
// f7931cc8-... never updated despite Clicksign dashboard showing all 200s).
func TestWebhook_SignEvent_V3PayloadShape(t *testing.T) {
	repo := &fakeRepo{envByExternal: map[string]*envelope.SignatureEnvelope{
		"cs-env-1": envelopeWithSigners("cs-env-1", "cs-sig-1"),
	}}
	bus := &stubBus{}
	r := newRouter(t, repo, bus, nil)

	w := doWebhookRequest(t, r, loadFixture(t, "sign_v3.json"))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	names := bus.names()
	// One signer, signing → signer_signed + envelope_finished (all signed).
	if len(names) != 2 || names[0] != "signature.signer_signed" || names[1] != "signature.envelope_finished" {
		t.Errorf("expected signer_signed + envelope_finished, got %v", names)
	}
}

// TestWebhook_SignEvent_DocumentKeyMismatchFallback covers the production
// failure mode we hit in May 2026: the Clicksign v3 webhook sends
// document.key (which is the document UUID), but the envelope was stored
// with a DIFFERENT UUID in external_envelope_id (the envelope UUID returned
// from POST /api/v3/envelopes). With only GetByExternalID, the lookup
// failed and every event landed on "envelope not found locally". With the
// GetByDocumentKey fallback, it resolves correctly.
func TestWebhook_SignEvent_DocumentKeyMismatchFallback(t *testing.T) {
	// Envelope stored locally with external_envelope_id="real-envelope-uuid"
	// but its uploaded document has key="cs-env-1" (matches the fixture's
	// document.key). The webhook will only find it via the document fallback.
	env := envelopeWithSigners("real-envelope-uuid", "cs-sig-1")
	env.Documents = []envelope.Document{{
		ClicksignDocumentKey: "cs-env-1",
	}}
	repo := &fakeRepo{envByExternal: map[string]*envelope.SignatureEnvelope{
		"real-envelope-uuid": env,
	}}
	bus := &stubBus{}
	r := newRouter(t, repo, bus, nil)

	w := doWebhookRequest(t, r, loadFixture(t, "sign_v3.json"))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	names := bus.names()
	if len(names) != 2 || names[0] != "signature.signer_signed" || names[1] != "signature.envelope_finished" {
		t.Errorf("expected fallback lookup to publish signer_signed + envelope_finished, got %v", names)
	}
	// A linha de auditoria nasceu com o document.key (tenant_id NULL). Depois do
	// fallback ela tem de apontar para o envelope real.
	if len(repo.relinked) != 1 || !strings.HasSuffix(repo.relinked[0], "→real-envelope-uuid/tenant-1") {
		t.Errorf("relink de auditoria = %v, quero terminar em →real-envelope-uuid/tenant-1", repo.relinked)
	}
}

func TestWebhook_AutoCloseEvent_PublishesEnvelopeFinished(t *testing.T) {
	env := envelopeWithSigners("cs-env-1", "cs-sig-1", "cs-sig-2")
	now := time.Now()
	for i := range env.Signers {
		env.Signers[i].Status = envelope.SignerStatusSigned
		env.Signers[i].SignedAt = &now
	}
	env.Status = envelope.StatusFinished
	repo := &fakeRepo{envByExternal: map[string]*envelope.SignatureEnvelope{"cs-env-1": env}}
	bus := &stubBus{}
	r := newRouter(t, repo, bus, nil)

	w := doWebhookRequest(t, r, loadFixture(t, "auto_close.json"))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	got := bus.names()
	if len(got) != 1 || got[0] != "signature.envelope_finished" {
		t.Errorf("expected signature.envelope_finished, got %v", got)
	}
	// Verify the SignedDocumentURL made it through.
	finished := bus.published[0].Payload().(pkgevents.SignatureEnvelopeFinishedEventPayload)
	if finished.SignedDocumentURL == "" {
		t.Errorf("expected signed document URL to be propagated")
	}
}

func TestWebhook_SignEvent_AllSigned_PublishesBoth(t *testing.T) {
	env := envelopeWithSigners("cs-env-1", "cs-sig-1")
	repo := &fakeRepo{envByExternal: map[string]*envelope.SignatureEnvelope{"cs-env-1": env}}
	bus := &stubBus{}
	r := newRouter(t, repo, bus, nil)

	w := doWebhookRequest(t, r, loadFixture(t, "sign.json"))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	names := bus.names()
	if len(names) != 2 || names[0] != "signature.signer_signed" || names[1] != "signature.envelope_finished" {
		t.Errorf("expected signer_signed + envelope_finished, got %v", names)
	}
}

func TestWebhook_RefusalEvent_PublishesRefused(t *testing.T) {
	repo := &fakeRepo{envByExternal: map[string]*envelope.SignatureEnvelope{
		"cs-env-1": envelopeWithSigners("cs-env-1", "cs-sig-1", "cs-sig-2"),
	}}
	bus := &stubBus{}
	r := newRouter(t, repo, bus, nil)

	w := doWebhookRequest(t, r, loadFixture(t, "refusal.json"))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	got := bus.names()
	if len(got) != 1 || got[0] != "signature.refused" {
		t.Errorf("expected signature.refused, got %v", got)
	}
}

func TestWebhook_DeadlineEvent_MarksExpiredAndPublishesExpired(t *testing.T) {
	env := envelopeWithSigners("cs-env-1", "cs-sig-1", "cs-sig-2")
	repo := &fakeRepo{envByExternal: map[string]*envelope.SignatureEnvelope{"cs-env-1": env}}
	bus := &stubBus{}
	r := newRouter(t, repo, bus, nil)

	w := doWebhookRequest(t, r, loadFixture(t, "deadline.json"))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if env.Status != envelope.StatusExpired {
		t.Errorf("envelope status = %s, want EXPIRED", env.Status)
	}
	got := bus.names()
	if len(got) != 1 || got[0] != "signature.expired" {
		t.Fatalf("expected signature.expired for deadline, got %v", got)
	}
	expired := bus.published[0].Payload().(pkgevents.SignatureExpiredEventPayload)
	if expired.ExpiredAt == "" {
		t.Errorf("expired_at should be populated, got empty")
	}
	if expired.ContractID != env.ContractID || expired.ExternalEnvelopeID != env.ExternalEnvelopeID {
		t.Errorf("payload identifiers wrong: %+v", expired)
	}
}

func TestWebhook_ExpiredEvent_MarksExpiredAndPublishesExpired(t *testing.T) {
	env := envelopeWithSigners("cs-env-1", "cs-sig-1")
	repo := &fakeRepo{envByExternal: map[string]*envelope.SignatureEnvelope{"cs-env-1": env}}
	bus := &stubBus{}
	r := newRouter(t, repo, bus, nil)

	w := doWebhookRequest(t, r, loadFixture(t, "expired.json"))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if env.Status != envelope.StatusExpired {
		t.Errorf("envelope status = %s, want EXPIRED", env.Status)
	}
	if got := bus.names(); len(got) != 1 || got[0] != "signature.expired" {
		t.Fatalf("expected signature.expired for expired event, got %v", got)
	}
}

func TestWebhook_BadHMAC_Returns401(t *testing.T) {
	repo := &fakeRepo{envByExternal: map[string]*envelope.SignatureEnvelope{}}
	bus := &stubBus{}
	r := newRouter(t, repo, bus, nil)

	body := loadFixture(t, "sign.json")
	req := httptest.NewRequest(http.MethodPost, "/webhooks/clicksign", bytes.NewReader(body))
	req.Header.Set("Content-Hmac", "sha256=wrong")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
	if len(bus.published) != 0 {
		t.Errorf("no events should be published on bad HMAC")
	}
}

func TestWebhook_IPNotAllowed_Returns403(t *testing.T) {
	repo := &fakeRepo{envByExternal: map[string]*envelope.SignatureEnvelope{}}
	bus := &stubBus{}
	r := newRouter(t, repo, bus, []string{"203.0.113.1"})

	w := doWebhookRequest(t, r, loadFixture(t, "sign.json"))
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

func TestWebhook_CIDRAllowlist_AllowsMatchingRange(t *testing.T) {
	// httptest sends requests with RemoteAddr=192.0.2.1 by default. A CIDR
	// that covers that range should let the request through (and fail later
	// at the envelope lookup, returning 200 because there's no orphan env).
	repo := &fakeRepo{envByExternal: map[string]*envelope.SignatureEnvelope{}}
	bus := &stubBus{}
	r := newRouter(t, repo, bus, []string{"192.0.2.0/24"})

	w := doWebhookRequest(t, r, loadFixture(t, "sign.json"))
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (CIDR should have admitted the request)", w.Code)
	}
}

func TestWebhook_CIDRAllowlist_RejectsOutsideRange(t *testing.T) {
	repo := &fakeRepo{envByExternal: map[string]*envelope.SignatureEnvelope{}}
	bus := &stubBus{}
	r := newRouter(t, repo, bus, []string{"10.0.0.0/8"})

	w := doWebhookRequest(t, r, loadFixture(t, "sign.json"))
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (192.0.2.1 is outside 10.0.0.0/8)", w.Code)
	}
}

func TestWebhook_OrphanEnvelope_StillReturns200(t *testing.T) {
	// envByExternal is empty — webhook arrives for an envelope we don't
	// know about. Should still return 200 to prevent Clicksign retries.
	repo := &fakeRepo{envByExternal: map[string]*envelope.SignatureEnvelope{}}
	bus := &stubBus{}
	r := newRouter(t, repo, bus, nil)

	w := doWebhookRequest(t, r, loadFixture(t, "sign.json"))
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if len(bus.published) != 0 {
		t.Errorf("no events should be published for orphan envelope")
	}
}

func TestWebhook_MalformedBody_Returns400(t *testing.T) {
	repo := &fakeRepo{envByExternal: map[string]*envelope.SignatureEnvelope{}}
	bus := &stubBus{}
	r := newRouter(t, repo, bus, nil)

	body := []byte(`not json`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/clicksign", bytes.NewReader(body))
	req.Header.Set("Content-Hmac", clicksign.SignBody(body, testSecret))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}
