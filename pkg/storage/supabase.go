package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SupabaseConfig contains configuration for the Supabase Storage Service
type SupabaseConfig struct {
	URL            string // The project URL (e.g. https://xyz.supabase.co)
	PublicURL      string // Browser-facing project URL; may differ from URL when the API runs in Docker
	ServiceRoleKey string // Must be the service_role key to bypass RLS and allow server-side operations
}

// SupabaseStorage implements the Service interface using the Supabase REST API
type SupabaseStorage struct {
	client        *http.Client
	baseURL       string
	publicBaseURL string
	serviceKey    string
}

// NewSupabaseStorage creates a new Supabase Storage service
func NewSupabaseStorage(cfg SupabaseConfig) (Service, error) {
	if cfg.ServiceRoleKey == "" {
		return nil, fmt.Errorf("supabase Service Role Key is required")
	}

	internalProjectURL, err := normalizeSupabaseProjectURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid supabase URL: %w", err)
	}

	publicProjectURL := strings.TrimSpace(cfg.PublicURL)
	if publicProjectURL == "" {
		publicProjectURL = browserFacingSupabaseURL(internalProjectURL)
	}
	publicProjectURL, err = normalizeSupabaseProjectURL(publicProjectURL)
	if err != nil {
		return nil, fmt.Errorf("invalid supabase public URL: %w", err)
	}

	return &SupabaseStorage{
		client:        &http.Client{Timeout: 30 * time.Second},
		baseURL:       internalProjectURL + "/storage/v1",
		publicBaseURL: publicProjectURL + "/storage/v1",
		serviceKey:    cfg.ServiceRoleKey,
	}, nil
}

func normalizeSupabaseProjectURL(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return "", fmt.Errorf("URL is required")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", fmt.Errorf("URL must use http or https and include a host")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("URL must not include a query string or fragment")
	}
	return parsed.String(), nil
}

// Docker services reach a local Supabase instance through
// host.docker.internal, while the browser reaches the same port through
// localhost. An explicit PublicURL takes precedence; this narrow fallback
// keeps existing local environments working when only SUPABASE_URL is set.
func browserFacingSupabaseURL(internalURL string) string {
	parsed, err := url.Parse(internalURL)
	if err != nil || !strings.EqualFold(parsed.Hostname(), "host.docker.internal") {
		return internalURL
	}
	port := parsed.Port()
	parsed.Host = "localhost"
	if port != "" {
		parsed.Host = net.JoinHostPort("localhost", port)
	}
	return parsed.String()
}

func (s *SupabaseStorage) signedURLForBase(raw, destinationBaseURL string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("supabase returned an empty signed URL")
	}

	signed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid signed URL returned by supabase: %w", err)
	}
	destinationBase, _ := url.Parse(destinationBaseURL)
	publicBase, _ := url.Parse(s.publicBaseURL)
	internalBase, _ := url.Parse(s.baseURL)

	signedPath := signed.Path
	if !strings.HasPrefix(signedPath, "/") {
		signedPath = "/" + signedPath
	}

	var suffix string
	var isStoragePath bool
	for _, prefix := range []string{publicBase.Path, internalBase.Path, "/storage/v1"} {
		if hasURLPathPrefix(signedPath, prefix) {
			suffix = strings.TrimPrefix(signedPath, strings.TrimRight(prefix, "/"))
			isStoragePath = true
			break
		}
	}
	if !isStoragePath && hasURLPathPrefix(signedPath, "/object") {
		suffix = signedPath
		isStoragePath = true
	}

	if isStoragePath {
		signed.Path = strings.TrimRight(destinationBase.Path, "/") + suffix
		signed.RawPath = ""
		signed.Scheme = destinationBase.Scheme
		signed.Host = destinationBase.Host
		signed.User = nil
		return signed.String(), nil
	}

	if signed.IsAbs() {
		return signed.String(), nil
	}

	return strings.TrimRight(destinationBaseURL, "/") + "/" + strings.TrimLeft(raw, "/"), nil
}

func hasURLPathPrefix(path, prefix string) bool {
	prefix = strings.TrimRight(prefix, "/")
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

// ResolveInternalURL rewrites a browser-facing signed Storage URL back to the
// endpoint reachable by backend services. It is intentionally an optional
// capability (not part of Service) so callers that fetch server-side can avoid
// split-horizon DNS and localhost hairpin failures without affecting S3/local.
func (s *SupabaseStorage) ResolveInternalURL(raw string) (string, error) {
	return s.signedURLForBase(raw, s.baseURL)
}

// buildReq creates a new HTTP request with Supabase Auth headers
func (s *SupabaseStorage) buildReq(ctx context.Context, method, endpoint string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, fmt.Sprintf("%s%s", s.baseURL, endpoint), body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+s.serviceKey)
	req.Header.Set("apikey", s.serviceKey)
	return req, nil
}

func escapeObjectName(name string) string {
	parts := strings.Split(name, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

func (s *SupabaseStorage) Upload(ctx context.Context, bucket, filename string, contentType string, content io.Reader) (string, error) {
	// For small files, buffering is ok. For large files, stream it.
	// Supabase expects a multipart/form-data or binary upload. Here we use standard binary upload.
	endpoint := fmt.Sprintf("/object/%s/%s", bucket, escapeObjectName(filename))

	req, err := s.buildReq(ctx, http.MethodPost, endpoint, content)
	if err != nil {
		return "", fmt.Errorf("failed to create upload request: %w", err)
	}

	if contentType == "" {
		contentType = "application/octet-stream"
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to upload to supabase: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("supabase upload error: status %d body %s", resp.StatusCode, string(bodyBytes))
	}

	return s.GetURL(bucket, filename), nil
}

func (s *SupabaseStorage) Delete(ctx context.Context, bucket, filename string) error {
	endpoint := fmt.Sprintf("/object/%s/%s", bucket, escapeObjectName(filename))

	req, err := s.buildReq(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete from supabase: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 && resp.StatusCode != 404 { // Ignore if not found
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("supabase delete error: status %d body %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

func (s *SupabaseStorage) GetURL(bucket, filename string) string {
	// For public buckets
	if !s.IsPrivate(bucket) {
		return fmt.Sprintf("%s/object/public/%s/%s", s.publicBaseURL, bucket, escapeObjectName(filename))
	}
	// For private buckets, the generic GetURL shouldn't be used (or it will return an inaccessible link).
	// But to conform with the interface, we return the authenticated route.
	return fmt.Sprintf("%s/object/authenticated/%s/%s", s.publicBaseURL, bucket, escapeObjectName(filename))
}

func (s *SupabaseStorage) GetPresignedURL(ctx context.Context, bucket, filename string, expiry time.Duration) (string, error) {
	endpoint := fmt.Sprintf("/object/sign/%s/%s", bucket, escapeObjectName(filename))

	payload := map[string]interface{}{
		"expiresIn": int(expiry.Seconds()),
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := s.buildReq(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get presigned url: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("supabase sign error: status %d body %s", resp.StatusCode, string(respBytes))
	}

	var result struct {
		SignedURL string `json:"signedURL"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	// Supabase normally returns a path relative to /storage/v1, but some
	// versions return an absolute URL. In both cases expose the public origin.
	return s.signedURLForBase(result.SignedURL, s.publicBaseURL)
}

func (s *SupabaseStorage) Download(ctx context.Context, bucket, filename string) (io.ReadCloser, string, int64, error) {
	endpoint := fmt.Sprintf("/object/%s/%s", bucket, escapeObjectName(filename))
	req, err := s.buildReq(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", 0, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, "", 0, fmt.Errorf("failed to download from supabase: %w", err)
	}
	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, "", 0, fmt.Errorf("supabase download error: status %d body %s", resp.StatusCode, string(bodyBytes))
	}

	return resp.Body, resp.Header.Get("Content-Type"), resp.ContentLength, nil
}

func (s *SupabaseStorage) GetPresignedPutURL(ctx context.Context, bucket, filename string, expiry time.Duration) (string, error) {
	// Note: Supabase supports presigned uploads (uploadUrl) using a slightly different endpoint
	endpoint := fmt.Sprintf("/object/upload/sign/%s/%s", bucket, escapeObjectName(filename))

	payload := map[string]interface{}{
		"expiresIn": int(expiry.Seconds()),
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := s.buildReq(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get presigned put url: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("supabase sign upload error: status %d body %s", resp.StatusCode, string(respBytes))
	}

	var result struct {
		URL string `json:"url"` // The presigned PUT URL
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return s.signedURLForBase(result.URL, s.publicBaseURL)
}

func (s *SupabaseStorage) IsPrivate(bucket string) bool {
	// According to the specification in docs/supabase/migration.md
	switch bucket {
	case "toca-message-attachments", "toca-documents", FinancialProofsBucket:
		return true
	default:
		// toca-avatars, toca-logos, toca-property-images
		return false
	}
}
