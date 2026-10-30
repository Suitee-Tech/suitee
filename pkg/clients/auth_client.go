package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type AuthClient struct {
	BaseClient *BaseClient

	approvalCacheMu      sync.RWMutex
	approvalCache        map[string]approvalPolicyCacheEntry
	approvalCacheTTL     time.Duration
	approvalStaleTTL     time.Duration
	approvalFetchTimeout time.Duration
	now                  func() time.Time
}

func NewAuthClient() *AuthClient {
	return &AuthClient{
		BaseClient:           NewBaseClient(os.Getenv("AUTH_SERVICE_URL")),
		approvalCache:        make(map[string]approvalPolicyCacheEntry),
		approvalCacheTTL:     durationSecondsFromEnv("APPROVAL_POLICY_CACHE_TTL_SECONDS", 5*time.Minute),
		approvalStaleTTL:     durationSecondsFromEnv("APPROVAL_POLICY_STALE_TTL_SECONDS", time.Hour),
		approvalFetchTimeout: durationMillisecondsFromEnv("APPROVAL_POLICY_FETCH_TIMEOUT_MS", 800*time.Millisecond),
		now:                  time.Now,
	}
}

const (
	ApprovalPolicyResolutionRemote     = "remote"
	ApprovalPolicyResolutionCache      = "cache"
	ApprovalPolicyResolutionStaleCache = "stale_cache"
)

type ApprovalPolicy struct {
	SchemaVersion      int       `json:"schema_version"`
	Revision           int64     `json:"revision"`
	Enabled            bool      `json:"enabled"`
	ThresholdBRL       float64   `json:"threshold_brl"`
	TargetStatuses     []string  `json:"target_statuses"`
	RequiredPermission string    `json:"required_permission"`
	UpdatedAt          time.Time `json:"updated_at"`
	UpdatedBy          string    `json:"updated_by,omitempty"`
	Source             string    `json:"source"`
	ResolutionSource   string    `json:"-"`
}

type approvalPolicyCacheEntry struct {
	policy     ApprovalPolicy
	expiresAt  time.Time
	staleUntil time.Time
}

func (p ApprovalPolicy) Validate() error {
	if p.SchemaVersion != 1 {
		return fmt.Errorf("unsupported approval policy schema_version %d", p.SchemaVersion)
	}
	if p.Revision < 0 {
		return fmt.Errorf("approval policy revision cannot be negative")
	}
	if math.IsNaN(p.ThresholdBRL) || math.IsInf(p.ThresholdBRL, 0) || p.ThresholdBRL < 0 || p.ThresholdBRL > 1_000_000_000_000 {
		return fmt.Errorf("invalid approval policy threshold_brl %.2f", p.ThresholdBRL)
	}
	if p.Enabled && p.ThresholdBRL <= 0 {
		return fmt.Errorf("enabled approval policy requires positive threshold_brl")
	}
	if p.RequiredPermission != "deals:approve" {
		return fmt.Errorf("unsupported approval policy permission %q", p.RequiredPermission)
	}
	if !slices.Equal(p.TargetStatuses, []string{"ANALYSIS", "CONTRACT"}) {
		return fmt.Errorf("unsupported approval policy target_statuses %v", p.TargetStatuses)
	}
	return nil
}

// GetApprovalPolicy resolves the tenant policy through toca-auth. A fresh
// cache hit avoids the network; a bounded stale value is returned when auth
// is temporarily unavailable. Once staleUntil passes, the error is surfaced
// so toca-deal can apply its documented environment/default fallback.
func (c *AuthClient) GetApprovalPolicy(ctx context.Context, tenantID string) (ApprovalPolicy, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return ApprovalPolicy{}, fmt.Errorf("tenant_id is required for approval policy")
	}

	now := c.clock()()
	if cached, ok := c.cachedApprovalPolicy(tenantID, now, false); ok {
		cached.ResolutionSource = ApprovalPolicyResolutionCache
		return cached, nil
	}

	requestCtx, cancel := context.WithTimeout(ctx, c.fetchTimeout())
	defer cancel()
	path := fmt.Sprintf("/api/v1/auth/internal/tenants/%s/approval-policy", url.PathEscape(tenantID))
	resp, err := c.BaseClient.Do(requestCtx, http.MethodGet, path, nil)
	if err != nil {
		return c.staleApprovalPolicyOrError(tenantID, now, fmt.Errorf("load approval policy: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.staleApprovalPolicyOrError(tenantID, now, fmt.Errorf("auth service returned status %d for approval policy", resp.StatusCode))
	}

	var response struct {
		Data ApprovalPolicy `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return c.staleApprovalPolicyOrError(tenantID, now, fmt.Errorf("decode approval policy: %w", err))
	}
	if err := response.Data.Validate(); err != nil {
		return c.staleApprovalPolicyOrError(tenantID, now, fmt.Errorf("validate approval policy: %w", err))
	}

	policy := cloneApprovalPolicy(response.Data)
	policy.ResolutionSource = ApprovalPolicyResolutionRemote
	c.storeApprovalPolicy(tenantID, policy, now)
	return policy, nil
}

func (c *AuthClient) cachedApprovalPolicy(tenantID string, now time.Time, allowStale bool) (ApprovalPolicy, bool) {
	c.approvalCacheMu.RLock()
	entry, ok := c.approvalCache[tenantID]
	c.approvalCacheMu.RUnlock()
	if !ok {
		return ApprovalPolicy{}, false
	}
	deadline := entry.expiresAt
	if allowStale {
		deadline = entry.staleUntil
	}
	if !now.Before(deadline) {
		return ApprovalPolicy{}, false
	}
	return cloneApprovalPolicy(entry.policy), true
}

func (c *AuthClient) staleApprovalPolicyOrError(tenantID string, now time.Time, cause error) (ApprovalPolicy, error) {
	if cached, ok := c.cachedApprovalPolicy(tenantID, now, true); ok {
		cached.ResolutionSource = ApprovalPolicyResolutionStaleCache
		return cached, nil
	}
	return ApprovalPolicy{}, cause
}

func (c *AuthClient) storeApprovalPolicy(tenantID string, policy ApprovalPolicy, now time.Time) {
	ttl := c.cacheTTL()
	c.approvalCacheMu.Lock()
	if c.approvalCache == nil {
		c.approvalCache = make(map[string]approvalPolicyCacheEntry)
	}
	c.approvalCache[tenantID] = approvalPolicyCacheEntry{
		policy:     cloneApprovalPolicy(policy),
		expiresAt:  now.Add(ttl),
		staleUntil: now.Add(ttl + c.staleTTL()),
	}
	c.approvalCacheMu.Unlock()
}

func cloneApprovalPolicy(policy ApprovalPolicy) ApprovalPolicy {
	policy.TargetStatuses = slices.Clone(policy.TargetStatuses)
	return policy
}

func (c *AuthClient) clock() func() time.Time {
	if c.now != nil {
		return c.now
	}
	return time.Now
}

func (c *AuthClient) cacheTTL() time.Duration {
	if c.approvalCacheTTL > 0 {
		return c.approvalCacheTTL
	}
	return 5 * time.Minute
}

func (c *AuthClient) staleTTL() time.Duration {
	if c.approvalStaleTTL > 0 {
		return c.approvalStaleTTL
	}
	return time.Hour
}

func (c *AuthClient) fetchTimeout() time.Duration {
	if c.approvalFetchTimeout > 0 {
		return c.approvalFetchTimeout
	}
	return 800 * time.Millisecond
}

func durationSecondsFromEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	seconds, err := strconv.Atoi(raw)
	if raw == "" || err != nil || seconds <= 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

func durationMillisecondsFromEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	millis, err := strconv.Atoi(raw)
	if raw == "" || err != nil || millis <= 0 {
		return fallback
	}
	return time.Duration(millis) * time.Millisecond
}

type CompanyMemberDetail struct {
	AccountID      string `json:"account_id"`
	Email          string `json:"email"`
	Name           string `json:"name"`
	AvatarURL      string `json:"avatar_url"`
	Profile        string `json:"profile"`
	FunctionalRole string `json:"functional_role"`
}

func (c *AuthClient) ListCompanyMembersInternal(ctx context.Context, companyID string, functionalRole string) ([]CompanyMemberDetail, error) {
	path := fmt.Sprintf("/api/v1/auth/internal/companies/%s/members", companyID)
	if functionalRole != "" {
		path += fmt.Sprintf("?functional_role=%s", functionalRole)
	}

	resp, err := c.BaseClient.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("auth service returned status: %d", resp.StatusCode)
	}

	var response struct {
		Data []CompanyMemberDetail `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return response.Data, nil
}
