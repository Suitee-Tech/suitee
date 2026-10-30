package key

import (
	"time"

	"github.com/google/uuid"
)

const (
	StatusAvailable  = "Available"
	StatusCheckedOut = "Checked Out"
)

type Key struct {
	ID           string     `json:"id" bson:"_id"`
	TenantID     string     `json:"tenant_id" bson:"tenant_id"`
	PropertyID   string     `json:"property_id" bson:"property_id"`
	Code         string     `json:"code" bson:"code"`
	Location     string     `json:"location" bson:"location"`
	Status       string     `json:"status" bson:"status"`
	CheckedOutBy string     `json:"checked_out_by,omitempty" bson:"checked_out_by,omitempty"`
	CheckedOutAt *time.Time `json:"checked_out_at,omitempty" bson:"checked_out_at,omitempty"`
	ReturnedAt   *time.Time `json:"returned_at,omitempty" bson:"returned_at,omitempty"`
	Notes        string     `json:"notes,omitempty" bson:"notes,omitempty"`
	CreatedAt    time.Time  `json:"created_at" bson:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" bson:"updated_at"`
}

func New(propertyID, code, location, notes, tenantID string) *Key {
	now := time.Now()
	return &Key{
		ID:         uuid.New().String(),
		TenantID:   tenantID,
		PropertyID: propertyID,
		Code:       code,
		Location:   location,
		Status:     StatusAvailable,
		Notes:      notes,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}
