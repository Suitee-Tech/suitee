package invitation

import (
	"time"

	"github.com/oginoino/toca/pkg/validation"
)

var validate = validation.New()

type Invitation struct {
	ID        string    `json:"id" bson:"_id,omitempty"`
	Email     string    `json:"email" bson:"email" validate:"required,email"`
	CompanyID string    `json:"company_id" bson:"company_id" validate:"required"`
	Role      string    `json:"role" bson:"role" validate:"required"` // staff, manager, admin
	GroupIDs  []string  `json:"group_ids" bson:"group_ids"`           // Groups to join upon accepting
	Status    string    `json:"status" bson:"status"`                 // pending, accepted, expired
	Token     string    `json:"token" bson:"token"`                   // unique token for invite link
	CreatedBy string    `json:"created_by" bson:"created_by"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
	ExpiresAt time.Time `json:"expires_at" bson:"expires_at"`
}

func (i *Invitation) Validate() error {
	return validate.Struct(i)
}

type CreateInvitationRequest struct {
	Email    string   `json:"email" validate:"required,email"`
	Role     string   `json:"role" validate:"omitempty"`
	GroupIDs []string `json:"group_ids"`
}

func (r *CreateInvitationRequest) Validate() error {
	return validate.Struct(r)
}

type AcceptInvitationRequest struct {
	Token string `json:"token" validate:"required"`
}

func (r *AcceptInvitationRequest) Validate() error {
	return validate.Struct(r)
}
