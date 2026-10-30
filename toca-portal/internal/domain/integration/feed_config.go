package integration

// FeedPropertyFilter defines which properties to include in the XML feed
type FeedPropertyFilter struct {
	Statuses      []string `json:"statuses,omitempty" bson:"statuses,omitempty"`             // e.g., ["published", "available"]
	PropertyTypes []string `json:"property_types,omitempty" bson:"property_types,omitempty"` // e.g., ["apartment", "house"]
	ListingTypes  []string `json:"listing_types,omitempty" bson:"listing_types,omitempty"`   // e.g., ["sale", "rent"]
	MinPrice      *float64 `json:"min_price,omitempty" bson:"min_price,omitempty"`
	MaxPrice      *float64 `json:"max_price,omitempty" bson:"max_price,omitempty"`
}
