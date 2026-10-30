package services

import (
	"context"
	"fmt"

	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
)

// NewCollectionEscalatedHandler keeps the contract aggregate clean while
// still surfacing inadimplência transitions to operators via the audit log.
// Per the FASE 4 decision (see plan), we do NOT mutate Contract.Status when
// a collection escalates — the live status is owned by toca-financial and
// the contract UI queries it via the security-deposit-balance + collection
// endpoints on demand.
func NewCollectionEscalatedHandler() events.EventHandler {
	return func(ctx context.Context, event events.Event) error {
		payload, err := decodeCollectionEscalatedPayload(event)
		if err != nil {
			return err
		}
		if payload.ContractID == "" || payload.TenantID == "" {
			return nil
		}
		audit.Record(ctx, audit.Entry{
			Action:     "collection.escalated",
			EntityType: "Contract",
			EntityID:   payload.ContractID,
			After: map[string]any{
				"collection_id":   payload.CollectionID,
				"previous_status": payload.PreviousStatus,
				"new_status":      payload.NewStatus,
				"total_debt":      payload.TotalDebt,
				"escalated_by":    payload.EscalatedBy,
			},
			Metadata: map[string]any{"path": "CollectionEscalatedHandler"},
		})
		return nil
	}
}

func decodeCollectionEscalatedPayload(event events.Event) (contracts.CollectionEscalatedEventPayload, error) {
	switch raw := event.Payload().(type) {
	case contracts.CollectionEscalatedEventPayload:
		return raw, nil
	case contracts.CollectionEscalatedEvent:
		return raw.Data, nil
	case map[string]interface{}:
		out := contracts.CollectionEscalatedEventPayload{
			CollectionID:   dealGetStr(raw, "collection_id"),
			ContractID:     dealGetStr(raw, "contract_id"),
			TenantID:       dealGetStr(raw, "tenant_id"),
			DebtorID:       dealGetStr(raw, "debtor_id"),
			PreviousStatus: dealGetStr(raw, "previous_status"),
			NewStatus:      dealGetStr(raw, "new_status"),
			EscalatedBy:    dealGetStr(raw, "escalated_by"),
			Timestamp:      dealGetStr(raw, "timestamp"),
		}
		if v, ok := raw["total_debt"].(float64); ok {
			out.TotalDebt = v
		}
		return out, nil
	default:
		return contracts.CollectionEscalatedEventPayload{}, fmt.Errorf("unsupported payload type %T", raw)
	}
}

// NewCollectionCreatedHandler registra no contrato que a inadimplência virou
// caso formal. É o par do escalated: o created marca o INÍCIO (dívida agrupada),
// o escalated marca cada mudança de estado do caso. Sem ele, o evento
// `collection.created` era publicado pelo financeiro sem nenhum consumidor.
//
// Como o escalated, NÃO muda Contract.Status: o caso é do financeiro e a UI do
// contrato consulta o estado on demand.
func NewCollectionCreatedHandler() events.EventHandler {
	return func(ctx context.Context, event events.Event) error {
		payload, err := decodeCollectionCreatedPayload(event)
		if err != nil {
			return err
		}
		if payload.ContractID == "" || payload.TenantID == "" {
			return nil
		}
		audit.Record(ctx, audit.Entry{
			Action:     "collection.created",
			EntityType: "Contract",
			EntityID:   payload.ContractID,
			After: map[string]any{
				"collection_id": payload.CollectionID,
				"total_debt":    payload.TotalDebt,
				"days_overdue":  payload.DaysOverdue,
				"charge_count":  payload.ChargeCount,
			},
			Metadata: map[string]any{"path": "CollectionCreatedHandler"},
		})
		return nil
	}
}

func decodeCollectionCreatedPayload(event events.Event) (contracts.CollectionCreatedEventPayload, error) {
	switch raw := event.Payload().(type) {
	case contracts.CollectionCreatedEventPayload:
		return raw, nil
	case contracts.CollectionCreatedEvent:
		return raw.Data, nil
	case map[string]interface{}:
		out := contracts.CollectionCreatedEventPayload{
			CollectionID: dealGetStr(raw, "collection_id"),
			ContractID:   dealGetStr(raw, "contract_id"),
			TenantID:     dealGetStr(raw, "tenant_id"),
			DebtorID:     dealGetStr(raw, "debtor_id"),
			Timestamp:    dealGetStr(raw, "timestamp"),
		}
		if v, ok := raw["total_debt"].(float64); ok {
			out.TotalDebt = v
		}
		if v, ok := raw["days_overdue"].(float64); ok {
			out.DaysOverdue = int(v)
		}
		if v, ok := raw["charge_count"].(float64); ok {
			out.ChargeCount = int(v)
		}
		return out, nil
	default:
		return contracts.CollectionCreatedEventPayload{}, fmt.Errorf("unsupported payload type %T", raw)
	}
}

// NewCollectionAgreementHandler registra na trilha do contrato o ciclo do acordo
// de inadimplência (criado, quebrado, cancelado, cumprido).
//
// Como os demais handlers desta família, NÃO altera Contract.Status: o acordo é
// do financeiro; o contrato guarda o rastro para quem consulta o histórico.
func NewCollectionAgreementHandler(acao string) events.EventHandler {
	return func(ctx context.Context, event events.Event) error {
		payload, err := decodeCollectionAgreementPayload(event)
		if err != nil {
			return err
		}
		if payload.ContractID == "" || payload.TenantID == "" {
			return nil
		}
		audit.Record(ctx, audit.Entry{
			Action:     acao,
			EntityType: "Contract",
			EntityID:   payload.ContractID,
			After: map[string]any{
				"agreement_id":   payload.AgreementID,
				"collection_id":  payload.CollectionID,
				"total_amount":   payload.TotalAmount,
				"discount":       payload.Discount,
				"installments":   payload.Installments,
				"original_total": payload.OriginalTotal,
				"status":         payload.Status,
				"reason":         payload.Reason,
			},
			Metadata: map[string]any{"path": "CollectionAgreementHandler"},
		})
		return nil
	}
}

// NewCollectionSettledHandler registra a baixa por garantia e o desfecho
// judicial na trilha do contrato — é onde o valor recuperado e as custas ficam
// visíveis para quem olha o contrato depois.
func NewCollectionSettledHandler(acao string) events.EventHandler {
	return func(ctx context.Context, event events.Event) error {
		payload, err := decodeCollectionSettlementPayload(event)
		if err != nil {
			return err
		}
		if payload.ContractID == "" || payload.TenantID == "" {
			return nil
		}
		audit.Record(ctx, audit.Entry{
			Action:     acao,
			EntityType: "Contract",
			EntityID:   payload.ContractID,
			After: map[string]any{
				"collection_id":       payload.CollectionID,
				"guarantee_type":      payload.GuaranteeType,
				"settled_amount":      payload.SettledAmount,
				"applied_amount":      payload.AppliedAmount,
				"credit_amount":       payload.CreditAmount,
				"judicial_status":     payload.JudicialStatus,
				"resolved_collection": payload.ResolvedCollection,
				"reference":           payload.Reference,
			},
			Metadata: map[string]any{"path": "CollectionSettledHandler"},
		})
		return nil
	}
}

func decodeCollectionAgreementPayload(event events.Event) (contracts.CollectionAgreementEventPayload, error) {
	switch raw := event.Payload().(type) {
	case contracts.CollectionAgreementEventPayload:
		return raw, nil
	case contracts.CollectionAgreementEvent:
		return raw.Data, nil
	case map[string]interface{}:
		out := contracts.CollectionAgreementEventPayload{
			AgreementID:  dealGetStr(raw, "agreement_id"),
			CollectionID: dealGetStr(raw, "collection_id"),
			ContractID:   dealGetStr(raw, "contract_id"),
			TenantID:     dealGetStr(raw, "tenant_id"),
			Status:       dealGetStr(raw, "status"),
			Reason:       dealGetStr(raw, "reason"),
		}
		out.TotalAmount = dealGetFloat(raw, "total_amount")
		out.Discount = dealGetFloat(raw, "discount")
		out.OriginalTotal = dealGetFloat(raw, "original_total")
		if v, ok := raw["installments"].(float64); ok {
			out.Installments = int(v)
		}
		return out, nil
	default:
		return contracts.CollectionAgreementEventPayload{}, fmt.Errorf("unsupported payload type %T", raw)
	}
}

func decodeCollectionSettlementPayload(event events.Event) (contracts.CollectionSettlementEventPayload, error) {
	switch raw := event.Payload().(type) {
	case contracts.CollectionSettlementEventPayload:
		return raw, nil
	case contracts.CollectionSettledEvent:
		return raw.Data, nil
	case contracts.CollectionJudicialClosedEvent:
		return raw.Data, nil
	case map[string]interface{}:
		out := contracts.CollectionSettlementEventPayload{
			CollectionID:   dealGetStr(raw, "collection_id"),
			ContractID:     dealGetStr(raw, "contract_id"),
			TenantID:       dealGetStr(raw, "tenant_id"),
			GuaranteeType:  dealGetStr(raw, "guarantee_type"),
			JudicialStatus: dealGetStr(raw, "judicial_status"),
			Reference:      dealGetStr(raw, "reference"),
		}
		out.SettledAmount = dealGetFloat(raw, "settled_amount")
		out.AppliedAmount = dealGetFloat(raw, "applied_amount")
		out.CreditAmount = dealGetFloat(raw, "credit_amount")
		if v, ok := raw["resolved_collection"].(bool); ok {
			out.ResolvedCollection = v
		}
		return out, nil
	default:
		return contracts.CollectionSettlementEventPayload{}, fmt.Errorf("unsupported payload type %T", raw)
	}
}
