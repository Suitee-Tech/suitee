package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/oginoino/toca-auth/internal/domain/tenantsettings"
	"github.com/oginoino/toca/pkg/database"
)

const defaultFallbackRole = "corretor"

type tenantSettingsPostgresRepository struct {
	db *database.PostgresDB
}

func NewTenantSettingsPostgresRepository(db *database.PostgresDB) *tenantSettingsPostgresRepository {
	return &tenantSettingsPostgresRepository{db: db}
}

func (r *tenantSettingsPostgresRepository) GetByTenantID(ctx context.Context, tenantID string) (map[string]interface{}, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata
		FROM toca_auth.tenant_settings
		WHERE tenant_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT 1
	`, tenantID).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return map[string]interface{}{}, nil
		}
		return map[string]interface{}{}, nil
	}

	var out map[string]interface{}
	if err := json.Unmarshal(metadata, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]interface{}{}
	}
	return out, nil
}

func (r *tenantSettingsPostgresRepository) GetDefaultRole(ctx context.Context, tenantID string) (string, error) {
	var role sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(metadata->>'defaultRole', metadata->>'default_role')
		FROM core.role_permissions
		WHERE tenant_id = $1
		LIMIT 1
	`, tenantID).Scan(&role)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return defaultFallbackRole, nil
		}
		return defaultFallbackRole, nil
	}
	if !role.Valid || role.String == "" {
		return defaultFallbackRole, nil
	}
	return role.String, nil
}

// GetAccessProfileBaseRole devolve o PAPEL DO CATÁLOGO em que um PERFIL DE
// ACESSO personalizado se baseia (`customRoles[].baseRole`), ou "" quando o id
// não é um perfil do tenant.
//
// Por que o toca-auth lê isto: é ele quem EMITE o token do contexto da empresa
// (`functional_role`). Quando o papel do membro é um perfil (`cr_…`), o papel
// base precisa viajar junto — é o que permite aos serviços decidirem
// VISIBILIDADE (carteira x tenant) para quem tem perfil personalizado, sem uma
// consulta ao core por requisição. Mesmo padrão cross-schema já usado por
// GetDefaultRole.
func (r *tenantSettingsPostgresRepository) GetAccessProfileBaseRole(ctx context.Context, tenantID, profileID string) (string, error) {
	if profileID == "" {
		return "", nil
	}
	var base sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(profile->>'baseRole', profile->>'base_role')
		FROM core.role_permissions rp
		CROSS JOIN LATERAL jsonb_array_elements(COALESCE(rp.metadata->'customRoles', '[]'::jsonb)) AS profile
		WHERE rp.tenant_id = $1::uuid AND profile->>'id' = $2
		LIMIT 1
	`, tenantID, profileID).Scan(&base)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", nil
	}
	if !base.Valid {
		return "", nil
	}
	return base.String, nil
}

// GetApprovalPolicy reads the typed, versioned policy stored inside
// toca_auth.tenant_settings.metadata. The bool distinguishes a persisted
// tenant policy from the platform default selected by the service layer.
func (r *tenantSettingsPostgresRepository) GetApprovalPolicy(ctx context.Context, tenantID string) (tenantsettings.ApprovalPolicy, bool, error) {
	var raw []byte
	err := r.db.QueryServiceRow(ctx, `
		SELECT metadata -> 'approval_policy'
		FROM toca_auth.tenant_settings
		WHERE tenant_id = $1::uuid AND deleted_at IS NULL
	`, tenantID).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tenantsettings.ApprovalPolicy{}, false, nil
		}
		return tenantsettings.ApprovalPolicy{}, false, fmt.Errorf("get approval policy: %w", err)
	}
	if len(raw) == 0 || string(raw) == "null" {
		return tenantsettings.ApprovalPolicy{}, false, nil
	}

	var policy tenantsettings.ApprovalPolicy
	if err := json.Unmarshal(raw, &policy); err != nil {
		return tenantsettings.ApprovalPolicy{}, false, fmt.Errorf("decode approval policy: %w", err)
	}
	policy.Source = "tenant_settings"
	if err := policy.Validate(); err != nil {
		return tenantsettings.ApprovalPolicy{}, false, fmt.Errorf("invalid persisted approval policy: %w", err)
	}
	return policy, true, nil
}

// UpsertApprovalPolicy writes a complete v1 policy and atomically increments
// its tenant revision. Keeping the increment in Postgres prevents concurrent
// settings updates from publishing the same revision.
func (r *tenantSettingsPostgresRepository) UpsertApprovalPolicy(
	ctx context.Context,
	tenantID string,
	enabled bool,
	thresholdBRL float64,
	updatedBy string,
) (tenantsettings.ApprovalPolicy, error) {
	var raw []byte
	err := r.db.QueryServiceRow(ctx, `
		INSERT INTO toca_auth.tenant_settings (tenant_id, metadata)
		VALUES (
			$1::uuid,
			jsonb_build_object(
				'approval_policy', jsonb_build_object(
					'schema_version', 1,
					'revision', 1,
					'enabled', $2::boolean,
					'threshold_brl', $3::numeric,
					'target_statuses', jsonb_build_array('ANALYSIS', 'CONTRACT'),
					'required_permission', 'deals:approve',
					'updated_at', NOW(),
					'updated_by', $4::text
				)
			)
		)
		ON CONFLICT (tenant_id) DO UPDATE SET
			metadata = jsonb_set(
				COALESCE(toca_auth.tenant_settings.metadata, '{}'::jsonb),
				'{approval_policy}',
				jsonb_build_object(
					'schema_version', 1,
					'revision', CASE
						WHEN (toca_auth.tenant_settings.metadata #>> '{approval_policy,revision}') ~ '^[0-9]+$'
						THEN (toca_auth.tenant_settings.metadata #>> '{approval_policy,revision}')::bigint + 1
						ELSE 1
					END,
					'enabled', EXCLUDED.metadata #> '{approval_policy,enabled}',
					'threshold_brl', EXCLUDED.metadata #> '{approval_policy,threshold_brl}',
					'target_statuses', jsonb_build_array('ANALYSIS', 'CONTRACT'),
					'required_permission', 'deals:approve',
					'updated_at', NOW(),
					'updated_by', $4::text
				),
				true
			),
			updated_at = NOW(),
			deleted_at = NULL
		RETURNING metadata -> 'approval_policy'
	`, tenantID, enabled, thresholdBRL, updatedBy).Scan(&raw)
	if err != nil {
		return tenantsettings.ApprovalPolicy{}, fmt.Errorf("upsert approval policy: %w", err)
	}

	var policy tenantsettings.ApprovalPolicy
	if err := json.Unmarshal(raw, &policy); err != nil {
		return tenantsettings.ApprovalPolicy{}, fmt.Errorf("decode updated approval policy: %w", err)
	}
	policy.Source = "tenant_settings"
	if err := policy.Validate(); err != nil {
		return tenantsettings.ApprovalPolicy{}, fmt.Errorf("invalid updated approval policy: %w", err)
	}
	return policy, nil
}
