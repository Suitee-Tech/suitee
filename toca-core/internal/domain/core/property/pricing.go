package property

type Pricing struct {
	SalePrice      *Money `json:"sale_price,omitempty"`
	RentPrice      *Money `json:"rent_price,omitempty"`
	CondominiumFee *Money `json:"condominium_fee,omitempty"`
	PropertyTax    *Money `json:"property_tax,omitempty"`
}

func (p *Pricing) Validate() error {
	return validate.Struct(p)
}

type Money struct {
	OriginalAmount float64 `json:"original_amount" validate:"required,number"`
	CurrentAmount  float64 `json:"current_amount" validate:"required,number"`
	Currency       string  `json:"currency" validate:"required"`
}

func (m *Money) Validate() error {
	return validate.Struct(m)
}
