package services_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/virusscan"
	"github.com/oginoino/toca/toca-deal/internal/domain/document"
	service "github.com/oginoino/toca/toca-deal/internal/services"
)

// fakeDocumentRepo collects the documents passed to Update so tests can
// assert on what was persisted. Other methods return zero values; tests
// using them populate `docs` directly.
type fakeDocumentRepo struct {
	docs    []*document.Document
	updates []*document.Document
	creates []*document.Document
}

func (r *fakeDocumentRepo) Create(_ context.Context, d *document.Document) error {
	r.creates = append(r.creates, d)
	r.docs = append(r.docs, d)
	return nil
}

// GetByID espelha o contrato tenant-scoped do repositório real (achado C11).
func (r *fakeDocumentRepo) GetByID(_ context.Context, tenantID, id string) (*document.Document, error) {
	// Espelha o repositório real (achado C11): tenant vazio é recusado
	// (fail-closed) e tenant diferente do dono não encontra o documento.
	if strings.TrimSpace(tenantID) == "" {
		return nil, errors.New("document not found: tenant is required")
	}
	for _, d := range r.docs {
		if d.ID == id && d.TenantID == tenantID {
			return d, nil
		}
	}
	return nil, nil
}
func (r *fakeDocumentRepo) Update(_ context.Context, d *document.Document) error {
	r.updates = append(r.updates, d)
	for i, x := range r.docs {
		if x.ID == d.ID {
			r.docs[i] = d
		}
	}
	return nil
}
func (r *fakeDocumentRepo) Delete(_ context.Context, _, _ string) error { return nil }
func (r *fakeDocumentRepo) ListByContract(_ context.Context, contractID, tenantID string) ([]*document.Document, error) {
	var out []*document.Document
	for _, d := range r.docs {
		if d.ContractID == contractID && d.TenantID == tenantID {
			out = append(out, d)
		}
	}
	return out, nil
}
func (r *fakeDocumentRepo) ListByEntity(_ context.Context, entityID string, entityType document.EntityType, tenantID string) ([]*document.Document, error) {
	var out []*document.Document
	for _, d := range r.docs {
		if d.EntityID == entityID && d.EntityType == entityType && d.TenantID == tenantID {
			out = append(out, d)
		}
	}
	return out, nil
}
func (r *fakeDocumentRepo) List(_ context.Context, tenantID string, filters document.ListFilters) ([]*document.Document, int64, error) {
	var out []*document.Document
	for _, d := range r.docs {
		if d.TenantID != tenantID {
			continue
		}
		if filters.Status != "" && d.Status != filters.Status {
			continue
		}
		out = append(out, d)
	}
	return out, int64(len(out)), nil
}
func (r *fakeDocumentRepo) Search(_ context.Context, _, _ string, _, _ int64) ([]*document.Document, int64, error) {
	return nil, 0, nil
}
func (r *fakeDocumentRepo) DistinctTags(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

// fakeVirusScanner devolve um veredito controlado (P1-13). Por padrão tudo
// é limpo; testes de vírus/indisponibilidade configuram err/clean.
type fakeVirusScanner struct {
	clean   bool
	err     error
	scanned [][]byte
}

func (f *fakeVirusScanner) Scan(_ context.Context, r io.Reader) (virusscan.Result, error) {
	b, _ := io.ReadAll(r)
	f.scanned = append(f.scanned, b)
	if f.err != nil {
		return virusscan.Result{}, f.err
	}
	if !f.clean {
		return virusscan.Result{Clean: false, Signature: "Eicar-Test-Signature"}, fmt.Errorf("%w: Eicar-Test-Signature", virusscan.ErrVirusFound)
	}
	return virusscan.Result{Clean: true}, nil
}

// fakeStorage captures Upload bytes and returns a deterministic URL so the
// test can verify the wiring without touching a real backend.
type fakeStorage struct {
	uploaded     []byte
	uploadKey    string
	uploadCalls  int
	presignCalls int
	uploadErr    error
	presignedURL string
	presignErr   error
	internalURL  string
	resolveCalls int
}

func (s *fakeStorage) Upload(_ context.Context, _, filename, _ string, content io.Reader) (string, error) {
	s.uploadCalls++
	if s.uploadErr != nil {
		return "", s.uploadErr
	}
	data, err := io.ReadAll(content)
	if err != nil {
		return "", err
	}
	s.uploaded = data
	s.uploadKey = filename
	return "http://fake-storage/" + filename, nil
}
func (s *fakeStorage) Delete(_ context.Context, _, _ string) error { return nil }
func (s *fakeStorage) GetURL(_, filename string) string            { return "http://fake-storage/" + filename }
func (s *fakeStorage) GetPresignedURL(_ context.Context, _, filename string, _ time.Duration) (string, error) {
	s.presignCalls++
	if s.presignErr != nil {
		return "", s.presignErr
	}
	if s.presignedURL != "" {
		return s.presignedURL, nil
	}
	return "http://fake-storage/presigned/" + filename, nil
}
func (s *fakeStorage) GetPresignedPutURL(_ context.Context, _, _ string, _ time.Duration) (string, error) {
	return "", nil
}
func (s *fakeStorage) IsPrivate(_ string) bool { return true }
func (s *fakeStorage) ResolveInternalURL(raw string) (string, error) {
	s.resolveCalls++
	if s.internalURL != "" {
		return s.internalURL, nil
	}
	return raw, nil
}

// buildFakeSignedPDF returns bytes that pass document.ValidateSignedPDF
// (size >= 1KB, %PDF- prefix, %%EOF in trailing window). Used by the
// PersistSignedContractPDF tests so the new structural validation
// doesn't reject test fixtures.
func buildFakeSignedPDF(contractID string) []byte {
	out := []byte("%PDF-1.4\n% fake signed contract ")
	out = append(out, []byte(contractID)...)
	// Pad to clear the 1KB minimum size threshold.
	pad := bytes.Repeat([]byte("padding "), 200)
	out = append(out, pad...)
	out = append(out, []byte("\n%%EOF\n")...)
	return out
}

func TestPersistSignedContractPDF_DownloadsAndStores(t *testing.T) {
	pdfBody := buildFakeSignedPDF("contract-1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(pdfBody)
	}))
	defer srv.Close()

	repo := &fakeDocumentRepo{
		docs: []*document.Document{{
			ID:         "doc-1",
			TenantID:   "tenant-1",
			ContractID: "contract-1",
			Category:   document.CategoryContrato,
			Status:     document.StatusRascunho,
			Name:       "Contrato deal-42",
		}},
	}
	stor := &fakeStorage{}
	svc := service.NewDocumentService(repo, stor).WithVirusScanner(&fakeVirusScanner{clean: true})

	if err := svc.PersistSignedContractPDF(context.Background(), "tenant-1", "contract-1", srv.URL); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stor.uploadCalls != 1 {
		t.Errorf("Upload calls = %d, want 1", stor.uploadCalls)
	}
	if string(stor.uploaded) != string(pdfBody) {
		t.Errorf("uploaded body mismatch: got %q want %q", string(stor.uploaded), string(pdfBody))
	}
	got := repo.docs[0]
	if got.StorageKey == "" || got.FileURL == "" {
		t.Errorf("expected StorageKey and FileURL to be set, got %+v", got)
	}
	if got.Status != document.StatusAssinado {
		t.Errorf("status = %s, want assinado", got.Status)
	}
	if !strings.Contains(strings.ToLower(got.Name), "assinado") {
		t.Errorf("expected name to contain 'assinado', got %q", got.Name)
	}
	// Persistence pipeline: Upload bumps the status to `stored` and
	// stamps the attempt timestamp so the backfill job can ignore this
	// row going forward.
	if got.PersistenceStatus != document.PersistenceStored {
		t.Errorf("PersistenceStatus = %q, want stored", got.PersistenceStatus)
	}
	if got.LastPersistError != "" {
		t.Errorf("LastPersistError should be empty on success, got %q", got.LastPersistError)
	}
	if got.PersistAttemptedAt == nil {
		t.Errorf("PersistAttemptedAt should be set after successful persist")
	}
	// Integrity: FileHash must match the SHA-256 of the bytes the storage
	// layer received. If this drifts, signed PDFs could be silently
	// swapped in MinIO without us being able to detect it.
	sum := sha256.Sum256(pdfBody)
	wantHash := hex.EncodeToString(sum[:])
	if got.FileHash != wantHash {
		t.Errorf("FileHash mismatch: got %q want %q", got.FileHash, wantHash)
	}
}

func TestPersistSignedContractPDF_IdempotentOnStorageKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Errorf("HTTP should not be hit when the signed PDF is already persisted")
		w.WriteHeader(500)
	}))
	defer srv.Close()

	sourceSum := sha256.Sum256([]byte(srv.URL))
	repo := &fakeDocumentRepo{
		docs: []*document.Document{{
			ID:               "doc-1",
			TenantID:         "tenant-1",
			ContractID:       "contract-1",
			Category:         document.CategoryContrato,
			Status:           document.StatusAssinado,
			StorageKey:       "already/uploaded.pdf",
			SignedSourceHash: hex.EncodeToString(sourceSum[:]),
			// FileType must be the PDF mime — só pula o download quando o
			// row já carrega o PDF assinado. StorageKey != "" sozinho era
			// o bug que causava o DOCX rascunho a contar como "persistido".
			FileType: "application/pdf",
			FileURL:  "http://fake-storage/already/uploaded.pdf",
		}},
	}
	stor := &fakeStorage{}
	svc := service.NewDocumentService(repo, stor).WithVirusScanner(&fakeVirusScanner{clean: true})

	if err := svc.PersistSignedContractPDF(context.Background(), "tenant-1", "contract-1", srv.URL); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stor.uploadCalls != 0 {
		t.Errorf("Upload should not be called on idempotent re-run, got %d calls", stor.uploadCalls)
	}
}

// Regenerar/enviar novamente depois de cancelar cria um PDF novo. O guard de
// idempotência não pode confundir a tentativa antiga com a final; a linha
// antiga (sem source hash) deve ser substituída pelo PDF do envelope atual.
func TestPersistSignedContractPDF_ReplacesStalePDFFromPreviousAttempt(t *testing.T) {
	pdfBody := buildFakeSignedPDF("contract-1-new")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(pdfBody)
	}))
	defer srv.Close()

	repo := &fakeDocumentRepo{
		docs: []*document.Document{{
			ID:         "doc-1",
			TenantID:   "tenant-1",
			ContractID: "contract-1",
			Category:   document.CategoryContrato,
			Status:     document.StatusAssinado,
			StorageKey: "tenant-1/contrato/old-canceled.pdf",
			FileType:   "application/pdf",
			FileURL:    "http://fake-storage/tenant-1/contrato/old-canceled.pdf",
			FileHash:   "old-canceled-hash",
		}},
	}
	stor := &fakeStorage{}
	svc := service.NewDocumentService(repo, stor).WithVirusScanner(&fakeVirusScanner{clean: true})

	if err := svc.PersistSignedContractPDF(context.Background(), "tenant-1", "contract-1", srv.URL); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stor.uploadCalls != 1 {
		t.Fatalf("Upload calls = %d, want 1 para substituir o PDF cancelado", stor.uploadCalls)
	}
	got := repo.docs[0]
	if got.FileHash == "old-canceled-hash" {
		t.Fatal("o PDF cancelado continuou salvo após o novo envelope finalizar")
	}
	sum := sha256.Sum256(pdfBody)
	if got.FileHash != hex.EncodeToString(sum[:]) {
		t.Fatalf("FileHash = %q, want hash do PDF novo", got.FileHash)
	}
	sourceSum := sha256.Sum256([]byte(srv.URL))
	if got.SignedSourceHash != hex.EncodeToString(sourceSum[:]) {
		t.Fatalf("SignedSourceHash = %q, want origem do envelope atual", got.SignedSourceHash)
	}
}

// TestPersistSignedContractPDF_ReplacesDocxDraftWithSignedPDF cobre o bug em
// que o rascunho DOCX, persistido por UploadDocxForContract, era tratado
// como "PDF assinado já persistido" pelo guarda de idempotência. O webhook
// envelope_finished então não baixava o PDF e o usuário via o DOCX no
// SuperDoc em vez do PDF da Clicksign.
func TestPersistSignedContractPDF_ReplacesDocxDraftWithSignedPDF(t *testing.T) {
	pdfBody := buildFakeSignedPDF("contract-1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(pdfBody)
	}))
	defer srv.Close()

	const docxKey = "tenant-1/contrato/abcd1234_rascunho-contract-1-deadbeef.docx"
	const docxMime = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	repo := &fakeDocumentRepo{
		docs: []*document.Document{{
			ID:         "doc-1",
			TenantID:   "tenant-1",
			ContractID: "contract-1",
			Category:   document.CategoryContrato,
			Status:     document.StatusPendenteAssinatura,
			Name:       "Contrato deal-42",
			StorageKey: docxKey,
			FileType:   docxMime,
			FileURL:    "http://fake-storage/" + docxKey,
		}},
	}
	stor := &fakeStorage{}
	svc := service.NewDocumentService(repo, stor).WithVirusScanner(&fakeVirusScanner{clean: true})

	if err := svc.PersistSignedContractPDF(context.Background(), "tenant-1", "contract-1", srv.URL); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stor.uploadCalls != 1 {
		t.Fatalf("Upload calls = %d, want 1 (DOCX must be replaced by signed PDF)", stor.uploadCalls)
	}
	got := repo.docs[0]
	if got.FileType != "application/pdf" {
		t.Errorf("FileType after persist = %q, want application/pdf", got.FileType)
	}
	if got.StorageKey == "" || got.StorageKey == docxKey {
		t.Errorf("StorageKey should be replaced with the new PDF key, still = %q", got.StorageKey)
	}
	if got.Status != document.StatusAssinado {
		t.Errorf("Status = %s, want assinado", got.Status)
	}
	sum := sha256.Sum256(pdfBody)
	wantHash := hex.EncodeToString(sum[:])
	if got.FileHash != wantHash {
		t.Errorf("FileHash mismatch: got %q want %q", got.FileHash, wantHash)
	}
	if !strings.Contains(strings.ToLower(got.Name), "assinado") {
		t.Errorf("expected name to contain 'assinado', got %q", got.Name)
	}
}

func TestPersistSignedContractPDF_AutoCreatesPlaceholder(t *testing.T) {
	pdfBody := buildFakeSignedPDF("contract-1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(pdfBody)
	}))
	defer srv.Close()

	repo := &fakeDocumentRepo{}
	stor := &fakeStorage{}
	svc := service.NewDocumentService(repo, stor).WithVirusScanner(&fakeVirusScanner{clean: true})

	if err := svc.PersistSignedContractPDF(context.Background(), "tenant-1", "contract-1", srv.URL); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.creates) != 1 {
		t.Fatalf("expected placeholder Document creation, got %d creates", len(repo.creates))
	}
	if repo.creates[0].Category != document.CategoryContrato {
		t.Errorf("expected placeholder category=contrato, got %s", repo.creates[0].Category)
	}
	if stor.uploadCalls != 1 {
		t.Errorf("expected Upload to run after placeholder created, got %d", stor.uploadCalls)
	}
}

// TestPersistSignedContractPDF_RejectsNonPDFPayload exercises Fase 3/S4
// — Clicksign occasionally responds with an HTML error page carrying
// `Content-Type: application/pdf`. ValidateSignedPDF should catch this
// before the bytes hit storage so operators don't end up with a
// "signed" contract that opens as an error page.
func TestPersistSignedContractPDF_RejectsNonPDFPayload(t *testing.T) {
	html := bytes.Repeat([]byte("<html><body>Bad Gateway</body></html>"), 50)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(html)
	}))
	defer srv.Close()

	repo := &fakeDocumentRepo{
		docs: []*document.Document{{
			ID:         "doc-1",
			TenantID:   "tenant-1",
			ContractID: "contract-1",
			Category:   document.CategoryContrato,
			Status:     document.StatusRascunho,
		}},
	}
	stor := &fakeStorage{}
	svc := service.NewDocumentService(repo, stor).WithVirusScanner(&fakeVirusScanner{clean: true})

	err := svc.PersistSignedContractPDF(context.Background(), "tenant-1", "contract-1", srv.URL)
	if err == nil {
		t.Fatalf("expected error when payload is not a real PDF")
	}
	if !errors.Is(err, document.ErrInvalidPDF) {
		t.Errorf("expected ErrInvalidPDF, got %v", err)
	}
	if stor.uploadCalls != 0 {
		t.Errorf("Upload must not be called when validation fails — got %d calls", stor.uploadCalls)
	}
	got := repo.docs[0]
	if got.PersistenceStatus != document.PersistenceFailed {
		t.Errorf("PersistenceStatus = %q, want failed", got.PersistenceStatus)
	}
}

func TestPersistSignedContractPDF_HTTPErrorPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	repo := &fakeDocumentRepo{
		docs: []*document.Document{{
			ID:         "doc-1",
			TenantID:   "tenant-1",
			ContractID: "contract-1",
			Category:   document.CategoryContrato,
			Status:     document.StatusRascunho,
		}},
	}
	stor := &fakeStorage{}
	svc := service.NewDocumentService(repo, stor).WithVirusScanner(&fakeVirusScanner{clean: true})

	err := svc.PersistSignedContractPDF(context.Background(), "tenant-1", "contract-1", srv.URL)
	if err == nil {
		t.Fatalf("expected error from 500 response")
	}
	if stor.uploadCalls != 0 {
		t.Errorf("Upload should not be called when download failed")
	}
	// Persistence failure must be stamped on the document so the
	// backfill job can find it.
	got := repo.docs[0]
	if got.PersistenceStatus != document.PersistenceFailed {
		t.Errorf("PersistenceStatus = %q, want failed", got.PersistenceStatus)
	}
	if got.LastPersistError == "" {
		t.Errorf("LastPersistError should be populated after failure")
	}
	if got.PersistAttemptedAt == nil {
		t.Errorf("PersistAttemptedAt should be set after failed persist")
	}
}

func TestPersistSignedContractPDF_StorageUploadErrorPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("pdf"))
	}))
	defer srv.Close()

	repo := &fakeDocumentRepo{
		docs: []*document.Document{{
			ID:         "doc-1",
			TenantID:   "tenant-1",
			ContractID: "contract-1",
			Category:   document.CategoryContrato,
		}},
	}
	stor := &fakeStorage{uploadErr: errors.New("minio down")}
	svc := service.NewDocumentService(repo, stor).WithVirusScanner(&fakeVirusScanner{clean: true})

	if err := svc.PersistSignedContractPDF(context.Background(), "tenant-1", "contract-1", srv.URL); err == nil {
		t.Fatalf("expected error when storage upload fails")
	}
}

func TestPersistSignedContractPDF_EmptySourceURLNoOp(t *testing.T) {
	repo := &fakeDocumentRepo{}
	stor := &fakeStorage{}
	svc := service.NewDocumentService(repo, stor).WithVirusScanner(&fakeVirusScanner{clean: true})

	if err := svc.PersistSignedContractPDF(context.Background(), "tenant-1", "contract-1", ""); err != nil {
		t.Fatalf("empty URL should be a no-op, got %v", err)
	}
	if stor.uploadCalls != 0 || len(repo.creates) != 0 {
		t.Errorf("no calls expected for empty URL, got upload=%d creates=%d", stor.uploadCalls, len(repo.creates))
	}
}

func TestGetSignedFileURL_ReturnsPresignedWhenStored(t *testing.T) {
	repo := &fakeDocumentRepo{
		docs: []*document.Document{{
			ID:         "doc-1",
			TenantID:   "tenant-1",
			ContractID: "contract-1",
			Category:   document.CategoryContrato,
			StorageKey: "tenant-1/contrato/abc.pdf",
			FileType:   "application/pdf",
		}},
	}
	stor := &fakeStorage{}
	svc := service.NewDocumentService(repo, stor).WithVirusScanner(&fakeVirusScanner{clean: true})

	url, err := svc.GetSignedFileURL(context.Background(), "tenant-1", "contract-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url == "" {
		t.Errorf("expected non-empty presigned URL")
	}
	if stor.presignCalls != 1 {
		t.Errorf("GetPresignedURL calls = %d, want 1", stor.presignCalls)
	}
}

func TestGetFileContent_UsesPresignedURLForStoredPrivateDocument(t *testing.T) {
	const fileBody = "%PDF-1.4\nprivate document\n%%EOF"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte(fileBody))
	}))
	defer server.Close()

	repo := &fakeDocumentRepo{
		docs: []*document.Document{{
			ID:         "doc-private",
			TenantID:   "tenant-1",
			StorageKey: "tenant-1/contracts/private.pdf",
			FileURL:    "https://supabase.example/storage/v1/object/authenticated/toca-documents/tenant-1/contracts/private.pdf",
			FileType:   "application/pdf",
		}},
	}
	stor := &fakeStorage{
		presignedURL: "https://public-storage.example.test/document.pdf?token=signed",
		internalURL:  server.URL + "?token=signed",
	}
	svc := service.NewDocumentService(repo, stor).WithVirusScanner(&fakeVirusScanner{clean: true})

	// SECURITY (C11): o chamador é resolvido dentro do tenant do documento.
	doc, content, err := svc.GetFileContent(pkgcontext.WithTenantID(context.Background(), "tenant-1"), "doc-private")
	if err != nil {
		t.Fatalf("GetFileContent() error = %v", err)
	}
	if doc.ID != "doc-private" {
		t.Errorf("document ID = %q, want doc-private", doc.ID)
	}
	if string(content) != fileBody {
		t.Errorf("content = %q, want %q", string(content), fileBody)
	}
	if stor.presignCalls != 1 {
		t.Errorf("GetPresignedURL calls = %d, want 1", stor.presignCalls)
	}
	if stor.resolveCalls != 1 {
		t.Errorf("ResolveInternalURL calls = %d, want 1", stor.resolveCalls)
	}
}

func TestGetDownloadURL_ReturnsExternalFileWhenStorageKeyIsEmpty(t *testing.T) {
	const externalURL = "https://files.example.test/inspection/report.pdf"
	repo := &fakeDocumentRepo{
		docs: []*document.Document{{
			ID:         "doc-external",
			TenantID:   "tenant-1",
			ContractID: "contract-1",
			Category:   document.CategoryVistoria,
			FileURL:    externalURL,
		}},
	}
	stor := &fakeStorage{}
	svc := service.NewDocumentService(repo, stor).WithVirusScanner(&fakeVirusScanner{clean: true})

	got, err := svc.GetDownloadURL(context.Background(), "doc-external", "tenant-1")
	if err != nil {
		t.Fatalf("GetDownloadURL() error = %v", err)
	}
	if got != externalURL {
		t.Errorf("GetDownloadURL() = %q, want %q", got, externalURL)
	}
	if stor.presignCalls != 0 {
		t.Errorf("GetPresignedURL calls = %d, want 0", stor.presignCalls)
	}
}

func TestGetDownloadURL_PropagatesSigningFailureAfterTenantCheck(t *testing.T) {
	repo := &fakeDocumentRepo{
		docs: []*document.Document{{
			ID:         "doc-private",
			TenantID:   "tenant-1",
			StorageKey: "tenant-1/contracts/private.pdf",
		}},
	}
	stor := &fakeStorage{presignErr: errors.New("storage unavailable")}
	svc := service.NewDocumentService(repo, stor).WithVirusScanner(&fakeVirusScanner{clean: true})

	if _, err := svc.GetDownloadURL(context.Background(), "doc-private", "tenant-1"); err == nil ||
		!strings.Contains(err.Error(), "storage unavailable") {
		t.Fatalf("GetDownloadURL() error = %v, want storage failure", err)
	}
	if stor.presignCalls != 1 {
		t.Errorf("GetPresignedURL calls = %d, want 1", stor.presignCalls)
	}

	stor.presignCalls = 0
	// SECURITY (C11): o documento de outro tenant agora nem é carregado —
	// respondemos "não encontrado" em vez de "access denied", para não
	// confirmar a existência do registro alheio.
	if _, err := svc.GetDownloadURL(context.Background(), "doc-private", "tenant-2"); err == nil ||
		!strings.Contains(err.Error(), "document not found") {
		t.Fatalf("cross-tenant GetDownloadURL() error = %v, want document not found", err)
	}
	if stor.presignCalls != 0 {
		t.Errorf("cross-tenant request called GetPresignedURL %d times, want 0", stor.presignCalls)
	}
}

func TestGetSignedFileURL_EmptyWhenStillDocxDraft(t *testing.T) {
	// Before PersistSignedContractPDF runs, the contract Document still holds
	// the editable DOCX draft. GetSignedFileURL must NOT hand that back as the
	// "signed file" — otherwise the viewer opens the draft in the editor.
	const docxMime = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	repo := &fakeDocumentRepo{
		docs: []*document.Document{{
			ID:         "doc-1",
			TenantID:   "tenant-1",
			ContractID: "contract-1",
			Category:   document.CategoryContrato,
			StorageKey: "tenant-1/contrato/abcd1234_rascunho-contract-1.docx",
			FileType:   docxMime,
		}},
	}
	stor := &fakeStorage{}
	svc := service.NewDocumentService(repo, stor).WithVirusScanner(&fakeVirusScanner{clean: true})

	url, err := svc.GetSignedFileURL(context.Background(), "tenant-1", "contract-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "" {
		t.Errorf("expected empty URL for a DOCX draft, got %q", url)
	}
	if stor.presignCalls != 0 {
		t.Errorf("GetPresignedURL should not be called for a DOCX draft, got %d", stor.presignCalls)
	}
}

func TestNewSignedPDFPersistHandler_CallsPersist(t *testing.T) {
	pdfBody := buildFakeSignedPDF("contract-1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(pdfBody)
	}))
	defer srv.Close()

	repo := &fakeDocumentRepo{
		docs: []*document.Document{{
			ID:         "doc-1",
			TenantID:   "tenant-1",
			ContractID: "contract-1",
			Category:   document.CategoryContrato,
		}},
	}
	stor := &fakeStorage{}
	docs := service.NewDocumentService(repo, stor).WithVirusScanner(&fakeVirusScanner{clean: true})

	handler := service.NewSignedPDFPersistHandler(docs, nil)
	evt := stubEvent{
		name: "signature.envelope_finished",
		payload: map[string]interface{}{
			"tenant_id":           "tenant-1",
			"contract_id":         "contract-1",
			"signed_document_url": srv.URL,
		},
	}
	if err := handler(context.Background(), evt); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if stor.uploadCalls != 1 {
		t.Errorf("expected Upload to be invoked via handler, got %d calls", stor.uploadCalls)
	}
}

func TestNewSignedPDFPersistHandler_MissingFieldsNoOp(t *testing.T) {
	repo := &fakeDocumentRepo{}
	stor := &fakeStorage{}
	docs := service.NewDocumentService(repo, stor).WithVirusScanner(&fakeVirusScanner{clean: true})
	handler := service.NewSignedPDFPersistHandler(docs, nil)

	// Missing contract_id: handler logs and returns nil (no redelivery).
	err := handler(context.Background(), stubEvent{
		name: "signature.envelope_finished",
		payload: map[string]interface{}{
			"tenant_id":           "tenant-1",
			"signed_document_url": "http://does-not-matter",
		},
	})
	if err != nil {
		t.Errorf("missing contract_id should not propagate as error (avoid redelivery loop), got %v", err)
	}
	if stor.uploadCalls != 0 {
		t.Errorf("no Upload should occur with missing fields")
	}
}

func TestNewSignedPDFPersistHandler_EmptySignedURLNoOp(t *testing.T) {
	repo := &fakeDocumentRepo{}
	stor := &fakeStorage{}
	docs := service.NewDocumentService(repo, stor).WithVirusScanner(&fakeVirusScanner{clean: true})
	handler := service.NewSignedPDFPersistHandler(docs, nil)

	// Clicksign occasionally publishes envelope_finished before the signed
	// file URL is ready. The handler must no-op gracefully.
	err := handler(context.Background(), stubEvent{
		name: "signature.envelope_finished",
		payload: map[string]interface{}{
			"tenant_id":           "tenant-1",
			"contract_id":         "contract-1",
			"signed_document_url": "",
		},
	})
	if err != nil {
		t.Errorf("empty signed URL should be a no-op, got %v", err)
	}
}

func TestGetSignedFileURL_EmptyWhenNoStorageKey(t *testing.T) {
	repo := &fakeDocumentRepo{
		docs: []*document.Document{{
			ID:         "doc-1",
			TenantID:   "tenant-1",
			ContractID: "contract-1",
			Category:   document.CategoryContrato,
			// StorageKey deliberately empty — placeholder document before
			// the signed PDF has been persisted.
		}},
	}
	stor := &fakeStorage{}
	svc := service.NewDocumentService(repo, stor).WithVirusScanner(&fakeVirusScanner{clean: true})

	url, err := svc.GetSignedFileURL(context.Background(), "tenant-1", "contract-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "" {
		t.Errorf("expected empty URL when StorageKey not set, got %q", url)
	}
	if stor.presignCalls != 0 {
		t.Errorf("GetPresignedURL should not be called when StorageKey empty")
	}
}

// fakeContractLookup satisfies ContractSignedURLLookup for tests that
// exercise the deal+contract document aggregation path.
type fakeContractLookup struct {
	contractIDsByDeal map[string][]string
}

func (f *fakeContractLookup) GetSignedDocumentURL(_ context.Context, _, _ string) (string, error) {
	return "", nil
}
func (f *fakeContractLookup) ListContractIDsByDeal(_ context.Context, _, dealID string) ([]string, error) {
	return f.contractIDsByDeal[dealID], nil
}
func (f *fakeContractLookup) RefreshSignedDocumentURLFromRemote(_ context.Context, _, _ string) (string, error) {
	return "", nil
}

func TestListByDealIncludingContracts_MergesDealAndContractDocs(t *testing.T) {
	repo := &fakeDocumentRepo{
		docs: []*document.Document{
			// Direct deal-linked doc (e.g. checklist attachment promoted
			// to the central repo)
			{
				ID:         "doc-deal-1",
				TenantID:   "tenant-1",
				EntityID:   "deal-42",
				EntityType: document.EntityDeal,
				Name:       "Comprovante de residência",
			},
			// Contract-linked doc (signed PDF persisted via webhook)
			{
				ID:         "doc-contract-signed",
				TenantID:   "tenant-1",
				ContractID: "contract-7",
				EntityID:   "contract-7",
				EntityType: document.EntityContract,
				Category:   document.CategoryContrato,
				Status:     document.StatusAssinado,
				Name:       "Contrato assinado",
			},
			// Doc belonging to another tenant — must be filtered out
			{
				ID:         "doc-other-tenant",
				TenantID:   "tenant-99",
				EntityID:   "deal-42",
				EntityType: document.EntityDeal,
			},
		},
	}
	svc := service.NewDocumentService(repo, &fakeStorage{})
	svc.SetContractLookup(&fakeContractLookup{
		contractIDsByDeal: map[string][]string{
			"deal-42": {"contract-7"},
		},
	})

	got, err := svc.ListByDealIncludingContracts(context.Background(), "tenant-1", "deal-42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 documents (1 deal + 1 contract), got %d: %+v", len(got), got)
	}
	ids := map[string]bool{got[0].ID: true, got[1].ID: true}
	if !ids["doc-deal-1"] || !ids["doc-contract-signed"] {
		t.Errorf("expected both deal-linked and contract-linked docs, got %v", ids)
	}
}

func TestListByDealIncludingContracts_NoContractLookupReturnsDealOnly(t *testing.T) {
	repo := &fakeDocumentRepo{
		docs: []*document.Document{
			{
				ID:         "doc-deal-1",
				TenantID:   "tenant-1",
				EntityID:   "deal-42",
				EntityType: document.EntityDeal,
			},
		},
	}
	svc := service.NewDocumentService(repo, &fakeStorage{})
	// Intentionally NOT calling SetContractLookup — older test wirings
	// shouldn't crash, they should just see the deal-scoped subset.

	got, err := svc.ListByDealIncludingContracts(context.Background(), "tenant-1", "deal-42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "doc-deal-1" {
		t.Errorf("expected deal-only fallback to return [doc-deal-1], got %+v", got)
	}
}

func TestEnsureSignedPDFURL_RefreshesLegacyDocumentWithoutSourceHash(t *testing.T) {
	pdfBody := buildFakeSignedPDF("contract-legacy")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(pdfBody)
	}))
	defer srv.Close()

	repo := &fakeDocumentRepo{
		docs: []*document.Document{{
			ID:         "doc-legacy",
			TenantID:   "tenant-1",
			ContractID: "contract-legacy",
			Category:   document.CategoryContrato,
			Status:     document.StatusAssinado,
			StorageKey: "tenant-1/contrato/canceled.pdf",
			FileType:   "application/pdf",
			// Sem SignedSourceHash: linha persistida antes do versionamento,
			// possivelmente pelo envelope cancelado.
		}},
	}
	stor := &fakeStorage{}
	lookup := &fakeContractLookupSigned{
		refreshedSignedURLByContract: map[string]string{"contract-legacy": srv.URL},
	}
	svc := service.NewDocumentService(repo, stor).WithVirusScanner(&fakeVirusScanner{clean: true})
	svc.SetContractLookup(lookup)

	url, err := svc.EnsureSignedPDFURL(context.Background(), "tenant-1", "contract-legacy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url == "" {
		t.Fatal("expected signed URL")
	}
	if lookup.refreshCalls != 1 || stor.uploadCalls != 1 {
		t.Fatalf("legacy PDF not refreshed: refreshCalls=%d uploadCalls=%d", lookup.refreshCalls, stor.uploadCalls)
	}
	if repo.docs[0].SignedSourceHash == "" {
		t.Fatal("legacy document should receive the current source hash after repair")
	}
}

// ── Backfill (Fase 3/S6) ──

// fakeContractLookupSigned satisfies ContractSignedURLLookup with a
// fixed signed-URL map per contract — used by BackfillUnstoredSignedPDFs
// tests where the lookup needs to return a real httptest URL.
type fakeContractLookupSigned struct {
	signedURLByContract map[string]string
	contractIDsByDeal   map[string][]string
	// refreshedSignedURLByContract is what RefreshSignedDocumentURLFromRemote
	// returns — the live Clicksign lookup. The backfill always uses this:
	// the cached signedURLByContract would be a stale ~5 min S3 link.
	refreshedSignedURLByContract map[string]string
	refreshCalls                 int
}

func (f *fakeContractLookupSigned) GetSignedDocumentURL(_ context.Context, _, contractID string) (string, error) {
	return f.signedURLByContract[contractID], nil
}
func (f *fakeContractLookupSigned) ListContractIDsByDeal(_ context.Context, _, dealID string) ([]string, error) {
	return f.contractIDsByDeal[dealID], nil
}
func (f *fakeContractLookupSigned) RefreshSignedDocumentURLFromRemote(_ context.Context, _, contractID string) (string, error) {
	f.refreshCalls++
	return f.refreshedSignedURLByContract[contractID], nil
}

func TestBackfillUnstoredSignedPDFs_PicksUpFailedRows(t *testing.T) {
	pdfBody := buildFakeSignedPDF("contract-stuck")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(pdfBody)
	}))
	defer srv.Close()

	repo := &fakeDocumentRepo{
		docs: []*document.Document{
			// Stuck signed doc — earlier persist attempt failed.
			{
				ID:                "doc-stuck",
				TenantID:          "tenant-1",
				ContractID:        "contract-stuck",
				Category:          document.CategoryContrato,
				Status:            document.StatusAssinado,
				PersistenceStatus: document.PersistenceFailed,
				LastPersistError:  "previous: download timeout",
			},
			// Already-stored doc — should not be reattempted. Carrega o PDF
			// assinado (FileType=application/pdf), que é o invariante que
			// o backfill agora usa para distinguir "PDF persistido" de
			// "só o rascunho DOCX em storage".
			{
				ID:                "doc-ok",
				TenantID:          "tenant-1",
				ContractID:        "contract-ok",
				Category:          document.CategoryContrato,
				Status:            document.StatusAssinado,
				StorageKey:        "tenant-1/contrato/cafe1234_already.pdf",
				FileType:          "application/pdf",
				PersistenceStatus: document.PersistenceStored,
			},
		},
	}
	stor := &fakeStorage{}
	svc := service.NewDocumentService(repo, stor).WithVirusScanner(&fakeVirusScanner{clean: true})
	svc.SetContractLookup(&fakeContractLookupSigned{
		// Backfill always re-queries Clicksign for a fresh URL.
		refreshedSignedURLByContract: map[string]string{"contract-stuck": srv.URL},
	})

	stats, err := svc.BackfillUnstoredSignedPDFs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.Scanned != 2 {
		t.Errorf("Scanned = %d, want 2", stats.Scanned)
	}
	if stats.Reattempted != 1 || stats.Succeeded != 1 {
		t.Errorf("expected 1 reattempt + 1 success, got %+v", stats)
	}
	if stats.Failed != 0 {
		t.Errorf("Failed = %d, want 0", stats.Failed)
	}
	stuck := repo.docs[0]
	if stuck.PersistenceStatus != document.PersistenceStored {
		t.Errorf("stuck doc should be PersistenceStored after backfill, got %q", stuck.PersistenceStatus)
	}
}

// TestBackfillUnstoredSignedPDFs_ReprocessesDocxRows cobre contratos
// pré-fix: o webhook envelope_finished promoveu Status para Assinado mas
// PersistSignedContractPDF (com o guarda antigo) pulou o download e
// deixou o DOCX no storage. Após o fix, o backfill deve reprocessar
// essas rows e substituir o DOCX pelo PDF assinado.
func TestBackfillUnstoredSignedPDFs_ReprocessesDocxRows(t *testing.T) {
	pdfBody := buildFakeSignedPDF("contract-docx-stuck")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(pdfBody)
	}))
	defer srv.Close()

	const docxKey = "tenant-1/contrato/abcd1234_rascunho-contract-docx-stuck-deadbeef.docx"
	const docxMime = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	repo := &fakeDocumentRepo{
		docs: []*document.Document{{
			ID:                "doc-docx-stuck",
			TenantID:          "tenant-1",
			ContractID:        "contract-docx-stuck",
			Category:          document.CategoryContrato,
			Status:            document.StatusAssinado,
			StorageKey:        docxKey,
			FileType:          docxMime,
			PersistenceStatus: document.PersistenceStored,
			Name:              "Contrato deal-99",
		}},
	}
	stor := &fakeStorage{}
	svc := service.NewDocumentService(repo, stor).WithVirusScanner(&fakeVirusScanner{clean: true})
	svc.SetContractLookup(&fakeContractLookupSigned{
		// Backfill always re-queries Clicksign for a fresh URL.
		refreshedSignedURLByContract: map[string]string{"contract-docx-stuck": srv.URL},
	})

	stats, err := svc.BackfillUnstoredSignedPDFs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.Reattempted != 1 || stats.Succeeded != 1 {
		t.Errorf("expected 1 reattempt + 1 success, got %+v", stats)
	}
	if stor.uploadCalls != 1 {
		t.Errorf("Upload calls = %d, want 1 (DOCX must be replaced by signed PDF)", stor.uploadCalls)
	}
	got := repo.docs[0]
	if got.FileType != "application/pdf" {
		t.Errorf("FileType after backfill = %q, want application/pdf", got.FileType)
	}
	if got.StorageKey == "" || got.StorageKey == docxKey {
		t.Errorf("StorageKey should be replaced with the new PDF key, still = %q", got.StorageKey)
	}
}

func TestBackfillUnstoredSignedPDFs_ErrorsWithoutContractLookup(t *testing.T) {
	repo := &fakeDocumentRepo{}
	svc := service.NewDocumentService(repo, &fakeStorage{})
	// Intentionally NOT wiring contractLookup — backfill should fail
	// loud rather than silently report 0 work done.

	_, err := svc.BackfillUnstoredSignedPDFs(context.Background(), "tenant-1")
	if err == nil {
		t.Fatal("expected error when contract lookup is not configured")
	}
}

func TestBackfillUnstoredSignedPDFs_RequiresTenantID(t *testing.T) {
	svc := service.NewDocumentService(&fakeDocumentRepo{}, &fakeStorage{})
	_, err := svc.BackfillUnstoredSignedPDFs(context.Background(), "")
	if err == nil {
		t.Fatal("expected error when tenant id is empty")
	}
}

// TestBackfillUnstoredSignedPDFs_RecoversViaRefreshWhenCacheEmpty exercises
// the Phase 3 reconcile gap fix: when GetSignedDocumentURL returns
// empty (contract never cached the URL because the original
// envelope_finished webhook came without it), the backfill must call
// RefreshSignedDocumentURLFromRemote — which hits toca-signature →
// Clicksign — to recover the URL and then persist normally. Without
// this branch the row stays stuck forever.
func TestBackfillUnstoredSignedPDFs_RecoversViaRefreshWhenCacheEmpty(t *testing.T) {
	pdfBody := buildFakeSignedPDF("contract-cache-miss")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(pdfBody)
	}))
	defer srv.Close()

	repo := &fakeDocumentRepo{
		docs: []*document.Document{{
			ID:                "doc-cache-miss",
			TenantID:          "tenant-1",
			ContractID:        "contract-cache-miss",
			Category:          document.CategoryContrato,
			Status:            document.StatusAssinado,
			PersistenceStatus: document.PersistencePending,
		}},
	}
	svc := service.NewDocumentService(repo, &fakeStorage{}).WithVirusScanner(&fakeVirusScanner{clean: true})
	lookup := &fakeContractLookupSigned{
		// Cache miss — original webhook didn't carry the URL.
		signedURLByContract: map[string]string{},
		// But Clicksign would now return it if we re-hit.
		refreshedSignedURLByContract: map[string]string{
			"contract-cache-miss": srv.URL,
		},
	}
	svc.SetContractLookup(lookup)

	stats, err := svc.BackfillUnstoredSignedPDFs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lookup.refreshCalls != 1 {
		t.Errorf("RefreshSignedDocumentURLFromRemote calls = %d, want 1 (cache miss should trigger refresh)", lookup.refreshCalls)
	}
	if stats.Succeeded != 1 {
		t.Errorf("expected 1 successful recovery, got %+v", stats)
	}
	if stats.SkippedNoURL != 0 {
		t.Errorf("must NOT skip when refresh returns a URL, got %+v", stats)
	}
	got := repo.docs[0]
	if got.PersistenceStatus != document.PersistenceStored {
		t.Errorf("PersistenceStatus = %q, want stored after refresh-recovered backfill", got.PersistenceStatus)
	}
}

// TestBackfillUnstoredSignedPDFs_IgnoresStaleCachedURL pins the stale-link
// fix: contract.SignedDocumentURL caches a ~5 min Clicksign S3 presigned
// link, so the backfill must NOT download the cached value (it returns 403
// once expired) — it always re-queries Clicksign for a fresh URL.
func TestBackfillUnstoredSignedPDFs_IgnoresStaleCachedURL(t *testing.T) {
	stale := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden) // expired presigned link
	}))
	defer stale.Close()
	pdfBody := buildFakeSignedPDF("contract-stale")
	fresh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(pdfBody)
	}))
	defer fresh.Close()

	repo := &fakeDocumentRepo{
		docs: []*document.Document{{
			ID:                "doc-stale",
			TenantID:          "tenant-1",
			ContractID:        "contract-stale",
			Category:          document.CategoryContrato,
			Status:            document.StatusAssinado,
			PersistenceStatus: document.PersistencePending,
		}},
	}
	svc := service.NewDocumentService(repo, &fakeStorage{}).WithVirusScanner(&fakeVirusScanner{clean: true})
	svc.SetContractLookup(&fakeContractLookupSigned{
		signedURLByContract:          map[string]string{"contract-stale": stale.URL}, // stale cache (would 403)
		refreshedSignedURLByContract: map[string]string{"contract-stale": fresh.URL}, // fresh, valid now
	})

	stats, err := svc.BackfillUnstoredSignedPDFs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.Succeeded != 1 || stats.Failed != 0 {
		t.Errorf("backfill must download the fresh URL, not the stale cache; got %+v", stats)
	}
}

func TestBackfillUnstoredSignedPDFs_SkipsWhenNoSignedURL(t *testing.T) {
	repo := &fakeDocumentRepo{
		docs: []*document.Document{
			{
				ID:         "doc-no-url",
				TenantID:   "tenant-1",
				ContractID: "contract-x",
				Status:     document.StatusAssinado,
				// Persistence pending and contract has no SignedDocumentURL
				PersistenceStatus: document.PersistencePending,
			},
		},
	}
	svc := service.NewDocumentService(repo, &fakeStorage{})
	svc.SetContractLookup(&fakeContractLookupSigned{
		// Empty map → GetSignedDocumentURL returns "".
		signedURLByContract: map[string]string{},
	})

	stats, err := svc.BackfillUnstoredSignedPDFs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.Reattempted != 0 || stats.SkippedNoURL != 1 {
		t.Errorf("expected skip when no signed URL, got %+v", stats)
	}
}

func TestListByDealIncludingContracts_Deduplicates(t *testing.T) {
	shared := &document.Document{
		ID:         "doc-shared",
		TenantID:   "tenant-1",
		ContractID: "contract-7",
		EntityID:   "deal-42", // also reachable via the deal index
		EntityType: document.EntityDeal,
		Name:       "Cross-linked",
	}
	repo := &fakeDocumentRepo{docs: []*document.Document{shared}}
	svc := service.NewDocumentService(repo, &fakeStorage{})
	svc.SetContractLookup(&fakeContractLookup{
		contractIDsByDeal: map[string][]string{"deal-42": {"contract-7"}},
	})

	got, err := svc.ListByDealIncludingContracts(context.Background(), "tenant-1", "deal-42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected dedup to collapse cross-linked doc to one entry, got %d", len(got))
	}
}
