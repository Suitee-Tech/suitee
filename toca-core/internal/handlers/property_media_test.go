package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/response"
)

type storageMock struct {
	uploadedBucket      string
	uploadedFilename    string
	uploadedContentType string
	deletedBucket       string
	deletedFilename     string
}

func (m *storageMock) Upload(ctx context.Context, bucket, filename string, contentType string, content io.Reader) (string, error) {
	m.uploadedBucket = bucket
	m.uploadedFilename = filename
	m.uploadedContentType = contentType
	_, _ = io.ReadAll(content)
	return m.GetURL(bucket, filename), nil
}

func (m *storageMock) Delete(ctx context.Context, bucket, filename string) error {
	m.deletedBucket = bucket
	m.deletedFilename = filename
	return nil
}

func (m *storageMock) GetURL(bucket, filename string) string {
	return "https://example.com/" + bucket + "/" + filename
}

func (m *storageMock) GetPresignedURL(ctx context.Context, bucket, filename string, expiry time.Duration) (string, error) {
	return "", nil
}

func (m *storageMock) GetPresignedPutURL(ctx context.Context, bucket, filename string, expiry time.Duration) (string, error) {
	return "", nil
}

func (m *storageMock) IsPrivate(bucket string) bool { return false }

func TestPropertyMedia_UploadImage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	st := &storageMock{}
	h := NewPropertyHandler(nil, st, nil)

	r := gin.New()
	r.POST("/properties/:id/media/images", h.UploadImage)

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	part, err := w.CreateFormFile("file", "photo.jpg")
	if err != nil {
		t.Fatal(err)
	}

	jpeg := append([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01}, bytes.Repeat([]byte{0x00}, 600)...)
	if _, err := part.Write(jpeg); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/properties/prop123/media/images", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}
	if st.uploadedBucket != "toca-property-images" {
		t.Fatalf("expected bucket toca-property-images, got %s", st.uploadedBucket)
	}
	if st.uploadedFilename == "" {
		t.Fatalf("expected filename to be set")
	}
	if !strings.HasPrefix(st.uploadedFilename, "prop123/") {
		t.Fatalf("expected filename prefix prop123/, got %s", st.uploadedFilename)
	}
	if st.uploadedContentType != "image/jpeg" {
		t.Fatalf("expected detected content type image/jpeg, got %s", st.uploadedContentType)
	}

	var resp response.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Success {
		t.Fatalf("expected success true")
	}
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected data map, got %T", resp.Data)
	}
	if data["url"] == "" || data["filename"] == "" {
		t.Fatalf("expected url and filename in response data")
	}
}

func TestPropertyMedia_DeleteImage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	st := &storageMock{}
	h := NewPropertyHandler(nil, st, nil)

	r := gin.New()
	r.DELETE("/properties/:id/media/images", h.DeleteImage)

	req := httptest.NewRequest(http.MethodDelete, "/properties/prop123/media/images?filename=prop123/x.jpg", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d: %s", http.StatusNoContent, rec.Code, rec.Body.String())
	}
	if st.deletedBucket != "toca-property-images" {
		t.Fatalf("expected bucket toca-property-images, got %s", st.deletedBucket)
	}
	if st.deletedFilename != "prop123/x.jpg" {
		t.Fatalf("expected filename prop123/x.jpg, got %s", st.deletedFilename)
	}
}

func TestPropertyMedia_DeleteImage_InvalidPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)

	st := &storageMock{}
	h := NewPropertyHandler(nil, st, nil)

	r := gin.New()
	r.DELETE("/properties/:id/media/images", h.DeleteImage)

	req := httptest.NewRequest(http.MethodDelete, "/properties/prop123/media/images?filename=other/x.jpg", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d: %s", http.StatusForbidden, rec.Code, rec.Body.String())
	}
}
