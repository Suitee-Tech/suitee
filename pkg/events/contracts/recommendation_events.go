package contracts

import (
	"time"

	"github.com/google/uuid"
)

// RecommendedPropertyEventPayload é o shape compartilhado dos eventos de
// recomendação de imóvel do corretor ao cliente ("Sugestão do seu corretor",
// beta de e-mail transacional F3). Publicado por toca-crm (escopo lead) e
// toca-deal (escopo negócio) quando o corretor define o imóvel recomendado;
// consumido por toca-comm para in-app + e-mail ao cliente.
//
// O payload carrega o snapshot do imóvel (título/bairro/preço/foto) para o
// consumidor não precisar round-trip ao core do tenant.
type RecommendedPropertyEventPayload struct {
	TenantID           string   `json:"tenant_id"`
	LeadID             string   `json:"lead_id,omitempty"`
	DealID             string   `json:"deal_id,omitempty"`
	PropertyID         string   `json:"property_id"`
	Title              string   `json:"title,omitempty"`
	Neighborhood       string   `json:"neighborhood,omitempty"`
	City               string   `json:"city,omitempty"`
	State              string   `json:"state,omitempty"`
	SalePrice          float64  `json:"sale_price,omitempty"`
	RentPrice          float64  `json:"rent_price,omitempty"`
	ListingType        string   `json:"listing_type,omitempty"`
	ImageURL           string   `json:"image_url,omitempty"`
	BrokerID           string   `json:"broker_id,omitempty"`
	CustomerAccountIDs []string `json:"customer_account_ids,omitempty"`
	Timestamp          string   `json:"timestamp"`
}

type LeadRecommendedPropertyEvent struct {
	EventIDVal string                         `json:"event_id"`
	Data       RecommendedPropertyEventPayload `json:"payload"`
}

func (e LeadRecommendedPropertyEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e LeadRecommendedPropertyEvent) Name() string { return "lead.recommended_property_set" }

func (e LeadRecommendedPropertyEvent) Payload() interface{} { return e.Data }

type DealRecommendedPropertyEvent struct {
	EventIDVal string                         `json:"event_id"`
	Data       RecommendedPropertyEventPayload `json:"payload"`
}

func (e DealRecommendedPropertyEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e DealRecommendedPropertyEvent) Name() string { return "deal.recommended_property_set" }

func (e DealRecommendedPropertyEvent) Payload() interface{} { return e.Data }

// RecommendedPropertyTimestamp helper para publishers preencherem o campo.
func RecommendedPropertyTimestamp(now time.Time) string { return now.UTC().Format(time.RFC3339) }
