package services

import (
	"context"

	"github.com/oginoino/toca/toca-financial/internal/domain/tenant_financial_settings"
)

// proofPolicySettings resolve a política de comprovante do tenant.
//
// Mesma forma de manualReceiptSettings: nil degrada para o default do domínio,
// preservando compatibilidade com serviços montados sem settings (testes e
// caminhos legados).
type proofPolicySettings interface {
	Get(ctx context.Context, tenantID string) (*tenant_financial_settings.Settings, error)
}

// resolveManualProofRequired resolve se a baixa manual exige comprovante.
//
// A decisão é consumida pela entidade de ordem de pagamento, que antes exigia
// comprovante incondicionalmente e por isso ignorava o setting do tenant.
//
// Leitura deliberadamente tolerante a falha: não conseguir ler a configuração
// não pode liberar a baixa sem comprovante. Na dúvida, exige.
func resolveManualProofRequired(ctx context.Context, settings proofPolicySettings, tenantID string) bool {
	if settings == nil {
		return tenant_financial_settings.DefaultPayoutProofReqd
	}
	tenantSettings, err := settings.Get(ctx, tenantID)
	if err != nil || tenantSettings == nil {
		return tenant_financial_settings.DefaultPayoutProofReqd
	}
	return tenantSettings.PayoutProofRequired
}
