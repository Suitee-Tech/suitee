package handlers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	appcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
)

type proofStorageStub struct {
	uploadErr      error
	deleteErr      error
	uploadedBucket string
	uploadedName   string
	uploadedType   string
	uploadedBody   []byte
	deletedName    string
	downloadErr    error
	downloadName   string
	downloadBody   []byte
}

func (s *proofStorageStub) Upload(_ context.Context, bucket, filename, contentType string, content io.Reader) (string, error) {
	s.uploadedBucket = bucket
	s.uploadedName = filename
	s.uploadedType = contentType
	s.uploadedBody, _ = io.ReadAll(content)
	return "", s.uploadErr
}

func (s *proofStorageStub) Delete(_ context.Context, _ string, filename string) error {
	s.deletedName = filename
	return s.deleteErr
}

func (s *proofStorageStub) GetURL(_, _ string) string { return "" }

func (s *proofStorageStub) GetPresignedURL(_ context.Context, _, _ string, _ time.Duration) (string, error) {
	return "", errors.New("presigned URLs are not used by financial proofs")
}

func (s *proofStorageStub) GetPresignedPutURL(context.Context, string, string, time.Duration) (string, error) {
	return "", errors.New("not implemented")
}

func (s *proofStorageStub) IsPrivate(bucket string) bool { return bucket == ProofBucket }

func (s *proofStorageStub) Download(_ context.Context, _, filename string) (io.ReadCloser, string, int64, error) {
	s.downloadName = filename
	if s.downloadErr != nil {
		return nil, "", 0, s.downloadErr
	}
	body := append([]byte(nil), s.downloadBody...)
	return io.NopCloser(bytes.NewReader(body)), "application/pdf", int64(len(body)), nil
}

func proofUploadRequest(t *testing.T, kind, filename string, content []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("kind", kind); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/proofs", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	ctx := appcontext.WithTenantID(req.Context(), "tenant-a")
	return req.WithContext(ctx)
}

func runProofUpload(t *testing.T, handler *ProofUploadHandler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = req
	handler.Upload(c)
	return recorder
}

func TestProofUploadStoresPrivateTenantScopedObject(t *testing.T) {
	storage := &proofStorageStub{}
	pdf := []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\n%%EOF")

	response := runProofUpload(t, NewProofUploadHandler(storage), proofUploadRequest(t, "commission", "comprovante.exe", pdf))

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if storage.uploadedBucket != ProofBucket {
		t.Errorf("bucket = %q, want %q", storage.uploadedBucket, ProofBucket)
	}
	if !strings.HasPrefix(storage.uploadedName, "tenant-a/commission/") || !strings.HasSuffix(storage.uploadedName, ".pdf") {
		t.Errorf("object name = %q, want tenant-scoped canonical PDF path", storage.uploadedName)
	}
	if storage.uploadedType != "application/pdf" {
		t.Errorf("content type = %q, want application/pdf", storage.uploadedType)
	}
	if !bytes.Equal(storage.uploadedBody, pdf) {
		t.Error("uploaded body differs from request body")
	}
	if !strings.Contains(response.Body.String(), `"object_key":"tenant-a/commission/`) {
		t.Fatalf("response does not contain the object key: %s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), "token=signed") {
		t.Fatal("upload response must not expose a storage signed URL")
	}
	if !strings.Contains(response.Body.String(), ProofContentRoute) {
		t.Fatalf("response does not contain the authenticated content route: %s", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"filename":"comprovante.exe"`) {
		t.Fatalf("response should preserve the original filename: %s", response.Body.String())
	}
}

// recordingEventBus captura publicações para o teste do publish.
type recordingEventBus struct {
	published  []events.Event
	publishErr error
}

func (b *recordingEventBus) Publish(_ context.Context, _ events.Event) error { return nil }
func (b *recordingEventBus) Subscribe(_ string, _ events.EventHandler) error { return nil }
func (b *recordingEventBus) SubscribeWithQueue(_, _ string, _ events.EventHandler) error {
	return nil
}
func (b *recordingEventBus) PublishReliable(_ context.Context, evt events.Event) error {
	b.published = append(b.published, evt)
	return b.publishErr
}

func TestProofUploadPublishesDocumentMaterializationEvent(t *testing.T) {
	storage := &proofStorageStub{}
	bus := &recordingEventBus{}
	pdf := []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\n%%EOF")

	handler := NewProofUploadHandler(storage).WithEventBus(bus)
	response := runProofUpload(t, handler, proofUploadRequest(t, "commission", "comprovante.pdf", pdf))

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if len(bus.published) != 1 {
		t.Fatalf("published events = %d, want 1", len(bus.published))
	}
	evt, ok := bus.published[0].(contracts.FinancialProofUploadedEvent)
	if !ok {
		t.Fatalf("event type = %T, want contracts.FinancialProofUploadedEvent", bus.published[0])
	}
	if evt.Name() != "financial.proof_uploaded" {
		t.Errorf("event name = %q", evt.Name())
	}
	if evt.Data.TenantID != "tenant-a" || evt.Data.Kind != "commission" || evt.Data.ObjectKey == "" {
		t.Errorf("payload incomplete: %+v", evt.Data)
	}
	if !strings.HasPrefix(evt.Data.URL, ProofContentRoute) {
		t.Errorf("url = %q, want internal content route", evt.Data.URL)
	}
}

func TestProofUploadPublishFailureDoesNotFailUpload(t *testing.T) {
	storage := &proofStorageStub{}
	bus := &recordingEventBus{publishErr: errors.New("rabbitmq down")}
	pdf := []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\n%%EOF")

	handler := NewProofUploadHandler(storage).WithEventBus(bus)
	response := runProofUpload(t, handler, proofUploadRequest(t, "payout", "comprovante.pdf", pdf))
	// O publish é best-effort: falha de barramento não desfaz o upload.
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestProofUploadRejectsUnsupportedContent(t *testing.T) {
	storage := &proofStorageStub{}
	response := runProofUpload(t, NewProofUploadHandler(storage), proofUploadRequest(t, "commission", "fake.pdf", []byte("not a PDF")))

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if storage.uploadedName != "" {
		t.Fatal("unsupported content must not be uploaded")
	}
}

func TestProofUploadHidesStorageFailureAndReturnsServiceUnavailable(t *testing.T) {
	storage := &proofStorageStub{uploadErr: errors.New("internal bucket detail")}
	response := runProofUpload(t, NewProofUploadHandler(storage), proofUploadRequest(t, "commission", "proof.pdf", []byte("%PDF-1.4\n%%EOF")))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "internal bucket detail") {
		t.Fatalf("internal storage error leaked to client: %s", response.Body.String())
	}
}

func proofViewRequest(t *testing.T, tenantID, objectKey string) *http.Request {
	t.Helper()
	token := strings.TrimPrefix(proofContentURL(objectKey), ProofContentRoute)
	req := httptest.NewRequest(http.MethodGet, ProofContentRoute+token, nil)
	ctx := appcontext.WithTenantID(req.Context(), tenantID)
	return req.WithContext(ctx)
}

func TestProofViewStreamsOnlyTheCurrentTenantObject(t *testing.T) {
	storage := &proofStorageStub{downloadBody: []byte("%PDF-1.4\n%%EOF")}
	handler := NewProofUploadHandler(storage)
	objectKey := "tenant-a/commission/2026-08/550e8400-e29b-41d4-a716-446655440000.pdf"
	req := proofViewRequest(t, "tenant-a", objectKey)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = req
	c.Params = gin.Params{{Key: "token", Value: strings.TrimPrefix(proofContentURL(objectKey), ProofContentRoute)}}
	handler.View(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Body.String(); got != string(storage.downloadBody) {
		t.Errorf("body = %q, want %q", got, storage.downloadBody)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Errorf("Cache-Control = %q", got)
	}
	if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/pdf") {
		t.Errorf("Content-Type = %q", got)
	}
	if storage.downloadName != objectKey {
		t.Errorf("download object = %q, want %q", storage.downloadName, objectKey)
	}
}

func TestProofViewDoesNotCrossTenantBoundary(t *testing.T) {
	storage := &proofStorageStub{downloadBody: []byte("secret")}
	objectKey := "tenant-a/commission/2026-08/550e8400-e29b-41d4-a716-446655440000.pdf"
	req := proofViewRequest(t, "tenant-b", objectKey)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = req
	c.Params = gin.Params{{Key: "token", Value: strings.TrimPrefix(proofContentURL(objectKey), ProofContentRoute)}}
	NewProofUploadHandler(storage).View(c)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if storage.downloadName != "" {
		t.Fatal("cross-tenant proof must not reach storage")
	}
}

func TestProofViewRequiresTenantContext(t *testing.T) {
	storage := &proofStorageStub{}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/proofs/content/token", nil)
	c.Params = gin.Params{{Key: "token", Value: "invalid"}}
	NewProofUploadHandler(storage).View(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}
