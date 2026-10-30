package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca-auth/internal/domain/auth"
	"github.com/oginoino/toca/pkg/database"
)

type credentialPostgresRepository struct {
	db *database.PostgresDB
}

func NewCredentialPostgresRepository(db *database.PostgresDB) auth.Repository {
	return &credentialPostgresRepository{db: db}
}

// NewCredentialServicePostgresRepository returns the pre-authentication
// (service-role) surface of the credential store. See auth.ServiceRepository.
func NewCredentialServicePostgresRepository(db *database.PostgresDB) auth.ServiceRepository {
	return &credentialPostgresRepository{db: db}
}

func (r *credentialPostgresRepository) Create(ctx context.Context, cred *auth.Credential) error {
	if cred.ID == "" {
		cred.ID = uuid.NewString()
	}
	cred.CreatedAt = time.Now()
	cred.UpdatedAt = time.Now()

	existing, _ := r.GetByEmail(ctx, cred.Email)
	if existing != nil {
		return errors.New("email already exists")
	}

	metadata, _ := json.Marshal(cred)
	query := `INSERT INTO toca_auth.credentials (id, email, account_id, metadata) VALUES ($1, $2, $3, $4)`
	_, err := r.db.ExecContext(ctx, query, cred.ID, cred.Email, cred.AccountID, string(metadata))
	return err
}

func (r *credentialPostgresRepository) GetByEmail(ctx context.Context, email string) (*auth.Credential, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, "SELECT metadata FROM toca_auth.credentials WHERE email = $1", email).Scan(&metadata)
	if err != nil {
		return nil, nil
	}
	var cred auth.Credential
	_ = json.Unmarshal(metadata, &cred)
	return &cred, nil
}

func (r *credentialPostgresRepository) GetByAccountID(ctx context.Context, accountID string) (*auth.Credential, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, "SELECT metadata FROM toca_auth.credentials WHERE account_id = $1", accountID).Scan(&metadata)
	if err != nil {
		return nil, nil
	}
	var cred auth.Credential
	_ = json.Unmarshal(metadata, &cred)
	return &cred, nil
}

func (r *credentialPostgresRepository) UpdatePassword(ctx context.Context, email, passwordHash string) error {
	cred, err := r.GetByEmail(ctx, email)
	if err != nil || cred == nil {
		return fmt.Errorf("credential not found")
	}
	cred.PasswordHash = passwordHash
	cred.UpdatedAt = time.Now()
	metadata, _ := json.Marshal(cred)
	_, err = r.db.ExecContext(ctx, "UPDATE toca_auth.credentials SET metadata = $1 WHERE email = $2", string(metadata), email)
	return err
}

func (r *credentialPostgresRepository) VerifyEmail(ctx context.Context, email string) error {
	cred, err := r.GetByEmail(ctx, email)
	if err != nil || cred == nil {
		return fmt.Errorf("credential not found")
	}
	cred.IsVerified = true
	cred.UpdatedAt = time.Now()
	metadata, _ := json.Marshal(cred)
	_, err = r.db.ExecContext(ctx, "UPDATE toca_auth.credentials SET metadata = $1 WHERE email = $2", string(metadata), email)
	return err
}

// ── Service-role (pre-authentication) variants ────────────────────────

func (r *credentialPostgresRepository) CreateService(ctx context.Context, cred *auth.Credential) error {
	if cred.ID == "" {
		cred.ID = uuid.NewString()
	}
	cred.CreatedAt = time.Now()
	cred.UpdatedAt = time.Now()

	metadata, _ := json.Marshal(cred)
	query := `INSERT INTO toca_auth.credentials (id, email, account_id, metadata) VALUES ($1, $2, $3, $4)`
	return r.db.ExecService(ctx, query, cred.ID, cred.Email, cred.AccountID, string(metadata))
}

func (r *credentialPostgresRepository) GetByEmailService(ctx context.Context, email string) (*auth.Credential, error) {
	var metadata []byte
	err := r.db.QueryServiceRow(ctx, "SELECT metadata FROM toca_auth.credentials WHERE email = $1", email).Scan(&metadata)
	if err != nil {
		return nil, nil
	}
	var cred auth.Credential
	_ = json.Unmarshal(metadata, &cred)
	return &cred, nil
}

func (r *credentialPostgresRepository) GetByAccountIDService(ctx context.Context, accountID string) (*auth.Credential, error) {
	var metadata []byte
	err := r.db.QueryServiceRow(ctx, "SELECT metadata FROM toca_auth.credentials WHERE account_id = $1", accountID).Scan(&metadata)
	if err != nil {
		return nil, nil
	}
	var cred auth.Credential
	_ = json.Unmarshal(metadata, &cred)
	return &cred, nil
}
