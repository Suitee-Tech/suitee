package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/managed_bill"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
)

// TransferChargeHook implementa chargeTransferHook (definido em
// charge_service.go) e é chamado por ChargeService.MarkPaid sempre que
// uma charge transita para PAID. Cria 1 OwnerTransfer por charge,
// idempotente em (tenant, source_charge_id), reutilizando o
// TransferService.EnsureMonthlyForChargePaid. Isso mantém parcelas SALE
// distintas mesmo quando vencem no mesmo reference_month.
//
// Aplicado para charges RENT e SALE — operador paga, repasse mensal
// é gerado automaticamente, fechando o ciclo end-to-end sem precisar
// de webhook do gateway.
//
// AdminFeePct vem do tenant_financial_settings.AdminFeePct (default
// 10%). Sem tenantSettings wired, cai para 0 (sem desconto) — a UI
// mostra warning quando esse é o caso para que operador configure.
type TransferChargeHook struct {
	transferService *TransferService
	tenantSettings  *TenantFinancialSettingsService
	managedBills    *ManagedBillService
}

func NewTransferChargeHook(ts *TransferService, settings *TenantFinancialSettingsService) *TransferChargeHook {
	return &TransferChargeHook{transferService: ts, tenantSettings: settings}
}

func (h *TransferChargeHook) WithManagedBillService(service *ManagedBillService) *TransferChargeHook {
	h.managedBills = service
	return h
}

// EnsureMonthlyForChargePaid resolve adminFeePct via tenant settings,
// delegando a criação/idempotência para o TransferService. Erros são
// logados e retornados; no caminho produtivo, ChargeService.MarkPaid mantém
// charge, repasse e downstream dentro da mesma transação.
//
// Aceita RENT e SALE, mas somente depois de a charge estar integralmente
// PAID. Para SALE sem reference_month, o EnsureMonthlyForChargePaid rejeita.
func (h *TransferChargeHook) EnsureMonthlyForChargePaid(ctx context.Context, c *charge.Charge) error {
	if h.transferService == nil || c == nil {
		return nil
	}
	if c.Status != charge.StatusPaid || c.PaidAmount == nil || c.PaidAmount.Cents() != c.TotalAmount.Cents() {
		return nil
	}
	// Accept RENT and SALE — both generate monthly owner transfers.
	// Other types (MAINTENANCE, etc.) are silently skipped.
	if c.Type != charge.TypeRent && c.Type != charge.TypeSale {
		return nil
	}
	// A charge without an owner (e.g. a manual charge not linked to a property)
	// has no repasse to release. The payment itself still succeeds; the owner
	// obligation is simply skipped instead of failing the whole reconcile.
	if strings.TrimSpace(c.OwnerID) == "" {
		slog.Warn("transfer hook: skipping owner transfer for charge without owner", "charge_id", c.ID, "contract_id", c.ContractID)
		return nil
	}
	var adminFeePct float64
	if h.tenantSettings != nil {
		ts, err := h.tenantSettings.Get(ctx, c.TenantID)
		if err != nil {
			return err
		}
		if ts != nil {
			adminFeePct = ts.AdminFeePct
		}
	}
	ownerTransfer, _, err := h.transferService.EnsureMonthlyForChargePaid(ctx, c, adminFeePct)
	if err != nil {
		slog.Warn("transfer hook: failed to ensure monthly transfer", "charge_id", c.ID, "contract_id", c.ContractID, "reference_month", c.ReferenceMonth, "error", err)
		return err
	}
	if h.managedBills != nil && ownerTransfer != nil {
		inputs, err := managedBillInputsForPaidCharge(c, ownerTransfer)
		if err != nil {
			return err
		}
		for _, input := range inputs {
			if _, err := h.managedBills.EnsureReserved(ctx, input); err != nil {
				return fmt.Errorf("ensure managed bill for charge item %s: %w", input.SourceChargeItemID, err)
			}
		}
	}
	return nil
}

func managedBillInputsForPaidCharge(c *charge.Charge, ownerTransfer *transfer.OwnerTransfer) ([]managed_bill.NewReservedInput, error) {
	if c == nil || ownerTransfer == nil {
		return nil, fmt.Errorf("charge and owner transfer are required")
	}
	if c.TenantID != ownerTransfer.TenantID || c.ID != ownerTransfer.SourceChargeID || c.ContractID != ownerTransfer.ContractID ||
		c.PropertyID != ownerTransfer.PropertyID || c.OwnerID != ownerTransfer.OwnerID {
		return nil, ErrManagedBillIdentityMismatch
	}
	ownerPayableByItem := make(map[string]int64)
	for _, payment := range c.Payments {
		for _, allocation := range payment.Allocations {
			if allocation.AllocationType == charge.AllocationTypeOwnerPayable &&
				strings.TrimSpace(allocation.BeneficiaryPartyID) == strings.TrimSpace(c.OwnerID) {
				ownerPayableByItem[strings.TrimSpace(allocation.ChargeItemID)] += allocation.AmountCents
			}
		}
	}
	inputs := make([]managed_bill.NewReservedInput, 0)
	for _, item := range c.LineItems {
		if item.SettlementMode != charge.SettlementModeDirectLiquidation && item.SettlementMode != charge.SettlementModeOffset {
			continue
		}
		// Managed bills consume the owner's reserved pocket. Tenant-owned or
		// third-party items are intentionally outside this owner subledger.
		if strings.TrimSpace(item.EconomicOwnerID) != strings.TrimSpace(c.OwnerID) {
			continue
		}
		amountCents := ownerPayableByItem[strings.TrimSpace(item.ID)]
		if amountCents <= 0 {
			return nil, fmt.Errorf("owner-managed charge item %s has no owner-payable allocation", item.ID)
		}
		component := strings.TrimSpace(item.SourceID)
		if component == "" {
			component = strings.TrimSpace(item.Description)
		}
		creditor := strings.TrimSpace(item.Description)
		inputs = append(inputs, managed_bill.NewReservedInput{
			TenantID: c.TenantID, SourceTransferID: ownerTransfer.ID, SourceChargeID: c.ID,
			SourceChargeItemID: item.ID, ContractID: c.ContractID, PropertyID: c.PropertyID, OwnerID: c.OwnerID,
			SourceKey: events.DeriveNotificationID("managed_bill.reserve", c.TenantID, ownerTransfer.ID, c.ID, item.ID),
			Component: component, Creditor: creditor, Description: item.Description, AmountCents: amountCents,
			DueDate: c.DueDate, CompetenceMonth: c.ReferenceMonth, ReservedBy: "system:charge-paid",
		})
	}
	return inputs, nil
}
