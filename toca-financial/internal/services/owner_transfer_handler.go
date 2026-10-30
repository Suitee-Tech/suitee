package services

import (
	"context"
)

// OwnerTransferHandler é o passo de repasse da pipeline contract.signed.
//
// Repasse não é mais provisionado na assinatura: nesse ponto ainda não há
// caixa recebido e a row poderia ser aprovada/processada antecipadamente.
// A criação acontece exclusivamente no TransferChargeHook após a charge
// correspondente ficar integralmente PAID.
type OwnerTransferHandler struct {
	transferService *TransferService
	tenantSettings  *TenantFinancialSettingsService
}

// NewOwnerTransferHandler builds the owner-transfer step.
func NewOwnerTransferHandler(transferService *TransferService, tenantSettings *TenantFinancialSettingsService) *OwnerTransferHandler {
	return &OwnerTransferHandler{transferService: transferService, tenantSettings: tenantSettings}
}

// Handle permanece na cadeia para compatibilidade estrutural, mas não
// materializa obrigação antes do recebimento.
func (h *OwnerTransferHandler) Handle(ctx context.Context, in ContractFinancialsInput) error {
	_ = ctx
	_ = in
	return nil
}
