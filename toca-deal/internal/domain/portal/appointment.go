package portal

import "time"

type Appointment struct {
	CanManage   bool      `json:"can_manage"`
	UpdatedAt   time.Time `json:"updated_at"`
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Date        string    `json:"date"`
	Time        string    `json:"time"`
	EndTime     string    `json:"end_time"`
	Type        string    `json:"type"`
	Status      string    `json:"status"`
	Description string    `json:"description,omitempty"`
	Property    string    `json:"property,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type CreateAppointmentRequest struct {
	PropertyID  string `json:"property_id,omitempty"`
	Title       string `json:"title"`
	Date        string `json:"date"`
	Time        string `json:"time"`
	EndTime     string `json:"end_time"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// Only schedule and notes may be changed by a client; identity comes from the deal.
type UpdateAppointmentRequest struct {
	ExpectedUpdatedAt time.Time `json:"expected_updated_at"`
	Date              string    `json:"date"`
	Time              string    `json:"time"`
	EndTime           string    `json:"end_time"`
	Description       *string   `json:"description,omitempty"`
}

type CancelAppointmentRequest struct {
	ExpectedUpdatedAt time.Time `json:"expected_updated_at"`
}
