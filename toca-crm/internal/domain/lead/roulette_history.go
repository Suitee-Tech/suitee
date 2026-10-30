package lead

import "time"

// RouletteHistory registra decisões de distribuição (fonte de verdade: CRM).
type RouletteHistory struct {
	ID           string    `bson:"_id"`
	TenantID     string    `bson:"tenant_id"`
	LeadID       string    `bson:"lead_id"`
	FromBrokerID string    `bson:"from_broker_id,omitempty"`
	ToBrokerID   string    `bson:"to_broker_id"`
	Stage        string    `bson:"stage"`  // normalmente status do lead (NEW, QUALIFIED, ...)
	Method       string    `bson:"method"` // sequential, load_balance, etc.
	Reason       string    `bson:"reason,omitempty"`
	CreatedAt    time.Time `bson:"created_at"`
}
