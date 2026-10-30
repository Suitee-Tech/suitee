package services

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-ops/internal/models/inspection"
)

type stubInspectionRepo struct {
	inspection.Repository
	store map[string]*inspection.Inspection
}

func newStubRepo(seed *inspection.Inspection) *stubInspectionRepo {
	r := &stubInspectionRepo{store: map[string]*inspection.Inspection{}}
	if seed != nil {
		r.store[seed.ID] = seed
	}
	return r
}

// GetByID espelha o repositório Postgres: só devolve o registro quando ele é do
// tenant pedido; tenant vazio ou estrangeiro resulta em "não encontrado".
func (r *stubInspectionRepo) GetByID(_ context.Context, tenantID, id string) (*inspection.Inspection, error) {
	rec, ok := r.store[id]
	if !ok || rec == nil || rec.TenantID != tenantID {
		return nil, nil
	}
	return rec, nil
}

func (r *stubInspectionRepo) Update(_ context.Context, insp *inspection.Inspection) error {
	r.store[insp.ID] = insp
	return nil
}

type stubStorage struct {
	uploads     []capturedUpload
	uploadError error
	failNthCall int
	callCount   int
}

type capturedUpload struct {
	Bucket      string
	Filename    string
	ContentType string
	Bytes       []byte
}

func (s *stubStorage) Upload(_ context.Context, bucket, filename, contentType string, content io.Reader) (string, error) {
	s.callCount++
	if s.failNthCall > 0 && s.callCount == s.failNthCall {
		return "", errors.New("stub storage forced failure")
	}
	if s.uploadError != nil {
		return "", s.uploadError
	}
	b, _ := io.ReadAll(content)
	s.uploads = append(s.uploads, capturedUpload{Bucket: bucket, Filename: filename, ContentType: contentType, Bytes: b})
	return "https://storage.example/" + bucket + "/" + filename, nil
}

func (s *stubStorage) Delete(_ context.Context, _, _ string) error { return nil }
func (s *stubStorage) GetURL(_, _ string) string                   { return "" }
func (s *stubStorage) GetPresignedURL(_ context.Context, _, _ string, _ time.Duration) (string, error) {
	return "", nil
}
func (s *stubStorage) GetPresignedPutURL(_ context.Context, _, _ string, _ time.Duration) (string, error) {
	return "", nil
}
func (s *stubStorage) IsPrivate(_ string) bool { return true }

func makeJPEGBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 100, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("jpeg.Encode: %v", err)
	}
	return buf.Bytes()
}

func makeInspection(findingID string) *inspection.Inspection {
	return &inspection.Inspection{
		ID:         "insp-1",
		TenantID:   "tenant-A",
		PropertyID: "prop-1",
		ContractID: "contract-1",
		Type:       inspection.TypeEntry,
		Status:     inspection.StatusEmAndamento,
		Findings: []inspection.Finding{
			{
				ID:          findingID,
				Room:        "Cozinha",
				Description: "Pia",
				Condition:   inspection.ConditionBom,
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func TestUploadFindingPhotos_HappyPath(t *testing.T) {
	insp := makeInspection("finding-1")
	repo := newStubRepo(insp)
	store := &stubStorage{}
	svc := NewInspectionService(repo, nil).WithStorage(store)

	urls, err := svc.UploadFindingPhotos(context.Background(), "tenant-A", "insp-1", "finding-1", "actor-1", "Operator", []FindingPhotoUpload{
		{FileName: "a.jpg", ContentType: "image/jpeg", RawBytes: makeJPEGBytes(t, 2000, 1500)},
		{FileName: "b.jpg", ContentType: "image/jpeg", RawBytes: makeJPEGBytes(t, 800, 600)},
	})
	if err != nil {
		t.Fatalf("UploadFindingPhotos: %v", err)
	}
	if len(urls.URLs) != 2 {
		t.Fatalf("expected 2 urls, got %d", len(urls.URLs))
	}
	if len(store.uploads) != 4 {
		t.Errorf("expected 4 uploads to storage (2 photos × {main, thumb}), got %d", len(store.uploads))
	}
	if store.uploads[0].ContentType != "image/jpeg" {
		t.Errorf("expected JPEG content-type after processing, got %q", store.uploads[0].ContentType)
	}
	if !strings.HasPrefix(store.uploads[0].Filename, "tenants/tenant-A/inspections/insp-1/finding-1/") {
		t.Errorf("unexpected storage path: %s", store.uploads[0].Filename)
	}
	if !strings.HasSuffix(store.uploads[0].Filename, ".jpg") {
		t.Errorf("expected .jpg extension, got %s", store.uploads[0].Filename)
	}

	stored, _ := repo.GetByID(context.Background(), "tenant-A", "insp-1")
	if got := len(stored.Findings[0].Photos); got != 2 {
		t.Errorf("expected 2 photos persisted in Finding, got %d", got)
	}
}

func TestUploadFindingPhotos_NoStorage_ReturnsError(t *testing.T) {
	svc := NewInspectionService(newStubRepo(nil), nil)
	_, err := svc.UploadFindingPhotos(context.Background(), "t", "i", "f", "a", "n", []FindingPhotoUpload{{RawBytes: []byte("x")}})
	if err == nil || err.Error() != "storage service not configured" {
		t.Errorf("expected storage-not-configured error, got %v", err)
	}
}

func TestUploadFindingPhotos_EmptyList(t *testing.T) {
	svc := NewInspectionService(newStubRepo(nil), nil).WithStorage(&stubStorage{})
	_, err := svc.UploadFindingPhotos(context.Background(), "t", "i", "f", "a", "n", nil)
	if err == nil {
		t.Error("expected error for empty uploads list")
	}
}

func TestUploadFindingPhotos_TooMany(t *testing.T) {
	svc := NewInspectionService(newStubRepo(nil), nil).WithStorage(&stubStorage{})
	uploads := make([]FindingPhotoUpload, 6)
	_, err := svc.UploadFindingPhotos(context.Background(), "t", "i", "f", "a", "n", uploads)
	if !errors.Is(err, ErrTooManyPhotos) {
		t.Errorf("expected ErrTooManyPhotos, got %v", err)
	}
}

func TestUploadFindingPhotos_InspectionNotFound(t *testing.T) {
	repo := newStubRepo(nil)
	svc := NewInspectionService(repo, nil).WithStorage(&stubStorage{})
	_, err := svc.UploadFindingPhotos(context.Background(), "t", "missing", "f", "a", "n", []FindingPhotoUpload{
		{FileName: "x.jpg", ContentType: "image/jpeg", RawBytes: makeJPEGBytes(t, 100, 100)},
	})
	if err == nil || err.Error() != "inspection not found" {
		t.Errorf("expected 'inspection not found', got %v", err)
	}
}

func TestUploadFindingPhotos_CrossTenant_HiddenAsNotFound(t *testing.T) {
	insp := makeInspection("finding-1")
	insp.TenantID = "tenant-A"
	repo := newStubRepo(insp)
	svc := NewInspectionService(repo, nil).WithStorage(&stubStorage{})

	_, err := svc.UploadFindingPhotos(context.Background(), "tenant-B", "insp-1", "finding-1", "a", "n", []FindingPhotoUpload{
		{FileName: "x.jpg", ContentType: "image/jpeg", RawBytes: makeJPEGBytes(t, 100, 100)},
	})
	if err == nil || err.Error() != "inspection not found" {
		t.Errorf("cross-tenant should be hidden as not-found, got %v", err)
	}
}

func TestUploadFindingPhotos_FindingNotFound(t *testing.T) {
	insp := makeInspection("finding-1")
	repo := newStubRepo(insp)
	svc := NewInspectionService(repo, nil).WithStorage(&stubStorage{})

	_, err := svc.UploadFindingPhotos(context.Background(), "tenant-A", "insp-1", "wrong-finding", "a", "n", []FindingPhotoUpload{
		{FileName: "x.jpg", ContentType: "image/jpeg", RawBytes: makeJPEGBytes(t, 100, 100)},
	})
	if !errors.Is(err, ErrFindingNotFound) {
		t.Errorf("expected ErrFindingNotFound, got %v", err)
	}
}

func TestUploadFindingPhotos_BackfillFindingID(t *testing.T) {
	insp := makeInspection("")
	insp.Findings[0].ID = ""
	repo := newStubRepo(insp)
	svc := NewInspectionService(repo, nil).WithStorage(&stubStorage{})

	backfilled := inspection.EnsureIDs(insp.Findings)
	expectedID := backfilled[0].ID
	if expectedID == "" {
		t.Fatal("EnsureIDs should have assigned an ID")
	}
	insp.Findings = backfilled
	_ = repo.Update(context.Background(), insp)

	_, err := svc.UploadFindingPhotos(context.Background(), "tenant-A", "insp-1", expectedID, "a", "n", []FindingPhotoUpload{
		{FileName: "x.jpg", ContentType: "image/jpeg", RawBytes: makeJPEGBytes(t, 200, 200)},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUploadFindingPhotos_RejectsBadContentTypes(t *testing.T) {
	insp := makeInspection("finding-1")
	repo := newStubRepo(insp)
	store := &stubStorage{}
	svc := NewInspectionService(repo, nil).WithStorage(store)

	_, err := svc.UploadFindingPhotos(context.Background(), "tenant-A", "insp-1", "finding-1", "a", "n", []FindingPhotoUpload{
		{FileName: "doc.pdf", ContentType: "application/pdf", RawBytes: []byte("%PDF-")},
	})
	if err == nil || err.Error() != "no photos were processed successfully" {
		t.Errorf("expected 'no photos were processed successfully', got %v", err)
	}
	if len(store.uploads) != 0 {
		t.Errorf("storage should not be called when all uploads have bad content-type, got %d uploads", len(store.uploads))
	}
}

func TestUploadFindingPhotos_PartialFailure_KeepsOthers(t *testing.T) {
	insp := makeInspection("finding-1")
	repo := newStubRepo(insp)
	store := &stubStorage{failNthCall: 3}
	svc := NewInspectionService(repo, nil).WithStorage(store)

	urls, err := svc.UploadFindingPhotos(context.Background(), "tenant-A", "insp-1", "finding-1", "a", "n", []FindingPhotoUpload{
		{FileName: "a.jpg", ContentType: "image/jpeg", RawBytes: makeJPEGBytes(t, 600, 400)},
		{FileName: "b.jpg", ContentType: "image/jpeg", RawBytes: makeJPEGBytes(t, 600, 400)},
		{FileName: "c.jpg", ContentType: "image/jpeg", RawBytes: makeJPEGBytes(t, 600, 400)},
	})
	if err != nil {
		t.Fatalf("partial-failure should not fail the call: %v", err)
	}
	if len(urls.URLs) != 2 {
		t.Errorf("expected 2 URLs (one main-upload failure), got %d", len(urls.URLs))
	}
	stored, _ := repo.GetByID(context.Background(), "tenant-A", "insp-1")
	if got := len(stored.Findings[0].Photos); got != 2 {
		t.Errorf("expected 2 photos persisted, got %d", got)
	}
	if got := len(stored.Findings[0].Thumbnails); got != 2 {
		t.Errorf("expected 2 thumbnails (parallel a Photos), got %d", got)
	}
}

func TestUploadFindingPhotos_ThumbnailFailure_PhotoStillPersists(t *testing.T) {
	insp := makeInspection("finding-1")
	repo := newStubRepo(insp)
	store := &stubStorage{failNthCall: 2}
	svc := NewInspectionService(repo, nil).WithStorage(store)

	urls, err := svc.UploadFindingPhotos(context.Background(), "tenant-A", "insp-1", "finding-1", "a", "n", []FindingPhotoUpload{
		{FileName: "a.jpg", ContentType: "image/jpeg", RawBytes: makeJPEGBytes(t, 600, 400)},
	})
	if err != nil {
		t.Fatalf("thumbnail failure should not fail the whole call: %v", err)
	}
	if len(urls.URLs) != 1 {
		t.Errorf("expected 1 photo URL even with thumb failure, got %d", len(urls.URLs))
	}
	stored, _ := repo.GetByID(context.Background(), "tenant-A", "insp-1")
	if len(stored.Findings[0].Photos) != 1 {
		t.Fatalf("expected 1 photo persisted, got %d", len(stored.Findings[0].Photos))
	}
	if len(stored.Findings[0].Thumbnails) != 1 {
		t.Fatalf("expected 1 thumbnail slot (parallel), got %d", len(stored.Findings[0].Thumbnails))
	}
	if stored.Findings[0].Thumbnails[0] != "" {
		t.Errorf("expected empty thumb URL on failure (fallback indicator), got %q", stored.Findings[0].Thumbnails[0])
	}
}

func TestUploadFindingPhotos_ThumbnailsPersistAlongsidePhotos(t *testing.T) {
	insp := makeInspection("finding-1")
	repo := newStubRepo(insp)
	store := &stubStorage{}
	svc := NewInspectionService(repo, nil).WithStorage(store)

	_, err := svc.UploadFindingPhotos(context.Background(), "tenant-A", "insp-1", "finding-1", "a", "n", []FindingPhotoUpload{
		{FileName: "a.jpg", ContentType: "image/jpeg", RawBytes: makeJPEGBytes(t, 800, 600)},
		{FileName: "b.jpg", ContentType: "image/jpeg", RawBytes: makeJPEGBytes(t, 800, 600)},
	})
	if err != nil {
		t.Fatalf("UploadFindingPhotos: %v", err)
	}
	stored, _ := repo.GetByID(context.Background(), "tenant-A", "insp-1")
	if got := len(stored.Findings[0].Photos); got != 2 {
		t.Fatalf("expected 2 photos, got %d", got)
	}
	if got := len(stored.Findings[0].Thumbnails); got != 2 {
		t.Fatalf("expected 2 thumbnails parallel a Photos, got %d", got)
	}
	for i, thumb := range stored.Findings[0].Thumbnails {
		if thumb == "" {
			t.Errorf("Thumbnails[%d] should be populated (no failure forced), got empty", i)
		}
		if !strings.Contains(thumb, "_thumb.jpg") {
			t.Errorf("Thumbnails[%d] should follow _thumb.jpg naming, got %q", i, thumb)
		}
	}
	if len(store.uploads) != 4 {
		t.Errorf("expected 4 storage uploads (2 photos + 2 thumbs), got %d", len(store.uploads))
	}
}

func TestUploadFindingPhotos_AllFailButOne_StillSucceeds(t *testing.T) {
	insp := makeInspection("finding-1")
	repo := newStubRepo(insp)
	svc := NewInspectionService(repo, nil).WithStorage(&stubStorage{})

	urls, err := svc.UploadFindingPhotos(context.Background(), "tenant-A", "insp-1", "finding-1", "a", "n", []FindingPhotoUpload{
		{FileName: "bad.xyz", ContentType: "application/garbage", RawBytes: []byte("bad")},
		{FileName: "good.jpg", ContentType: "image/jpeg", RawBytes: makeJPEGBytes(t, 500, 500)},
	})
	if err != nil {
		t.Fatalf("expected success when at least one succeeds, got %v", err)
	}
	if len(urls.URLs) != 1 {
		t.Errorf("expected 1 URL, got %d", len(urls.URLs))
	}
}

func TestSanitize(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"abc-123", "abc-123"},
		{"../etc/passwd", "etcpasswd"},
		{"tenant_id_42", "tenant_id_42"},
		{"", "unknown"},
		{"!@#$%^", "unknown"},
		{"valid-uuid-4242", "valid-uuid-4242"},
	}
	for _, tc := range cases {
		if got := sanitize(tc.in); got != tc.want {
			t.Errorf("sanitize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
