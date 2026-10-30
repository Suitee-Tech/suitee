package rental_charge_config

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type FirstChargeType string

const (
	SameMonthProRata     FirstChargeType = "same_month_pro_rata"
	NextMonthAccumulated FirstChargeType = "next_month_accumulated"
)

// ProRataPolicy selects the convention used to prorate a partial month.
type ProRataPolicy string

const (
	// ProRataPolicyLegacy30_360 is the historical rule (v1): the monthly
	// amount is divided by a fixed 30-day denominator regardless of the real
	// month length. Rows persisted before the policy was versioned resolve to
	// this convention so they stay byte-for-byte reproducible.
	ProRataPolicyLegacy30_360 ProRataPolicy = "legacy_30_360"
	// ProRataPolicyActualActual is the current rule (v2): the numerator is the
	// number of days in effect and the denominator is the real number of days
	// in the month (28/29/30/31), so a partial month never exceeds the full
	// monthly amount.
	ProRataPolicyActualActual ProRataPolicy = "actual_actual"
)

// DefaultProRataPolicy is what New() assigns to freshly created configs.
const DefaultProRataPolicy = ProRataPolicyActualActual

// BundledItem is a fixed extra cost folded into the monthly rent charge
// (condomínio, IPTU, taxa de administração or any tenant-defined custom
// item). The total billed each month is MonthlyAmount + Σ enabled bundled
// items; each appears as a charge.LineItem so the customer sees the
// breakdown on a single consolidated amount. For variable utility costs
// (água, luz por consumo) use UtilityAccount in VARIABLE_SEPARATE mode
// instead — those flow as separate charges.
type BundledItem struct {
	// Key identifies the bundle slot (well-known: condominio, iptu,
	// admin_fee, seguro_fianca; or "custom" for user-defined items).
	Key         string `json:"key"`
	Description string `json:"description"`
	AmountCents int64  `json:"amount_cents"`
	Enabled     bool   `json:"enabled"`
}

// RentAdjustment is an immutable, ordered rental-price revision. Persisting
// the schedule prevents an older broker delivery from reverting a newer
// amount and lets future charges use the value effective for their date.
type RentAdjustment struct {
	EventID       string    `json:"event_id"`
	EffectiveFrom time.Time `json:"effective_from"`
	MonthlyAmount int64     `json:"monthly_amount"`
	OccurredAt    time.Time `json:"occurred_at"`
}

// IntermediationShare freezes one broker slice of the first-rent
// intermediation fee. TargetAmountCents is contractual; FundedAmountCents is
// the portion already backed by collected rent. Keeping both values separate
// lets a pro-rata first charge carry the uncovered balance into later months
// without changing what the broker earned.
type IntermediationShare struct {
	BrokerID          string `json:"broker_id"`
	Role              string `json:"role"`
	CommissionID      string `json:"commission_id,omitempty"`
	PayoutOrderID     string `json:"payout_order_id,omitempty"`
	TargetAmountCents int64  `json:"target_amount_cents"`
	FundedAmountCents int64  `json:"funded_amount_cents"`
}

// IntermediationFundingShare records the exact commission slice financed by
// one rent payment. CommissionID/PayoutOrderID are frozen links to the
// aggregates that become payable once fully funded.
type IntermediationFundingShare struct {
	BrokerID      string `json:"broker_id"`
	Role          string `json:"role"`
	CommissionID  string `json:"commission_id,omitempty"`
	PayoutOrderID string `json:"payout_order_id,omitempty"`
	AmountCents   int64  `json:"amount_cents"`
}

// IntermediationFundingEntry is append-only payment provenance. AvailableCents
// is the rent left after the recurring administration fee; AppliedCents is the
// portion retained as intermediation revenue in this competence.
type IntermediationFundingEntry struct {
	PaymentID             string                       `json:"payment_id"`
	ChargeID              string                       `json:"charge_id"`
	ReferenceMonth        string                       `json:"reference_month"`
	AvailableCents        int64                        `json:"available_cents"`
	AppliedCents          int64                        `json:"applied_cents"`
	AgencyRetainedCents   int64                        `json:"agency_retained_cents"`
	BrokerFunding         []IntermediationFundingShare `json:"broker_funding,omitempty"`
	OutstandingAfterCents int64                        `json:"outstanding_after_cents"`
	FundedAt              time.Time                    `json:"funded_at"`
}

// IntermediationPlan is the immutable first-rent fee target plus its mutable,
// append-only funding progress. Broker shares are funded before the agency's
// retained margin so third-party obligations are covered prudently. The
// recurring administration fee is not part of this plan.
type IntermediationPlan struct {
	DealID                          string                       `json:"deal_id,omitempty"`
	PlanHash                        string                       `json:"plan_hash"`
	TotalAmountCents                int64                        `json:"total_amount_cents"`
	AgencyRetainedTargetCents       int64                        `json:"agency_retained_target_cents"`
	AgencyRetainedFundedAmountCents int64                        `json:"agency_retained_funded_amount_cents"`
	Shares                          []IntermediationShare        `json:"shares,omitempty"`
	FundingEntries                  []IntermediationFundingEntry `json:"funding_entries,omitempty"`
}

type RentalChargeConfig struct {
	ID              string          `json:"id"`
	TenantID        string          `json:"tenant_id"`
	PropertyID      string          `json:"property_id"`
	ContractID      string          `json:"contract_id"`
	TenantPartyID   string          `json:"tenant_party_id"`
	OwnerID         string          `json:"owner_id"`
	StartDate       time.Time       `json:"start_date"`
	EndDate         time.Time       `json:"end_date"`
	FirstChargeDate time.Time       `json:"first_charge_date"`
	FirstChargeType FirstChargeType `json:"first_charge_type"`
	// ProRataPolicy selects the proration convention for the first and last
	// partial months. Empty (legacy persisted rows) resolves to the legacy
	// 30/360 rule; New() writes the current default so fresh configs use
	// actual/actual.
	ProRataPolicy ProRataPolicy `json:"pro_rata_policy,omitempty"`
	DefaultDueDay int           `json:"default_due_day"`
	MonthlyAmount int64         `json:"monthly_amount"`
	// BaseMonthlyAmount is the value before the first scheduled readjustment.
	// Legacy rows leave it zero and fall back to MonthlyAmount.
	BaseMonthlyAmount int64            `json:"base_monthly_amount,omitempty"`
	Adjustments       []RentAdjustment `json:"adjustments,omitempty"`
	FirstChargePaid   bool             `json:"first_charge_paid"`
	// MonthlyProductRevision bumps when the rent is readjusted or bundled
	// items change. It's a plain counter that flows into the source_event_id
	// of regenerated charges so they don't collide on the unique index.
	MonthlyProductRevision int `json:"monthly_product_revision"`
	// BundledItems are fixed extras inlined into the monthly charge. Empty
	// preserves the legacy behavior of a single "Aluguel + Encargos" line.
	// When non-empty, the buildCharges path:
	//   1. computes the monthly total as MonthlyAmount + Σ enabled items,
	//   2. emits each charge with LineItems = [base, ...enabled items].
	BundledItems []BundledItem `json:"bundled_items,omitempty"`
	// AdminFeePercent é a taxa de administração da imobiliária como % do
	// aluguel (ex.: 8 → 8%). Congelada na config para reproduzibilidade; ao
	// gerar as cobranças, o aluguel é dividido em [repasse ao proprietário]
	// + [taxa admin da imobiliária (revenue)] — mesma fatia que o repasse
	// desconta. Sem item fixo `admin_fee` no bundle. 0 = sem taxa (legado).
	AdminFeePercent float64 `json:"admin_fee_percent,omitempty"`
	// IntermediationPlan freezes the one-time first-rent fee and tracks how
	// monthly receipts fund it. Nil means the contract predates this policy or
	// has no intermediation commission.
	IntermediationPlan *IntermediationPlan `json:"intermediation_plan,omitempty"`
	CreatedAt          time.Time           `json:"created_at"`
	UpdatedAt          time.Time           `json:"updated_at"`
}

// UnmarshalJSON keeps rows written before RentalChargeConfig gained explicit
// snake_case tags readable during rolling deployments. New keys always win
// when a payload happens to contain both representations; the legacy value is
// only used when its snake_case counterpart is absent.
func (c *RentalChargeConfig) UnmarshalJSON(data []byte) error {
	type rentalChargeConfigWire RentalChargeConfig

	var decoded rentalChargeConfigWire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	legacyFields := []struct {
		snakeKey  string
		legacyKey string
		target    any
	}{
		{snakeKey: "id", legacyKey: "ID", target: &decoded.ID},
		{snakeKey: "tenant_id", legacyKey: "TenantID", target: &decoded.TenantID},
		{snakeKey: "property_id", legacyKey: "PropertyID", target: &decoded.PropertyID},
		{snakeKey: "contract_id", legacyKey: "ContractID", target: &decoded.ContractID},
		{snakeKey: "tenant_party_id", legacyKey: "TenantPartyID", target: &decoded.TenantPartyID},
		{snakeKey: "owner_id", legacyKey: "OwnerID", target: &decoded.OwnerID},
		{snakeKey: "start_date", legacyKey: "StartDate", target: &decoded.StartDate},
		{snakeKey: "end_date", legacyKey: "EndDate", target: &decoded.EndDate},
		{snakeKey: "first_charge_date", legacyKey: "FirstChargeDate", target: &decoded.FirstChargeDate},
		{snakeKey: "first_charge_type", legacyKey: "FirstChargeType", target: &decoded.FirstChargeType},
		{snakeKey: "pro_rata_policy", legacyKey: "ProRataPolicy", target: &decoded.ProRataPolicy},
		{snakeKey: "default_due_day", legacyKey: "DefaultDueDay", target: &decoded.DefaultDueDay},
		{snakeKey: "monthly_amount", legacyKey: "MonthlyAmount", target: &decoded.MonthlyAmount},
		{snakeKey: "base_monthly_amount", legacyKey: "BaseMonthlyAmount", target: &decoded.BaseMonthlyAmount},
		{snakeKey: "adjustments", legacyKey: "Adjustments", target: &decoded.Adjustments},
		{snakeKey: "first_charge_paid", legacyKey: "FirstChargePaid", target: &decoded.FirstChargePaid},
		{snakeKey: "monthly_product_revision", legacyKey: "MonthlyProductRevision", target: &decoded.MonthlyProductRevision},
		{snakeKey: "bundled_items", legacyKey: "BundledItems", target: &decoded.BundledItems},
		{snakeKey: "intermediation_plan", legacyKey: "IntermediationPlan", target: &decoded.IntermediationPlan},
		{snakeKey: "created_at", legacyKey: "CreatedAt", target: &decoded.CreatedAt},
		{snakeKey: "updated_at", legacyKey: "UpdatedAt", target: &decoded.UpdatedAt},
	}
	for _, field := range legacyFields {
		if _, hasCurrent := raw[field.snakeKey]; hasCurrent {
			continue
		}
		legacyValue, hasLegacy := raw[field.legacyKey]
		if !hasLegacy {
			continue
		}
		if err := json.Unmarshal(legacyValue, field.target); err != nil {
			return fmt.Errorf("decode legacy rental charge config field %s: %w", field.legacyKey, err)
		}
	}

	*c = RentalChargeConfig(decoded)
	return nil
}

// FreezeIntermediationPlan stores the canonical contract-time target exactly
// once. A redelivery with the same plan is a no-op; changing an already frozen
// plan is a financial conflict and must fail the surrounding transaction.
func (c *RentalChargeConfig) FreezeIntermediationPlan(plan IntermediationPlan) (bool, error) {
	plan.DealID = strings.TrimSpace(plan.DealID)
	plan.PlanHash = strings.TrimSpace(plan.PlanHash)
	if plan.TotalAmountCents <= 0 {
		return false, nil
	}
	if plan.PlanHash == "" {
		return false, errors.New("intermediation plan_hash is required")
	}

	shares := append([]IntermediationShare(nil), plan.Shares...)
	sort.SliceStable(shares, func(i, j int) bool {
		leftRole := strings.ToLower(strings.TrimSpace(shares[i].Role))
		rightRole := strings.ToLower(strings.TrimSpace(shares[j].Role))
		if leftRole != rightRole {
			return leftRole < rightRole
		}
		return strings.TrimSpace(shares[i].BrokerID) < strings.TrimSpace(shares[j].BrokerID)
	})
	seen := make(map[string]struct{}, len(shares))
	var brokerTarget int64
	for i := range shares {
		shares[i].BrokerID = strings.TrimSpace(shares[i].BrokerID)
		shares[i].Role = strings.ToLower(strings.TrimSpace(shares[i].Role))
		if shares[i].BrokerID == "" || shares[i].Role == "" {
			return false, errors.New("intermediation broker_id and role are required")
		}
		if shares[i].TargetAmountCents <= 0 {
			return false, errors.New("intermediation share target must be positive")
		}
		if shares[i].FundedAmountCents != 0 || shares[i].CommissionID != "" || shares[i].PayoutOrderID != "" {
			return false, errors.New("new intermediation plan cannot contain funding or aggregate links")
		}
		key := shares[i].BrokerID + "\x1f" + shares[i].Role
		if _, duplicated := seen[key]; duplicated {
			return false, errors.New("duplicated intermediation broker share")
		}
		seen[key] = struct{}{}
		brokerTarget += shares[i].TargetAmountCents
	}
	if brokerTarget > plan.TotalAmountCents {
		return false, errors.New("intermediation broker targets exceed total amount")
	}
	// The residual is canonical. Computing it from integer broker targets
	// conserves cents even when percentage rounding differs by one cent.
	plan.AgencyRetainedTargetCents = plan.TotalAmountCents - brokerTarget
	plan.AgencyRetainedFundedAmountCents = 0
	plan.Shares = shares
	plan.FundingEntries = nil

	if c.IntermediationPlan != nil {
		existing := c.IntermediationPlan
		if existing.PlanHash != plan.PlanHash || existing.DealID != plan.DealID ||
			existing.TotalAmountCents != plan.TotalAmountCents ||
			existing.AgencyRetainedTargetCents != plan.AgencyRetainedTargetCents ||
			!sameIntermediationTargets(existing.Shares, plan.Shares) {
			return false, errors.New("intermediation plan is already frozen with different data")
		}
		return false, nil
	}

	c.IntermediationPlan = &plan
	c.UpdatedAt = time.Now()
	return true, nil
}

func sameIntermediationTargets(left, right []IntermediationShare) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if strings.TrimSpace(left[i].BrokerID) != strings.TrimSpace(right[i].BrokerID) ||
			strings.ToLower(strings.TrimSpace(left[i].Role)) != strings.ToLower(strings.TrimSpace(right[i].Role)) ||
			left[i].TargetAmountCents != right[i].TargetAmountCents {
			return false
		}
	}
	return true
}

// LinkIntermediationCommission freezes the Commission identity created by the
// contract pipeline. Reusing the link is idempotent; pointing the same share to
// another aggregate is rejected.
func (c *RentalChargeConfig) LinkIntermediationCommission(brokerID, role, commissionID string) (bool, error) {
	return c.linkIntermediationAggregate(brokerID, role, commissionID, true)
}

// LinkIntermediationPayout freezes the payout identity that will be released
// when this broker share is fully funded.
func (c *RentalChargeConfig) LinkIntermediationPayout(brokerID, role, payoutOrderID string) (bool, error) {
	return c.linkIntermediationAggregate(brokerID, role, payoutOrderID, false)
}

func (c *RentalChargeConfig) linkIntermediationAggregate(brokerID, role, aggregateID string, commissionLink bool) (bool, error) {
	brokerID = strings.TrimSpace(brokerID)
	role = strings.ToLower(strings.TrimSpace(role))
	aggregateID = strings.TrimSpace(aggregateID)
	if c.IntermediationPlan == nil {
		return false, errors.New("intermediation plan is not frozen")
	}
	if brokerID == "" || role == "" || aggregateID == "" {
		return false, errors.New("intermediation aggregate link requires broker, role and id")
	}
	for i := range c.IntermediationPlan.Shares {
		share := &c.IntermediationPlan.Shares[i]
		if share.BrokerID != brokerID || share.Role != role {
			continue
		}
		current := share.PayoutOrderID
		if commissionLink {
			current = share.CommissionID
		}
		if current == aggregateID {
			return false, nil
		}
		if current != "" {
			return false, errors.New("intermediation aggregate is already linked")
		}
		if commissionLink {
			share.CommissionID = aggregateID
		} else {
			share.PayoutOrderID = aggregateID
		}
		c.UpdatedAt = time.Now()
		return true, nil
	}
	return false, errors.New("intermediation share not found")
}

// IntermediationOutstandingCents returns the contractual target still not
// backed by collected rent.
func (c *RentalChargeConfig) IntermediationOutstandingCents() int64 {
	if c == nil || c.IntermediationPlan == nil {
		return 0
	}
	plan := c.IntermediationPlan
	funded := plan.AgencyRetainedFundedAmountCents
	for _, share := range plan.Shares {
		funded += share.FundedAmountCents
	}
	outstanding := plan.TotalAmountCents - funded
	if outstanding < 0 {
		return 0
	}
	return outstanding
}

// AllocateIntermediationFunding applies one rent payment to the frozen plan.
// Broker shares are funded first and in deterministic order; the agency's
// retained margin receives the residual. The payment ID makes retries a no-op.
func (c *RentalChargeConfig) AllocateIntermediationFunding(paymentID, chargeID, referenceMonth string, availableCents int64, fundedAt time.Time) (IntermediationFundingEntry, bool, error) {
	paymentID = strings.TrimSpace(paymentID)
	chargeID = strings.TrimSpace(chargeID)
	referenceMonth = strings.TrimSpace(referenceMonth)
	// chargeID is provenance for charge payments; a manual receipt funds the
	// plan with an empty chargeID and its own payment ID. The payment ID is the
	// idempotency key in both cases.
	if paymentID == "" {
		return IntermediationFundingEntry{}, false, errors.New("intermediation funding requires payment_id")
	}
	if availableCents < 0 {
		return IntermediationFundingEntry{}, false, errors.New("intermediation available amount cannot be negative")
	}
	if fundedAt.IsZero() {
		return IntermediationFundingEntry{}, false, errors.New("intermediation funded_at is required")
	}
	if c.IntermediationPlan == nil || c.IntermediationPlan.TotalAmountCents <= 0 {
		return IntermediationFundingEntry{}, false, nil
	}
	for _, existing := range c.IntermediationPlan.FundingEntries {
		if existing.PaymentID != paymentID {
			continue
		}
		if existing.ChargeID != chargeID || existing.ReferenceMonth != referenceMonth || existing.AvailableCents != availableCents {
			return IntermediationFundingEntry{}, false, errors.New("intermediation payment_id was reused with different data")
		}
		return existing, false, nil
	}

	outstanding := c.IntermediationOutstandingCents()
	applied := availableCents
	if applied > outstanding {
		applied = outstanding
	}
	if applied <= 0 {
		return IntermediationFundingEntry{}, false, nil
	}

	entry := IntermediationFundingEntry{
		PaymentID: paymentID, ChargeID: chargeID, ReferenceMonth: referenceMonth,
		AvailableCents: availableCents, AppliedCents: applied, FundedAt: fundedAt,
	}
	remaining := applied
	for i := range c.IntermediationPlan.Shares {
		share := &c.IntermediationPlan.Shares[i]
		shareOutstanding := share.TargetAmountCents - share.FundedAmountCents
		if shareOutstanding <= 0 || remaining <= 0 {
			continue
		}
		funded := shareOutstanding
		if funded > remaining {
			funded = remaining
		}
		share.FundedAmountCents += funded
		remaining -= funded
		entry.BrokerFunding = append(entry.BrokerFunding, IntermediationFundingShare{
			BrokerID: share.BrokerID, Role: share.Role, CommissionID: share.CommissionID,
			PayoutOrderID: share.PayoutOrderID, AmountCents: funded,
		})
	}
	if remaining > 0 {
		agencyOutstanding := c.IntermediationPlan.AgencyRetainedTargetCents - c.IntermediationPlan.AgencyRetainedFundedAmountCents
		if agencyOutstanding < remaining {
			return IntermediationFundingEntry{}, false, errors.New("intermediation funding exceeds frozen plan")
		}
		c.IntermediationPlan.AgencyRetainedFundedAmountCents += remaining
		entry.AgencyRetainedCents = remaining
	}
	entry.OutstandingAfterCents = c.IntermediationOutstandingCents()
	c.IntermediationPlan.FundingEntries = append(c.IntermediationPlan.FundingEntries, entry)
	c.UpdatedAt = time.Now()
	return entry, true, nil
}

// TotalMonthlyCents returns the actual cents to bill each month:
// MonthlyAmount + sum of enabled bundled items. Always falls back to
// MonthlyAmount when BundledItems is empty so callers don't branch.
func (c *RentalChargeConfig) TotalMonthlyCents() int64 {
	total := c.MonthlyAmount
	for _, it := range c.BundledItems {
		if it.Enabled {
			total += it.AmountCents
		}
	}
	return total
}

// MonthlyAmountFor resolves the last adjustment effective on asOf. The
// result is deterministic even if persisted events arrived out of order.
func (c *RentalChargeConfig) MonthlyAmountFor(asOf time.Time) int64 {
	amount := c.BaseMonthlyAmount
	if amount <= 0 {
		amount = c.MonthlyAmount
	}
	var selected *RentAdjustment
	for i := range c.Adjustments {
		adj := &c.Adjustments[i]
		if asOf.Before(adj.EffectiveFrom) {
			continue
		}
		if selected == nil || adj.EffectiveFrom.After(selected.EffectiveFrom) ||
			(adj.EffectiveFrom.Equal(selected.EffectiveFrom) && adj.OccurredAt.After(selected.OccurredAt)) {
			selected = adj
		}
	}
	if selected != nil {
		amount = selected.MonthlyAmount
	}
	return amount
}

func (c *RentalChargeConfig) TotalMonthlyCentsFor(asOf time.Time) int64 {
	total := c.MonthlyAmountFor(asOf)
	for _, it := range c.BundledItems {
		if it.Enabled {
			total += it.AmountCents
		}
	}
	return total
}

// ScheduleAdjustment stores a readjustment exactly once. A newer correction
// for the same effective date replaces the older event; a late delivery with
// an older occurred_at is ignored. MonthlyAmount remains the latest scheduled
// value for backward-compatible readers, while MonthlyAmountFor is the source
// of truth for charge generation.
func (c *RentalChargeConfig) ScheduleAdjustment(eventID string, effectiveFrom time.Time, monthlyAmount int64, occurredAt time.Time) (bool, error) {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return false, errors.New("readjustment event_id is required")
	}
	if effectiveFrom.IsZero() {
		return false, errors.New("readjustment effective_from is required")
	}
	if monthlyAmount <= 0 {
		return false, errors.New("readjustment monthly_amount must be greater than zero")
	}
	if occurredAt.IsZero() {
		return false, errors.New("readjustment occurred_at is required")
	}
	for _, adj := range c.Adjustments {
		if adj.EventID != eventID {
			continue
		}
		if adj.MonthlyAmount != monthlyAmount || !adj.EffectiveFrom.Equal(effectiveFrom) {
			return false, fmt.Errorf("readjustment event_id %s was already used with different data", eventID)
		}
		return false, nil
	}

	if c.BaseMonthlyAmount <= 0 {
		c.BaseMonthlyAmount = c.MonthlyAmount
	}
	incoming := RentAdjustment{EventID: eventID, EffectiveFrom: effectiveFrom, MonthlyAmount: monthlyAmount, OccurredAt: occurredAt}
	replaced := false
	for i, adj := range c.Adjustments {
		if !sameCalendarDay(adj.EffectiveFrom, effectiveFrom) {
			continue
		}
		if !occurredAt.After(adj.OccurredAt) {
			return false, nil
		}
		c.Adjustments[i] = incoming
		replaced = true
		break
	}
	if !replaced {
		c.Adjustments = append(c.Adjustments, incoming)
	}
	sort.SliceStable(c.Adjustments, func(i, j int) bool {
		if c.Adjustments[i].EffectiveFrom.Equal(c.Adjustments[j].EffectiveFrom) {
			return c.Adjustments[i].OccurredAt.Before(c.Adjustments[j].OccurredAt)
		}
		return c.Adjustments[i].EffectiveFrom.Before(c.Adjustments[j].EffectiveFrom)
	})
	c.MonthlyAmount = c.Adjustments[len(c.Adjustments)-1].MonthlyAmount
	c.MonthlyProductRevision++
	c.UpdatedAt = time.Now()
	return true, nil
}

func sameCalendarDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// EnabledBundledItems returns only the items currently active. The order
// is preserved so the UI can render them deterministically.
func (c *RentalChargeConfig) EnabledBundledItems() []BundledItem {
	out := make([]BundledItem, 0, len(c.BundledItems))
	for _, it := range c.BundledItems {
		if it.Enabled {
			out = append(out, it)
		}
	}
	return out
}

func New(tenantID, propertyID, contractID, tenantPartyID, ownerID string, startDate, endDate, firstChargeDate time.Time, firstChargeType FirstChargeType, defaultDueDay int, monthlyAmount int64) (*RentalChargeConfig, error) {
	if tenantID == "" || contractID == "" {
		return nil, errors.New("tenant_id and contract_id are required")
	}
	if propertyID == "" || tenantPartyID == "" || ownerID == "" {
		return nil, errors.New("property_id, tenant_party_id and owner_id are required")
	}
	if defaultDueDay < 1 || defaultDueDay > 31 {
		return nil, errors.New("default_due_day must be between 1 and 31")
	}
	if monthlyAmount <= 0 {
		return nil, errors.New("monthly_amount must be greater than zero")
	}
	if endDate.Before(startDate) {
		return nil, errors.New("end_date must be >= start_date")
	}
	if startDate.AddDate(0, 1, 0).After(endDate) {
		return nil, errors.New("contract term must be at least 1 month")
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if endDate.Before(today) {
		return nil, errors.New("end_date must be in the future")
	}
	if err := ValidateFirstChargeType(firstChargeType); err != nil {
		return nil, err
	}

	return &RentalChargeConfig{
		ID:              uuid.New().String(),
		TenantID:        tenantID,
		PropertyID:      propertyID,
		ContractID:      contractID,
		TenantPartyID:   tenantPartyID,
		OwnerID:         ownerID,
		StartDate:       startDate,
		EndDate:         endDate,
		FirstChargeDate: firstChargeDate,
		FirstChargeType: firstChargeType,
		ProRataPolicy:   DefaultProRataPolicy,
		DefaultDueDay:   defaultDueDay,
		MonthlyAmount:   monthlyAmount,
		FirstChargePaid: false,
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil
}

func ValidateFirstChargeType(t FirstChargeType) error {
	if t == SameMonthProRata || t == NextMonthAccumulated {
		return nil
	}
	return errors.New("invalid first_charge_type")
}

func (c *RentalChargeConfig) CanBeEdited() bool {
	return !c.FirstChargePaid
}

func (c *RentalChargeConfig) GetTermInMonths() int {
	if c.EndDate.Before(c.StartDate) {
		return 0
	}
	sy, sm, sd := c.StartDate.Date()
	ey, em, ed := c.EndDate.Date()
	months := (ey-sy)*12 + int(em-sm)
	if ed < sd {
		months--
	}
	if months < 0 {
		return 0
	}
	return months
}

func CalculateFirstChargeAmount(startDate, endDate time.Time, monthlyAmount int64, chargeType FirstChargeType, policy ProRataPolicy) (int64, error) {
	if monthlyAmount <= 0 {
		return 0, errors.New("monthly_amount must be greater than zero")
	}
	if err := ValidateFirstChargeType(chargeType); err != nil {
		return 0, err
	}

	first := CalculateProRataAmount(startDate, monthlyAmount, policy)
	if chargeType == SameMonthProRata {
		return first, nil
	}

	// NextMonthAccumulated: prorated start month plus the following month.
	// When the contract ends inside that following month, only the occupied
	// days are charged so the last month is prorated like the first.
	next := monthlyAmount
	if !endDate.IsZero() {
		nextStart := startOfMonth(startDate).AddDate(0, 1, 0)
		nextEnd := endOfMonth(nextStart)
		if !endDate.After(nextEnd) {
			next = CalculateLastMonthAmount(endDate, monthlyAmount, policy)
		}
	}
	return first + next, nil
}

// CalculateProRataAmount computes the prorated cents for a contract that
// starts on startDate and runs through the end of that calendar month
// (inclusive). The proration rule is selected by policy.
func CalculateProRataAmount(startDate time.Time, monthlyAmount int64, policy ProRataPolicy) int64 {
	if monthlyAmount <= 0 {
		return 0
	}
	days := endOfMonth(startDate).Day() - startDate.Day() + 1
	if days < 0 {
		return 0
	}
	return proRataCents(days, daysInMonth(startDate), monthlyAmount, policy)
}

// CalculateLastMonthAmount computes the amount due for the final calendar
// month when the contract ends on endDate (inclusive). A full month is
// charged only when endDate falls on the month's last day; otherwise the
// occupied days are prorated with the same policy as the first month.
func CalculateLastMonthAmount(endDate time.Time, monthlyAmount int64, policy ProRataPolicy) int64 {
	if monthlyAmount <= 0 {
		return 0
	}
	days := endDate.Day()
	if days <= 0 {
		return 0
	}
	if days >= daysInMonth(endDate) {
		return monthlyAmount
	}
	return proRataCents(days, daysInMonth(endDate), monthlyAmount, policy)
}

// proRataCents scales monthlyAmount by days/denominator using integer floor
// division so the result is a deterministic cent amount. The legacy 30/360
// convention fixes the denominator at 30; actual/actual uses the real number
// of days in the month.
func proRataCents(days, daysInMonth int, monthlyAmount int64, policy ProRataPolicy) int64 {
	denominator := int64(daysInMonth)
	if resolveProRataPolicy(policy) == ProRataPolicyLegacy30_360 {
		denominator = 30
	}
	if denominator <= 0 || days <= 0 {
		return 0
	}
	return (monthlyAmount * int64(days)) / denominator
}

// resolveProRataPolicy returns the effective policy, treating the empty value
// (legacy persisted rows) as the legacy 30/360 convention.
func resolveProRataPolicy(p ProRataPolicy) ProRataPolicy {
	if p == "" {
		return ProRataPolicyLegacy30_360
	}
	return p
}

func daysInMonth(d time.Time) int {
	return endOfMonth(d).Day()
}

func startOfMonth(d time.Time) time.Time {
	return time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, d.Location())
}

func endOfMonth(d time.Time) time.Time {
	return time.Date(d.Year(), d.Month()+1, 0, 0, 0, 0, 0, d.Location())
}
