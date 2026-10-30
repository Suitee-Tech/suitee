package auth

import "github.com/golang-jwt/jwt/v5"

type Claims struct {
	jwt.RegisteredClaims
	AccountID        string   `json:"account_id"`
	Email            string   `json:"email"`
	Name             string   `json:"name,omitempty"`
	AvatarURL        string   `json:"avatar_url,omitempty"`
	Role             string   `json:"role"`
	Permission       string   `json:"permission"`
	Type             string   `json:"type"` // individual or company
	TenantID         string   `json:"tenant_id,omitempty"`
	PartnerTenantIDs []string `json:"partner_tenant_ids,omitempty"`
	TokenType        string   `json:"token_type,omitempty"`
	SessionID        string   `json:"session_id,omitempty"`
	FamilyID         string   `json:"family_id,omitempty"`
}
