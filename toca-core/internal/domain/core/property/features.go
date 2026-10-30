package property

type Features struct {
	Bedrooms      *int  `json:"bedrooms,omitempty"`
	Bathrooms     *int  `json:"bathrooms,omitempty"`
	Suites        *int  `json:"suites,omitempty"`
	ParkingSpaces *int  `json:"parking_spaces,omitempty"`
	Floors        *int  `json:"floors,omitempty"`
	FloorNumber   *int  `json:"floor_number,omitempty"`
	HasElevator   *bool `json:"has_elevator,omitempty"`
	Furnished     *bool `json:"furnished,omitempty"`
	PetsAllowed   *bool `json:"pets_allowed,omitempty"`
}

func (f *Features) Validate() error {
	return validate.Struct(f)
}
