package services

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/pkg/money"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
)

// NewContractTerminatedEventHandler wires the contract.terminated pipeline:
// rental → ENDED/RESCINDED, future charges → CANCELLED, optional early
// termination fee charge, optional security deposit return transaction.
//
// Best-effort orchestration: a per-step failure logs but doesn't abort the
// chain. The terminal status on the rental row is the source of truth for
// future operator inspection.
func NewContractTerminatedEventHandler(
	chargeService *ChargeService,
	chargeRepo rentalChargeConfigChargeRepo,
	transferService *TransferService,
	txService *TransactionService,
	rentalRepo rental.Repository,
) events.EventHandler {
	return func(ctx context.Context, event events.Event) error {
		var payload contracts.ContractTerminatedEventPayload
		switch raw := event.Payload().(type) {
		case contracts.ContractTerminatedEventPayload:
			payload = raw
		case *contracts.ContractTerminatedEventPayload:
			if raw == nil {
				return fmt.Errorf("invalid nil contract.terminated payload")
			}
			payload = *raw
		case map[string]interface{}:
			payload = contracts.ContractTerminatedEventPayload{
				TenantID:                  getString(raw, "tenant_id"),
				ContractID:                getString(raw, "contract_id"),
				DealID:                    getString(raw, "deal_id"),
				PropertyID:                getString(raw, "property_id"),
				OwnerID:                   getString(raw, "owner_id"),
				CustomerID:                getString(raw, "customer_id"),
				TerminationDate:           getString(raw, "termination_date"),
				Reason:                    getString(raw, "reason"),
				EarlyTerminationFee:       getFloat(raw, "early_termination_fee"),
				SecurityDeposit:           getFloat(raw, "security_deposit"),
				SecurityDepositDeductions: getFloat(raw, "security_deposit_deductions"),
				TriggeredBy:               getString(raw, "triggered_by"),
				Timestamp:                 getString(raw, "timestamp"),
			}
		default:
			return fmt.Errorf("invalid payload type: %T", event.Payload())
		}

		tenantID := payload.TenantID
		contractID := payload.ContractID
		propertyID := payload.PropertyID
		ownerID := payload.OwnerID
		customerID := payload.CustomerID
		reason := payload.Reason
		dealID := payload.DealID
		fee := payload.EarlyTerminationFee
		depositReturn := payload.SecurityDeposit
		depositDeductions := payload.SecurityDepositDeductions

		if tenantID == "" || contractID == "" {
			return fmt.Errorf("invalid contract.terminated payload: tenant_id and contract_id required")
		}
		terminationDate, err := parseReadjustmentTime(payload.TerminationDate)
		if err != nil {
			return fmt.Errorf("invalid contract.terminated termination_date: %w", err)
		}

		log.Printf("[AUDIT] contract.terminated tenant=%s contract=%s reason=%q fee=%.2f depositReturn=%.2f",
			tenantID, contractID, reason, fee, depositReturn)

		// 1. Mark the consolidated Rental aggregate as RESCINDED (early
		//    termination). MarkEnded is reserved for natural end-of-term.
		if rentalRepo != nil {
			r, err := rentalRepo.GetByContractID(ctx, tenantID, contractID)
			if err == nil && r != nil {
				r.Rescind()
				if err := rentalRepo.Update(ctx, r); err != nil {
					log.Printf("[WARN] contract.terminated: rental Update failed for contract=%s: %v", contractID, err)
				}
			}
		}

		// 2. Cancel only PENDING installments strictly after the effective
		//    termination date. OVERDUE and PARTIALLY_PAID rows are accrued debt
		//    and must survive the end of the recurring schedule.
		if err := cancelFutureCharges(ctx, chargeRepo, tenantID, contractID, terminationDate); err != nil {
			log.Printf("[WARN] contract.terminated: cancel future charges failed for contract=%s: %v", contractID, err)
		}

		// 3. Optional multa por rescisão antecipada — created as a one-off
		//    OTHER charge so it shows up in the tenant's pending balance and
		//    flows through the same billing / reminder pipeline as rent.
		if fee > 0 && chargeService != nil {
			due := time.Now().AddDate(0, 0, 10)
			ref := time.Now().Format("2006-01")
			desc := "Multa por rescisão antecipada"
			if reason != "" {
				desc = desc + " — " + reason
			}
			sourceEventID := events.DeriveNotificationID(event.EventID(), "contract.terminated", contractID, "early_termination_fee")
			if _, err := chargeService.CreateWithSourceEventID(ctx, sourceEventID, contractID, dealID, customerID, customerID, ownerID, propertyID, tenantID, charge.TypeOther, desc, ref, fee, due); err != nil {
				log.Printf("[WARN] contract.terminated: early termination fee charge failed: %v", err)
			}
		}

		// 4. Acerto de caução. depositReturn é o valor a devolver ao locatário
		//    após abater deductions identificadas na vistoria. Registramos como
		//    transação (despesa para a administradora; receita para o
		//    locatário). Valor zero significa "sem caução" e é ignorado.
		if (depositReturn > 0 || depositDeductions > 0) && txService != nil {
			net := depositReturn
			// Já recebemos depositReturn líquido — deductions é informativo
			// para fins de auditoria; vai no Description para não duplicar.
			parts := []string{"Acerto de caução"}
			if depositDeductions > 0 {
				parts = append(parts, fmt.Sprintf("descontos %s", money.BRL(depositDeductions)))
			}
			if net > 0 {
				parts = append(parts, fmt.Sprintf("devolução %s", money.BRL(net)))
			}
			desc := strings.Join(parts, " — ")
			sourceEventID := events.DeriveNotificationID(event.EventID(), "contract.terminated", contractID, "security_deposit_settlement")
			if err := createTerminationTransaction(ctx, txService, sourceEventID, contractID, tenantID, desc, math.Abs(net)); err != nil {
				log.Printf("[WARN] contract.terminated: deposit acerto failed: %v", err)
			}
		}

		return nil
	}
}

// NewContractReadjustedEventHandler stores an ordered effective-date schedule
// and recalculates only future PENDING charges covered by the adjustment.
func NewContractReadjustedEventHandler(rentalConfigService *RentalChargeConfigService) events.EventHandler {
	return func(ctx context.Context, event events.Event) error {
		var payload contracts.ContractReadjustedEventPayload
		switch raw := event.Payload().(type) {
		case contracts.ContractReadjustedEventPayload:
			payload = raw
		case *contracts.ContractReadjustedEventPayload:
			if raw == nil {
				return fmt.Errorf("invalid nil contract.readjusted payload")
			}
			payload = *raw
		case map[string]interface{}:
			payload = contracts.ContractReadjustedEventPayload{
				TenantID:      getString(raw, "tenant_id"),
				ContractID:    getString(raw, "contract_id"),
				DealID:        getString(raw, "deal_id"),
				PreviousValue: getFloat(raw, "previous_value"),
				NewValue:      getFloat(raw, "new_value"),
				EffectiveFrom: getString(raw, "effective_from"),
				Timestamp:     getString(raw, "timestamp"),
			}
		default:
			return fmt.Errorf("invalid payload type: %T", event.Payload())
		}
		if payload.TenantID == "" || payload.ContractID == "" || payload.NewValue <= 0 {
			return fmt.Errorf("invalid contract.readjusted payload")
		}
		effectiveFrom, err := parseReadjustmentTime(payload.EffectiveFrom)
		if err != nil {
			return fmt.Errorf("invalid contract.readjusted effective_from: %w", err)
		}
		occurredAt, err := parseReadjustmentTime(payload.Timestamp)
		if err != nil {
			return fmt.Errorf("invalid contract.readjusted timestamp: %w", err)
		}
		if rentalConfigService == nil {
			return nil
		}
		newCents := int64(math.Round(payload.NewValue * 100))
		cfg, updated, err := rentalConfigService.ApplyReadjustment(
			ctx, payload.TenantID, payload.ContractID, event.EventID(), newCents, effectiveFrom, occurredAt,
		)
		if err != nil {
			return err
		}
		if cfg == nil {
			log.Printf("contract.readjusted: no rental_charge_config for contract=%s tenant=%s — skipping", payload.ContractID, payload.TenantID)
			return nil
		}
		log.Printf("[AUDIT] contract.readjusted tenant=%s contract=%s effective_from=%s newCents=%d updatedCharges=%d",
			payload.TenantID, payload.ContractID, effectiveFrom.Format(time.RFC3339), newCents, updated)
		return nil
	}
}

func parseReadjustmentTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("value is required")
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported time format %q", raw)
}

// createTerminationTransaction registra um lançamento de caução como Transaction
// (categoria "Caução") para o acerto contábil. value é o módulo do valor a
// devolver — o tipo é "despesa" do ponto de vista da administradora.
func createTerminationTransaction(ctx context.Context, txService *TransactionService, sourceEventID, contractID, tenantID, description string, value float64) error {
	if value <= 0 {
		return nil
	}
	tx := transaction.New(contractID, transaction.TypeDespesa, description, value, time.Now(), tenantID)
	tx.Category = "Caução"
	return txService.CreateWithSourceEventID(ctx, sourceEventID, tx)
}

// cancelFutureCharges cancels only unaccrued PENDING installments after the
// effective termination date. OVERDUE/PARTIALLY_PAID rows represent debt that
// already accrued and therefore remain collectible after contract termination.
// Iteration mirrors RentalChargeConfigService.listAllCharges.
func cancelFutureCharges(ctx context.Context, chargeRepo rentalChargeConfigChargeRepo, tenantID, contractID string, terminationDate time.Time) error {
	if chargeRepo == nil {
		return nil
	}
	if terminationDate.IsZero() {
		return fmt.Errorf("termination date is required to cancel future charges")
	}
	var offset int64
	limit := int64(200)
	cancelled := 0
	for {
		items, total, err := chargeRepo.List(ctx, tenantID, contractID, "", "", limit, offset)
		if err != nil {
			return err
		}
		for _, c := range items {
			if c == nil {
				continue
			}
			if c.Status != charge.StatusPending || !dateAfter(c.DueDate, terminationDate) {
				continue
			}
			expectedVersion := c.UpdatedAt
			if err := c.Cancel(); err != nil {
				continue
			}
			updated, err := updateChargeIfCurrent(ctx, chargeRepo, c, expectedVersion)
			if err != nil {
				log.Printf("[WARN] cancel charge %s failed: %v", c.ID, err)
				continue
			}
			if !updated {
				log.Printf("[WARN] cancel charge %s skipped after concurrent modification", c.ID)
				continue
			}
			cancelled++
		}
		offset += int64(len(items))
		if offset >= total || len(items) == 0 {
			break
		}
	}
	if cancelled > 0 {
		log.Printf("[AUDIT] cancelled %d future charges for contract=%s", cancelled, contractID)
	}
	return nil
}

// dateAfter compares civil dates rather than instants. Charges due on the
// effective termination date have already accrued for that day and are kept,
// regardless of timezone or clock time in the event payload.
func dateAfter(candidate, boundary time.Time) bool {
	cy, cm, cd := candidate.Date()
	by, bm, bd := boundary.In(candidate.Location()).Date()
	if cy != by {
		return cy > by
	}
	if cm != bm {
		return cm > bm
	}
	return cd > bd
}

// updateChargeIfCurrent uses the optimistic repository capability available
// in PostgreSQL to prevent stale background jobs from overwriting a payment,
// reversal or cancellation committed after their read. Lightweight test fakes
// and alternate repositories retain the legacy Update fallback.
func updateChargeIfCurrent(ctx context.Context, repo rentalChargeConfigChargeRepo, c *charge.Charge, expectedVersion time.Time) (bool, error) {
	if versioned, ok := repo.(chargeVersionedRepository); ok {
		return versioned.UpdateIfVersion(ctx, c, expectedVersion)
	}
	if err := repo.Update(ctx, c); err != nil {
		return false, err
	}
	return true, nil
}
