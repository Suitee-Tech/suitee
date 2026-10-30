package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca-auth/internal/domain/account"
	"github.com/oginoino/toca/pkg/database"
)

type accountPostgresRepository struct {
	db *database.PostgresDB
}

func NewAccountPostgresRepository(db *database.PostgresDB) account.Repository {
	return &accountPostgresRepository{db: db}
}

// NewAccountServicePostgresRepository returns the pre-authentication
// (service-role) surface of the account store. See account.ServiceRepository.
func NewAccountServicePostgresRepository(db *database.PostgresDB) account.ServiceRepository {
	return &accountPostgresRepository{db: db}
}

func (r *accountPostgresRepository) Create(ctx context.Context, acc *account.Account) error {
	if acc.ID == "" {
		acc.ID = uuid.NewString()
		if acc.Individual != nil && acc.Individual.ID == "" {
			acc.Individual.ID = acc.ID
		}
	}
	acc.CreatedAt = time.Now()
	acc.UpdatedAt = time.Now()

	metadata, _ := json.Marshal(acc)
	query := `INSERT INTO toca_auth.accounts (id, metadata) VALUES ($1, $2)`
	_, err := r.db.ExecContext(ctx, query, acc.ID, string(metadata))
	return err
}

func (r *accountPostgresRepository) GetByID(ctx context.Context, id string) (*account.Account, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, "SELECT metadata FROM toca_auth.accounts WHERE id = $1", id).Scan(&metadata)
	if err != nil {
		return nil, nil
	}
	var acc account.Account
	_ = json.Unmarshal(metadata, &acc)
	return &acc, nil
}

func (r *accountPostgresRepository) Update(ctx context.Context, acc *account.Account) error {
	acc.UpdatedAt = time.Now()
	metadata, _ := json.Marshal(acc)
	_, err := r.db.ExecContext(ctx, "UPDATE toca_auth.accounts SET metadata = $1 WHERE id = $2", string(metadata), acc.ID)
	return err
}

func (r *accountPostgresRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM toca_auth.accounts WHERE id = $1", id)
	return err
}

func (r *accountPostgresRepository) GetCompaniesByMemberID(ctx context.Context, memberID string) ([]*account.Account, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT metadata
		FROM toca_auth.accounts a
		WHERE a.metadata->>'Type' = 'company'
		  AND EXISTS (
				SELECT 1
				FROM jsonb_array_elements(a.metadata->'Company'->'Members') m
				WHERE m->>'account_id' = $1
		  )
	`, memberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*account.Account
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var acc account.Account
		_ = json.Unmarshal(metadata, &acc)
		out = append(out, &acc)
	}
	return out, rows.Err()
}

func (r *accountPostgresRepository) GetCompanyByTaxIDOrEmail(ctx context.Context, taxID, email string) (*account.Account, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata
		FROM toca_auth.accounts
		WHERE metadata->>'Type' = 'company'
		  AND (
				regexp_replace(metadata->'Company'->>'TaxID', '\D', '', 'g') = $1
		  )
		LIMIT 1
	`, taxID).Scan(&metadata)
	if err != nil {
		return nil, nil
	}
	var acc account.Account
	_ = json.Unmarshal(metadata, &acc)
	return &acc, nil
}

func (r *accountPostgresRepository) GetByIndividualEmail(ctx context.Context, email string) (*account.Account, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata
		FROM toca_auth.accounts
		WHERE metadata->>'Type' = 'individual'
		  AND lower(metadata->'Individual'->>'Email') = lower($1)
		LIMIT 1
	`, email).Scan(&metadata)
	if err != nil {
		return nil, nil
	}
	var acc account.Account
	_ = json.Unmarshal(metadata, &acc)
	return &acc, nil
}

func (r *accountPostgresRepository) GetByIndividualPhone(ctx context.Context, phone string) (*account.Account, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata
		FROM toca_auth.accounts
		WHERE metadata->>'Type' = 'individual'
		  AND (metadata->'Individual'->>'Phone') = $1
		LIMIT 1
	`, phone).Scan(&metadata)
	if err != nil {
		return nil, nil
	}
	var acc account.Account
	_ = json.Unmarshal(metadata, &acc)
	return &acc, nil
}

// ── Service-role (pre-authentication) variants ────────────────────────
// These run on the service pool (toca_service, BYPASSRLS) because the
// caller has no JWT and the tenant-scoped pool would be RLS-denied.

func (r *accountPostgresRepository) CreateService(ctx context.Context, acc *account.Account) error {
	if acc.ID == "" {
		acc.ID = uuid.NewString()
		if acc.Individual != nil && acc.Individual.ID == "" {
			acc.Individual.ID = acc.ID
		}
	}
	acc.CreatedAt = time.Now()
	acc.UpdatedAt = time.Now()

	metadata, _ := json.Marshal(acc)
	query := `INSERT INTO toca_auth.accounts (id, metadata) VALUES ($1, $2)`
	return r.db.ExecService(ctx, query, acc.ID, string(metadata))
}

func (r *accountPostgresRepository) GetByIDService(ctx context.Context, id string) (*account.Account, error) {
	var metadata []byte
	err := r.db.QueryServiceRow(ctx, "SELECT metadata FROM toca_auth.accounts WHERE id = $1", id).Scan(&metadata)
	if err != nil {
		return nil, nil
	}
	var acc account.Account
	_ = json.Unmarshal(metadata, &acc)
	return &acc, nil
}

func (r *accountPostgresRepository) GetByIndividualEmailService(ctx context.Context, email string) (*account.Account, error) {
	var metadata []byte
	err := r.db.QueryServiceRow(ctx, `
		SELECT metadata
		FROM toca_auth.accounts
		WHERE metadata->>'Type' = 'individual'
		  AND lower(metadata->'Individual'->>'Email') = lower($1)
		LIMIT 1
	`, email).Scan(&metadata)
	if err != nil {
		return nil, nil
	}
	var acc account.Account
	_ = json.Unmarshal(metadata, &acc)
	return &acc, nil
}

func (r *accountPostgresRepository) GetByIndividualPhoneService(ctx context.Context, phone string) (*account.Account, error) {
	var metadata []byte
	err := r.db.QueryServiceRow(ctx, `
		SELECT metadata
		FROM toca_auth.accounts
		WHERE metadata->>'Type' = 'individual'
		  AND (metadata->'Individual'->>'Phone') = $1
		LIMIT 1
	`, phone).Scan(&metadata)
	if err != nil {
		return nil, nil
	}
	var acc account.Account
	_ = json.Unmarshal(metadata, &acc)
	return &acc, nil
}

func (r *accountPostgresRepository) UpdateService(ctx context.Context, acc *account.Account) error {
	acc.UpdatedAt = time.Now()
	metadata, _ := json.Marshal(acc)
	return r.db.ExecService(ctx, "UPDATE toca_auth.accounts SET metadata = $1 WHERE id = $2", string(metadata), acc.ID)
}

func (r *accountPostgresRepository) DeleteService(ctx context.Context, id string) error {
	return r.db.ExecService(ctx, "DELETE FROM toca_auth.accounts WHERE id = $1", id)
}
