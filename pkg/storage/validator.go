package storage

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gabriel-vasile/mimetype"
)

type BucketLimit struct {
	MaxSize     int64
	AllowedMIME []string
}

var limits = map[string]BucketLimit{
	"toca-avatars": {
		MaxSize: 5 * 1024 * 1024, // 5MB
		AllowedMIME: []string{
			"image/jpeg",
			"image/png",
			"image/webp",
			"image/svg+xml",
		},
	},
	"toca-logos": {
		MaxSize: 5 * 1024 * 1024, // 5MB
		AllowedMIME: []string{
			"image/jpeg",
			"image/png",
			"image/webp",
			"image/svg+xml",
		},
	},
	"toca-documents": {
		MaxSize: 25 * 1024 * 1024, // 25MB
		AllowedMIME: []string{
			"application/pdf",
			"image/jpeg",
			"image/png",
		},
	},
	"toca-message-attachments": {
		MaxSize: 10 * 1024 * 1024, // 10MB
		AllowedMIME: []string{
			"image/jpeg",
			"image/png",
			"image/webp",
			"image/gif",
			"application/pdf",
		},
	},
	"toca-property-images": {
		MaxSize: 10 * 1024 * 1024,
		AllowedMIME: []string{
			"image/jpeg",
			"image/png",
			"image/webp",
		},
	},
}

func Validate(bucket string, filename string, size int64, headBytes []byte) (string, error) {
	ext, _, err := ValidateWithMIME(bucket, filename, size, headBytes)
	return ext, err
}

// ValidateWithMIME validates a file and returns both its safe extension and
// the MIME detected from the content. Upload callers should use the returned
// MIME instead of trusting the multipart header or persisting "image/*".
func ValidateWithMIME(bucket string, filename string, size int64, headBytes []byte) (string, string, error) {
	limit, ok := limits[bucket]
	if !ok {
		return "", "", fmt.Errorf("unknown bucket: %s", bucket)
	}

	if size > limit.MaxSize {
		return "", "", fmt.Errorf("file size %d exceeds limit %d for bucket %s", size, limit.MaxSize, bucket)
	}

	mtype := mimetype.Detect(headBytes)
	detectedMime := mtype.String()

	// Check if MIME is allowed
	allowed := false
	for _, mime := range limit.AllowedMIME {
		if strings.HasPrefix(detectedMime, mime) {
			allowed = true
			break
		}
	}

	// Special case for SVG since mimetype package might detect it as text/xml
	if !allowed && strings.Contains(detectedMime, "xml") && strings.HasSuffix(strings.ToLower(filename), ".svg") {
		for _, mime := range limit.AllowedMIME {
			if mime == "image/svg+xml" {
				allowed = true
				detectedMime = "image/svg+xml"
				break
			}
		}
	}

	if !allowed {
		return "", "", fmt.Errorf("file type %s is not allowed in bucket %s", detectedMime, bucket)
	}

	// Safe extension based on mime type, fallback to original extension if needed
	ext := mtype.Extension()
	if detectedMime == "image/svg+xml" {
		ext = ".svg"
	}
	if ext == "" {
		ext = filepath.Ext(filename)
	}

	return ext, detectedMime, nil
}
