package credential

import (
	"context"
	"errors"
)

// Credentials para chamadas à Clicksign. Ainda não persistimos por tenant —
// hoje todos os tenants compartilham as credenciais lidas das envs. A interface
// foi mantida pronta para um TenantDBCredentialProvider no futuro.
type Credentials struct {
	BaseURL       string
	AccessToken   string
	WebhookSecret string
	Environment   string // sandbox | production
	// AuthScheme controla como o token é apresentado no header Authorization.
	// A doc oficial Clicksign v3 (docs/signature/signature.md) usa o token cru,
	// sem prefixo: `Authorization: $access_token`. Por isso o default é "raw".
	// Mantemos "bearer" como opção para tenants que precisem do prefixo.
	AuthScheme string
}

const (
	AuthSchemeBearer = "bearer"
	AuthSchemeRaw    = "raw"
)

// CredentialProvider abstrai a fonte de credenciais para a Clicksign.
// O parâmetro tenantID é aceito hoje mas pode ser ignorado por implementações
// que carregam de envs globais.
type CredentialProvider interface {
	Get(ctx context.Context, tenantID string) (*Credentials, error)
}

var ErrCredentialsNotFound = errors.New("clicksign credentials not configured for tenant")
