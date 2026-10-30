package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/pkg/money"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// Portal proposal decisions are isolated from the core lifecycle because they
// coordinate participant authorization, counters and client notifications.

func (s *DealService) AcceptMyDealProposal(ctx context.Context, accountID, email, dealID, proposalID string) (*deal.Deal, error) {
	foundDeal, err := s.findMyDeal(ctx, accountID, email, dealID)
	if err != nil {
		return nil, err
	}

	d, err := s.portal().GetByID(ctx, foundDeal.TenantID, foundDeal.ID)
	if err != nil {
		return nil, err
	}

	d.EnsurePartiesFromLegacy()

	myParty := (*deal.Party)(nil)
	if accountID != "" {
		for i := range d.Parties {
			if d.Parties[i].AccountID == accountID {
				myParty = &d.Parties[i]
				break
			}
		}
	}
	if myParty == nil && email != "" {
		for i := range d.Parties {
			if strings.EqualFold(d.Parties[i].Email, email) {
				myParty = &d.Parties[i]
				break
			}
		}
	}
	if myParty == nil {
		return nil, errors.New("party not found")
	}
	acceptingParty := myParty
	if myParty.Role == deal.PartyRoleAttorney || myParty.Role == deal.PartyRoleLegalRep || myParty.Role == deal.PartyRoleGuardian || myParty.Role == deal.PartyRoleCurator {
		principal := (*deal.Party)(nil)
		for i := range d.Parties {
			p := &d.Parties[i]
			if p.Representation == nil {
				continue
			}
			if p.Representation.RepresentativePartyID != myParty.ID {
				continue
			}
			if !p.Representation.AllowRepresentativeProposal {
				continue
			}
			if p.Role != deal.PartyRoleTenant && p.Role != deal.PartyRoleBuyer && p.Role != deal.PartyRoleOwner {
				continue
			}
			if !p.IsPrimary {
				continue
			}
			principal = p
			break
		}
		if principal == nil {
			return nil, errors.New("party role cannot accept proposals")
		}
		acceptingParty = principal
	}

	if acceptingParty.Role != deal.PartyRoleTenant && acceptingParty.Role != deal.PartyRoleBuyer && acceptingParty.Role != deal.PartyRoleOwner {
		return nil, errors.New("party role cannot accept proposals")
	}
	if !acceptingParty.IsSignatureRequired() {
		return nil, errors.New("only required parties can accept proposals")
	}

	found := false
	var acceptedProposal *deal.Proposal
	for i := range d.Proposals {
		if d.Proposals[i].ID == proposalID {
			already := false
			for _, a := range d.Proposals[i].AcceptedBy {
				if a.PartyID == acceptingParty.ID {
					already = true
					break
				}
			}
			if !already {
				d.Proposals[i].AcceptedBy = append(d.Proposals[i].AcceptedBy, deal.ProposalAcceptance{
					PartyID:    acceptingParty.ID,
					Role:       acceptingParty.Role,
					AcceptedAt: time.Now(),
				})
			}
			acceptedProposal = &d.Proposals[i]
			found = true
			break
		}
	}

	if !found {
		return nil, errors.New("proposal not found")
	}

	customerRole := deal.PartyRoleTenant
	if d.Type == deal.TypeSale {
		customerRole = deal.PartyRoleBuyer
	}
	required := []string{}
	for i := range d.Parties {
		p := &d.Parties[i]
		if !p.IsSignatureRequired() {
			continue
		}
		if p.Role != customerRole && p.Role != deal.PartyRoleOwner {
			continue
		}
		required = append(required, p.ID)
	}

	hasAcceptance := func(partyID string) bool {
		if acceptedProposal == nil {
			return false
		}
		for _, a := range acceptedProposal.AcceptedBy {
			if a.PartyID == partyID {
				return true
			}
		}
		return false
	}

	allAccepted := len(required) > 0
	for _, id := range required {
		if id == "" || !hasAcceptance(id) {
			allAccepted = false
			break
		}
	}

	if allAccepted {
		acceptedProposal.Status = deal.ProposalStatusAccepted

		// Quando todas as partes obrigatórias aceitam, o valor do deal
		// é atualizado para o valor da proposta aceita. Isso garante que
		// cobranças, comissões e o board da esteira reflitam o valor
		// efetivamente negociado (e não o valor inicial do cadastro).
		if acceptedProposal.Value != d.Value {
			beforeDealValue := d.Value
			d.Value = acceptedProposal.Value

			d.AddHistory(d.Status, email, fmt.Sprintf(
				"Deal value updated to %s upon full proposal acceptance (proposal=%s)",
				money.BRL(acceptedProposal.Value), proposalID))

			audit.Record(ctx, audit.Entry{
				Action:     "deal.value_updated",
				EntityType: "Deal",
				EntityID:   d.ID,
				Before: map[string]any{
					"deal_value": beforeDealValue,
				},
				After: map[string]any{
					"deal_value": d.Value,
				},
				Metadata: map[string]any{
					"proposal_id": proposalID,
					"value_delta": d.Value - beforeDealValue,
					"trigger":     "proposal_fully_accepted",
					"actor":       email,
				},
			})
		}
	}

	// Before snapshot — #471. Captura status do proposal antes da aceitação
	// para o audit.events ter o diff completo (pending → accepted) e
	// preservar o accepted_by anterior (caso seja o último aceite que
	// transiciona o status do proposal).
	var beforeProposalStatus deal.ProposalStatus
	var beforeAcceptedByCount int
	if acceptedProposal != nil {
		// acceptedProposal aponta para o slice mutável; captura snapshot
		// PRÉ mutação ler a posição original no slice (mas como acima já
		// mutamos d.Proposals[i].AcceptedBy, precisamos contabilizar a
		// nova entrada como pós). Aqui o "before" é construído a partir
		// dos dados antes do append — len-1 desfaz o crescimento.
		beforeProposalStatus = acceptedProposal.Status
		beforeAcceptedByCount = len(acceptedProposal.AcceptedBy) - 1
		if beforeAcceptedByCount < 0 {
			beforeAcceptedByCount = 0
		}
	}

	d.AddHistory(d.Status, email, "Proposal accepted by client")

	if err := s.portal().Update(ctx, d); err != nil {
		return nil, err
	}

	if s.eventBus != nil {
		evt := contracts.ProposalRespondedEvent{
			Data: contracts.ProposalRespondedEventPayload{
				DealID:            d.ID,
				TenantID:          d.TenantID,
				ProposalID:        proposalID,
				Action:            "accepted",
				CustomerAccountID: accountID,
				DealValue:         &d.Value,
			},
		}
		if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
			log.Printf("WARN: failed to enqueue proposal.responded (accepted) for deal=%s proposal=%s: %v", d.ID, proposalID, err)
		}
	}

	// Audit trail (#471) — proposal acceptance é juridicamente importante
	// (acordo de vontade). Before/after compactos: status do proposal +
	// count de aceites, sem expor PII das parties. allAccepted no metadata
	// permite filtrar rapidamente proposals que efetivamente fecharam.
	if acceptedProposal != nil {
		audit.Record(ctx, audit.Entry{
			Action:     "proposal.accepted",
			EntityType: "Deal",
			EntityID:   d.ID,
			Before: map[string]any{
				"proposal_status":   string(beforeProposalStatus),
				"accepted_by_count": beforeAcceptedByCount,
			},
			After: map[string]any{
				"proposal_status":   string(acceptedProposal.Status),
				"accepted_by_count": len(acceptedProposal.AcceptedBy),
			},
			Metadata: map[string]any{
				"proposal_id":   proposalID,
				"party_id":      acceptingParty.ID,
				"party_role":    string(acceptingParty.Role),
				"all_accepted":  allAccepted,
				"actor_account": accountID,
				"source":        "client_portal",
			},
		})
	}

	return d, nil
}

// DeleteProposal removes a proposal from a deal
func (s *DealService) DeleteProposal(ctx context.Context, tenantID, dealID, proposalID string) error {
	d, err := s.repo.GetByID(ctx, tenantID, dealID)
	if err != nil {
		return err
	}
	if d == nil {
		return errors.New("deal not found")
	}

	found := false
	var updatedProposals []deal.Proposal
	for _, p := range d.Proposals {
		if p.ID == proposalID {
			found = true
		} else {
			updatedProposals = append(updatedProposals, p)
		}
	}

	if !found {
		return errors.New("proposal not found")
	}

	d.Proposals = updatedProposals
	d.UpdatedAt = time.Now()

	return s.repo.Update(ctx, d)
}

func (s *DealService) RejectMyDealProposal(ctx context.Context, accountID, email, dealID, proposalID, reason string, counterValue *float64) (*deal.Deal, error) {
	foundDeal, err := s.findMyDeal(ctx, accountID, email, dealID)
	if err != nil {
		return nil, err
	}

	d, err := s.portal().GetByID(ctx, foundDeal.TenantID, foundDeal.ID)
	if err != nil {
		return nil, err
	}

	// Snapshot do proposal antes da mutação para o audit (#471).
	var beforeProposalStatus deal.ProposalStatus
	var beforeCounterValue *float64
	for i := range d.Proposals {
		if d.Proposals[i].ID == proposalID {
			beforeProposalStatus = d.Proposals[i].Status
			beforeCounterValue = d.Proposals[i].CounterValue
			break
		}
	}

	found := false
	action := "rejected"
	for i := range d.Proposals {
		if d.Proposals[i].ID == proposalID {
			if counterValue != nil {
				d.Proposals[i].Status = deal.ProposalStatusCounter
				d.Proposals[i].CounterValue = counterValue
				action = "counter_offer"
			} else {
				d.Proposals[i].Status = deal.ProposalStatusRejected
			}
			if reason != "" {
				d.Proposals[i].Notes = reason
			}
			found = true
			break
		}
	}

	if !found {
		return nil, errors.New("proposal not found")
	}

	d.AddHistory(d.Status, email, "Proposal rejected by client")

	if err := s.portal().Update(ctx, d); err != nil {
		return nil, err
	}

	if s.eventBus != nil {
		evt := contracts.ProposalRespondedEvent{
			Data: contracts.ProposalRespondedEventPayload{
				DealID:            d.ID,
				TenantID:          d.TenantID,
				ProposalID:        proposalID,
				Action:            action,
				CustomerAccountID: accountID,
			},
		}
		if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
			log.Printf("WARN: failed to enqueue proposal.responded (%s) for deal=%s proposal=%s: %v", action, d.ID, proposalID, err)
		}
	}

	// Audit trail (#471). Rejection / counter-offer são juridicamente
	// importantes (contraproposta vincula intenção de negociar). reason
	// fica em Metadata (não em After) — texto livre pode ter PII.
	var afterProposalStatus deal.ProposalStatus
	var afterCounterValue *float64
	for i := range d.Proposals {
		if d.Proposals[i].ID == proposalID {
			afterProposalStatus = d.Proposals[i].Status
			afterCounterValue = d.Proposals[i].CounterValue
			break
		}
	}
	auditEntry := audit.Entry{
		Action:     "proposal." + action,
		EntityType: "Deal",
		EntityID:   d.ID,
		Before: map[string]any{
			"proposal_status": string(beforeProposalStatus),
			"counter_value":   beforeCounterValue,
		},
		After: map[string]any{
			"proposal_status": string(afterProposalStatus),
			"counter_value":   afterCounterValue,
		},
		Metadata: map[string]any{
			"proposal_id":   proposalID,
			"actor_account": accountID,
			"source":        "client_portal",
			"has_reason":    reason != "",
		},
	}
	audit.Record(ctx, auditEntry)

	return d, nil
}

// ErrProposalNotInCounterOffer indica que o caller pediu accept-counter-offer
// numa proposal que NÃO está em status COUNTER_OFFER — fluxo legacy
// ainda usaria AcceptMyDealProposal. Mantém o endpoint dedicado claro
// (admin só usa quando há contraproposta pendente). Mensagem em inglês.
var ErrProposalNotInCounterOffer = errors.New("proposal is not in COUNTER_OFFER status")

// AcceptCounterOffer é o endpoint admin/operador dedicado (#424) para
// aceitar uma contraproposta enviada pelo cliente. Diferente de
// AcceptMyDealProposal (que é o lado do cliente aceitando), este é
// chamado pelo gestor/admin para fechar a negociação ao valor da
// contraproposta — fluxo típico do J02 etapa contraproposta.
//
// Comportamento:
//
//   - Valida proposal existe e status == COUNTER_OFFER
//   - Atualiza Deal.Value para CounterValue (valor agora aceito)
//   - Marca proposal.Status = ACCEPTED
//   - Audit "proposal.counter_offer_accepted" com before/after + diff de
//     valor (transparência forense — quem aceitou, quando, por quanto)
//   - Publica ProposalRespondedEvent para downstream (toca-comm notifica
//     cliente, toca-financial inicia comissão, etc.)
//
// actorID/actorEmail vêm do JWT do gestor — usado para audit. dealID +
// proposalID identificam a linha exata.
func (s *DealService) AcceptCounterOffer(ctx context.Context, tenantID, dealID, proposalID, actorID, actorEmail string) (*deal.Deal, error) {
	d, err := s.repo.GetByID(ctx, tenantID, dealID)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, errors.New("deal not found")
	}

	// Snapshot antes da mutação para audit diff (#471 pattern).
	var beforeProposalStatus deal.ProposalStatus
	var beforeCounterValue *float64
	beforeDealValue := d.Value
	proposalIdx := -1
	for i := range d.Proposals {
		if d.Proposals[i].ID == proposalID {
			proposalIdx = i
			beforeProposalStatus = d.Proposals[i].Status
			beforeCounterValue = d.Proposals[i].CounterValue
			break
		}
	}
	if proposalIdx < 0 {
		return nil, errors.New("proposal not found")
	}

	if beforeProposalStatus != deal.ProposalStatusCounter {
		return nil, fmt.Errorf("%w: proposal_id=%s current_status=%s",
			ErrProposalNotInCounterOffer, proposalID, beforeProposalStatus)
	}

	// Aceitar contraproposta → status ACCEPTED, novo valor do deal vem
	// do CounterValue (cliente disse "eu pago X"; agora vendedor concorda).
	// Não populamos AcceptedBy aqui — esse slice é para parties (tenant/
	// buyer/owner) assinando o aceite; admin aceitando em nome da
	// imobiliária é gravado no audit + history, não como party.
	d.Proposals[proposalIdx].Status = deal.ProposalStatusAccepted
	if beforeCounterValue != nil && *beforeCounterValue > 0 {
		d.Value = *beforeCounterValue
	}

	d.AddHistory(d.Status, actorID, fmt.Sprintf("Counter-offer accepted (proposal=%s, new value=%s)",
		proposalID, money.BRL(d.Value)))

	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}

	// Audit dedicado — distinto de proposal.accepted para que o /audit-trail
	// possa filtrar especificamente aceites de contrapropostas e analisar
	// margem de negociação (delta entre Value original e contraproposta).
	audit.Record(ctx, audit.Entry{
		Action:     "proposal.counter_offer_accepted",
		EntityType: "Deal",
		EntityID:   d.ID,
		Before: map[string]any{
			"proposal_status": string(beforeProposalStatus),
			"deal_value":      beforeDealValue,
			"counter_value":   beforeCounterValue,
		},
		After: map[string]any{
			"proposal_status": string(d.Proposals[proposalIdx].Status),
			"deal_value":      d.Value,
		},
		Metadata: map[string]any{
			"proposal_id":   proposalID,
			"actor_account": actorID,
			"actor_email":   actorEmail,
			"source":        "admin_endpoint",
			"value_delta":   d.Value - beforeDealValue,
		},
	})

	if s.eventBus != nil {
		evt := contracts.ProposalRespondedEvent{
			Data: contracts.ProposalRespondedEventPayload{
				DealID:            d.ID,
				TenantID:          d.TenantID,
				ProposalID:        proposalID,
				Action:            "counter_offer_accepted",
				CustomerAccountID: "",
				DealValue:         &d.Value,
			},
		}
		if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
			log.Printf("WARN: failed to enqueue proposal.responded (counter_offer_accepted) for deal=%s proposal=%s: %v",
				d.ID, proposalID, err)
		}
	}

	return d, nil
}
