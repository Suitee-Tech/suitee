package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/tax_obligation"
)

// TaxObligationService dá vida ao tributo retido na comissão.
//
// O plano de contas tem "Tributos retidos a recolher" (2.1.05) e a liquidação da
// comissão credita essa conta pelos descontos de IRRF/INSS/ISS. Faltava o
// passivo ter existência: quanto foi retido, de quem, em que competência, até
// quando recolher e quando foi recolhido. Sem isso o valor ficava parado na
// conta contábil, o operador não tinha o que pagar e a imobiliária não conseguia
// provar o recolhimento.
//
// A obrigação é DERIVADA da comissão (itens = desconto que a originou), então
// toda operação aqui é idempotente e reconstruível: `SyncCommission` recalcula a
// partir do estado atual da comissão e pode ser chamada quantas vezes for
// preciso (retry, correção, ou uma passada de reparo).
type TaxObligationService struct {
	repo tax_obligation.Repository
	// commissions é a fonte da verdade dos descontos (a obrigação é derivada).
	commissions commissionLookup
	journal     *JournalService
	now         func() time.Time
}

// NewTaxObligationService recebe o repositório das obrigações e o das comissões
// (de onde os descontos são derivados).
func NewTaxObligationService(repo tax_obligation.Repository, commissions commissionLookup) *TaxObligationService {
	return &TaxObligationService{repo: repo, commissions: commissions, now: time.Now}
}

// WithJournal liga o lançamento contábil da baixa (débito em tributos retidos /
// crédito em caixa). Sem ele a baixa é registrada, mas o razão não é fechado.
func (s *TaxObligationService) WithJournal(journalService *JournalService) *TaxObligationService {
	s.journal = journalService
	return s
}

// ApplyDeduction registra o desconto de uma comissão na obrigação da competência.
// Chamado dentro do fluxo que lança o desconto.
func (s *TaxObligationService) ApplyDeduction(ctx context.Context, c *commission.Commission, d commission.Deduction) error {
	if s == nil || s.repo == nil || c == nil {
		return nil
	}
	kind, ok := taxKindForDeduction(d.Kind)
	if !ok {
		return fmt.Errorf("tax obligation: tipo de desconto desconhecido %q", d.Kind)
	}
	return s.applyItems(ctx, c, []commission.Deduction{d}, kind)
}

// SyncCommission recalcula as obrigações desta comissão a partir dos descontos
// atuais: apaga o que ela tinha e relança. É o caminho de reparo e o que garante
// que remoção de desconto (ou mudança de valor) não deixe obrigação fantasma.
func (s *TaxObligationService) SyncCommission(ctx context.Context, tenantID, commissionID string) error {
	if s == nil || s.repo == nil || strings.TrimSpace(commissionID) == "" {
		return nil
	}
	if s.commissions == nil {
		return errors.New("tax obligation service requires a commission lookup")
	}
	c, err := s.commissions.GetByID(ctx, tenantID, commissionID)
	if err != nil || c == nil {
		return err
	}
	return s.syncCommissionObligations(ctx, c)
}

// commissionLookup é o mínimo que o serviço precisa da comissão.
type commissionLookup interface {
	GetByID(ctx context.Context, tenantID, id string) (*commission.Commission, error)
	List(ctx context.Context, tenantID string, limit, offset int64) ([]*commission.Commission, int64, error)
}

// Recompute reconstrói as obrigações a partir das comissões do tenant — a
// ferramenta de reparo de uma sincronização que falhou (o desconto está gravado
// na comissão, então a obrigação é sempre derivável). Sem `commissionID`,
// reprocessa as comissões recentes.
func (s *TaxObligationService) Recompute(ctx context.Context, tenantID, commissionID string) (int, error) {
	if s == nil || s.repo == nil {
		return 0, nil
	}
	if s.commissions == nil {
		return 0, errors.New("tax obligation service requires a commission lookup")
	}
	lookup := s.commissions
	if strings.TrimSpace(commissionID) != "" {
		if err := s.SyncCommission(ctx, tenantID, commissionID); err != nil {
			return 0, err
		}
		return 1, nil
	}
	commissions, _, err := lookup.List(ctx, tenantID, 200, 0)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, c := range commissions {
		if c == nil {
			continue
		}
		if err := s.syncCommissionObligations(ctx, c); err != nil {
			return processed, fmt.Errorf("comissão %s: %w", c.ID, err)
		}
		processed++
	}
	return processed, nil
}

// RemoveCommission registra que a comissão saiu (cancelamento/estorno) — os itens
// dela deixam de contar como retenção ativa.
func (s *TaxObligationService) RemoveCommission(ctx context.Context, c *commission.Commission, reason string) error {
	if s == nil || s.repo == nil || c == nil {
		return nil
	}
	for _, kind := range taxKindsForDeductions(c.Deductions) {
		competences := competencesForKind(c.Deductions, kind)
		for _, competence := range competences {
			obligation, err := s.repo.GetByKindCompetence(ctx, c.TenantID, kind, competence)
			if err != nil || obligation == nil {
				if err != nil {
					return err
				}
				continue
			}
			if _, err := obligation.RemoveCommissionItems(c.ID, s.now()); err != nil {
				return err
			}
			if err := s.persist(ctx, obligation); err != nil {
				return err
			}
		}
	}
	_ = reason
	return nil
}

// EnsureDeductionChangeAllowed barra mudança em desconto cujo tributo já foi
// RECOLHIDO.
//
// O ciclo do dinheiro é: desconto → obrigação aberta → guia recolhida. Depois do
// recolhimento, mexer no desconto (remover, ou lançar outro na mesma
// competência) deixaria o razão contando um imposto que não corresponde mais ao
// que foi retido do corretor. O caminho para desfazer é estorno, não edição.
func (s *TaxObligationService) EnsureDeductionChangeAllowed(ctx context.Context, tenantID string, kind commission.DeductionKind, competence string) error {
	if s == nil || s.repo == nil {
		return nil
	}
	taxKind, ok := taxKindForDeduction(kind)
	if !ok || strings.TrimSpace(competence) == "" {
		return nil
	}
	obligation, err := s.repo.GetByKindCompetence(ctx, tenantID, taxKind, competence)
	if err != nil {
		return err
	}
	if obligation == nil || obligation.Status != tax_obligation.StatusPaid {
		return nil
	}
	return fmt.Errorf("%w: %s da competência %s já foi recolhido — estorne o recolhimento antes de mexer no desconto",
		tax_obligation.ErrAlreadySettled, taxKind, competence)
}

// Pay dá baixa na obrigação: registra o comprovante do recolhimento e fecha o
// razão (débito em tributos retidos / crédito em caixa). Idempotente pelo
// source_event_id do lançamento, então um retry não recolhe duas vezes.
func (s *TaxObligationService) Pay(ctx context.Context, tenantID, id, actor, proofURL, proofFilename, note string) (*tax_obligation.Obligation, error) {
	obligation, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if obligation == nil {
		return nil, tax_obligation.ErrNotFound
	}
	expected := obligation.UpdatedAt
	if err := obligation.Pay(actor, proofURL, proofFilename, note, s.now()); err != nil {
		return nil, err
	}
	if s.journal != nil && obligation.JournalEntryID == "" {
		entry, err := s.buildSettlementEntry(obligation, actor)
		if err != nil {
			return nil, err
		}
		sourceEventID := "tax_obligation.settled:" + obligation.ID
		created, err := s.journal.CreateCanonicalWithSourceEventID(ctx, sourceEventID, entry)
		if err != nil {
			return nil, fmt.Errorf("registrar recolhimento no razão: %w", err)
		}
		if created != nil {
			obligation.JournalEntryID = created.ID
		}
	}
	if err := s.repo.Update(ctx, obligation, expected); err != nil {
		return nil, err
	}
	return obligation, nil
}

// buildSettlementEntry monta o lançamento do recolhimento: a obrigação
// (2.1.05 — tributos retidos a recolher) é baixada contra o caixa.
func (s *TaxObligationService) buildSettlementEntry(o *tax_obligation.Obligation, actor string) (*journal.Entry, error) {
	if o.AmountCents <= 0 {
		return nil, fmt.Errorf("obrigação %s sem valor a recolher", o.ID)
	}
	paidAt := s.now()
	if o.PaidAt != nil && !o.PaidAt.IsZero() {
		paidAt = *o.PaidAt
	}
	entry, err := journal.New(o.TenantID, "", fmt.Sprintf("Recolhimento de %s — competência %s", o.Kind, o.Competence), paidAt, []journal.Leg{
		{Account: journal.AccountTaxWithheld, Direction: journal.Debit, AmountCents: o.AmountCents},
		{Account: journal.AccountCash, Direction: journal.Credit, AmountCents: o.AmountCents},
	})
	if err != nil {
		return nil, err
	}
	entry.OperationID = o.ID
	return entry, nil
}

func (s *TaxObligationService) List(ctx context.Context, tenantID string, filter tax_obligation.ListFilter) ([]*tax_obligation.Obligation, error) {
	return s.repo.List(ctx, tenantID, filter)
}

func (s *TaxObligationService) Totals(ctx context.Context, tenantID string, filter tax_obligation.ListFilter) (*tax_obligation.Totals, error) {
	return s.repo.Totals(ctx, tenantID, filter)
}

func (s *TaxObligationService) GetByID(ctx context.Context, tenantID, id string) (*tax_obligation.Obligation, error) {
	return s.repo.GetByID(ctx, tenantID, id)
}

// applyItems lança os descontos de um tipo na obrigação da competência,
// criando-a quando ainda não existe.
func (s *TaxObligationService) applyItems(ctx context.Context, c *commission.Commission, deductions []commission.Deduction, kind tax_obligation.Kind) error {
	for _, deduction := range deductions {
		competence := competenceFor(c, deduction)
		obligation, err := s.repo.GetByKindCompetence(ctx, c.TenantID, kind, competence)
		if err != nil {
			return err
		}
		if obligation == nil {
			obligation, err = tax_obligation.New(c.TenantID, kind, competence, s.now())
			if err != nil {
				return err
			}
			if _, err := obligation.ApplyItem(itemFor(c, deduction, competence), s.now()); err != nil {
				return err
			}
			if err := s.repo.Create(ctx, obligation); err != nil {
				return err
			}
			continue
		}
		changed, err := obligation.ApplyItem(itemFor(c, deduction, competence), s.now())
		if err != nil {
			return err
		}
		if !changed {
			continue
		}
		if err := s.persist(ctx, obligation); err != nil {
			return err
		}
	}
	return nil
}

// syncCommissionObligations alinha as obrigações ao estado ATUAL da comissão:
// remove tudo que ela tinha e relança o que existe hoje.
func (s *TaxObligationService) syncCommissionObligations(ctx context.Context, c *commission.Commission) error {
	// Primeiro tira a comissão de todas as obrigações abertas do tenant, para o
	// caso de o desconto ter mudado de tipo (IRRF → ISS) ou de competência.
	open := []tax_obligation.Status{tax_obligation.StatusOpen}
	obligations, err := s.repo.List(ctx, c.TenantID, tax_obligation.ListFilter{Status: open, Limit: 200})
	if err != nil {
		return err
	}
	for _, obligation := range obligations {
		removed, err := obligation.RemoveCommissionItems(c.ID, s.now())
		if err != nil {
			return err
		}
		if removed == 0 {
			continue
		}
		if err := s.persist(ctx, obligation); err != nil {
			return err
		}
	}
	for _, kind := range taxKindsForDeductions(c.Deductions) {
		if err := s.applyItems(ctx, c, deductionsOfKind(c.Deductions, kind), kind); err != nil {
			return err
		}
	}
	return nil
}

func (s *TaxObligationService) persist(ctx context.Context, obligation *tax_obligation.Obligation) error {
	expected := obligation.UpdatedAt
	if err := s.repo.Update(ctx, obligation, expected); err != nil {
		// A revisão pode ter mudado (baixa concorrente): relê e tenta de novo uma
		// vez, para uma baixa legítima não derrubar a recomputação.
		current, getErr := s.repo.GetByID(ctx, obligation.TenantID, obligation.ID)
		if getErr != nil || current == nil {
			return err
		}
		if current.UpdatedAt.Equal(expected) {
			return err
		}
		reloaded := *obligation
		reloaded.UpdatedAt = current.UpdatedAt
		return s.repo.Update(ctx, &reloaded, current.UpdatedAt)
	}
	return nil
}

func itemFor(c *commission.Commission, d commission.Deduction, competence string) tax_obligation.Item {
	at := d.CreatedAt
	if at.IsZero() {
		at = time.Now()
	}
	return tax_obligation.Item{
		CommissionID: c.ID,
		DeductionID:  d.ID,
		ContractID:   c.ContractID,
		BrokerID:     c.BrokerID,
		AmountCents:  int64(math.Round(d.Amount * 100)),
		Competence:   competence,
		At:           at.UTC().Truncate(time.Microsecond),
	}
}

// competenceFor é a competência do lançamento: o mês em que o desconto foi
// registrado (carimbado uma vez, em CreatedAt). Sem o carimbo, uma recomputação
// jogaria o mesmo desconto em competências diferentes.
func competenceFor(c *commission.Commission, d commission.Deduction) string {
	at := d.CreatedAt
	if at.IsZero() {
		at = c.UpdatedAt
	}
	if at.IsZero() {
		at = time.Now()
	}
	return tax_obligation.CompetenceOf(at)
}

func taxKindForDeduction(kind commission.DeductionKind) (tax_obligation.Kind, bool) {
	switch kind {
	case commission.DeductionIRRF:
		return tax_obligation.KindIRRF, true
	case commission.DeductionINSS:
		return tax_obligation.KindINSS, true
	case commission.DeductionISS:
		return tax_obligation.KindISS, true
	case commission.DeductionOutros:
		return tax_obligation.KindOutros, true
	default:
		return "", false
	}
}

func taxKindsForDeductions(deductions []commission.Deduction) []tax_obligation.Kind {
	seen := map[tax_obligation.Kind]bool{}
	out := make([]tax_obligation.Kind, 0, 4)
	for _, d := range deductions {
		kind, ok := taxKindForDeduction(d.Kind)
		if !ok || seen[kind] {
			continue
		}
		seen[kind] = true
		out = append(out, kind)
	}
	return out
}

func deductionsOfKind(deductions []commission.Deduction, kind tax_obligation.Kind) []commission.Deduction {
	out := make([]commission.Deduction, 0, len(deductions))
	for _, d := range deductions {
		if mapped, ok := taxKindForDeduction(d.Kind); ok && mapped == kind {
			out = append(out, d)
		}
	}
	return out
}

func competencesForKind(deductions []commission.Deduction, kind tax_obligation.Kind) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(deductions))
	for _, d := range deductions {
		if mapped, ok := taxKindForDeduction(d.Kind); !ok || mapped != kind {
			continue
		}
		competence := tax_obligation.CompetenceOf(d.CreatedAt)
		if seen[competence] {
			continue
		}
		seen[competence] = true
		out = append(out, competence)
	}
	return out
}
