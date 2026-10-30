package repositories

import (
	"context"
	"encoding/json"
	"strings"

	appCtx "github.com/oginoino/toca/pkg/context"
)

func jwtClaimsFromContext(ctx context.Context) (string, bool) {
	tenantID, _ := appCtx.GetTenantID(ctx)
	accountID, _ := appCtx.GetAccountID(ctx)

	claims := map[string]any{}
	if strings.TrimSpace(accountID) != "" {
		claims["sub"] = accountID
	}
	if strings.TrimSpace(tenantID) != "" {
		claims["app_metadata"] = map[string]any{
			"tenant_id": tenantID,
		}
	}
	if len(claims) == 0 {
		return "", false
	}
	b, err := json.Marshal(claims)
	if err != nil || len(b) == 0 {
		return "", false
	}
	return string(b), true
}
