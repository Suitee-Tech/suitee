package storage

import (
	"fmt"
	"os"
	"strings"
)

// NewFromEnv initializes a storage Service based on environment variables.
func NewFromEnv() (Service, error) {
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_PROVIDER")))

	if provider == "supabase" {
		supabaseURL := os.Getenv("SUPABASE_URL")
		serviceKey := os.Getenv("SUPABASE_SERVICE_ROLE_KEY")
		if strings.TrimSpace(supabaseURL) == "" || strings.TrimSpace(serviceKey) == "" {
			return nil, fmt.Errorf("supabase storage requires SUPABASE_URL and SUPABASE_SERVICE_ROLE_KEY")
		}
		cfg := SupabaseConfig{
			URL:            supabaseURL,
			PublicURL:      os.Getenv("SUPABASE_PUBLIC_URL"),
			ServiceRoleKey: serviceKey,
		}
		return NewSupabaseStorage(cfg)
	}

	if provider == "s3" {
		accessKey := os.Getenv("S3_ACCESS_KEY")
		if accessKey == "" {
			accessKey = os.Getenv("MINIO_ACCESS_KEY")
		}
		if accessKey == "" {
			accessKey = os.Getenv("MINIO_ROOT_USER")
		}

		secretKey := os.Getenv("S3_SECRET_KEY")
		if secretKey == "" {
			secretKey = os.Getenv("MINIO_SECRET_KEY")
		}
		if secretKey == "" {
			secretKey = os.Getenv("MINIO_ROOT_PASSWORD")
		}

		cfg := S3Config{
			Endpoint:       os.Getenv("S3_ENDPOINT"),
			PublicEndpoint: os.Getenv("S3_PUBLIC_ENDPOINT"),
			AccessKey:      accessKey,
			SecretKey:      secretKey,
			Region:         os.Getenv("S3_REGION"),
			UseSSL:         os.Getenv("S3_USE_SSL") == "true",
		}
		if cfg.Region == "" {
			cfg.Region = "us-east-1"
		}
		return NewS3Storage(cfg)
	}

	if provider != "" && provider != "local" {
		return nil, fmt.Errorf("unsupported storage provider %q", provider)
	}

	// Explicit local storage, or the development fallback when no provider is set.
	publicUploadsURL := localUploadsURL()

	storageDir := os.Getenv("STORAGE_DIR")
	if storageDir == "" {
		storageDir = "/uploads"
	}

	return NewLocalStorage(storageDir, publicUploadsURL)
}

func localUploadsURL() string {
	for _, envKey := range []string{"PUBLIC_UPLOADS_URL", "PUBLIC_GATEWAY_URL", "PUBLIC_API_URL"} {
		value := strings.TrimRight(strings.TrimSpace(os.Getenv(envKey)), "/")
		if value == "" {
			continue
		}
		if strings.HasSuffix(value, "/uploads") {
			return value
		}
		// PUBLIC_API_URL is the host root in beta/prod. Keep this fallback
		// relative to the active environment instead of embedding production.
		value = strings.TrimSuffix(value, "/api/v1")
		return value + "/uploads"
	}

	// Safe local default for an explicitly local/fallback deployment.
	return "http://localhost:8080/uploads"
}
