package storage

import (
	"context"
	"fmt"
	"io"
	"log"
	"mime"
	"path/filepath"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3Config struct {
	Endpoint       string
	PublicEndpoint string
	AccessKey      string
	SecretKey      string
	Region         string
	UseSSL         bool
}

type S3Storage struct {
	client         *minio.Client
	cfg            S3Config
	publicBuckets  map[string]bool
	privateBuckets map[string]bool
}

func NewS3Storage(cfg S3Config) (*S3Storage, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create minio client: %w", err)
	}

	publicBuckets := map[string]bool{
		"toca-avatars":             true,
		"toca-logos":               true,
		"toca-message-attachments": true,
		"toca-property-images":     true,
	}

	privateBuckets := map[string]bool{
		"toca-documents":      true,
		FinancialProofsBucket: true,
	}

	ctx := context.Background()

	// Initialize buckets
	allBuckets := []string{"toca-avatars", "toca-logos", "toca-message-attachments", "toca-property-images", "toca-documents", FinancialProofsBucket}
	for _, bucket := range allBuckets {
		err = client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: cfg.Region})
		if err != nil {
			exists, errBucketExists := client.BucketExists(ctx, bucket)
			if errBucketExists == nil && exists {
				// Bucket already exists, fine
			} else {
				return nil, fmt.Errorf("failed to create bucket %s: %w", bucket, err)
			}
		}

		if publicBuckets[bucket] {
			policy := fmt.Sprintf(`{
				"Version": "2012-10-17",
				"Statement": [
					{
						"Effect": "Allow",
						"Principal": {"AWS": ["*"]},
						"Action": ["s3:GetObject"],
						"Resource": ["arn:aws:s3:::%s/*"]
					}
				]
			}`, bucket)
			if err := client.SetBucketPolicy(ctx, bucket, policy); err != nil {
				log.Printf("[storage-s3] WARN: failed to set public policy for %s (bucket ainda funciona, mas pode precisar de politica manual): %v", bucket, err)
			}
		}
	}

	return &S3Storage{
		client:         client,
		cfg:            cfg,
		publicBuckets:  publicBuckets,
		privateBuckets: privateBuckets,
	}, nil
}

func (s *S3Storage) Upload(ctx context.Context, bucket, filename string, contentType string, content io.Reader) (string, error) {
	// Read all content to get size, minio-go PutObject prefers known size or -1 for unknown.
	// Since it's streaming, we can use -1 but we need a Seekable stream or we must read it into buffer if it's small.
	// Using -1 for size if io.Reader is not io.Seeker
	// Wait, standard PutObject can stream if size is -1 and partSize is specified.
	// Better yet, just use minio.PutObject with size -1
	_, err := s.client.PutObject(ctx, bucket, filename, content, -1, minio.PutObjectOptions{
		ContentType: contentType,
		PartSize:    10 * 1024 * 1024,
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload to minio: %w", err)
	}

	if s.IsPrivate(bucket) {
		return "", nil // For private buckets, URL is not returned here directly
	}

	return s.GetURL(bucket, filename), nil
}

func (s *S3Storage) Delete(ctx context.Context, bucket, filename string) error {
	return s.client.RemoveObject(ctx, bucket, filename, minio.RemoveObjectOptions{})
}

func (s *S3Storage) GetURL(bucket, filename string) string {
	if s.IsPrivate(bucket) {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s", s.cfg.PublicEndpoint, bucket, filename)
}

func (s *S3Storage) GetPresignedURL(ctx context.Context, bucket, filename string, expiry time.Duration) (string, error) {
	reqParams := make(map[string][]string)
	url, err := s.client.PresignedGetObject(ctx, bucket, filename, expiry, reqParams)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}
	return url.String(), nil
}

func (s *S3Storage) Download(ctx context.Context, bucket, filename string) (io.ReadCloser, string, int64, error) {
	object, err := s.client.GetObject(ctx, bucket, filename, minio.GetObjectOptions{})
	if err != nil {
		return nil, "", 0, fmt.Errorf("failed to open object in minio: %w", err)
	}

	info, err := object.Stat()
	if err != nil {
		_ = object.Close()
		return nil, "", 0, fmt.Errorf("failed to stat object in minio: %w", err)
	}
	contentType := info.ContentType
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(filename))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return object, contentType, info.Size, nil
}

func (s *S3Storage) GetPresignedPutURL(ctx context.Context, bucket, filename string, expiry time.Duration) (string, error) {
	url, err := s.client.PresignedPutObject(ctx, bucket, filename, expiry)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned PUT URL: %w", err)
	}
	return url.String(), nil
}

func (s *S3Storage) IsPrivate(bucket string) bool {
	return s.privateBuckets[bucket]
}
