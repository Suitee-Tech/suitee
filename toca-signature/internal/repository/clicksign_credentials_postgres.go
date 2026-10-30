// Package repository — persistência das credenciais Clicksign por tenant
// (P1-14). Leituras e escritas usam o POOL DE SERVIÇO (BYPASSRLS), o mesmo
// padrão do repositório de envelopes: o tenant é resolvido da credencial
// do chamador e conferido no código; o RLS protege o pool de aplicação.
package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-signature/internal/domain/credential"
)

// clicksignCredentialsPostgresRepository implementa credential.CredentialRepository.
// Os segredos chegam/partem CRIPTOGRAFADOS (envelope "v1:..." do
// domain/credential) — este repositório nunca vê texto puro.
type clicksignCredentialsPostgresRepository struct {
	db *database.PostgresDB
}

func NewClicksignCredentialsPostgresRepository(db *database.PostgresDB) credential.CredentialRepository {
	return &clicksignCredentialsPostgresRepository{db: db}
}

func (r *clicksignCredentialsPostgresRepository) GetByTenant(ctx context.Context, tenantID string) (*credential.CredentialRow, error) {
	var row credential.CredentialRow
	err := r.db.QueryServiceRow(ctx, `
		SELECT tenant_id::text, base_url, access_token_encrypted,
		       COALESCE(webhook_secret_encrypted, ''), environment, auth_scheme,
		       revision, updated_at::text
		FROM signature.clicksign_credentials
		WHERE tenant_id = $1 AND deleted_at IS NULL
	`, tenantID).Scan(
		&row.TenantID, &row.BaseURL, &row.AccessTokenEncrypted,
		&row.WebhookSecretEncrypted, &row.Environment, &row.AuthScheme,
		&row.Revision, &row.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, credential.ErrCredentialTenantNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *clicksignCredentialsPostgresRepository) Upsert(ctx context.Context, tenantID string, row *credential.CredentialRow) (int, error) {
	var revision int
	err := r.db.QueryServiceRow(ctx, `
		INSERT INTO signature.clicksign_credentials
			(tenant_id, base_url, access_token_encrypted, webhook_secret_encrypted,
			 environment, auth_scheme, revision, updated_at, deleted_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, 1, NOW(), NULL)
		ON CONFLICT (tenant_id) DO UPDATE SET
			base_url = EXCLUDED.base_url,
			access_token_encrypted = EXCLUDED.access_token_encrypted,
			webhook_secret_encrypted = EXCLUDED.webhook_secret_encrypted,
			environment = EXCLUDED.environment,
			auth_scheme = EXCLUDED.auth_scheme,
			revision = signature.clicksign_credentials.revision + 1,
			updated_at = NOW(),
			deleted_at = NULL
		RETURNING revision
	`, tenantID, row.BaseURL, row.AccessTokenEncrypted, row.WebhookSecretEncrypted,
		row.Environment, row.AuthScheme).Scan(&revision)
	if err != nil {
		return 0, err
	}
	return revision, nil
}

func (r *clicksignCredentialsPostgresRepository) AppendRotation(ctx context.Context, tenantID string, revision int, rotatedBy, note string) error {
	return r.db.ExecService(ctx, `
		INSERT INTO signature.clicksign_credential_rotations (tenant_id, revision, rotated_by, note)
		VALUES ($1, $2, $3, NULLIF($4, ''))
	`, tenantID, revision, rotatedBy, note)
}

func (r *clicksignCredentialsPostgresRepository) ListRotations(ctx context.Context, tenantID string, limit int) ([]*credential.CredentialRotation, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.db.QueryServiceRows(ctx, `
		SELECT tenant_id::text, revision, rotated_by, COALESCE(note, ''), created_at::text
		FROM signature.clicksign_credential_rotations
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*credential.CredentialRotation, 0, limit)
	for rows.Next() {
		var rot credential.CredentialRotation
		if err := rows.Scan(&rot.TenantID, &rot.Revision, &rot.RotatedBy, &rot.Note, &rot.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &rot)
	}
	return out, rows.Err()
}
