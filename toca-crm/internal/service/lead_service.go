package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/clients"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/domain"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/metrics"
	pkgvalidation "github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-crm/internal/domain/broker"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
	"github.com/oginoino/toca/toca-crm/internal/repository"
)

// LeadCreatedEvent is published when a new lead is created
type LeadCreatedEvent struct {
	LeadID           string    `json:"lead_id"`
	TenantID         string    `json:"tenant_id"`
	LeadName         string    `json:"name"`
	Email            string    `json:"email"`
	Phone            string    `json:"phone"`
	Source           string    `json:"source"`
	AssignedBrokerID string    `json:"assigned_broker_id"`
	Status           string    `json:"status"`
	Timestamp        time.Time `json:"timestamp"`
}

func (e LeadCreatedEvent) EventID() string {
	return fmt.Sprintf("%s-%s-%d", e.Name(), e.LeadID, time.Now().UnixNano())
}
func (e LeadCreatedEvent) Name() string         { return "lead.created" }
func (e LeadCreatedEvent) Payload() interface{} { return e }

// LeadStatusChangedEvent is published when a lead's status changes
type LeadStatusChangedEvent struct {
	LeadID            string    `json:"lead_id"`
	TenantID          string    `json:"tenant_id"`
	CustomerAccountID string    `json:"customer_account_id"`
	OldStatus         string    `json:"old_status"`
	NewStatus         string    `json:"new_status"`
	Timestamp         time.Time `json:"timestamp"`
}

func (e LeadStatusChangedEvent) EventID() string {
	return fmt.Sprintf("%s-%s-%d", e.Name(), e.LeadID, time.Now().UnixNano())
}
func (e LeadStatusChangedEvent) Name() string         { return "lead.status_changed" }
func (e LeadStatusChangedEvent) Payload() interface{} { return e }

type LeadService struct {
	repo         lead.Repository
	rouletteRepo lead.RouletteRepository
	historyRepo  lead.RouletteHistoryRepository
	brokerRepo   broker.Repository
	eventBus     events.EventBus
	db           *database.PostgresDB // NOVO

	// propertyClient é wirado via WithPropertyClient — opcional, usado
	// apenas pelo MatchProperties (#415). Quando nil, o handler de
	// matched-properties responde "lookup client not configured" e a
	// rota continua respondendo 500 com mensagem clara.
	propertyClient propertyLookup
	calendarClient visitCalendar
	commClient     *clients.CommClient

	// crossRepo reads core.partnerships to resolve which partner companies
	// the caller shares leads with. Wired via WithCrossRepo — optional; when
	// nil, SharedLeadsForCompany returns an error instead of panicking.
	crossRepo crossPartnershipLookup

	bootstrapping map[string]bool // tenantID -> running
}

// crossPartnershipLookup is the subset of repository.CrossTenantRepository the
// lead service needs. Declared as an interface here so tests can inject a fake
// without importing the repository package.
type crossPartnershipLookup interface {
	ActivePartnershipsForCompany(ctx context.Context, companyID string) ([]*repository.PartnershipSharing, error)
	PropertySnapshot(ctx context.Context, tenantID, propertyID string) (*repository.PropertySnapshot, error)
	AvailableProperties(ctx context.Context, tenantIDs []string, listingType string, limit int) ([]*repository.AvailableProperty, error)
}

// WithCrossRepo wires the cross-schema partnership lookup used by
// SharedLeadsForCompany. Kept as a setter to mirror WithPropertyClient — no
// init-time cycle and a stub can be injected for tests.
func (s *LeadService) WithCrossRepo(c crossPartnershipLookup) *LeadService {
	s.crossRepo = c
	return s
}

// WithPropertyClient wires the CoreClient-backed property lookup used
// by MatchProperties. Kept as a setter to mirror SetCommClient pattern
// in financial — no init-time cycle and a stub can be injected for tests.
func (s *LeadService) WithPropertyClient(c propertyLookup) *LeadService {
	s.propertyClient = c
	return s
}

// WithCalendarClient wires the calendar service client used to sync the
// lead visit as a real event in the broker's agenda.
func (s *LeadService) WithCalendarClient(c visitCalendar) *LeadService {
	s.calendarClient = c
	return s
}

// WithCommClient wires the messaging client used to open a direct chat
// channel between the client and the broker.
func (s *LeadService) WithCommClient(c *clients.CommClient) *LeadService {
	s.commClient = c
	return s
}

// EnsureDirectChannel abre (acha ou cria) um canal direto de chat entre o
// cliente e o corretor do lead. Retorna o id do canal.
func (s *LeadService) EnsureDirectChannel(ctx context.Context, leadID string) (string, error) {
	// A client role or tenant membership does not authorize access to every
	// lead. Share the ownership check used by portal visits before contacting comm.
	l, err := s.visitLead(ctx, leadID, true)
	if err != nil {
		return "", err
	}
	if s.commClient == nil {
		return "", fmt.Errorf("messaging not configured")
	}
	if l.AssignedBrokerID == "" {
		return "", fmt.Errorf("lead has no assigned broker")
	}
	// O cliente pode ter mais de uma identidade de conta (conta OTP/sub e a
	// conta do invite/portal). Incluímos AMBAS como participantes (dedupe) para
	// o canal funcionar independentemente de qual conta o portal resolve
	// (lista de canais / envio / recebimento).
	clientAccountID, _ := pkgcontext.GetAccountID(ctx)
	participants := []string{l.AssignedBrokerID}
	seen := map[string]bool{l.AssignedBrokerID: true}
	add := func(id string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		participants = append(participants, id)
	}
	add(clientAccountID)
	add(l.AccountID)
	if len(participants) == 1 {
		return "", fmt.Errorf("lead has no client account")
	}
	tid, _ := domain.NewTenantID(l.TenantID)
	ch, err := s.commClient.EnsureDirectChannel(ctx, clients.EnsureChannelRequest{
		TenantID:     tid,
		DealID:       "",
		Name:         fmt.Sprintf("Conversa com %s", l.Name),
		Participants: participants,
	})
	if err != nil {
		return "", err
	}
	if ch == nil {
		return "", fmt.Errorf("channel not created")
	}
	return ch.ID, nil
}

func (s *LeadService) publishAsync(_ context.Context, evt events.Event) {
	if s.eventBus == nil || evt == nil {
		return
	}
	// Usa um contexto de fundo (não o da request) — o handler HTTP retorna
	// imediatamente e cancela o ctx da request, o que fazia o publish falhar
	// com "context canceled" (o evento nunca chegava ao bus → sem e-mail).
	go func() {
		backgroundCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.eventBus.PublishReliable(backgroundCtx, evt); err != nil {
			log.Printf("WARN: failed to enqueue %s event: %v", evt.Name(), err)
		}
	}()
}

func NewLeadService(
	repo lead.Repository,
	rouletteRepo lead.RouletteRepository,
	historyRepo lead.RouletteHistoryRepository,
	brokerRepo broker.Repository,
	eventBus events.EventBus,
	db *database.PostgresDB,
) *LeadService {
	return &LeadService{
		repo:          repo,
		rouletteRepo:  rouletteRepo,
		historyRepo:   historyRepo,
		brokerRepo:    brokerRepo,
		eventBus:      eventBus,
		db:            db,
		bootstrapping: map[string]bool{},
	}
}

func clientRoleFromPartyRole(role string, dealType string) (lead.ClientRole, bool) {
	switch role {
	case "owner":
		return lead.ClientRoleOwner, true
	case "buyer":
		return lead.ClientRoleBuyer, true
	case "tenant":
		return lead.ClientRoleTenant, true
	case "guarantor":
		return lead.ClientRoleGuarantor, true
	}
	if dealType == "SALE" || dealType == "sale" {
		return lead.ClientRoleBuyer, true
	}
	if dealType == "RENT" || dealType == "rent" {
		return lead.ClientRoleTenant, true
	}
	return "", false
}

// HandlePropertyCapturedConvert é o subscriber de property.captured (#409).
// Quando toca-core registra captação de imóvel com owner_lead_id, promove
// o lead automaticamente para cliente com role=owner. Sem isso, o gestor
// tinha que abrir o lead manualmente e clicar "Converter para Cliente" —
// duplicava trabalho e deixava captações sem cliente vinculado quando
// esquecido.
//
// Idempotente: se o lead já é cliente (IsClient=true) ou já está com
// status CONVERTED, a função não-op (apenas log). Skip também quando
// owner_lead_id vazio (auto-captação por broker sem proprietário ainda
// cadastrado — caso legítimo do BDD J01).
func (s *LeadService) HandlePropertyCapturedConvert(ctx context.Context, event events.Event) error {
	payload := event.Payload()

	var p contracts.PropertyCapturedEventPayload
	switch v := payload.(type) {
	case contracts.PropertyCapturedEventPayload:
		p = v
	case map[string]interface{}:
		b, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		_ = json.Unmarshal(b, &p)
	default:
		b, err := json.Marshal(payload)
		if err != nil {
			return nil
		}
		_ = json.Unmarshal(b, &p)
	}

	if p.OwnerLeadID == "" || p.PropertyID == "" || p.TenantID == "" {
		// Sem owner identificado: caso da captação direta por broker.
		// Frontend pode cadastrar o owner depois e disparar o convert
		// manualmente via /leads/:id/convert.
		return nil
	}

	l, err := s.repo.GetByID(ctx, p.TenantID, p.OwnerLeadID)
	if err != nil || l == nil {
		// Lead pode não existir ainda (race com o cadastro) ou ter sido
		// deletado. Não falha o handler — RabbitMQ acabaria re-entregando
		// indefinidamente.
		log.Printf("WARN: HandlePropertyCapturedConvert: owner lead %s not found for property %s: %v",
			p.OwnerLeadID, p.PropertyID, err)
		return nil
	}

	// Already client? Pode ter sido convertido manualmente ou por outro
	// path (contract.signed). Idempotência: pula sem erro.
	if l.IsClient || l.Status == lead.LeadStatusConverted {
		return nil
	}

	// Lead pode ter sido criado sem tenant_id (caso legacy do J01) — usa
	// o tenant do evento para garantir consistência.
	if l.TenantID == "" && p.TenantID != "" {
		l.TenantID = p.TenantID
	}

	// Persistir a vinculação captação→imóvel no lead para que o histórico
	// futuro saiba qual property motivou a conversão.
	l.CaptacaoPropertyID = p.PropertyID
	l.ConvertToClient(lead.ClientRoleOwner, "")

	if err := s.Update(ctx, l); err != nil {
		// Erro de update propaga para o broker retentar — distinct do
		// "lead não existe" acima que é terminal.
		return fmt.Errorf("HandlePropertyCapturedConvert: update lead %s: %w", l.ID, err)
	}

	audit.Record(ctx, audit.Entry{
		Action:     "lead.converted_to_client",
		EntityType: "lead",
		EntityID:   l.ID,
		After: map[string]any{
			"status":      string(l.Status),
			"is_client":   true,
			"role":        string(lead.ClientRoleOwner),
			"property_id": p.PropertyID,
		},
		Metadata: map[string]any{
			"source":            "property.captured",
			"capturing_brokers": p.CapturingBrokerIDs,
		},
	})

	return nil
}

func (s *LeadService) HandleDealContractSignedConvert(ctx context.Context, event events.Event) error {
	payload := event.Payload()

	var p contracts.ContractSignedEventPayload
	switch v := payload.(type) {
	case contracts.ContractSignedEventPayload:
		p = v
	case map[string]interface{}:
		b, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		_ = json.Unmarshal(b, &p)
	default:
		b, err := json.Marshal(payload)
		if err != nil {
			return nil
		}
		_ = json.Unmarshal(b, &p)
	}

	if p.ContractID == "" || p.TenantID == "" {
		return nil
	}

	parties := p.Parties
	if len(parties) == 0 && p.CustomerID != "" {
		parties = []contracts.PartyPayload{{
			LeadID: p.CustomerID,
			Role:   "",
		}}
	}

	for _, party := range parties {
		leadID := party.LeadID
		if leadID == "" {
			continue
		}
		role, ok := clientRoleFromPartyRole(party.Role, p.Type)
		if !ok {
			continue
		}
		l, err := s.repo.GetByID(ctx, p.TenantID, leadID)
		if err != nil || l == nil {
			continue
		}
		if l.TenantID == "" && p.TenantID != "" {
			l.TenantID = p.TenantID
		}
		l.ConvertToClient(role, p.ContractID)
		_ = s.Update(ctx, l)
	}
	return nil
}

func (s *LeadService) Create(ctx context.Context, l *lead.Lead) error {
	if l.Phone != "" {
		normalized, err := pkgvalidation.NormalizeBRPhone(l.Phone)
		if err != nil {
			return fmt.Errorf("invalid phone: %w", err)
		}
		l.Phone = normalized
	}
	assignedChannels := []string{}
	if l.AssignedBrokerID != "" && s.rouletteRepo != nil && l.TenantID != "" {
		cfgCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if cfg, _ := s.rouletteRepo.GetConfig(cfgCtx, l.TenantID); cfg != nil {
			for _, ch := range cfg.NotifChannels {
				if ch.Enabled {
					assignedChannels = append(assignedChannels, ch.Name)
				}
			}
		}
	}
	// recordLeadCreatedAudit é fechado por path do método (outbox vs direto).
	// Mantém o audit fora da TX para não bloquear o commit caso o INSERT em
	// audit.events demore — Record() é fire-and-forget e a falha é loggada.
	recordLeadCreatedAudit := func() {
		audit.Record(ctx, audit.Entry{
			Action:     "lead.created",
			EntityType: "lead",
			EntityID:   l.ID,
			After: audit.Snapshot(l,
				"id", "tenant_id", "name", "email", "phone",
				"status", "source", "role", "assigned_broker_id",
			),
			Metadata: map[string]any{
				"source":   string(l.Source),
				"role":     string(l.Role),
				"assigned": l.AssignedBrokerID != "",
			},
		})
	}
	rb, useOutbox := s.eventBus.(*events.RabbitMQEventBus)
	if useOutbox && s.db != nil {
		if err := s.db.RunInTx(ctx, func(tx *sqlx.Tx) error {
			if l.ID == "" {
				return fmt.Errorf("lead ID is required")
			}
			if l.TenantID == "" {
				return fmt.Errorf("tenant ID is required")
			}

			metadata, _ := json.Marshal(l)
			if err := tx.QueryRowContext(ctx, `INSERT INTO crm.leads (id, tenant_id, metadata) VALUES ($1, $2, $3) RETURNING created_at`, l.ID, l.TenantID, string(metadata)).Scan(&l.CreatedAt); err != nil {
				return err
			}
			l.UpdatedAt = l.CreatedAt

			evt := LeadCreatedEvent{
				LeadID:           l.ID,
				TenantID:         l.TenantID,
				LeadName:         l.Name,
				Email:            l.Email,
				Phone:            l.Phone,
				Source:           string(l.Source),
				AssignedBrokerID: l.AssignedBrokerID,
				Status:           string(l.Status),
				Timestamp:        time.Now(),
			}

			// RLS: events.outbox_events é deny-by-default no pool de aplicação.
			// EnqueueOutboxInTx adia o INSERT para depois do commit via pool de
			// serviço, preservando a atomicidade (rollback descarta a linha).
			if _, err := rb.EnqueueOutboxInTx(ctx, tx, evt); err != nil {
				return err
			}

			if l.AssignedBrokerID != "" && len(assignedChannels) > 0 {
				assignedEvt := LeadAssignedEvent{
					LeadID:   l.ID,
					TenantID: l.TenantID,
					BrokerID: l.AssignedBrokerID,
					Channels: assignedChannels,
				}
				if _, err := rb.EnqueueOutboxInTx(ctx, tx, assignedEvt); err != nil {
					return err
				}
			}

			return nil
		}); err != nil {
			return err
		}
		recordLeadCreatedAudit()
		return nil
	}

	if err := s.repo.Create(ctx, l); err != nil {
		return err
	}

	evt := LeadCreatedEvent{
		LeadID:           l.ID,
		TenantID:         l.TenantID,
		LeadName:         l.Name,
		Email:            l.Email,
		Phone:            l.Phone,
		Source:           string(l.Source),
		AssignedBrokerID: l.AssignedBrokerID,
		Status:           string(l.Status),
		Timestamp:        time.Now(),
	}
	s.publishAsync(ctx, evt)
	recordLeadCreatedAudit()
	return nil
}

// ErrTenantContextRequired — B10 da revisão de permissões de 2026-09-19: o
// contexto não traz tenant. Antes os handlers do CRM usavam o account_id como
// tenant; agora o erro é explícito para que o handler responda 403 (e não o 404
// genérico, que confundia "sem contexto" com "não existe").
var ErrTenantContextRequired = errors.New("tenant context is required")

// tenantFromContext extrai o tenant do chamador do contexto da requisição.
//
// SECURITY (achado C9): falha fechado. Sem tenant não há como provar que a
// linha pertence a quem pediu, e o caminho anterior (busca só por `id`)
// permitia leitura e escrita cross-tenant. Fluxos de worker/evento sem tenant
// no contexto devem passar o tenant explicitamente (payload do evento) em vez
// de usar este helper com um contexto vazio.
func tenantFromContext(ctx context.Context) (string, error) {
	tenantID, err := pkgcontext.GetTenantID(ctx)
	if err != nil || strings.TrimSpace(tenantID) == "" {
		return "", ErrTenantContextRequired
	}
	return tenantID, nil
}

// GetByIDForTenant devolve o lead de um tenant EXPLÍCITO.
//
// SECURITY (achado A3): contrato para rotas internas, onde o tenant vem da
// credencial resolvida por `auth.ResolveTenantForInternalRequest` (chave de
// serviço com allowlist ou JWT de usuário) — nunca cru do query string. Não use
// em rota de operador: ali o tenant é o do token (ver GetByID).
func (s *LeadService) GetByIDForTenant(ctx context.Context, tenantID, id string) (*lead.Lead, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, errors.New("tenant is required")
	}
	// Pool de SERVIÇO: a rota interna não tem claims de JWT e a política de
	// RLS esconderia a linha do pool de aplicação (404 falso). O tenant
	// continua explícito e resolvido da credencial.
	return s.repo.GetByIDForTenantService(ctx, tenantID, id)
}

// GetByID devolve o lead do tenant do chamador.
//
// SECURITY (achado C9): o tenant vem do contexto (JWT) e é repassado ao
// repositório, que filtra por `tenant_id`. Um lead de outro tenant responde
// "não encontrado" — nunca vaza o registro.
func (s *LeadService) GetByID(ctx context.Context, id string) (*lead.Lead, error) {
	tenantID, err := tenantFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, tenantID, id)
}

// RegisterFirstContact — corretor marca o primeiro contato com o lead.
// Idempotente: chamadas subsequentes não sobrescrevem o timestamp.
// Status transita para CONTACTED quando ainda em NEW/PENDING_ACCEPTANCE.
// Issue #427.
//
// Retorna (lead, true) quando registrou pela primeira vez, (lead, false)
// quando já estava registrado (caller usa pra decidir toast feedback).
func (s *LeadService) RegisterFirstContact(ctx context.Context, leadID string) (*lead.Lead, bool, error) {
	tenantID, err := tenantFromContext(ctx)
	if err != nil {
		return nil, false, err
	}
	l, err := s.repo.GetByID(ctx, tenantID, leadID)
	if err != nil {
		return nil, false, err
	}
	if l == nil {
		return nil, false, fmt.Errorf("lead not found")
	}
	now := time.Now()
	changed := l.MarkFirstContact(now)
	if !changed {
		return l, false, nil
	}
	if err := s.repo.Update(ctx, l); err != nil {
		return nil, false, err
	}
	// Publish event para downstream — UI atualiza, métricas registram
	// o lead como "contatado dentro do SLA". Não é fatal se falhar.
	if s.eventBus != nil {
		evt := LeadFirstContactRegisteredEvent{
			LeadID:           l.ID,
			TenantID:         l.TenantID,
			AssignedBrokerID: l.AssignedBrokerID,
			ContactedAt:      now.UTC().Format(time.RFC3339),
		}
		s.publishAsync(ctx, evt)
	}
	return l, true, nil
}

// LeadFirstContactRegisteredEvent — disparado quando o corretor marca
// o primeiro contato. Outros services consomem para zerar contadores
// de SLA, atualizar dashboards etc.
type LeadFirstContactRegisteredEvent struct {
	LeadID           string `json:"lead_id"`
	TenantID         string `json:"tenant_id"`
	AssignedBrokerID string `json:"assigned_broker_id,omitempty"`
	ContactedAt      string `json:"contacted_at"`
}

func (e LeadFirstContactRegisteredEvent) EventID() string      { return e.LeadID + ":fc_registered" }
func (e LeadFirstContactRegisteredEvent) Name() string         { return "lead.first_contact_registered" }
func (e LeadFirstContactRegisteredEvent) Payload() interface{} { return e }

func (s *LeadService) GetByEmail(ctx context.Context, tenantID, email string) (*lead.Lead, error) {
	return s.repo.GetByEmail(ctx, tenantID, email)
}

func (s *LeadService) GetByPhone(ctx context.Context, tenantID, phone string) (*lead.Lead, error) {
	return s.repo.GetByPhone(ctx, tenantID, phone)
}

func (s *LeadService) GetByDisplayID(ctx context.Context, tenantID, displayID string) (*lead.Lead, error) {
	return s.repo.GetByDisplayID(ctx, tenantID, displayID)
}

func (s *LeadService) UpdatePreservingAssignment(ctx context.Context, l *lead.Lead, preserveAssignedBroker bool) error {
	if !preserveAssignedBroker || l == nil || l.ID == "" {
		return s.Update(ctx, l)
	}

	current, err := s.repo.GetByID(ctx, l.TenantID, l.ID)
	if err == nil && current != nil {
		if l.AssignedBrokerID == "" && current.AssignedBrokerID != "" {
			l.AssignedBrokerID = current.AssignedBrokerID
		}
	}

	return s.Update(ctx, l)
}

func (s *LeadService) Patch(ctx context.Context, leadID, tenantID string, expected *lead.LeadExpectedVersion, updates map[string]interface{}) (*lead.Lead, bool, error) {
	if raw, present := updates["Name"]; present {
		// Edição também passa pela regra do nome completo: sem isso o operador
		// podia salvar um lead monônimo e só descobrir o problema ao iniciar o
		// negócio na esteira.
		if rawStr, isStr := raw.(string); isStr {
			clean := pkgvalidation.SanitizePersonName(rawStr)
			if err := pkgvalidation.ValidateFullPersonName(clean, "Nome"); err != nil {
				return nil, false, err
			}
			updates["Name"] = clean
		}
	}
	if raw, present := updates["Phone"]; present {
		if rawStr, isStr := raw.(string); isStr && rawStr != "" {
			normalized, err := pkgvalidation.NormalizeBRPhone(rawStr)
			if err != nil {
				return nil, false, fmt.Errorf("invalid phone: %w", err)
			}
			updates["Phone"] = normalized
		}
	}
	existing, _ := s.repo.GetByID(ctx, tenantID, leadID)
	updated, ok, err := s.repo.Patch(ctx, leadID, tenantID, expected, updates)
	if err != nil || !ok || updated == nil {
		return updated, ok, err
	}

	if s.eventBus != nil {
		if existing != nil {
			if existing.AssignedBrokerID != updated.AssignedBrokerID && updated.AssignedBrokerID != "" {
				channels := []string{}
				if cfg, _ := s.rouletteRepo.GetConfig(ctx, tenantID); cfg != nil {
					for _, ch := range cfg.NotifChannels {
						if ch.Enabled {
							channels = append(channels, ch.Name)
						}
					}
				}
				s.publishAsync(ctx, LeadAssignedEvent{
					LeadID:   leadID,
					TenantID: tenantID,
					BrokerID: updated.AssignedBrokerID,
					Channels: channels,
				})
			}

			if existing.Status != updated.Status {
				metrics.IncLeadStatusChanged(string(existing.Status), string(updated.Status))
				s.publishAsync(ctx, LeadStatusChangedEvent{
					LeadID:            leadID,
					TenantID:          tenantID,
					CustomerAccountID: updated.AccountID,
					OldStatus:         string(existing.Status),
					NewStatus:         string(updated.Status),
					Timestamp:         time.Now(),
				})
			}
		}
	}

	// Audit trail (#467). Action específico para mudança de status (mais
	// importante para forense), senão genérico. Existing pode ser nil se
	// o lead foi criado em janela de race entre os SELECTs — nesse caso
	// só registra o After.
	auditAction := "lead.updated"
	if existing != nil && existing.Status != updated.Status {
		auditAction = "lead.status_changed"
	}
	auditFields := []string{
		"name", "email", "phone", "status", "role", "assigned_broker_id",
	}
	var beforeSnap map[string]any
	if existing != nil {
		beforeSnap = audit.Snapshot(existing, auditFields...)
	}
	audit.Record(ctx, audit.Entry{
		Action:     auditAction,
		EntityType: "lead",
		EntityID:   leadID,
		Before:     beforeSnap,
		After:      audit.Snapshot(updated, auditFields...),
		Metadata: map[string]any{
			"via": "patch",
		},
	})

	return updated, ok, nil
}

func (s *LeadService) Update(ctx context.Context, l *lead.Lead) error {
	// Get existing lead to detect status changes + capture before snapshot
	// para o audit.events. Se a leitura falhar (lead novo ou DB flaky),
	// seguimos o caminho de update direto — degradação aceitável, o repo
	// abaixo é a fonte de verdade.
	existing, err := s.repo.GetByID(ctx, l.TenantID, l.ID)
	if err != nil {
		if updErr := s.repo.Update(ctx, l); updErr != nil {
			return updErr
		}
		// Sem before disponível — audit registra apenas o estado novo.
		audit.Record(ctx, audit.Entry{
			Action:     "lead.updated",
			EntityType: "lead",
			EntityID:   l.ID,
			After: audit.Snapshot(l,
				"id", "name", "email", "phone", "status", "role",
				"assigned_broker_id",
			),
		})
		return nil
	}

	if err := s.repo.Update(ctx, l); err != nil {
		return err
	}

	// Publish lead.assigned event if broker changed
	if s.eventBus != nil && existing.AssignedBrokerID != l.AssignedBrokerID && l.AssignedBrokerID != "" {
		// Try to load roulette config to see which channels to notify
		channels := []string{}
		if cfg, _ := s.rouletteRepo.GetConfig(ctx, l.TenantID); cfg != nil {
			for _, ch := range cfg.NotifChannels {
				if ch.Enabled {
					channels = append(channels, ch.Name)
				}
			}
		}

		s.publishAsync(ctx, LeadAssignedEvent{
			LeadID:   l.ID,
			LeadName: l.Name,
			TenantID: l.TenantID,
			BrokerID: l.AssignedBrokerID,
			Channels: channels,
		})
	}

	// Publish lead.status_changed event if status changed
	if s.eventBus != nil && existing.Status != l.Status {
		metrics.IncLeadStatusChanged(string(existing.Status), string(l.Status))
		evt := LeadStatusChangedEvent{
			LeadID:            l.ID,
			TenantID:          l.TenantID,
			CustomerAccountID: l.AccountID,
			OldStatus:         string(existing.Status),
			NewStatus:         string(l.Status),
			Timestamp:         time.Now(),
		}
		s.publishAsync(ctx, evt)
	}

	// Catch-all lead.updated: fires whenever any user-visible field changes
	// (name/email/phone/role). Lets downstream services that snapshot
	// lead labels (toca-deal contract titles, toca-financial fiscal records,
	// toca-portal client cache) refresh their projections so they don't drift
	// from the canonical lead record kept here.
	if s.eventBus != nil && leadVisibleFieldsChanged(existing, l) {
		s.publishAsync(ctx, contracts.LeadUpdatedEvent{
			Data: contracts.LeadUpdatedEventPayload{
				LeadID:    l.ID,
				TenantID:  l.TenantID,
				Name:      l.Name,
				Email:     l.Email,
				Phone:     l.Phone,
				Role:      string(l.Role),
				UpdatedAt: time.Now().UTC().Format(time.RFC3339),
			},
		})
	}

	// Audit trail (#467). Action específico quando muda status, senão um
	// genérico lead.updated. Diff é calculado depois para reduzir payload.
	auditAction := "lead.updated"
	if existing.Status != l.Status {
		auditAction = "lead.status_changed"
	}
	auditFields := []string{
		"name", "email", "phone", "status", "role", "assigned_broker_id",
	}
	beforeSnap := audit.Snapshot(existing, auditFields...)
	afterSnap := audit.Snapshot(l, auditFields...)
	audit.Record(ctx, audit.Entry{
		Action:     auditAction,
		EntityType: "lead",
		EntityID:   l.ID,
		Before:     beforeSnap,
		After:      afterSnap,
		Metadata: map[string]any{
			"status_changed":     existing.Status != l.Status,
			"assignment_changed": existing.AssignedBrokerID != l.AssignedBrokerID,
		},
	})

	return nil
}

// leadVisibleFieldsChanged reports whether any of the fields that downstream
// projections care about have changed between the persisted and incoming
// lead. Status/assignment changes have their own events and are excluded.
func leadVisibleFieldsChanged(existing, incoming *lead.Lead) bool {
	if existing == nil || incoming == nil {
		return false
	}
	return existing.Name != incoming.Name ||
		existing.Email != incoming.Email ||
		existing.Phone != incoming.Phone ||
		existing.Role != incoming.Role
}

func (s *LeadService) Delete(ctx context.Context, id string) error {
	tenantID, err := tenantFromContext(ctx)
	if err != nil {
		return err
	}
	// Snapshot antes do delete para o audit ter contexto de quem foi
	// removido — sem isso "lead.deleted" no audit trail seria opaco
	// (operadora investigando incidente teria que cruzar com backup).
	var beforeSnap map[string]any
	if existing, loadErr := s.repo.GetByID(ctx, tenantID, id); loadErr == nil && existing != nil {
		beforeSnap = audit.Snapshot(existing,
			"id", "tenant_id", "name", "email", "phone", "status",
			"role", "assigned_broker_id",
		)
	}
	if err := s.repo.Delete(ctx, tenantID, id); err != nil {
		// SECURITY (achado C9): o repositório devolve sql.ErrNoRows quando
		// nenhuma linha do tenant foi afetada (lead inexistente ou de outro
		// tenant). Antes isso virava "sucesso" com entrada de auditoria
		// registrando uma exclusão que nunca aconteceu.
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("lead not found")
		}
		return err
	}
	audit.Record(ctx, audit.Entry{
		Action:     "lead.deleted",
		EntityType: "lead",
		EntityID:   id,
		Before:     beforeSnap,
	})
	return nil
}

func (s *LeadService) List(ctx context.Context, tenantID string, filter lead.LeadFilter) ([]*lead.Lead, int64, error) {
	return s.repo.List(ctx, tenantID, filter)
}

// SharedLead wraps a privacy-projected lead exposed through the partner
// shared-content feature with the partnership it came from. SourceCompanyID is
// the counterpart (owner) tenant; SourceCompanyName is best-effort — toca-crm
// has no cross-company name lookup, so it is left empty (the frontend resolves
// the label from the company directory it already loads).
type SharedLead struct {
	Lead              *lead.Lead `json:"lead"`
	PartnershipID     string     `json:"partnership_id"`
	SourceCompanyID   string     `json:"source_company_id"`
	SourceCompanyName string     `json:"source_company_name,omitempty"`
}

// SharedLeadsForCompany aggregates the leads shared with the caller across
// EVERY active partnership that grants the `leads` view scope. For each such
// partnership it lists the counterpart company's opted-in leads
// (PartnershipShared=true) and projects each one for partner consumption
// (LGPD — see lead.ProjectLeadForPartner). A repository error on one
// partnership's listing aborts the call.
//
// Mirrors toca-core PartnershipService.AllSharedPropertiesForCompany.
func (s *LeadService) SharedLeadsForCompany(ctx context.Context, callerTenantID string) ([]SharedLead, error) {
	if callerTenantID == "" {
		return nil, fmt.Errorf("access denied")
	}
	if s.crossRepo == nil {
		return nil, fmt.Errorf("partnership lookup not configured")
	}

	partnerships, err := s.crossRepo.ActivePartnershipsForCompany(ctx, callerTenantID)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	out := make([]SharedLead, 0)

	for _, p := range partnerships {
		if p == nil || p.Status != "active" {
			continue
		}
		if _, dup := seen[p.ID]; dup {
			continue
		}
		seen[p.ID] = struct{}{}

		// Only partnerships that grant the leads/view scope expose leads.
		if !p.GrantFrom(p.Counterpart(callerTenantID)).Allows("leads") {
			continue
		}
		ownerTenantID := p.Counterpart(callerTenantID)
		if ownerTenantID == "" {
			continue
		}

		leads, err := s.repo.ListSharedForPartner(ctx, ownerTenantID)
		if err != nil {
			return nil, err
		}
		for _, l := range leads {
			out = append(out, SharedLead{
				Lead:              lead.ProjectLeadForPartner(l, p.GrantFrom(ownerTenantID).Sharing.ShareLeadContact),
				PartnershipID:     p.ID,
				SourceCompanyID:   ownerTenantID,
				SourceCompanyName: p.CompanyNames[ownerTenantID],
			})
		}
	}

	return out, nil
}

// Disqualify marca o lead como DISQUALIFIED com um motivo obrigatório
// (#429). Diferente de uma troca de status genérica: registra o motivo
// no histórico do lead e emite o audit lead.disqualified — o evento que
// uma investigação de incidente (J10) procura. Idempotente: lead já
// desqualificado é devolvido sem reprocessar.
func (s *LeadService) Disqualify(ctx context.Context, leadID, reason, actor string) (*lead.Lead, error) {
	tenantID, err := tenantFromContext(ctx)
	if err != nil {
		return nil, err
	}
	l, err := s.repo.GetByID(ctx, tenantID, leadID)
	if err != nil {
		return nil, err
	}
	if l == nil {
		return nil, fmt.Errorf("lead not found")
	}
	if l.Status == lead.LeadStatusDisqualified {
		return l, nil // idempotente — já desqualificado
	}

	oldStatus := l.Status
	before := audit.Snapshot(l, "id", "name", "status", "assigned_broker_id")
	l.Status = lead.LeadStatusDisqualified
	l.AddHistory(lead.LeadActionDisqualified, actor, reason, &oldStatus, &l.Status)

	if err := s.repo.Update(ctx, l); err != nil {
		return nil, err
	}

	// Notifica downstream da mudança de status (mesma rota do Update).
	if s.eventBus != nil {
		s.publishAsync(ctx, LeadStatusChangedEvent{
			LeadID:            l.ID,
			TenantID:          l.TenantID,
			CustomerAccountID: l.AccountID,
			OldStatus:         string(oldStatus),
			NewStatus:         string(l.Status),
			Timestamp:         time.Now(),
		})
	}

	// Audit dedicado: ação lead.disqualified carrega o motivo no
	// metadata — é o que a timeline de incidente do /audit-trail mostra.
	audit.Record(ctx, audit.Entry{
		Action:     "lead.disqualified",
		EntityType: "lead",
		EntityID:   l.ID,
		Before:     before,
		After:      audit.Snapshot(l, "id", "name", "status", "assigned_broker_id"),
		Metadata:   map[string]any{"reason": reason},
	})

	return l, nil
}

func (s *LeadService) GetStats(ctx context.Context, tenantID string, brokerID string) (int64, map[string]int64, float64, error) {
	return s.repo.GetStats(ctx, tenantID, brokerID)
}

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

// ClientVisitRequestedEvent é publicado quando o cliente solicita uma visita.
type ClientVisitRequestedEvent struct {
	LeadID        string `json:"lead_id"`
	BrokerID      string `json:"broker_id"`
	TenantID      string `json:"tenant_id"`
	ClientName    string `json:"client_name"`
	ClientEmail   string `json:"client_email"`
	PropertyID    string `json:"property_id"`
	Date          string `json:"date"`
	Time          string `json:"time"`
	AppointmentID string `json:"appointment_id"`
}

func (e ClientVisitRequestedEvent) EventID() string {
	return fmt.Sprintf("%s-%s-%s", e.Name(), e.LeadID, e.Date+" "+e.Time)
}
func (e ClientVisitRequestedEvent) Name() string         { return "client.visit.requested" }
func (e ClientVisitRequestedEvent) Payload() interface{} { return e }

// ClientVisitConfirmedEvent é publicado quando o corretor confirma a visita.
type ClientVisitConfirmedEvent struct {
	LeadID      string `json:"lead_id"`
	BrokerID    string `json:"broker_id"`
	TenantID    string `json:"tenant_id"`
	ClientEmail string `json:"client_email"`
	Date        string `json:"date"`
	Time        string `json:"time"`
}

func (e ClientVisitConfirmedEvent) EventID() string      { return fmt.Sprintf("%s-%s", e.Name(), e.LeadID) }
func (e ClientVisitConfirmedEvent) Name() string         { return "client.visit.confirmed" }
func (e ClientVisitConfirmedEvent) Payload() interface{} { return e }

func (s *LeadService) InviteClient(ctx context.Context, id string) error {
	tenantID, err := tenantFromContext(ctx)
	if err != nil {
		return err
	}
	l, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if l == nil {
		return fmt.Errorf("lead not found")
	}

	if l.Email == "" {
		return fmt.Errorf("lead must have an email to be invited")
	}

	authBaseURL := os.Getenv("AUTH_SERVICE_URL")
	internalKey := os.Getenv("INTERNAL_API_KEY")
	if authBaseURL == "" {
		return fmt.Errorf("AUTH_SERVICE_URL not configured")
	}
	if internalKey == "" {
		return fmt.Errorf("INTERNAL_API_KEY not configured")
	}
	authURL := authBaseURL + "/api/v1/auth/internal/accounts/ensure-client"

	payload := map[string]string{
		"email":     l.Email,
		"name":      l.Name,
		"phone":     l.Phone,
		"tenant_id": l.TenantID,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, authURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Key", internalKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call auth service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if len(body) > 0 {
			return fmt.Errorf("auth service returned status: %d: %s", resp.StatusCode, string(body))
		}
		return fmt.Errorf("auth service returned status: %d", resp.StatusCode)
	}

	var authResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			AccountID string `json:"account_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return fmt.Errorf("failed to decode auth response: %w", err)
	}

	if !authResp.Success {
		if authResp.Message != "" {
			return fmt.Errorf("failed to ensure client account: %s", authResp.Message)
		}
		return fmt.Errorf("failed to ensure client account")
	}
	if authResp.Data.AccountID == "" {
		return fmt.Errorf("auth service did not return an account_id")
	}

	l.AccountID = authResp.Data.AccountID
	if err := s.repo.Update(ctx, l); err != nil {
		return fmt.Errorf("failed to update lead with account_id: %w", err)
	}

	if s.eventBus != nil {
		evt := ClientInvitedEvent{
			Email:      l.Email,
			ClientName: l.Name,
			Phone:      l.Phone,
			AccountID:  l.AccountID,
			TenantID:   l.TenantID,
		}
		s.publishAsync(ctx, evt)
	}

	return nil
}

// ClientPortalOverviewLead é o payload do GET /client-portal/overview.
// Para um cliente convidado (account_id), devolve o(s) lead(s) ligado(s) e,
// quando houver preferências de busca, os imóveis recomendados (match #415).
type ClientPortalOverviewLead struct {
	ID              string             `json:"id"`
	DisplayID       string             `json:"display_id,omitempty"`
	TenantID        string             `json:"tenant_id"`
	Name            string             `json:"name"`
	Email           string             `json:"email"`
	Phone           string             `json:"phone"`
	Status          string             `json:"status,omitempty"`
	CreatedAt       time.Time          `json:"created_at,omitempty"`
	UpdatedAt       time.Time          `json:"updated_at,omitempty"`
	LastInteraction *time.Time         `json:"last_interaction,omitempty"`
	Interest        *lead.LeadProperty `json:"interest,omitempty"`
	// Role é o papel do cliente no atendimento (buyer/tenant/owner): é o que
	// permite abrir a busca na finalidade certa para quem não tem negócio
	// aberto (comprador → venda, locatário → locação).
	Role                  string               `json:"role,omitempty"`
	HasPreferences        bool                 `json:"has_preferences"`
	RecommendedPropertyID string               `json:"recommended_property_id,omitempty"`
	RecommendedProperty   *clients.PropertyDTO `json:"recommended_property,omitempty"`
	BrokerName            string               `json:"broker_name,omitempty"`
	BrokerEmail           string               `json:"broker_email,omitempty"`
	BrokerAvatar          string               `json:"broker_avatar,omitempty"`
	VisitRequest          *lead.VisitRequest   `json:"visit_request,omitempty"`
	MatchedProperties     []MatchedProperty    `json:"matched_properties"`
}

// ErrRecommendedPropertyUnreadable é devolvido quando o imóvel informado não
// pode ser lido pelo operador (não existe, foi excluído ou é de outro tenant).
// A recomendação falha fechada: melhor recusar do que deixar o lead apontando
// para um imóvel que o cliente não conseguirá ver.
var ErrRecommendedPropertyUnreadable = errors.New("recommended property not found or access denied")

func (s *LeadService) SetRecommendedProperty(ctx context.Context, leadID, propertyID string) error {
	tenantID, err := tenantFromContext(ctx)
	if err != nil {
		return err
	}
	l, err := s.repo.GetByID(ctx, tenantID, leadID)
	if err != nil {
		return err
	}
	if l == nil {
		return fmt.Errorf("lead not found")
	}
	// SECURITY/consistência (revisão de acesso a imóveis, 2026-09-19): o imóvel
	// precisa ser LEGÍVEL pelo operador antes de virar recomendação. O
	// `GetPropertyByID` vai ao core com o token dele (escopo de tenant), então
	// id inexistente — ou de OUTRA imobiliária — devolve erro; antes o código
	// ignorava o erro e gravava o id mesmo assim, deixando o lead com uma
	// recomendação que o portal não conseguia descrever (ou descrevia com dado
	// alheio).
	if s.propertyClient == nil {
		return fmt.Errorf("property lookup not configured")
	}
	prop, perr := s.propertyClient.GetPropertyByID(ctx, propertyID)
	if perr != nil {
		return fmt.Errorf("%w: %v", ErrRecommendedPropertyUnreadable, perr)
	}
	l.RecommendedPropertyID = propertyID
	// Snapshot do imóvel recomendado (o operador tem token para acessar o core;
	// o cliente no portal não). O overview usa esse snapshot para exibir a
	// sugestão com detalhes.
	l.RecommendedPropertySnapshot = &lead.RecommendedPropertySnapshot{
		ID: prop.ID, Title: prop.Title, Neighborhood: prop.Neighborhood,
		City: prop.City, State: prop.State, SalePrice: prop.SalePrice,
		RentPrice: prop.RentPrice, ListingType: prop.ListingType, ThumbnailURL: prop.ThumbnailURL,
		MediaImages: prop.MediaImages,
	}
	if err := s.repo.Update(ctx, l); err != nil {
		return err
	}

	// Beta de e-mail transacional (F3): publica a recomendação para o
	// toca-comm notificar o cliente (in-app + e-mail "Sugestão do seu
	// corretor"). Falha de publicação não desfaz a recomendação gravada.
	if s.eventBus != nil {
		accounts := []string{}
		if l.AccountID != "" {
			accounts = append(accounts, l.AccountID)
		}
		s.publishAsync(ctx, contracts.LeadRecommendedPropertyEvent{
			Data: contracts.RecommendedPropertyEventPayload{
				TenantID:           l.TenantID,
				LeadID:             l.ID,
				PropertyID:         prop.ID,
				Title:              prop.Title,
				Neighborhood:       prop.Neighborhood,
				City:               prop.City,
				State:              prop.State,
				SalePrice:          prop.SalePrice,
				RentPrice:          prop.RentPrice,
				ListingType:        prop.ListingType,
				ImageURL:           prop.ThumbnailURL,
				BrokerID:           l.AssignedBrokerID,
				CustomerAccountIDs: accounts,
				Timestamp:          contracts.RecommendedPropertyTimestamp(time.Now()),
			},
		})
	}
	return nil
}

// lookupBrokerForPortal resolve o corretor no tenant do LEAD pelo pool de
// serviço. O portal do cliente é cross-tenant por definição (o tenant do token
// é a conta local do cliente) e, sob RLS, o pool de aplicação não enxergaria o
// corretor da imobiliária — o card do portal ficava sem nome/avatar.
func (s *LeadService) lookupBrokerForPortal(ctx context.Context, tenantID, brokerID string) *broker.Broker {
	if brokerID == "" || s.brokerRepo == nil {
		return nil
	}
	if sr, ok := s.brokerRepo.(interface {
		GetByIDService(context.Context, string, string) (*broker.Broker, error)
	}); ok {
		if b, err := sr.GetByIDService(ctx, tenantID, brokerID); err == nil {
			return b
		}
		return nil
	}
	if b, err := s.brokerRepo.GetByID(ctx, tenantID, brokerID); err == nil {
		return b
	}
	return nil
}

func (s *LeadService) ClientPortalOverview(ctx context.Context, accountID, email string) ([]ClientPortalOverviewLead, error) {
	leads, err := s.repo.FindByAccountID(ctx, accountID, 20)
	if err != nil {
		return nil, err
	}
	// A identidade do cliente convidado pode divergir entre o account_id
	// (ensure-client) e o usuário do OTP (Supabase). Como fallback robusto,
	// busca também pelo e-mail, que é a identidade estável do lead.
	if len(leads) == 0 && email != "" {
		// SECURITY (A19): cross-tenant DELIBERADO — a autorização aqui é a
		// posse do e-mail pelo token do portal (o tenant do token de um cliente
		// OTP é a conta local dele, não a empresa dona do lead).
		if eLeads, eErr := s.repo.FindByEmailAnyTenant(ctx, email, 20); eErr == nil && len(eLeads) > 0 {
			leads = eLeads
		}
	}
	if err != nil {
		return nil, err
	}
	out := make([]ClientPortalOverviewLead, 0, len(leads))
	for _, l := range leads {
		// A agenda é ENRIQUECIMENTO do cartão de entrada do portal: reflete o
		// que o corretor confirmou no calendário. Uma indisponibilidade do
		// calendário não pode derrubar a busca do cliente inteira — era o que
		// acontecia (o handler traduzia ErrCalendarUnavailable em 503 e o
		// cliente via "Não foi possível carregar sua busca" mesmo com a visita
		// intacta no calendário). Mantém o último estado PERSISTIDO da visita e
		// segue montando o item; a falha fica registrada para o operador.
		if vErr := s.refreshVisitFromAgenda(ctx, l); vErr != nil {
			logger.WarnContext(ctx, "portal visit refresh failed; keeping stored visit state",
				"lead_id", l.ID, "tenant_id", l.TenantID, "error", vErr.Error())
		}
		// O snapshot foi gravado no dia da recomendação. Antes de montar a
		// resposta, confere o estado ATUAL do imóvel: recomendação de imóvel
		// vendido, alugado, pausado ou excluído deixa de aparecer, e o valor
		// exibido é o de hoje — não o congelado (revisão de acesso a imóveis,
		// 2026-09-19, item A7). A checagem acontece ANTES de montar o item,
		// porque é o item que carrega o id para o portal.
		if l.RecommendedPropertyID != "" {
			if live := s.livePropertySnapshot(ctx, l.TenantID, l.RecommendedPropertyID); live != nil {
				if !live.Available() {
					l.RecommendedPropertyID = ""
					l.RecommendedPropertySnapshot = nil
				} else {
					l.RecommendedPropertySnapshot = snapshotFromLive(live)
				}
			}
		}
		item := ClientPortalOverviewLead{
			ID: l.ID, DisplayID: l.DisplayID, TenantID: l.TenantID, Name: l.Name, Email: l.Email, Phone: l.Phone,
			Status:                string(l.Status),
			CreatedAt:             l.CreatedAt,
			UpdatedAt:             l.UpdatedAt,
			LastInteraction:       l.LastInteraction,
			Interest:              l.PropertyInterest,
			Role:                  string(l.Role),
			RecommendedPropertyID: l.RecommendedPropertyID,
			VisitRequest:          l.VisitRequest,
			MatchedProperties:     []MatchedProperty{},
		}
		if l.AssignedBrokerID != "" && s.brokerRepo != nil {
			// A19: o corretor é resolvido no tenant do lead (não no tenant do
			// token do portal, que para um cliente OTP é a conta local dele).
			// Leitura pelo pool de serviço — o portal é cross-tenant por
			// definição e, sob RLS, o pool de aplicação não enxerga o corretor
			// da imobiliária.
			if b := s.lookupBrokerForPortal(ctx, l.TenantID, l.AssignedBrokerID); b != nil {
				item.BrokerName = b.Name
				item.BrokerEmail = b.Email
				item.BrokerAvatar = b.Avatar
			}
		}
		if l.RecommendedPropertyID != "" {
			if l.RecommendedPropertySnapshot != nil {
				recProp := &clients.PropertyDTO{
					ID: l.RecommendedPropertySnapshot.ID, Title: l.RecommendedPropertySnapshot.Title,
					Neighborhood: l.RecommendedPropertySnapshot.Neighborhood, City: l.RecommendedPropertySnapshot.City,
					State: l.RecommendedPropertySnapshot.State, SalePrice: l.RecommendedPropertySnapshot.SalePrice,
					RentPrice: l.RecommendedPropertySnapshot.RentPrice, ListingType: l.RecommendedPropertySnapshot.ListingType,
					ThumbnailURL: l.RecommendedPropertySnapshot.ThumbnailURL,
					MediaImages:  l.RecommendedPropertySnapshot.MediaImages,
				}
				// Recomendações antigas podem não ter media_images no snapshot —
				// completa com as fotos do match (se o imóvel estiver entre eles).
				if len(recProp.MediaImages) == 0 && recProp.ThumbnailURL != "" {
					recProp.MediaImages = []string{recProp.ThumbnailURL}
				}
				item.RecommendedProperty = recProp
			} else if s.propertyClient != nil {
				if prop, perr := s.propertyClient.GetPropertyByID(ctx, l.RecommendedPropertyID); perr == nil {
					item.RecommendedProperty = prop
				}
			}
		}
		if l.PropertyInterest != nil && hasAnyPreference(*l.PropertyInterest) {
			item.HasPreferences = true
			if matches, mErr := s.MatchPropertiesForLead(ctx, l, 10); mErr == nil {
				item.MatchedProperties = matches
			}
		}
		out = append(out, item)
	}
	return out, nil
}

// livePropertySnapshot lê o estado atual do imóvel no portfólio do tenant.
//
// Devolve nil quando não há como ler (cross repo não wired, erro de banco ou
// imóvel inexistente): nesse caso o snapshot gravado continua valendo — degradar
// para o dado antigo é melhor que sumir com a recomendação por indisponibilidade
// de leitura.
func (s *LeadService) livePropertySnapshot(ctx context.Context, tenantID, propertyID string) *repository.PropertySnapshot {
	if s.crossRepo == nil || tenantID == "" || propertyID == "" {
		return nil
	}
	snap, err := s.crossRepo.PropertySnapshot(ctx, tenantID, propertyID)
	if err != nil {
		return nil
	}
	return snap
}

// snapshotFromLive projeta a leitura viva no formato que o portal consome.
func snapshotFromLive(live *repository.PropertySnapshot) *lead.RecommendedPropertySnapshot {
	if live == nil {
		return nil
	}
	images := live.MediaImages
	if len(images) == 0 && live.ThumbnailURL != "" {
		images = []string{live.ThumbnailURL}
	}
	return &lead.RecommendedPropertySnapshot{
		ID: live.ID, Title: live.Title, Neighborhood: live.Neighborhood,
		City: live.City, State: live.State, SalePrice: live.SalePrice,
		RentPrice: live.RentPrice, ListingType: live.ListingType,
		ThumbnailURL: live.ThumbnailURL, MediaImages: images,
	}
}

// ClientPortalPortfolio devolve a vitrine do cliente que NÃO tem negócio aberto:
// os imóveis disponíveis das imobiliárias com quem ele se relaciona (os tenants
// dos leads dele).
//
// Antes, quem não tinha negócio só via a recomendação/match do lead — a busca do
// portal é `deal-scoped`. O recorte de disponibilidade e de finalidade é o mesmo
// da busca (`available_for`, status publicados), e a autorização é a POSSE: os
// tenants saem dos leads do próprio cliente (por account_id e, no fallback do
// convite, por e-mail).
func (s *LeadService) ClientPortalPortfolio(ctx context.Context, accountID, email, listingType string, limit int) ([]*repository.AvailableProperty, error) {
	leads, err := s.repo.FindByAccountID(ctx, accountID, 50)
	if err != nil {
		return nil, err
	}
	if len(leads) == 0 && email != "" {
		if eLeads, eErr := s.repo.FindByEmailAnyTenant(ctx, email, 50); eErr == nil {
			leads = eLeads
		}
	}
	seen := map[string]bool{}
	tenantIDs := make([]string, 0, len(leads))
	for _, l := range leads {
		if l == nil || l.TenantID == "" || seen[l.TenantID] {
			continue
		}
		seen[l.TenantID] = true
		tenantIDs = append(tenantIDs, l.TenantID)
	}
	if len(tenantIDs) == 0 {
		return []*repository.AvailableProperty{}, nil
	}
	if s.crossRepo == nil {
		return nil, fmt.Errorf("property portfolio lookup not configured")
	}
	properties, err := s.crossRepo.AvailableProperties(ctx, tenantIDs, listingType, limit)
	if err != nil {
		return nil, err
	}
	return properties, nil
}
