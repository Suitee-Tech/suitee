package handlers

import (
	"context"
	"errors"
	"strings"

	"github.com/oginoino/toca/toca-financial/internal/domain/tenant_financial_settings"
)

// proofPolicySettings resolve a política de comprovante do tenant.
type proofPolicySettings interface {
	Get(ctx context.Context, tenantID string) (*tenant_financial_settings.Settings, error)
}

// ErrManualProofRequired é devolvido quando a política do tenant exige
// comprovante e a baixa manual chegou sem evidência. A mensagem é exibida ao
// operador, então nomeia a ação em vez de falar em "política".
var ErrManualProofRequired = errors.New(
	"esta confirmação manual exige comprovante: anexe o arquivo ou informe a URL antes de concluir",
)

// requireManualProof valida a evidência de uma baixa manual.
//
// A regra vive aqui, na borda HTTP, porque é aqui que a confirmação do
// operador entra: a interface não é barreira suficiente, já que qualquer
// cliente pode chamar o endpoint. Comissão, repasse, baixa de lançamento e
// ordem de pagamento compartilham esta exigência — todas confirmam dinheiro
// movido fora do sistema.
//
// Leitura tolerante a falha por decisão deliberada: não conseguir ler a
// configuração não pode liberar a baixa sem comprovante. Na dúvida, exige.
func requireManualProof(ctx context.Context, settings proofPolicySettings, tenantID, proofURL string) error {
	if strings.TrimSpace(proofURL) != "" {
		return nil
	}
	required := tenant_financial_settings.DefaultPayoutProofReqd
	if settings != nil {
		if tenantSettings, err := settings.Get(ctx, tenantID); err == nil && tenantSettings != nil {
			required = tenantSettings.PayoutProofRequired
		}
	}
	if !required {
		return nil
	}
	return ErrManualProofRequired
}
