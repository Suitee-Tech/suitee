package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/tenant_financial_settings"
)

type tenantFinancialSettingsPostgresRepository struct {
	db *database.PostgresDB
}

// NewTenantFinancialSettingsPostgresRepository devolve o port satisfeito
// para o service. O contrato Repository.Get **nunca retorna ErrNoRows** —
// ausência vira defaults via tenant_financial_settings.New(tenantID).
// Vários callers (delinquency, charge_service, rental_charge_config) pegam
// Get no caminho crítico e não devem precisar lidar com "ausente".
func NewTenantFinancialSettingsPostgresRepository(db *database.PostgresDB) tenant_financial_settings.Repository {
	return &tenantFinancialSettingsPostgresRepository{db: db}
}

func (r *tenantFinancialSettingsPostgresRepository) Get(ctx context.Context, tenantID string) (*tenant_financial_settings.Settings, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}
	var metadata []byte
	err := r.db.QueryRow(ctx,
		`SELECT metadata FROM financial.tenant_financial_settings
		 WHERE tenant_id = $1 AND deleted_at IS NULL`,
		tenantID,
	).Scan(&metadata)
	if errors.Is(err, sql.ErrNoRows) {
		// Tenant sem row → devolve defaults populados. Caller não precisa
		// distinguir "novo tenant" vs. "tenant que aceitou defaults".
		return tenant_financial_settings.New(tenantID), nil
	}
	if err != nil {
		return nil, err
	}
	var s tenant_financial_settings.Settings
	if err := json.Unmarshal(metadata, &s); err != nil {
		return nil, err
	}
	// Defensivo: tenant_id no metadata pode estar fora de sincronia com a
	// coluna — coluna ganha porque é o pivot da query.
	s.TenantID = tenantID
	// Para campos ausentes na primeira gravação (ex: tenants que migrarem
	// versão antiga do payload), preencher com defaults globais. A presença
	// da chave é consultada no JSON original para preservar zero/false quando
	// foram configurados explicitamente pelo tenant.
	if err := fillDefaults(&s, metadata); err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *tenantFinancialSettingsPostgresRepository) Upsert(ctx context.Context, s *tenant_financial_settings.Settings) error {
	if s == nil || s.TenantID == "" {
		return errors.New("settings or tenant_id is empty")
	}
	s.UpdatedAt = time.Now().UTC()
	metadata, err := json.Marshal(s)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO financial.tenant_financial_settings (tenant_id, metadata)
		VALUES ($1, $2)
		ON CONFLICT (tenant_id) DO UPDATE
		   SET metadata = EXCLUDED.metadata,
		       updated_at = NOW()
	`, s.TenantID, string(metadata))
	return err
}

// fillDefaults aplica defaults somente a chaves ausentes de um Settings
// desserializado. Comparar o valor Go com zero não é suficiente: comissão,
// juros, multa e taxa administrativa aceitam zero como configuração válida;
// payout_proof_required também pode ser false de forma explícita.
func fillDefaults(s *tenant_financial_settings.Settings, metadata []byte) error {
	var present map[string]json.RawMessage
	if err := json.Unmarshal(metadata, &present); err != nil {
		return err
	}
	def := tenant_financial_settings.New(s.TenantID)
	if _, ok := present["default_late_fee_pct"]; !ok {
		s.DefaultLateFeePct = def.DefaultLateFeePct
	}
	if _, ok := present["default_monthly_interest_pct"]; !ok {
		s.DefaultMonthlyInterestPct = def.DefaultMonthlyInterestPct
	}
	if _, ok := present["default_due_day"]; !ok {
		s.DefaultDueDay = def.DefaultDueDay
	}
	if _, ok := present["default_commission_pct_rent"]; !ok {
		s.DefaultCommissionPctRent = def.DefaultCommissionPctRent
	}
	if _, ok := present["default_commission_pct_sale"]; !ok {
		s.DefaultCommissionPctSale = def.DefaultCommissionPctSale
	}
	if _, ok := present["default_currency"]; !ok {
		s.DefaultCurrency = def.DefaultCurrency
	}
	if _, ok := present["payout_proof_required"]; !ok {
		s.PayoutProofRequired = def.PayoutProofRequired
	}
	if _, ok := present["admin_fee_pct"]; !ok {
		s.AdminFeePct = def.AdminFeePct
	}
	if _, ok := present["default_lease_term_months"]; !ok {
		s.DefaultLeaseTermMonths = def.DefaultLeaseTermMonths
	}
	return nil
}
