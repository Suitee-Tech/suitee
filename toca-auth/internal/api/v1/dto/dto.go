package dto

import (
	"time"

	"github.com/oginoino/toca-auth/internal/domain/account"
	"github.com/oginoino/toca-auth/internal/domain/company"
	"github.com/oginoino/toca-auth/internal/domain/user"
	"github.com/oginoino/toca/pkg/validation"
)

type UpdateCompanyRequest struct {
	Name      string `json:"name" validate:"required"`
	LegalName string `json:"legal_name" validate:"required"`
	TaxID     string `json:"tax_id" validate:"required,cnpj_valid"`
	Email     string `json:"email" validate:"required,email"`
	Phone     string `json:"phone" validate:"required"`
	Address   struct {
		Street     string `json:"street" validate:"required"`
		Number     string `json:"number" validate:"required"`
		Complement string `json:"complement"`
		District   string `json:"district" validate:"required"`
		City       string `json:"city" validate:"required"`
		State      string `json:"state" validate:"required"`
		ZipCode    string `json:"zip_code" validate:"required"`
	} `json:"address" validate:"required"`
	Type string `json:"type"`
}

type OTPRequest struct {
	Contact string `json:"contact" validate:"required"`
	Method  string `json:"method" validate:"required,oneof=email whatsapp"`
}

func (r *OTPRequest) Validate() error {
	return validate.Struct(r)
}

type OTPVerify struct {
	Contact string `json:"contact" validate:"required"`
	Code    string `json:"code" validate:"required,numeric,min=6,max=8"`
}

func (r *OTPVerify) Validate() error {
	return validate.Struct(r)
}

type EnsureClientRequest struct {
	Email string `json:"email" validate:"required,email"`
	Name  string `json:"name" validate:"required"`
	Phone string `json:"phone"`
}

func (r *EnsureClientRequest) Validate() error {
	return validate.Struct(r)
}

var validate = validation.New()

func (r *UpdateCompanyRequest) Validate() error {
	return validate.Struct(r)
}

// UserResponse represents the DTO for user.User
type UserResponse struct {
	ID            string              `json:"id"`
	FirstName     string              `json:"firstname"`
	LastName      string              `json:"lastname"`
	FullName      string              `json:"fullname"`
	Email         string              `json:"email"`
	Role          string              `json:"role"`
	IsActive      bool                `json:"is_active"`
	Phone         string              `json:"phone"`
	Bio           string              `json:"bio"`
	AvatarURL     string              `json:"avatar_url"`
	Address       *user.Address       `json:"address,omitempty"`
	Notifications *user.Notifications `json:"notifications,omitempty"`
	// Banking exposed via GET /auth/me para que o portal e o admin
	// renderizem o formulário pré-preenchido. PATCH /auth/me aceita
	// updates via UpdateMeRequest.Banking. Issue #463.
	Banking *user.BankingInfo `json:"banking,omitempty"`
}

// CompanyResponse represents the DTO for company.Company
type CompanyResponse struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	LegalName string          `json:"legal_name"`
	TaxID     string          `json:"tax_id"`
	Email     string          `json:"email"`
	Phone     string          `json:"phone"`
	Address   company.Address `json:"address"`
	Type      string          `json:"type"`
	IsActive  bool            `json:"is_active"`
	OwnerID   string          `json:"owner_id"`
	Members   []CompanyMember `json:"members"`
}

type CompanyMember struct {
	AccountID string `json:"account_id"`
	Profile   string `json:"profile"`
}

// AccountResponse represents the DTO for account.Account
type AccountResponse struct {
	ID         string           `json:"id"`
	Type       string           `json:"type"`
	Individual *UserResponse    `json:"individual,omitempty"`
	Company    *CompanyResponse `json:"company,omitempty"`
	CreatedAt  time.Time        `json:"created_at"`
	UpdatedAt  time.Time        `json:"updated_at"`
}

// MapUserToDTO converts a domain User to a UserResponse DTO
func MapUserToDTO(u *user.User) *UserResponse {
	if u == nil {
		return nil
	}
	res := &UserResponse{
		ID:            u.ID,
		FirstName:     u.FirstName,
		LastName:      u.LastName,
		FullName:      u.FullName,
		Email:         u.Email,
		Role:          u.Role,
		IsActive:      u.IsActive,
		Phone:         u.Phone,
		Bio:           u.Bio,
		AvatarURL:     u.AvatarURL,
		Address:       &u.Address,
		Notifications: &u.Notifications,
	}
	// Banking só aparece no payload se houver pelo menos um campo
	// preenchido — frontend distingue "nunca informado" (nil) de
	// "informado-em-branco" (objeto com strings vazias).
	if !u.Banking.IsEmpty() {
		bankingCopy := u.Banking
		res.Banking = &bankingCopy
	}
	return res
}

// MapCompanyToDTO converts a domain Company to a CompanyResponse DTO
func MapCompanyToDTO(c *company.Company) *CompanyResponse {
	if c == nil {
		return nil
	}

	members := make([]CompanyMember, len(c.Members))
	for i, m := range c.Members {
		members[i] = CompanyMember{
			AccountID: m.AccountID,
			Profile:   m.Profile,
		}
	}

	return &CompanyResponse{
		ID:        c.ID,
		Name:      c.Name,
		LegalName: c.LegalName,
		TaxID:     c.TaxID,
		Email:     c.Email,
		Phone:     c.Phone,
		Address:   c.Address,
		Type:      c.Type,
		IsActive:  c.IsActive,
		OwnerID:   c.OwnerID,
		Members:   members,
	}
}

// MapAccountToDTO converts a domain Account to an AccountResponse DTO
func MapAccountToDTO(a *account.Account) *AccountResponse {
	if a == nil {
		return nil
	}
	return &AccountResponse{
		ID:         a.ID,
		Type:       string(a.Type),
		Individual: MapUserToDTO(a.Individual),
		Company:    MapCompanyToDTO(a.Company),
		CreatedAt:  a.CreatedAt,
		UpdatedAt:  a.UpdatedAt,
	}
}

// MapAccountsToDTO converts a slice of domain Accounts to a slice of AccountResponse DTOs
func MapAccountsToDTO(accounts []*account.Account) []AccountResponse {
	dtos := make([]AccountResponse, len(accounts))
	for i, a := range accounts {
		dtos[i] = *MapAccountToDTO(a)
	}
	return dtos
}
