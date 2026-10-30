package mapper

import "strings"

// amenityMap maps common TOCA amenity strings to VRSync Feature values
var amenityMap = map[string]string{
	// Pool
	"piscina":          "Pool",
	"pool":             "Pool",
	"piscina_aquecida": "Heated Pool",
	// Gym
	"academia": "Gym",
	"gym":      "Gym",
	// Sauna
	"sauna": "Sauna",
	// Barbecue
	"churrasqueira": "BBQ",
	"barbecue":      "BBQ",
	// Playground
	"playground": "Playground",
	// Party Room
	"salao_de_festas": "Party Room",
	"party_room":      "Party Room",
	"salao_festas":    "Party Room",
	// Elevator
	"elevador": "Elevator",
	"elevator": "Elevator",
	// Security
	"portaria_24h": "24-hour Concierge",
	"seguranca":    "Security System",
	"security":     "Security System",
	// Garage
	"garagem": "Garage",
	"garage":  "Garage",
	// Garden
	"jardim": "Garden",
	"garden": "Garden",
	// Balcony
	"varanda": "Balcony",
	"balcony": "Balcony",
	// Air conditioning
	"ar_condicionado":  "Air Conditioning",
	"air_conditioning": "Air Conditioning",
	// Laundry
	"lavanderia": "Laundry",
	"laundry":    "Laundry",
	// Sports court
	"quadra_esportiva": "Sports Court",
	"sports_court":     "Sports Court",
	// Gourmet area
	"espaco_gourmet": "Gourmet Area",
	"gourmet":        "Gourmet Area",
	// Furnished
	"mobiliado": "Furnished",
	"furnished": "Furnished",
	// Pet friendly
	"pet_friendly":   "Pets Allowed",
	"aceita_animais": "Pets Allowed",
	// Doorman
	"porteiro": "Doorman",
	"doorman":  "Doorman",
	// Intercom
	"interfone": "Intercom",
	"intercom":  "Intercom",
	// Cable TV
	"tv_cabo":  "Cable TV",
	"cable_tv": "Cable TV",
	// Internet
	"internet": "Internet Access",
}

// MapAmenities converts TOCA amenity strings to VRSync Feature values
func MapAmenities(amenities []string) []string {
	seen := make(map[string]bool)
	var features []string

	for _, amenity := range amenities {
		normalized := strings.ToLower(strings.TrimSpace(amenity))
		normalized = strings.ReplaceAll(normalized, " ", "_")

		if mapped, ok := amenityMap[normalized]; ok {
			if !seen[mapped] {
				seen[mapped] = true
				features = append(features, mapped)
			}
		}
	}

	return features
}
