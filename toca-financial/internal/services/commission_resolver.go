package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/oginoino/toca/pkg/domain/valueobjects"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/tenant_financial_settings"
)

// CommissionPlanInput contains every value needed to resolve a commission
// projection. It deliberately has no repositories or services so the same
// calculation can be reused by previews and write-side event handlers.
type CommissionPlanInput struct {
	DealType             string
	DealValue            float64
	CommissionPct        float64
	BrokerParticipations []contracts.BrokerParticipation
	Rules                []commission.Rule
	TenantSettings       *tenant_financial_settings.Settings
	// CrossCompany carries the cross-company split and franchise royalty
	// context resolved by the cross-company step. When non-nil it is folded
	// into the PlanHash so preview and materialisation produce the same
	// fingerprint for partnership deals.
	CrossCompany *CrossCompanyPlanInput
}

// CrossCompanyPlanInput is the pure cross-company context needed to make the
// plan hash sensitive to the partnership split and franchise royalty. It
// carries no amounts: the owner share and royalty legs are computed by their
// own handlers, but their inputs must participate in the fingerprint.
type CrossCompanyPlanInput struct {
	OwnerPercentage   float64
	ClosingPercentage float64
	RoyaltyPercentage float64
	RoyaltyBaseMode   string
}

// ResolvedCommissionShare is the canonical percentage/amount pair for one
// beneficiary. Ledger transactions, Commission entities, previews and payouts
// must consume these resolved values instead of reinterpreting the raw event.
// Amounts are integer cents so every consumer shares the exact same value.
type ResolvedCommissionShare struct {
	BrokerID    string
	Role        string
	Percentage  float64
	AmountCents int64
}

// ResolvedCommissionPlan is the canonical result of commission resolution for
// a deal snapshot. Once populated in the contract pipeline it is shared as a
// single immutable snapshot. Amounts are integer cents; consumers convert to
// float only at the persistence boundary of Transaction/Commission, which is
// exact for cent values.
//
// PlanHash is a deterministic SHA-256 fingerprint of all inputs so preview
// and contract.signed materialisation produce the same hash for the same
// inputs. Downstream consumers (payout, inbox) read only the frozen plan;
// they must never re-resolve the commission independently.
type ResolvedCommissionPlan struct {
	CommissionPct    float64
	TotalAmountCents int64
	// RetainedPercentage/RetainedAmountCents são o que a imobiliária retém:
	// a fatia declarada como imobiliária MAIS o resíduo que não foi atribuído a
	// nenhum corretor (o rateio sempre fecha 100%).
	RetainedPercentage  float64
	RetainedAmountCents int64
	Shares              []ResolvedCommissionShare
	PlanHash            string // deterministic fingerprint of resolution inputs
}

// IsZero reports whether the plan has been populated by the pipeline.
// A zero plan means the pipeline did not reach the broker step.
func (p ResolvedCommissionPlan) IsZero() bool {
	return p.TotalAmountCents == 0 && len(p.Shares) == 0 && p.PlanHash == ""
}

// SameHash reports whether two plans were resolved from the same inputs.
// It compares the canonical hash, not field-by-field equality.
func (p ResolvedCommissionPlan) SameHash(other ResolvedCommissionPlan) bool {
	return p.PlanHash != "" && p.PlanHash == other.PlanHash
}

// ResolveCommissionPercentage applies the tenant default only when the deal
// did not provide a percentage. This is the shared rule used by both preview
// and contract.signed materialization.
func ResolveCommissionPercentage(dealType string, configuredPct float64, settings *tenant_financial_settings.Settings) float64 {
	if configuredPct != 0 || settings == nil {
		return configuredPct
	}
	if isRentalDealType(dealType) {
		return settings.DefaultCommissionPctRent
	}
	return settings.DefaultCommissionPctSale
}

// ResolveCommissionPlan turns raw deal/event values into the single snapshot
// consumed by every downstream financial representation. It is intentionally
// pure: no repository reads, logging, audit writes or mutation of the input.
//
// PlanHash is computed BEFORE validation so both the preview and the
// contract.signed materialisation produce the same hash even when the plan
// is later rejected for missing participants — this lets operators compare
// hashes before the deal is fully configured.
func ResolveCommissionPlan(in CommissionPlanInput) (ResolvedCommissionPlan, error) {
	planHash := computePlanHash(in)

	pct := ResolveCommissionPercentage(in.DealType, in.CommissionPct, in.TenantSettings)
	if math.IsNaN(in.DealValue) || math.IsInf(in.DealValue, 0) || in.DealValue < 0 {
		return ResolvedCommissionPlan{}, fmt.Errorf("invalid deal value")
	}
	if math.IsNaN(pct) || math.IsInf(pct, 0) || pct < 0 || pct > 100 {
		return ResolvedCommissionPlan{}, fmt.Errorf("commission percentage must be between 0 and 100")
	}

	dealValueCents := valueobjects.NewMoneyFromFloat(in.DealValue).Cents()
	totalCents := valueobjects.NewMoney(dealValueCents).Percentage(pct).Cents()

	shares := make([]ResolvedCommissionShare, 0, len(in.BrokerParticipations))
	seen := make(map[string]struct{}, len(in.BrokerParticipations))
	var distributedPct float64
	var retainedPct float64
	for _, bp := range in.BrokerParticipations {
		brokerID := strings.TrimSpace(bp.BrokerID)
		role := commission.Role(strings.ToLower(strings.TrimSpace(bp.Role)))
		if !isKnownCommissionRole(role) {
			return ResolvedCommissionPlan{}, fmt.Errorf("invalid commission role: %s", bp.Role)
		}
		if role != commission.RoleImobiliaria && brokerID == "" {
			return ResolvedCommissionPlan{}, fmt.Errorf("broker_id is required for role %s", role)
		}
		key := brokerID + "\x1f" + string(role)
		if _, duplicate := seen[key]; duplicate {
			return ResolvedCommissionPlan{}, fmt.Errorf("duplicate commission participant for broker %q and role %s", brokerID, role)
		}
		seen[key] = struct{}{}

		sharePct := bp.Percentage
		if math.IsNaN(sharePct) || math.IsInf(sharePct, 0) || sharePct < 0 || sharePct > 100 {
			return ResolvedCommissionPlan{}, fmt.Errorf("participant percentage must be between 0 and 100")
		}
		if sharePct == 0 {
			sharePct = commission.PercentageFor(in.Rules, role)
		}
		if role == commission.RoleImobiliaria {
			retainedPct += sharePct
			distributedPct += sharePct
			continue
		}
		distributedPct += sharePct
		shares = append(shares, ResolvedCommissionShare{
			BrokerID:    brokerID,
			Role:        string(role),
			Percentage:  sharePct,
			AmountCents: valueobjects.NewMoney(totalCents).Percentage(sharePct).Cents(),
		})
	}
	// Regra fechada dos dois lados (Configurações › Comissões): a distribuição
	// SEMPRE fecha 100%, e o que não for atribuído a corretor fica retido pela
	// imobiliária. É a mesma leitura da tela de configuração ("os papéis somam
	// X%; o restante é atribuído à imobiliária como receita retida") e o que a
	// criação de negócio no painel já faz.
	//
	// Só a soma ACIMA de 100% continua sendo erro: não existe retenção negativa
	// e nenhum corretor pode receber além do que foi configurado. Nesse caso a
	// materialização falha alto (o operador corrige o rateio) em vez de reduzir
	// silenciosamente o valor de alguém.
	if distributedPct > 100+0.02 {
		return ResolvedCommissionPlan{}, fmt.Errorf("commission distribution must not exceed 100%%: got %.2f%%", distributedPct)
	}
	if math.Abs(distributedPct-100) > 0.02 {
		// O que faltou para 100% é retenção da imobiliária — a soma não é mais
		// usada depois deste ponto, então o reajuste não é reatribuído.
		retainedPct += 100 - distributedPct
	}

	return ResolvedCommissionPlan{
		CommissionPct:       pct,
		TotalAmountCents:    totalCents,
		RetainedPercentage:  retainedPct,
		RetainedAmountCents: valueobjects.NewMoney(totalCents).Percentage(retainedPct).Cents(),
		Shares:              shares,
		PlanHash:            planHash,
	}, nil
}

// computePlanHash produces a deterministic SHA-256 fingerprint of the
// resolution inputs. It includes TenantSettings when available so the
// same deal with different tenant defaults produces a different hash.
// Participants and rules are sorted for determinism. Cross-company and
// franchise royalty inputs are included when present so a partnership deal
// cannot collide with a single-company deal of the same face value.
func computePlanHash(in CommissionPlanInput) string {
	h := sha256.New()

	fmt.Fprintf(h, "v=2\n")
	fmt.Fprintf(h, "deal_type=%s\n", strings.ToLower(strings.TrimSpace(in.DealType)))
	dealValueCents := valueobjects.NewMoneyFromFloat(in.DealValue).Cents()
	fmt.Fprintf(h, "deal_value_cents=%d\n", dealValueCents)
	fmt.Fprintf(h, "commission_pct=%.6f\n", in.CommissionPct)

	// Broker participations in deterministic order.
	type part struct {
		brokerID   string
		role       string
		percentage float64
	}
	parts := make([]part, 0, len(in.BrokerParticipations))
	for _, bp := range in.BrokerParticipations {
		parts = append(parts, part{
			brokerID:   strings.TrimSpace(bp.BrokerID),
			role:       strings.ToLower(strings.TrimSpace(bp.Role)),
			percentage: bp.Percentage,
		})
	}
	sort.Slice(parts, func(i, j int) bool {
		if parts[i].role != parts[j].role {
			return parts[i].role < parts[j].role
		}
		return parts[i].brokerID < parts[j].brokerID
	})
	for _, p := range parts {
		fmt.Fprintf(h, "part=%s|%s|%.6f\n", p.brokerID, p.role, p.percentage)
	}

	// Commission rules in deterministic order.
	type rule struct {
		role       string
		percentage float64
	}
	rules := make([]rule, 0, len(in.Rules))
	for _, r := range in.Rules {
		rules = append(rules, rule{
			role:       string(r.Role),
			percentage: r.DefaultPercentage,
		})
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].role < rules[j].role })
	for _, r := range rules {
		fmt.Fprintf(h, "rule=%s|%.6f\n", r.role, r.percentage)
	}

	// Tenant settings (always included for deterministic hashing).
	// nil settings are treated as zero-valued so the handler path
	// (which passes nil when the service is unavailable) and the
	// preview path (which may pass &Settings{}) produce the same hash.
	if in.TenantSettings == nil {
		in.TenantSettings = &tenant_financial_settings.Settings{}
	}
	fmt.Fprintf(h, "ts_default_commission_pct_rent=%.6f\n", in.TenantSettings.DefaultCommissionPctRent)
	fmt.Fprintf(h, "ts_default_commission_pct_sale=%.6f\n", in.TenantSettings.DefaultCommissionPctSale)
	fmt.Fprintf(h, "ts_admin_fee_pct=%.6f\n", in.TenantSettings.AdminFeePct)
	fmt.Fprintf(h, "ts_default_due_day=%d\n", in.TenantSettings.DefaultDueDay)

	// Cross-company and franchise royalty context, when applicable.
	if in.CrossCompany != nil {
		fmt.Fprintf(h, "cc_owner_pct=%.6f\n", in.CrossCompany.OwnerPercentage)
		fmt.Fprintf(h, "cc_closing_pct=%.6f\n", in.CrossCompany.ClosingPercentage)
		fmt.Fprintf(h, "cc_royalty_pct=%.6f\n", in.CrossCompany.RoyaltyPercentage)
		fmt.Fprintf(h, "cc_royalty_base=%s\n", in.CrossCompany.RoyaltyBaseMode)
	}

	return hex.EncodeToString(h.Sum(nil))
}

func isKnownCommissionRole(role commission.Role) bool {
	switch role {
	case commission.RoleCaptador,
		commission.RoleCoCaptador,
		commission.RoleVisitador,
		commission.RoleAtendente,
		commission.RoleFechamento,
		commission.RoleImobiliaria:
		return true
	default:
		return false
	}
}
