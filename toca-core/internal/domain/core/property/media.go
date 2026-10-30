package property

type Media struct {
	Images       []Image       `json:"images,omitempty"`
	Videos       []Video       `json:"videos,omitempty"`
	VirtualTours []VirtualTour `json:"virtual_tours,omitempty"`
}

type Image struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
	Order       int    `json:"order,omitempty"`
}

type Video struct {
	URL      string `json:"url"`
	Platform string `json:"platform,omitempty"`
}

type VirtualTour struct {
	URL string `json:"url"`
}
