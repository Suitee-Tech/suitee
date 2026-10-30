package user

import (
	"regexp"
	"strings"

	"github.com/oginoino/toca/pkg/validation"
)

var validate = validation.New()

type Address struct {
	Street     string `bson:"street" json:"street"`
	Number     string `bson:"number" json:"number"`
	Complement string `bson:"complement" json:"complement"`
	District   string `bson:"district" json:"district"`
	City       string `bson:"city" json:"city"`
	State      string `bson:"state" json:"state"`
	ZipCode    string `bson:"zip_code" json:"zip_code"`
}

type Notifications struct {
	Email bool `bson:"email" json:"email"`
	SMS   bool `bson:"sms" json:"sms"`
	Push  bool `bson:"push" json:"push"`
}

// PixKeyType — categoria da chave PIX. Definida no padrão SPB/BACEN.
// Valida formato de cada tipo separadamente para que o usuário não
// salve uma chave "email" que é na verdade um CPF, etc.
type PixKeyType string

const (
	PixKeyTypeCPF   PixKeyType = "cpf"
	PixKeyTypeCNPJ  PixKeyType = "cnpj"
	PixKeyTypeEmail PixKeyType = "email"
	PixKeyTypePhone PixKeyType = "phone"
	PixKeyTypeEVP   PixKeyType = "evp" // chave aleatória (UUID v4)
)

func (t PixKeyType) IsValid() bool {
	switch t {
	case PixKeyTypeCPF, PixKeyTypeCNPJ, PixKeyTypeEmail, PixKeyTypePhone, PixKeyTypeEVP:
		return true
	}
	return false
}

// AccountType — corrente ou poupança. Influencia no roteamento do
// SPB (algumas operadoras tratam savings com prefixo diferente).
type BankAccountType string

const (
	BankAccountTypeChecking BankAccountType = "checking"
	BankAccountTypeSavings  BankAccountType = "savings"
)

func (t BankAccountType) IsValid() bool {
	return t == BankAccountTypeChecking || t == BankAccountTypeSavings
}

// BankingInfo — dados bancários do usuário para receber repasses
// (aluguel, comissões). Issue #463.
//
// Por que armazenar holder separado do User?
//   - Pode ser conta de cônjuge / sócio.
//   - Em pessoa jurídica (corretora), o holder é a empresa, não o
//     operador que está editando.
//   - Auditoria de fraude precisa do nome registrado pelo usuário no
//     momento, não o nome atual do perfil (que pode ter mudado).
//
// LGPD: nenhum desses campos é PII pública. AccountNumber/PixKey
// devem ser tratados como secret-like nos logs (audit entries gravam
// only o tipo de mudança + últimos 4 dígitos quando aplicável, não
// o valor inteiro). Ver UpdateBankingInfo no service.
type BankingInfo struct {
	// Banco — código compe (3 dígitos, ex.: 341 Itaú) ou ISPB (8
	// dígitos para participantes do SPB que não têm compe).
	// Mantemos como string porque alguns bancos digitais usam ISPB.
	BankCode string `bson:"bank_code,omitempty" json:"bank_code,omitempty"`
	BankName string `bson:"bank_name,omitempty" json:"bank_name,omitempty"`

	// Agência — só dígitos, sem dígito verificador (o BR não tem padrão
	// uniforme — alguns bancos têm 4 dígitos, outros 5; alguns têm
	// DV, outros não — não validamos aqui).
	Agency       string `bson:"agency,omitempty" json:"agency,omitempty"`
	AgencyDigit  string `bson:"agency_digit,omitempty" json:"agency_digit,omitempty"`
	Account      string `bson:"account,omitempty" json:"account,omitempty"`
	AccountDigit string `bson:"account_digit,omitempty" json:"account_digit,omitempty"`

	// AccountType — corrente ou poupança.
	AccountType BankAccountType `bson:"account_type,omitempty" json:"account_type,omitempty"`

	// PIX — chave de transferência instantânea. Tipo separado do valor
	// porque a validação muda por tipo (CPF vs. email vs. EVP).
	PixKeyType PixKeyType `bson:"pix_key_type,omitempty" json:"pix_key_type,omitempty"`
	PixKey     string     `bson:"pix_key,omitempty" json:"pix_key,omitempty"`

	// Holder — titular da conta. Quando vazio, assumimos que é o
	// próprio usuário (FullName). Document é CPF (PF) ou CNPJ (PJ).
	HolderName     string `bson:"holder_name,omitempty" json:"holder_name,omitempty"`
	HolderDocument string `bson:"holder_document,omitempty" json:"holder_document,omitempty"`
}

// IsEmpty é true quando todos os campos estão vazios. Permite o
// service distinguir "usuário não informou" de "usuário limpou os
// dados" — para audit + UI.
func (b BankingInfo) IsEmpty() bool {
	return b == BankingInfo{}
}

var pixPhoneRegex = regexp.MustCompile(`^\+?\d{10,13}$`)
var pixEmailRegex = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
var pixEVPRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Validate verifica internamente a consistência dos campos
// preenchidos. Não exige todos os campos juntos (usuário pode salvar
// só PIX, só dados bancários tradicionais, ou ambos), mas se
// preencher um trio (Bank/Agency/Account) exige os três, e se
// preencher PIX exige tipo + key consistentes.
func (b BankingInfo) Validate() error {
	if b.IsEmpty() {
		return nil
	}

	// Trio bancário tradicional: se um campo estiver setado, exige os
	// três (BankCode, Agency, Account) — meio-preenchido confunde
	// callers de repasse.
	hasAnyBank := strings.TrimSpace(b.BankCode) != "" ||
		strings.TrimSpace(b.Agency) != "" ||
		strings.TrimSpace(b.Account) != ""
	if hasAnyBank {
		if strings.TrimSpace(b.BankCode) == "" {
			return errFieldRequired("bank_code")
		}
		if strings.TrimSpace(b.Agency) == "" {
			return errFieldRequired("agency")
		}
		if strings.TrimSpace(b.Account) == "" {
			return errFieldRequired("account")
		}
		if b.AccountType != "" && !b.AccountType.IsValid() {
			return errInvalidValue("account_type", string(b.AccountType))
		}
	}

	// PIX: tipo + key devem vir juntos. Cada tipo tem seu formato.
	hasAnyPix := strings.TrimSpace(b.PixKey) != "" || b.PixKeyType != ""
	if hasAnyPix {
		if !b.PixKeyType.IsValid() {
			return errInvalidValue("pix_key_type", string(b.PixKeyType))
		}
		if strings.TrimSpace(b.PixKey) == "" {
			return errFieldRequired("pix_key")
		}
		key := strings.TrimSpace(b.PixKey)
		switch b.PixKeyType {
		case PixKeyTypeCPF:
			if !validation.IsValidCPF(key) {
				return errInvalidValue("pix_key", "cpf inválido")
			}
		case PixKeyTypeCNPJ:
			if !validation.IsValidCNPJ(key) {
				return errInvalidValue("pix_key", "cnpj inválido")
			}
		case PixKeyTypeEmail:
			if !pixEmailRegex.MatchString(key) {
				return errInvalidValue("pix_key", "email inválido")
			}
		case PixKeyTypePhone:
			if !pixPhoneRegex.MatchString(key) {
				return errInvalidValue("pix_key", "telefone deve ter 10-13 dígitos")
			}
		case PixKeyTypeEVP:
			if !pixEVPRegex.MatchString(key) {
				return errInvalidValue("pix_key", "evp deve ser UUID v4")
			}
		}
	}

	// Holder document opcional — se preenchido, deve ser CPF ou CNPJ
	// válido (qualquer um). Não exigimos qual: portais de PF têm CPF,
	// PJ têm CNPJ.
	if doc := strings.TrimSpace(b.HolderDocument); doc != "" {
		if !validation.IsValidCPF(doc) && !validation.IsValidCNPJ(doc) {
			return errInvalidValue("holder_document", "deve ser CPF ou CNPJ válido")
		}
	}

	return nil
}

type User struct {
	ID            string        `bson:"_id,omitempty"`
	FirstName     string        `bson:"firstname" validate:"required"`
	LastName      string        `bson:"lastname" validate:"required"`
	FullName      string        `bson:"fullname"`
	Email         string        `bson:"email" validate:"required,email"`
	Role          string        `bson:"role"` // Functional role (e.g. broker, staff). NOT a PartyRole (buyer/seller)
	IsActive      bool          `bson:"is_active"`
	Phone         string        `bson:"phone"`
	Bio           string        `bson:"bio"`
	AvatarURL     string        `bson:"avatar_url"`
	Address       Address       `bson:"address,omitempty" json:"address,omitempty"`
	Notifications Notifications `bson:"notifications,omitempty" json:"notifications,omitempty"`
	// Banking — dados bancários para repasses (#463). Opcional, mas
	// quando preenchido valida via BankingInfo.Validate.
	Banking BankingInfo `bson:"banking,omitempty" json:"banking,omitempty"`
}

func (u *User) Validate() error {
	if err := validate.Struct(u); err != nil {
		return err
	}
	if err := u.Banking.Validate(); err != nil {
		return err
	}
	return nil
}

// BankingValidationError — erro tipado retornado por
// BankingInfo.Validate. Carrega field + reason para que o handler
// possa montar um response 400 estruturado.
type BankingValidationError struct {
	Field  string
	Reason string
}

func (e *BankingValidationError) Error() string {
	return "banking." + e.Field + ": " + e.Reason
}

func errFieldRequired(field string) error {
	return &BankingValidationError{Field: field, Reason: "campo obrigatório"}
}

func errInvalidValue(field, reason string) error {
	return &BankingValidationError{Field: field, Reason: reason}
}
