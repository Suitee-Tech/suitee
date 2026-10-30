package portal

import "github.com/oginoino/toca/pkg/propertymatch"

// PropertyMedia é uma imagem do imóvel exposta ao portal: URL pública/assinada
// e a legenda cadastrada (ex.: "Fachada", "Sala de estar"), que ajuda o cliente
// a navegar a galeria.
type PropertyMedia struct {
	URL         string `json:"url" bson:"url"`
	Description string `json:"description,omitempty" bson:"description,omitempty"`
}

type PropertySummary struct {
	ID            string  `json:"id" bson:"id"`
	Title         string  `json:"title" bson:"title"`
	Address       string  `json:"address" bson:"address"`
	Neighborhood  string  `json:"neighborhood" bson:"neighborhood"`
	City          string  `json:"city" bson:"city"`
	State         string  `json:"state" bson:"state"`
	ImageURL      string  `json:"image_url,omitempty" bson:"image_url,omitempty"`
	Price         float64 `json:"price" bson:"price"`
	Bedrooms      int     `json:"bedrooms,omitempty" bson:"bedrooms,omitempty"`
	Bathrooms     int     `json:"bathrooms,omitempty" bson:"bathrooms,omitempty"`
	ParkingSpaces int     `json:"parking_spaces,omitempty" bson:"parking_spaces,omitempty"`
	TotalAreaM2   float64 `json:"total_area_m2,omitempty" bson:"total_area_m2,omitempty"`
	ListingType   string  `json:"listing_type,omitempty" bson:"listing_type,omitempty"`

	// Detalhes que qualificam a decisão do cliente. Nada aqui é identificação
	// exata: rua, número e CEP saem mascarados das leituras de vitrine — ver
	// MaskPropertyPrivacy.
	Suites       int             `json:"suites,omitempty" bson:"suites,omitempty"`
	UsableAreaM2 float64         `json:"usable_area_m2,omitempty" bson:"usable_area_m2,omitempty"`
	Amenities    []string        `json:"amenities,omitempty" bson:"amenities,omitempty"`
	Description  string          `json:"description,omitempty" bson:"description,omitempty"`
	Images       []PropertyMedia `json:"images,omitempty" bson:"images,omitempty"`

	// AvailableFor is used internally by recommendation scoring. The API
	// already communicates the operation through listing_type/deal context.
	AvailableFor []string                 `json:"-" bson:"-"`
	MatchScore   *float64                 `json:"match_score,omitempty" bson:"match_score,omitempty"`
	MatchReasons *propertymatch.Breakdown `json:"match_reasons,omitempty" bson:"match_reasons,omitempty"`
}

// MaskPropertyPrivacy remove a identificação exata do imóvel (rua e número) das
// leituras de VITRINE — busca e recomendações. O cliente navega por bairro,
// cidade e UF; o endereço completo é liberado quando ele agenda a visita, e aí
// sai pelo imóvel do próprio negócio (onde ele já é parte).
//
// A máscara fica no backend (e não só na tela) para a lista de sugestões não
// carregar o endereço até o navegador: privacidade como valor pretendido, não
// como detalhe de apresentação.
func MaskPropertyPrivacy(p *PropertySummary) *PropertySummary {
	if p == nil {
		return nil
	}
	p.Address = ""
	return p
}
