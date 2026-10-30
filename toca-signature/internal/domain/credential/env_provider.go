package credential

import (
	"context"
	"os"
	"strings"
)

// EnvCredentialProvider lê as credenciais Clicksign de variáveis de ambiente
// globais (CLICKSIGN_ACCESS_TOKEN, CLICKSIGN_WEBHOOK_SECRET, etc.). Ignora o
// tenantID — é o trade-off conhecido do MVP. Para suportar credenciais por
// tenant no futuro, criar um TenantDBCredentialProvider que leia da tabela
// signature.clicksign_integrations(tenant_id, access_token_encrypted, ...) e
// trocar a injeção em main.go sem mexer no resto do código.
type EnvCredentialProvider struct{}

func NewEnvCredentialProvider() *EnvCredentialProvider {
	return &EnvCredentialProvider{}
}

func (p *EnvCredentialProvider) Get(_ context.Context, _ string) (*Credentials, error) {
	token := strings.TrimSpace(os.Getenv("CLICKSIGN_ACCESS_TOKEN"))
	if token == "" {
		return nil, ErrCredentialsNotFound
	}
	base := strings.TrimSpace(os.Getenv("CLICKSIGN_BASE_URL"))
	if base == "" {
		base = "https://sandbox.clicksign.com"
	}
	env := strings.TrimSpace(os.Getenv("CLICKSIGN_ENVIRONMENT"))
	if env == "" {
		env = "sandbox"
	}
	scheme := strings.ToLower(strings.TrimSpace(os.Getenv("CLICKSIGN_AUTH_SCHEME")))
	if scheme != AuthSchemeBearer {
		scheme = AuthSchemeRaw
	}
	return &Credentials{
		BaseURL:       strings.TrimRight(base, "/"),
		AccessToken:   token,
		WebhookSecret: strings.TrimSpace(os.Getenv("CLICKSIGN_WEBHOOK_SECRET")),
		Environment:   env,
		AuthScheme:    scheme,
	}, nil
}
