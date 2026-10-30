package tenant_financial_settings

import "context"

// Repository é o port que o service consome. O contrato garante que Get
// nunca falha com "not found": quando não há row, devolve um Settings
// populado com defaults (via New). Esse contrato é importante porque
// vários callers (delinquency, charge_service, rental_charge_config)
// chamam Get no caminho crítico e não devem precisar tratar "ausente".
type Repository interface {
	Get(ctx context.Context, tenantID string) (*Settings, error)
	Upsert(ctx context.Context, s *Settings) error
}
