package services

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/oginoino/toca/pkg/domain/valueobjects"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
)

// NewMaintenanceCompletedEventHandler reconciles the charge that
// maintenance.requested created with the final cost reported when the
// ticket transitions to Concluído. Three buckets:
//
//	diff == 0                       → ack, nothing to reconcile.
//	diff  > 0 (actual > estimated)  → create an extra OTHER charge for the diff.
//	diff  < 0 (actual < estimated):
//	  - original is PENDING/OVERDUE → apply Discount on the original.
//	  - original is PAID             → register a credit Transaction so the
//	                                    admin's books reflect the refund.
//
// All side effects are idempotent: the diff charge is keyed by
// (ticket_id, "maintenance.completed.diff") and the discount/transaction
// branches are no-ops on retry because they read the current state first.
//
// O chamado também pode ter a política financeira definida **só na
// conclusão** (tela "Integração Financeira"): nesse caso o evento
// `maintenance.requested` saiu sem payer_role e nada foi criado. Por isso a
// conclusão cria a cobrança quando ela ainda não existe, com as mesmas regras
// do handler de requisição e com o custo final.
func NewMaintenanceCompletedEventHandler(
	chargeService *ChargeService,
	chargeRepo charge.RepositoryInterface,
	txService *TransactionService,
	billing *MaintenanceBillingService,
) events.EventHandler {
	return func(ctx context.Context, event events.Event) error {
		payload, err := decodeMaintenanceCompletedPayload(event)
		if err != nil {
			return err
		}
		if payload.TicketID == "" || payload.TenantID == "" {
			return fmt.Errorf("maintenance.completed payload missing ticket_id or tenant_id")
		}
		if payload.PayerRole == "" {
			// No charge was ever created for this ticket — nothing to reconcile.
			return nil
		}

		requestedID := events.DeriveNotificationID(payload.TicketID, "maintenance.requested")
		existing, err := chargeRepo.GetBySourceEventID(ctx, payload.TenantID, requestedID)
		if err != nil {
			return fmt.Errorf("lookup billing for ticket %s: %w", payload.TicketID, err)
		}
		if existing == nil {
			// Destinos que não criam cobrança própria no pedido: item somado
			// na próxima cobrança do inquilino, desconto no repasse do
			// proprietário, ou cobrança emitida só agora. O Reconcile ajusta
			// o que já existe para o custo final e cria quando não existe.
			amount := maintenanceBillingAmount(payload)
			if amount <= 0 {
				// Sem valor não há o que lançar; a tela exige o valor do serviço.
				return nil
			}
			if billing == nil {
				return fmt.Errorf("maintenance billing service is not configured")
			}
			_, err := billing.Reconcile(ctx, MaintenanceBillingInput{
				TicketID:        payload.TicketID,
				TenantID:        payload.TenantID,
				ContractID:      payload.ContractID,
				PropertyID:      payload.PropertyID,
				RequesterLeadID: payload.RequesterLeadID,
				Category:        payload.Category,
				PayerRole:       payload.PayerRole,
				Target:          payload.BillingTarget,
				AdvancedBy:      payload.AdvancedBy,
			}, amount, requestedID)
			return err
		}

		diff := payload.ActualCost - payload.EstimatedCost
		// Tolerância de R$ 0,01 para evitar gerar ajuste por arredondamento.
		if diff > -0.01 && diff < 0.01 {
			return nil
		}

		original := existing

		if diff > 0 {
			// Cobrança adicional pelo excesso de custo.
			return createDiffCharge(ctx, chargeService, payload, diff, original)
		}

		// diff < 0 — credit path. Need the original charge to decide.
		if original == nil {
			log.Printf("[INFO] maintenance.completed: no original charge for ticket=%s — skipping credit", payload.TicketID)
			return nil
		}
		switch original.Status {
		case charge.StatusPaid:
			return creditViaTransaction(ctx, txService, payload, -diff)
		case charge.StatusPending, charge.StatusOverdue, charge.StatusPartiallyPaid:
			return applyDiscount(ctx, chargeRepo, original, payload.TicketID, -diff)
		default:
			// CANCELLED ou estado desconhecido — apenas registra e ack.
			log.Printf("[INFO] maintenance.completed: original charge=%s in status=%s, skipping credit", original.ID, original.Status)
			return nil
		}
	}
}

func createDiffCharge(ctx context.Context, chargeService *ChargeService, p contracts.MaintenanceCompletedEventPayload, diff float64, original *charge.Charge) error {
	leadID := p.RequesterLeadID
	ownerID := p.RequesterLeadID
	if original != nil {
		// Mantém a linkagem da charge original quando disponível —
		// importante para o caso payer_role=owner onde owner_id
		// não bate com requester_lead_id.
		if original.TenantPartyID != "" {
			leadID = original.TenantPartyID
		}
		if original.OwnerID != "" {
			ownerID = original.OwnerID
		}
	}

	dealID := ""
	if original != nil {
		dealID = original.DealID
	}

	desc := fmt.Sprintf("Ajuste de manutenção: %s (#%s)", p.Category, p.TicketID)
	ref := time.Now().Format("2006-01")
	due := time.Now().AddDate(0, 0, 15)
	sourceEventID := events.DeriveNotificationID(p.TicketID, "maintenance.completed.diff")

	_, err := chargeService.CreateWithSourceEventID(
		ctx,
		sourceEventID,
		p.ContractID,
		dealID,
		leadID,
		leadID,
		ownerID,
		p.PropertyID,
		p.TenantID,
		charge.TypeOther,
		desc,
		ref,
		diff,
		due,
	)
	if err != nil {
		return fmt.Errorf("create diff charge for ticket %s: %w", p.TicketID, err)
	}
	return nil
}

func applyDiscount(ctx context.Context, chargeRepo charge.RepositoryInterface, c *charge.Charge, ticketID string, amount float64) error {
	if c == nil || amount <= 0 {
		return nil
	}
	discountEventID := events.DeriveNotificationID(ticketID, "maintenance.completed.discount")
	for _, seen := range c.AppliedEventIDs {
		if seen == discountEventID {
			return nil
		}
	}
	c.AppliedEventIDs = append(c.AppliedEventIDs, discountEventID)
	maxDiscount := c.Amount.Add(c.LateFee).Add(c.Interest).Float()
	nextDiscount := c.Discount.Float() + amount
	if nextDiscount > maxDiscount {
		nextDiscount = maxDiscount
	}
	c.Discount = valueobjects.NewMoneyFromFloat(nextDiscount)
	c.CalculateTotal()
	if err := chargeRepo.Update(ctx, c); err != nil {
		return fmt.Errorf("apply maintenance discount on charge %s: %w", c.ID, err)
	}
	log.Printf("[AUDIT] maintenance.completed: discounted charge=%s amount=%.2f", c.ID, amount)
	return nil
}

func creditViaTransaction(ctx context.Context, txService *TransactionService, p contracts.MaintenanceCompletedEventPayload, amount float64) error {
	if txService == nil || amount <= 0 {
		return nil
	}
	desc := fmt.Sprintf("Acerto manutenção (crédito): %s (#%s)", p.Category, p.TicketID)
	// Nota de crédito: devolve o que foi cobrado a mais, sem virar despesa nem
	// receita — o valor nunca pertenceu à imobiliária.
	tx := transaction.NewCreditNote(p.ContractID, desc, amount, time.Now(), p.TenantID)
	tx.Category = "Manutenção"
	// Mantém o crédito na competência do chamado, para aparecer junto do
	// efeito econômico correspondente.
	tx.Month = time.Now().Format("2006-01")
	sourceEventID := events.DeriveNotificationID(p.TicketID, "maintenance.completed.credit")
	if err := txService.CreateWithSourceEventID(ctx, sourceEventID, tx); err != nil {
		return fmt.Errorf("create credit transaction for ticket %s: %w", p.TicketID, err)
	}
	return nil
}

func decodeMaintenanceCompletedPayload(event events.Event) (contracts.MaintenanceCompletedEventPayload, error) {
	switch raw := event.Payload().(type) {
	case contracts.MaintenanceCompletedEventPayload:
		return raw, nil
	case contracts.MaintenanceCompletedEvent:
		return raw.Data, nil
	case map[string]interface{}:
		return contracts.MaintenanceCompletedEventPayload{
			TicketID:        getStr(raw, "ticket_id"),
			TenantID:        getStr(raw, "tenant_id"),
			ContractID:      getStr(raw, "contract_id"),
			RentalID:        getStr(raw, "rental_id"),
			PropertyID:      getStr(raw, "property_id"),
			RequesterLeadID: getStr(raw, "requester_lead_id"),
			Category:        getStr(raw, "category"),
			PayerRole:       getStr(raw, "payer_role"),
			BillingTarget:   getStr(raw, "billing_target"),
			AdvancedBy:      getStr(raw, "advanced_by"),
			EstimatedCost:   getFloat(raw, "estimated_cost"),
			ActualCost:      getFloat(raw, "actual_cost"),
			CompletedAt:     getStr(raw, "completed_at"),
			Timestamp:       getStr(raw, "timestamp"),
		}, nil
	default:
		return contracts.MaintenanceCompletedEventPayload{}, fmt.Errorf("unsupported payload type %T", raw)
	}
}
