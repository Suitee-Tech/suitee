package services

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-ops/internal/models/inspection"
)

func NewContractSignedInspectionHandler(inspectionService *InspectionService) events.EventHandler {
	return func(ctx context.Context, event events.Event) error {
		payload, err := decodeContractSignedPayload(event)
		if err != nil {
			return err
		}
		if payload.ContractID == "" || payload.TenantID == "" || payload.PropertyID == "" {
			return fmt.Errorf("invalid contract.signed payload: missing contract/tenant/property")
		}
		if payload.Type != "" && payload.Type != "locacao" {
			return nil
		}

		scheduled, err := parseScheduledDate(payload.StartDate)
		if err != nil {
			log.Printf("contract.signed inspection: unparseable start_date=%q for contract=%s, defaulting to tomorrow", payload.StartDate, payload.ContractID)
			scheduled = time.Now().Add(24 * time.Hour)
		}

		id := events.DeriveNotificationID(payload.ContractID, "inspection.entry")
		insp := &inspection.Inspection{
			TenantID:      payload.TenantID,
			PropertyID:    payload.PropertyID,
			ContractID:    payload.ContractID,
			Type:          inspection.TypeEntry,
			ScheduledDate: scheduled,
			Status:        inspection.StatusAgendada,
			Findings:      []inspection.Finding{},
		}
		if err := inspectionService.CreateScheduled(ctx, id, insp); err != nil {
			return fmt.Errorf("create ENTRY inspection for contract %s: %w", payload.ContractID, err)
		}
		log.Printf("[AUDIT] contract.signed: scheduled ENTRY inspection id=%s for contract=%s on %s", id, payload.ContractID, scheduled.Format("2006-01-02"))
		return nil
	}
}

func decodeContractSignedPayload(event events.Event) (contracts.ContractSignedEventPayload, error) {
	switch raw := event.Payload().(type) {
	case contracts.ContractSignedEventPayload:
		return raw, nil
	case contracts.ContractSignedEvent:
		return raw.Data, nil
	case map[string]interface{}:
		out := contracts.ContractSignedEventPayload{
			DealID:     getStr(raw, "deal_id"),
			ContractID: getStr(raw, "contract_id"),
			TenantID:   getStr(raw, "tenant_id"),
			SignedAt:   getStr(raw, "signed_at"),
			StartDate:  getStr(raw, "start_date"),
			EndDate:    getStr(raw, "end_date"),
			Type:       getStr(raw, "type"),
			CustomerID: getStr(raw, "customer_id"),
			OwnerID:    getStr(raw, "owner_id"),
			PropertyID: getStr(raw, "property_id"),
		}
		return out, nil
	default:
		return contracts.ContractSignedEventPayload{}, fmt.Errorf("unsupported payload type %T", raw)
	}
}

func getStr(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func parseScheduledDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("unparseable date %q", s)
}
