package auth

import (
	"fmt"
	"time"
	"unicode"

	"github.com/golang-jwt/jwt/v5"
	"github.com/oginoino/toca/pkg/validation"
)

var validate = validation.New()

type Credential struct {
	ID           string    `json:"id" bson:"_id,omitempty"`
	AccountID    string    `json:"account_id" bson:"account_id" validate:"required"`
	Email        string    `json:"email" bson:"email" validate:"required,email"`
	PasswordHash string    `json:"-" bson:"password_hash" validate:"required"`
	IsVerified   bool      `json:"is_verified" bson:"is_verified"`
	Role         string    `json:"role" bson:"role"`
	CreatedAt    time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" bson:"updated_at"`
}

func (c *Credential) Validate() error {
	return validate.Struct(c)
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// TokenType constants for distinguishing access vs refresh tokens.
const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

type Claims struct {
	jwt.RegisteredClaims
	AccountID        string   `json:"account_id"`
	Email            string   `json:"email"`
	Name             string   `json:"name,omitempty"`
	AvatarURL        string   `json:"avatar_url,omitempty"`
	Role             string   `json:"role"`            // functional role
	Permission       string   `json:"permission"`      // company access profile
	FunctionalRole   string   `json:"functional_role"` // the new functional role from company member
	Type             string   `json:"type"`            // individual or company
	TenantID         string   `json:"tenant_id,omitempty"`
	PartnerTenantIDs []string `json:"partner_tenant_ids,omitempty"`
	TokenType        string   `json:"token_type,omitempty"` // "access" or "refresh"
	SessionID        string   `json:"session_id,omitempty"` // links token to server-side session
	FamilyID         string   `json:"family_id,omitempty"`  // refresh token rotation family
}

type RegisterRequest struct {
	Email           string `json:"email" validate:"required,email"`
	Password        string `json:"password" validate:"required,min=8"`
	ConfirmPassword string `json:"confirm_password" validate:"required,eqfield=Password"`
	Type            string `json:"type" validate:"required,oneof=individual company"`

	// For Individual
	FirstName string `json:"firstname" validate:"required_if=Type individual"`
	LastName  string `json:"lastname" validate:"required_if=Type individual"`
	Phone     string `json:"phone"`
	Bio       string `json:"bio"`
	Role      string `json:"role"` // Functional role (e.g. Administrador, Corretor, Gerente)

	// For Company
	CompanyName string `json:"company_name" validate:"required_if=Type company"`
	TaxID       string `json:"tax_id" validate:"required_if=Type company,omitempty,cnpj_valid"`
}

func (r *RegisterRequest) Validate() error {
	return validate.Struct(r)
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

func (l *LoginRequest) Validate() error {
	return validate.Struct(l)
}

type OAuthLoginRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

func (r *OAuthLoginRequest) Validate() error {
	return validate.Struct(r)
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type CreateCompanyRequest struct {
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
	Type string `json:"type"` // franchisor, franchisee
}

func (r *CreateCompanyRequest) Validate() error {
	return validate.Struct(r)
}

type AddUserRequest struct {
	AccountID string `json:"account_id"`
	FirstName string `json:"firstname" validate:"required"`
	LastName  string `json:"lastname" validate:"required"`
	Email     string `json:"email" validate:"required,email"`
	Password  string `json:"password" validate:"required,min=8"`
	Role      string `json:"role"`    // functional role (e.g. broker, staff)
	Profile   string `json:"profile"` // company access level (e.g. admin, manager)
}

func (r *AddUserRequest) Validate() error {
	return validate.Struct(r)
}

// UpdateMemberRequest — P2/M6+M7: alteração de papel/perfil e desativação de um
// vínculo existente. Todos os campos são opcionais (ponteiro/string vazia = não
// mexe), e a validação de valor é feita no serviço contra o catálogo de papéis
// (pkg/auth) e a allowlist de perfis — porque depende também de QUEM chama.
type UpdateMemberRequest struct {
	// FunctionalRole é o papel de catálogo (admin, gestor, corretor, ...).
	FunctionalRole string `json:"functional_role"`
	// Profile é o nível de acesso na empresa (member, viewer, admin).
	Profile string `json:"profile"`
	// IsActive — nil não mexe; false suspende o vínculo; true reativa.
	IsActive *bool `json:"is_active"`
}

// ValidatePasswordComplexity enforces minimum password strength requirements:
// at least 8 characters, 1 uppercase, 1 lowercase, 1 digit, 1 special character.
func ValidatePasswordComplexity(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, ch := range password {
		switch {
		case unicode.IsUpper(ch):
			hasUpper = true
		case unicode.IsLower(ch):
			hasLower = true
		case unicode.IsDigit(ch):
			hasDigit = true
		case unicode.IsPunct(ch) || unicode.IsSymbol(ch):
			hasSpecial = true
		}
	}
	if !hasUpper {
		return fmt.Errorf("password must contain at least one uppercase letter")
	}
	if !hasLower {
		return fmt.Errorf("password must contain at least one lowercase letter")
	}
	if !hasDigit {
		return fmt.Errorf("password must contain at least one digit")
	}
	if !hasSpecial {
		return fmt.Errorf("password must contain at least one special character")
	}
	return nil
}
