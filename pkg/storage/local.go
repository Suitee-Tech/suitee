package storage

import (
	"context"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LocalStorage implements Service using the local file system
type LocalStorage struct {
	baseDir string
	baseURL string
}

func NewLocalStorage(baseDir, baseURL string) (*LocalStorage, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, err
	}
	return &LocalStorage{
		baseDir: baseDir,
		baseURL: baseURL,
	}, nil
}

func (s *LocalStorage) Upload(ctx context.Context, bucket, filename string, contentType string, content io.Reader) (string, error) {
	dir := filepath.Join(s.baseDir, bucket)
	path := filepath.Join(dir, filename)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}

	file, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	if _, err := io.Copy(file, content); err != nil {
		return "", err
	}

	return s.GetURL(bucket, filename), nil
}

func (s *LocalStorage) Delete(ctx context.Context, bucket, filename string) error {
	path := filepath.Join(s.baseDir, bucket, filename)
	return os.Remove(path)
}

func (s *LocalStorage) GetURL(bucket, filename string) string {
	return fmt.Sprintf("%s/%s/%s", s.baseURL, bucket, filename)
}

func (s *LocalStorage) GetPresignedURL(ctx context.Context, bucket, filename string, expiry time.Duration) (string, error) {
	// Local storage does not support actual signed URLs, return standard URL
	return s.GetURL(bucket, filename), nil
}

func (s *LocalStorage) GetPresignedPutURL(ctx context.Context, bucket, filename string, expiry time.Duration) (string, error) {
	return "", fmt.Errorf("presigned PUT not supported for local storage")
}

func (s *LocalStorage) Download(_ context.Context, bucket, filename string) (io.ReadCloser, string, int64, error) {
	path := filepath.Join(s.baseDir, bucket, filename)
	rel, err := filepath.Rel(s.baseDir, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, "", 0, fmt.Errorf("invalid storage object path")
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, "", 0, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, "", 0, err
	}
	contentType := mime.TypeByExtension(filepath.Ext(filename))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return file, contentType, info.Size(), nil
}

func (s *LocalStorage) IsPrivate(bucket string) bool {
	// Local storage currently exposes everything under /uploads
	return false
}
