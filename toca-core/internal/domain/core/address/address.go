package address

import (
	"encoding/json"

	"github.com/oginoino/toca/pkg/validation"
)

var validate = validation.New()

type Address struct {
	Street           string `json:"street" validate:"required"`            // nome da rua
	Number           string `json:"number" validate:"required"`            // número
	Complement       string `json:"complement"`                            // complemento (apto, bloco, sala)
	Neighborhood     string `json:"neighborhood" validate:"required"`      // bairro
	District         string `json:"district"`                              // distrito ou região administrativa
	City             string `json:"city" validate:"required"`              // cidade
	State            string `json:"state" validate:"required"`             // estado
	StateCode        string `json:"stateCode" validate:"required,len=2"`   // sigla do estado (SP, RJ)
	PostalCode       string `json:"postalCode" validate:"required"`        // CEP
	Country          string `json:"country" validate:"required"`           // país
	CountryCode      string `json:"countryCode" validate:"required,len=2"` // código ISO do país
	IsVerified       bool   `json:"is_verified"`                           // se o endereço foi verificado
	Longitude        string `json:"longitude"`                             // longitude
	Latitude         string `json:"latitude"`                              // latitude
	FormattedAddress string `json:"formatted_address"`                     // endereço formatado
	ReferencePoint   string `json:"reference_point"`                       // ponto de referência
}

func (a *Address) UnmarshalJSON(data []byte) error {
	type Alias Address
	aux := struct {
		Alias
		PostalCodeSnake  string `json:"postal_code"`
		StateCodeSnake   string `json:"state_code"`
		CountryCodeSnake string `json:"country_code"`
	}{}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	*a = Address(aux.Alias)

	if a.PostalCode == "" && aux.PostalCodeSnake != "" {
		a.PostalCode = aux.PostalCodeSnake
	}
	if a.StateCode == "" && aux.StateCodeSnake != "" {
		a.StateCode = aux.StateCodeSnake
	}
	if a.CountryCode == "" && aux.CountryCodeSnake != "" {
		a.CountryCode = aux.CountryCodeSnake
	}

	return nil
}

// Validate validates the address struct
func (a *Address) Validate() error {
	return validate.Struct(a)
}
