package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/managed_bill"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental_charge_config"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
)

// OwnerStatementService agrega Transfer + breakdown do mês para um único
// proprietário (lead). O resultado é estável o suficiente para render em
// PDF (RenderMonthlyOwnerStatementPDF) ou em JSON (handler portal).
//
// Lê de transferRepo via Search com filter por owner_id + reference_month;
// não toca em transaction/charge porque o repasse já carrega gross/admin/
// deductions consolidados (vide transfer_service.calculateTransfer).
type OwnerStatementService struct {
	transferRepo    transfer.RepositoryInterface
	managedBillRepo managed_bill.Repository
	journalRepo     journal.Repository
	contractLookup  ownerStatementContractLookup
	// intermediationLookup traz o plano congelado de intermediação do contrato
	// (quanto foi cobrado, quanto já foi coberto e quanto falta). É o "reflexo"
	// que explica por que o repasse do proprietário fica menor nos primeiros
	// meses da locação. Opcional: sem ele o demonstrativo segue sem a linha.
	intermediationLookup ownerStatementIntermediationLookup
}

func NewOwnerStatementService(transferRepo transfer.RepositoryInterface, managedBillRepo managed_bill.Repository, journalRepo journal.Repository) *OwnerStatementService {
	return &OwnerStatementService{transferRepo: transferRepo, managedBillRepo: managedBillRepo, journalRepo: journalRepo}
}

// Contract lookup is only needed for legacy transfers without frozen modality.
type ownerStatementContractLookup interface {
	GetContract(context.Context, string, string) (*clients.ContractDTO, error)
}

// ownerStatementIntermediationLookup é o recorte do repositório do plano-base da
// locação usado aqui: ler o plano congelado para reportar saldo de intermediação.
type ownerStatementIntermediationLookup interface {
	GetByContractID(context.Context, string, string) (*rental_charge_config.RentalChargeConfig, error)
}

// WithIntermediationLookup wires o plano de intermediação (opcional).
func (s *OwnerStatementService) WithIntermediationLookup(lookup ownerStatementIntermediationLookup) *OwnerStatementService {
	s.intermediationLookup = lookup
	return s
}

func (s *OwnerStatementService) WithContractLookup(lookup ownerStatementContractLookup) *OwnerStatementService {
	s.contractLookup = lookup
	return s
}

// StatementTransfer adds presentation metadata without changing the stored transfer.
type StatementTransfer struct {
	*transfer.OwnerTransfer
	StatementFlow   string   `json:"statement_flow"`
	BrokerageAmount *float64 `json:"brokerage_amount"`
	// IntermediationAmount é o quanto da intermediação foi descontado do
	// aluguel NESTE repasse (comissão do primeiro aluguel diluída nos primeiros
	// meses). Nil quando o repasse é legado ou não há intermediação — a UI
	// distingue "sem intermediação" de "zero".
	IntermediationAmount *float64 `json:"intermediation_amount"`
	// IntermediationCommissionFundedAmount é a fatia da intermediação que foi
	// para comissões de corretor (o resto é receita da imobiliária).
	IntermediationCommissionFundedAmount *float64 `json:"intermediation_commission_funded_amount"`
	// IntermediationOutstandingAmount é o quanto ainda falta descontar do
	// aluguel para cobrir a intermediação contratada deste contrato.
	IntermediationOutstandingAmount *float64 `json:"intermediation_outstanding_amount"`
}

type StatementFlowSummary struct {
	Flow                  string   `json:"flow"`
	TransferCount         int      `json:"transfer_count"`
	GrossAmount           float64  `json:"gross_amount"`
	AdminFeeAmount        float64  `json:"admin_fee_amount"`
	BrokerageAmount       *float64 `json:"brokerage_amount"`
	MaintenanceDeductions float64  `json:"maintenance_deductions"`
	OtherDeductions       float64  `json:"other_deductions"`
	DiscountAmount        float64  `json:"discount_amount"`
	// IntermediationAmount/IntermediationCommissionFundedAmount explicam a
	// diferença entre o aluguel bruto e o repasse nos meses em que a comissão
	// do primeiro aluguel é diluída — sem eles a taxa de administração sozinha
	// dava a impressão de que o repasse estava incompleto.
	IntermediationAmount                 float64 `json:"intermediation_amount"`
	IntermediationCommissionFundedAmount float64 `json:"intermediation_commission_funded_amount"`
	NetAmount                            float64 `json:"net_amount"`
}

func statementFlow(values ...string) string {
	for _, value := range values {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "sale":
			return "sale"
		case "rent":
			return "rent"
		}
	}
	return "unknown"
}

// RebuildFlowSummaries is also used after combining multiple portal owner leads.
func (st *MonthlyStatement) RebuildFlowSummaries() {
	st.FlowSummaries = []StatementFlowSummary{}
	for _, flow := range []string{"sale", "rent", "unknown"} {
		summary := StatementFlowSummary{Flow: flow}
		brokerage := 0.0
		completeBrokerage := true
		for _, t := range st.Transfers {
			if t.StatementFlow != flow {
				continue
			}
			summary.TransferCount++
			summary.GrossAmount += t.GrossRent
			summary.AdminFeeAmount += t.AdminFeeAmount
			summary.MaintenanceDeductions += t.MaintenanceDeductions
			summary.OtherDeductions += t.OtherDeductions
			summary.NetAmount += t.NetAmount
			if t.BrokerageAmount == nil {
				completeBrokerage = false
			} else {
				brokerage += *t.BrokerageAmount
			}
			if t.EconomicBreakdown != nil {
				summary.DiscountAmount += float64(t.EconomicBreakdown.DiscountCents) / 100
				summary.IntermediationAmount += float64(t.EconomicBreakdown.IntermediationRevenueCents) / 100
				summary.IntermediationCommissionFundedAmount += float64(t.EconomicBreakdown.CommissionFundedCents) / 100
			}
		}
		if flow == "sale" && completeBrokerage {
			summary.BrokerageAmount = &brokerage
		}
		if summary.TransferCount > 0 {
			st.FlowSummaries = append(st.FlowSummaries, summary)
		}
	}
}

// ManagedBillLine é a linha de conta administrada incorporada ao
// demonstrativo — consumo do reservado do proprietário no período.
type ManagedBillLine struct {
	ID          string     `json:"id"`
	Component   string     `json:"component"`
	Creditor    string     `json:"creditor_name"`
	Description string     `json:"description"`
	Amount      float64    `json:"amount"`
	Status      string     `json:"status"`
	DueDate     *time.Time `json:"due_date,omitempty"`
}

// JournalTotals são os totais derivados do ledger (append-only) para o
// contrato do proprietário no período — permitem conferir o demonstrativo
// contra a contabilização real.
type JournalTotals struct {
	Revenue            float64 `json:"revenue"`
	OwnerPayable       float64 `json:"owner_payable"`
	AccountsReceivable float64 `json:"accounts_receivable"`
	Cash               float64 `json:"cash"`
}

// MonthlyStatement é o agregado consumido pelo PDF renderer e pela
// resposta JSON do handler. Soma todos os Transfers do owner em um mês,
// preservando os Transfer individuais para a lista detalhada do PDF.
type MonthlyStatement struct {
	TenantID              string  `json:"tenant_id"`
	OwnerLeadID           string  `json:"owner_lead_id"`
	Year                  int     `json:"year"`
	Month                 int     `json:"month"`
	ReferenceMonth        string  `json:"reference_month"`
	GrossRent             float64 `json:"gross_rent"`
	AdminFeeAmount        float64 `json:"admin_fee_amount"`
	MaintenanceDeductions float64 `json:"maintenance_deductions"`
	OtherDeductions       float64 `json:"other_deductions"`
	NetAmount             float64 `json:"net_amount"`
	// Intermediação do período (descontada do aluguel antes do repasse) e o
	// saldo que ainda será descontado nos próximos meses.
	IntermediationAmount                 float64                `json:"intermediation_amount"`
	IntermediationCommissionFundedAmount float64                `json:"intermediation_commission_funded_amount"`
	IntermediationOutstandingAmount      float64                `json:"intermediation_outstanding_amount"`
	OwnerBalanceTotal                    float64                `json:"owner_balance_total"`
	ReservedAmount                       float64                `json:"reserved_amount"`
	BlockedAmount                        float64                `json:"blocked_amount"`
	AvailableAmount                      float64                `json:"available_amount"`
	BalanceCoveredCount                  int                    `json:"balance_covered_count"`
	ManagedBills                         []ManagedBillLine      `json:"managed_bills"`
	ManagedBillsTotal                    float64                `json:"managed_bills_total"`
	JournalTotals                        JournalTotals          `json:"journal_totals"`
	FlowSummaries                        []StatementFlowSummary `json:"flow_summaries"`
	Transfers                            []*StatementTransfer   `json:"transfers"`
	GeneratedAt                          time.Time              `json:"generated_at"`
}

// GenerateMonthly busca todos os Transfers do owner cujo reference_month
// bata com year/month e devolve o agregado. month 1-12; year >= 2020 por
// sanidade. Sem transfers no período → MonthlyStatement vazio (não erro).
func (s *OwnerStatementService) GenerateMonthly(ctx context.Context, tenantID, ownerLeadID string, year, month int) (*MonthlyStatement, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}
	if ownerLeadID == "" {
		return nil, errors.New("owner_lead_id is required")
	}
	if year < 2020 || year > 2100 {
		return nil, errors.New("year out of range")
	}
	if month < 1 || month > 12 {
		return nil, errors.New("month must be 1..12")
	}
	refMonth := fmt.Sprintf("%04d-%02d", year, month)
	transfers, _, err := s.transferRepo.Search(ctx, tenantID, transfer.SearchFilter{
		OwnerID:        ownerLeadID,
		ReferenceMonth: refMonth,
		Limit:          200, // 200 transfers de um único owner num mês é teto folgado
	})
	if err != nil {
		return nil, err
	}

	stmt := &MonthlyStatement{
		TenantID:       tenantID,
		OwnerLeadID:    ownerLeadID,
		Year:           year,
		Month:          month,
		ReferenceMonth: refMonth,
		Transfers:      []*StatementTransfer{},
		GeneratedAt:    time.Now().UTC(),
	}
	contractFlows := make(map[string]string)
	for _, t := range transfers {
		if t == nil {
			continue
		}
		breakdownFlow := ""
		if t.EconomicBreakdown != nil {
			breakdownFlow = t.EconomicBreakdown.Flow
		}
		flow := statementFlow(t.SettlementType, breakdownFlow)
		if flow == "unknown" && t.ContractID != "" && s.contractLookup != nil {
			cached, ok := contractFlows[t.ContractID]
			if !ok {
				cached = "unknown"
				// Failure to enrich must not fabricate rental income or prevent a statement.
				contract, lookupErr := s.contractLookup.GetContract(ctx, tenantID, t.ContractID)
				if lookupErr == nil && contract != nil {
					cached = statementFlow(contract.Type)
				}
				contractFlows[t.ContractID] = cached
			}
			flow = cached
		}
		line := &StatementTransfer{OwnerTransfer: t, StatementFlow: flow}
		if t.EconomicBreakdown != nil && t.EconomicBreakdown.IntermediationRevenueCents > 0 {
			// Intermediação descontada do aluguel neste repasse. O valor também
			// entra na base do proprietário já líquido (a alocação retira a
			// comissão da cobrança), então reportar aqui é o que explica o
			// repasse menor sem inventar dedução.
			amount := float64(t.EconomicBreakdown.IntermediationRevenueCents) / 100
			funded := float64(t.EconomicBreakdown.CommissionFundedCents) / 100
			line.IntermediationAmount = &amount
			line.IntermediationCommissionFundedAmount = &funded
			stmt.IntermediationAmount += amount
			stmt.IntermediationCommissionFundedAmount += funded
		}
		if flow == "sale" && t.EconomicBreakdown != nil && statementFlow(t.EconomicBreakdown.Flow) == "sale" {
			// Retained agency revenue + broker obligation avoids double-counting funded commissions.
			brokerage := float64(t.EconomicBreakdown.AgencyRetainedCents+t.EconomicBreakdown.CommissionPayableCents) / 100
			line.BrokerageAmount = &brokerage
		}
		stmt.Transfers = append(stmt.Transfers, line)
		stmt.GrossRent += t.GrossRent
		stmt.AdminFeeAmount += t.AdminFeeAmount
		stmt.MaintenanceDeductions += t.MaintenanceDeductions
		stmt.OtherDeductions += t.OtherDeductions
		stmt.NetAmount += t.NetAmount
		if t.OwnerBalance != nil {
			stmt.OwnerBalanceTotal += float64(t.OwnerBalance.TotalCents) / 100
			stmt.ReservedAmount += float64(t.OwnerBalance.ReservedCents) / 100
			stmt.BlockedAmount += float64(t.OwnerBalance.BlockedCents) / 100
			stmt.AvailableAmount += float64(t.OwnerBalance.AvailableCents()) / 100
			stmt.BalanceCoveredCount++
		}
	}

	stmt.RebuildFlowSummaries()

	// Contas administradas do período (consumo do reservado do proprietário,
	// compêndio §21) e totais derivados do ledger (append-only, §10.1.3).
	// Ambos são agrupados por contrato: o demonstrativo de um owner cobre os
	// contratos que geraram repasse no mês.
	contractIDs := make(map[string]struct{})
	for _, t := range stmt.Transfers {
		if t.ContractID != "" {
			contractIDs[t.ContractID] = struct{}{}
		}
	}
	// Saldo de intermediação por contrato (plano-base congelado): quanto ainda
	// será descontado do aluguel nos próximos repasses. Uma leitura por contrato
	// do período, com erro tolerado — o demonstrativo não pode falhar por causa
	// do enriquecimento.
	if s.intermediationLookup != nil {
		for contractID := range contractIDs {
			cfg, err := s.intermediationLookup.GetByContractID(ctx, tenantID, contractID)
			if err != nil || cfg == nil || cfg.IntermediationPlan == nil {
				continue
			}
			outstanding := float64(cfg.IntermediationOutstandingCents()) / 100
			stmt.IntermediationOutstandingAmount += outstanding
			for _, line := range stmt.Transfers {
				if line.ContractID != contractID {
					continue
				}
				value := outstanding
				line.IntermediationOutstandingAmount = &value
			}
		}
	}

	stmt.ManagedBills = []ManagedBillLine{}
	for contractID := range contractIDs {
		if s.managedBillRepo != nil {
			bills, _, err := s.managedBillRepo.List(ctx, tenantID, managed_bill.Filter{
				ContractID:      contractID,
				CompetenceMonth: refMonth,
				Limit:           200,
			})
			if err != nil {
				return nil, fmt.Errorf("list managed bills for contract %s: %w", contractID, err)
			}
			for _, b := range bills {
				if b == nil {
					continue
				}
				due := b.DueDate
				stmt.ManagedBills = append(stmt.ManagedBills, ManagedBillLine{
					ID:          b.ID,
					Component:   b.Component,
					Creditor:    b.Creditor,
					Description: b.Description,
					Amount:      float64(b.AmountCents) / 100,
					Status:      string(b.Status),
					DueDate:     &due,
				})
				stmt.ManagedBillsTotal += float64(b.AmountCents) / 100
			}
		}
		if s.journalRepo != nil {
			entries, _, err := s.journalRepo.List(ctx, tenantID, contractID, 0, 1000)
			if err != nil {
				return nil, fmt.Errorf("list journal entries for contract %s: %w", contractID, err)
			}
			for _, e := range entries {
				if e == nil {
					continue
				}
				for _, leg := range e.Legs {
					sign := int64(1)
					if leg.Direction == journal.Credit {
						sign = -1
					}
					switch leg.Account {
					case journal.AccountRevenue:
						stmt.JournalTotals.Revenue += float64(leg.AmountCents*sign) / 100
					case journal.AccountOwnerPayable:
						stmt.JournalTotals.OwnerPayable += float64(leg.AmountCents*sign) / 100
					case journal.AccountAccountsReceivable:
						stmt.JournalTotals.AccountsReceivable += float64(leg.AmountCents*sign) / 100
					case journal.AccountCash:
						stmt.JournalTotals.Cash += float64(leg.AmountCents*sign) / 100
					}
				}
			}
		}
	}
	return stmt, nil
}

// OwnerBalanceSummary é o saldo corrente do proprietário para o widget fixo:
// total/reservado/bloqueado/disponível derivados do repasse allocation-aware
// mais recente (ações de conta administrada já reaplicadas), sem precisar
// agregar o demonstrativo de um mês específico.
type OwnerBalanceSummary struct {
	TenantID       string    `json:"tenant_id"`
	OwnerLeadID    string    `json:"owner_lead_id"`
	TransferID     string    `json:"transfer_id"`
	ReferenceMonth string    `json:"reference_month"`
	TotalCents     int64     `json:"total_cents"`
	ReservedCents  int64     `json:"reserved_cents"`
	BlockedCents   int64     `json:"blocked_cents"`
	AvailableCents int64     `json:"available_cents"`
	HasBalance     bool      `json:"has_balance"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// LatestBalance devolve o OwnerBalance do transfer mais recente do owner
// (Search ordena por created_at DESC). Rows legadas sem allocation retornam
// HasBalance=false e zeros — o widget então mostra estado vazio em vez de
// inventar saldo.
func (s *OwnerStatementService) LatestBalance(ctx context.Context, tenantID, ownerLeadID string) (*OwnerBalanceSummary, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}
	if ownerLeadID == "" {
		return nil, errors.New("owner_lead_id is required")
	}
	// Busca algumas rows: o reorder/filtro pode ter criado um transfer
	// "mais novo" sem o subledger (ex.: rascunho), então prefere o mais
	// recente que carregue OwnerBalance/base.
	transfers, _, err := s.transferRepo.Search(ctx, tenantID, transfer.SearchFilter{
		OwnerID: ownerLeadID,
		Limit:   20,
	})
	if err != nil {
		return nil, err
	}
	summary := &OwnerBalanceSummary{
		TenantID:    tenantID,
		OwnerLeadID: ownerLeadID,
	}
	for _, t := range transfers {
		baseTotal, baseReserved := t.OwnerBalanceBaseTotalCents, t.OwnerBalanceBaseReservedCents
		if baseTotal <= 0 && baseReserved <= 0 && t.OwnerBalance != nil {
			// Rolling-deployment compat: rows anteriores ao campo base.
			baseTotal, baseReserved = t.OwnerBalance.TotalCents, t.OwnerBalance.ReservedCents
		}
		if t.OwnerBalance == nil && baseTotal <= 0 && baseReserved <= 0 {
			continue
		}
		balance, err := t.ExpectedOwnerBalanceForBase(baseTotal, baseReserved)
		if err != nil {
			// Dados inconsistentes em uma row: parte para a anterior em vez
			// de derrubar o widget inteiro.
			continue
		}
		summary.TransferID = t.ID
		summary.ReferenceMonth = t.ReferenceMonth
		summary.TotalCents = balance.TotalCents
		summary.ReservedCents = balance.ReservedCents
		summary.BlockedCents = balance.BlockedCents
		summary.AvailableCents = balance.AvailableCents()
		summary.HasBalance = true
		summary.UpdatedAt = t.UpdatedAt
		return summary, nil
	}
	// Nenhuma row com subledger: usa o transfer mais recente (se houver)
	// apenas para timestamps e reporta HasBalance=false.
	if len(transfers) > 0 {
		summary.TransferID = transfers[0].ID
		summary.ReferenceMonth = transfers[0].ReferenceMonth
		summary.UpdatedAt = transfers[0].UpdatedAt
	}
	return summary, nil
}
