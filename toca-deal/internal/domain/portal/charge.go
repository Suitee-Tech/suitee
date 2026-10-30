package portal

import "time"

// Charge é a cobrança como o cliente do portal a vê.
//
// O cliente recebia só o total e não entendia por que o valor subiu depois do
// vencimento (G32 do plano de inadimplência). Por isso a resposta discrimina os
// encargos: `amount` é o valor original do período e `total_amount` já inclui
// `late_fee` (multa) + `interest` (juros) − `discount` (desconto) — os três vêm
// prontos do domínio `charge.Charge` do toca-financial pela rota interna.
// `days_overdue` é derivado no mapper do portal porque a origem não o
// serializa (lá ele existe apenas como método `DaysOverdue()`).
type Charge struct {
	ID             string     `json:"id"`
	Description    string     `json:"description"`
	DueDate        time.Time  `json:"due_date"`
	Amount         float64    `json:"amount"`
	LateFee        float64    `json:"late_fee"`
	Interest       float64    `json:"interest"`
	Discount       float64    `json:"discount"`
	TotalAmount    float64    `json:"total_amount"`
	DaysOverdue    int        `json:"days_overdue"`
	Status         string     `json:"status"`
	PaymentURL     string     `json:"payment_url,omitempty"`
	PaidAt         *time.Time `json:"paid_at,omitempty"`
	ReferenceMonth string     `json:"reference_month,omitempty"`
}
