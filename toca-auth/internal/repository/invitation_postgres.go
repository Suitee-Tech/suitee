package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca-auth/internal/domain/invitation"
	"github.com/oginoino/toca/pkg/database"
)

type invitationPostgresRepository struct {
	db *database.PostgresDB
}

func NewInvitationPostgresRepository(db *database.PostgresDB) invitation.Repository {
	return &invitationPostgresRepository{db: db}
}

// NewInvitationServicePostgresRepository returns the pre-authentication
// (service-role) surface of the invitation store. See invitation.ServiceRepository.
func NewInvitationServicePostgresRepository(db *database.PostgresDB) invitation.ServiceRepository {
	return &invitationPostgresRepository{db: db}
}

func (r *invitationPostgresRepository) Create(ctx context.Context, invite *invitation.Invitation) error {
	if invite.ID == "" {
		invite.ID = uuid.New().String()
	}
	invite.CreatedAt = time.Now()
	metadata, _ := json.Marshal(invite)
	query := `INSERT INTO toca_auth.invitations (id, company_id, email, token, status, metadata) VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := r.db.ExecContext(ctx, query, invite.ID, invite.CompanyID, invite.Email, invite.Token, invite.Status, string(metadata))
	return err
}

func (r *invitationPostgresRepository) GetByToken(ctx context.Context, token string) (*invitation.Invitation, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, "SELECT metadata FROM toca_auth.invitations WHERE token = $1 AND deleted_at IS NULL", token).Scan(&metadata)
	if err != nil {
		return nil, nil
	}
	var invite invitation.Invitation
	_ = json.Unmarshal(metadata, &invite)
	return &invite, nil
}

// GetByTokenService lê o convite pela pool de serviço (BYPASSRLS) — usado no
// link público de convite, que não tem JWT de tenant.
func (r *invitationPostgresRepository) GetByTokenService(ctx context.Context, token string) (*invitation.Invitation, error) {
	var metadata []byte
	err := r.db.QueryServiceRow(ctx, "SELECT metadata FROM toca_auth.invitations WHERE token = $1 AND deleted_at IS NULL", token).Scan(&metadata)
	if err != nil {
		return nil, nil
	}
	var invite invitation.Invitation
	_ = json.Unmarshal(metadata, &invite)
	return &invite, nil
}

func (r *invitationPostgresRepository) GetByEmailAndCompany(ctx context.Context, email, companyID string) (*invitation.Invitation, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, "SELECT metadata FROM toca_auth.invitations WHERE email = $1 AND company_id = $2 AND status = 'pending' AND deleted_at IS NULL", email, companyID).Scan(&metadata)
	if err != nil {
		return nil, nil
	}
	var invite invitation.Invitation
	_ = json.Unmarshal(metadata, &invite)
	return &invite, nil
}

func (r *invitationPostgresRepository) UpdateStatus(ctx context.Context, companyID, id, status string) error {
	companyID = strings.TrimSpace(companyID)
	if companyID == "" {
		return fmt.Errorf("invitation not found: tenant context required")
	}
	_, err := r.db.ExecContext(ctx, "UPDATE toca_auth.invitations SET status = $1, metadata = jsonb_set(metadata, '{status}', to_jsonb($1::text)), updated_at = NOW() WHERE id = $2 AND company_id = $3 AND deleted_at IS NULL", status, id, companyID)
	return err
}

// UpdateStatusService marca o status do convite pela pool de serviço (BYPASSRLS)
// — usado no aceite público, sem JWT de tenant.
func (r *invitationPostgresRepository) UpdateStatusService(ctx context.Context, companyID, id, status string) error {
	companyID = strings.TrimSpace(companyID)
	if companyID == "" {
		return fmt.Errorf("invitation not found: tenant context required")
	}
	return r.db.ExecService(ctx, "UPDATE toca_auth.invitations SET status = $1, metadata = jsonb_set(metadata, '{status}', to_jsonb($1::text)), updated_at = NOW() WHERE id = $2 AND company_id = $3 AND deleted_at IS NULL", status, id, companyID)
}

func (r *invitationPostgresRepository) ListByCompany(ctx context.Context, companyID string) ([]*invitation.Invitation, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT metadata
		FROM toca_auth.invitations
		WHERE company_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*invitation.Invitation, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var inv invitation.Invitation
		if err := json.Unmarshal(metadata, &inv); err != nil {
			return nil, err
		}
		out = append(out, &inv)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
