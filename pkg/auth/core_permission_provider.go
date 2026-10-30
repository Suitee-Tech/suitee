package auth

import (
	"context"
	"fmt"

	"github.com/oginoino/toca/pkg/clients"
	appCtx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/metrics"
)

type CorePermissionProvider struct {
	coreClient *clients.CoreClient
}

func NewCorePermissionProvider(coreClient *clients.CoreClient) *CorePermissionProvider {
	return &CorePermissionProvider{coreClient: coreClient}
}

// GetRolesMap resolves whether the current user's functional role has the
// required permission for the tenant. It is the single authorization
// decision point — the middleware no longer carries a hardcoded
// owner/admin bypass; instead, root profiles are short-circuited here so
// that decision is visible alongside every other permission lookup in
// the audit trail (event=auth.permission_bypass).
//
// Returns map[functional_role]bool with the user's role set to true when
// access is granted, or an empty map when denied. This shape matches
// what RequirePermission/RequireAnyPermission expect.
func (p *CorePermissionProvider) GetRolesMap(ctx context.Context, tenantID, requiredPermission string) (map[string]bool, error) {
	functionalRole, _ := appCtx.GetFunctionalRole(ctx)

	// Profile-level root access. Owner/admin (the "profile" axis) skip the
	// per-permission catalog lookup — they're treated as full-access
	// principals by design. Logged at every check so a compromised
	// owner/admin token leaves a paper trail.
	profile, _ := appCtx.GetUserPermission(ctx)
	if IsRootProfile(profile) {
		auditRootBypass(ctx, tenantID, profile, requiredPermission)
		return map[string]bool{functionalRole: true}, nil
	}

	if p.coreClient == nil {
		return nil, fmt.Errorf("core client not configured")
	}

	perms, err := p.coreClient.GetMyPermissions(ctx)
	if err != nil {
		return nil, err
	}

	allowed := false
	for _, id := range perms.Permissions {
		if id == requiredPermission {
			allowed = true
			break
		}
	}

	if !allowed {
		return map[string]bool{}, nil
	}

	return map[string]bool{functionalRole: true}, nil
}

// auditRootBypass records every owner/admin bypass via the shared
// structured logger. Replaces the per-call-site logPermissionBypass that
// lived in middleware.go before the bypass was centralized here.
//
// Fields kept deliberately small (no Gin context dependency) so the
// provider remains framework-agnostic and reusable from non-HTTP code
// paths.
func auditRootBypass(ctx context.Context, tenantID, profile, permission string) {
	userID, _ := appCtx.GetAccountID(ctx)
	logger.Info("auth bypass: root profile granted access without checking permission catalog", map[string]interface{}{
		"event":        "auth.permission_bypass",
		"tenant_id":    tenantID,
		"user_id":      userID,
		"user_profile": profile,
		"permission":   permission,
	})
	metrics.IncAuthAdminBypass(profile)
}
