package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// M11 — waive/adjust de valores sem limiar nem aprovador.
//
// A revisão de permissões de 2026-09-19 registrou que `PUT /charges/:id` aceitava
// alterar `amount`, `late_fee`, `interest`, `discount` e `line_items` — e
// `POST /commissions/:id/deductions` aceitava criar dedução de qualquer valor —
// com a MESMA permissão de escrita (`charges:write`/`finance:write`) que o
// operador usa no dia a dia. Um ator sozinho concedia desconto, perdoava multa
// ou abatia comissão sem segunda assinatura, enquanto o mesmo repositório já
// exige aprovação por valor nas transições de deal (`deals:approve`, achado
// A23).
//
// A correção segue o padrão do deal: acima do limiar, a operação exige uma
// permissão DEDICADA de aprovação — `finance:approve_adjustment` — verificada
// in-handler com `auth.HasPermission`, que falha fechado (sem provider, sem
// tenant, sem papel ⇒ nega). O limiar é
// `FINANCIAL_ADJUSTMENT_APPROVAL_THRESHOLD_BRL` (default 5.000; zero significa
// "toda alteração exige aprovação").
//
// Continuação registrada: mover o limiar para a política por tenant (o
// `approval_policy` do toca-auth hoje é o contrato v1 de deals: limiar único,
// `deals:approve` fixo). Enquanto isso, o limiar é de implantação — o mesmo
// desenho já usado por outros gates por env nesta base.

const (
	// AdjustmentApprovalPermission é a permissão dedicada de aprovação.
	AdjustmentApprovalPermission = "finance:approve_adjustment"

	// defaultAdjustmentApprovalThresholdBRL — R$ 5.000,00.
	defaultAdjustmentApprovalThresholdBRL = 5000.0

	adjustmentThresholdEnv = "FINANCIAL_ADJUSTMENT_APPROVAL_THRESHOLD_BRL"
)

// adjustmentApprovalThresholdBRL resolve o limiar a partir do ambiente.
//
// A implementação vive em `services` (fonte única): o mesmo limiar passou a
// valer para o desconto do acordo de inadimplência, que é decidido dentro do
// serviço — ter duas leituras da mesma variável era convite a divergirem.
func adjustmentApprovalThresholdBRL() float64 {
	return service.AdjustmentApprovalThresholdBRL()
}

// chargeAdjustmentMagnitude mede o tamanho do ajuste entre dois estados da
// cobrança, em reais.
//
// É o MAIOR entre:
//   - a variação do total (dinheiro que efetivamente mudou de valor);
//   - a soma dos módulos das variações de desconto, multa e juros — a concessão
//     dada pode ser compensada por outro campo e não mover o total, e é
//     justamente o waive que o achado aponta.
func chargeAdjustmentMagnitude(before, after charge.Charge) float64 {
	totalDelta := absFloat(after.TotalAmount.Float() - before.TotalAmount.Float())
	waiveDelta := absFloat(after.Discount.Float()-before.Discount.Float()) +
		absFloat(after.LateFee.Float()-before.LateFee.Float()) +
		absFloat(after.Interest.Float()-before.Interest.Float())
	if waiveDelta > totalDelta {
		return waiveDelta
	}
	return totalDelta
}

// adjustmentRequiresApproval decide se a magnitude exige segunda assinatura.
func adjustmentRequiresApproval(magnitude, threshold float64) bool {
	return magnitude > threshold
}

// actorCanApproveAdjustment decide se o chamador tem a permissão de aprovação.
//
// Falha FECHADO (mesma decisão do A23 em toca-deal): sem provider configurado a
// resposta é "não pode aprovar" — uma regressão de wiring desliga a segunda
// assinatura se o default fosse permissivo.
func actorCanApproveAdjustment(c *gin.Context, provider auth.PermissionProvider) bool {
	if provider == nil {
		return false
	}
	return auth.HasPermission(c, provider, AdjustmentApprovalPermission)
}

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}
