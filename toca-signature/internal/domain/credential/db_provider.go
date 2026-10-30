package credential

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// CredentialRepository é a fatia mínima que o DBCredentialProvider precisa
// do repositório (ver toca-signature/internal/repository). Definida aqui
// para o domínio não depender da camada de persistência.
type CredentialRepository interface {
	GetByTenant(ctx context.Context, tenantID string) (*CredentialRow, error)
	Upsert(ctx context.Context, tenantID string, row *CredentialRow) (int, error)
	AppendRotation(ctx context.Context, tenantID string, revision int, rotatedBy, note string) error
	ListRotations(ctx context.Context, tenantID string, limit int) ([]*CredentialRotation, error)
}

// CredentialRow espelha a linha persistida (segredos criptografados).
type CredentialRow struct {
	TenantID               string
	BaseURL                string
	AccessTokenEncrypted   string
	WebhookSecretEncrypted string
	Environment            string
	AuthScheme             string
	Revision               int
	UpdatedAt              string
}

// CredentialRotation é uma entrada da trilha de rotação.
type CredentialRotation struct {
	TenantID  string
	Revision  int
	RotatedBy string
	Note      string
	CreatedAt string
}

// ErrCredentialTenantNotFound marca tenant SEM credencial persistida —
// o provedor cai no fallback global de ambiente (compatibilidade MVP).
var ErrCredentialTenantNotFound = errors.New("clicksign credentials not found for tenant")

// DBCredentialProvider resolve credenciais POR TENANT (P1-14): a linha do
// tenant é descriptografada com a chave de aplicação; tenant sem linha cai
// no provedor global de ambiente. A mesma instância também grava/rotaciona
// credenciais via endpoint administrativo.
type DBCredentialProvider struct {
	repo     CredentialRepository
	encKey   []byte
	fallback CredentialProvider
}

// NewDBCredentialProvider monta o provedor por tenant. encKey é o material
// AES-256 (credential.KeyFromHex); fallback é o provedor global (env).
func NewDBCredentialProvider(repo CredentialRepository, encKey []byte, fallback CredentialProvider) *DBCredentialProvider {
	return &DBCredentialProvider{repo: repo, encKey: encKey, fallback: fallback}
}

func (p *DBCredentialProvider) Get(ctx context.Context, tenantID string) (*Credentials, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID != "" && p.repo != nil {
		row, err := p.repo.GetByTenant(ctx, tenantID)
		if err == nil {
			return p.decryptRow(row)
		}
		if !errors.Is(err, ErrCredentialTenantNotFound) {
			return nil, fmt.Errorf("load clicksign credentials for tenant: %w", err)
		}
	}
	if p.fallback == nil {
		return nil, ErrCredentialsNotFound
	}
	return p.fallback.Get(ctx, tenantID)
}

func (p *DBCredentialProvider) decryptRow(row *CredentialRow) (*Credentials, error) {
	if len(p.encKey) != 32 {
		return nil, fmt.Errorf("tenant %s has persisted clicksign credentials but the decryption key is not configured: %w", row.TenantID, ErrCredentialCryptoKeyMissing)
	}
	token, err := Decrypt(p.encKey, row.AccessTokenEncrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypt clicksign access token for tenant %s: %w", row.TenantID, err)
	}
	secret := ""
	if row.WebhookSecretEncrypted != "" {
		secret, err = Decrypt(p.encKey, row.WebhookSecretEncrypted)
		if err != nil {
			return nil, fmt.Errorf("decrypt clicksign webhook secret for tenant %s: %w", row.TenantID, err)
		}
	}
	return &Credentials{
		BaseURL:       strings.TrimRight(strings.TrimSpace(row.BaseURL), "/"),
		AccessToken:   token,
		WebhookSecret: secret,
		Environment:   strings.TrimSpace(row.Environment),
		AuthScheme:    authSchemeOrRaw(row.AuthScheme),
	}, nil
}

func authSchemeOrRaw(scheme string) string {
	scheme = strings.ToLower(strings.TrimSpace(scheme))
	if scheme != AuthSchemeBearer {
		return AuthSchemeRaw
	}
	return AuthSchemeBearer
}

// Store grava (ou substitui) a credencial do tenant, criptografando os
// segredos com a chave de aplicação, e registra a rotação na trilha.
// Devolve a revisão efetiva.
func (p *DBCredentialProvider) Store(ctx context.Context, tenantID, baseURL, accessToken, webhookSecret, environment, authScheme, rotatedBy, note string) (int, error) {
	if p.repo == nil {
		return 0, errors.New("credential repository not configured")
	}
	if len(p.encKey) != 32 {
		return 0, ErrCredentialCryptoKeyMissing
	}
	if strings.TrimSpace(accessToken) == "" {
		return 0, errors.New("access token is required")
	}
	encToken, err := Encrypt(p.encKey, accessToken)
	if err != nil {
		return 0, err
	}
	encSecret := ""
	if strings.TrimSpace(webhookSecret) != "" {
		encSecret, err = Encrypt(p.encKey, webhookSecret)
		if err != nil {
			return 0, err
		}
	}
	revision, err := p.repo.Upsert(ctx, tenantID, &CredentialRow{
		TenantID:               tenantID,
		BaseURL:                strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		AccessTokenEncrypted:   encToken,
		WebhookSecretEncrypted: encSecret,
		Environment:            strings.TrimSpace(environment),
		AuthScheme:             authSchemeOrRaw(authScheme),
	})
	if err != nil {
		return 0, err
	}
	if err := p.repo.AppendRotation(ctx, tenantID, revision, rotatedBy, note); err != nil {
		return revision, fmt.Errorf("credential stored but rotation audit failed: %w", err)
	}
	return revision, nil
}

// MaskedInfo descreve a configuração SEM expor segredos — é o contrato do
// GET administrativo.
type MaskedInfo struct {
	TenantID     string `json:"tenant_id"`
	BaseURL      string `json:"base_url"`
	Environment  string `json:"environment"`
	AuthScheme   string `json:"auth_scheme"`
	Revision     int    `json:"revision"`
	UpdatedAt    string `json:"updated_at"`
	HasToken     bool   `json:"has_token"`
	HasWebhook   bool   `json:"has_webhook_secret"`
	UsesFallback bool   `json:"uses_fallback"`
}

// Info devolve o estado mascarado da credencial do tenant (nunca segredos).
func (p *DBCredentialProvider) Info(ctx context.Context, tenantID string) (*MaskedInfo, error) {
	if p.repo == nil {
		return nil, errors.New("credential repository not configured")
	}
	row, err := p.repo.GetByTenant(ctx, tenantID)
	if errors.Is(err, ErrCredentialTenantNotFound) {
		return &MaskedInfo{TenantID: tenantID, UsesFallback: true}, nil
	}
	if err != nil {
		return nil, err
	}
	return &MaskedInfo{
		TenantID:    row.TenantID,
		BaseURL:     row.BaseURL,
		Environment: row.Environment,
		AuthScheme:  authSchemeOrRaw(row.AuthScheme),
		Revision:    row.Revision,
		UpdatedAt:   row.UpdatedAt,
		HasToken:    row.AccessTokenEncrypted != "",
		HasWebhook:  row.WebhookSecretEncrypted != "",
	}, nil
}

// Rotations lista a trilha de rotações do tenant.
func (p *DBCredentialProvider) Rotations(ctx context.Context, tenantID string, limit int) ([]*CredentialRotation, error) {
	if p.repo == nil {
		return nil, errors.New("credential repository not configured")
	}
	return p.repo.ListRotations(ctx, tenantID, limit)
}
