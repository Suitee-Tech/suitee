package services

import (
	"fmt"
	"strings"
	"time"

	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// CustomerEmailsWithoutAccount devolve os e-mails das partes cliente
// (buyer/tenant) que ainda NÃO têm conta de portal. Complementa os
// customer_account_ids nos eventos de negócio: o toca-comm notifica essas
// partes por e-mail direto (sem in-app) até o convite ao portal existir.
// Deduplicado e sem vazios.
func CustomerEmailsWithoutAccount(d *deal.Deal) []string {
	if d == nil {
		return nil
	}
	emails := []string{}
	seen := map[string]bool{}
	for _, p := range d.Parties {
		if p.Role != deal.PartyRoleBuyer && p.Role != deal.PartyRoleTenant {
			continue
		}
		if p.AccountID != "" {
			continue
		}
		e := strings.TrimSpace(p.Email)
		if e == "" || seen[e] {
			continue
		}
		seen[e] = true
		emails = append(emails, e)
	}
	return emails
}

// SignerAccountIDs devolve as contas de portal dos signatários do negócio,
// deduplicadas e sem vazios. É a fonte canônica do destinatário "cliente" nos
// eventos de proposta (proposal.sent e o follow-up §F6): manter a mesma função
// garante que o lembrete chegue exatamente para quem recebeu a proposta.
func SignerAccountIDs(d *deal.Deal) []string {
	if d == nil {
		return nil
	}
	ids := make([]string, 0, len(d.Signers))
	seen := map[string]bool{}
	for _, signer := range d.Signers {
		if signer.AccountID == "" || seen[signer.AccountID] {
			continue
		}
		seen[signer.AccountID] = true
		ids = append(ids, signer.AccountID)
	}
	return ids
}

// Deal and portal event contracts are kept together so publishers remain
// discoverable without coupling their schemas to service orchestration.

// Events

// PartyInfo carries minimal, non-PII party data for channel info auto-population.
type PartyInfo struct {
	Name   string `json:"name"`
	Role   string `json:"role"`
	Type   string `json:"type,omitempty"`    // "proprietario", "locador", "locatario", "fiador"
	LeadID string `json:"lead_id,omitempty"` // link to /leads/:id
}

type DealCreatedEvent struct {
	DealID            string      `json:"deal_id"`
	TenantID          string      `json:"tenant_id"`
	PropertyID        string      `json:"property_id"`
	Title             string      `json:"title"`
	DealType          string      `json:"deal_type"`
	DealValue         float64     `json:"deal_value"`
	Status            deal.Status `json:"status"`
	CustomerAccountID string      `json:"customer_account_id"`
	BrokerID          string      `json:"broker_id"`
	BrokerName        string      `json:"broker_name"`
	Parties           []PartyInfo `json:"parties,omitempty"`
	Timestamp         time.Time   `json:"timestamp"`
}

func (e DealCreatedEvent) Name() string {
	return "deal.created"
}

func (e DealCreatedEvent) Payload() interface{} {
	return e
}

type DealStatusChangedEvent struct {
	DealID             string      `json:"deal_id"`
	TenantID           string      `json:"tenant_id"`
	PropertyID         string      `json:"property_id"`
	OldStatus          deal.Status `json:"old_status"`
	NewStatus          deal.Status `json:"new_status"`
	DealType           string      `json:"deal_type,omitempty"`
	CustomerAccountIDs []string    `json:"customer_account_ids,omitempty"`
	// CustomerEmails carrega os e-mails das partes cliente (buyer/tenant) que
	// AINDA não têm conta de portal — o consumidor notifica por e-mail direto,
	// sem in-app (beta de e-mail transacional, F4).
	CustomerEmails []string `json:"customer_emails,omitempty"`
	// ActorUserID is the user who triggered the transition. The Esteira's
	// realtime hook uses it to skip the "X moveu para Y" toast on the
	// originating browser (no self-notifications).
	ActorUserID string    `json:"actor_user_id,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

func (e DealStatusChangedEvent) Name() string {
	return "deal.status_changed"
}

func (e DealStatusChangedEvent) Payload() interface{} {
	return e
}

type DealDeletedEvent struct {
	DealID    string    `json:"deal_id"`
	Timestamp time.Time `json:"timestamp"`
}

func (e DealDeletedEvent) Name() string {
	return "deal.deleted"
}

func (e DealDeletedEvent) Payload() interface{} {
	return e
}

func (e DealDeletedEvent) EventID() string {
	return ""
}

func (e DealStatusChangedEvent) EventID() string {
	return ""
}

func (e DealCreatedEvent) EventID() string {
	return ""
}

// NewDealCreatedEvent builds a fully-populated DealCreatedEvent from the deal
// domain object. Extracts parties, signers, and broker information for
// downstream consumers (toca-comm channel auto-population).
func NewDealCreatedEvent(d *deal.Deal) DealCreatedEvent {
	customerAccountID := ""
	for _, signer := range d.Signers {
		if signer.AccountID != "" {
			customerAccountID = signer.AccountID
			break
		}
	}

	// Collect party info (non-PII: name + role only)
	parties := make([]PartyInfo, 0, len(d.Parties))
	for _, p := range d.Parties {
		if p.Name == "" {
			continue
		}
		kind := ""
		if p.Kind != "" {
			kind = string(p.Kind)
		}
		parties = append(parties, PartyInfo{
			Name:   p.Name,
			Role:   string(p.Role),
			Type:   kind,
			LeadID: p.LeadID,
		})
	}

	return DealCreatedEvent{
		DealID:            d.ID,
		TenantID:          d.TenantID,
		PropertyID:        d.PropertyID,
		Title:             d.DisplayID,
		DealType:          string(d.Type),
		DealValue:         d.Value,
		Status:            d.Status,
		CustomerAccountID: customerAccountID,
		BrokerID:          d.BrokerID,
		BrokerName:        "",
		Parties:           parties,
		Timestamp:         time.Now(),
	}
}

// Portal client events

type ClientTicketCreatedEvent struct {
	TicketID          string    `json:"ticket_id"`
	DealID            string    `json:"deal_id"`
	TenantID          string    `json:"tenant_id"`
	PropertyID        string    `json:"property_id"`
	CustomerAccountID string    `json:"customer_account_id"`
	Priority          string    `json:"priority"`
	Category          string    `json:"category"`
	Timestamp         time.Time `json:"timestamp"`
}

func (e ClientTicketCreatedEvent) Name() string         { return "client.ticket.created" }
func (e ClientTicketCreatedEvent) Payload() interface{} { return e }
func (e ClientTicketCreatedEvent) EventID() string      { return "" }

type ClientAppointmentRequestedEvent struct {
	AppointmentID     string    `json:"appointment_id"`
	DealID            string    `json:"deal_id"`
	TenantID          string    `json:"tenant_id"`
	Date              string    `json:"date"`
	PropertyID        string    `json:"property_id"`
	CustomerAccountID string    `json:"customer_account_id"`
	BrokerID          string    `json:"broker_id"`
	Timestamp         time.Time `json:"timestamp"`
}

func (e ClientAppointmentRequestedEvent) Name() string         { return "client.appointment.requested" }
func (e ClientAppointmentRequestedEvent) Payload() interface{} { return e }
func (e ClientAppointmentRequestedEvent) EventID() string      { return "" }

type ClientDocumentUploadedEvent struct {
	DealID            string    `json:"deal_id"`
	ItemID            string    `json:"item_id"`
	FileName          string    `json:"file_name"`
	TenantID          string    `json:"tenant_id"`
	CustomerAccountID string    `json:"customer_account_id"`
	Timestamp         time.Time `json:"timestamp"`
}

func (e ClientDocumentUploadedEvent) Name() string         { return "client.document.uploaded" }
func (e ClientDocumentUploadedEvent) Payload() interface{} { return e }
func (e ClientDocumentUploadedEvent) EventID() string      { return "" }

// ClientInvitedEvent is published when a client is invited to the portal
type ClientInvitedEvent struct {
	Email      string `json:"email"`
	ClientName string `json:"name"`
	Phone      string `json:"phone"`
	AccountID  string `json:"account_id"`
	TenantID   string `json:"tenant_id"`
}

func (e ClientInvitedEvent) EventID() string {
	return fmt.Sprintf("%s-%s-%d", e.Name(), e.Email, time.Now().UnixNano())
}
func (e ClientInvitedEvent) Name() string         { return "auth.client.invited" }
func (e ClientInvitedEvent) Payload() interface{} { return e }
