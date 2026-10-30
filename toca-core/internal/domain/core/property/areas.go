package property

type Areas struct {
	TotalAreaM2  float64  `json:"total_area_m2,omitempty"`
	UsableAreaM2 *float64 `json:"usable_area_m2,omitempty"`
	LandAreaM2   *float64 `json:"land_area_m2,omitempty"`
	BuiltAreaM2  *float64 `json:"built_area_m2,omitempty"`
}

func (a *Areas) Validate() error {
	return validate.Struct(a)
}
