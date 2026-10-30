package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/oginoino/toca/pkg/database"
)

// PartnershipFinancials is a LOCAL projection of the financial slice of a
// core.partnerships row, decoded from its metadata JSONB. toca-financial does
// not import the toca-core partnership domain — it reads the cross-schema
// row directly (same pilot pattern as the CRM lookups in this file) and only
// needs these fields to apply the cross-company commission split and the
// franchise royalty.
type PartnershipFinancials struct {
	FeePayer        string `json:"fee_payer,omitempty"`
	ID              string `json:"id"`
	Type            string `json:"type"` // "franchise" | "partner"
	Status          string `json:"status"`
	RequesterTenant string `json:"requester_tenant"`
	TargetTenant    string `json:"target_tenant"`

	RoyaltyPercentage float64 `json:"royalty_percentage"`
	RoyaltyBase       string  `json:"royalty_base"` // gross_commission | net_commission | revenue
	AdminFee          float64 `json:"admin_fee"`
	TransactionFee    float64 `json:"transaction_fee"`

	// CommissionModel — structured cross-company split. CommissionSplit is
	// the legacy "X/Y" string kept for old rows.
	OwnerCompanyPct   float64
	ClosingCompanyPct float64
	LegacySplit       string
}

// financialConfigJSON mirrors the shape of metadata.financial_config so the
// commission model nesting decodes cleanly.
type financialConfigJSON struct {
	FeePayer          string  `json:"fee_payer,omitempty"`
	RoyaltyPercentage float64 `json:"royalty_percentage"`
	RoyaltyBase       string  `json:"royalty_base"`
	AdminFee          float64 `json:"admin_fee"`
	TransactionFee    float64 `json:"transaction_fee"`
	CommissionSplit   string  `json:"commission_split"`
	CommissionModel   struct {
		OwnerCompanyPct   float64 `json:"owner_company_pct"`
		ClosingCompanyPct float64 `json:"closing_company_pct"`
	} `json:"commission_model"`
}

type partnershipMetadataJSON struct {
	ID              string              `json:"id"`
	Type            string              `json:"type"`
	Status          string              `json:"status"`
	FinancialConfig financialConfigJSON `json:"financial_config"`
}

// IsFranchise reports whether the partnership is a franchise agreement.
func (p *PartnershipFinancials) IsFranchise() bool {
	return p != nil && p.Type == "franchise"
}

// EffectiveCommissionModel returns the cross-company split: the structured
// CommissionModel when set, else the parsed legacy "X/Y" string, else 50/50.
func (p *PartnershipFinancials) EffectiveCommissionModel() (ownerPct, closingPct float64) {
	if p == nil {
		return 50, 50
	}
	if p.OwnerCompanyPct != 0 || p.ClosingCompanyPct != 0 {
		return p.OwnerCompanyPct, p.ClosingCompanyPct
	}
	if o, c, ok := parseLegacySplit(p.LegacySplit); ok {
		return o, c
	}
	return 50, 50
}

// EffectiveRoyaltyBase defaults an empty base to gross_commission.
func (p *PartnershipFinancials) EffectiveRoyaltyBase() string {
	if p == nil || p.RoyaltyBase == "" {
		return "gross_commission"
	}
	return p.RoyaltyBase
}

// parseLegacySplit parses a legacy "X/Y" commission split string. Returns
// ok=false when the string is empty or malformed.
func parseLegacySplit(s string) (owner, closing float64, ok bool) {
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

// CrossTenantRepository exposes minimal cross-schema lookups for
// portal-scoped flows and cross-company financials. Same pilot pattern as
// toca-deal/cross_postgres.go and toca-core/cross_postgres.go: the schemas
// live in the same Postgres instance so we read crm.leads / core.properties /
// core.partnerships directly. Splitting modules into their own DBs would
// require swapping this for HTTP calls — but the interface stays the same.
type CrossTenantRepository interface {
	// LeadIDsForAccount returns every lead.id within the tenant whose
	// `metadata->>'account_id'` matches the given account.
	LeadIDsForAccount(ctx context.Context, tenantID, accountID string) ([]string, error)

	// PropertyOwnerTenant returns the tenant_id that owns a property. Used
	// to detect a cross-company deal (the property's owner tenant differs
	// from the closing deal's tenant). Returns sql.ErrNoRows-wrapped error
	// when the property does not exist or is soft-deleted.
	PropertyOwnerTenant(ctx context.Context, propertyID string) (string, error)

	// ActivePartnershipBetween returns the ACTIVE partnership between two
	// companies (in either direction), or nil when none exists.
	ActivePartnershipBetween(ctx context.Context, companyA, companyB string) (*PartnershipFinancials, error)
}

type crossPostgresRepository struct {
	db *database.PostgresDB
}

func NewCrossPostgresRepository(db *database.PostgresDB) CrossTenantRepository {
	return &crossPostgresRepository{db: db}
}

func (r *crossPostgresRepository) LeadIDsForAccount(ctx context.Context, tenantID, accountID string) ([]string, error) {
	// A chave da conta no JSON do lead é `AccountID` (PascalCase, como o CRM
	// serializa); a consulta só olhava `account_id` e por isso NUNCA casava —
	// medido no beta: 0 leads pela minúscula contra 95 pela PascalCase. Aceitar
	// as duas mantém compatibilidade com dados antigos.
	// POOL DE SERVIÇO explícito: chamado tanto pelo operador quanto pelo portal do
	// cliente. O tenant continua no WHERE (escopo explícito), então o resultado
	// segue limitado ao tenant pedido — o que muda é não depender da RLS do pool
	// de aplicação, que para o cliente cross-tenant filtrava tudo.
	rows, err := r.db.QueryServiceRows(ctx, `
		SELECT id::text
		FROM crm.leads
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND (metadata->>'AccountID' = $2 OR metadata->>'account_id' = $2)
	`, tenantID, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *crossPostgresRepository) PropertyOwnerTenant(ctx context.Context, propertyID string) (string, error) {
	if propertyID == "" {
		return "", errors.New("property_id required")
	}
	var tenantID string
	err := r.db.QueryRow(ctx, `
		SELECT tenant_id::text
		FROM core.properties
		WHERE id = $1 AND deleted_at IS NULL
	`, propertyID).Scan(&tenantID)
	if err != nil {
		return "", err
	}
	return tenantID, nil
}

func (r *crossPostgresRepository) ActivePartnershipBetween(ctx context.Context, companyA, companyB string) (*PartnershipFinancials, error) {
	if companyA == "" || companyB == "" {
		return nil, nil
	}
	var requesterTenant, targetTenant string
	var metadata []byte
	err := r.db.QueryRow(ctx, `
		SELECT requester_id::text, target_id::text, metadata
		FROM core.partnerships
		WHERE status = 'active'
		  AND deleted_at IS NULL
		  AND (
		        (requester_id::text = $1 AND target_id::text = $2)
		     OR (requester_id::text = $2 AND target_id::text = $1)
		      )
		ORDER BY created_at DESC
		LIMIT 1
	`, companyA, companyB).Scan(&requesterTenant, &targetTenant, &metadata)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var raw partnershipMetadataJSON
	if err := json.Unmarshal(metadata, &raw); err != nil {
		return nil, err
	}

	return &PartnershipFinancials{
		ID:                raw.ID,
		Type:              raw.Type,
		Status:            raw.Status,
		RequesterTenant:   requesterTenant,
		TargetTenant:      targetTenant,
		RoyaltyPercentage: raw.FinancialConfig.RoyaltyPercentage,
		RoyaltyBase:       raw.FinancialConfig.RoyaltyBase,
		FeePayer:          raw.FinancialConfig.FeePayer,
		AdminFee:          raw.FinancialConfig.AdminFee,
		TransactionFee:    raw.FinancialConfig.TransactionFee,
		OwnerCompanyPct:   raw.FinancialConfig.CommissionModel.OwnerCompanyPct,
		ClosingCompanyPct: raw.FinancialConfig.CommissionModel.ClosingCompanyPct,
		LegacySplit:       raw.FinancialConfig.CommissionSplit,
	}, nil
}

// PartnershipForDeal freezes the accepted terms of existing deals even after
// the agreement is changed or revoked. Old partner deals without a snapshot
// require historical review; no current agreement is substituted.
func (r *crossPostgresRepository) PartnershipForDeal(ctx context.Context, dealID, owner, closing string) (*PartnershipFinancials, error) {
	var snapshot []byte
	err := r.db.QueryRow(ctx, `SELECT metadata->'partnership_snapshot' FROM deal.deals WHERE id::text=$1 AND tenant_id::text=$2`, dealID, closing).Scan(&snapshot)
	if err != nil {
		return nil, err
	}
	if len(snapshot) == 0 || string(snapshot) == "null" {
		return nil, ErrHistoricalPartnershipRequired
	}
	var raw struct {
		ID              string              `json:"id"`
		Type            string              `json:"type"`
		RequesterID     string              `json:"requester_id"`
		TargetID        string              `json:"target_id"`
		FinancialConfig financialConfigJSON `json:"financial_config"`
	}
	if err := json.Unmarshal(snapshot, &raw); err != nil {
		return nil, err
	}
	if raw.ID == "" || !((raw.RequesterID == owner && raw.TargetID == closing) || (raw.TargetID == owner && raw.RequesterID == closing)) {
		return nil, fmt.Errorf("deal partnership snapshot does not match companies")
	}
	return &PartnershipFinancials{ID: raw.ID, Type: raw.Type, Status: "accepted_snapshot", RequesterTenant: raw.RequesterID, TargetTenant: raw.TargetID, RoyaltyPercentage: raw.FinancialConfig.RoyaltyPercentage, RoyaltyBase: raw.FinancialConfig.RoyaltyBase, FeePayer: raw.FinancialConfig.FeePayer, AdminFee: raw.FinancialConfig.AdminFee, TransactionFee: raw.FinancialConfig.TransactionFee, OwnerCompanyPct: raw.FinancialConfig.CommissionModel.OwnerCompanyPct, ClosingCompanyPct: raw.FinancialConfig.CommissionModel.ClosingCompanyPct, LegacySplit: raw.FinancialConfig.CommissionSplit}, nil
}

func ResolvePartnershipForDeal(ctx context.Context, repo interface {
	ActivePartnershipBetween(context.Context, string, string) (*PartnershipFinancials, error)
}, dealID, owner, closing string) (*PartnershipFinancials, error) {
	if resolver, ok := repo.(interface {
		PartnershipForDeal(context.Context, string, string, string) (*PartnershipFinancials, error)
	}); ok && dealID != "" {
		return resolver.PartnershipForDeal(ctx, dealID, owner, closing)
	}
	return repo.ActivePartnershipBetween(ctx, owner, closing)
}

// ErrHistoricalPartnershipRequired prevents today's agreement from rewriting
// the economics of an old deal for which accepted terms were never recorded.
var ErrHistoricalPartnershipRequired = errors.New("condições da parceria a apurar: este negócio não possui um acordo financeiro registrado; revise o histórico antes de provisionar valores")

// FinancialPartnershipForDeal resolves the origin recorded when the deal was
// authorized. A shared lead is a partner contribution even with a local property.
func (r *crossPostgresRepository) FinancialPartnershipForDeal(ctx context.Context, dealID, closing string) (*PartnershipFinancials, string, error) {
	var metadata []byte
	if err := r.db.QueryRow(ctx, `SELECT metadata FROM deal.deals WHERE id::text=$1 AND tenant_id::text=$2`, dealID, closing).Scan(&metadata); err != nil {
		return nil, "", err
	}
	var d struct {
		SourceCompanyID string `json:"source_company_id"`
		CoBrokering     bool   `json:"co_brokering"`
		PropertyID      string `json:"property_id"`
		Snapshot        *struct {
			RequesterID string `json:"requester_id"`
			TargetID    string `json:"target_id"`
		} `json:"partnership_snapshot"`
	}
	if err := json.Unmarshal(metadata, &d); err != nil {
		return nil, "", err
	}
	if d.Snapshot != nil {
		source := ""
		if d.Snapshot.RequesterID == closing {
			source = d.Snapshot.TargetID
		} else if d.Snapshot.TargetID == closing {
			source = d.Snapshot.RequesterID
		}
		if source == "" || source == closing || (d.SourceCompanyID != "" && d.SourceCompanyID != source) {
			return nil, "", fmt.Errorf("origem da parceria incompatível com o negócio")
		}
		p, err := r.PartnershipForDeal(ctx, dealID, source, closing)
		return p, source, err
	}
	if d.CoBrokering || d.SourceCompanyID != "" {
		return nil, "", ErrHistoricalPartnershipRequired
	}
	if d.PropertyID != "" {
		owner, err := r.PropertyOwnerTenant(ctx, d.PropertyID)
		if err != nil {
			return nil, "", err
		}
		if owner != "" && owner != closing {
			return nil, "", ErrHistoricalPartnershipRequired
		}
	}
	return nil, "", nil
}

func ResolveDealPartnership(ctx context.Context, repo interface {
	PropertyOwnerTenant(context.Context, string) (string, error)
	ActivePartnershipBetween(context.Context, string, string) (*PartnershipFinancials, error)
}, dealID, propertyID, closing string) (*PartnershipFinancials, string, error) {
	if resolver, ok := repo.(interface {
		FinancialPartnershipForDeal(context.Context, string, string) (*PartnershipFinancials, string, error)
	}); ok && dealID != "" {
		return resolver.FinancialPartnershipForDeal(ctx, dealID, closing)
	}
	// Compatibility for adapters that have not adopted persisted deal origins.
	if propertyID == "" {
		return nil, "", nil
	}
	owner, err := repo.PropertyOwnerTenant(ctx, propertyID)
	if err != nil {
		return nil, "", err
	}
	if owner == "" || owner == closing {
		return nil, "", nil
	}
	p, err := ResolvePartnershipForDeal(ctx, repo, dealID, owner, closing)
	return p, owner, err
}

// PortalLeadScope é um vínculo do cliente: o tenant da IMOBILIÁRIA (não o tenant
// do token, que para um cliente OTP é a conta local dele) e os leads daquele
// tenant ligados à conta/e-mail do cliente.
type PortalLeadScope struct {
	TenantID string
	LeadIDs  []string
}

// LeadScopesForAccount lista os vínculos do cliente no POOL DE SERVIÇO.
//
// Por que aqui: o portal do cliente é cross-tenant por definição. O token do
// cliente carrega o tenant da conta dele, e as locações/cobranças/repasses são
// do tenant da imobiliária — sob a RLS do pool de aplicação (e com o tenant
// errado) o portal financeiro devolvia vazio. A autorização é a posse da conta
// (account_id na claim) ou do e-mail pelo token.
//
// A chave da conta no JSON do lead é `AccountID` (PascalCase); `account_id` fica
// aceito por compatibilidade.
func (r *crossPostgresRepository) LeadScopesForAccount(ctx context.Context, accountID, email string) ([]PortalLeadScope, error) {
	if accountID == "" && email == "" {
		return nil, nil
	}
	rows, err := r.db.QueryServiceRows(ctx, `
		SELECT tenant_id::text, id::text
		FROM crm.leads
		WHERE deleted_at IS NULL
		  AND (
		    ($1 <> '' AND (metadata->>'AccountID' = $1 OR metadata->>'account_id' = $1))
		    OR ($2 <> '' AND lower(metadata->>'Email') = lower($2))
		  )
	`, accountID, email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	order := make([]string, 0)
	byTenant := map[string][]string{}
	for rows.Next() {
		var tenantID, leadID string
		if err := rows.Scan(&tenantID, &leadID); err != nil {
			return nil, err
		}
		if _, seen := byTenant[tenantID]; !seen {
			order = append(order, tenantID)
		}
		byTenant[tenantID] = append(byTenant[tenantID], leadID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]PortalLeadScope, 0, len(order))
	for _, tenantID := range order {
		out = append(out, PortalLeadScope{TenantID: tenantID, LeadIDs: byTenant[tenantID]})
	}
	return out, nil
}
