package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/oginoino/toca/toca-portal/internal/domain/integration"
	"github.com/oginoino/toca/toca-portal/internal/services/mapper"
)

type FeedService struct {
	cacheRepo      integration.FeedCacheRepository
	coreServiceURL string
	internalKey    string
	httpClient     *http.Client
}

func NewFeedService(cacheRepo integration.FeedCacheRepository, coreServiceURL, internalKey string) *FeedService {
	return &FeedService{
		cacheRepo:      cacheRepo,
		coreServiceURL: coreServiceURL,
		internalKey:    internalKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetCachedFeed returns the cached XML feed for an integration
func (s *FeedService) GetCachedFeed(ctx context.Context, integ *integration.PortalIntegration) (string, error) {
	cache, err := s.cacheRepo.Get(ctx, integ.ID)
	if err != nil {
		return "", err
	}
	if cache == nil {
		return "", nil
	}
	return string(cache.Content), nil
}

// GenerateAndCacheFeed fetches properties from toca-core and generates the VRSync XML feed
func (s *FeedService) GenerateAndCacheFeed(ctx context.Context, integ *integration.PortalIntegration) (int, int, int64, error) {
	// Fetch properties from toca-core
	properties, err := s.fetchProperties(ctx, integ)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to fetch properties: %w", err)
	}

	log.Printf("[FeedService] Fetched %d properties for tenant %s, portal %s", len(properties), integ.TenantID, integ.Portal)

	// Generate VRSync XML
	xmlContent, successCount, failCount, err := mapper.GenerateVRSyncFeed(
		properties,
		integ.ContactName,
		integ.ContactEmail,
		integ.ContactPhone,
		integ.PublicationType,
	)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to generate XML: %w", err)
	}

	sizeBytes := int64(len(xmlContent))

	// Cache the generated XML
	cache := &integration.FeedCache{
		IntegrationID: integ.ID,
		Content:       []byte(xmlContent),
		UpdatedAt:     time.Now(),
	}
	_ = s.cacheRepo.Set(ctx, cache)

	return successCount, failCount, sizeBytes, nil
}

// fetchProperties calls toca-core API to get published properties for a tenant
func (s *FeedService) fetchProperties(ctx context.Context, integ *integration.PortalIntegration) ([]mapper.PropertyData, error) {
	// Build URL with filters
	url := fmt.Sprintf("%s/api/v1/properties?limit=1000&offset=0", s.coreServiceURL)

	// Apply property filter
	if len(integ.PropertyFilter.Statuses) > 0 {
		url += "&status=" + integ.PropertyFilter.Statuses[0]
	} else {
		url += "&status=published"
	}

	if len(integ.PropertyFilter.PropertyTypes) > 0 {
		url += "&property_type=" + integ.PropertyFilter.PropertyTypes[0]
	}

	if len(integ.PropertyFilter.ListingTypes) > 0 {
		url += "&listing_type=" + integ.PropertyFilter.ListingTypes[0]
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-Internal-Key", s.internalKey)
	req.Header.Set("X-Tenant-Id", integ.TenantID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call toca-core: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("toca-core returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []mapper.PropertyData `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode properties response: %w", err)
	}

	return result.Data, nil
}
