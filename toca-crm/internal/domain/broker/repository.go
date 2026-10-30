package broker

import (
	"context"
	"time"
)

type Repository interface {
	GetByIDs(ctx context.Context, tenantID string, ids []string) (map[string]*Broker, error)
	// GetByID resolve o corretor DENTRO do tenant informado.
	//
	// SECURITY (achado A19 da revisão de permissões de 2026-09-19): a query
	// filtrava só por `id`, devolvendo PII de corretor (nome, e-mail, telefone,
	// papel, pesos) de qualquer tenant para quem tivesse a rota de leitura.
	GetByID(ctx context.Context, tenantID, id string) (*Broker, error)
	Create(ctx context.Context, b *Broker) error
	Update(ctx context.Context, b *Broker) error
	SetPaused(ctx context.Context, brokerID, tenantID string, paused bool) (bool, error)
	// SetPausedUntil atualiza Paused + PausedUntil em uma única transação
	// para suportar auto-unpause (Wave P1.7). Passar `until=nil` mantém o
	// comportamento legado (pausa indefinida). Para despausar imediatamente,
	// chame com paused=false e until=nil.
	SetPausedUntil(ctx context.Context, brokerID, tenantID string, paused bool, until *time.Time) (bool, error)
	Delete(ctx context.Context, id, tenantID string) error
	List(ctx context.Context, tenantID string) ([]*Broker, error)
	Search(ctx context.Context, tenantID string, filter BrokerFilter) ([]*Broker, int64, error)
	GetPerformance(ctx context.Context, brokerID, tenantID string) (*BrokerPerformance, error)
}

type BrokerPerformance struct {
	SalesVolume    float64 `json:"sales_volume"`
	ActiveLeads    int     `json:"active_leads"`
	ConversionRate float64 `json:"conversion_rate"`
}
