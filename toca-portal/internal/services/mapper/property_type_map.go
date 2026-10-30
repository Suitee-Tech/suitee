package mapper

// MapPropertyType maps TOCA property types to VRSync PropertyType values
func MapPropertyType(tocaType string) string {
	typeMap := map[string]string{
		"apartment":  "Residential / Apartment",
		"house":      "Residential / Home",
		"land":       "Residential / Land/Lot",
		"commercial": "Commercial / Building",
	}

	if mapped, ok := typeMap[tocaType]; ok {
		return mapped
	}
	return "Residential / Home"
}
