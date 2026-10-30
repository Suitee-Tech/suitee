package repositories

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/oginoino/toca/pkg/database"
)

// TenantIdentityPostgresRepository lê a identificação da imobiliária logada
// (o tenant) para documentos que não são donos desse dado — o cadastro da
// empresa vive em toca_auth.accounts, fora do schema finance.
type TenantIdentityPostgresRepository struct {
	db *database.PostgresDB
}

func NewTenantIdentityPostgresRepository(db *database.PostgresDB) *TenantIdentityPostgresRepository {
	return &TenantIdentityPostgresRepository{db: db}
}

// CompanyName devolve o nome que deve ser impresso nos recibos. Prefere o
// nome fantasia (Company.Name) e cai para a razão social (Company.LegalName);
// string vazia significa "sem nome" e o chamador mantém o fallback neutro.
//
// Roda no pool de serviço porque o papel do browser não lê o registro da
// empresa — mesma premissa de PlatformSubscriptionRepository.TenantIdentity.
func (r *TenantIdentityPostgresRepository) CompanyName(ctx context.Context, tenantID string) (string, error) {
	if tenantID == "" {
		return "", nil
	}
	var name string
	err := r.db.QueryServiceRow(ctx,
		`SELECT COALESCE(NULLIF(metadata->'Company'->>'Name', ''), NULLIF(metadata->'Company'->>'LegalName', ''), '')
		   FROM toca_auth.accounts WHERE id=$1::uuid`, tenantID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return name, nil
}
