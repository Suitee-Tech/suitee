package services

import (
	"context"
	"fmt"
	"time"

	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
)

// NewInspectionDeductionsHandler subscribes the financial side to
// inspection.deductions_recorded events. Each deduction line becomes a
// Charge type=OTHER tagged as security-deposit acerto. The source_event_id
// is derived per-line so retries via the broker collapse via the unique
// index introduced in FASE 1.
func NewInspectionDeductionsHandler(chargeService *ChargeService) events.EventHandler {
	return func(ctx context.Context, event events.Event) error {
		payload, err := decodeInspectionDeductionsPayload(event)
		if err != nil {
			return err
		}
		if payload.ContractID == "" || payload.TenantID == "" || payload.InspectionID == "" {
			return fmt.Errorf("invalid inspection.deductions_recorded payload")
		}
		if len(payload.Deductions) == 0 {
			return nil
		}

		ref := time.Now().Format("2006-01")
		due := time.Now().AddDate(0, 0, 10)

		for idx, d := range payload.Deductions {
			if d.Amount <= 0 {
				continue
			}
			sourceEventID := events.DeriveNotificationID(payload.InspectionID, "deduction", fmt.Sprintf("%d", idx))
			desc := fmt.Sprintf("Dedução caução: %s", d.Description)
			if d.Room != "" {
				desc = fmt.Sprintf("Dedução caução (%s): %s", d.Room, d.Description)
			}
			if _, err := chargeService.CreateWithSourceEventID(
				ctx,
				sourceEventID,
				payload.ContractID,
				"",
				"",
				"",
				"",
				"",
				payload.TenantID,
				charge.TypeOther,
				desc,
				ref,
				d.Amount,
				due,
			); err != nil {
				return fmt.Errorf("create deduction charge for inspection %s line %d: %w", payload.InspectionID, idx, err)
			}
		}
		return nil
	}
}

func decodeInspectionDeductionsPayload(event events.Event) (contracts.InspectionDeductionsRecordedEventPayload, error) {
	switch raw := event.Payload().(type) {
	case contracts.InspectionDeductionsRecordedEventPayload:
		return raw, nil
	case contracts.InspectionDeductionsRecordedEvent:
		return raw.Data, nil
	case map[string]interface{}:
		out := contracts.InspectionDeductionsRecordedEventPayload{
			InspectionID: getString(raw, "inspection_id"),
			TenantID:     getString(raw, "tenant_id"),
			ContractID:   getString(raw, "contract_id"),
			Timestamp:    getString(raw, "timestamp"),
		}
		if v, ok := raw["total_amount"].(float64); ok {
			out.TotalAmount = v
		}
		if rawLines, ok := raw["deductions"].([]interface{}); ok {
			for _, line := range rawLines {
				m, ok := line.(map[string]interface{})
				if !ok {
					continue
				}
				ded := contracts.DeductionLine{
					Room:        getString(m, "room"),
					Description: getString(m, "description"),
				}
				if amt, ok := m["amount"].(float64); ok {
					ded.Amount = amt
				}
				out.Deductions = append(out.Deductions, ded)
			}
		}
		return out, nil
	default:
		return contracts.InspectionDeductionsRecordedEventPayload{}, fmt.Errorf("unsupported payload type %T", raw)
	}
}
