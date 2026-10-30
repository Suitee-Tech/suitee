package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/money"
	"github.com/oginoino/toca/toca-comm/internal/domain/message"
	"github.com/oginoino/toca/toca-comm/internal/domain/notification"
	"github.com/oginoino/toca/toca-comm/internal/repository"
)

type EventConsumer struct {
	eventBus           events.EventBus
	msgService         *MessageService
	inAppService       *InAppNotificationService
	externalDispatcher ExternalChannelDispatcher
	authClient         *clients.AuthClient
	// emailService sends transactional emails (partnership invite/accepted
	// notifications). Optional — when nil the email side of a handler is
	// skipped and only the in-app notification is delivered.
	emailService *EmailService
	// emailNotifier aplica as regras do beta de e-mail transacional
	// (opt-out, presença, cooldown, idempotência) e dispara via Resend.
	// Quando nil, os braços de e-mail dos handlers ficam inertes.
	emailNotifier *EmailNotifier
	// recipientResolver resolve e-mail de destinatário a partir da
	// identidade mista do domínio (corretor/conta/lead). Usado pelo braço
	// de e-mail quando o evento não carrega o e-mail no payload.
	recipientResolver repository.EmailRecipientResolver
	// sentExternals dedupes external dispatches (WhatsApp/SMS/Email) so
	// RabbitMQ redeliveries don't fan out duplicate messages to recipients.
	// Nil-safe: when not wired (tests), the helpers fall back to direct
	// dispatch without the dedup check.
	sentExternals repository.SentExternalsRepository
}

func dealStatusLabel(status string) string {
	switch status {
	case "NEGOTIATION":
		return "Em Negociação"
	case "PROPOSAL":
		return "Proposta"
	case "ANALYSIS":
		return "Análise"
	case "CONTRACT":
		return "Contrato"
	case "CONVERTED":
		return "Concluído"
	case "CANCELLED":
		return "Perdido"
	default:
		return status
	}
}

func ticketStatusLabel(status string) string {
	switch status {
	case "open":
		return "Aberto"
	case "assigned":
		return "Atribuído"
	case "in_progress":
		return "Em andamento"
	case "awaiting_client":
		return "Aguardando cliente"
	case "resolved":
		return "Resolvido"
	case "closed":
		return "Fechado"
	case "cancelled":
		return "Cancelado"
	default:
		return status
	}
}

func ticketTypeLabel(t string) string {
	switch t {
	case "support":
		return "Suporte"
	case "maintenance":
		return "Manutenção"
	case "billing":
		return "Financeiro"
	case "documentation":
		return "Documentação"
	case "general":
		return "Geral"
	default:
		return t
	}
}

func NewEventConsumer(eventBus events.EventBus, msgService *MessageService, inAppService *InAppNotificationService) *EventConsumer {
	return &EventConsumer{
		eventBus:           eventBus,
		msgService:         msgService,
		inAppService:       inAppService,
		externalDispatcher: &NoopExternalDispatcher{},
		authClient:         clients.NewAuthClient(),
	}
}

// WithSentExternalsRepo wires the dedup repo for external dispatches.
// Optional — if not set, external dispatch is performed unconditionally
// (matches previous behavior, kept for tests that don't wire a DB).
func (c *EventConsumer) WithSentExternalsRepo(repo repository.SentExternalsRepository) *EventConsumer {
	c.sentExternals = repo
	return c
}

// WithEmailService wires the EmailService used to send partnership
// invite/accepted emails. Optional — when not set, those handlers still
// deliver the in-app notification but skip the email.
func (c *EventConsumer) WithEmailService(emailService *EmailService) *EventConsumer {
	c.emailService = emailService
	return c
}

// WithEmailNotifier wires the beta transactional email notifier. Optional —
// when not set, the transactional email arms of the handlers are inert and
// the legacy external-channel dispatch (noop by default) keeps applying.
func (c *EventConsumer) WithEmailNotifier(notifier *EmailNotifier) *EventConsumer {
	c.emailNotifier = notifier
	return c
}

// WithRecipientResolver wires the email recipient resolver used by the
// transactional email arms when the event payload doesn't carry the email.
func (c *EventConsumer) WithRecipientResolver(resolver repository.EmailRecipientResolver) *EventConsumer {
	c.recipientResolver = resolver
	return c
}

// dispatchExternalIdempotent gates an external dispatch (WhatsApp/SMS/Email)
// behind comm.sent_externals so RabbitMQ redeliveries of the same source
// event don't fan out duplicate messages. Falls back to direct dispatch
// when the dedup repo isn't wired (tests).
//
// eventID:   the source event's unique id (evt.EventID())
// channel:   "whatsapp" | "sms" | "email"
// recipient: identifier the dispatcher needs (account_id, broker_id, phone)
//
//	— combined with channel + eventID as the dedup key
//
// tenantID:  for RLS + multi-tenant analytics
// dispatch:  the actual dispatcher call. Only invoked when this is the
//
//	first delivery for (eventID, channel, recipient).
func (c *EventConsumer) dispatchExternalIdempotent(ctx context.Context, eventID, channel, recipient, tenantID string, dispatch func() error) {
	if dispatch == nil {
		return
	}
	if c.sentExternals == nil || eventID == "" || recipient == "" {
		// No dedup repo wired or no usable key — preserve legacy behavior.
		if err := dispatch(); err != nil {
			logger.Warn("external dispatch failed", "channel", channel, "recipient", recipient, "error", err.Error())
		}
		return
	}
	first, err := c.sentExternals.RegisterIfFirst(ctx, eventID, channel, recipient, tenantID, "")
	if err != nil {
		// Registration failed — log and SKIP dispatch. Better to under-deliver
		// once (recoverable via reconcile job) than to over-deliver
		// uncontrollably on a stuck DB.
		logger.Warn("sent_externals register failed; skipping dispatch", "channel", channel, "recipient", recipient, "event_id", eventID, "error", err.Error())
		return
	}
	if !first {
		// Already dispatched for this event/channel/recipient — skip.
		return
	}
	if err := dispatch(); err != nil {
		logger.Warn("external dispatch failed", "channel", channel, "recipient", recipient, "event_id", eventID, "error", err.Error())
	}
}

// notifyClientEmail envia o braço de e-mail para um cliente do portal
// (identidade = customer account id). Resolve o e-mail via
// recipientResolver; sem e-mail resolvido o envio é pulado (gate 4 do
// EmailNotifier). force=true para marcos críticos (ignora presença/cooldown).
func (c *EventConsumer) notifyClientEmail(ctx context.Context, eventID, tenantID, accountID string, category EmailCategory, subject, bodyHTML, btnText, btnURL string, force bool) {
	if c.emailNotifier == nil || c.recipientResolver == nil || accountID == "" {
		return
	}
	email, err := c.recipientResolver.ParticipantEmailByID(ctx, tenantID, accountID)
	if err != nil {
		logger.Warn("email notify: failed to resolve client email",
			"tenant_id", tenantID, "account_id", accountID, "error", err.Error())
		return
	}
	_, _ = c.emailNotifier.Notify(ctx, NotifyOptions{
		Category:     category,
		TenantID:     tenantID,
		UserID:       accountID,
		Recipient:    email,
		Subject:      subject,
		BodyHTML:     c.emailNotifier.BuildEmail(subject, bodyHTML, btnText, c.emailNotifier.FrontendLink(btnURL), clientEmailDisclaimer()),
		EventID:      eventID,
		SkipIfOnline: true,
		Force:        force,
	})
}

// notifyClientEmailWithCTA é o envio com dois convites: o botão principal e um
// link secundário. Os demais fluxos continuam usando notifyClientEmail, que tem
// um convite só.
func (c *EventConsumer) notifyClientEmailWithCTA(ctx context.Context, eventID, tenantID, accountID string, category EmailCategory, subject, bodyHTML, btnText, btnURL, secondaryText, secondaryURL string, force bool) {
	if secondaryText == "" || secondaryURL == "" {
		c.notifyClientEmail(ctx, eventID, tenantID, accountID, category, subject, bodyHTML, btnText, btnURL, force)
		return
	}
	if c.emailNotifier == nil || c.recipientResolver == nil || accountID == "" {
		return
	}
	email, err := c.recipientResolver.ParticipantEmailByID(ctx, tenantID, accountID)
	if err != nil {
		logger.Warn("email notify: failed to resolve client email",
			"tenant_id", tenantID, "account_id", accountID, "error", err.Error())
		return
	}
	// O botão principal já é absoluto quando é o arquivo do calendário; só o
	// convite do portal passa pelo prefixo do frontend.
	primary := btnURL
	if !strings.HasPrefix(btnURL, "http://") && !strings.HasPrefix(btnURL, "https://") {
		primary = c.emailNotifier.FrontendLink(btnURL)
	}
	_, _ = c.emailNotifier.Notify(ctx, NotifyOptions{
		Category:  category,
		TenantID:  tenantID,
		UserID:    accountID,
		Recipient: email,
		Subject:   subject,
		BodyHTML: c.emailNotifier.BuildEmailWithSecondary(subject, bodyHTML, btnText, primary,
			secondaryText, c.emailNotifier.FrontendLink(secondaryURL), clientEmailDisclaimer()),
		EventID:      eventID,
		SkipIfOnline: true,
		Force:        force,
	})
}

// notifyBrokerEmail envia o braço de e-mail para um corretor
// (identidade = broker id em crm.brokers).
func (c *EventConsumer) notifyBrokerEmail(ctx context.Context, eventID, tenantID, brokerID string, category EmailCategory, subject, bodyHTML, btnText, btnURL string, force bool) {
	if c.emailNotifier == nil || c.recipientResolver == nil || brokerID == "" {
		return
	}
	email, err := c.recipientResolver.ParticipantEmailByID(ctx, tenantID, brokerID)
	if err != nil {
		logger.Warn("email notify: failed to resolve broker email",
			"tenant_id", tenantID, "broker_id", brokerID, "error", err.Error())
		return
	}
	_, _ = c.emailNotifier.Notify(ctx, NotifyOptions{
		Category:     category,
		TenantID:     tenantID,
		UserID:       brokerID,
		Recipient:    email,
		Subject:      subject,
		BodyHTML:     c.emailNotifier.BuildEmail(subject, bodyHTML, btnText, c.emailNotifier.FrontendLink(btnURL), internalEmailDisclaimer()),
		EventID:      eventID,
		SkipIfOnline: false,
		Force:        force,
	})
}

// notifyOperatorEmail envia o braço de e-mail para um operador/corretor do
// canal do negócio (identidade mista: broker id ou account id — o resolver
// cobre os dois).
func (c *EventConsumer) notifyOperatorEmail(ctx context.Context, eventID, tenantID, userID string, category EmailCategory, subject, bodyHTML, btnText, btnURL string) {
	if c.emailNotifier == nil || c.recipientResolver == nil || userID == "" {
		return
	}
	email, err := c.recipientResolver.ParticipantEmailByID(ctx, tenantID, userID)
	if err != nil {
		logger.Warn("email notify: failed to resolve operator email",
			"tenant_id", tenantID, "user_id", userID, "error", err.Error())
		return
	}
	_, _ = c.emailNotifier.Notify(ctx, NotifyOptions{
		Category:     category,
		TenantID:     tenantID,
		UserID:       userID,
		Recipient:    email,
		Subject:      subject,
		BodyHTML:     c.emailNotifier.BuildEmail(subject, bodyHTML, btnText, c.emailNotifier.FrontendLink(btnURL), internalEmailDisclaimer()),
		EventID:      eventID,
		SkipIfOnline: true,
		Force:        false,
	})
}

// notifyEmailDirect envia o braço de e-mail para um ENDEREÇO sem conta de
// portal (parte do negócio ainda não convidada). A identidade de dedup/prefs
// vira "email:<endereço>" para não colidir com account/broker ids no ledger;
// não há presença para suprimir (quem não tem conta não tem sessão WS).
func (c *EventConsumer) notifyEmailDirect(ctx context.Context, eventID, tenantID, email string, category EmailCategory, subject, bodyHTML, btnText, btnURL string, force bool) {
	if c.emailNotifier == nil || email == "" {
		return
	}
	_, _ = c.emailNotifier.Notify(ctx, NotifyOptions{
		Category:     category,
		TenantID:     tenantID,
		UserID:       "email:" + email,
		Recipient:    email,
		Subject:      subject,
		BodyHTML:     c.emailNotifier.BuildEmail(subject, bodyHTML, btnText, c.emailNotifier.FrontendLink(btnURL), clientEmailDisclaimer()),
		EventID:      eventID,
		SkipIfOnline: false,
		Force:        force,
	})
}

// leadAssignedPayload é o shape do evento lead.assigned consumido por
// comm_lead_assigned. LeadName foi acrescentado no beta de e-mail
// transacional (toca-crm passou a publicar); eventos antigos chegam sem ele.
type leadAssignedPayload struct {
	LeadID    string   `json:"lead_id"`
	LeadName  string   `json:"lead_name"`
	TenantID  string   `json:"tenant_id"`
	BrokerID  string   `json:"broker_id"`
	Channels  []string `json:"channels"`
	Stage     string   `json:"stage"`
	Method    string   `json:"method"`
	Tags      []string `json:"tags"`
	MessageID string   `json:"message_id,omitempty"`
}

func clientEmailDisclaimer() string {
	return "Você está recebendo este e-mail porque acompanha este processo no portal. Acesse a plataforma para gerenciar suas preferências de notificação."
}

func internalEmailDisclaimer() string {
	return "Você está recebendo este e-mail como participante da operação na plataforma."
}

func (c *EventConsumer) resolveRecipients(ctx context.Context, tenantID string, functionalRole string) []string {
	if c.authClient == nil {
		return []string{}
	}
	members, err := c.authClient.ListCompanyMembersInternal(ctx, tenantID, functionalRole)
	if err != nil {
		logger.Warn("failed to resolve recipients", "tenant_id", tenantID, "error", err.Error())
		return []string{}
	}
	out := make([]string, 0, len(members))
	for _, m := range members {
		if m.AccountID != "" {
			out = append(out, m.AccountID)
		}
	}
	return out
}

// resolveCompanyAdmins returns the company members that should receive
// company-level notifications (partnership invites/acceptances). It prefers
// members whose Profile is "owner" or "admin"; if none match (or the lookup
// fails) it falls back to every member so a notification is never silently
// dropped. The returned slice carries the account id + email so the caller
// can fan out both an in-app notification and an email.
func (c *EventConsumer) resolveCompanyAdmins(ctx context.Context, companyID string) []clients.CompanyMemberDetail {
	if c.authClient == nil || companyID == "" {
		return nil
	}
	members, err := c.authClient.ListCompanyMembersInternal(ctx, companyID, "")
	if err != nil {
		logger.Warn("failed to resolve company admins", "company_id", companyID, "error", err.Error())
		return nil
	}
	admins := make([]clients.CompanyMemberDetail, 0, len(members))
	for _, m := range members {
		if m.AccountID == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(m.Profile)) {
		case "owner", "admin":
			admins = append(admins, m)
		}
	}
	if len(admins) > 0 {
		return admins
	}
	// No explicit owner/admin — fall back to all members so the invite is
	// still surfaced to someone in the company.
	all := make([]clients.CompanyMemberDetail, 0, len(members))
	for _, m := range members {
		if m.AccountID != "" {
			all = append(all, m)
		}
	}
	return all
}

// partnershipTypeNoun returns the pt-BR noun for a partnership type, used to
// build user-facing notification copy. "franchise" -> "franquia", anything
// else (incl. "partner") -> "parceria".
func partnershipTypeNoun(t string) string {
	if strings.ToLower(strings.TrimSpace(t)) == "franchise" {
		return "franquia"
	}
	return "parceria"
}

func (c *EventConsumer) Start(ctx context.Context) error {
	// deal.created -> Ensure Channel
	err := c.eventBus.SubscribeWithQueue("comm_deal_created", "deal.created", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			DealID            string  `json:"deal_id"`
			TenantID          string  `json:"tenant_id"`
			CustomerAccountID string  `json:"customer_account_id"`
			BrokerID          string  `json:"broker_id"`
			BrokerName        string  `json:"broker_name"`
			PropertyID        string  `json:"property_id"`
			Title             string  `json:"title"`
			DealType          string  `json:"deal_type"`
			DealValue         float64 `json:"deal_value"`
			Parties           []struct {
				Name   string `json:"name"`
				Role   string `json:"role"`
				Type   string `json:"type"`
				LeadID string `json:"lead_id"`
			} `json:"parties"`
		}
		b, _ := json.Marshal(evt.Payload())
		if err := json.Unmarshal(b, &payload); err != nil {
			return err
		}

		participants := []string{}
		if payload.CustomerAccountID != "" {
			participants = append(participants, payload.CustomerAccountID)
		}
		if payload.BrokerID != "" {
			participants = append(participants, payload.BrokerID)
		}

		// Build channel info sections from deal context.
		// Required field (PropertyID) is validated inside BuildChannelInfo;
		// optional fields gracefully degrade (empty sections are omitted).
		var info *message.ChannelInfo
		pp := DealPayload{
			DealID:     payload.DealID,
			TenantID:   payload.TenantID,
			PropertyID: payload.PropertyID,
			Title:      payload.Title,
			DealType:   payload.DealType,
			DealValue:  payload.DealValue,
			BrokerID:   payload.BrokerID,
			BrokerName: payload.BrokerName,
		}
		for _, p := range payload.Parties {
			pp.Parties = append(pp.Parties, PartyPayload{
				Name:   p.Name,
				Role:   p.Role,
				Type:   p.Type,
				LeadID: p.LeadID,
			})
		}
		if built, err := BuildChannelInfo(pp); err != nil {
			logger.WarnContext(ctx, "deal_created: failed to build channel info",
				"deal_id", payload.DealID,
				"error", err,
			)
		} else {
			info = built
		}

		_, err := c.msgService.EnsureChannelForDealWithInfo(ctx, payload.TenantID, payload.DealID, participants, info)
		// Fan out to operators (admins/managers/brokers) so the Esteira
		// Kanban can react to new deals without a manual refresh. Clients
		// don't need this — they're already linked via the channel above.
		if c.msgService != nil && c.msgService.Hub() != nil && payload.TenantID != "" {
			c.msgService.Hub().BroadcastEventToTenantInternal(payload.TenantID, "deal_created", payload)
		}
		return err
	})
	if err != nil {
		return err
	}

	// deal.status_changed -> Notify clients
	err = c.eventBus.SubscribeWithQueue("comm_deal_status_changed", "deal.status_changed", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			DealID             string   `json:"deal_id"`
			TenantID           string   `json:"tenant_id"`
			OldStatus          string   `json:"old_status"`
			NewStatus          string   `json:"new_status"`
			CustomerAccountIDs []string `json:"customer_account_ids"`
			CustomerEmails     []string `json:"customer_emails"`
			ActorUserID        string   `json:"actor_user_id,omitempty"`
		}
		b, _ := json.Marshal(evt.Payload())
		if err := json.Unmarshal(b, &payload); err != nil {
			return err
		}

		if len(payload.CustomerAccountIDs) > 0 {
			for _, accountID := range payload.CustomerAccountIDs {
				data := map[string]interface{}{"tenant_id": payload.TenantID, "deal_id": payload.DealID}
				dedupID := events.DeriveNotificationID(evt.EventID(), accountID, "deal_status_changed", payload.NewStatus)
				_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.TenantID, accountID, notification.InAppTypeSystem, notification.PriorityNormal, "Atualização do Negócio", fmt.Sprintf("Status: %s", dealStatusLabel(payload.NewStatus)), fmt.Sprintf("/portal/deals/%s", payload.DealID), data)

				// Braço de e-mail (beta): o cliente recebe a atualização de
				// etapa; quem executou a mudança não recebe o próprio eco.
				if c.emailNotifier != nil && accountID != payload.ActorUserID {
					c.notifyClientEmail(ctx, evt.EventID(), payload.TenantID, accountID,
						CategoryDealUpdates,
						"Atualização do seu negócio",
						fmt.Sprintf("<p style='margin: 0 0 24px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>Seu negócio avançou para a etapa <strong>%s</strong>.</p>", dealStatusLabel(payload.NewStatus)),
						"Acompanhar no portal",
						fmt.Sprintf("/portal/deals/%s", payload.DealID),
						false)
				}
			}
		}
		// Partes ainda SEM conta de portal recebem a atualização por e-mail
		// direto (o e-mail é a única via até o convite ao portal existir).
		if c.emailNotifier != nil {
			for _, email := range payload.CustomerEmails {
				if email == "" {
					continue
				}
				c.notifyEmailDirect(ctx, evt.EventID(), payload.TenantID, email,
					CategoryDealUpdates,
					"Atualização do seu negócio",
					fmt.Sprintf("<p style='margin: 0 0 24px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>Seu negócio avançou para a etapa <strong>%s</strong>.</p>", dealStatusLabel(payload.NewStatus)),
					"Acompanhar no portal",
					fmt.Sprintf("/portal/deals/%s", payload.DealID),
					false)
			}
		}
		if c.msgService != nil && c.msgService.Hub() != nil {
			if len(payload.CustomerAccountIDs) > 0 {
				c.msgService.Hub().BroadcastEvent(payload.TenantID, payload.CustomerAccountIDs, "deal_status_changed", payload)
			}
			// Mirror to operators so the Esteira Digital reflects stage
			// changes in real time. The client-targeted broadcast above
			// only reaches role="cliente"; this one excludes them.
			if payload.TenantID != "" {
				c.msgService.Hub().BroadcastEventToTenantInternal(payload.TenantID, "deal_status_changed", payload)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// proposal.sent -> Notify clients
	err = c.eventBus.SubscribeWithQueue("comm_proposal_sent", "proposal.sent", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			DealID             string   `json:"deal_id"`
			TenantID           string   `json:"tenant_id"`
			ProposalID         string   `json:"proposal_id"`
			Value              float64  `json:"value"`
			CustomerAccountIDs []string `json:"customer_account_ids"`
			CustomerEmails     []string `json:"customer_emails"`
		}
		b, _ := json.Marshal(evt.Payload())
		if err := json.Unmarshal(b, &payload); err != nil {
			return err
		}
		for _, accountID := range payload.CustomerAccountIDs {
			data := map[string]interface{}{"tenant_id": payload.TenantID, "deal_id": payload.DealID, "proposal_id": payload.ProposalID}
			dedupID := events.DeriveNotificationID(evt.EventID(), accountID, "proposal_sent")
			_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.TenantID, accountID, notification.InAppTypeSystem, notification.PriorityImportant, "Nova Proposta", proposalSentInAppBody(payload.Value), fmt.Sprintf("/portal/deals/%s", payload.DealID), data)

			// Braço de e-mail (beta): proposta nova é um marco de negociação.
			if c.emailNotifier != nil {
				c.notifyClientEmail(ctx, evt.EventID(), payload.TenantID, accountID,
					CategoryProposals,
					proposalSentSubject(),
					proposalSentClientBody(payload.Value),
					"Ver proposta",
					fmt.Sprintf("/portal/deals/%s", payload.DealID),
					false)
			}
		}
		// Partes ainda SEM conta de portal recebem a proposta por e-mail direto.
		if c.emailNotifier != nil {
			for _, email := range payload.CustomerEmails {
				if email == "" {
					continue
				}
				c.notifyEmailDirect(ctx, evt.EventID(), payload.TenantID, email,
					CategoryProposals,
					proposalSentSubject(),
					proposalSentClientBody(payload.Value),
					"Ver proposta",
					fmt.Sprintf("/portal/deals/%s", payload.DealID),
					false)
			}
		}
		if c.msgService != nil && c.msgService.Hub() != nil && len(payload.CustomerAccountIDs) > 0 {
			c.msgService.Hub().BroadcastEvent(payload.TenantID, payload.CustomerAccountIDs, "proposal_update", payload)
		}
		// Operators always get a mirrored fan-out so the Esteira reflects
		// new proposals even when no customer account is linked yet.
		if c.msgService != nil && c.msgService.Hub() != nil && payload.TenantID != "" {
			c.msgService.Hub().BroadcastEventToTenantInternal(payload.TenantID, "proposal_update", payload)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// proposal.responded -> Notify operators
	err = c.eventBus.SubscribeWithQueue("comm_proposal_responded", "proposal.responded", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			DealID            string   `json:"deal_id"`
			TenantID          string   `json:"tenant_id"`
			ProposalID        string   `json:"proposal_id"`
			Action            string   `json:"action"`
			CustomerAccountID string   `json:"customer_account_id"`
			DealValue         *float64 `json:"deal_value,omitempty"`
		}
		b, _ := json.Marshal(evt.Payload())
		if err := json.Unmarshal(b, &payload); err != nil {
			return err
		}

		// Mensagem de notificação enriquecida com o valor do deal quando
		// disponível — ajuda operadores a contextualizar rapidamente.
		actionLabel := payload.Action
		switch payload.Action {
		case "accepted":
			actionLabel = "aceitou a proposta"
		case "rejected":
			actionLabel = "rejeitou a proposta"
		case "counter_offer":
			actionLabel = "enviou contraproposta"
		case "counter_offer_accepted":
			actionLabel = "contraproposta aceita"
		}
		notifBody := fmt.Sprintf("Cliente %s", actionLabel)
		if payload.DealValue != nil && *payload.DealValue > 0 {
			notifBody = fmt.Sprintf("Cliente %s (valor do negócio: %s)", actionLabel, money.BRL(*payload.DealValue))
		}

		channels, _ := c.msgService.repo.ListChannels(ctx, payload.TenantID, "", payload.DealID)
		recipients := []string{}
		seen := map[string]bool{}
		for _, ch := range channels {
			if ch.Type != message.ChannelTypeDeal {
				continue
			}
			for _, p := range ch.Participants {
				if p == payload.CustomerAccountID || p == "" {
					continue
				}
				if seen[p] {
					continue
				}
				seen[p] = true
				recipients = append(recipients, p)
			}
		}

		for _, userID := range recipients {
			data := map[string]interface{}{"tenant_id": payload.TenantID, "deal_id": payload.DealID, "proposal_id": payload.ProposalID}
			dedupID := events.DeriveNotificationID(evt.EventID(), userID, "proposal_responded")
			_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.TenantID, userID, notification.InAppTypeSystem, notification.PriorityImportant, "Resposta de Proposta", notifBody, fmt.Sprintf("/esteira/deals/%s", payload.DealID), data)

			// Braço de e-mail (beta): corretor/operador do canal do negócio
			// é avisado por e-mail quando o cliente responde a proposta.
			if c.emailNotifier != nil {
				c.notifyOperatorEmail(ctx, evt.EventID(), payload.TenantID, userID,
					CategoryProposals,
					"Resposta de proposta",
					fmt.Sprintf("<p style='margin: 0 0 24px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>%s.</p>", notifBody),
					"Ver negociação",
					fmt.Sprintf("/esteira/deals/%s", payload.DealID))
			}
		}
		if c.msgService != nil && c.msgService.Hub() != nil && len(recipients) > 0 {
			c.msgService.Hub().BroadcastEvent(payload.TenantID, recipients, "proposal_update", payload)
		}
		// Tenant-wide fallback for admins/managers who aren't channel
		// participants but still need to see proposal responses in the
		// Esteira Digital.
		if c.msgService != nil && c.msgService.Hub() != nil && payload.TenantID != "" {
			c.msgService.Hub().BroadcastEventToTenantInternal(payload.TenantID, "proposal_update", payload)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// proposal.followup_due -> lembrete temporal de proposta sem resposta
	// (beta §F6). O motor em toca-deal publica UM evento por grupo (proposta +
	// estágio) com event_id estável; aqui o próprio evt.EventID() é a chave de
	// dedup, então uma republicação do mesmo grupo (retry do outbox, queda
	// entre publish e mark) não vira um segundo e-mail.
	err = c.eventBus.SubscribeWithQueue("comm_proposal_followup_email", "proposal.followup_due", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			DealID             string   `json:"deal_id"`
			TenantID           string   `json:"tenant_id"`
			ProposalID         string   `json:"proposal_id"`
			Stage              int      `json:"stage"`
			MaxStages          int      `json:"max_stages"`
			DaysPending        int      `json:"days_pending"`
			ProposalStatus     string   `json:"proposal_status"`
			AwaitingFor        string   `json:"awaiting_for"`
			Value              float64  `json:"value"`
			CounterValue       *float64 `json:"counter_value"`
			BrokerID           string   `json:"broker_id"`
			CustomerAccountIDs []string `json:"customer_account_ids"`
			CustomerEmails     []string `json:"customer_emails"`
		}
		b, _ := json.Marshal(evt.Payload())
		if err := json.Unmarshal(b, &payload); err != nil {
			return err
		}
		if payload.TenantID == "" || payload.DealID == "" || payload.ProposalID == "" {
			return nil
		}
		stage := payload.Stage
		if stage <= 0 {
			stage = 1
		}
		// Chave de dedup LÓGICA (proposta + estágio), derivada do payload: o
		// EventID de um evento que passou pelo outbox é o id da linha do
		// outbox, novo a cada publicação. Ver proposalFollowUpGroupID.
		groupID := proposalFollowUpGroupID(payload.ProposalID, stage)
		where := proposalFollowUpWhere(payload.AwaitingFor, payload.Value, payload.CounterValue, payload.DaysPending)
		link := fmt.Sprintf("/esteira/deals/%s", payload.DealID)

		if payload.AwaitingFor == "broker" {
			// Contraproposta do cliente aguardando o corretor: a bola está
			// com o operador.
			if c.emailNotifier != nil && payload.BrokerID != "" {
				c.notifyOperatorEmail(ctx, groupID, payload.TenantID, payload.BrokerID,
					CategoryProposals, proposalFollowUpBrokerSubject(payload.DaysPending),
					proposalFollowUpBrokerBody(where), "Ver negociação", link)
			}
			if payload.BrokerID != "" {
				data := map[string]interface{}{"tenant_id": payload.TenantID, "deal_id": payload.DealID, "proposal_id": payload.ProposalID, "stage": stage}
				dedupID := events.DeriveNotificationID(groupID, payload.BrokerID, "proposal_followup_inapp")
				_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.TenantID, payload.BrokerID, notification.InAppTypeSystem, notification.PriorityImportant, "Contraproposta sem resposta", proposalFollowUpBrokerInApp(payload.DaysPending), link, data)
			}
			return nil
		}

		subject := proposalFollowUpClientSubject(payload.DaysPending)
		body := proposalFollowUpClientBody(where)
		portalLink := fmt.Sprintf("/portal/deals/%s", payload.DealID)
		for _, accountID := range payload.CustomerAccountIDs {
			data := map[string]interface{}{"tenant_id": payload.TenantID, "deal_id": payload.DealID, "proposal_id": payload.ProposalID, "stage": stage}
			dedupID := events.DeriveNotificationID(groupID, accountID, "proposal_followup_inapp")
			_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.TenantID, accountID, notification.InAppTypeSystem, notification.PriorityImportant, "Proposta aguardando sua resposta", proposalFollowUpClientInApp(payload.DaysPending), portalLink, data)
			if c.emailNotifier != nil {
				c.notifyClientEmail(ctx, groupID, payload.TenantID, accountID,
					CategoryProposals, subject, body, "Ver proposta", portalLink, false)
			}
		}
		if c.emailNotifier != nil {
			for _, email := range payload.CustomerEmails {
				if email == "" {
					continue
				}
				c.notifyEmailDirect(ctx, groupID, payload.TenantID, email,
					CategoryProposals, subject, body, "Ver proposta", portalLink, false)
			}
			// Cópia para o corretor responsável: ele precisa saber que o
			// cliente foi cobrado (e por qual canal) para agir no telefone.
			if payload.BrokerID != "" {
				c.notifyOperatorEmail(ctx, groupID, payload.TenantID, payload.BrokerID,
					CategoryProposals, proposalFollowUpCopySubject(),
					proposalFollowUpCopyBody(stage, payload.MaxStages, where), "Ver negociação", link)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// client.ticket.created -> Notify
	err = c.eventBus.SubscribeWithQueue("comm_ticket_created", "client.ticket.created", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			TicketID          string `json:"ticket_id"`
			DealID            string `json:"deal_id"`
			TenantID          string `json:"tenant_id"`
			CustomerAccountID string `json:"customer_account_id"`
			Category          string `json:"category"`
		}
		b, _ := json.Marshal(evt.Payload())
		_ = json.Unmarshal(b, &payload)

		channels, _ := c.msgService.repo.ListChannels(ctx, payload.TenantID, "", payload.DealID)
		for _, ch := range channels {
			if ch.Type == message.ChannelTypeDeal {
				for _, p := range ch.Participants {
					if p != payload.CustomerAccountID {
						data := map[string]interface{}{"tenant_id": payload.TenantID, "deal_id": payload.DealID, "ticket_id": payload.TicketID}
						dedupID := events.DeriveNotificationID(evt.EventID(), p, "client_ticket_created")
						_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.TenantID, p, notification.InAppTypeSystem, notification.PriorityImportant, "Novo Chamado", fmt.Sprintf("%s · %s", ticketTypeLabel("maintenance"), payload.Category), "/chamados", data)
					}
				}
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	// ticket.created -> Notify operators
	err = c.eventBus.SubscribeWithQueue("comm_ticket_created_v2", "ticket.created", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			TicketID          string `json:"ticket_id"`
			DealID            string `json:"deal_id"`
			TenantID          string `json:"tenant_id"`
			CustomerAccountID string `json:"customer_account_id"`
			Type              string `json:"type"`
			Category          string `json:"category"`
			Subject           string `json:"subject"`
		}
		b, _ := json.Marshal(evt.Payload())
		if err := json.Unmarshal(b, &payload); err != nil {
			return err
		}

		channels, _ := c.msgService.repo.ListChannels(ctx, payload.TenantID, "", payload.DealID)
		for _, ch := range channels {
			if ch.Type != message.ChannelTypeDeal {
				continue
			}
			for _, p := range ch.Participants {
				if p == payload.CustomerAccountID {
					continue
				}
				data := map[string]interface{}{"tenant_id": payload.TenantID, "deal_id": payload.DealID, "ticket_id": payload.TicketID, "ticket_type": payload.Type, "ticket_category": payload.Category, "ticket_subject": payload.Subject}
				dedupID := events.DeriveNotificationID(evt.EventID(), p, "ticket_created")
				_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.TenantID, p, notification.InAppTypeSystem, notification.PriorityImportant, "Novo Chamado", fmt.Sprintf("%s · %s", ticketTypeLabel(payload.Type), payload.Subject), "/chamados", data)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// ticket.status_changed -> Notify both sides
	err = c.eventBus.SubscribeWithQueue("comm_ticket_status_changed", "ticket.status_changed", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			TicketID          string `json:"ticket_id"`
			DealID            string `json:"deal_id"`
			TenantID          string `json:"tenant_id"`
			OldStatus         string `json:"old_status"`
			NewStatus         string `json:"new_status"`
			CustomerAccountID string `json:"customer_account_id"`
			AssignedToID      string `json:"assigned_to_id"`
		}
		b, _ := json.Marshal(evt.Payload())
		if err := json.Unmarshal(b, &payload); err != nil {
			return err
		}

		recipients := []string{}
		seen := map[string]bool{}
		for _, id := range []string{payload.CustomerAccountID, payload.AssignedToID} {
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			recipients = append(recipients, id)
		}
		for _, userID := range recipients {
			data := map[string]interface{}{"tenant_id": payload.TenantID, "ticket_id": payload.TicketID, "deal_id": payload.DealID, "ticket_status": payload.NewStatus}
			dedupID := events.DeriveNotificationID(evt.EventID(), userID, "ticket_status_changed", payload.NewStatus)
			_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.TenantID, userID, notification.InAppTypeSystem, notification.PriorityNormal, "Atualização de Chamado", fmt.Sprintf("Status: %s", ticketStatusLabel(payload.NewStatus)), "/chamados", data)
		}
		if c.msgService != nil && c.msgService.Hub() != nil && len(recipients) > 0 {
			c.msgService.Hub().BroadcastEvent(payload.TenantID, recipients, "ticket_update", payload)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// ticket.comment_added -> Notify other party
	err = c.eventBus.SubscribeWithQueue("comm_ticket_comment_added", "ticket.comment_added", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			TicketID          string `json:"ticket_id"`
			DealID            string `json:"deal_id"`
			TenantID          string `json:"tenant_id"`
			CommentID         string `json:"comment_id"`
			AuthorID          string `json:"author_id"`
			AuthorType        string `json:"author_type"`
			CustomerAccountID string `json:"customer_account_id"`
			AssignedToID      string `json:"assigned_to_id"`
		}
		b, _ := json.Marshal(evt.Payload())
		if err := json.Unmarshal(b, &payload); err != nil {
			return err
		}

		recipients := []string{}
		if payload.AuthorType == "operator" {
			if payload.CustomerAccountID != "" {
				recipients = append(recipients, payload.CustomerAccountID)
			}
		} else {
			if payload.AssignedToID != "" {
				recipients = append(recipients, payload.AssignedToID)
			}
		}

		for _, userID := range recipients {
			data := map[string]interface{}{"tenant_id": payload.TenantID, "ticket_id": payload.TicketID, "deal_id": payload.DealID, "comment_id": payload.CommentID}
			dedupID := events.DeriveNotificationID(evt.EventID(), userID, "ticket_comment_added", payload.CommentID)
			_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.TenantID, userID, notification.InAppTypeSystem, notification.PriorityNormal, "Novo Comentário", "Novo comentário em um chamado", "/chamados", data)
		}
		if c.msgService != nil && c.msgService.Hub() != nil && len(recipients) > 0 {
			c.msgService.Hub().BroadcastEvent(payload.TenantID, recipients, "ticket_comment", payload)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// ticket.assigned -> Notify assignee (#450). Sem este subscriber o
	// novo assignee descobria o chamado apenas ao abrir a lista — SLA
	// de aceite degradado. Diferenciamos primeira atribuição (Old vazio)
	// de reassignment para que o título da notificação seja mais útil.
	err = c.eventBus.SubscribeWithQueue("comm_ticket_assigned", "ticket.assigned", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			TicketID          string `json:"ticket_id"`
			DealID            string `json:"deal_id"`
			TenantID          string `json:"tenant_id"`
			OldAssignedToID   string `json:"old_assigned_to_id"`
			AssignedToID      string `json:"assigned_to_id"`
			AssignedToName    string `json:"assigned_to_name"`
			CustomerAccountID string `json:"customer_account_id"`
			Subject           string `json:"subject"`
			Priority          string `json:"priority"`
		}
		b, _ := json.Marshal(evt.Payload())
		if err := json.Unmarshal(b, &payload); err != nil {
			return err
		}

		if payload.AssignedToID == "" {
			return nil // defensive: assign sem assignee
		}

		title := "Novo chamado atribuído"
		if payload.OldAssignedToID != "" && payload.OldAssignedToID != payload.AssignedToID {
			title = "Chamado reatribuído para você"
		}
		desc := payload.Subject
		if desc == "" {
			desc = "Você foi atribuído a um chamado"
		}

		data := map[string]interface{}{
			"tenant_id":  payload.TenantID,
			"ticket_id":  payload.TicketID,
			"deal_id":    payload.DealID,
			"priority":   payload.Priority,
			"reassigned": payload.OldAssignedToID != "" && payload.OldAssignedToID != payload.AssignedToID,
		}
		dedupID := events.DeriveNotificationID(evt.EventID(), payload.AssignedToID, "ticket_assigned", payload.TicketID)
		// Priority do ticket determina a urgência da notificação. Urgent
		// vira PriorityUrgent; high vira PriorityImportant; restantes
		// usam Normal — alinhado com inapp.go enum.
		prio := notification.PriorityNormal
		switch payload.Priority {
		case "urgent":
			prio = notification.PriorityUrgent
		case "high":
			prio = notification.PriorityImportant
		}
		_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.TenantID, payload.AssignedToID, notification.InAppTypeSystem, prio, title, desc, "/chamados", data)

		// WS broadcast no canal pessoal do assignee — frontend
		// (NotificationBell) recebe sem precisar reabrir a lista.
		if c.msgService != nil && c.msgService.Hub() != nil {
			c.msgService.Hub().BroadcastEvent(payload.TenantID, []string{payload.AssignedToID}, "ticket_assigned", payload)
		}

		return nil
	})
	if err != nil {
		return err
	}

	// client.appointment.requested -> Notify
	err = c.eventBus.SubscribeWithQueue("comm_appointment_requested", "client.appointment.requested", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			AppointmentID     string `json:"appointment_id"`
			DealID            string `json:"deal_id"`
			TenantID          string `json:"tenant_id"`
			Date              string `json:"date"`
			CustomerAccountID string `json:"customer_account_id"`
			BrokerID          string `json:"broker_id"`
		}
		b, _ := json.Marshal(evt.Payload())
		_ = json.Unmarshal(b, &payload)

		targets := map[string]struct{}{}
		if payload.BrokerID != "" && payload.BrokerID != payload.CustomerAccountID {
			targets[payload.BrokerID] = struct{}{}
		}

		channels, _ := c.msgService.repo.ListChannels(ctx, payload.TenantID, "", payload.DealID)
		for _, ch := range channels {
			if ch.Type != message.ChannelTypeDeal {
				continue
			}
			for _, p := range ch.Participants {
				if p == "" || p == payload.CustomerAccountID {
					continue
				}
				targets[p] = struct{}{}
			}
		}

		for p := range targets {
			data := map[string]interface{}{"tenant_id": payload.TenantID, "deal_id": payload.DealID, "appointment_id": payload.AppointmentID}
			dedupID := events.DeriveNotificationID(evt.EventID(), p, "appointment_requested", payload.AppointmentID)
			_, _ = c.inAppService.CreateIdempotent(
				ctx,
				dedupID,
				payload.TenantID,
				p,
				notification.InAppTypeSystem,
				notification.PriorityNormal,
				"Visita Solicitada",
				fmt.Sprintf("Visita solicitada para %s", payload.Date),
				fmt.Sprintf("/agenda?apt=%s", payload.AppointmentID),
				data,
			)
		}

		return nil
	})
	if err != nil {
		return err
	}

	// appointment.updated -> Notify client
	err = c.eventBus.SubscribeWithQueue("comm_appointment_updated", "appointment.updated", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			AppointmentID     string `json:"appointment_id"`
			DealID            string `json:"deal_id"`
			TenantID          string `json:"tenant_id"`
			Status            string `json:"status"`
			Date              string `json:"date"`
			Time              string `json:"time"`
			EndTime           string `json:"end_time"`
			LeadID            string `json:"lead_id"`
			CustomerAccountID string `json:"customer_account_id"`
			AgentID           string `json:"agent_id"`
			Title             string `json:"title"`
			CalendarURL       string `json:"calendar_url"`
		}
		b, _ := json.Marshal(evt.Payload())
		if err := json.Unmarshal(b, &payload); err != nil {
			return err
		}
		if payload.CustomerAccountID != "" {
			data := map[string]interface{}{"tenant_id": payload.TenantID, "deal_id": payload.DealID, "appointment_id": payload.AppointmentID, "lead_id": payload.LeadID}
			dedupID := events.DeriveNotificationID(evt.EventID(), payload.CustomerAccountID, "appointment_updated", payload.AppointmentID, payload.Status)
			_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.TenantID, payload.CustomerAccountID, notification.InAppTypeSystem, notification.PriorityNormal, "Agendamento Atualizado", fmt.Sprintf("Status: %s · %s %s–%s", payload.Status, payload.Date, payload.Time, payload.EndTime), appointmentPortalLink(payload.DealID), data)
			if c.msgService != nil && c.msgService.Hub() != nil {
				c.msgService.Hub().BroadcastEvent(payload.TenantID, []string{payload.CustomerAccountID}, "appointment_update", payload)
			}
			// Braço de e-mail (beta F2): cliente recebe os marcos da agenda
			// (confirmado/reagendado/cancelado); demais status só in-app.
			if c.emailNotifier != nil && appointmentStatusNeedsEmail(payload.Status) {
				// Cancelamento não convida a adicionar de novo: o link só chega
				// no payload quando o compromisso segue marcado.
				btnText, btnURL, secondaryText, secondaryURL := appointmentCalendarCTA(payload.CalendarURL, appointmentPortalLink(payload.DealID))
				c.notifyClientEmailWithCTA(ctx, evt.EventID(), payload.TenantID, payload.CustomerAccountID,
					CategoryAppointments,
					appointmentClientSubjectChanged(payload.Status, payload.Title),
					appointmentClientBodyChanged(payload.Status, payload.Title, appointmentWhen(payload.Date, payload.Time)),
					btnText, btnURL, secondaryText, secondaryURL,
					false)
			}
		}
		// Braço de e-mail (beta F2): corretor responsável também recebe os
		// marcos — o contrato passou a carregar agent_id (F2, toca-calendar).
		if c.emailNotifier != nil && payload.AgentID != "" && appointmentStatusNeedsEmail(payload.Status) {
			c.notifyOperatorEmail(ctx, evt.EventID(), payload.TenantID, payload.AgentID,
				CategoryAppointments,
				appointmentOperatorSubjectChanged(payload.Status, payload.Title),
				appointmentOperatorBodyChanged(payload.Status, payload.Title, appointmentWhen(payload.Date, payload.Time)),
				"Ver agenda",
				"/agenda?compromisso="+payload.AppointmentID)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// appointment.cancelled -> Notify client
	err = c.eventBus.SubscribeWithQueue("comm_appointment_cancelled", "appointment.cancelled", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			AppointmentID     string `json:"appointment_id"`
			DealID            string `json:"deal_id"`
			TenantID          string `json:"tenant_id"`
			Reason            string `json:"reason"`
			CustomerAccountID string `json:"customer_account_id"`
			AgentID           string `json:"agent_id"`
			Title             string `json:"title"`
			Date              string `json:"date"`
			Time              string `json:"time"`
		}
		b, _ := json.Marshal(evt.Payload())
		if err := json.Unmarshal(b, &payload); err != nil {
			return err
		}
		if payload.CustomerAccountID != "" {
			data := map[string]interface{}{"tenant_id": payload.TenantID, "deal_id": payload.DealID, "appointment_id": payload.AppointmentID}
			dedupID := events.DeriveNotificationID(evt.EventID(), payload.CustomerAccountID, "appointment_cancelled", payload.AppointmentID)
			_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.TenantID, payload.CustomerAccountID, notification.InAppTypeSystem, notification.PriorityNormal, "Agendamento Cancelado", "Um agendamento foi cancelado", appointmentPortalLink(payload.DealID), data)
			if c.msgService != nil && c.msgService.Hub() != nil {
				c.msgService.Hub().BroadcastEvent(payload.TenantID, []string{payload.CustomerAccountID}, "appointment_update", payload)
			}
			// Braço de e-mail (beta F2): cancelamento é marco — cliente e
			// corretor recebem; demais status seguem só in-app.
			if c.emailNotifier != nil {
				c.notifyClientEmail(ctx, evt.EventID(), payload.TenantID, payload.CustomerAccountID,
					CategoryAppointments,
					appointmentClientSubjectChanged("Cancelado", payload.Title),
					appointmentClientBodyChanged("Cancelado", payload.Title, appointmentWhen(payload.Date, payload.Time)),
					"Ver no portal",
					appointmentPortalLink(payload.DealID),
					false)
			}
		}
		if c.emailNotifier != nil && payload.AgentID != "" {
			c.notifyOperatorEmail(ctx, evt.EventID(), payload.TenantID, payload.AgentID,
				CategoryAppointments,
				appointmentOperatorSubjectChanged("Cancelado", payload.Title),
				appointmentOperatorBodyChanged("Cancelado", payload.Title, appointmentWhen(payload.Date, payload.Time)),
				"Ver agenda",
				"/agenda")
		}
		return nil
	})
	if err != nil {
		return err
	}

	// appointment.created -> Notify client (beta F2)
	// O payload é o appointment.Appointment serializado (mesmo shape que o
	// lead_visit_sync do toca-crm consome) — espelhamos só os campos usados.
	err = c.eventBus.SubscribeWithQueue("comm_appointment_created", "appointment.created", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			ID                string `json:"id"`
			TenantID          string `json:"tenant_id"`
			Title             string `json:"title"`
			Date              string `json:"date"`
			Time              string `json:"time"`
			EndTime           string `json:"end_time"`
			Type              string `json:"type"`
			LeadID            string `json:"lead_id"`
			AgentID           string `json:"agent_id"`
			DealID            string `json:"deal_id"`
			CustomerAccountID string `json:"customer_account_id"`
			Status            string `json:"status"`
			CalendarURL       string `json:"calendar_url"`
		}
		b, _ := json.Marshal(evt.Payload())
		if err := json.Unmarshal(b, &payload); err != nil {
			return err
		}
		if payload.ID == "" || payload.TenantID == "" {
			return nil
		}
		// Agendamentos internos (sem cliente do portal vinculado) seguem só
		// na agenda do operador — nada para notificar aqui.
		if payload.CustomerAccountID == "" {
			return nil
		}
		when := appointmentWhen(payload.Date, payload.Time)
		data := map[string]interface{}{"tenant_id": payload.TenantID, "deal_id": payload.DealID, "appointment_id": payload.ID, "lead_id": payload.LeadID}
		dedupID := events.DeriveNotificationID(evt.EventID(), payload.CustomerAccountID, "appointment_created", payload.ID)
		_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.TenantID, payload.CustomerAccountID, notification.InAppTypeSystem, notification.PriorityNormal, "Compromisso Agendado", fmt.Sprintf("%s · %s", appointmentDisplayName(payload.Title, payload.Type), when), appointmentPortalLink(payload.DealID), data)
		if c.msgService != nil && c.msgService.Hub() != nil {
			c.msgService.Hub().BroadcastEvent(payload.TenantID, []string{payload.CustomerAccountID}, "appointment_update", payload)
		}
		// Braço de e-mail (beta F2): criação com cliente vinculado avisa o
		// cliente; o corretor que agendou não precisa de eco.
		if c.emailNotifier != nil {
			btnText, btnURL, secondaryText, secondaryURL := appointmentCalendarCTA(payload.CalendarURL, appointmentPortalLink(payload.DealID))
			c.notifyClientEmailWithCTA(ctx, evt.EventID(), payload.TenantID, payload.CustomerAccountID,
				CategoryAppointments,
				appointmentClientSubjectCreated(payload.Title, payload.Type, payload.Status),
				appointmentClientBodyCreated(payload.Title, payload.Type, when, payload.Status),
				btnText, btnURL, secondaryText, secondaryURL,
				false)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// client.visit.requested -> e-mail para o corretor (beta F2).
	// A notificação in-app do corretor continua no AuthEventHandler
	// (toca-comm.client.visit.requested); esta fila cuida só do e-mail,
	// com os gates do EmailNotifier (opt-out, presença, cooldown, dedup).
	err = c.eventBus.SubscribeWithQueue("comm_client_visit_requested_email", "client.visit.requested", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			LeadID        string `json:"lead_id"`
			BrokerID      string `json:"broker_id"`
			TenantID      string `json:"tenant_id"`
			ClientName    string `json:"client_name"`
			PropertyID    string `json:"property_id"`
			Date          string `json:"date"`
			Time          string `json:"time"`
			AppointmentID string `json:"appointment_id"`
		}
		b, _ := json.Marshal(evt.Payload())
		if err := json.Unmarshal(b, &payload); err != nil {
			return err
		}
		if payload.TenantID == "" || payload.BrokerID == "" || c.emailNotifier == nil {
			return nil
		}
		clientName := payload.ClientName
		if clientName == "" {
			clientName = "Um cliente"
		}
		link := "/agenda"
		if payload.AppointmentID != "" {
			link = "/agenda?compromisso=" + payload.AppointmentID
		}
		c.notifyOperatorEmail(ctx, evt.EventID(), payload.TenantID, payload.BrokerID,
			CategoryAppointments,
			"Visita solicitada pelo cliente",
			fmt.Sprintf("<p style='margin: 0 0 24px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'><strong>%s</strong> solicitou uma visita para <strong>%s</strong>.</p>", clientName, appointmentWhen(payload.Date, payload.Time)),
			"Ver agenda",
			link)
		return nil
	})
	if err != nil {
		return err
	}

	// Recomendações do corretor (beta F3) — "Sugestão do seu corretor".
	// Escopos lead (toca-crm) e deal (toca-deal) publicam o mesmo shape;
	// filas separadas para replay/DLQ independentes.
	handleRecommendation := func(ctx context.Context, evt events.Event) error {
		var payload struct {
			TenantID           string   `json:"tenant_id"`
			LeadID             string   `json:"lead_id"`
			DealID             string   `json:"deal_id"`
			PropertyID         string   `json:"property_id"`
			Title              string   `json:"title"`
			Neighborhood       string   `json:"neighborhood"`
			City               string   `json:"city"`
			State              string   `json:"state"`
			SalePrice          float64  `json:"sale_price"`
			RentPrice          float64  `json:"rent_price"`
			ListingType        string   `json:"listing_type"`
			ImageURL           string   `json:"image_url"`
			BrokerID           string   `json:"broker_id"`
			CustomerAccountIDs []string `json:"customer_account_ids"`
		}
		b, _ := json.Marshal(evt.Payload())
		if err := json.Unmarshal(b, &payload); err != nil {
			return err
		}
		if payload.TenantID == "" || payload.PropertyID == "" {
			return nil
		}
		portalLink := "/portal"
		if payload.DealID != "" {
			portalLink = fmt.Sprintf("/portal/deals/%s", payload.DealID)
		}
		for _, accountID := range payload.CustomerAccountIDs {
			if accountID == "" {
				continue
			}
			data := map[string]interface{}{"tenant_id": payload.TenantID, "lead_id": payload.LeadID, "deal_id": payload.DealID, "property_id": payload.PropertyID}
			dedupID := events.DeriveNotificationID(evt.EventID(), accountID, "recommendation_created", payload.PropertyID)
			_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.TenantID, accountID, notification.InAppTypeSystem, notification.PriorityImportant, "Sugestão do seu corretor", recommendationInAppContent(payload.Title, payload.Neighborhood, payload.City), portalLink, data)

			// Braço de e-mail (beta F3): cliente recebe a sugestão com
			// opt-out disponível (categoria recommendations) e cooldown.
			if c.emailNotifier != nil {
				c.notifyClientEmail(ctx, evt.EventID(), payload.TenantID, accountID,
					CategoryRecommendations,
					recommendationSubject(payload.Title),
					recommendationClientBody(payload.Title, payload.Neighborhood, payload.City, payload.State, payload.SalePrice, payload.RentPrice),
					"Ver imóvel",
					portalLink,
					false)
			}
		}
		return nil
	}

	err = c.eventBus.SubscribeWithQueue("comm_lead_recommended_property", "lead.recommended_property_set", func(ctx context.Context, evt events.Event) error {
		return handleRecommendation(ctx, evt)
	})
	if err != nil {
		return err
	}

	err = c.eventBus.SubscribeWithQueue("comm_deal_recommended_property", "deal.recommended_property_set", func(ctx context.Context, evt events.Event) error {
		return handleRecommendation(ctx, evt)
	})
	if err != nil {
		return err
	}

	// lead.status_changed -> Notify client portal
	err = c.eventBus.SubscribeWithQueue("comm_lead_status_changed", "lead.status_changed", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			LeadID            string `json:"lead_id"`
			TenantID          string `json:"tenant_id"`
			CustomerAccountID string `json:"customer_account_id"`
			OldStatus         string `json:"old_status"`
			NewStatus         string `json:"new_status"`
			MessageID         string `json:"message_id,omitempty"`
		}
		b, _ := json.Marshal(evt.Payload())
		if err := json.Unmarshal(b, &payload); err != nil {
			return err
		}
		payload.MessageID = evt.EventID()

		if payload.CustomerAccountID != "" {
			data := map[string]interface{}{"tenant_id": payload.TenantID, "lead_id": payload.LeadID}
			dedupID := events.DeriveNotificationID(evt.EventID(), payload.CustomerAccountID, "lead_status_changed", payload.NewStatus)
			_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.TenantID, payload.CustomerAccountID, notification.InAppTypeSystem, notification.PriorityNormal, "Atualização do Interesse", fmt.Sprintf("Status atualizado para %s", payload.NewStatus), "/portal/leads", data)
			if c.msgService != nil && c.msgService.Hub() != nil {
				c.msgService.Hub().BroadcastEvent(payload.TenantID, []string{payload.CustomerAccountID}, "lead_status_changed", payload)
			}
		}
		if c.msgService != nil && c.msgService.Hub() != nil {
			if payload.TenantID != "" {
				c.msgService.Hub().BroadcastEventToTenantInternal(payload.TenantID, "lead_status_changed", payload)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	err = c.eventBus.SubscribeWithQueue("comm_lead_created", "lead.created", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			LeadID           string `json:"lead_id"`
			TenantID         string `json:"tenant_id"`
			LeadName         string `json:"name"`
			Email            string `json:"email"`
			Phone            string `json:"phone"`
			Source           string `json:"source"`
			AssignedBrokerID string `json:"assigned_broker_id"`
			Status           string `json:"status"`
			Timestamp        string `json:"timestamp"`
			MessageID        string `json:"message_id,omitempty"`
		}
		b, _ := json.Marshal(evt.Payload())
		if err := json.Unmarshal(b, &payload); err != nil {
			return err
		}
		payload.MessageID = evt.EventID()
		if payload.TenantID == "" || payload.LeadID == "" {
			return nil
		}
		if c.msgService != nil && c.msgService.Hub() != nil {
			c.msgService.Hub().BroadcastEventToTenantInternal(payload.TenantID, "lead_created", payload)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// client.document.uploaded -> Notify
	err = c.eventBus.SubscribeWithQueue("comm_document_uploaded", "client.document.uploaded", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			DealID            string `json:"deal_id"`
			FileName          string `json:"file_name"`
			TenantID          string `json:"tenant_id"`
			CustomerAccountID string `json:"customer_account_id"`
		}
		b, _ := json.Marshal(evt.Payload())
		_ = json.Unmarshal(b, &payload)

		channels, _ := c.msgService.repo.ListChannels(ctx, payload.TenantID, "", payload.DealID)
		for _, ch := range channels {
			if ch.Type == message.ChannelTypeDeal {
				for _, p := range ch.Participants {
					if p != payload.CustomerAccountID {
						data := map[string]interface{}{"tenant_id": payload.TenantID, "deal_id": payload.DealID}
						dedupID := events.DeriveNotificationID(evt.EventID(), p, "document_uploaded", payload.FileName)
						_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.TenantID, p, notification.InAppTypeSystem, notification.PriorityNormal, "Documento Enviado", fmt.Sprintf("Documento enviado: %s", payload.FileName), fmt.Sprintf("/esteira/deals/%s", payload.DealID), data)
					}
				}
			}
		}

		return nil
	})

	if err != nil {
		return err
	}

	err = c.eventBus.SubscribeWithQueue("comm_lead_assigned", "lead.assigned", func(ctx context.Context, evt events.Event) error {
		var payload leadAssignedPayload
		b, _ := json.Marshal(evt.Payload())
		if err := json.Unmarshal(b, &payload); err != nil {
			return err
		}
		payload.MessageID = evt.EventID()
		if payload.TenantID == "" || payload.BrokerID == "" || payload.LeadID == "" {
			return nil
		}

		data := map[string]interface{}{"tenant_id": payload.TenantID, "lead_id": payload.LeadID, "stage": payload.Stage, "method": payload.Method, "tags": payload.Tags}
		// Idempotency: the in-app insert is the gate for downstream side
		// effects (broadcast + external dispatches). If the dedup id already
		// exists in comm.inapp_notifications, this assignment has already
		// been processed for this broker — skip Hub broadcast and external
		// channels to avoid duplicate WhatsApp/SMS/email messages on a
		// redelivered event.
		created := true
		if c.inAppService != nil {
			dedupID := events.DeriveNotificationID(evt.EventID(), payload.BrokerID, "lead_assigned", payload.LeadID)
			// InAppTypeLeadAssigned (não InAppTypeSystem): o NotificationBell
			// no frontend faz tratamento específico para esse type — ícone
			// CircleAlert + toast com CTA "Ver Lead" + som de prioridade.
			// Com type=System a notificação aparecia genérica.
			ok, err := c.inAppService.CreateIdempotent(ctx, dedupID, payload.TenantID, payload.BrokerID, notification.InAppTypeLeadAssigned, notification.PriorityImportant, "Novo Lead Atribuído", fmt.Sprintf("Lead %s", payload.LeadID), fmt.Sprintf("/leads/%s", payload.LeadID), data)
			if err != nil {
				return err
			}
			created = ok
		}
		if !created {
			return nil
		}
		if c.msgService != nil && c.msgService.Hub() != nil {
			// Broadcast tenant-wide so admins/managers viewing the live Monitor
			// receive the event, not only the assigned broker.
			c.msgService.Hub().BroadcastEventToTenantInternal(payload.TenantID, "lead_assigned", payload)
		}

		for _, ch := range payload.Channels {
			key := strings.ToLower(strings.TrimSpace(ch))
			switch key {
			case "whatsapp":
				c.dispatchExternalIdempotent(ctx, evt.EventID(), "whatsapp", payload.BrokerID, payload.TenantID, func() error {
					return c.externalDispatcher.DispatchWhatsApp(ctx, payload.TenantID, payload.BrokerID, "Novo Lead Atribuído", fmt.Sprintf("Lead %s", payload.LeadID))
				})
			case "sms":
				c.dispatchExternalIdempotent(ctx, evt.EventID(), "sms", payload.BrokerID, payload.TenantID, func() error {
					return c.externalDispatcher.DispatchSMS(ctx, payload.TenantID, payload.BrokerID, fmt.Sprintf("Novo lead: %s", payload.LeadID))
				})
			case "e-mail", "email":
				// Braço de e-mail (beta): marco crítico — ignora presença e
				// cooldown (Force), mas mantém idempotência por evento.
				if c.emailNotifier != nil {
					c.notifyBrokerEmail(ctx, evt.EventID(), payload.TenantID, payload.BrokerID,
						CategoryLeadAssigned,
						leadAssignedSubject(payload.LeadName),
						leadAssignedBody(payload),
						"Ver lead",
						fmt.Sprintf("/leads/%s", payload.LeadID),
						true)
				} else {
					// Fallback legado: dispatcher externo (noop por padrão).
					c.dispatchExternalIdempotent(ctx, evt.EventID(), "email", payload.BrokerID, payload.TenantID, func() error {
						return c.externalDispatcher.DispatchEmail(ctx, payload.TenantID, payload.BrokerID, "Novo Lead Atribuído", fmt.Sprintf("<p>Novo lead: %s</p>", payload.LeadID))
					})
				}
			}
		}

		return nil
	})

	if err != nil {
		return err
	}

	// deal.contract.signed -> Notify clients (esteira digital advancing into completion)
	err = c.eventBus.SubscribeWithQueue("comm_deal_contract_signed", "deal.contract.signed", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			DealID             string   `json:"deal_id"`
			ContractID         string   `json:"contract_id"`
			TenantID           string   `json:"tenant_id"`
			CustomerAccountIDs []string `json:"customer_account_ids"`
			Parties            []struct {
				AccountID string `json:"account_id"`
				Email     string `json:"email"`
				Role      string `json:"role"`
			} `json:"parties"`
		}
		b, _ := json.Marshal(evt.Payload())
		if err := json.Unmarshal(b, &payload); err != nil {
			return err
		}
		if c.msgService != nil && c.msgService.Hub() != nil && len(payload.CustomerAccountIDs) > 0 {
			c.msgService.Hub().BroadcastEvent(payload.TenantID, payload.CustomerAccountIDs, "deal_contract_signed", payload)
		}
		// Operators always get the contract-signed event so the Esteira
		// "Assinatura" tab badge updates without waiting for a refetch.
		if c.msgService != nil && c.msgService.Hub() != nil && payload.TenantID != "" {
			c.msgService.Hub().BroadcastEventToTenantInternal(payload.TenantID, "deal_contract_signed", payload)
		}
		// Issue #452: garante um canal de suporte aberto vinculado ao
		// contrato logo após a assinatura. Idempotente: redeliveries do
		// evento não criam canais duplicados (EnsureChannelForSupport
		// usa GetChannelByReference). Falha aqui é loggada mas NÃO aborta
		// o handler — notificações in-app abaixo são o que o usuário vê,
		// e o canal pode ser recriado via API depois se necessário.
		if c.msgService != nil && payload.TenantID != "" && payload.ContractID != "" && len(payload.CustomerAccountIDs) > 0 {
			if _, err := c.msgService.EnsureChannelForSupport(ctx, payload.TenantID, payload.ContractID, payload.CustomerAccountIDs); err != nil {
				logger.WarnContext(ctx, "ensure support channel failed",
					"error", err.Error(),
					"tenant_id", payload.TenantID,
					"contract_id", payload.ContractID,
				)
			}
		}
		for _, accountID := range payload.CustomerAccountIDs {
			data := map[string]interface{}{"tenant_id": payload.TenantID, "deal_id": payload.DealID, "contract_id": payload.ContractID}
			dedupID := events.DeriveNotificationID(evt.EventID(), accountID, "contract_signed", payload.ContractID)
			_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.TenantID, accountID, notification.InAppTypeSystem, notification.PriorityImportant, "Contrato Assinado", "Todas as assinaturas foram coletadas.", fmt.Sprintf("/portal/deals/%s", payload.DealID), data)

			// Braço de e-mail (beta): marco crítico — Force ignora presença
			// e cooldown para o cliente não perder a conclusão do contrato.
			if c.emailNotifier != nil {
				c.notifyClientEmail(ctx, evt.EventID(), payload.TenantID, accountID,
					CategoryDealUpdates,
					"Contrato assinado",
					"<p style='margin: 0 0 24px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>Todas as assinaturas foram coletadas e o contrato do seu negócio está assinado.</p>",
					"Ver contrato",
					fmt.Sprintf("/portal/deals/%s", payload.DealID),
					true)
			}
		}
		// Partes cliente ainda SEM conta de portal recebem o marco por e-mail
		// direto (mesma regra do deal.status_changed).
		if c.emailNotifier != nil {
			for _, p := range payload.Parties {
				if p.Role != "buyer" && p.Role != "tenant" {
					continue
				}
				if p.AccountID != "" || p.Email == "" {
					continue
				}
				c.notifyEmailDirect(ctx, evt.EventID(), payload.TenantID, p.Email,
					CategoryDealUpdates,
					"Contrato assinado",
					"<p style='margin: 0 0 24px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>Todas as assinaturas foram coletadas e o contrato do seu negócio está assinado.</p>",
					"Ver contrato",
					fmt.Sprintf("/portal/deals/%s", payload.DealID),
					true)
			}
		}
		// Admins da empresa também recebem o marco por e-mail.
		if c.emailNotifier != nil && payload.TenantID != "" {
			for _, admin := range c.resolveCompanyAdmins(ctx, payload.TenantID) {
				if admin.AccountID == "" || admin.Email == "" {
					continue
				}
				_, _ = c.emailNotifier.Notify(ctx, NotifyOptions{
					Category:  CategoryDealUpdates,
					TenantID:  payload.TenantID,
					UserID:    admin.AccountID,
					Recipient: admin.Email,
					Subject:   "Contrato assinado no negócio",
					BodyHTML: c.emailNotifier.BuildEmail(
						"Contrato assinado",
						"<p style='margin: 0 0 24px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>Todas as assinaturas foram coletadas em um negócio da sua empresa.</p>",
						"Ver negócio",
						c.emailNotifier.FrontendLink(fmt.Sprintf("/esteira/deals/%s", payload.DealID)),
						"Você recebe este e-mail como responsável pela empresa na plataforma.",
					),
					EventID:      evt.EventID(),
					SkipIfOnline: true,
					Force:        true,
				})
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// checklist.solicited -> Notify the specific party that a document was requested
	err = c.eventBus.SubscribeWithQueue("comm_checklist_solicited", "checklist.solicited", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			DealID            string `json:"deal_id"`
			TenantID          string `json:"tenant_id"`
			ItemID            string `json:"item_id"`
			ItemName          string `json:"item_name"`
			PartyID           string `json:"party_id"`
			CustomerAccountID string `json:"customer_account_id"`
		}
		b, _ := json.Marshal(evt.Payload())
		if err := json.Unmarshal(b, &payload); err != nil {
			return err
		}
		if c.msgService != nil && c.msgService.Hub() != nil {
			if payload.CustomerAccountID != "" {
				c.msgService.Hub().BroadcastEvent(payload.TenantID, []string{payload.CustomerAccountID}, "checklist_solicited", payload)
			}
			// Operators on the Esteira need to see the document request
			// land in the "Documentos" tab in real time.
			if payload.TenantID != "" {
				c.msgService.Hub().BroadcastEventToTenantInternal(payload.TenantID, "checklist_solicited", payload)
			}
		}
		if payload.CustomerAccountID == "" {
			return nil
		}
		data := map[string]interface{}{"tenant_id": payload.TenantID, "deal_id": payload.DealID, "item_id": payload.ItemID}
		dedupID := events.DeriveNotificationID(evt.EventID(), payload.CustomerAccountID, "checklist_solicited", payload.ItemID)
		_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.TenantID, payload.CustomerAccountID, notification.InAppTypeSystem, notification.PriorityImportant, "Documento solicitado", fmt.Sprintf("Envie: %s", payload.ItemName), fmt.Sprintf("/portal/deals/%s", payload.DealID), data)

		// Braço de e-mail (beta): pedido de documento ao cliente.
		if c.emailNotifier != nil {
			c.notifyClientEmail(ctx, evt.EventID(), payload.TenantID, payload.CustomerAccountID,
				CategoryDealUpdates,
				"Documento solicitado",
				fmt.Sprintf("<p style='margin: 0 0 24px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>Foi solicitado o envio do documento <strong>%s</strong> no seu negócio.</p>", payload.ItemName),
				"Enviar documento",
				fmt.Sprintf("/portal/deals/%s", payload.DealID),
				false)
		}
		return nil
	})
	if err != nil {
		return err
	}

	err = c.eventBus.SubscribeWithQueue("comm_lead_escalated", "lead.escalated", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			LeadID    string `json:"lead_id"`
			TenantID  string `json:"tenant_id"`
			Action    string `json:"action"`
			Level     int    `json:"level"`
			StepLabel string `json:"step_label"`
		}
		b, _ := json.Marshal(evt.Payload())
		if err := json.Unmarshal(b, &payload); err != nil {
			return err
		}
		if payload.TenantID == "" || payload.LeadID == "" {
			return nil
		}

		switch payload.Action {
		case "notify_manager":
			recipients := c.resolveRecipients(ctx, payload.TenantID, "gestor")
			for _, userID := range recipients {
				data := map[string]interface{}{"tenant_id": payload.TenantID, "lead_id": payload.LeadID, "action": payload.Action, "level": payload.Level}
				dedupID := events.DeriveNotificationID(evt.EventID(), userID, "lead_escalated", payload.LeadID, payload.Action)
				_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.TenantID, userID, notification.InAppTypeSystem, notification.PriorityImportant, "SLA Violado — Escalação", payload.StepLabel, fmt.Sprintf("/leads/%s", payload.LeadID), data)
			}
			if c.msgService != nil && c.msgService.Hub() != nil && len(recipients) > 0 {
				c.msgService.Hub().BroadcastEvent(payload.TenantID, recipients, "lead_escalated", payload)
			}
		case "redirect_team":
			return nil
		case "notify_whatsapp":
			// Synthetic recipient for tenant-wide SLA alerts (no specific
			// broker assigned) — uses tenant_id so the dedup key is stable
			// across redeliveries while still allowing per-tenant fan-out.
			c.dispatchExternalIdempotent(ctx, evt.EventID(), "whatsapp", "tenant:"+payload.TenantID, payload.TenantID, func() error {
				return c.externalDispatcher.DispatchWhatsApp(ctx, payload.TenantID, "", "SLA Violado", fmt.Sprintf("Lead %s", payload.LeadID))
			})
		case "auto_assign_supervisor":
			if c.eventBus != nil {
				evt := LeadSupervisorRequiredEvent{LeadID: payload.LeadID, TenantID: payload.TenantID}
				if err := c.eventBus.PublishReliable(ctx, evt); err != nil {
					logger.FromContext(ctx).Warn("failed to enqueue lead.supervisor_required event", "lead_id", payload.LeadID, "error", err)
				}
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	// Issue #427 — lead.first_contact_sla_violated. Cliente acabou de
	// cruzar a janela de SLA do primeiro contato (default 1h). Notifica:
	//  - corretor responsável (in-app + WS) com CTA "Registrar contato"
	//  - gestores do tenant (in-app) pra acompanhamento
	err = c.eventBus.SubscribeWithQueue("comm_lead_first_contact_sla_violated", "lead.first_contact_sla_violated", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			LeadID           string `json:"lead_id"`
			TenantID         string `json:"tenant_id"`
			AssignedBrokerID string `json:"assigned_broker_id"`
			MinutesOverdue   int    `json:"minutes_overdue"`
			SLAMinutes       int    `json:"sla_minutes"`
		}
		b, _ := json.Marshal(evt.Payload())
		if err := json.Unmarshal(b, &payload); err != nil {
			return err
		}
		if payload.TenantID == "" || payload.LeadID == "" {
			return nil
		}

		// Mensagem amigável. Mostra o atraso em minutos quando ainda
		// pequeno; em horas quando passou de 60min (saber que está
		// "5h atrasado" pesa diferente de "300min atrasado").
		title := "SLA primeiro contato violado"
		body := fmt.Sprintf("Lead aguarda primeiro contato há mais de %dmin", payload.SLAMinutes)
		if payload.MinutesOverdue >= 60 {
			body = fmt.Sprintf("Lead aguarda primeiro contato há mais de %dh", (payload.SLAMinutes+payload.MinutesOverdue)/60)
		}
		ctaURL := fmt.Sprintf("/leads/%s", payload.LeadID)
		data := map[string]interface{}{
			"tenant_id":       payload.TenantID,
			"lead_id":         payload.LeadID,
			"sla_minutes":     payload.SLAMinutes,
			"minutes_overdue": payload.MinutesOverdue,
		}

		// Corretor primeiro — ele é quem precisa AGIR.
		if payload.AssignedBrokerID != "" {
			dedupID := events.DeriveNotificationID(evt.EventID(), payload.AssignedBrokerID, "first_contact_sla", payload.LeadID)
			_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.TenantID, payload.AssignedBrokerID, notification.InAppTypeSystem, notification.PriorityImportant, title, body, ctaURL, data)
			if c.msgService != nil && c.msgService.Hub() != nil {
				c.msgService.Hub().BroadcastEvent(payload.TenantID, []string{payload.AssignedBrokerID}, "lead_first_contact_sla_violated", payload)
			}
		}

		// Gestores — visibilidade pra cobrar o time.
		managers := c.resolveRecipients(ctx, payload.TenantID, "gestor")
		for _, userID := range managers {
			if userID == payload.AssignedBrokerID {
				continue // já notificado acima
			}
			dedupID := events.DeriveNotificationID(evt.EventID(), userID, "first_contact_sla", payload.LeadID)
			_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.TenantID, userID, notification.InAppTypeSystem, notification.PriorityNormal, title, body, ctaURL, data)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Phase 3.3 — rental.created → in-app notification for both leads of the
	// rental. Tenant gets "Locação iniciada"; owner gets "Imóvel locado".
	err = c.eventBus.SubscribeWithQueue("comm_rental_created", "rental.created", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			RentalID     string `json:"rental_id"`
			TenantID     string `json:"tenant_id"`
			DealID       string `json:"deal_id"`
			PropertyID   string `json:"property_id"`
			TenantLeadID string `json:"tenant_lead_id"`
			OwnerLeadID  string `json:"owner_lead_id"`
		}
		b, _ := json.Marshal(evt.Payload())
		if err := json.Unmarshal(b, &payload); err != nil {
			return err
		}
		if payload.TenantID == "" || payload.RentalID == "" {
			return nil
		}
		data := map[string]interface{}{"tenant_id": payload.TenantID, "rental_id": payload.RentalID, "deal_id": payload.DealID, "property_id": payload.PropertyID}
		if payload.TenantLeadID != "" {
			dedupID := events.DeriveNotificationID(evt.EventID(), payload.TenantLeadID, "rental_created_tenant", payload.RentalID)
			_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.TenantID, payload.TenantLeadID, notification.InAppTypeSystem, notification.PriorityImportant, "Locação iniciada", "Sua locação foi registrada com sucesso.", fmt.Sprintf("/portal/locacao/%s", payload.RentalID), data)
		}
		if payload.OwnerLeadID != "" {
			dedupID := events.DeriveNotificationID(evt.EventID(), payload.OwnerLeadID, "rental_created_owner", payload.RentalID)
			_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.TenantID, payload.OwnerLeadID, notification.InAppTypeSystem, notification.PriorityImportant, "Imóvel locado", "Seu imóvel foi locado e os repasses estão configurados.", fmt.Sprintf("/portal/imoveis/%s", payload.PropertyID), data)
		}
		if c.msgService != nil && c.msgService.Hub() != nil {
			recipients := []string{}
			if payload.TenantLeadID != "" {
				recipients = append(recipients, payload.TenantLeadID)
			}
			if payload.OwnerLeadID != "" {
				recipients = append(recipients, payload.OwnerLeadID)
			}
			if len(recipients) > 0 {
				c.msgService.Hub().BroadcastEvent(payload.TenantID, recipients, "rental_created", payload)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Phase 3.3 — property.captured → in-app for the owner-lead.
	err = c.eventBus.SubscribeWithQueue("comm_property_captured", "property.captured", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			PropertyID    string `json:"property_id"`
			TenantID      string `json:"tenant_id"`
			OwnerLeadID   string `json:"owner_lead_id"`
			PropertyTitle string `json:"property_title"`
			Status        string `json:"status"`
			ListingType   string `json:"listing_type"`
		}
		b, _ := json.Marshal(evt.Payload())
		if err := json.Unmarshal(b, &payload); err != nil {
			return err
		}
		if payload.TenantID == "" || payload.OwnerLeadID == "" {
			return nil
		}
		title := payload.PropertyTitle
		if title == "" {
			title = "Seu imóvel"
		}
		data := map[string]interface{}{"tenant_id": payload.TenantID, "property_id": payload.PropertyID, "status": payload.Status, "listing_type": payload.ListingType}
		dedupID := events.DeriveNotificationID(evt.EventID(), payload.OwnerLeadID, "property_captured", payload.PropertyID)
		_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.TenantID, payload.OwnerLeadID, notification.InAppTypeSystem, notification.PriorityNormal, "Captação iniciada", fmt.Sprintf("%s foi cadastrado no portfólio.", title), fmt.Sprintf("/portal/imoveis/%s", payload.PropertyID), data)
		if c.msgService != nil && c.msgService.Hub() != nil {
			c.msgService.Hub().BroadcastEvent(payload.TenantID, []string{payload.OwnerLeadID}, "property_captured", payload)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Phase 3.3 — rental.charge_overdue → urgent in-app to tenant + nudge owner.
	err = c.eventBus.SubscribeWithQueue("comm_rental_charge_overdue", "rental.charge_overdue", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			ChargeID      string  `json:"charge_id"`
			RentalID      string  `json:"rental_id"`
			TenantID      string  `json:"tenant_id"`
			TenantLeadID  string  `json:"tenant_lead_id"`
			OwnerLeadID   string  `json:"owner_lead_id"`
			Amount        float64 `json:"amount"`
			DueDate       string  `json:"due_date"`
			DaysOverdue   int     `json:"days_overdue"`
			PropertyTitle string  `json:"property_title"`
		}
		b, _ := json.Marshal(evt.Payload())
		if err := json.Unmarshal(b, &payload); err != nil {
			return err
		}
		if payload.TenantID == "" || payload.ChargeID == "" {
			return nil
		}
		data := map[string]interface{}{"tenant_id": payload.TenantID, "rental_id": payload.RentalID, "charge_id": payload.ChargeID, "amount": payload.Amount, "days_overdue": payload.DaysOverdue}
		if payload.TenantLeadID != "" {
			dedupID := events.DeriveNotificationID(evt.EventID(), payload.TenantLeadID, "rental_charge_overdue_tenant", payload.ChargeID)
			_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.TenantID, payload.TenantLeadID, notification.InAppTypeSystem, notification.PriorityImportant, "Cobrança em atraso", fmt.Sprintf("%s vencida há %d dia(s).", money.BRL(payload.Amount), payload.DaysOverdue), fmt.Sprintf("/portal/locacao/%s", payload.RentalID), data)
		}
		if payload.OwnerLeadID != "" {
			dedupID := events.DeriveNotificationID(evt.EventID(), payload.OwnerLeadID, "rental_charge_overdue_owner", payload.ChargeID)
			_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.TenantID, payload.OwnerLeadID, notification.InAppTypeSystem, notification.PriorityNormal, "Inquilino em atraso", fmt.Sprintf("Cobrança de %s está atrasada há %d dia(s).", money.BRL(payload.Amount), payload.DaysOverdue), fmt.Sprintf("/portal/imoveis/%s", payload.RentalID), data)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// F0.4 do plano de inadimplência — charge.overdue → aviso imediato ao
	// devedor.
	//
	// O evento era publicado pelo motor do financeiro e NÃO tinha nenhum
	// consumidor no monorepo (a régua externa simplesmente não existia; só
	// `rental.charge_overdue` tinha assinante, e esse não tinha publisher).
	// Aqui entra o consumidor que fecha esse lado: no momento em que a cobrança
	// vira vencida, o devedor recebe in-app. A régua agendada do financeiro
	// (e-mail/in-app/WhatsApp por regra do tenant) continua sendo o canal
	// recorrente — este é o aviso do primeiro dia.
	//
	// Dedup: a chave deriva de (event id, charge) e o CreateIdempotent ignora
	// reentrega, então retry do RabbitMQ não duplica o aviso.
	err = c.eventBus.SubscribeWithQueue("comm_charge_overdue", "charge.overdue", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			ChargeID    string  `json:"charge_id"`
			ContractID  string  `json:"contract_id"`
			TenantID    string  `json:"tenant_id"`
			DebtorID    string  `json:"debtor_id"`
			Amount      float64 `json:"amount"`
			TotalAmount float64 `json:"total_amount"`
			DaysOverdue int     `json:"days_overdue"`
		}
		b, _ := json.Marshal(evt.Payload())
		if err := json.Unmarshal(b, &payload); err != nil {
			return err
		}
		if payload.TenantID == "" || payload.ChargeID == "" || payload.DebtorID == "" {
			return nil
		}
		valor := payload.TotalAmount
		if valor <= 0 {
			valor = payload.Amount
		}
		dedupID := events.DeriveNotificationID(evt.EventID(), payload.DebtorID, "charge_overdue_debtor", payload.ChargeID)
		_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.TenantID, payload.DebtorID,
			notification.InAppTypeSystem, notification.PriorityImportant,
			"Cobrança em atraso",
			fmt.Sprintf("%s vencida há %d dia(s). Regularize para evitar encargos.", money.BRL(valor), payload.DaysOverdue),
			"/portal/financeiro",
			map[string]interface{}{"tenant_id": payload.TenantID, "charge_id": payload.ChargeID, "contract_id": payload.ContractID, "amount": valor, "days_overdue": payload.DaysOverdue})
		return nil
	})
	if err != nil {
		return err
	}

	// Each committed agreement change refreshes partner and shared-content views.
	err = c.eventBus.SubscribeWithQueue("comm_partnership_changed", "partnership.changed", func(ctx context.Context, evt events.Event) error {
		var p struct {
			RequesterID string `json:"requester_id"`
			TargetID    string `json:"target_id"`
		}
		b, err := json.Marshal(evt.Payload())
		if err != nil {
			return err
		}
		if err := json.Unmarshal(b, &p); err != nil {
			return err
		}
		if c.msgService != nil && c.msgService.Hub() != nil {
			for _, tenant := range []string{p.RequesterID, p.TargetID} {
				if tenant != "" {
					c.msgService.Hub().BroadcastEventToTenantInternal(tenant, "partnership_changed", evt.Payload())
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// partnership.invited — a company invited another company into a
	// partnership/franchise. Notify the TARGET company's admins (in-app +
	// email) so they can accept or reject. Idempotent: the in-app insert is
	// gated by a deterministic dedup id and the email goes through
	// dispatchExternalIdempotent so RabbitMQ redeliveries don't fan out
	// duplicate messages.
	err = c.eventBus.SubscribeWithQueue("comm_partnership_invited", "partnership.invited", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			PartnershipID string `json:"partnership_id"`
			RequesterID   string `json:"requester_id"`
			TargetID      string `json:"target_id"`
			Type          string `json:"type"`
			CreatedAt     string `json:"created_at"`
		}
		b, _ := json.Marshal(evt.Payload())
		if err := json.Unmarshal(b, &payload); err != nil {
			return err
		}
		if payload.PartnershipID == "" || payload.TargetID == "" {
			return nil
		}

		noun := partnershipTypeNoun(payload.Type)
		title := fmt.Sprintf("Novo convite de %s", noun)
		content := fmt.Sprintf("Você recebeu um convite de %s. Acesse para aceitar ou recusar.", noun)
		link := "/parceiros?acordo=" + payload.PartnershipID + "&empresa=" + payload.TargetID
		data := map[string]interface{}{
			"tenant_id":      payload.TargetID,
			"partnership_id": payload.PartnershipID,
			"requester_id":   payload.RequesterID,
			"type":           payload.Type,
		}

		// The target company is itself the tenant for the in-app
		// notification; the recipient user is each of its admins.
		admins := c.resolveCompanyAdmins(ctx, payload.TargetID)
		for _, admin := range admins {
			dedupID := events.DeriveNotificationID(evt.EventID(), admin.AccountID, "partnership_invited", payload.PartnershipID)
			_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.TargetID, admin.AccountID, notification.InAppTypeSystem, notification.PriorityImportant, title, content, link, data)

			if c.emailService != nil && admin.Email != "" {
				emailTo := admin.Email
				html := c.emailService.buildEmailHTML(
					title,
					fmt.Sprintf("<p style='margin: 0 0 24px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>Sua empresa recebeu um convite de %s. Acesse a plataforma para revisar os detalhes e aceitar ou recusar o convite.</p>", noun),
					"Ver convite",
					c.emailService.frontendLink(link),
					"Se você não esperava este convite, pode ignorar este e-mail com segurança.",
				)
				c.dispatchExternalIdempotent(ctx, evt.EventID(), "email", emailTo, payload.TargetID, func() error {
					return c.emailService.SendRawEmail(ctx, emailTo, title, html)
				})
			}
		}

		// Real-time nudge so an admin already in the app sees the invite
		// without a refresh.
		if c.msgService != nil && c.msgService.Hub() != nil {
			c.msgService.Hub().BroadcastEventToTenantInternal(payload.TargetID, "partnership_invited", payload)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// partnership.accepted — the target company accepted the invite (status
	// active). Notify the REQUESTER company's admins (in-app + email).
	err = c.eventBus.SubscribeWithQueue("comm_partnership_accepted", "partnership.accepted", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			PartnershipID string `json:"partnership_id"`
			RequesterID   string `json:"requester_id"`
			TargetID      string `json:"target_id"`
			Type          string `json:"type"`
			AcceptedAt    string `json:"accepted_at"`
		}
		b, _ := json.Marshal(evt.Payload())
		if err := json.Unmarshal(b, &payload); err != nil {
			return err
		}
		if payload.PartnershipID == "" || payload.RequesterID == "" {
			return nil
		}

		noun := partnershipTypeNoun(payload.Type)
		title := fmt.Sprintf("Convite de %s aceito", noun)
		content := fmt.Sprintf("Seu convite de %s foi aceito.", noun)
		link := "/parceiros?acordo=" + payload.PartnershipID + "&empresa=" + payload.RequesterID
		data := map[string]interface{}{
			"tenant_id":      payload.RequesterID,
			"partnership_id": payload.PartnershipID,
			"target_id":      payload.TargetID,
			"type":           payload.Type,
		}

		admins := c.resolveCompanyAdmins(ctx, payload.RequesterID)
		for _, admin := range admins {
			dedupID := events.DeriveNotificationID(evt.EventID(), admin.AccountID, "partnership_accepted", payload.PartnershipID)
			_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.RequesterID, admin.AccountID, notification.InAppTypeSystem, notification.PriorityImportant, title, content, link, data)

			if c.emailService != nil && admin.Email != "" {
				emailTo := admin.Email
				html := c.emailService.buildEmailHTML(
					title,
					fmt.Sprintf("<p style='margin: 0 0 24px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>Boas notícias! O convite de %s enviado pela sua empresa foi aceito. A parceria já está ativa na plataforma.</p>", noun),
					"Ver parceria",
					c.emailService.frontendLink(link),
					"Você está recebendo este e-mail porque sua empresa enviou um convite de parceria.",
				)
				c.dispatchExternalIdempotent(ctx, evt.EventID(), "email", emailTo, payload.RequesterID, func() error {
					return c.emailService.SendRawEmail(ctx, emailTo, title, html)
				})
			}
		}

		if c.msgService != nil && c.msgService.Hub() != nil {
			c.msgService.Hub().BroadcastEventToTenantInternal(payload.RequesterID, "partnership_accepted", payload)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// partner.deal_initiated — a partner company started a co-brokering deal
	// against a property/lead this company SHARED. Notify the resource OWNER
	// company's admins (in-app + email) so they know a partner is now
	// operating their shared content. Mirrors the comm_partnership_invited
	// consumer: idempotent in-app insert + dedup'd external dispatch.
	err = c.eventBus.SubscribeWithQueue("comm_partner_deal_initiated", "partner.deal_initiated", func(ctx context.Context, evt events.Event) error {
		var payload struct {
			DealID            string `json:"deal_id"`
			ResourceType      string `json:"resource_type"`
			ResourceID        string `json:"resource_id"`
			InitiatorTenantID string `json:"initiator_tenant_id"`
			OwnerTenantID     string `json:"owner_tenant_id"`
			CreatedAt         string `json:"created_at"`
		}
		b, _ := json.Marshal(evt.Payload())
		if err := json.Unmarshal(b, &payload); err != nil {
			return err
		}
		if payload.DealID == "" || payload.OwnerTenantID == "" {
			return nil
		}

		resourceNoun := "imóvel compartilhado"
		if payload.ResourceType == "lead" {
			resourceNoun = "lead compartilhado"
		}
		title := "Parceiro iniciou um negócio"
		content := fmt.Sprintf("Um parceiro iniciou um negócio com seu %s.", resourceNoun)
		link := "/parceiros"
		data := map[string]interface{}{
			"tenant_id":           payload.OwnerTenantID,
			"deal_id":             payload.DealID,
			"resource_type":       payload.ResourceType,
			"resource_id":         payload.ResourceID,
			"initiator_tenant_id": payload.InitiatorTenantID,
		}

		// The resource owner company is the tenant for the in-app
		// notification; the recipients are its admins.
		admins := c.resolveCompanyAdmins(ctx, payload.OwnerTenantID)
		for _, admin := range admins {
			dedupID := events.DeriveNotificationID(evt.EventID(), admin.AccountID, "partner_deal_initiated", payload.DealID)
			_, _ = c.inAppService.CreateIdempotent(ctx, dedupID, payload.OwnerTenantID, admin.AccountID, notification.InAppTypeSystem, notification.PriorityImportant, title, content, link, data)

			if c.emailService != nil && admin.Email != "" {
				emailTo := admin.Email
				html := c.emailService.buildEmailHTML(
					title,
					fmt.Sprintf("<p style='margin: 0 0 24px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>Uma empresa parceira iniciou um negócio com seu %s. Acesse a plataforma para acompanhar.</p>", resourceNoun),
					"Ver parcerias",
					c.emailService.frontendLink(link),
					"Você está recebendo este e-mail porque sua empresa compartilha conteúdo com parceiros.",
				)
				c.dispatchExternalIdempotent(ctx, evt.EventID(), "email", emailTo, payload.OwnerTenantID, func() error {
					return c.emailService.SendRawEmail(ctx, emailTo, title, html)
				})
			}
		}

		// Real-time nudge so an admin already in the app sees it without a refresh.
		if c.msgService != nil && c.msgService.Hub() != nil {
			c.msgService.Hub().BroadcastEventToTenantInternal(payload.OwnerTenantID, "partner_deal_initiated", payload)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// signature.* fan-out — every transition from toca-signature gets
	// broadcast tenant-wide as a generic "signature_update". Both admin
	// (ContractTab) and portal (ClientSignaturePanel) listen and invalidate
	// their queries; the events themselves carry envelope_id/contract_id
	// so the frontend can filter to "is this for me?".
	for queue, topic := range map[string]string{
		"comm_signature_envelope_started":  "signature.envelope_started",
		"comm_signature_signer_signed":     "signature.signer_signed",
		"comm_signature_envelope_finished": "signature.envelope_finished",
		"comm_signature_signer_viewed":     "signature.signer_viewed",
		"comm_signature_refused":           "signature.refused",
		"comm_signature_expired":           "signature.expired",
	} {
		topic := topic // capture
		err = c.eventBus.SubscribeWithQueue(queue, topic, func(ctx context.Context, evt events.Event) error {
			var payload map[string]interface{}
			b, _ := json.Marshal(evt.Payload())
			if err := json.Unmarshal(b, &payload); err != nil {
				return err
			}
			tenantID, _ := payload["tenant_id"].(string)
			if tenantID == "" {
				return nil
			}
			// Echo the source event name in the payload so the frontend
			// can react differently (e.g. show a toast on signer_signed
			// but only invalidate on viewed).
			payload["event"] = topic
			if c.msgService != nil && c.msgService.Hub() != nil {
				c.msgService.Hub().BroadcastEventToTenant(tenantID, "signature_update", payload)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	return err
}

type LeadSupervisorRequiredEvent struct {
	LeadID   string `json:"lead_id"`
	TenantID string `json:"tenant_id"`
}

func (e LeadSupervisorRequiredEvent) EventID() string      { return e.LeadID }
func (e LeadSupervisorRequiredEvent) Name() string         { return "lead.supervisor_required" }
func (e LeadSupervisorRequiredEvent) Payload() interface{} { return e }

func appointmentPortalLink(dealID string) string {
	if dealID == "" {
		return "/portal"
	}
	return fmt.Sprintf("/portal/deals/%s", dealID)
}
