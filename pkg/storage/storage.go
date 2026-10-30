package storage

import (
	"context"
	"io"
	"strings"
	"time"
)

const FinancialProofsBucket = "financial-proofs"

// SanitizeFilenameBase reduz a base de um nome de arquivo (sem extensão) a um
// charset seguro para object keys de storage (Supabase/S3). Mantém apenas
// [A-Za-z0-9-_.], troca espaços por '_' e descarta o restante — acentos,
// "(", ")", "[", "]", emoji etc. — que quebram a URL assinada de PUT
// (o caractere é codificado de formas diferentes na assinatura e no request,
// causando mismatch e falha no upload). Retorna "file" quando nada sobra.
//
// A função é lossy de propósito: o nome humano original deve ser preservado
// pelos callers em FileName/file_name. O prefixo UUID na storage key garante
// unicidade, então a base saneada serve apenas para legibilidade.
func SanitizeFilenameBase(base string) string {
	var b strings.Builder
	b.Grow(len(base))
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '_', r == '.':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('_')
		}
	}
	// Remove separadores das pontas para evitar keys "ocultas" (.foo) ou
	// terminadas em separador.
	cleaned := strings.Trim(b.String(), "._-")
	if cleaned == "" {
		return "file"
	}
	return cleaned
}

// Service defines the interface for file storage operations
type Service interface {
	// Upload saves a file to the storage provider and returns its URL
	Upload(ctx context.Context, bucket, filename string, contentType string, content io.Reader) (string, error)

	// Delete removes a file from the storage provider
	Delete(ctx context.Context, bucket, filename string) error

	// GetURL returns the public URL for a file
	GetURL(bucket, filename string) string

	// GetPresignedURL returns a short-lived signed URL for private files
	GetPresignedURL(ctx context.Context, bucket, filename string, expiry time.Duration) (string, error)

	// GetPresignedPutURL returns a short-lived signed URL for uploading a file via PUT
	GetPresignedPutURL(ctx context.Context, bucket, filename string, expiry time.Duration) (string, error)

	// IsPrivate returns whether a given bucket is private
	IsPrivate(bucket string) bool
}

// DownloadService is implemented by providers that can stream a private
// object through an authenticated application endpoint. It is intentionally
// separate from Service so existing upload-only integrations remain source
// compatible while private viewers opt in explicitly.
type DownloadService interface {
	Download(ctx context.Context, bucket, filename string) (io.ReadCloser, string, int64, error)
}
