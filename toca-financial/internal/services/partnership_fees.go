package services

import (
	"errors"
	"fmt"
	"github.com/oginoino/toca/pkg/domain/valueobjects"
	access "github.com/oginoino/toca/pkg/partnershipaccess"
	"github.com/oginoino/toca/toca-financial/internal/repositories"
	"math"
)

type PartnershipFeePreview struct {
	Direction   string `json:"direction"`
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	AmountCents int64  `json:"amount_cents"`
	PayerTenant string `json:"payer_tenant"`
	PayeeTenant string `json:"payee_tenant"`
}

var ErrPartnershipFeesReview = errors.New("revise as taxas e o responsável pelo pagamento no acordo aceito da parceria")

func partnershipFees(p *repositories.PartnershipFinancials, source, closing string) ([]PartnershipFeePreview, error) {
	if p == nil {
		return nil, nil
	}
	if err := access.ValidateFees(p.AdminFee, p.TransactionFee, p.FeePayer); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPartnershipFeesReview, err)
	}
	payer, payee := closing, source
	if p.FeePayer == access.FeeSourcePaysClosing {
		payer, payee = source, closing
	}
	fees := []PartnershipFeePreview{}
	for _, v := range []struct {
		kind, label string
		amount      float64
	}{{"admin_fee", "Taxa administrativa da parceria", p.AdminFee}, {"transaction_fee", "Taxa por negócio da parceria", p.TransactionFee}} {
		if math.IsNaN(v.amount) || math.IsInf(v.amount, 0) {
			return nil, fmt.Errorf("taxa de parceria inválida")
		}
		if v.amount <= 0 {
			continue
		}
		if payer == "" || payee == "" || payer == payee {
			return nil, fmt.Errorf("empresas das taxas de parceria inválidas")
		}
		direction := "payable"
		if payee == closing {
			direction = "receivable"
		}
		fees = append(fees, PartnershipFeePreview{Direction: direction, Kind: v.kind, Label: v.label, AmountCents: valueobjects.NewMoneyFromFloat(v.amount).Cents(), PayerTenant: payer, PayeeTenant: payee})
	}
	return fees, nil
}
