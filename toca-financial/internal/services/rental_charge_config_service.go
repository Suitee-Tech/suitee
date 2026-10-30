package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/domain/valueobjects"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental_charge_config"
)

type RentalChargeConfigService struct {
	repo       rental_charge_config.Repository
	chargeRepo rentalChargeConfigChargeRepo
	crmClient  chargePartyLookup
	dealClient *clients.DealClient
	// tenantSettings (opcional) é consultado quando o caller passa
	// defaultDueDay = 0 — o tenant default substitui o "fallback ao dia
	// do startDate". Sem ele, mantém o comportamento legado.
	tenantSettings *TenantFinancialSettingsService
}

const defaultLeaseTermMonths = 30

type rentalChargeConfigChargeRepo interface {
	Create(ctx context.Context, c *charge.Charge) error
	Update(ctx context.Context, c *charge.Charge) error
	List(ctx context.Context, tenantID, contractID, status, referenceMonth string, limit, offset int64) ([]*charge.Charge, int64, error)
}

func NewRentalChargeConfigService(repo rental_charge_config.Repository, chargeRepo rentalChargeConfigChargeRepo, dealClient *clients.DealClient) *RentalChargeConfigService {
	return &RentalChargeConfigService{repo: repo, chargeRepo: chargeRepo, dealClient: dealClient}
}

// SetCrmClient wires the lead lookup used to resolve the payer. Nil-safe.
func (s *RentalChargeConfigService) SetCrmClient(crm chargePartyLookup) {
	s.crmClient = crm
}

// SetTenantSettings wires o tenantFinancialSettingsService — quando o
// caller passa defaultDueDay = 0 (ex.: contract.signed sem due_day no
// payload), o service consulta o tenant default antes do fallback global.
func (s *RentalChargeConfigService) SetTenantSettings(t *TenantFinancialSettingsService) {
	s.tenantSettings = t
}

func (s *RentalChargeConfigService) CreateAndGenerateCharges(
	ctx context.Context,
	contractID, tenantID, tenantPartyID, ownerID, propertyID string,
	startDate, endDate time.Time,
	firstChargeDate time.Time,
	firstChargeType rental_charge_config.FirstChargeType,
	defaultDueDay int,
	monthlyAmount int64,
) (*rental_charge_config.RentalChargeConfig, int, error) {
	startDate, endDate = s.normalizeWithContract(ctx, tenantID, contractID, startDate, endDate)
	// Substitui defaultDueDay zerado pelo tenant default — quando o caller
	// (ex.: contract.signed sem due_day no payload) não consegue resolver,
	// o tenant default vence sobre o fallback "dia do startDate".
	if defaultDueDay == 0 && s.tenantSettings != nil {
		ts, err := s.tenantSettings.Get(ctx, tenantID)
		if err != nil {
			return nil, 0, fmt.Errorf("load tenant financial settings: %w", err)
		}
		if ts != nil && ts.DefaultDueDay > 0 {
			defaultDueDay = ts.DefaultDueDay
		}
	}
	return s.createAndGenerate(ctx, contractID, tenantID, tenantPartyID, ownerID, propertyID, startDate, endDate, firstChargeDate, firstChargeType, defaultDueDay, monthlyAmount)
}

// CreateConfigOnly persiste a configuração de cobrança do aluguel
// (cronograma / base de controle) SEM materializar nenhuma Charge.
// Usado pelo fluxo de provisionamento manual em contract.signed: a
// imobiliária registra o que será devido mensalmente mas não emite
// cobrança ao inquilino (sem gateway). Idempotente por contrato — se já
// existe config, devolve a existente. A geração de charges segue
// disponível depois via GenerateChargesByID, caso a cobrança seja
// reativada.
func (s *RentalChargeConfigService) CreateConfigOnly(
	ctx context.Context,
	contractID, tenantID, tenantPartyID, ownerID, propertyID string,
	startDate, endDate time.Time,
	firstChargeDate time.Time,
	firstChargeType rental_charge_config.FirstChargeType,
	defaultDueDay int,
	monthlyAmount int64,
) (*rental_charge_config.RentalChargeConfig, error) {
	startDate, endDate = s.normalizeWithContract(ctx, tenantID, contractID, startDate, endDate)
	if defaultDueDay == 0 && s.tenantSettings != nil {
		ts, err := s.tenantSettings.Get(ctx, tenantID)
		if err != nil {
			return nil, fmt.Errorf("load tenant financial settings: %w", err)
		}
		if ts != nil && ts.DefaultDueDay > 0 {
			defaultDueDay = ts.DefaultDueDay
		}
	}
	existingCfg, err := s.repo.GetByContractID(ctx, tenantID, contractID)
	if err != nil {
		return nil, err
	}
	if existingCfg != nil {
		return existingCfg, nil
	}
	cfg, err := rental_charge_config.New(tenantID, propertyID, contractID, tenantPartyID, ownerID, startDate, endDate, firstChargeDate, firstChargeType, defaultDueDay, monthlyAmount)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, cfg); err != nil {
		if !errors.Is(err, rental_charge_config.ErrAlreadyExists) {
			return nil, err
		}
		// Another delivery may win between the lookup above and the insert.
		// The database unique index is the arbiter; return the canonical row
		// instead of failing an otherwise idempotent contract.signed retry.
		canonical, getErr := s.repo.GetByContractID(ctx, tenantID, contractID)
		if getErr != nil {
			return nil, getErr
		}
		if canonical == nil {
			return nil, fmt.Errorf("rental charge config conflicted but canonical row was not found")
		}
		return canonical, nil
	}
	return cfg, nil
}

func (s *RentalChargeConfigService) UpdateConfig(ctx context.Context, tenantID, configID string, firstChargeDate time.Time, defaultDueDay int) (*rental_charge_config.RentalChargeConfig, error) {
	cfg, err := s.repo.GetByID(ctx, tenantID, configID)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, fmt.Errorf("rental charge config not found")
	}
	if !cfg.CanBeEdited() {
		return nil, fmt.Errorf("first charge already paid")
	}
	if defaultDueDay < 1 || defaultDueDay > 31 {
		return nil, errors.New("default_due_day must be between 1 and 31")
	}

	oldFirstChargeDate := cfg.FirstChargeDate
	cfg.FirstChargeDate = firstChargeDate
	cfg.DefaultDueDay = defaultDueDay

	if err := s.repo.Update(ctx, cfg); err != nil {
		return nil, err
	}
	if err := s.updateChargeDates(ctx, cfg, oldFirstChargeDate); err != nil {
		return nil, err
	}
	return cfg, nil
}

// UpdateBundledItems replaces the BundledItems slice on the config, bumps the
// revision counter and re-generates every PENDING charge whose due date
// is in the future. Charges already PAID or PARTIALLY_PAID are left
// untouched; CANCELLED ones too. The new revision flows into the
// source_event_id of regenerated charges so they don't collide with
// previously-emitted rows on the unique index.
func (s *RentalChargeConfigService) UpdateBundledItems(ctx context.Context, tenantID, configID string, items []rental_charge_config.BundledItem) (*rental_charge_config.RentalChargeConfig, int, error) {
	cfg, err := s.repo.GetByID(ctx, tenantID, configID)
	if err != nil {
		return nil, 0, err
	}
	if cfg == nil {
		return nil, 0, fmt.Errorf("rental charge config not found")
	}
	cfg.BundledItems = items
	cfg.MonthlyProductRevision++
	if err := s.repo.Update(ctx, cfg); err != nil {
		return nil, 0, err
	}
	created, err := s.RegenerateFutureCharges(ctx, cfg)
	return cfg, created, err
}

// RegenerateFutureCharges cancels every PENDING charge of the config that
// has a future due date and re-emits a fresh charge for the same reference
// month with the current configuration (including bundled items). Returns
// how many new charges were created.
func (s *RentalChargeConfigService) RegenerateFutureCharges(ctx context.Context, cfg *rental_charge_config.RentalChargeConfig) (int, error) {
	return s.RegenerateChargesFrom(ctx, cfg, time.Now())
}

// RegenerateChargesFrom reapplies the scheduled price to existing PENDING
// charges on/after effectiveFrom. Rows are updated in place so a mid-loop
// failure is safely retryable; cancel+recreate could strand a cancelled row
// behind the unique source_event_id constraint.
func (s *RentalChargeConfigService) RegenerateChargesFrom(ctx context.Context, cfg *rental_charge_config.RentalChargeConfig, effectiveFrom time.Time) (int, error) {
	existing, err := s.listAllCharges(ctx, cfg.TenantID, cfg.ContractID)
	if err != nil {
		return 0, err
	}

	rebuilt, err := s.buildCharges(ctx, cfg)
	if err != nil {
		return 0, err
	}
	byReference := make(map[string]*charge.Charge, len(rebuilt))
	for _, ch := range rebuilt {
		byReference[ch.ReferenceMonth] = ch
	}
	updated := 0
	for _, current := range existing {
		if current.Status != charge.StatusPending || current.DueDate.Before(effectiveFrom) {
			continue
		}
		next := byReference[current.ReferenceMonth]
		if next == nil {
			continue
		}
		// Itens que não vêm do plano (ex.: manutenção somada na próxima
		// cobrança do inquilino) precisam sobreviver à regravação — senão o
		// reajuste apaga a obrigação do financeiro e o total volta ao valor
		// do plano.
		planItems, extraItems, extraCents := splitPlanAndExtraItems(current.LineItems, next.LineItems)
		nextAmount := valueobjects.NewMoney(next.Amount.Cents() + extraCents)
		nextLineItems := append(append([]charge.LineItem(nil), planItems...), extraItems...)

		if current.Description == next.Description &&
			current.Amount.Cents() == nextAmount.Cents() &&
			current.TotalAmount.Cents() == next.TotalAmount.Cents() &&
			current.DueDate.Equal(next.DueDate) &&
			current.SourceEventID == next.SourceEventID &&
			reflect.DeepEqual(current.LineItems, nextLineItems) {
			continue
		}
		current.Description = next.Description
		current.Amount = nextAmount
		current.LateFee = next.LateFee
		current.Interest = next.Interest
		current.Discount = next.Discount
		current.DueDate = next.DueDate
		current.LineItems = nextLineItems
		current.SourceEventID = next.SourceEventID
		current.CalculateTotal()
		current.UpdatedAt = time.Now()
		if err := s.chargeRepo.Update(ctx, current); err != nil {
			return 0, err
		}
		updated++
	}
	return updated, nil
}

// rentalPlanSourceTypes identifica os itens que o plano-base recria em cada
// regravação. Item com SourceType fora desta lista (e não vazio) é obrigação
// somada depois — manutenção, por exemplo — e é preservado.
var rentalPlanSourceTypes = map[string]bool{
	"rental_base":         true,
	"rental_bundled_item": true,
}

// splitPlanAndExtraItems separa a composição regravada pelo plano dos itens
// que não pertencem a ele. Devolve os itens do plano (na ordem do plano), os
// extras preservados e a soma dos extras, para o total da cobrança fechar com
// a soma dos itens (invariante validado por SetLineItems).
func splitPlanAndExtraItems(current, plan []charge.LineItem) ([]charge.LineItem, []charge.LineItem, int64) {
	planItems := append([]charge.LineItem(nil), plan...)
	var extras []charge.LineItem
	var extraCents int64
	for _, item := range current {
		if item.SourceType == "" || rentalPlanSourceTypes[item.SourceType] {
			continue
		}
		extras = append(extras, item)
		extraCents += item.AmountCents
	}
	return planItems, extras, extraCents
}

// ApplyReadjustment persists an ordered, idempotent schedule entry and
// recalculates only unpaid charges covered by its effective date.
func (s *RentalChargeConfigService) ApplyReadjustment(ctx context.Context, tenantID, contractID, eventID string, newCents int64, effectiveFrom, occurredAt time.Time) (*rental_charge_config.RentalChargeConfig, int, error) {
	cfg, err := s.repo.GetByContractID(ctx, tenantID, contractID)
	if err != nil {
		return nil, 0, err
	}
	if cfg == nil {
		return nil, 0, nil
	}
	changed, err := cfg.ScheduleAdjustment(eventID, effectiveFrom, newCents, occurredAt)
	if err != nil {
		return nil, 0, err
	}
	if changed {
		if err := s.repo.Update(ctx, cfg); err != nil {
			return nil, 0, err
		}
	}
	// A duplicate delivery still runs the idempotent repair. This matters if a
	// previous attempt persisted the schedule but failed midway through charge
	// updates; retry must converge the remaining rows.
	updated, err := s.RegenerateChargesFrom(ctx, cfg, effectiveFrom)
	return cfg, updated, err
}

func (s *RentalChargeConfigService) MarkFirstChargePaid(ctx context.Context, tenantID, configID string) error {
	cfg, err := s.repo.GetByID(ctx, tenantID, configID)
	if err != nil {
		return err
	}
	if cfg == nil {
		return fmt.Errorf("rental charge config not found")
	}
	if cfg.FirstChargePaid {
		return nil
	}
	cfg.FirstChargePaid = true
	return s.repo.Update(ctx, cfg)
}

func (s *RentalChargeConfigService) GetByID(ctx context.Context, tenantID, id string) (*rental_charge_config.RentalChargeConfig, error) {
	return s.repo.GetByID(ctx, tenantID, id)
}

func (s *RentalChargeConfigService) GetByContractID(ctx context.Context, tenantID, contractID string) (*rental_charge_config.RentalChargeConfig, error) {
	return s.repo.GetByContractID(ctx, tenantID, contractID)
}

// BuildChargesPreview produces, in memory, the exact list of charges that
// would be created by CreateAndGenerateCharges for the given inputs — but
// without persisting, without contacting the gateway, and without
// touching the repository. Reused by FinancialPreviewService to render
// the "Financeiro (Prévia)" tab in the esteira.
//
// Each returned *charge.Charge carries the same SourceEventID it would
// receive in production (rental.<previewID>.first.r0 /
// rental.<previewID>.month.<YYYY-MM>.r0) so the preview matches the
// materialization 1:1 — useful for tests and for the UI to display a
// stable identifier per row across re-renders.
func (s *RentalChargeConfigService) BuildChargesPreview(
	ctx context.Context,
	contractID, tenantID, tenantPartyID, ownerID, propertyID string,
	startDate, endDate time.Time,
	firstChargeDate time.Time,
	firstChargeType rental_charge_config.FirstChargeType,
	defaultDueDay int,
	monthlyCents int64,
	bundledItems []rental_charge_config.BundledItem,
) ([]*charge.Charge, error) {
	cfg := &rental_charge_config.RentalChargeConfig{
		ID:              "preview",
		TenantID:        tenantID,
		PropertyID:      propertyID,
		ContractID:      contractID,
		TenantPartyID:   tenantPartyID,
		OwnerID:         ownerID,
		StartDate:       startDate,
		EndDate:         endDate,
		FirstChargeDate: firstChargeDate,
		FirstChargeType: firstChargeType,
		ProRataPolicy:   rental_charge_config.DefaultProRataPolicy,
		DefaultDueDay:   defaultDueDay,
		MonthlyAmount:   monthlyCents,
		BundledItems:    bundledItems,
	}
	return s.buildCharges(ctx, cfg)
}

func (s *RentalChargeConfigService) GenerateChargesByID(ctx context.Context, tenantID, configID string) (int, error) {
	cfg, err := s.repo.GetByID(ctx, tenantID, configID)
	if err != nil {
		return 0, err
	}
	if cfg == nil {
		return 0, fmt.Errorf("rental charge config not found")
	}
	_, created, err := s.CreateAndGenerateCharges(ctx, cfg.ContractID, cfg.TenantID, cfg.TenantPartyID, cfg.OwnerID, cfg.PropertyID, cfg.StartDate, cfg.EndDate, cfg.FirstChargeDate, cfg.FirstChargeType, cfg.DefaultDueDay, cfg.MonthlyAmount)
	return created, err
}

func (s *RentalChargeConfigService) createAndGenerate(
	ctx context.Context,
	contractID, tenantID, tenantPartyID, ownerID, propertyID string,
	startDate, endDate time.Time,
	firstChargeDate time.Time,
	firstChargeType rental_charge_config.FirstChargeType,
	defaultDueDay int,
	monthlyAmount int64,
) (*rental_charge_config.RentalChargeConfig, int, error) {
	existingCfg, err := s.repo.GetByContractID(ctx, tenantID, contractID)
	if err != nil {
		return nil, 0, err
	}

	if existingCfg != nil {
		if err := s.ValidateContractConsistency(ctx, existingCfg); err != nil {
			logger.FromContext(ctx).Warn("contract/config mismatch", "error", err, "contract_id", contractID)
		}
	}

	cfg := existingCfg
	if cfg == nil {
		cfg, err = rental_charge_config.New(tenantID, propertyID, contractID, tenantPartyID, ownerID, startDate, endDate, firstChargeDate, firstChargeType, defaultDueDay, monthlyAmount)
		if err != nil {
			return nil, 0, err
		}
		if err := s.repo.Create(ctx, cfg); err != nil {
			if !errors.Is(err, rental_charge_config.ErrAlreadyExists) {
				return nil, 0, err
			}
			cfg, err = s.repo.GetByContractID(ctx, tenantID, contractID)
			if err != nil {
				return nil, 0, err
			}
			if cfg == nil {
				return nil, 0, fmt.Errorf("rental charge config conflicted but canonical row was not found")
			}
		}
	}

	if err := s.ValidateContractConsistency(ctx, cfg); err != nil {
		logger.FromContext(ctx).Warn("contract/config mismatch", "error", err, "contract_id", contractID)
	}

	charges, err := s.buildCharges(ctx, cfg)
	if err != nil {
		return nil, 0, err
	}
	// Cada charge carrega um source_event_id determinístico por competência
	// (reference month). Uma reentrega salta as linhas já existentes em vez de
	// abortar, de modo que uma falha no meio do loop é reparada na próxima
	// entrega sem duplicar o calendário.
	created := 0
	for _, ch := range charges {
		inserted, err := s.createChargeIfAbsent(ctx, ch)
		if err != nil {
			return nil, 0, err
		}
		if inserted {
			created++
		}
	}
	return cfg, created, nil
}

func (s *RentalChargeConfigService) buildCharges(ctx context.Context, cfg *rental_charge_config.RentalChargeConfig) ([]*charge.Charge, error) {
	// Taxa de administração da imobiliária: usa a congelada na config; se a
	// config (legada) não a tem, resolve do tenant settings para que as LINHAS
	// da cobrança já reflitam a fatia da imobiliária — consistente com a
	// alocação do pagamento e com o repasse ao proprietário.
	adminFeePct := cfg.AdminFeePercent
	if adminFeePct <= 0 && s.tenantSettings != nil {
		if ts, err := s.tenantSettings.Get(ctx, cfg.TenantID); err == nil && ts != nil {
			adminFeePct = ts.AdminFeePct
		}
	}

	firstMonthly := cfg.TotalMonthlyCentsFor(cfg.StartDate)
	firstAmount, err := rental_charge_config.CalculateFirstChargeAmount(cfg.StartDate, cfg.EndDate, firstMonthly, cfg.FirstChargeType, cfg.ProRataPolicy)
	if err != nil {
		return nil, err
	}

	startMonth := time.Date(cfg.StartDate.Year(), cfg.StartDate.Month(), 1, 0, 0, 0, 0, cfg.StartDate.Location())
	firstRefMonth := startMonth
	if cfg.FirstChargeType == rental_charge_config.NextMonthAccumulated {
		firstRefMonth = startMonth.AddDate(0, 1, 0)
	}

	desc := "Aluguel + Encargos"
	firstLineItems := buildBundledLineItems(cfg, firstAmount, firstMonthly, adminFeePct)
	firstCharge, err := charge.NewWithLineItems(cfg.ContractID, cfg.TenantPartyID, cfg.OwnerID, cfg.PropertyID, cfg.TenantID, charge.TypeRent, desc, monthKey(firstRefMonth), centsToFloat(firstAmount), cfg.FirstChargeDate, firstLineItems)
	if err != nil {
		return nil, err
	}
	firstCharge.SourceEventID = fmt.Sprintf("rental.%s.first.r%d", cfg.ID, cfg.MonthlyProductRevision)

	var out []*charge.Charge
	out = append(out, firstCharge)

	lastMonth := time.Date(cfg.EndDate.Year(), cfg.EndDate.Month(), 1, 0, 0, 0, 0, cfg.EndDate.Location())
	nextMonth := firstRefMonth.AddDate(0, 1, 0)
	for m := nextMonth; !m.After(lastMonth); m = m.AddDate(0, 1, 0) {
		due := setDayOfMonth(m.Year(), m.Month(), cfg.DefaultDueDay, cfg.StartDate.Location())
		totalMonthly := cfg.TotalMonthlyCentsFor(due)
		chargeAmount := totalMonthly
		if sameMonth(m, cfg.EndDate) {
			chargeAmount = rental_charge_config.CalculateLastMonthAmount(cfg.EndDate, totalMonthly, cfg.ProRataPolicy)
		}
		monthlyLineItems := buildBundledLineItems(cfg, chargeAmount, totalMonthly, adminFeePct)
		ch, err := charge.NewWithLineItems(cfg.ContractID, cfg.TenantPartyID, cfg.OwnerID, cfg.PropertyID, cfg.TenantID, charge.TypeRent, desc, monthKey(m), centsToFloat(chargeAmount), due, monthlyLineItems)
		if err != nil {
			return nil, err
		}
		ch.SourceEventID = fmt.Sprintf("rental.%s.month.%s.r%d", cfg.ID, monthKey(m), cfg.MonthlyProductRevision)
		out = append(out, ch)
	}

	return out, nil
}

// buildBundledLineItems splits a charge total into discriminated economic
// items (base rent + each enabled bundled item). The base item is emitted even
// without extras: new RENT charges must carry enough information for payment
// allocation, DRE and owner balance to derive the same result. chargeCents is
// the actual charge total (it may differ from monthlyTotal in a pro-rata
// month); the rounding residual is absorbed by the base item.
//
// adminFeePct (>0) divide o aluguel em [repasse ao proprietário] + [taxa de
// administração da imobiliária (revenue)] — MESMA fatia que a alocação do
// pagamento e o repasse aplicam, então o valor da imobiliária aparece já na
// composição da cobrança (e não só após a baixa). O total cobrado não muda.
func buildBundledLineItems(cfg *rental_charge_config.RentalChargeConfig, chargeCents, monthlyTotal int64, adminFeePct float64) []charge.LineItem {
	enabled := cfg.EnabledBundledItems()
	if monthlyTotal == 0 {
		return nil
	}
	ruleVersionID := rentalChargeRuleVersionID(cfg)
	items := make([]charge.LineItem, 0, len(enabled)+2)
	var allocated int64
	// Each bundled item scales with the charge/monthly ratio so a half-month
	// pro-rata first charge ends up with half-condomínio and half-rent.
	for _, it := range enabled {
		scaled := (it.AmountCents * chargeCents) / monthlyTotal
		if scaled <= 0 {
			continue
		}
		ownerID, mode := bundledItemSettlement(cfg, it.Key)
		items = append(items, charge.LineItem{
			Description:     it.Description,
			AmountCents:     scaled,
			SourceType:      "rental_bundled_item",
			SourceID:        it.Key,
			EconomicOwnerID: ownerID,
			SettlementMode:  mode,
			RuleVersionID:   ruleVersionID,
		})
		allocated += scaled
	}
	baseAmount := chargeCents - allocated
	if baseAmount <= 0 {
		baseAmount = 0
	}

	// Fatiamento da taxa de administração (se configurada e sem item fixo
	// admin_fee explícito no bundle).
	hasExplicitAdmin := false
	for _, it := range enabled {
		if strings.EqualFold(strings.TrimSpace(it.Key), "admin_fee") {
			hasExplicitAdmin = true
			break
		}
	}
	adminCents := int64(0)
	if adminFeePct > 0 && !hasExplicitAdmin {
		adminCents = int64(math.Round(float64(baseAmount) * adminFeePct / 100))
		if adminCents < 0 {
			adminCents = 0
		}
		if adminCents > baseAmount {
			adminCents = baseAmount
		}
	}
	ownerRentCents := baseAmount - adminCents

	baseItems := make([]charge.LineItem, 0, 2)
	baseItems = append(baseItems, charge.LineItem{
		Description:     "Aluguel",
		AmountCents:     ownerRentCents,
		SourceType:      "rental_base",
		SourceID:        cfg.ID,
		EconomicOwnerID: cfg.OwnerID,
		SettlementMode:  charge.SettlementModeCollection,
		RuleVersionID:   ruleVersionID,
	})
	if adminCents > 0 {
		baseItems = append(baseItems, charge.LineItem{
			Description:     "Taxa de administração",
			AmountCents:     adminCents,
			SourceType:      "rental_bundled_item",
			SourceID:        "admin_fee",
			EconomicOwnerID: cfg.TenantID,
			SettlementMode:  charge.SettlementModeCollection,
			AllocationType:  charge.AllocationTypeRevenue,
			RuleVersionID:   ruleVersionID,
		})
	}
	// Prepend the base rent so the customer always sees "Aluguel" first.
	return append(baseItems, items...)
}

func rentalChargeRuleVersionID(cfg *rental_charge_config.RentalChargeConfig) string {
	key := fmt.Sprintf("toca/rental-charge-rule/v1|%s|%d", strings.TrimSpace(cfg.ID), cfg.MonthlyProductRevision)
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(key)).String()
}

// bundledItemSettlement maps a well-known bundled-item key to the economic
// owner and settlement mode of that component (seção 9.4 do compêndio). The
// base rent and owner-borne charges (condomínio, IPTU) settle to the property
// owner; the imobiliária's own admin fee settles to the tenant (a imobiliária
// em si, identificada pelo TenantID do config).
func bundledItemSettlement(cfg *rental_charge_config.RentalChargeConfig, key string) (ownerID string, mode charge.SettlementMode) {
	switch key {
	case "condominio", "iptu":
		return cfg.OwnerID, charge.SettlementModeDirectLiquidation
	case "admin_fee":
		return cfg.TenantID, charge.SettlementModeCollection
	default:
		return cfg.OwnerID, charge.SettlementModeCollection
	}
}

func (s *RentalChargeConfigService) ValidateContractConsistency(ctx context.Context, cfg *rental_charge_config.RentalChargeConfig) error {
	if s.dealClient == nil || cfg == nil {
		return nil
	}

	c, err := s.dealClient.GetContract(ctx, cfg.TenantID, cfg.ContractID)
	if err != nil {
		return err
	}

	contractType := strings.ToLower(strings.TrimSpace(c.Type))
	if contractType != "locacao" && contractType != "locação" {
		return fmt.Errorf("contract type is not locacao: %s", c.Type)
	}

	contractStart := c.StartDate
	contractEnd := c.EndDate
	if contractEnd == nil {
		d := contractStart.AddDate(0, defaultLeaseTermMonths, 0)
		contractEnd = &d
	}

	updated := false
	if !sameDay(cfg.StartDate, contractStart) {
		cfg.StartDate = contractStart
		updated = true
	}
	if contractEnd != nil && !sameDay(cfg.EndDate, *contractEnd) {
		cfg.EndDate = *contractEnd
		updated = true
	}

	if updated {
		if err := s.repo.Update(ctx, cfg); err != nil {
			return err
		}
		return fmt.Errorf("config dates adjusted to match contract")
	}

	return nil
}

func (s *RentalChargeConfigService) normalizeWithContract(ctx context.Context, tenantID, contractID string, startDate, endDate time.Time) (time.Time, time.Time) {
	if s.dealClient == nil {
		if endDate.IsZero() {
			endDate = startDate.AddDate(0, defaultLeaseTermMonths, 0)
		}
		return startDate, endDate
	}

	c, err := s.dealClient.GetContract(ctx, tenantID, contractID)
	if err != nil {
		if endDate.IsZero() {
			endDate = startDate.AddDate(0, defaultLeaseTermMonths, 0)
		}
		return startDate, endDate
	}

	if strings.ToLower(strings.TrimSpace(c.Type)) == "locacao" || strings.ToLower(strings.TrimSpace(c.Type)) == "locação" {
		startDate = c.StartDate
		if c.EndDate != nil {
			endDate = *c.EndDate
		} else {
			endDate = startDate.AddDate(0, defaultLeaseTermMonths, 0)
		}
	}

	if endDate.IsZero() {
		endDate = startDate.AddDate(0, defaultLeaseTermMonths, 0)
	}
	if endDate.Before(startDate) {
		endDate = startDate
	}

	return startDate, endDate
}

func (s *RentalChargeConfigService) updateChargeDates(ctx context.Context, cfg *rental_charge_config.RentalChargeConfig, oldFirstChargeDate time.Time) error {
	charges, err := s.listAllCharges(ctx, cfg.TenantID, cfg.ContractID)
	if err != nil {
		return err
	}

	for _, ch := range charges {
		// Configuration edits may only rewrite future, untouched receivables.
		// Historical PAID/CANCELLED/REVERSED rows and partially-paid debt keep
		// their original dates for audit and reconciliation consistency.
		if ch == nil || ch.Status != charge.StatusPending {
			continue
		}
		expectedVersion := ch.UpdatedAt
		if sameDay(ch.DueDate, oldFirstChargeDate) {
			ch.DueDate = cfg.FirstChargeDate
			updated, err := updateChargeIfCurrent(ctx, s.chargeRepo, ch, expectedVersion)
			if err != nil {
				return err
			}
			if !updated {
				return charge.ErrConcurrentModification
			}
			continue
		}

		ref, err := parseMonthKey(ch.ReferenceMonth, cfg.StartDate.Location())
		if err != nil {
			continue
		}
		newDue := setDayOfMonth(ref.Year(), ref.Month(), cfg.DefaultDueDay, cfg.StartDate.Location())
		if !sameDay(ch.DueDate, newDue) {
			ch.DueDate = newDue
			updated, err := updateChargeIfCurrent(ctx, s.chargeRepo, ch, expectedVersion)
			if err != nil {
				return err
			}
			if !updated {
				return charge.ErrConcurrentModification
			}
		}
	}
	return nil
}

func (s *RentalChargeConfigService) listAllCharges(ctx context.Context, tenantID, contractID string) ([]*charge.Charge, error) {
	var all []*charge.Charge
	var offset int64
	limit := int64(200)
	for {
		items, total, err := s.chargeRepo.List(ctx, tenantID, contractID, "", "", limit, offset)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		offset += int64(len(items))
		if offset >= total || len(items) == 0 {
			break
		}
	}
	return all, nil
}

// createChargeIfAbsent persiste uma única charge PENDING e informa se uma
// nova linha foi inserida. Uma charge já materializada para o mesmo
// source_event_id (competência) é pulada, tornando retries idempotentes por
// reference month.
func (s *RentalChargeConfigService) createChargeIfAbsent(ctx context.Context, c *charge.Charge) (bool, error) {
	// Charges are created in PENDING with no checkout; settled manually via
	// ChargeService.MarkPaid (reconcile-manual).
	if err := s.chargeRepo.Create(ctx, c); err != nil {
		if errors.Is(err, charge.ErrChargeAlreadyExists) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func centsToFloat(cents int64) float64 {
	return float64(cents) / 100
}

func monthKey(d time.Time) string {
	return d.Format("2006-01")
}

func parseMonthKey(s string, loc *time.Location) (time.Time, error) {
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return time.Time{}, errors.New("invalid reference_month")
	}
	y, err := strconv.Atoi(parts[0])
	if err != nil {
		return time.Time{}, err
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(y, time.Month(m), 1, 0, 0, 0, 0, loc), nil
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func sameMonth(a, b time.Time) bool {
	ay, am, _ := a.Date()
	by, bm, _ := b.Date()
	return ay == by && am == bm
}

func setDayOfMonth(year int, month time.Month, day int, loc *time.Location) time.Time {
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, loc).Day()
	if day > last {
		day = last
	}
	return time.Date(year, month, day, 0, 0, 0, 0, loc)
}
