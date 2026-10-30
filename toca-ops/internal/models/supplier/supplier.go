// Package supplier registra o profissional/fornecedor da imobiliária —
// eletricista, encanador, vistoriador, chaveiro — com as características do
// serviço, contato e tipo de serviço. É a fonte de verdade para reutilizar o
// mesmo prestador em chamados e vistorias, em vez de digitar o nome de novo.
package supplier

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/validation"
)

// Kind distingue o prestador externo do profissional interno da imobiliária.
const (
	KindExternal = "externo"
	KindInternal = "interno"
)

// Supplier é o prestador cadastrado pela imobiliária.
type Supplier struct {
	ID       string `json:"id" bson:"_id"`
	TenantID string `json:"tenant_id" bson:"tenant_id"`
	// Name é como o prestador aparece na seleção.
	Name string `json:"name" bson:"name"`
	// ServiceType é o tipo de serviço (Elétrica, Hidráulica, Vistoria...).
	ServiceType string `json:"service_type" bson:"service_type"`
	// Kind: "externo" (terceirizado) ou "interno" (equipe da imobiliária).
	Kind string `json:"kind" bson:"kind"`
	// Document é o CPF/CNPJ, quando houver — usado para evitar duplicidade.
	Document string `json:"document,omitempty" bson:"document,omitempty"`
	Email    string `json:"email,omitempty" bson:"email,omitempty"`
	Phone    string `json:"phone,omitempty" bson:"phone,omitempty"`
	// Notes reúne as características: especialidades, marcas que atende,
	// região, condições comerciais combinadas.
	Notes string `json:"notes,omitempty" bson:"notes,omitempty"`
	City  string `json:"city,omitempty" bson:"city,omitempty"`
	// Active permite tirar o prestador da seleção sem apagar o histórico.
	Active    bool      `json:"active" bson:"active"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
}

// Normalize limpa espaços e aplica os defaults do cadastro.
func (v *Supplier) Normalize() {
	v.Name = strings.TrimSpace(v.Name)
	v.ServiceType = strings.TrimSpace(v.ServiceType)
	v.Document = strings.NewReplacer(".", "", "-", "", "/", "", " ", "").Replace(strings.TrimSpace(v.Document))
	v.Email = strings.ToLower(strings.TrimSpace(v.Email))
	v.Phone = strings.TrimSpace(v.Phone)
	if phoneCharacters.MatchString(v.Phone) {
		if canonical, err := validation.NormalizeBRPhone(v.Phone); err == nil {
			v.Phone = canonical
		}
	}
	v.Notes = strings.TrimSpace(v.Notes)
	v.City = strings.TrimSpace(v.City)
	v.Kind = strings.ToLower(strings.TrimSpace(v.Kind))
	if v.Kind == "" {
		v.Kind = KindExternal
	}
}

var phoneCharacters = regexp.MustCompile(`^[+0-9().\s-]+$`)
var documentCharacters = regexp.MustCompile(`^[0-9]+$`)
var supplierValidator = validation.New()

// Validate exige o mínimo para o prestador ser reutilizável: nome, tipo de
// serviço e ao menos um contato — sem contato o cadastro não serve para
// acionar ninguém.
func (v *Supplier) Validate() error {
	if v.Name == "" {
		return ErrNameRequired
	}
	if v.ServiceType == "" {
		return ErrServiceTypeRequired
	}
	if v.Phone == "" && v.Email == "" {
		return ErrContactRequired
	}
	if v.Phone != "" {
		if _, err := validation.NormalizeBRPhone(v.Phone); err != nil || !phoneCharacters.MatchString(v.Phone) {
			return ErrInvalidPhone
		}
	}
	if v.Email != "" && supplierValidator.Var(v.Email, "email") != nil {
		return ErrInvalidEmail
	}
	if v.Document != "" && (!documentCharacters.MatchString(v.Document) || (!validation.IsValidCPF(v.Document) && !validation.IsValidCNPJ(v.Document))) {
		return ErrInvalidDocument
	}
	if v.Kind != KindExternal && v.Kind != KindInternal {
		return ErrInvalidKind
	}
	return nil
}

// Display é o rótulo usado na seleção: "Carlos Silva (Elétrica)".
func (v *Supplier) Display() string {
	if v.ServiceType == "" {
		return v.Name
	}
	return v.Name + " (" + v.ServiceType + ")"
}

func New(name, serviceType, kind, document, email, phone, notes, city, tenantID string) *Supplier {
	now := time.Now()
	v := &Supplier{
		ID:          uuid.New().String(),
		TenantID:    tenantID,
		Name:        name,
		ServiceType: serviceType,
		Kind:        kind,
		Document:    document,
		Email:       email,
		Phone:       phone,
		Notes:       notes,
		City:        city,
		Active:      true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	v.Normalize()
	return v
}
