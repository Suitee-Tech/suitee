package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/oginoino/toca-auth/internal/domain/auth"
	"github.com/oginoino/toca/pkg/database"
)

type passwordResetPostgresRepository struct {
	db *database.PostgresDB
}

func NewPasswordResetPostgresRepository(db *database.PostgresDB) auth.PasswordResetRepository {
	return &passwordResetPostgresRepository{db: db}
}

func (r *passwordResetPostgresRepository) Create(ctx context.Context, reset *auth.PasswordResetToken) error {
	reset.CreatedAt = time.Now()
	metadata, _ := json.Marshal(reset)
	query := `INSERT INTO toca_auth.password_resets (id, token, metadata) VALUES ($1, $2, $3)`
	_, err := r.db.ExecContext(ctx, query, reset.ID, reset.Token, string(metadata))
	return err
}

func (r *passwordResetPostgresRepository) GetByToken(ctx context.Context, token string) (*auth.PasswordResetToken, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, "SELECT metadata FROM toca_auth.password_resets WHERE token = $1", token).Scan(&metadata)
	if err != nil {
		return nil, nil
	}
	var reset auth.PasswordResetToken
	_ = json.Unmarshal(metadata, &reset)
	return &reset, nil
}

func (r *passwordResetPostgresRepository) MarkAsUsed(ctx context.Context, token string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE toca_auth.password_resets SET metadata = jsonb_set(metadata, '{used}', 'true'::jsonb) WHERE token = $1", token)
	return err
}
