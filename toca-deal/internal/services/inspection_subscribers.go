package services

import (
	"context"
	"fmt"

	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
)

// NewInspectionCompletedHandler subscribes toca-deal to inspection.completed
// events published by toca-ops. The handler delegates to
// ContractService.RecordInspectionCompleted which pins EntryInspectionID
// (for ENTRY) and audits (for EXIT). The actual TERMINATE flow stays
// driven by the operator — HasCompletedExitInspection already gates it.
func NewInspectionCompletedHandler(contractService *ContractService) events.EventHandler {
	return func(ctx context.Context, event events.Event) error {
		payload, err := decodeInspectionCompletedPayload(event)
		if err != nil {
			return err
		}
		if payload.ContractID == "" || payload.TenantID == "" || payload.InspectionID == "" {
			// Inspections without a linked contract are noise here — ack.
			return nil
		}
		return contractService.RecordInspectionCompleted(ctx, payload.TenantID, payload.ContractID, payload.Type, payload.InspectionID)
	}
}

func decodeInspectionCompletedPayload(event events.Event) (contracts.InspectionCompletedEventPayload, error) {
	switch raw := event.Payload().(type) {
	case contracts.InspectionCompletedEventPayload:
		return raw, nil
	case contracts.InspectionCompletedEvent:
		return raw.Data, nil
	case map[string]interface{}:
		out := contracts.InspectionCompletedEventPayload{
			InspectionID: dealGetStr(raw, "inspection_id"),
			TenantID:     dealGetStr(raw, "tenant_id"),
			PropertyID:   dealGetStr(raw, "property_id"),
			ContractID:   dealGetStr(raw, "contract_id"),
			Type:         dealGetStr(raw, "type"),
			CompletedAt:  dealGetStr(raw, "completed_at"),
			ReportURL:    dealGetStr(raw, "report_url"),
			Timestamp:    dealGetStr(raw, "timestamp"),
		}
		if fc, ok := raw["findings_count"].(float64); ok {
			out.FindingsCount = int(fc)
		}
		if hd, ok := raw["has_damage"].(bool); ok {
			out.HasDamage = hd
		}
		return out, nil
	default:
		return contracts.InspectionCompletedEventPayload{}, fmt.Errorf("unsupported payload type %T", raw)
	}
}

// dealGetStr is the local map-accessor — keeping it unique here avoids a
// clash with any future deal-wide helper of the same name.
func dealGetStr(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// dealGetFloat lê um número de payload decodificado como mapa (mensagens que
// passaram pelo broker chegam como map[string]interface{}).
func dealGetFloat(m map[string]interface{}, key string) float64 {
	switch v := m[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	default:
		return 0
	}
}
