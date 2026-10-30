package account

import (
	"time"

	"github.com/oginoino/toca-auth/internal/domain/company"
	"github.com/oginoino/toca-auth/internal/domain/user"
	"github.com/oginoino/toca/pkg/validation"
)

var validate = validation.New()

type AccountType string

const (
	AccountTypeIndividual AccountType = "individual"
	AccountTypeCompany    AccountType = "company"
)

type Account struct {
	ID         string           `bson:"_id,omitempty"`
	Type       AccountType      `bson:"type" validate:"required,oneof=individual company"`
	Individual *user.User       `bson:"individual,omitempty" validate:"required_if=Type individual"`
	Company    *company.Company `bson:"company,omitempty" validate:"required_if=Type company"`
	CreatedAt  time.Time        `bson:"created_at"`
	UpdatedAt  time.Time        `bson:"updated_at"`
}

func (a *Account) Validate() error {
	return validate.Struct(a)
}
