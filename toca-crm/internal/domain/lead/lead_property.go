package lead

type PropertyType string

const (
	PropertyTypeApartment  PropertyType = "APARTMENT"
	PropertyTypeHouse      PropertyType = "HOUSE"
	PropertyTypeLand       PropertyType = "LAND"
	PropertyTypeCommercial PropertyType = "COMMERCIAL"
	PropertyTypeOther      PropertyType = "OTHER"
)

type PriceRange struct {
	Min float64 `bson:"min" json:"min" validate:"min=0"`
	Max float64 `bson:"max" json:"max" validate:"min=0,gtecsfield=Min"`
}

type PropertyFeatures struct {
	Bedrooms  *int     `bson:"bedrooms,omitempty" json:"bedrooms,omitempty" validate:"omitempty,min=0"`
	Bathrooms *int     `bson:"bathrooms,omitempty" json:"bathrooms,omitempty" validate:"omitempty,min=0"`
	Parking   *int     `bson:"parking,omitempty" json:"parking,omitempty" validate:"omitempty,min=0"`
	AreaMin   *float64 `bson:"area_min,omitempty" json:"area_min,omitempty" validate:"omitempty,min=0"`
	AreaMax   *float64 `bson:"area_max,omitempty" json:"area_max,omitempty" validate:"omitempty,min=0,gtecsfield=AreaMin"`
}

type LeadProperty struct {
	Type              PropertyType     `bson:"type,omitempty" json:"type,omitempty" validate:"omitempty"`
	PreferredLocation string           `bson:"preferred_location,omitempty" json:"preferred_location,omitempty" validate:"omitempty"`
	PriceRange        PriceRange       `bson:"price_range,omitempty" json:"price_range,omitempty"`
	Features          PropertyFeatures `bson:"features,omitempty" json:"features,omitempty"`
	Notes             string           `bson:"notes,omitempty" json:"notes,omitempty"`
}
