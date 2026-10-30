package contracts

import "time"

// Full snapshot: retries and out-of-order delivery never recreate an older slot.
// InspectorID is an ops supplier, ScheduledBy is an authenticated account.
type InspectionCalendarPayload struct {
	InspectionID  string    `json:"inspection_id"`
	TenantID      string    `json:"tenant_id"`
	ContractID    string    `json:"contract_id"`
	PropertyID    string    `json:"property_id"`
	Type          string    `json:"type"`
	ScheduledDate time.Time `json:"scheduled_date"`
	TimeZone      string    `json:"time_zone"`
	ScheduledBy   string    `json:"scheduled_by"`
	InspectorID   string    `json:"inspector_id"`
	InspectorName string    `json:"inspector_name"`
	Status        string    `json:"status"`
	UpdatedAt     time.Time `json:"updated_at"`
}
type InspectionCalendarEvent struct {
	ID   string                    `json:"event_id"`
	Data InspectionCalendarPayload `json:"payload"`
}

func (e InspectionCalendarEvent) EventID() string      { return e.ID }
func (e InspectionCalendarEvent) Name() string         { return "inspection.calendar_changed" }
func (e InspectionCalendarEvent) Payload() interface{} { return e.Data }
