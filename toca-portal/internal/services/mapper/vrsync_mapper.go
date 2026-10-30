package mapper

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"
)

// VRSync XML structures following the Grupo ZAP/VivaReal specification

type ListingDataFeed struct {
	XMLName  xml.Name `xml:"ListingDataFeed"`
	Xmlns    string   `xml:"xmlns,attr"`
	XmlnsXsi string   `xml:"xmlns:xsi,attr"`
	XsiLoc   string   `xml:"xsi:schemaLocation,attr"`
	Header   Header   `xml:"Header"`
	Listings Listings `xml:"Listings"`
}

type Header struct {
	Provider    string `xml:"Provider"`
	Email       string `xml:"Email"`
	ContactName string `xml:"ContactName"`
	PublishDate string `xml:"PublishDate"`
	Telephone   string `xml:"Telephone"`
}

type Listings struct {
	Listing []Listing `xml:"Listing"`
}

type Listing struct {
	ListingID       string      `xml:"ListingID"`
	Title           string      `xml:"Title"`
	TransactionType string      `xml:"TransactionType"`
	Location        Location    `xml:"Location"`
	Details         Details     `xml:"Details"`
	Media           Media       `xml:"Media"`
	ContactInfo     ContactInfo `xml:"ContactInfo"`
	PublicationType string      `xml:"PublicationType,omitempty"`
	VirtualTourLink string      `xml:"VirtualTourLink,omitempty"`
}

type Location struct {
	DisplayAddress string  `xml:"displayAddress,attr"`
	Country        Country `xml:"Country"`
	State          string  `xml:"State"`
	City           string  `xml:"City"`
	Zone           string  `xml:"Zone,omitempty"`
	Neighborhood   string  `xml:"Neighborhood"`
	Address        string  `xml:"Address"`
	StreetNumber   string  `xml:"StreetNumber"`
	Complement     string  `xml:"Complement,omitempty"`
	PostalCode     string  `xml:"PostalCode"`
	Latitude       string  `xml:"Latitude,omitempty"`
	Longitude      string  `xml:"Longitude,omitempty"`
}

type Country struct {
	Abbreviation string `xml:"abbreviation,attr"`
}

type Details struct {
	PropertyType              string    `xml:"PropertyType"`
	Description               string    `xml:"Description"`
	ListPrice                 *Price    `xml:"ListPrice,omitempty"`
	RentalPrice               *Rental   `xml:"RentalPrice,omitempty"`
	PropertyAdministrationFee *Price    `xml:"PropertyAdministrationFee,omitempty"`
	Iptu                      *Price    `xml:"Iptu,omitempty"`
	LivingArea                *Area     `xml:"LivingArea,omitempty"`
	LotArea                   *Area     `xml:"LotArea,omitempty"`
	Bedrooms                  int       `xml:"Bedrooms,omitempty"`
	Bathrooms                 int       `xml:"Bathrooms,omitempty"`
	Suites                    int       `xml:"Suites,omitempty"`
	Garage                    int       `xml:"Garage,omitempty"`
	Floors                    int       `xml:"Floors,omitempty"`
	UnitFloor                 int       `xml:"UnitFloor,omitempty"`
	Features                  *Features `xml:"Features,omitempty"`
}

type Price struct {
	Currency string `xml:"currency,attr"`
	Value    string `xml:",chardata"`
}

type Rental struct {
	Currency string `xml:"currency,attr"`
	Period   string `xml:"period,attr"`
	Value    string `xml:",chardata"`
}

type Area struct {
	Unit  string `xml:"unit,attr"`
	Value string `xml:",chardata"`
}

type Features struct {
	Feature []string `xml:"Feature"`
}

type Media struct {
	Item []MediaItem `xml:"Item"`
}

type MediaItem struct {
	Medium  string `xml:"medium,attr"`
	Caption string `xml:"caption,attr,omitempty"`
	Primary string `xml:"primary,attr,omitempty"`
	URL     string `xml:",chardata"`
}

type ContactInfo struct {
	Name      string `xml:"Name"`
	Email     string `xml:"Email"`
	Telephone string `xml:"Telephone,omitempty"`
}

// PropertyData represents the property data fetched from toca-core
// This is a simplified representation of the toca-core Property model
type PropertyData struct {
	ID           string       `json:"id"`
	Title        string       `json:"title"`
	Description  string       `json:"description"`
	PropertyType string       `json:"property_type"`
	ListingType  string       `json:"listing_type"`
	Condition    string       `json:"condition"`
	Address      AddressData  `json:"address"`
	Pricing      PricingData  `json:"pricing"`
	Areas        AreasData    `json:"areas"`
	Features     FeaturesData `json:"features"`
	Amenities    []string     `json:"amenities"`
	Media        MediaData    `json:"media"`
	AvailableFor []string     `json:"available_for"`
}

type AddressData struct {
	Street       string `json:"street"`
	Number       string `json:"number"`
	Complement   string `json:"complement"`
	Neighborhood string `json:"neighborhood"`
	City         string `json:"city"`
	State        string `json:"state"`
	StateCode    string `json:"stateCode"`
	PostalCode   string `json:"postalCode"`
	Country      string `json:"country"`
	CountryCode  string `json:"countryCode"`
	Latitude     string `json:"latitude"`
	Longitude    string `json:"longitude"`
}

type PricingData struct {
	SalePrice      *MoneyData `json:"sale_price"`
	RentPrice      *MoneyData `json:"rent_price"`
	CondominiumFee *MoneyData `json:"condominium_fee"`
	PropertyTax    *MoneyData `json:"property_tax"`
}

type MoneyData struct {
	CurrentAmount float64 `json:"current_amount"`
	Currency      string  `json:"currency"`
}

type AreasData struct {
	TotalAreaM2 float64  `json:"total_area_m2"`
	LandAreaM2  *float64 `json:"land_area_m2"`
}

type FeaturesData struct {
	Bedrooms      *int `json:"bedrooms"`
	Bathrooms     *int `json:"bathrooms"`
	Suites        *int `json:"suites"`
	ParkingSpaces *int `json:"parking_spaces"`
	Floors        *int `json:"floors"`
	FloorNumber   *int `json:"floor_number"`
}

type MediaData struct {
	Images       []ImageData       `json:"images"`
	Videos       []VideoData       `json:"videos"`
	VirtualTours []VirtualTourData `json:"virtual_tours"`
}

type ImageData struct {
	URL         string `json:"url"`
	Description string `json:"description"`
	Order       int    `json:"order"`
}

type VideoData struct {
	URL      string `json:"url"`
	Platform string `json:"platform"`
}

type VirtualTourData struct {
	URL string `json:"url"`
}

// GenerateVRSyncFeed generates the full VRSync XML feed from a list of properties
func GenerateVRSyncFeed(properties []PropertyData, contactName, contactEmail, contactPhone, publicationType string) (string, int, int, error) {
	feed := ListingDataFeed{
		Xmlns:    "http://www.vivareal.com/schemas/1.0/VRSync",
		XmlnsXsi: "http://www.w3.org/2001/XMLSchema-instance",
		XsiLoc:   "http://www.vivareal.com/schemas/1.0/VRSync http://xml.vivareal.com/vrsync.xsd",
		Header: Header{
			Provider:    "TOCA Imoveis",
			Email:       contactEmail,
			ContactName: contactName,
			PublishDate: time.Now().Format("2006-01-02T15:04:05"),
			Telephone:   contactPhone,
		},
	}

	successCount := 0
	failCount := 0

	for _, prop := range properties {
		listing, err := mapPropertyToListing(prop, contactName, contactEmail, contactPhone, publicationType)
		if err != nil {
			failCount++
			continue
		}
		feed.Listings.Listing = append(feed.Listings.Listing, *listing)
		successCount++
	}

	xmlHeader := `<?xml version="1.0" encoding="UTF-8"?>` + "\n"
	output, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		return "", 0, 0, fmt.Errorf("failed to marshal XML: %w", err)
	}

	return xmlHeader + string(output), successCount, failCount, nil
}

func mapPropertyToListing(prop PropertyData, contactName, contactEmail, contactPhone, publicationType string) (*Listing, error) {
	// Validate minimum required fields
	if prop.ID == "" || prop.Title == "" {
		return nil, fmt.Errorf("missing required fields for property %s", prop.ID)
	}

	title := prop.Title
	if len(title) > 100 {
		title = title[:100]
	}
	if len(title) < 10 {
		title = title + " - Imovel"
	}

	listing := &Listing{
		ListingID:       prop.ID,
		Title:           title,
		TransactionType: mapTransactionType(prop),
		Location:        mapLocation(prop.Address),
		Details:         mapDetails(prop),
		Media:           mapMedia(prop.Media),
		ContactInfo: ContactInfo{
			Name:      contactName,
			Email:     contactEmail,
			Telephone: contactPhone,
		},
		PublicationType: publicationType,
	}

	// Add virtual tour link if available
	if len(prop.Media.VirtualTours) > 0 {
		listing.VirtualTourLink = prop.Media.VirtualTours[0].URL
	}

	return listing, nil
}

func mapTransactionType(prop PropertyData) string {
	// Check if property is available for both sale and rent
	hasSale := false
	hasRent := false

	for _, t := range prop.AvailableFor {
		if t == "sale" {
			hasSale = true
		}
		if t == "rent" {
			hasRent = true
		}
	}

	if hasSale && hasRent {
		return "Sale/Rent"
	}

	// Fallback to listing_type
	switch prop.ListingType {
	case "sale":
		return "For Sale"
	case "rent":
		return "For Rent"
	default:
		return "For Sale"
	}
}

func mapLocation(addr AddressData) Location {
	countryCode := addr.CountryCode
	if countryCode == "" {
		countryCode = "BR"
	}

	return Location{
		DisplayAddress: "Neighborhood",
		Country:        Country{Abbreviation: countryCode},
		State:          addr.StateCode,
		City:           addr.City,
		Neighborhood:   addr.Neighborhood,
		Address:        addr.Street,
		StreetNumber:   addr.Number,
		Complement:     addr.Complement,
		PostalCode:     addr.PostalCode,
		Latitude:       addr.Latitude,
		Longitude:      addr.Longitude,
	}
}

func mapDetails(prop PropertyData) Details {
	details := Details{
		PropertyType: MapPropertyType(prop.PropertyType),
	}

	// Description (CDATA, 50-3000 chars)
	desc := prop.Description
	if len(desc) > 3000 {
		desc = desc[:3000]
	}
	if len(desc) < 50 && desc != "" {
		desc = desc + strings.Repeat(" ", 50-len(desc))
	}
	details.Description = desc

	// Pricing
	if prop.Pricing.SalePrice != nil && prop.Pricing.SalePrice.CurrentAmount > 0 {
		details.ListPrice = &Price{
			Currency: "BRL",
			Value:    fmt.Sprintf("%.0f", prop.Pricing.SalePrice.CurrentAmount),
		}
	}

	if prop.Pricing.RentPrice != nil && prop.Pricing.RentPrice.CurrentAmount > 0 {
		details.RentalPrice = &Rental{
			Currency: "BRL",
			Period:   "Monthly",
			Value:    fmt.Sprintf("%.0f", prop.Pricing.RentPrice.CurrentAmount),
		}
	}

	if prop.Pricing.CondominiumFee != nil && prop.Pricing.CondominiumFee.CurrentAmount > 0 {
		details.PropertyAdministrationFee = &Price{
			Currency: "BRL",
			Value:    fmt.Sprintf("%.0f", prop.Pricing.CondominiumFee.CurrentAmount),
		}
	}

	if prop.Pricing.PropertyTax != nil && prop.Pricing.PropertyTax.CurrentAmount > 0 {
		details.Iptu = &Price{
			Currency: "BRL",
			Value:    fmt.Sprintf("%.0f", prop.Pricing.PropertyTax.CurrentAmount),
		}
	}

	// Areas
	if prop.Areas.TotalAreaM2 > 0 {
		details.LivingArea = &Area{
			Unit:  "square metres",
			Value: fmt.Sprintf("%.0f", prop.Areas.TotalAreaM2),
		}
	}

	if prop.Areas.LandAreaM2 != nil && *prop.Areas.LandAreaM2 > 0 {
		details.LotArea = &Area{
			Unit:  "square metres",
			Value: fmt.Sprintf("%.0f", *prop.Areas.LandAreaM2),
		}
	}

	// Features
	if prop.Features.Bedrooms != nil {
		details.Bedrooms = *prop.Features.Bedrooms
	}
	if prop.Features.Bathrooms != nil {
		details.Bathrooms = *prop.Features.Bathrooms
	}
	if prop.Features.Suites != nil {
		details.Suites = *prop.Features.Suites
	}
	if prop.Features.ParkingSpaces != nil {
		details.Garage = *prop.Features.ParkingSpaces
	}
	if prop.Features.Floors != nil {
		details.Floors = *prop.Features.Floors
	}
	if prop.Features.FloorNumber != nil {
		details.UnitFloor = *prop.Features.FloorNumber
	}

	// Amenities → VRSync Features
	if len(prop.Amenities) > 0 {
		mappedFeatures := MapAmenities(prop.Amenities)
		if len(mappedFeatures) > 0 {
			details.Features = &Features{Feature: mappedFeatures}
		}
	}

	return details
}

func mapMedia(media MediaData) Media {
	m := Media{}

	for i, img := range media.Images {
		// Validate image URL
		if img.URL == "" || len(img.URL) > 255 {
			continue
		}
		// Check extension (JPG/JPEG)
		lowerURL := strings.ToLower(img.URL)
		if !strings.HasSuffix(lowerURL, ".jpg") && !strings.HasSuffix(lowerURL, ".jpeg") &&
			!strings.Contains(lowerURL, ".jpg?") && !strings.Contains(lowerURL, ".jpeg?") &&
			!strings.Contains(lowerURL, "format=jpg") && !strings.Contains(lowerURL, "format=jpeg") {
			// Allow URLs without clear extension (CDN URLs)
			// Only skip if it's clearly a non-JPEG
			if strings.HasSuffix(lowerURL, ".png") || strings.HasSuffix(lowerURL, ".gif") ||
				strings.HasSuffix(lowerURL, ".webp") || strings.HasSuffix(lowerURL, ".svg") {
				continue
			}
		}

		item := MediaItem{
			Medium:  "image",
			Caption: img.Description,
			URL:     img.URL,
		}
		if i == 0 {
			item.Primary = "true"
		}
		m.Item = append(m.Item, item)
	}

	// Add first YouTube video if available
	for _, vid := range media.Videos {
		if strings.Contains(vid.URL, "youtube") || strings.Contains(vid.URL, "youtu.be") {
			m.Item = append(m.Item, MediaItem{
				Medium: "video",
				URL:    vid.URL,
			})
			break // Only 1 video allowed
		}
	}

	return m
}
