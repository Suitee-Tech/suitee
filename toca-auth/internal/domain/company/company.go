package company

import (
	"github.com/oginoino/toca/pkg/validation"
	"time"
)

var validate = validation.New()

type Address struct {
	Street     string `bson:"street" json:"street" validate:"required"`
	Number     string `bson:"number" json:"number" validate:"required"`
	Complement string `bson:"complement" json:"complement"`
	District   string `bson:"district" json:"district" validate:"required"`
	City       string `bson:"city" json:"city" validate:"required"`
	State      string `bson:"state" json:"state" validate:"required"`
	ZipCode    string `bson:"zip_code" json:"zip_code" validate:"required"`
}

type Company struct {
	ID        string          `bson:"_id,omitempty"`
	Name      string          `bson:"name" validate:"required"`
	LegalName string          `bson:"legal_name" validate:"required"`
	TaxID     string          `bson:"tax_id" validate:"required,cnpj_valid"`
	Email     string          `bson:"email" validate:"required,email"`
	Phone     string          `bson:"phone" validate:"required"`
	Address   Address         `bson:"address" validate:"required"`
	Type      string          `bson:"type"` // franchisor, franchisee
	IsActive  bool            `bson:"is_active"`
	OwnerID   string          `bson:"owner_id"`
	Members   []CompanyMember `bson:"members"`
}

type CompanyMember struct {
	AccountID      string `bson:"account_id" json:"account_id"`
	Profile        string `bson:"profile" json:"profile"`                 // e.g. admin, manager, member, viewer
	FunctionalRole string `bson:"functional_role" json:"functional_role"` // novo: 9 valores funcionais

	// P2/M7 — desativação de vínculo. O ponteiro preserva a compatibilidade:
	// vínculos gravados antes desta mudança não têm o campo e continuam ATIVOS
	// (nil != false). A suspensão vale para a empresa; o acesso deixa de ser
	// possível imediatamente porque a marca de revogação do A15 é gravada junto
	// e o middleware a consulta por requisição.
	IsActive      *bool      `bson:"is_active,omitempty" json:"is_active,omitempty"`
	DeactivatedAt *time.Time `bson:"deactivated_at,omitempty" json:"deactivated_at,omitempty"`
}

// Active informa se o vínculo está ativo (campo ausente = ativo).
func (m CompanyMember) Active() bool {
	return m.IsActive == nil || *m.IsActive
}

func (c *Company) Validate() error {
	return validate.Struct(c)
}
