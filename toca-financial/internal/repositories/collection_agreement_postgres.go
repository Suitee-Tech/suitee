package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/collection"
)

// collectionAgreementPostgresRepository persiste o acordo em
// financial.collection_agreements (metadata JSONB), no mesmo padrão do caso:
// RLS por tenant no banco e predicado de tenant em toda escrita.
type collectionAgreementPostgresRepository struct {
	db *database.PostgresDB
}

func NewCollectionAgreementPostgresRepository(db *database.PostgresDB) collection.AgreementRepository {
	return &collectionAgreementPostgresRepository{db: db}
}

func (r *collectionAgreementPostgresRepository) Create(ctx context.Context, a *collection.Agreement) error {
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}
	a.UpdatedAt = time.Now()
	metadata, err := json.Marshal(a)
	if err != nil {
		return fmt.Errorf("encode agreement: %w", err)
	}
	query := `INSERT INTO financial.collection_agreements (id, tenant_id, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $4) RETURNING created_at`
	return r.db.QueryRow(ctx, query, a.ID, a.TenantID, string(metadata), a.CreatedAt).Scan(&a.CreatedAt)
}

func (r *collectionAgreementPostgresRepository) GetByID(ctx context.Context, tenantID, id string) (*collection.Agreement, error) {
	var metadata []byte
	err := r.db.QueryRow(ctx,
		`SELECT metadata FROM financial.collection_agreements
		 WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`, id, tenantID).Scan(&metadata)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return decodeAgreement(metadata)
}

// GetActiveByCollection devolve o acordo vigente do caso (DRAFT ou ACTIVE).
// O índice único parcial garante no máximo um.
func (r *collectionAgreementPostgresRepository) GetActiveByCollection(ctx context.Context, tenantID, collectionID string) (*collection.Agreement, error) {
	var metadata []byte
	err := r.db.QueryRow(ctx,
		`SELECT metadata FROM financial.collection_agreements
		 WHERE tenant_id = $1
		   AND deleted_at IS NULL
		   AND (metadata->>'collection_id') = $2
		   AND (metadata->>'status') IN ('DRAFT','ACTIVE')
		 ORDER BY updated_at DESC
		 LIMIT 1`, tenantID, collectionID).Scan(&metadata)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return decodeAgreement(metadata)
}

// Update grava o acordo com proteção de concorrência otimista por updated_at:
// duas quebras simultâneas não podem gerar duas transições.
func (r *collectionAgreementPostgresRepository) Update(ctx context.Context, a *collection.Agreement) error {
	a.UpdatedAt = time.Now()
	metadata, err := json.Marshal(a)
	if err != nil {
		return fmt.Errorf("encode agreement: %w", err)
	}
	query := `UPDATE financial.collection_agreements
		SET metadata = $3, updated_at = $4
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`
	tag, err := r.db.Exec(ctx, query, a.ID, a.TenantID, string(metadata), a.UpdatedAt)
	if err != nil {
		return err
	}
	afetadas, err := tag.RowsAffected()
	if err != nil {
		return err
	}
	if afetadas == 0 {
		return collection.ErrAgreementNotFound
	}
	return nil
}

func decodeAgreement(metadata []byte) (*collection.Agreement, error) {
	var a collection.Agreement
	if err := json.Unmarshal(metadata, &a); err != nil {
		return nil, fmt.Errorf("decode agreement: %w", err)
	}
	return &a, nil
}
