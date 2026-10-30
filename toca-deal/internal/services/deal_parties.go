package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// Party, signer and due-diligence mutations share the same aggregate boundary
// and remain together so identity projections stay discoverable.

func (s *DealService) UpdateDueDiligence(ctx context.Context, tenantID, dealID, brokerID string, score *int, cndStatus, legalOpinion string, monthlyIncome *int, status string, userID string) (*deal.Deal, error) {
	d, err := s.getDealByScope(ctx, tenantID, dealID, brokerID)
	if err != nil {
		return nil, err
	}

	customerRole := deal.PartyRoleTenant
	if d.Type == deal.TypeSale {
		customerRole = deal.PartyRoleBuyer
	}
	primary := d.PrimaryByRole(customerRole)
	if primary == nil {
		return nil, errors.New("primary party not found")
	}

	oldStatus, newStatus, err := d.UpdatePartyDueDiligence(primary.ID, score, cndStatus, legalOpinion, monthlyIncome, status)
	if err != nil {
		return nil, err
	}

	d.DueDiligence = d.AggregatedDueDiligence()
	d.AddHistory(d.Status, userID, "Due Diligence updated")
	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}

	if s.eventBus != nil {
		accountID := ""
		if p := d.PrimaryByRole(customerRole); p != nil {
			accountID = p.AccountID
		}
		evt := contracts.DealPartyDueDiligenceUpdatedEvent{
			Data: contracts.DealPartyDueDiligenceUpdatedEventPayload{
				DealID:    d.ID,
				PartyID:   primary.ID,
				TenantID:  d.TenantID,
				AccountID: accountID,
				OldStatus: oldStatus,
				NewStatus: newStatus,
				UpdatedAt: time.Now().Format(time.RFC3339),
			},
		}
		if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
			log.Printf("WARN: failed to enqueue deal.party.due_diligence.updated (primary) for deal=%s party=%s: %v", d.ID, primary.ID, err)
		}
	}
	return d, nil
}

func (s *DealService) UpdatePartyDueDiligence(ctx context.Context, tenantID, dealID, brokerID, partyID string, score *int, cndStatus, legalOpinion string, monthlyIncome *int, status string, userID string) (*deal.Deal, error) {
	d, err := s.getDealByScope(ctx, tenantID, dealID, brokerID)
	if err != nil {
		return nil, err
	}

	oldStatus, newStatus, err := d.UpdatePartyDueDiligence(partyID, score, cndStatus, legalOpinion, monthlyIncome, status)
	if err != nil {
		return nil, err
	}

	d.DueDiligence = d.AggregatedDueDiligence()
	d.AddHistory(d.Status, userID, "Party due diligence updated")
	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}

	if s.eventBus != nil {
		accountID := ""
		if idx := findPartyIndexByID(d.Parties, partyID); idx != -1 {
			accountID = d.Parties[idx].AccountID
		}
		evt := contracts.DealPartyDueDiligenceUpdatedEvent{
			Data: contracts.DealPartyDueDiligenceUpdatedEventPayload{
				DealID:    d.ID,
				PartyID:   partyID,
				TenantID:  d.TenantID,
				AccountID: accountID,
				OldStatus: oldStatus,
				NewStatus: newStatus,
				UpdatedAt: time.Now().Format(time.RFC3339),
			},
		}
		if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
			log.Printf("WARN: failed to enqueue deal.party.due_diligence.updated for deal=%s party=%s: %v", d.ID, partyID, err)
		}
	}

	return d, nil
}

var validDueDiligenceCriteria = map[string]bool{
	"score": true, "cnd": true, "legal_opinion": true, "income": true,
}

var validDueDiligenceReviewStatuses = map[string]bool{
	"pending": true, "in_review": true, "approved": true, "rejected": true, "waived": true,
}

// UpdatePartyDueDiligenceCriterion registra a revisão de um critério e
// recalcula o status da parte a partir dos quatro critérios.
func (s *DealService) UpdatePartyDueDiligenceCriterion(ctx context.Context, tenantID, dealID, brokerID, partyID, criterion, status, notes, userID string) (*deal.Deal, error) {
	criterion = strings.ToLower(strings.TrimSpace(criterion))
	status = strings.ToLower(strings.TrimSpace(status))
	if !validDueDiligenceCriteria[criterion] {
		return nil, fmt.Errorf("invalid due diligence criterion")
	}
	if !validDueDiligenceReviewStatuses[status] {
		return nil, fmt.Errorf("invalid due diligence criterion status")
	}

	d, err := s.getDealByScope(ctx, tenantID, dealID, brokerID)
	if err != nil {
		return nil, err
	}

	oldStatus, newStatus, err := d.UpdatePartyDueDiligenceCriterion(partyID, criterion, status, userID, notes)
	if err != nil {
		return nil, err
	}

	d.DueDiligence = d.AggregatedDueDiligence()
	d.AddHistory(d.Status, userID, "Due diligence criterion "+criterion+" updated to "+status)
	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}

	if s.eventBus != nil {
		accountID := ""
		if idx := findPartyIndexByID(d.Parties, partyID); idx != -1 {
			accountID = d.Parties[idx].AccountID
		}
		evt := contracts.DealPartyDueDiligenceUpdatedEvent{
			Data: contracts.DealPartyDueDiligenceUpdatedEventPayload{
				DealID:    d.ID,
				PartyID:   partyID,
				TenantID:  d.TenantID,
				AccountID: accountID,
				OldStatus: oldStatus,
				NewStatus: newStatus,
				UpdatedAt: time.Now().Format(time.RFC3339),
			},
		}
		if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
			log.Printf("WARN: failed to enqueue deal.party.due_diligence.updated (criterion) for deal=%s party=%s: %v", d.ID, partyID, err)
		}
	}
	return d, nil
}

func (s *DealService) AddSigner(ctx context.Context, tenantID, dealID, brokerID, name, email string, role deal.SignatoryRole, isRequired *bool, userID string) (*deal.Deal, error) {
	d, err := s.getDealByScope(ctx, tenantID, dealID, brokerID)
	if err != nil {
		return nil, err
	}
	if err := d.AddSigner(name, email, role); err != nil {
		return nil, err
	}
	if isRequired != nil {
		// AddSigner appends; the new signer is the last entry. Apply the
		// caller's explicit choice; if nil, IsSignerRequired() defaults to true.
		val := *isRequired
		d.Signers[len(d.Signers)-1].IsRequired = &val
	}
	// Signatário cadastrado aqui também é gente do negócio: sem conta de portal
	// ele pediria o código e não receberia nada (o toca-auth só envia para
	// contato que já é cliente). O account_id entra no mesmo Update abaixo.
	s.ProvisionPortalAccounts(ctx, d)
	d.AddHistory(d.Status, userID, "Signer added: "+name)
	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// SignerUpdate carries optional fields for partial signer updates. Use pointer
// fields so unspecified values are not overwritten.
type SignerUpdate struct {
	IsRequired *bool
	Name       *string
	Email      *string
	Role       *deal.SignatoryRole
}

func (s *DealService) UpdateSigner(ctx context.Context, tenantID, dealID, brokerID, signerID string, patch SignerUpdate, userID string) (*deal.Deal, error) {
	d, err := s.getDealByScope(ctx, tenantID, dealID, brokerID)
	if err != nil {
		return nil, err
	}

	idx := -1
	for i := range d.Signers {
		if d.Signers[i].ID == signerID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil, errors.New("signer not found")
	}

	if patch.IsRequired != nil {
		val := *patch.IsRequired
		d.Signers[idx].IsRequired = &val
	}
	if patch.Name != nil {
		d.Signers[idx].Name = *patch.Name
	}
	if patch.Email != nil {
		d.Signers[idx].Email = *patch.Email
	}
	if patch.Role != nil {
		d.Signers[idx].Role = *patch.Role
	}
	d.UpdatedAt = time.Now()

	// E-mail novo/alterado é contato novo no portal: garantir a conta aqui
	// evita o mesmo beco sem saída do OTP descartado em silêncio.
	s.ProvisionPortalAccounts(ctx, d)
	d.AddHistory(d.Status, userID, "Signer updated")
	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *DealService) UpdateSignerStatus(ctx context.Context, tenantID, dealID, brokerID, signerID string, status deal.SignerStatus, signatureURL, userID string) (*deal.Deal, error) {
	d, err := s.getDealByScope(ctx, tenantID, dealID, brokerID)
	if err != nil {
		return nil, err
	}
	d.UpdateSignerStatus(signerID, status, signatureURL)
	d.AddHistory(d.Status, userID, "Signer status updated")
	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *DealService) RemoveSigner(ctx context.Context, tenantID, dealID, brokerID, signerID, userID string) (*deal.Deal, error) {
	d, err := s.getDealByScope(ctx, tenantID, dealID, brokerID)
	if err != nil {
		return nil, err
	}

	d.RemoveSigner(signerID)
	d.AddHistory(d.Status, userID, "Signer removed")
	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *DealService) ListDealParties(ctx context.Context, tenantID, dealID, brokerID string) ([]deal.Party, error) {
	d, err := s.getDealByScope(ctx, tenantID, dealID, brokerID)
	if err != nil {
		return nil, err
	}
	return d.Parties, nil
}

func (s *DealService) AddParty(ctx context.Context, tenantID, dealID, brokerID string, p deal.Party, userID string) (*deal.Deal, error) {
	d, err := s.getDealByScope(ctx, tenantID, dealID, brokerID)
	if err != nil {
		return nil, err
	}
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	if err := d.AddParty(p); err != nil {
		return nil, err
	}
	// If the caller supplied a LeadID without the lead profile fields, pull them
	// from the CRM so the new party renders correctly in the parties panel.
	s.hydratePartiesFromLeads(ctx, d)
	if idx := findPartyIndexByID(d.Parties, p.ID); idx != -1 {
		p = d.Parties[idx]
	}
	if p.Name != "" && p.Email != "" {
		exists := false
		for i := range d.Signers {
			if d.Signers[i].ID == p.ID {
				exists = true
				break
			}
		}
		if !exists {
			if sr, ok := signatoryRoleFromPartyRole(d.Type, p.Role); ok {
				_ = d.AddSigner(p.Name, p.Email, sr)
				if p.AccountID != "" {
					for i := range d.Signers {
						if d.Signers[i].Email == p.Email {
							d.Signers[i].AccountID = p.AccountID
							if partyIdx := findPartyIndexByID(d.Parties, d.Signers[i].ID); partyIdx != -1 {
								d.Parties[partyIdx].AccountID = p.AccountID
								if d.Parties[partyIdx].LeadID == "" && p.LeadID != "" {
									d.Parties[partyIdx].LeadID = p.LeadID
								}
							}
							break
						}
					}
				}
			}
		}
	}
	// A parte pode ter entrado depois da criação (ou ganhado e-mail agora):
	// provisiona antes do Update para o account_id viajar no mesmo write.
	s.ProvisionPortalAccounts(ctx, d)
	d.SyncLegacyIDsFromParties()
	d.AddHistory(d.Status, userID, "Party added")
	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *DealService) RemoveParty(ctx context.Context, tenantID, dealID, brokerID, partyID, userID string) (*deal.Deal, error) {
	d, err := s.getDealByScope(ctx, tenantID, dealID, brokerID)
	if err != nil {
		return nil, err
	}
	if err := d.RemoveParty(partyID); err != nil {
		return nil, err
	}
	if len(d.Signers) > 0 {
		filtered := make([]deal.Signer, 0, len(d.Signers))
		for _, sgn := range d.Signers {
			if sgn.ID == partyID {
				continue
			}
			filtered = append(filtered, sgn)
		}
		d.Signers = filtered
	}
	d.SyncLegacyIDsFromParties()
	d.AddHistory(d.Status, userID, "Party removed")
	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *DealService) UpdatePartyRole(ctx context.Context, tenantID, dealID, brokerID, partyID string, role deal.PartyRole, userID string) (*deal.Deal, error) {
	d, err := s.getDealByScope(ctx, tenantID, dealID, brokerID)
	if err != nil {
		return nil, err
	}
	if err := d.UpdatePartyRole(partyID, role); err != nil {
		return nil, err
	}
	for i := range d.Signers {
		if d.Signers[i].ID != partyID {
			continue
		}
		sr, ok := signatoryRoleFromPartyRole(d.Type, role)
		if ok {
			d.Signers[i].Role = sr
		}
		break
	}
	d.SyncLegacyIDsFromParties()
	d.AddHistory(d.Status, userID, "Party role updated")
	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// PartyIdentityPatch carrega os campos identificadores editáveis de uma
// Party. Cada campo nil é deixado como está; non-nil sobrescreve (após
// TrimSpace). Permite ao operador completar o cadastro de uma parte — em
// especial o Document (CPF/CNPJ) — direto pelo painel de partes da
// Esteira, que antes não tinha caminho de edição para esses campos.
type PartyIdentityPatch struct {
	Name                *string
	Email               *string
	Phone               *string
	Document            *string
	BirthDate           *string
	CustomFields        map[string]string
	RemovedCustomFields []string
}

func (s *DealService) UpdateParty(ctx context.Context, tenantID, dealID, brokerID, partyID string, role *deal.PartyRole, noSpouseDeclared *bool, representation *deal.PartyRepresentation, signatureRequired *bool, identity *PartyIdentityPatch, userID string) (*deal.Deal, error) {
	d, err := s.getDealByScope(ctx, tenantID, dealID, brokerID)
	if err != nil {
		return nil, err
	}

	if role != nil {
		if err := d.UpdatePartyRole(partyID, *role); err != nil {
			return nil, err
		}
		for i := range d.Signers {
			if d.Signers[i].ID != partyID {
				continue
			}
			sr, ok := signatoryRoleFromPartyRole(d.Type, *role)
			if ok {
				d.Signers[i].Role = sr
			}
			break
		}
	}

	if idx := findPartyIndexByID(d.Parties, partyID); idx != -1 {
		if noSpouseDeclared != nil {
			d.Parties[idx].NoSpouseDeclared = *noSpouseDeclared
		}
		if representation != nil {
			d.Parties[idx].Representation = representation
		}
		if signatureRequired != nil {
			val := *signatureRequired
			d.Parties[idx].SignatureRequired = &val
		}
		if identity != nil {
			now := time.Now()
			markManual := func(key string) {
				if d.Parties[idx].FieldProvenance == nil {
					d.Parties[idx].FieldProvenance = map[string]deal.FieldProvenance{}
				}
				d.Parties[idx].FieldProvenance[key] = deal.FieldProvenance{
					Source:      deal.ProvenanceManual,
					ValidatedBy: userID,
					UpdatedAt:   now,
				}
			}
			if identity.Name != nil {
				next := strings.TrimSpace(*identity.Name)
				if next != d.Parties[idx].Name {
					markManual("name")
				}
				d.Parties[idx].Name = next
			}
			if identity.Email != nil {
				next := strings.TrimSpace(*identity.Email)
				if next != d.Parties[idx].Email {
					markManual("email")
				}
				d.Parties[idx].Email = next
			}
			if identity.Phone != nil {
				next := strings.TrimSpace(*identity.Phone)
				if next != d.Parties[idx].Phone {
					markManual("phone")
				}
				d.Parties[idx].Phone = next
			}
			if identity.Document != nil {
				next := strings.TrimSpace(*identity.Document)
				if next != d.Parties[idx].Document {
					markManual("document")
				}
				d.Parties[idx].Document = next
			}
			if identity.BirthDate != nil {
				next := strings.TrimSpace(*identity.BirthDate)
				if next != d.Parties[idx].BirthDate {
					markManual("birth_date")
				}
				d.Parties[idx].BirthDate = next
			}
			if len(identity.RemovedCustomFields) > 0 {
				for _, key := range identity.RemovedCustomFields {
					key = strings.TrimSpace(key)
					if key == "" {
						continue
					}
					delete(d.Parties[idx].CustomFields, key)
					delete(d.Parties[idx].FieldProvenance, key)
				}
			}
			if identity.CustomFields != nil {
				if d.Parties[idx].CustomFields == nil {
					d.Parties[idx].CustomFields = map[string]string{}
				}
				for key, value := range identity.CustomFields {
					key = strings.TrimSpace(key)
					if key == "" {
						continue
					}
					next := strings.TrimSpace(value)
					if d.Parties[idx].CustomFields[key] == next {
						continue
					}
					d.Parties[idx].CustomFields[key] = next
					markManual(key)
				}
			}
		}
		d.Parties[idx].UpdatedAt = time.Now()
	}

	// Parties e Signers compartilham o mesmo ID (ver EnsurePartiesFromLegacy)
	// e o envelope Clicksign é gerado a partir dos Signers — então uma
	// correção de nome/e-mail na Party precisa refletir no Signer para os
	// dois não divergirem.
	if identity != nil && (identity.Name != nil || identity.Email != nil) {
		for i := range d.Signers {
			if d.Signers[i].ID != partyID {
				continue
			}
			if identity.Name != nil {
				d.Signers[i].Name = strings.TrimSpace(*identity.Name)
			}
			if identity.Email != nil {
				d.Signers[i].Email = strings.TrimSpace(*identity.Email)
			}
			break
		}
	}

	// `identity.Email` pode ter acabado de chegar: sem conta de portal o
	// titular pede o código e o toca-auth descarta o pedido em silêncio.
	s.ProvisionPortalAccounts(ctx, d)
	d.SyncLegacyIDsFromParties()
	d.AddHistory(d.Status, userID, "Party updated")
	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// ExtractedFieldApply é um campo a gravar numa party — vindo da IA (auto-apply de
// alta confiança) ou da validação do operador.
type ExtractedFieldApply struct {
	Name       string
	Target     string
	Value      string
	Confidence float64
}

// ApplyExtractedFields grava os campos identificados na party (party.* direto ou
// custom_fields.*) e registra a origem em Party.FieldProvenance. `source` é
// deal.ProvenanceAI (auto-aplicado) ou deal.ProvenanceValidated (operador).
// brokerID="" carrega em escopo de sistema (usado pelo subscriber de extração);
// não-vazio aplica o escopo do corretor (endpoint do operador).
func (s *DealService) ApplyExtractedFields(ctx context.Context, tenantID, dealID, brokerID, partyID string, fields []ExtractedFieldApply, source, documentID, userID string) (*deal.Deal, error) {
	d, err := s.getDealByScope(ctx, tenantID, dealID, brokerID)
	if err != nil {
		return nil, err
	}
	idx := findPartyIndexByID(d.Parties, partyID)
	if idx == -1 {
		return nil, fmt.Errorf("party %s not found in deal %s", partyID, dealID)
	}

	now := time.Now()
	p := &d.Parties[idx]
	if p.FieldProvenance == nil {
		p.FieldProvenance = map[string]deal.FieldProvenance{}
	}
	if p.CustomFields == nil {
		p.CustomFields = map[string]string{}
	}
	validated := source == deal.ProvenanceValidated

	applied := 0
	for _, f := range fields {
		val := strings.TrimSpace(f.Value)
		if val == "" {
			continue
		}
		applyPartyTargetValue(p, f.Name, f.Target, val)
		prov := deal.FieldProvenance{
			Source:     source,
			Confidence: f.Confidence,
			DocumentID: documentID,
			UpdatedAt:  now,
		}
		if validated {
			prov.ValidatedBy = userID
			prov.ValidatedAt = &now
		}
		p.FieldProvenance[f.Name] = prov
		applied++
	}
	if applied == 0 {
		return d, nil
	}
	p.UpdatedAt = now

	// Mantém o Signer correspondente em sincronia (nome/e-mail) — mesmo motivo
	// de UpdateParty: o envelope Clicksign é gerado a partir dos Signers.
	for i := range d.Signers {
		if d.Signers[i].ID != partyID {
			continue
		}
		if p.Name != "" {
			d.Signers[i].Name = p.Name
		}
		if p.Email != "" {
			d.Signers[i].Email = p.Email
		}
		break
	}

	d.SyncLegacyIDsFromParties()
	d.AddHistory(d.Status, userID, "Campos da parte aplicados a partir da extração ("+source+")")
	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}

	audit.Record(ctx, audit.Entry{
		Action:     "apply_extraction",
		EntityType: "Party",
		EntityID:   partyID,
		After: map[string]any{
			"deal_id":     dealID,
			"source":      source,
			"document_id": documentID,
			"fields":      applied,
		},
	})
	return d, nil
}

// applyPartyTargetValue grava um valor no destino correto da party (campo direto
// ou custom_fields.*). Target vazio cai em "party.<name>".
func applyPartyTargetValue(p *deal.Party, name, target, value string) {
	t := target
	if t == "" {
		t = "party." + name
	}
	switch t {
	case "party.name":
		p.Name = value
	case "party.email":
		p.Email = value
	case "party.phone":
		p.Phone = value
	case "party.document":
		p.Document = value
	case "party.birth_date":
		p.BirthDate = value
	default:
		const pfx = "party.custom_fields."
		key := name
		if strings.HasPrefix(t, pfx) {
			key = strings.TrimPrefix(t, pfx)
		}
		if key != "" {
			p.CustomFields[key] = value
		}
	}
}

func (s *DealService) SetPrimaryParty(ctx context.Context, tenantID, dealID, brokerID, partyID, userID string) (*deal.Deal, error) {
	d, err := s.getDealByScope(ctx, tenantID, dealID, brokerID)
	if err != nil {
		return nil, err
	}
	if err := d.SetPrimaryParty(partyID); err != nil {
		return nil, err
	}
	d.SyncLegacyIDsFromParties()
	d.AddHistory(d.Status, userID, "Primary party updated")
	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}
