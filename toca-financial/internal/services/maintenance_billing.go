package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/domain/valueobjects"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental_charge_config"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
)

// MaintenanceBillingService decide ONDE o custo de uma manutenção entra no
// financeiro. O valor não é uma obrigação solta: ele se acopla ao próximo
// documento que o inquilino (ou o proprietário) já espera receber.
//
//	payer_role=tenant → compõe a próxima cobrança em aberto do contrato.
//	                    Sem cobrança em aberto, emite uma cobrança avulsa
//	                    já com composição (sem item, o pagamento ficaria sem
//	                    dono econômico — `BuildPaymentAllocations` devolve
//	                    vazio para cobranças sem line items).
//	payer_role=owner  → desconta no próximo repasse pendente do proprietário.
//	payer_role=agency → responsabilidade da imobiliária: nada a lançar.
//
// Todas as operações são idempotentes pelo `reference_id`/`source_event_id`
// derivados do chamado.
type MaintenanceBillingService struct {
	chargeService    *ChargeService
	rentalConfigRepo rental_charge_config.Repository
	chargeRepo       charge.RepositoryInterface
	transferRepo     transfer.RepositoryInterface
	transferService  *TransferService
	txService        *TransactionService
	journal          *JournalService
	now              func() time.Time
}

// WithTransactions habilita o crédito avulso (devolução ao inquilino ou ao
// proprietário) quando não há documento seguinte para absorver o ajuste.
func (s *MaintenanceBillingService) WithTransactions(tx *TransactionService) *MaintenanceBillingService {
	s.txService = tx
	return s
}

// WithJournal habilita a baixa contábil da dedução do proprietário: o valor
// retido sai de owner_payable contra receita (recuperação do custo que a
// imobiliária adiantou). Sem journal configurado o serviço segue operando —
// apenas sem o lançamento no razão.
func (s *MaintenanceBillingService) WithJournal(j *JournalService) *MaintenanceBillingService {
	s.journal = j
	return s
}

func NewMaintenanceBillingService(
	chargeService *ChargeService,
	chargeRepo charge.RepositoryInterface,
	transferRepo transfer.RepositoryInterface,
	transferService *TransferService,
) *MaintenanceBillingService {
	return &MaintenanceBillingService{
		chargeService:    chargeService,
		rentalConfigRepo: chargeService.rentalConfigRepo,
		chargeRepo:       chargeRepo,
		transferRepo:     transferRepo,
		transferService:  transferService,
		now:              time.Now,
	}
}

// Destinos possíveis do custo. `Target` é a escolha explícita do operador;
// vazio cai no padrão do payer_role (compatível com chamados anteriores).
const (
	// TargetNextCharge soma na próxima cobrança em aberto do contrato.
	TargetNextCharge = "next_charge"
	// TargetNextTransfer desconta no próximo repasse pendente do proprietário.
	TargetNextTransfer = "next_transfer"
	// TargetStandalone emite uma cobrança avulsa — o caminho que continua
	// valendo quando o contrato acabou e não há mais cobrança nem repasse
	// programados (locação encerrada, venda já repassada).
	TargetStandalone = "standalone"
)

// MaintenanceBillingInput é o que a cobrança precisa saber do chamado.
type MaintenanceBillingInput struct {
	TicketID        string
	TenantID        string
	ContractID      string
	PropertyID      string
	RequesterLeadID string
	// PayerLeadID é a parte cobrada na cobrança avulsa (dono do imóvel quando
	// a responsabilidade é dele). Cai para RequesterLeadID quando vazio.
	PayerLeadID string
	// OwnerLeadID é o proprietário do imóvel, usado como dono econômico quando
	// ele adiantou o pagamento do serviço.
	OwnerLeadID string
	Category    string
	PayerRole   string
	Target      string
	// AdvancedBy diz quem pagou o fornecedor: "imobiliaria" (default),
	// "proprietario" ou "inquilino". É o que define a natureza do valor — o
	// custo é sempre de terceiro, a imobiliária só adianta e recebe de volta.
	AdvancedBy string
}

// Partes que podem ter adiantado o pagamento do serviço.
const (
	AdvancedByAgency = "imobiliaria"
	AdvancedByOwner  = "proprietario"
	AdvancedByTenant = "inquilino"
)

// advancedBy devolve quem adiantou, com o default do caso dominante.
func (in MaintenanceBillingInput) advancedBy() string {
	switch strings.ToLower(strings.TrimSpace(in.AdvancedBy)) {
	case AdvancedByOwner:
		return AdvancedByOwner
	case AdvancedByTenant:
		return AdvancedByTenant
	default:
		return AdvancedByAgency
	}
}

// maintenanceParty traduz o papel de quem paga (payer_role) para a mesma
// vocabulário de quem adiantou (advanced_by). Sem isso "tenant" nunca seria
// reconhecido como "inquilino" e a mesma parte apareceria como duas.
func maintenanceParty(payerRole string) string {
	switch payerRole {
	case "tenant":
		return AdvancedByTenant
	case "owner", "shared":
		return AdvancedByOwner
	case "agency":
		return AdvancedByAgency
	default:
		return ""
	}
}

// maintenanceNature descreve como o custo de terceiro entra no financeiro:
// se há documento, a quem o valor pertence e como ele liquida.
type maintenanceNature struct {
	// Movement diz se há algo a lançar. Quando quem adiantou o pagamento é a
	// mesma parte que assume o custo, o valor não passa pela imobiliária.
	Movement bool
	// Reason explica a ausência de movimento (log e tela).
	Reason string
	// ItemOwner é o dono econômico do valor cobrado do pagador.
	ItemOwner string
	// ItemAllocation é a natureza do item na cobrança.
	ItemAllocation charge.AllocationType
	// ItemSettlement é como o valor liquida para o dono econômico.
	ItemSettlement charge.SettlementMode
	// OwnerRecoveryAllocation é a contrapartida da retenção no repasse.
	OwnerRecoveryAllocation charge.AllocationType
	// AdvancesFromAgency: a imobiliária desembolsou pelo fornecedor.
	AdvancesFromAgency bool
}

// resolveMaintenanceNature aplica "todo valor tem dono econômico" ao custo de
// manutenção/vistoria. O custo NUNCA é receita própria: ou pertence a quem
// adiantou o pagamento, ou é a recuperação do adiantamento da imobiliária.
//
//	imobiliária adiantou + inquilino assume → direito a receber (1.1.02)
//	imobiliária adiantou + proprietário assume → retenção compensa o adiantamento
//	proprietário adiantou + inquilino assume → o valor é do proprietário
//	quem adiantou é quem assume → nada a lançar
func resolveMaintenanceNature(in MaintenanceBillingInput) maintenanceNature {
	advanced := in.advancedBy()
	bears := in.PayerRole

	if advanced == maintenanceParty(bears) {
		return maintenanceNature{
			Movement: false,
			Reason:   "quem adiantou o pagamento é a mesma parte que assume o custo: não há valor a cobrar nem a reembolsar",
		}
	}

	switch {
	case advanced == AdvancedByAgency && bears == "tenant":
		return maintenanceNature{
			Movement: true, Reason: "a imobiliária adiantou e o inquilino reembolsa",
			ItemOwner: in.TenantID, ItemAllocation: charge.AllocationTypeReceivable,
			ItemSettlement: charge.SettlementModeDirectLiquidation, AdvancesFromAgency: true,
		}
	case advanced == AdvancedByAgency && bears == "owner":
		return maintenanceNature{
			Movement: true, Reason: "a imobiliária adiantou e o proprietário reembolsa no repasse",
			ItemOwner: in.TenantID, ItemAllocation: charge.AllocationTypeReceivable,
			ItemSettlement:          charge.SettlementModeOffset,
			OwnerRecoveryAllocation: charge.AllocationTypeReceivable, AdvancesFromAgency: true,
		}
	case advanced == AdvancedByOwner && bears == "tenant":
		return maintenanceNature{
			Movement: true, Reason: "o proprietário adiantou e o inquilino reembolsa — o valor é do proprietário",
			ItemOwner:      billingFirstNonEmpty(in.OwnerLeadID, in.TenantID),
			ItemAllocation: charge.AllocationTypeOwnerPayable,
			ItemSettlement: charge.SettlementModeReimbursement,
		}
	case advanced == AdvancedByTenant && bears == "owner":
		return maintenanceNature{
			Movement: true, Reason: "o inquilino adiantou e o proprietário reembolsa — o valor é do inquilino",
			ItemOwner:               billingFirstNonEmpty(in.PayerLeadID, in.RequesterLeadID),
			ItemAllocation:          charge.AllocationTypeReceivable,
			ItemSettlement:          charge.SettlementModeReimbursement,
			OwnerRecoveryAllocation: charge.AllocationTypeReceivable,
		}
	default:
		return maintenanceNature{
			Movement: false,
			Reason:   "combinação de quem adiantou e quem assume não gera cobrança nem reembolso",
		}
	}
}

// resolveTarget aplica o destino escolhido; sem escolha, deriva do papel de
// quem paga (inquilino → próxima cobrança; proprietário → próximo repasse).
func resolveTarget(role, target string) string {
	switch target {
	case TargetNextCharge, TargetNextTransfer, TargetStandalone:
		return target
	}
	switch role {
	case "owner", "shared":
		return TargetNextTransfer
	case "tenant":
		return TargetNextCharge
	default:
		return ""
	}
}

func (in MaintenanceBillingInput) description() string {
	category := strings.TrimSpace(in.Category)
	if category == "" {
		category = "serviço"
	}
	return fmt.Sprintf("Manutenção: %s (#%s)", category, in.TicketID)
}

// MaintenanceBillingTarget descreve o destino escolhido, para que a tela
// consiga dizer ao operador onde o custo entrou.
type MaintenanceBillingTarget struct {
	// Kind: "charge" (cobrança do inquilino) ou "transfer" (repasse do proprietário).
	Kind           string
	ChargeID       string
	DueDate        time.Time
	ReferenceMonth string
	AmountCents    int64
	Created        bool
}

const maintenanceLineSourceType = "maintenance"

// Apply lança o custo no destino definido pelo `payer_role`.
func (s *MaintenanceBillingService) Apply(
	ctx context.Context,
	in MaintenanceBillingInput,
	amount float64,
	sourceEventID string,
) (*MaintenanceBillingTarget, error) {
	if s == nil || s.chargeService == nil {
		return nil, errors.New("maintenance billing service is not configured")
	}
	if strings.TrimSpace(in.TicketID) == "" || strings.TrimSpace(in.TenantID) == "" {
		return nil, errors.New("maintenance billing requires ticket and tenant")
	}
	if amount <= 0 {
		return nil, nil
	}

	if in.PayerRole == "" {
		return nil, nil
	}
	if in.PayerRole == "agency" {
		// Ninguém reembolsa: o custo é da imobiliária e vira despesa própria.
		return nil, s.postAgencyMaintenanceExpense(ctx, in, amount)
	}

	// Reentrega do evento: o custo já está lançado (item na cobrança ou
	// desconto no repasse). Não relança — a reconciliação é do Reconcile.
	if placed, err := s.findPlacement(ctx, in); err != nil {
		return nil, err
	} else if placed != nil {
		return placed.target(), nil
	}

	nature, err := s.natureFor(ctx, in)
	if err != nil {
		return nil, err
	}
	if !nature.Movement {
		log.Printf("[INFO] maintenance.billing: ticket=%s sem lançamento — %s", in.TicketID, nature.Reason)
		return nil, nil
	}
	if nature.AdvancesFromAgency {
		if err := s.postAgencyAdvance(ctx, in, amount); err != nil {
			return nil, err
		}
	}

	switch resolveTarget(in.PayerRole, in.Target) {
	case TargetNextCharge:
		return s.applyToNextTenantCharge(ctx, in, amount, sourceEventID, nature)
	case TargetNextTransfer:
		return s.applyToNextOwnerTransfer(ctx, in, amount)
	case TargetStandalone:
		return s.createStandaloneMaintenanceCharge(ctx, in, amount, sourceEventID, nature)
	default:
		return nil, nil
	}
}

// applyToNextTenantCharge soma o valor à próxima cobrança em aberto do
// contrato. É o que o operador espera ao "cobrar do inquilino": o valor
// aparece no próximo documento, discriminado, em vez de virar uma segunda
// cobrança paralela.
func (s *MaintenanceBillingService) applyToNextTenantCharge(
	ctx context.Context,
	in MaintenanceBillingInput,
	amount float64,
	sourceEventID string,
	nature maintenanceNature,
) (*MaintenanceBillingTarget, error) {
	target, err := s.findNextOpenCharge(ctx, in)
	if err != nil {
		return nil, err
	}

	amountCents := int64(amount*100 + 0.5)
	if target == nil {
		// Sem cobrança em aberto não há onde somar: a cobrança avulsa é o
		// destino que continua valendo (contrato encerrado, plano sem emissão).
		log.Printf("[INFO] maintenance.billing: ticket=%s sem cobrança em aberto no contrato=%s — emitindo cobrança avulsa", in.TicketID, in.ContractID)
		return s.createStandaloneMaintenanceCharge(ctx, in, amount, sourceEventID, nature)
	}

	items := append([]charge.LineItem{}, target.LineItems...)
	items = append(items, charge.LineItem{
		Description:     in.description(),
		AmountCents:     amountCents,
		SourceType:      maintenanceLineSourceType,
		SourceID:        in.TicketID,
		EconomicOwnerID: nature.ItemOwner,
		SettlementMode:  nature.ItemSettlement,
		AllocationType:  nature.ItemAllocation,
	})

	// A composição precisa fechar com o valor da cobrança: o total sobe junto.
	nextAmount := target.Amount.Float() + amount
	target.Amount = valueobjects.NewMoneyFromFloat(nextAmount)
	if err := target.SetLineItems(items); err != nil {
		return nil, fmt.Errorf("compose maintenance into charge %s: %w", target.ID, err)
	}
	target.CalculateTotal()
	if err := s.chargeService.Update(ctx, target); err != nil {
		return nil, fmt.Errorf("update charge %s with maintenance item: %w", target.ID, err)
	}
	log.Printf("[AUDIT] maintenance.billing: ticket=%s somado na cobrança=%s (+%.2f)", in.TicketID, target.ID, amount)

	return &MaintenanceBillingTarget{
		Kind:           "charge",
		ChargeID:       target.ID,
		DueDate:        target.DueDate,
		ReferenceMonth: target.ReferenceMonth,
		AmountCents:    amountCents,
	}, nil
}

// findNextOpenCharge devolve a cobrança que o inquilino ainda vai pagar:
// a mais próxima em aberto a partir de hoje; se todas já venceram, a mais
// antiga ainda pendente. Cobranças sem composição ficam de fora — somar nelas
// exigiria inventar o dono econômico do valor que já estava lá.
func (s *MaintenanceBillingService) findNextOpenCharge(
	ctx context.Context,
	in MaintenanceBillingInput,
) (*charge.Charge, error) {
	charges, _, err := s.chargeRepo.List(ctx, in.TenantID, in.ContractID, "", "", 500, 0)
	if err != nil {
		return nil, fmt.Errorf("list charges for maintenance billing: %w", err)
	}

	today := s.now()
	candidates := make([]*charge.Charge, 0, len(charges))
	for _, c := range charges {
		if c == nil || c.Status != charge.StatusPending {
			continue
		}
		// Só entra em cobrança que já tem composição: sem ela não se sabe o
		// dono econômico do valor original, e o sistema não inventa essa
		// decisão. Nesse caso a cobrança avulsa é o destino seguro.
		if len(c.LineItems) == 0 {
			continue
		}
		candidates = append(candidates, c)
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].DueDate.Before(candidates[j].DueDate)
	})
	for _, c := range candidates {
		if !c.DueDate.Before(today) {
			return c, nil
		}
	}
	return candidates[0], nil
}

// createStandaloneMaintenanceCharge emite a cobrança avulsa. É o destino
// escolhido quando o contrato não permite programação (locação encerrada sem
// repasse, venda já repassada) e também o fallback quando não há cobrança em
// aberto para somar. Sai com composição para que o pagamento tenha dono
// econômico.
func (s *MaintenanceBillingService) createStandaloneMaintenanceCharge(
	ctx context.Context,
	in MaintenanceBillingInput,
	amount float64,
	sourceEventID string,
	nature maintenanceNature,
) (*MaintenanceBillingTarget, error) {
	amountCents := int64(amount*100 + 0.5)
	// O pagador é a parte do chamado: o inquilino ou o proprietário,
	// conforme a responsabilidade escolhida.
	payerLeadID := billingFirstNonEmpty(in.PayerLeadID, in.RequesterLeadID)
	ownerID := payerLeadID
	referenceMonth := s.now().Format("2006-01")
	due := s.now().AddDate(0, 0, 30)

	items := []charge.LineItem{{
		Description:     in.description(),
		AmountCents:     amountCents,
		SourceType:      maintenanceLineSourceType,
		SourceID:        in.TicketID,
		EconomicOwnerID: nature.ItemOwner,
		SettlementMode:  nature.ItemSettlement,
		AllocationType:  nature.ItemAllocation,
	}}

	created, err := s.chargeService.CreateWithCompositionFromRequest(
		ctx,
		sourceEventID,
		in.ContractID,
		"",
		payerLeadID,
		payerLeadID,
		ownerID,
		in.PropertyID,
		in.TenantID,
		charge.TypeMaintenance,
		in.description(),
		referenceMonth,
		amount,
		0,
		due,
		items,
	)
	if err != nil {
		return nil, fmt.Errorf("create maintenance charge for ticket %s: %w", in.TicketID, err)
	}
	if created == nil {
		return nil, nil
	}

	return &MaintenanceBillingTarget{
		Kind:           "charge",
		ChargeID:       created.ID,
		DueDate:        created.DueDate,
		ReferenceMonth: created.ReferenceMonth,
		AmountCents:    amountCents,
		Created:        true,
	}, nil
}

// applyToNextOwnerTransfer desconta no próximo repasse pendente do contrato.
// O repasse do mês corrente pode já ter sido processado — usar o "próximo
// pendente" evita perder o desconto quando o ciclo do mês já fechou.
func (s *MaintenanceBillingService) applyToNextOwnerTransfer(
	ctx context.Context,
	in MaintenanceBillingInput,
	amount float64,
) (*MaintenanceBillingTarget, error) {
	if s.transferService == nil || s.transferRepo == nil {
		return nil, errors.New("owner maintenance deduction requires transfer service")
	}

	target, err := s.findNextPendingTransfer(ctx, in)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, fmt.Errorf(
			"não há repasse pendente para o contrato %s (encerrado ou já repassado): escolha a cobrança avulsa para lançar a manutenção #%s",
			in.ContractID, in.TicketID,
		)
	}

	// O repasse absorve o desconto até o que o proprietário tem a receber:
	// acima disso o excedente não seria descontado (o líquido não fica
	// negativo), então o valor se perderia em silêncio.
	capacityCents := maintenanceTransferCapacityCents(target)
	if amountCents := int64(amount*100 + 0.5); amountCents > capacityCents {
		return nil, fmt.Errorf(
			"o repasse de %s absorve até %s: o excedente do desconto da manutenção #%s não é retido — use a cobrança avulsa para o valor integral",
			target.ReferenceMonth, formatBRL(float64(capacityCents)/100), in.TicketID,
		)
	}

	nature, err := s.natureFor(ctx, in)
	if err != nil {
		return nil, err
	}
	if err := s.transferService.ApplyDeductionDetailed(
		ctx,
		in.TenantID,
		in.ContractID,
		target.ReferenceMonth,
		transfer.Deduction{
			Description: in.description(),
			Amount:      amount,
			Type:        "MAINTENANCE",
			ReferenceID: in.TicketID,
			// A contrapartida depende de quem adiantou: reembolso do direito a
			// receber da imobiliária ou obrigação com o terceiro que pagou.
			LedgerAccount: string(deductionRecoveryAccount(nature)),
		},
	); err != nil {
		return nil, fmt.Errorf("apply maintenance deduction for ticket %s: %w", in.TicketID, err)
	}
	// A baixa contábil da retenção (owner_payable × receita) é postada pelo
	// TransferService.ApplyDeduction — ponto único de toda dedução de repasse.

	return &MaintenanceBillingTarget{
		Kind:           "transfer",
		ReferenceMonth: target.ReferenceMonth,
		AmountCents:    int64(amount*100 + 0.5),
	}, nil
}

// findNextPendingTransfer escolhe o repasse pendente de referência >= mês
// corrente; sem nenhum à frente, usa o pendente mais antigo.
func (s *MaintenanceBillingService) findNextPendingTransfer(
	ctx context.Context,
	in MaintenanceBillingInput,
) (*transfer.OwnerTransfer, error) {
	transfers, _, err := s.transferRepo.List(ctx, in.TenantID, in.ContractID, "", string(transfer.StatusPending), "", 200, 0)
	if err != nil {
		return nil, fmt.Errorf("list pending transfers for maintenance deduction: %w", err)
	}
	if len(transfers) == 0 {
		return nil, nil
	}

	sort.SliceStable(transfers, func(i, j int) bool {
		return transfers[i].ReferenceMonth < transfers[j].ReferenceMonth
	})
	currentMonth := s.now().Format("2006-01")
	for _, t := range transfers {
		if t.ReferenceMonth >= currentMonth {
			return t, nil
		}
	}
	return transfers[0], nil
}

// natureFor aplica a matriz de natureza e resolve o dono econômico do valor
// quando ele é o proprietário do imóvel.
func (s *MaintenanceBillingService) natureFor(ctx context.Context, in MaintenanceBillingInput) (maintenanceNature, error) {
	return s.resolveItemOwner(ctx, in, resolveMaintenanceNature(in))
}

// resolveItemOwner resolve o dono econômico do valor cobrado. Quem adiantou o
// pagamento é o dono: quando foi o proprietário do imóvel, o valor não pode
// cair na imobiliária — e inventar um dono é exatamente o que a premissa
// "todo valor tem dono econômico" proíbe. Sem conseguir resolver, o lançamento
// falha em vez de creditar a parte errada.
func (s *MaintenanceBillingService) resolveItemOwner(
	ctx context.Context,
	in MaintenanceBillingInput,
	nature maintenanceNature,
) (maintenanceNature, error) {
	if nature.ItemAllocation != charge.AllocationTypeOwnerPayable {
		return nature, nil
	}
	if ownerID := strings.TrimSpace(in.OwnerLeadID); ownerID != "" {
		nature.ItemOwner = ownerID
		return nature, nil
	}
	if s.rentalConfigRepo != nil && strings.TrimSpace(in.ContractID) != "" {
		cfg, err := s.rentalConfigRepo.GetByContractID(ctx, in.TenantID, in.ContractID)
		if err != nil {
			return nature, fmt.Errorf("resolve property owner for maintenance ticket %s: %w", in.TicketID, err)
		}
		if cfg != nil && strings.TrimSpace(cfg.OwnerID) != "" {
			nature.ItemOwner = cfg.OwnerID
			return nature, nil
		}
	}
	return nature, fmt.Errorf(
		"não foi possível identificar o proprietário que adiantou o pagamento do serviço #%s: o valor não pode ser creditado à imobiliária",
		in.TicketID,
	)
}

// postAgencyAdvance registra o desembolso da imobiliária pelo serviço. O
// custo é de terceiro, então o adiantamento é um direito a receber — não é
// despesa própria enquanto houver responsável pelo reembolso.
//
//	Débito  contas a receber (1.1.02)
//	Crédito caixa
//
// Idempotente pelo source_event derivado do chamado.
func (s *MaintenanceBillingService) postAgencyAdvance(ctx context.Context, in MaintenanceBillingInput, amount float64) error {
	if s.journal == nil || amount <= 0 {
		return nil
	}
	cents := int64(amount*100 + 0.5)
	if cents <= 0 {
		return nil
	}
	entry, err := journal.New(
		in.TenantID,
		in.ContractID,
		"Adiantamento de manutenção: "+in.description(),
		s.now(),
		[]journal.Leg{
			{Account: journal.AccountAccountsReceivable, Direction: journal.Debit, AmountCents: cents, RelatedEntityID: in.TicketID},
			{Account: journal.AccountCash, Direction: journal.Credit, AmountCents: cents, RelatedEntityID: in.TicketID},
		},
	)
	if err != nil {
		return fmt.Errorf("build maintenance advance entry for ticket %s: %w", in.TicketID, err)
	}
	entry.OperationID = in.TicketID
	// O adiantamento não tem documento operacional: o fluxo de caixa o lê do
	// razão por este marcador.
	entry.CashFlowSource = "maintenance_advance"
	sourceEventID := events.DeriveNotificationID(in.TicketID, "maintenance.advance")
	if err := s.journal.CreateWithSourceEventID(ctx, sourceEventID, entry); err != nil {
		return fmt.Errorf("post maintenance advance for ticket %s: %w", in.TicketID, err)
	}
	return nil
}

// postAgencyMaintenanceExpense registra o custo quando a imobiliária é a
// responsável final: sem reembolso, o valor é despesa própria e aparece no
// resultado (mesmo tratamento de uma conta de consumo assumida pela agência).
func (s *MaintenanceBillingService) postAgencyMaintenanceExpense(ctx context.Context, in MaintenanceBillingInput, amount float64) error {
	if s.txService == nil || amount <= 0 {
		return nil
	}
	tx := transaction.New(in.ContractID, transaction.TypeDespesa, in.description(), amount, s.now(), in.TenantID)
	tx.Category = "Manutenção"
	tx.Month = s.now().Format("2006-01")
	sourceEventID := events.DeriveNotificationID(in.TicketID, "maintenance.agency-expense")
	if err := s.txService.CreateWithSourceEventID(ctx, sourceEventID, tx); err != nil {
		return fmt.Errorf("create agency maintenance expense for ticket %s: %w", in.TicketID, err)
	}
	log.Printf("[AUDIT] maintenance.billing: ticket=%s custo assumido pela imobiliária (%s)", in.TicketID, formatBRL(amount))
	return nil
}

// postDeductionAdjustment contabiliza o incremento da retenção quando o custo
// final sobe depois do lançamento original. O source_event_id é qualificado
// pelo valor para não colidir com a baixa do valor anterior.
func (s *MaintenanceBillingService) postDeductionAdjustment(
	ctx context.Context,
	in MaintenanceBillingInput,
	target *transfer.OwnerTransfer,
	amount float64,
) error {
	if s.journal == nil || target == nil || amount <= 0 {
		return nil
	}
	cents := int64(amount*100 + 0.5)
	if cents <= 0 {
		return nil
	}
	entry, err := journal.New(
		in.TenantID,
		in.ContractID,
		fmt.Sprintf("Ajuste de manutenção retida do repasse: %s", in.description()),
		s.now(),
		[]journal.Leg{
			{
				Account: journal.AccountOwnerPayable, Direction: journal.Debit, AmountCents: cents,
				TransferID: target.ID, BeneficiaryPartyID: target.OwnerID,
			},
			{
				Account: journal.AccountRevenue, Direction: journal.Credit, AmountCents: cents,
				TransferID: target.ID, BeneficiaryPartyID: target.OwnerID,
			},
		},
	)
	if err != nil {
		return fmt.Errorf("build maintenance adjustment entry for ticket %s: %w", in.TicketID, err)
	}
	entry.OperationID = target.ID
	sourceEventID := events.DeriveNotificationID(in.TicketID, "maintenance.recovery-adjustment", target.ReferenceMonth, fmt.Sprintf("%d", cents))
	if err := s.journal.CreateWithSourceEventID(ctx, sourceEventID, entry); err != nil {
		return fmt.Errorf("post maintenance adjustment for ticket %s: %w", in.TicketID, err)
	}
	return nil
}

// postDeductionReversal desfaz a recuperação quando o custo final é menor que
// o retido: receita devolvida, passivo com o proprietário restabelecido.
func (s *MaintenanceBillingService) postDeductionReversal(
	ctx context.Context,
	in MaintenanceBillingInput,
	target *transfer.OwnerTransfer,
	amount float64,
) error {
	if s.journal == nil || target == nil || amount <= 0 {
		return nil
	}
	cents := int64(amount*100 + 0.5)
	if cents <= 0 {
		return nil
	}
	entry, err := journal.New(
		in.TenantID,
		in.ContractID,
		fmt.Sprintf("Crédito de manutenção ao proprietário: %s", in.description()),
		s.now(),
		[]journal.Leg{
			{
				Account: journal.AccountRevenue, Direction: journal.Debit, AmountCents: cents,
				TransferID: target.ID, BeneficiaryPartyID: target.OwnerID,
			},
			{
				Account: journal.AccountOwnerPayable, Direction: journal.Credit, AmountCents: cents,
				TransferID: target.ID, BeneficiaryPartyID: target.OwnerID,
			},
		},
	)
	if err != nil {
		return fmt.Errorf("build maintenance credit entry for ticket %s: %w", in.TicketID, err)
	}
	entry.OperationID = target.ID
	sourceEventID := events.DeriveNotificationID(in.TicketID, "maintenance.recovery-reversal", target.ReferenceMonth)
	if err := s.journal.CreateWithSourceEventID(ctx, sourceEventID, entry); err != nil {
		return fmt.Errorf("post maintenance credit for ticket %s: %w", in.TicketID, err)
	}
	return nil
}

// settleCredit distribui um crédito de manutenção que não pôde ser abatido no
// próprio documento (cobrança já paga / repasse já processado): aplica na
// próxima cobrança em aberto do inquilino, no próximo repasse pendente do
// proprietário e, no que sobrar, em um crédito avulso. Cada etapa é
// idempotente, então a reentrega do evento não credita duas vezes.
func (s *MaintenanceBillingService) settleCredit(ctx context.Context, in MaintenanceBillingInput, creditCents int64, competence string) error {
	if creditCents <= 0 {
		return nil
	}
	if strings.TrimSpace(competence) == "" {
		competence = s.now().Format("2006-01")
	}
	remaining := creditCents
	var err error
	if in.PayerRole == "owner" || in.PayerRole == "shared" {
		remaining, err = s.creditNextTransfer(ctx, in, remaining)
		if err != nil {
			return err
		}
		if remaining <= 0 {
			return nil
		}
		return s.creditOwnerStandalone(ctx, in, remaining, competence)
	}

	remaining, err = s.creditNextCharges(ctx, in, remaining)
	if err != nil {
		return err
	}
	if remaining <= 0 {
		return nil
	}
	return s.creditTenantStandalone(ctx, in, remaining, competence)
}

// creditNextCharges abate o crédito como desconto nas próximas cobranças em
// aberto, na ordem de vencimento. Devolve o que não coube.
func (s *MaintenanceBillingService) creditNextCharges(ctx context.Context, in MaintenanceBillingInput, creditCents int64) (int64, error) {
	charges, _, err := s.chargeRepo.List(ctx, in.TenantID, in.ContractID, "", "", 500, 0)
	if err != nil {
		return creditCents, fmt.Errorf("list charges for maintenance credit: %w", err)
	}
	if len(charges) == 0 {
		return creditCents, nil
	}
	sort.SliceStable(charges, func(i, j int) bool { return charges[i].DueDate.Before(charges[j].DueDate) })

	remaining := creditCents
	for _, c := range charges {
		if remaining <= 0 {
			break
		}
		if c == nil || c.Status != charge.StatusPending || c.Type == charge.TypeMaintenance {
			continue
		}
		eventID := events.DeriveNotificationID(in.TicketID, "maintenance.credit.discount", c.ID)
		if containsString(c.AppliedEventIDs, eventID) {
			continue
		}
		// O desconto não pode passar do que a cobrança tem a receber.
		maxDiscountCents := c.Amount.Add(c.LateFee).Add(c.Interest).Cents() - c.Discount.Cents()
		if maxDiscountCents <= 0 {
			continue
		}
		applied := remaining
		if applied > maxDiscountCents {
			applied = maxDiscountCents
		}
		c.Discount = valueobjects.NewMoney(c.Discount.Cents() + applied)
		c.AppliedEventIDs = append(c.AppliedEventIDs, eventID)
		c.CalculateTotal()
		if err := s.chargeService.Update(ctx, c); err != nil {
			return remaining, fmt.Errorf("apply maintenance credit on charge %s: %w", c.ID, err)
		}
		log.Printf("[AUDIT] maintenance.billing: ticket=%s crédito de %s abatido na cobrança=%s", in.TicketID, formatBRL(float64(applied)/100), c.ID)
		remaining -= applied
	}
	return remaining, nil
}

// creditNextTransfer devolve o crédito reduzindo a retenção do próximo repasse
// pendente — o proprietário recebe de volta o que foi retido a mais. Devolve o
// que não coube no teto da retenção.
func (s *MaintenanceBillingService) creditNextTransfer(ctx context.Context, in MaintenanceBillingInput, creditCents int64) (int64, error) {
	target, err := s.findNextPendingTransfer(ctx, in)
	if err != nil || target == nil {
		return creditCents, err
	}
	reference := maintenanceCreditReference(in.TicketID)
	for _, detail := range target.DeductionDetails {
		if detail.ReferenceID == reference {
			return creditCents, nil
		}
	}
	roomCents := int64((target.MaintenanceDeductions+target.OtherDeductions)*100 + 0.5)
	if roomCents <= 0 {
		return creditCents, nil
	}
	applied := creditCents
	if applied > roomCents {
		applied = roomCents
	}

	target.MaintenanceDeductions -= float64(applied) / 100
	target.DeductionDetails = append(target.DeductionDetails, transfer.Deduction{
		Description: in.description() + " — crédito do custo final",
		Amount:      -float64(applied) / 100,
		Type:        "MAINTENANCE",
		ReferenceID: reference,
	})
	target.Calculate()
	if err := s.transferRepo.Update(ctx, target); err != nil {
		return creditCents, fmt.Errorf("apply maintenance credit on transfer %s: %w", target.ID, err)
	}
	if err := s.postDeductionReversal(ctx, in, target, float64(applied)/100); err != nil {
		return creditCents, err
	}
	log.Printf("[AUDIT] maintenance.billing: ticket=%s crédito de %s devolvido no repasse=%s", in.TicketID, formatBRL(float64(applied)/100), target.ID)
	return creditCents - applied, nil
}

// creditTenantStandalone registra o crédito ao inquilino quando não há
// cobrança seguinte para abater.
func (s *MaintenanceBillingService) creditTenantStandalone(ctx context.Context, in MaintenanceBillingInput, creditCents int64, competence string) error {
	if s.txService == nil {
		log.Printf("[WARN] maintenance.billing: ticket=%s crédito de %s ao inquilino sem destino (transaction service ausente)",
			in.TicketID, formatBRL(float64(creditCents)/100))
		return nil
	}
	amount := float64(creditCents) / 100
	// Mesmo source_event do crédito legado: seja qual for o caminho, credita
	// uma única vez.
	sourceEventID := events.DeriveNotificationID(in.TicketID, "maintenance.completed.credit")
	// Nota de crédito: o valor cobrado a mais nunca foi da imobiliária. Não é
	// receita nem despesa — é valor de terceiro retido a maior, devolvido.
	tx := transaction.NewCreditNote(in.ContractID, in.description()+" — devolução ao inquilino", amount, s.now(), in.TenantID)
	tx.Category = "Manutenção"
	// O crédito entra na competência do lançamento que o gerou.
	tx.Month = competence
	if err := s.txService.CreateWithSourceEventID(ctx, sourceEventID, tx); err != nil {
		return fmt.Errorf("create tenant maintenance credit for ticket %s: %w", in.TicketID, err)
	}
	// A obrigação com o inquilino nasce no razão: o valor retido a maior
	// reduz o direito a receber e vira obrigação com ele.
	if err := s.postTenantCreditEntry(ctx, in, amount); err != nil {
		return err
	}
	log.Printf("[AUDIT] maintenance.billing: ticket=%s crédito avulso de %s ao inquilino", in.TicketID, formatBRL(amount))
	return nil
}

// creditOwnerStandalone paga ao proprietário o que foi retido a mais, quando
// não há repasse seguinte para absorver o crédito.
func (s *MaintenanceBillingService) creditOwnerStandalone(ctx context.Context, in MaintenanceBillingInput, creditCents int64, competence string) error {
	if s.txService == nil {
		log.Printf("[WARN] maintenance.billing: ticket=%s crédito de %s ao proprietário sem destino (transaction service ausente)",
			in.TicketID, formatBRL(float64(creditCents)/100))
		return nil
	}
	amount := float64(creditCents) / 100
	sourceEventID := events.DeriveNotificationID(in.TicketID, "maintenance.credit.owner")
	tx := transaction.New(in.ContractID, transaction.TypeRepasse, in.description()+" — devolução ao proprietário", amount, s.now(), in.TenantID)
	tx.Category = "Manutenção"
	// O crédito entra na competência do lançamento que o gerou.
	tx.Month = competence
	if err := s.txService.CreateWithSourceEventID(ctx, sourceEventID, tx); err != nil {
		return fmt.Errorf("create owner maintenance credit for ticket %s: %w", in.TicketID, err)
	}
	log.Printf("[AUDIT] maintenance.billing: ticket=%s crédito avulso de %s ao proprietário", in.TicketID, formatBRL(amount))
	return nil
}

// postTenantCreditEntry registra a devolução ao inquilino: o valor cobrado a
// mais deixa de ser direito a receber e passa a ser obrigação com ele.
func (s *MaintenanceBillingService) postTenantCreditEntry(ctx context.Context, in MaintenanceBillingInput, amount float64) error {
	if s.journal == nil || amount <= 0 {
		return nil
	}
	cents := int64(amount*100 + 0.5)
	if cents <= 0 {
		return nil
	}
	entry, err := journal.New(
		in.TenantID,
		in.ContractID,
		"Devolução de manutenção ao inquilino: "+in.description(),
		s.now(),
		[]journal.Leg{
			{Account: journal.AccountAccountsReceivable, Direction: journal.Debit, AmountCents: cents, RelatedEntityID: in.TicketID},
			{Account: journal.AccountOtherPayable, Direction: journal.Credit, AmountCents: cents, RelatedEntityID: in.TicketID},
		},
	)
	if err != nil {
		return fmt.Errorf("build tenant maintenance credit entry for ticket %s: %w", in.TicketID, err)
	}
	entry.OperationID = in.TicketID
	sourceEventID := events.DeriveNotificationID(in.TicketID, "maintenance.tenant-credit")
	if err := s.journal.CreateWithSourceEventID(ctx, sourceEventID, entry); err != nil {
		return fmt.Errorf("post tenant maintenance credit for ticket %s: %w", in.TicketID, err)
	}
	return nil
}

// maintenanceCreditReference identifica a linha de crédito no repasse, para a
// reentrega do evento não devolver o valor duas vezes.
func maintenanceCreditReference(ticketID string) string {
	return ticketID + ":credito"
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// maintenanceTransferCapacityCents é quanto o repasse consegue absorver sem
// truncar. Com subledger (allocation-aware) o desconto bloqueia o bolsão
// disponível, cujo teto é total − reservado; transfer legado usa o líquido.
func maintenanceTransferCapacityCents(t *transfer.OwnerTransfer) int64 {
	if t == nil {
		return 0
	}
	if t.OwnerBalance != nil {
		capacity := t.OwnerBalance.TotalCents - t.OwnerBalance.ReservedCents
		if capacity < 0 {
			return 0
		}
		return capacity
	}
	if t.NetAmount < 0 {
		return 0
	}
	return int64(t.NetAmount*100 + 0.5)
}

// maintenancePlacement descreve onde o custo do chamado está lançado hoje —
// como item de uma cobrança ou como desconto em um repasse.
type maintenancePlacement struct {
	charge         *charge.Charge
	itemIndex      int
	transfer       *transfer.OwnerTransfer
	deductionIndex int
	amountCents    int64
	referenceMonth string
	dueDate        time.Time
}

func (p *maintenancePlacement) target() *MaintenanceBillingTarget {
	if p == nil {
		return nil
	}
	if p.charge != nil {
		return &MaintenanceBillingTarget{
			Kind:           "charge",
			ChargeID:       p.charge.ID,
			DueDate:        p.charge.DueDate,
			ReferenceMonth: p.charge.ReferenceMonth,
			AmountCents:    p.amountCents,
		}
	}
	return &MaintenanceBillingTarget{
		Kind:           "transfer",
		ReferenceMonth: p.referenceMonth,
		AmountCents:    p.amountCents,
	}
}

// findPlacement procura o lançamento do chamado pelo id do ticket: primeiro
// como item de composição, depois como desconto de repasse.
func (s *MaintenanceBillingService) findPlacement(ctx context.Context, in MaintenanceBillingInput) (*maintenancePlacement, error) {
	if s == nil || s.chargeRepo == nil || strings.TrimSpace(in.TicketID) == "" {
		return nil, nil
	}
	charges, _, err := s.chargeRepo.List(ctx, in.TenantID, in.ContractID, "", "", 500, 0)
	if err != nil {
		return nil, fmt.Errorf("list charges for maintenance placement: %w", err)
	}
	for _, c := range charges {
		if c == nil {
			continue
		}
		for index, item := range c.LineItems {
			if item.SourceType == maintenanceLineSourceType && item.SourceID == in.TicketID {
				return &maintenancePlacement{
					charge:      c,
					itemIndex:   index,
					amountCents: item.AmountCents,
					dueDate:     c.DueDate,
				}, nil
			}
		}
	}
	if s.transferRepo == nil {
		return nil, nil
	}
	transfers, _, err := s.transferRepo.List(ctx, in.TenantID, in.ContractID, "", "", "", 200, 0)
	if err != nil {
		return nil, fmt.Errorf("list transfers for maintenance placement: %w", err)
	}
	for _, t := range transfers {
		if t == nil {
			continue
		}
		for index, detail := range t.DeductionDetails {
			if detail.ReferenceID != in.TicketID {
				continue
			}
			return &maintenancePlacement{
				transfer:       t,
				deductionIndex: index,
				amountCents:    deductionCents(detail.Amount),
				referenceMonth: t.ReferenceMonth,
			}, nil
		}
	}
	return nil, nil
}

// Reconcile ajusta o lançamento ao custo final do chamado. Sem lançamento
// prévio, cria com o valor final (política definida só na conclusão).
//
//	item na cobrança em aberto → ajusta o item e o total da cobrança
//	desconto no repasse pendente → ajusta o desconto e o líquido
//	lançamento já liquidado       → excesso vira cobrança avulsa; crédito é
//	                                registrado para o operador tratar
func (s *MaintenanceBillingService) Reconcile(
	ctx context.Context,
	in MaintenanceBillingInput,
	finalAmount float64,
	sourceEventID string,
) (*MaintenanceBillingTarget, error) {
	if in.PayerRole == "" || in.PayerRole == "agency" {
		return nil, nil
	}

	placed, err := s.findPlacement(ctx, in)
	if err != nil {
		return nil, err
	}
	if placed == nil {
		if finalAmount <= 0 {
			return nil, nil
		}
		return s.Apply(ctx, in, finalAmount, sourceEventID)
	}
	// Com lançamento existente, custo final zerado significa retirar a
	// cobrança/desconto: nada a receber.

	finalCents := int64(0)
	if finalAmount > 0 {
		finalCents = int64(finalAmount*100 + 0.5)
	}
	deltaCents := finalCents - placed.amountCents
	if deltaCents == 0 {
		return placed.target(), nil
	}

	if placed.charge != nil {
		return s.adjustChargePlacement(ctx, in, placed, deltaCents)
	}
	return s.adjustTransferPlacement(ctx, in, placed, deltaCents)
}

// adjustChargePlacement aplica a diferença no item da cobrança. Cobrança já
// paga não pode ser editada: o excesso vira cobrança avulsa e o crédito é
// reportado (não há como devolver automaticamente dentro do ciclo).
func (s *MaintenanceBillingService) adjustChargePlacement(
	ctx context.Context,
	in MaintenanceBillingInput,
	placed *maintenancePlacement,
	deltaCents int64,
) (*MaintenanceBillingTarget, error) {
	target := placed.charge
	if target.Status != charge.StatusPending && target.Status != charge.StatusOverdue {
		if deltaCents > 0 {
			log.Printf("[INFO] maintenance.billing: ticket=%s cobrança=%s em %s — excesso de %s emitido como cobrança avulsa",
				in.TicketID, target.ID, target.Status, formatBRL(float64(deltaCents)/100))
			nature, err := s.natureFor(ctx, in)
			if err != nil {
				return nil, err
			}
			return s.createStandaloneMaintenanceCharge(ctx, in, float64(deltaCents)/100, sourceEventIDForAdjustment(in.TicketID), nature)
		}
		// Crédito em cobrança já liquidada: não dá para editar o documento
		// fechado, então o restante segue para a próxima cobrança em aberto
		// ou vira crédito avulso.
		competence := target.ReferenceMonth
		if strings.TrimSpace(competence) == "" {
			competence = s.now().Format("2006-01")
		}
		if err := s.settleCredit(ctx, in, -deltaCents, competence); err != nil {
			return nil, err
		}
		return placed.target(), nil
	}

	items := append([]charge.LineItem{}, target.LineItems...)
	newItemCents := placed.amountCents + deltaCents
	if newItemCents <= 0 {
		// O ajuste zera o item: a manutenção sai da cobrança e o valor volta.
		items = append(items[:placed.itemIndex], items[placed.itemIndex+1:]...)
	} else {
		items[placed.itemIndex].AmountCents = newItemCents
	}
	// O total muda junto com o item: a composição precisa fechar (invariante
	// validado dentro de SetLineItems).
	target.Amount = valueobjects.NewMoney(target.Amount.Cents() + deltaCents)
	if target.Amount.Cents() < 0 {
		return nil, fmt.Errorf("reconcile maintenance item on charge %s: resulting amount is negative", target.ID)
	}
	if err := target.SetLineItems(items); err != nil {
		return nil, fmt.Errorf("reconcile maintenance item on charge %s: %w", target.ID, err)
	}
	target.CalculateTotal()
	if err := s.chargeService.Update(ctx, target); err != nil {
		return nil, fmt.Errorf("persist reconciled maintenance on charge %s: %w", target.ID, err)
	}
	log.Printf("[AUDIT] maintenance.billing: ticket=%s cobrança=%s ajustada em %s (custo final)", in.TicketID, target.ID, formatBRL(float64(deltaCents)/100))

	return &MaintenanceBillingTarget{
		Kind:           "charge",
		ChargeID:       target.ID,
		DueDate:        target.DueDate,
		ReferenceMonth: target.ReferenceMonth,
		AmountCents:    maxInt64(newItemCents, 0),
	}, nil
}

// adjustTransferPlacement aplica a diferença no desconto do repasse pendente.
func (s *MaintenanceBillingService) adjustTransferPlacement(
	ctx context.Context,
	in MaintenanceBillingInput,
	placed *maintenancePlacement,
	deltaCents int64,
) (*MaintenanceBillingTarget, error) {
	item := placed.transfer
	if item.Status != transfer.StatusPending || s.transferRepo == nil {
		if deltaCents > 0 {
			log.Printf("[INFO] maintenance.billing: ticket=%s repasse=%s em %s — excesso de %s emitido como cobrança avulsa",
				in.TicketID, item.ID, item.Status, formatBRL(float64(deltaCents)/100))
			nature, err := s.natureFor(ctx, in)
			if err != nil {
				return nil, err
			}
			return s.createStandaloneMaintenanceCharge(ctx, in, float64(deltaCents)/100, sourceEventIDForAdjustment(in.TicketID), nature)
		}
		if err := s.settleCredit(ctx, in, -deltaCents, item.ReferenceMonth); err != nil {
			return nil, err
		}
		return placed.target(), nil
	}

	details := append([]transfer.Deduction{}, item.DeductionDetails...)
	newAmount := (placed.amountCents + deltaCents)
	if newAmount <= 0 {
		details = append(details[:placed.deductionIndex], details[placed.deductionIndex+1:]...)
		item.MaintenanceDeductions -= float64(placed.amountCents) / 100
	} else {
		details[placed.deductionIndex].Amount = float64(newAmount) / 100
		item.MaintenanceDeductions += float64(deltaCents) / 100
	}
	if item.MaintenanceDeductions < 0 {
		item.MaintenanceDeductions = 0
	}
	item.DeductionDetails = details
	item.Calculate()
	// Um custo de manutenção maior pode consumir todo o valor a repassar: o
	// repasse encerra sem saldo aqui, na mesma transação do ajuste, em vez de
	// ficar pendente e sem baixa possível.
	item.SyncNoPayableBalanceState(time.Now())
	if err := s.transferRepo.Update(ctx, item); err != nil {
		return nil, fmt.Errorf("persist reconciled maintenance on transfer %s: %w", item.ID, err)
	}
	if deltaCents < 0 {
		// Parte da recuperação deixa de existir: devolve a receita e
		// restabelece o passivo com o proprietário.
		if err := s.postDeductionReversal(ctx, in, item, float64(-deltaCents)/100); err != nil {
			return nil, err
		}
	} else if err := s.postDeductionAdjustment(ctx, in, item, float64(deltaCents)/100); err != nil {
		// O ajuste a maior também precisa da sua baixa: a retenção original
		// já tinha sido contabilizada pelo valor anterior.
		return nil, err
	}
	log.Printf("[AUDIT] maintenance.billing: ticket=%s repasse=%s ajustado em %s (custo final)", in.TicketID, item.ID, formatBRL(float64(deltaCents)/100))

	return &MaintenanceBillingTarget{
		Kind:           "transfer",
		ReferenceMonth: item.ReferenceMonth,
		AmountCents:    maxInt64(newAmount, 0),
	}, nil
}

// sourceEventIDForAdjustment identifica o lançamento de ajuste, separado do
// lançamento original para não colidir com o índice único de source_event_id.
func sourceEventIDForAdjustment(ticketID string) string {
	return events.DeriveNotificationID(ticketID, "maintenance.adjustment")
}

// deductionRecoveryAccount é a contrapartida da retenção do proprietário: o
// reembolso baixa o direito a receber quando a imobiliária adiantou, ou a
// obrigação com quem adiantou quando foi um terceiro.
func deductionRecoveryAccount(nature maintenanceNature) journal.Account {
	if nature.OwnerRecoveryAllocation == charge.AllocationTypeReceivable {
		return journal.AccountAccountsReceivable
	}
	if nature.ItemAllocation == charge.AllocationTypeOwnerPayable {
		return journal.AccountOtherPayable
	}
	return journal.AccountAccountsReceivable
}

func deductionCents(amount float64) int64 {
	if amount <= 0 {
		return 0
	}
	return int64(amount*100 + 0.5)
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func billingFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// maintenanceRequestedSourceEventID é a identidade determinística do
// lançamento de um chamado — a mesma usada pelo handler de requisição, para
// que requisição e conclusão nunca gerem dois lançamentos.
func maintenanceRequestedSourceEventID(ticketID string) string {
	return events.DeriveNotificationID(ticketID, "maintenance.requested")
}

// maintenanceBillingAmount usa o custo final e cai para o estimado quando a
// conclusão não trouxe valor.
func maintenanceBillingAmount(p contracts.MaintenanceCompletedEventPayload) float64 {
	if p.ActualCost > 0 {
		return p.ActualCost
	}
	return p.EstimatedCost
}
