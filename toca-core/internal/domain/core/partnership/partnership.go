package partnership

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	access "github.com/oginoino/toca/pkg/partnershipaccess"
	"github.com/oginoino/toca/pkg/validation"
)

var validate = validation.New()

type PartnershipType string

const (
	PartnershipTypeFranchise PartnershipType = "franchise"
	PartnershipTypePartner   PartnershipType = "partner"
)

type PartnershipStatus string

const (
	PartnershipStatusActive   PartnershipStatus = "active"
	PartnershipStatusPending  PartnershipStatus = "pending"
	PartnershipStatusInactive PartnershipStatus = "inactive"
)

// RoyaltyBase identifies which amount the royalty percentage is applied to
// when a franchise deal closes.
type RoyaltyBase string

const (
	RoyaltyBaseGrossCommission RoyaltyBase = "gross_commission"
	RoyaltyBaseNetCommission   RoyaltyBase = "net_commission"
	RoyaltyBaseRevenue         RoyaltyBase = "revenue"
)

// CommissionModel is the split applied when a CROSS-COMPANY deal closes —
// i.e. a partner company sells a property shared by the other company. The
// two percentages should sum to 100.
type CommissionModel struct {
	OwnerCompanyPct   float64 `json:"owner_company_pct" bson:"owner_company_pct" validate:"gte=0,lte=100"`
	ClosingCompanyPct float64 `json:"closing_company_pct" bson:"closing_company_pct" validate:"gte=0,lte=100"`
}

// IsZero reports whether the model carries no configuration (both pcts zero).
func (m CommissionModel) IsZero() bool {
	return m.OwnerCompanyPct == 0 && m.ClosingCompanyPct == 0
}

type FinancialConfig struct {
	FeePayer          string  `json:"fee_payer,omitempty"`
	RoyaltyPercentage float64 `json:"royalty_percentage" bson:"royalty_percentage" validate:"gte=0,lte=100"`
	AdminFee          float64 `json:"admin_fee" bson:"admin_fee" validate:"gte=0"`
	// CommissionSplit is the legacy "X/Y" string. No longer required — kept
	// only so old rows keep deserialising. Prefer CommissionModel.
	CommissionSplit string  `json:"commission_split,omitempty" bson:"commission_split,omitempty"`
	TransactionFee  float64 `json:"transaction_fee" bson:"transaction_fee" validate:"gte=0"`

	// CommissionModel is the structured cross-company commission split.
	CommissionModel CommissionModel `json:"commission_model" bson:"commission_model"`
	// RoyaltyBase is the base the royalty percentage is applied to. Empty
	// is treated as gross_commission by EffectiveRoyaltyBase().
	RoyaltyBase RoyaltyBase `json:"royalty_base,omitempty" bson:"royalty_base,omitempty" validate:"omitempty,oneof=gross_commission net_commission revenue"`
}

// EffectiveCommissionModel returns the structured CommissionModel when it is
// set, otherwise parses the legacy "X/Y" CommissionSplit string, otherwise
// defaults to a 50/50 split.
func (c FinancialConfig) EffectiveCommissionModel() CommissionModel {
	if !c.CommissionModel.IsZero() {
		return c.CommissionModel
	}
	if owner, closing, ok := parseCommissionSplit(c.CommissionSplit); ok {
		return CommissionModel{OwnerCompanyPct: owner, ClosingCompanyPct: closing}
	}
	return CommissionModel{OwnerCompanyPct: 50, ClosingCompanyPct: 50}
}

// EffectiveRoyaltyBase returns the configured RoyaltyBase, defaulting to
// gross_commission when empty.
func (c FinancialConfig) EffectiveRoyaltyBase() RoyaltyBase {
	if c.RoyaltyBase == "" {
		return RoyaltyBaseGrossCommission
	}
	return c.RoyaltyBase
}

// parseCommissionSplit parses a legacy "X/Y" string into owner/closing
// percentages. Returns ok=false when the string is empty or malformed.
func parseCommissionSplit(s string) (owner, closing float64, ok bool) {
	parts := strings.Split(strings.TrimSpace(s), "/")
	if len(parts) != 2 {
		return 0, 0, false
	}
	o, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	c, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return o, c, true
}

// SharingConfig holds the partnership-level rules for the shared portfolio
// feature: whether owner contact PII is shared, whether co-brokering is
// allowed, and optional whitelists narrowing which property types/regions
// are exposed to the partner.
type SharingConfig = access.Sharing

type PermissionScope = string

const (
	PermissionScopeSharedPortfolio PermissionScope = "shared_portfolio"
	PermissionScopeLeads           PermissionScope = "leads"
)

type AccessLevel = string

const (
	AccessLevelView   AccessLevel = "view"
	AccessLevelCreate AccessLevel = "create"
	AccessLevelEdit   AccessLevel = "edit"
	AccessLevelDelete AccessLevel = "delete"
)

type Permission = access.Permission

type AuditLog struct {
	TenantID  string    `json:"tenant_id"`
	Version   int       `json:"version"`
	Before    any       `json:"before,omitempty"`
	After     any       `json:"after,omitempty"`
	Action    string    `json:"action" bson:"action"`
	UserID    string    `json:"user_id" bson:"user_id"`
	Timestamp time.Time `json:"timestamp" bson:"timestamp"`
	Details   string    `json:"details,omitempty" bson:"details,omitempty"`
}

type Partnership struct {
	Version          int                     `json:"version"`
	FinancialVersion int                     `json:"financial_version"`
	Grants           map[string]access.Grant `json:"grants,omitempty"`
	PendingFinancial *FinancialProposal      `json:"pending_financial,omitempty"`
	Companies        map[string]Company      `json:"companies,omitempty"`
	EventAction      string                  `json:"-"`
	ID               string                  `json:"id" bson:"_id,omitempty"`
	RequesterID      string                  `json:"requester_id" bson:"requester_id" validate:"required"` // Franchisor or Initiator
	TargetID         string                  `json:"target_id" bson:"target_id" validate:"required"`       // Franchisee or Partner
	Type             PartnershipType         `json:"type" bson:"type" validate:"required"`
	Status           PartnershipStatus       `json:"status" bson:"status" validate:"required"`
	FinancialConfig  FinancialConfig         `json:"financial_config" bson:"financial_config"`
	SharingConfig    SharingConfig           `json:"sharing_config" bson:"sharing_config"`
	Permissions      []Permission            `json:"permissions" bson:"permissions"`
	AuditLogs        []AuditLog              `json:"audit_logs,omitempty" bson:"audit_logs,omitempty"`
	CreatedAt        time.Time               `json:"created_at" bson:"created_at"`
	UpdatedAt        time.Time               `json:"updated_at" bson:"updated_at"`
}

func (p *Partnership) Validate() error {
	if p.RequesterID == p.TargetID {
		return errors.New("a empresa parceira deve ser diferente da sua empresa")
	}
	if p.Type != PartnershipTypePartner && p.Type != PartnershipTypeFranchise {
		return errors.New("tipo de parceria inválido")
	}
	if err := validate.Struct(p); err != nil {
		return err
	}
	return p.FinancialConfig.Validate()
}

// Counterpart returns the OTHER participant id given one participant. Returns
// an empty string when callerID is neither participant.
func (p *Partnership) Counterpart(callerID string) string {
	switch callerID {
	case p.RequesterID:
		return p.TargetID
	case p.TargetID:
		return p.RequesterID
	default:
		return ""
	}
}

// IsParticipant reports whether tenantID is one of the two companies in the
// partnership.
func (p *Partnership) IsParticipant(tenantID string) bool {
	return tenantID != "" && (tenantID == p.RequesterID || tenantID == p.TargetID)
}

func (c FinancialConfig) Validate() error {
	if err := access.ValidateFees(c.AdminFee, c.TransactionFee, c.FeePayer); err != nil {
		return err
	}
	if err := validate.Struct(c); err != nil {
		return errors.New("valores financeiros inválidos")
	}
	model := c.EffectiveCommissionModel()
	if math.IsNaN(model.OwnerCompanyPct) || math.IsNaN(model.ClosingCompanyPct) || math.IsInf(model.OwnerCompanyPct, 0) || math.IsInf(model.ClosingCompanyPct, 0) || model.OwnerCompanyPct < 0 || model.OwnerCompanyPct > 100 || model.ClosingCompanyPct < 0 || model.ClosingCompanyPct > 100 || math.Abs(model.OwnerCompanyPct+model.ClosingCompanyPct-100) > 0.01 {
		return errors.New("a divisão da comissão deve somar 100%")
	}
	return nil
}
func (p *Partnership) GrantFrom(owner string) access.Grant {
	if !p.IsParticipant(owner) {
		return access.Grant{}
	}
	return access.Resolve(p.Grants, owner, p.SharingConfig, p.Permissions)
}
func (p *Partnership) FreezeGrants() {
	if p.Grants == nil {
		p.Grants = map[string]access.Grant{p.RequesterID: p.GrantFrom(p.RequesterID), p.TargetID: p.GrantFrom(p.TargetID)}
	}
}

type Company struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	TaxID  string `json:"tax_id"`
	City   string `json:"city"`
	State  string `json:"state"`
	Active bool   `json:"active"`
}
type FinancialProposal struct {
	ProposedBy string          `json:"proposed_by"`
	Version    int             `json:"version"`
	Config     FinancialConfig `json:"financial_config"`
}

var ErrConflict = errors.New("o acordo foi alterado; recarregue antes de continuar")
var ErrNotFound = errors.New("parceria não encontrada")
var ErrDuplicate = errors.New("já existe convite ou parceria vigente entre estas empresas")
var ErrCompany = errors.New("empresa parceira inexistente ou inativa")

// Equivalent compares effective terms, including defaults used by legacy agreements.
func (c FinancialConfig) Equivalent(other FinancialConfig) bool {
	return c.FeePayer == other.FeePayer && c.RoyaltyPercentage == other.RoyaltyPercentage && c.AdminFee == other.AdminFee &&
		c.TransactionFee == other.TransactionFee && c.EffectiveCommissionModel() == other.EffectiveCommissionModel() &&
		c.EffectiveRoyaltyBase() == other.EffectiveRoyaltyBase()
}
