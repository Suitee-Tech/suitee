package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/metrics"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// Lifecycle deletion and status transitions are isolated from creation and
// portal orchestration while preserving the DealService API.

func (s *DealService) Delete(ctx context.Context, tenantID, id, brokerID string) error {
	// Check if deal exists
	d, err := s.getDealByScope(ctx, tenantID, id, brokerID)
	if err != nil {
		return err
	}

	if brokerID != "" {
		if err := s.repo.DeleteScoped(ctx, tenantID, id, brokerID); err != nil {
			return err
		}
	} else if err := s.repo.Delete(ctx, tenantID, id); err != nil {
		return err
	}

	// Publish DealDeleted event
	if s.eventBus != nil {
		// Define event struct locally or use map for simplicity if strict type not needed
		// For consistency, let's assume a generic event or skip payload details if not critical
		// Or define DealDeletedEvent at bottom of file
		evt := DealDeletedEvent{
			DealID:    d.ID,
			Timestamp: time.Now(),
		}
		if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
			log.Printf("WARN: failed to enqueue deal.deleted event for deal=%s: %v", d.ID, err)
		}
	}
	audit.Record(ctx, audit.Entry{
		Action:     "delete",
		EntityType: "Deal",
		EntityID:   d.ID,
		Before: map[string]any{
			"status":      string(d.Status),
			"type":        string(d.Type),
			"property_id": d.PropertyID,
		},
	})

	return nil
}

// UpdateStatusTrusted é o caminho de transição para chamadores INTERNOS
// confiáveis (workers, subscribers, migrations) — assume aprovação concedida.
//
// SECURITY (achado A23 da revisão de permissões de 2026-09-19): o nome antigo
// (`UpdateStatus`) escondia que ele ignora o gate de aprovação por valor; um
// handler HTTP que o chamasse desligaria o controle sem que nada no call site
// denunciasse isso. O nome agora diz o que ele é, e o HTTP usa
// `UpdateStatusAuthorized` com a permissão real do JWT.
func (s *DealService) UpdateStatusTrusted(ctx context.Context, tenantID, id, brokerID string, status deal.Status, userID string, notes string) (*deal.Deal, error) {
	return s.UpdateStatusAuthorized(ctx, tenantID, id, brokerID, status, userID, notes, true)
}

// UpdateStatusAuthorized é a variante consciente da permissão do caller.
// actorCanApprove deve ser true quando o JWT do caller satisfaz
// permission `deals:approve` (admin/gestor/operações). Quando o deal
// excede o approval threshold (#416) e actorCanApprove=false, retorna
// ErrApprovalRequired e a transição é abortada antes do persist.
func (s *DealService) UpdateStatusAuthorized(ctx context.Context, tenantID, id, brokerID string, status deal.Status, userID string, notes string, actorCanApprove bool) (*deal.Deal, error) {
	d, err := s.getDealByScope(ctx, tenantID, id, brokerID)
	if err != nil {
		return nil, err
	}

	oldStatus := d.Status

	// Validate transition (Basic state machine)
	if !isValidTransition(d.Status, status) {
		return nil, errors.New("invalid status transition")
	}

	// Resolve the tenant's versioned toca-auth policy only for the v1 target
	// stages. AuthClient supplies fresh/stale cache; a bounded fetch failure
	// falls back to the historical env/default threshold explicitly.
	approvalPolicy := fallbackApprovalPolicy()
	approvalRequired := false
	if status == deal.StatusAnalysis || status == deal.StatusContract {
		approvalPolicy = s.resolveApprovalPolicy(ctx, tenantID)
		approvalRequired = requiresApprovalWithPolicy(d, status, approvalPolicy)
	}
	if !actorCanApprove && approvalRequired {
		return nil, fmt.Errorf("%w: deal_value=%.2f threshold=%.2f target=%s policy_revision=%d policy_source=%s",
			ErrApprovalRequired, d.Value, approvalPolicy.ThresholdBRL, status, approvalPolicy.Revision, approvalPolicy.Source)
	}

	if status == deal.StatusAnalysis || status == deal.StatusContract {
		d.EnsurePartiesFromLegacy()

		// proposal_not_approved (#412): bloqueia avanço sem proposal aceita —
		// MAS respeitando os "Parâmetros da Proposta Digital".
		//
		// A tela de parâmetros deixa o operador marcar o aceite de cada parte
		// como opcional e anuncia isso explicitamente ("Sem partes obrigatórias,
		// o deal avança sem aceite formal da proposta"). O gate era tudo-ou-nada
		// e ignorava a configuração: com todas as partes opcionais ele continuava
		// exigindo a proposta aceita e bloqueava o operador que quis prosseguir
		// sem aceite — exatamente o defeito relatado.
		//
		// A exceção só vale quando a configuração é EXPLÍCITA: pelo menos uma
		// parte com `signature_required` definido e nenhuma exigindo aceite. Sem
		// isso, negócios antigos (campo ausente, padrão = parte primária) mantêm
		// o comportamento anterior.
		acceptanceOptional := false
		if len(d.Parties) > 0 {
			configured := false
			required := false
			for i := range d.Parties {
				party := &d.Parties[i]
				if party.SignatureRequired != nil {
					configured = true
					if *party.SignatureRequired {
						required = true
					}
					continue
				}
				if party.IsSignatureRequired() {
					required = true
				}
			}
			acceptanceOptional = configured && !required
		}

		if !acceptanceOptional {
			if len(d.Proposals) == 0 {
				return nil, ErrProposalNotApproved
			}
			latest := d.Proposals[len(d.Proposals)-1]
			if latest.Status != deal.ProposalStatusAccepted {
				return nil, ErrProposalNotApproved
			}
		}

		customerRole := deal.PartyRoleTenant
		if d.Type == deal.TypeSale {
			customerRole = deal.PartyRoleBuyer
		}

		// parties_pending (#412): parties obrigatórias precisam de Name +
		// Email já na entrada em ANALYSIS — são o que identifica a parte e
		// vêm do cadastro do lead. O Document (CPF/CNPJ) só é exigido na
		// transição para CONTRACT: é na própria etapa "Análise (Doc)" que
		// ele é coletado/conferido, e o Clicksign só precisa dele na hora
		// de gerar o envelope. Exigir o documento para *entrar* na análise
		// travava o funil sem caminho de correção — o documento da Party
		// só é editável pelo painel de partes da Esteira.
		for i := range d.Parties {
			p := &d.Parties[i]
			if !p.IsSignatureRequired() {
				continue
			}
			// Spouse role tem regras próprias (validadas abaixo).
			if p.Role == deal.PartyRoleSpouse {
				continue
			}
			if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.Email) == "" {
				return nil, fmt.Errorf("%w: party_id=%s role=%s missing=name/email",
					ErrPartiesPending, p.ID, p.Role)
			}
			if status == deal.StatusContract && strings.TrimSpace(p.Document) == "" {
				return nil, fmt.Errorf("%w: party_id=%s role=%s missing=document",
					ErrPartiesPending, p.ID, p.Role)
			}
		}

		hasSpouse := func(partyID string) bool {
			for _, p := range d.Parties {
				if p.Role == deal.PartyRoleSpouse && p.SpouseOfPartyID == partyID {
					return true
				}
			}
			return false
		}

		for i := range d.Parties {
			principal := &d.Parties[i]
			if !principal.IsSignatureRequired() {
				continue
			}
			if principal.Role != customerRole && principal.Role != deal.PartyRoleOwner {
				continue
			}
			if principal.NoSpouseDeclared {
				continue
			}
			if !hasSpouse(principal.ID) {
				return nil, fmt.Errorf("%w: principal_party_id=%s",
					ErrSpouseMissing, principal.ID)
			}
		}

		// Phase 4/W3 — checklist gate. Off by default; when
		// `TOCA_REQUIRE_CHECKLIST_GATE_ALL=true` or the tenant is in
		// the explicit `TOCA_REQUIRE_CHECKLIST_GATE_TENANTS` list, every
		// required checklist item must be APPROVED before the deal can
		// advance to CONTRACT. Enforced at the CONTRACT entry point
		// only (not on ANALYSIS) so operators can still move into
		// ANALYSIS to *work on* the docs.
		if status == deal.StatusContract && IsChecklistGateEnabled(tenantID) {
			if !d.RequiredChecklistComplete() {
				return nil, ErrChecklistNotApproved
			}
		}
	}

	if status == deal.StatusConverted {
		if s.signersStaleAgainstContract(d) {
			// Stale projection — re-sync from the contract and reload the
			// deal before failing the gate. This recovers from the case where
			// a Clicksign envelope_finished event landed before the signer
			// sync code was hardened, leaving d.Signers permanently PENDING
			// even though the contract is SIGNED.
			//
			// Log every step so operators have a forensic trail when the
			// "Concluído" gate fails despite the auto-heal — without this,
			// a sync error silently dropped through the if-syncErr-==-nil
			// guard left no clue why the projection didn't refresh.
			if syncErr := s.signerSyncer.SyncDealSignersFromActiveContract(ctx, tenantID, d.ID); syncErr != nil {
				log.Printf("MoveDealStatus auto-heal: SyncDealSignersFromActiveContract failed for deal=%s tenant=%s: %v", d.ID, tenantID, syncErr)
			} else {
				if refreshed, refreshErr := s.repo.GetByID(ctx, tenantID, d.ID); refreshErr != nil {
					log.Printf("MoveDealStatus auto-heal: deal reload after sync failed for deal=%s tenant=%s: %v", d.ID, tenantID, refreshErr)
				} else if refreshed != nil {
					d = refreshed
				}
			}
		}
		for i := range d.Signers {
			sig := &d.Signers[i]
			if !sig.IsSignerRequired() {
				continue
			}
			if sig.Status != deal.SignerStatusSigned {
				log.Printf("MoveDealStatus gate: required signer still not SIGNED after auto-heal — deal=%s tenant=%s signer_id=%s signer_email=%s status=%s (contract may also be stale; check contract.signatories vs signature.envelopes)",
					d.ID, tenantID, sig.ID, sig.Email, sig.Status)
				return nil, ErrSignersNotComplete
			}
		}
	}

	d.Status = status
	d.AddHistory(status, userID, notes)

	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}

	// Forensic trail — every deal status change is auditable for 5 years.
	// before/after carry only the status fields so the audit row stays
	// compact (no PII risk here since both are enum values, not user data).
	s.recordDealAudit(ctx, audit.Entry{
		Action:     "deal.status_changed",
		EntityType: "Deal",
		EntityID:   d.ID,
		Before:     map[string]any{"status": string(oldStatus)},
		After:      map[string]any{"status": string(status)},
		Metadata:   map[string]any{"actor_notes": notes},
	})

	// Crossing a policy gate with deals:approve is an explicit override, not
	// merely another status transition. Record the exact policy revision and
	// cache/fallback source used so incident review can reproduce the decision.
	if approvalRequired && actorCanApprove {
		s.recordDealAudit(ctx, audit.Entry{
			Action:     "deal.approval_override",
			EntityType: "Deal",
			EntityID:   d.ID,
			Before: map[string]any{
				"status": string(oldStatus),
			},
			After: map[string]any{
				"status": string(status),
			},
			Metadata: map[string]any{
				"deal_value":                  d.Value,
				"threshold_brl":               approvalPolicy.ThresholdBRL,
				"policy_schema_version":       approvalPolicy.SchemaVersion,
				"policy_revision":             approvalPolicy.Revision,
				"policy_source":               approvalPolicy.Source,
				"policy_resolution_source":    approvalPolicy.ResolutionSource,
				"required_permission":         approvalPolicy.RequiredPermission,
				"actor_authorized_to_approve": true,
			},
		})
	}

	// Business metric: every deal status transition feeds the funnel
	// dashboards. Time-to-* histograms emit at the moments the deal
	// crosses the relevant stage so PMs can chart team speed.
	metrics.IncDealConversion(string(oldStatus), string(status))
	if status == deal.StatusContract && !d.CreatedAt.IsZero() {
		metrics.ObserveTimeToContract(time.Since(d.CreatedAt))
	}
	if status == deal.StatusConverted && !d.CreatedAt.IsZero() {
		metrics.ObserveTimeToConversion(time.Since(d.CreatedAt))
	}

	// Publish DealStatusChanged event
	if s.eventBus != nil {
		customerAccountIDs := []string{}
		seen := map[string]bool{}
		for _, signer := range d.Signers {
			if signer.AccountID == "" {
				continue
			}
			if seen[signer.AccountID] {
				continue
			}
			seen[signer.AccountID] = true
			customerAccountIDs = append(customerAccountIDs, signer.AccountID)
		}
		evt := DealStatusChangedEvent{
			DealID:             d.ID,
			TenantID:           d.TenantID,
			PropertyID:         d.PropertyID,
			OldStatus:          oldStatus,
			NewStatus:          status,
			DealType:           string(d.Type),
			CustomerAccountIDs: customerAccountIDs,
			CustomerEmails:     CustomerEmailsWithoutAccount(d),
			ActorUserID:        userID,
			Timestamp:          time.Now(),
		}
		if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
			log.Printf("WARN: failed to enqueue deal.status_changed event for deal=%s: %v", d.ID, err)
		}

		// `deal.converted` NÃO é mais publicado. O único consumidor era o
		// toca-financial, e ele passou a derivar comissão/locação de
		// `deal.contract.signed` (FIN-01: o antigo deal.converted criava uma
		// comissão fixa de 6% sem contrato nem split). O publish sobreviveu à
		// remoção do consumidor e, sem binding em `toca.events`, cada negócio
		// convertido gerava uma linha de outbox "NO_ROUTE" que só ia para FAILED
		// depois de esgotar as tentativas.
	}

	return d, nil
}

func isValidTransition(current, next deal.Status) bool {
	_ = current
	_ = next
	// Allow any transition for now to support flexible drag and drop in the CRM
	return true
}
