package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental_charge_config"
)

type rentalChargeConfigPostgresRepository struct {
	db *database.PostgresDB
}

// AllocateIntermediationFunding serializes the read/allocate/write cycle for
// one rental contract. It is normally called inside ChargeService's payment
// transaction; the local transaction fallback keeps repository-level callers
// safe as well.
func (r *rentalChargeConfigPostgresRepository) AllocateIntermediationFunding(
	ctx context.Context,
	tenantID, contractID, paymentID, chargeID, referenceMonth string,
	availableCents int64,
	fundedAt time.Time,
) (*rental_charge_config.RentalChargeConfig, rental_charge_config.IntermediationFundingEntry, bool, error) {
	var cfg *rental_charge_config.RentalChargeConfig
	var entry rental_charge_config.IntermediationFundingEntry
	var changed bool
	work := func(txCtx context.Context) error {
		var metadata []byte
		err := r.db.QueryRow(txCtx, `
			SELECT metadata
			FROM financial.rental_charge_configs
			WHERE tenant_id = $1
			  AND deleted_at IS NULL
			  AND COALESCE(metadata->>'contract_id', metadata->>'ContractID') = $2
			ORDER BY created_at DESC
			LIMIT 1
			FOR UPDATE
		`, tenantID, contractID).Scan(&metadata)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return err
		}
		var current rental_charge_config.RentalChargeConfig
		if err := json.Unmarshal(metadata, &current); err != nil {
			return fmt.Errorf("decode rental charge config for intermediation funding: %w", err)
		}
		entry, changed, err = current.AllocateIntermediationFunding(
			paymentID, chargeID, referenceMonth, availableCents, fundedAt,
		)
		if err != nil {
			return err
		}
		cfg = &current
		if !changed {
			return nil
		}
		current.UpdatedAt = time.Now().UTC().Truncate(time.Microsecond)
		updatedMetadata, err := json.Marshal(&current)
		if err != nil {
			return fmt.Errorf("encode rental charge config funding: %w", err)
		}
		result, err := r.db.Exec(txCtx, `
			UPDATE financial.rental_charge_configs
			SET metadata = $1, updated_at = $2
			WHERE id = $3 AND tenant_id = $4 AND deleted_at IS NULL
		`, string(updatedMetadata), current.UpdatedAt, current.ID, current.TenantID)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows != 1 {
			return errors.New("rental charge config disappeared while funding intermediation")
		}
		cfg = &current
		return nil
	}
	if database.TxFromCtx(ctx) != nil {
		if err := work(ctx); err != nil {
			return nil, rental_charge_config.IntermediationFundingEntry{}, false, err
		}
	} else if err := r.db.RunInTx(ctx, func(tx *sqlx.Tx) error {
		return work(database.WithTx(ctx, tx))
	}); err != nil {
		return nil, rental_charge_config.IntermediationFundingEntry{}, false, err
	}
	return cfg, entry, changed, nil
}

func NewRentalChargeConfigPostgresRepository(db *database.PostgresDB) rental_charge_config.Repository {
	return &rentalChargeConfigPostgresRepository{db: db}
}

func (r *rentalChargeConfigPostgresRepository) Create(ctx context.Context, config *rental_charge_config.RentalChargeConfig) error {
	config.CreatedAt = time.Now()
	metadata, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("encode rental charge config: %w", err)
	}
	query := `INSERT INTO financial.rental_charge_configs (id, tenant_id, metadata)
		VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id, (COALESCE(metadata->>'contract_id', metadata->>'ContractID')))
		WHERE deleted_at IS NULL
		  AND COALESCE(metadata->>'contract_id', metadata->>'ContractID') IS NOT NULL
		DO NOTHING
		RETURNING created_at`
	if err := r.db.QueryRow(ctx, query, config.ID, config.TenantID, string(metadata)).Scan(&config.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return rental_charge_config.ErrAlreadyExists
		}
		return err
	}
	return nil
}

func (r *rentalChargeConfigPostgresRepository) GetByID(ctx context.Context, tenantID, id string) (*rental_charge_config.RentalChargeConfig, error) {
	var metadata []byte
	err := r.db.QueryRow(ctx, "SELECT metadata FROM financial.rental_charge_configs WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL", id, tenantID).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("rental_charge_config lookup by id: %w", err)
	}
	var c rental_charge_config.RentalChargeConfig
	if err := json.Unmarshal(metadata, &c); err != nil {
		return nil, fmt.Errorf("decode rental charge config: %w", err)
	}
	return &c, nil
}

func (r *rentalChargeConfigPostgresRepository) GetByContractID(ctx context.Context, tenantID, contractID string) (*rental_charge_config.RentalChargeConfig, error) {
	var metadata []byte
	err := r.db.QueryRow(ctx, `
		SELECT metadata
		FROM financial.rental_charge_configs
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND COALESCE(metadata->>'contract_id', metadata->>'ContractID') = $2
		ORDER BY created_at DESC
		LIMIT 1
	`, tenantID, contractID).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("rental_charge_config lookup by contract_id: %w", err)
	}
	var c rental_charge_config.RentalChargeConfig
	if err := json.Unmarshal(metadata, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *rentalChargeConfigPostgresRepository) Update(ctx context.Context, config *rental_charge_config.RentalChargeConfig) error {
	config.UpdatedAt = time.Now()
	metadata, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("encode rental charge config: %w", err)
	}
	result, err := r.db.Exec(ctx, "UPDATE financial.rental_charge_configs SET metadata = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL", string(metadata), config.ID, config.TenantID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("rental charge config not found")
	}
	return nil
}

func (r *rentalChargeConfigPostgresRepository) Delete(ctx context.Context, tenantID, id string) error {
	_, err := r.db.Exec(ctx, "UPDATE financial.rental_charge_configs SET deleted_at = NOW() WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL", id, tenantID)
	return err
}
