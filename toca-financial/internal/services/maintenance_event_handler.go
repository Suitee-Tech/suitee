package services

import (
	"context"
	"fmt"

	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
)

// NewMaintenanceRequestedEventHandler reacts to maintenance.requested events
// from toca-ops by attaching the cost to the next document the payer already
// expects:
//
//	payer_role="tenant"  → soma na próxima cobrança em aberto do contrato
//	                       (cobrança avulsa com composição quando não há nenhuma)
//	payer_role="owner"   → desconta no próximo repasse pendente do proprietário
//	payer_role="shared"  → mesmo destino de "owner"
//	payer_role="agency"  → responsabilidade da imobiliária: nada a lançar
//	payer_role=""        → sem política: apenas confirma o evento
//
// Skipping the billing when `payer_role` is empty keeps existing customers
// (who haven't configured policies yet) running without surprise billing.
// Idempotency: the ticket id keys the line item and the transfer deduction,
// and the fallback charge carries the derived source_event_id.
func NewMaintenanceRequestedEventHandler(billing *MaintenanceBillingService) events.EventHandler {
	return func(ctx context.Context, event events.Event) error {
		payload, err := decodeMaintenancePayload(event)
		if err != nil {
			return err
		}

		if payload.PayerRole == "" || payload.EstimatedCost <= 0 {
			// No policy → we simply ack. The notification handler in
			// toca-comm still fires (it doesn't need PayerRole).
			return nil
		}
		if billing == nil {
			return fmt.Errorf("maintenance billing service is not configured")
		}

		_, err = billing.Apply(ctx, MaintenanceBillingInput{
			TicketID:        payload.TicketID,
			TenantID:        payload.TenantID,
			ContractID:      payload.ContractID,
			PropertyID:      payload.PropertyID,
			RequesterLeadID: payload.RequesterLeadID,
			Category:        payload.Category,
			PayerRole:       payload.PayerRole,
			Target:          payload.BillingTarget,
			AdvancedBy:      payload.AdvancedBy,
		}, payload.EstimatedCost, maintenanceRequestedSourceEventID(payload.TicketID))
		return err
	}
}

// decodeMaintenancePayload extracts MaintenanceRequestedEventPayload from a
// raw events.Event without depending on JSON re-marshalling. Falls back to
// a permissive map decode so older publishers (that haven't migrated to
// the typed contract) still flow through.
func decodeMaintenancePayload(event events.Event) (contracts.MaintenanceRequestedEventPayload, error) {
	switch raw := event.Payload().(type) {
	case contracts.MaintenanceRequestedEventPayload:
		return raw, nil
	case contracts.MaintenanceRequestedEvent:
		return raw.Data, nil
	case map[string]interface{}:
		return contracts.MaintenanceRequestedEventPayload{
			TicketID:        getStr(raw, "ticket_id"),
			TenantID:        getStr(raw, "tenant_id"),
			PropertyID:      getStr(raw, "property_id"),
			ContractID:      getStr(raw, "contract_id"),
			RentalID:        getStr(raw, "rental_id"),
			RequesterLeadID: getStr(raw, "requester_lead_id"),
			Category:        getStr(raw, "category"),
			Priority:        getStr(raw, "priority"),
			Description:     getStr(raw, "description"),
			EstimatedCost:   getFloat(raw, "estimated_cost"),
			PayerRole:       getStr(raw, "payer_role"),
			BillingTarget:   getStr(raw, "billing_target"),
			AdvancedBy:      getStr(raw, "advanced_by"),
			Timestamp:       getStr(raw, "timestamp"),
		}, nil
	default:
		return contracts.MaintenanceRequestedEventPayload{}, fmt.Errorf("unsupported payload type %T", raw)
	}
}

func getStr(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getFloat(m map[string]interface{}, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0
}
