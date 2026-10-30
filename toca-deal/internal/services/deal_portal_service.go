package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/domain"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/storage"
	"github.com/oginoino/toca/pkg/timeutil"
	"github.com/oginoino/toca/pkg/virusscan"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	"github.com/oginoino/toca/toca-deal/internal/domain/portal"
	repository "github.com/oginoino/toca/toca-deal/internal/repositories"
)

// Client-facing deal operations live here to keep the core deal lifecycle
// independent from portal orchestration while preserving the same service API.

type ClientLeadResponse struct {
	DealID     string `json:"deal_id"`
	TenantID   string `json:"tenant_id"`
	LeadID     string `json:"lead_id,omitempty"`
	AccountID  string `json:"account_id,omitempty"`
	Role       string `json:"role,omitempty"`
	PropertyID string `json:"property_id,omitempty"`
}

func (s *DealService) GetMyLeads(ctx context.Context, accountID string) ([]ClientLeadResponse, error) {
	deals, err := s.portal().FindByPartyAccountID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	out := make([]ClientLeadResponse, 0)
	for _, d := range deals {
		leadID := ""
		role := ""
		for _, p := range d.Parties {
			if p.AccountID == accountID {
				leadID = p.LeadID
				role = string(p.Role)
				break
			}
		}
		if leadID == "" {
			leadID = d.CustomerID
		}
		out = append(out, ClientLeadResponse{
			DealID:     d.ID,
			TenantID:   d.TenantID,
			LeadID:     leadID,
			AccountID:  accountID,
			Role:       role,
			PropertyID: d.PropertyID,
		})
	}
	return out, nil
}

func (s *DealService) ListBySignerEmail(ctx context.Context, email string) ([]*deal.Deal, error) {
	return s.portal().FindBySignerEmail(ctx, email)
}

func (s *DealService) ListBySignerIdentity(ctx context.Context, accountID, email string) ([]*deal.Deal, error) {
	return s.portal().FindBySignerIdentity(ctx, accountID, email)
}

// ErrPortalAccessDenied marca o negócio que não pertence ao cliente autenticado.
//
// A mensagem é a MESMA string histórica (`"deal not found or access denied"`)
// porque os handlers do portal a comparam por texto; o sentinel existe para que
// o mapeamento de status não dependa dessa comparação — e para separar "acesso
// negado" (403) de "serviço de destino falhou" (503), que antes caíam os dois
// no mesmo 403 e escondiam defeito de configuração como se fosse autorização.
var ErrPortalAccessDenied = errors.New("deal not found or access denied")

// ErrPortalUpstreamUnavailable marca falha ao falar com um serviço de destino
// (financeiro, calendário, ops) — nunca culpa do cliente.
var ErrPortalUpstreamUnavailable = errors.New("portal upstream service unavailable")

// ErrPortalTicketAccessDenied é o par de ErrPortalAccessDenied para chamados: o
// chamado não existe ou não é do cliente autenticado.
var ErrPortalTicketAccessDenied = errors.New("ticket not found or access denied")

func (s *DealService) findMyDeal(ctx context.Context, accountID, email, dealID string) (*deal.Deal, error) {
	deals, err := s.portal().FindBySignerIdentity(ctx, accountID, email)
	if err != nil {
		return nil, err
	}
	var found *deal.Deal
	for _, d := range deals {
		if d.ID == dealID {
			found = d
			break
		}
	}
	if found == nil {
		return nil, ErrPortalAccessDenied
	}
	s.populateChecklistURLs(ctx, found)
	return found, nil
}

func (s *DealService) GetMyDealProperty(ctx context.Context, accountID, email, dealID string) (*portal.PropertySummary, error) {
	found, err := s.findMyDeal(ctx, accountID, email, dealID)
	if err != nil {
		return nil, err
	}
	if s.propertyRepo == nil {
		return nil, errors.New("property repository not configured")
	}
	// O portal é cross-tenant: prefere a leitura no pool de serviço (a mesma
	// consulta do operador, sem o filtro de tenant que não se aplica ao cliente).
	if serviceReader, ok := s.propertyRepo.(interface {
		GetSummaryByIDService(context.Context, string, string, deal.Type) (*portal.PropertySummary, error)
	}); ok {
		return serviceReader.GetSummaryByIDService(ctx, found.TenantID, found.PropertyID, found.Type)
	}
	return s.propertyRepo.GetSummaryByID(ctx, found.TenantID, found.PropertyID, found.Type)
}

func (s *DealService) GetMyDealRecommendations(ctx context.Context, accountID, email, dealID string, limit int64) ([]*portal.PropertySummary, error) {
	found, err := s.findMyDeal(ctx, accountID, email, dealID)
	if err != nil {
		return nil, err
	}
	if s.propertyRepo == nil {
		return nil, errors.New("property repository not configured")
	}

	var interest interface{}
	if s.leadRepo != nil {
		leadID := found.LeadID
		if leadID == "" {
			leadID = found.CustomerID
		}
		if leadID != "" {
			i, err := s.leadRepo.GetInterest(ctx, found.TenantID, leadID)
			if err == nil {
				interest = i
			}
		}
	}

	return s.propertyRepo.ListRecommendations(ctx, found.TenantID, found.Type, found.PropertyID, interest, limit)
}

// ListMyChannels lista os canais do cliente do portal.
//
// SECURITY (achados A3/A9 da revisão de permissões de 2026-09-19): o toca-comm
// deixou de aceitar `?participant=` sem tenant (a chave interna listava canais
// de qualquer tenant). Como o cliente do portal tem `tenant_id` no token igual à
// própria conta (VerifyOTP), o tenant do canal não sai do token dele: sai dos
// vínculos dele com cada imobiliária — que é exatamente o conjunto de tenants
// cujos canais o portal deve mostrar. A lista é a união por participante dentro
// de cada um desses tenants.
//
// O vínculo NÃO é só o negócio: o corretor cadastra o cliente como LEAD, abre a
// conversa e só depois existe negócio — é o caso do proprietário convidado para
// o portal. Por isso os tenants vêm de negócios E de atendimentos
// (`ClientLeadTenants`). Derivando só de negócios, o cliente sem negócio não
// tinha tenant para consultar e via "Nenhuma conversa disponível" com a mensagem
// do corretor já enviada (incidente 2026-09-21).
//
// Dentro do tenant, a identidade casada é resolvida pelo toca-comm (conta do
// portal + leads vinculados): o canal aberto pelo painel nomeia o lead.
func (s *DealService) ListMyChannels(ctx context.Context, accountID string) ([]*clients.ChannelResponse, error) {
	tenants, leadTenantErr, err := s.clientTenants(ctx, accountID)
	if err != nil {
		return nil, err
	}

	seenChannel := map[string]bool{}
	out := make([]*clients.ChannelResponse, 0)
	var firstErr error
	for _, tenantID := range tenants {
		channels, cerr := s.commClient.ListChannelsByParticipant(ctx, tenantID, accountID)
		if cerr != nil {
			// Um tenant indisponível não pode apagar a lista dos outros: o
			// portal mostra o que deu para ler e a falha fica registrada.
			logger.Warn("portal.list_my_channels_failed",
				"event", "portal.list_my_channels_failed",
				"tenant_id", tenantID, "account_id", accountID, "error", cerr.Error())
			if firstErr == nil {
				firstErr = cerr
			}
			continue
		}
		for _, ch := range channels {
			if ch == nil || seenChannel[ch.ID] {
				continue
			}
			seenChannel[ch.ID] = true
			out = append(out, ch)
		}
	}

	// Se NENHUM tenant respondeu, o portal precisa saber: uma lista vazia por
	// indisponibilidade é indistinguível de "não tenho canais" e some com o
	// estado de erro da tela. A leitura dos atendimentos entra pelo mesmo motivo,
	// e só quando ela é a única explicação possível para a lista vazia.
	if len(out) == 0 {
		if firstErr == nil {
			firstErr = leadTenantErr
		}
		if firstErr != nil {
			return nil, firstErr
		}
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out, nil
}

// clientTenants reúne as imobiliárias com que a conta se relaciona: as dos
// negócios em que ela é parte/assinante e as dos atendimentos (leads) dela.
//
// Devolve os tenants, o erro da leitura de ATENDIMENTOS (não fatal: o cliente
// pode ter só negócios) e o erro da leitura de NEGÓCIOS (fatal — é a fonte
// histórica e a falha dela já interrompia a listagem).
func (s *DealService) clientTenants(ctx context.Context, accountID string) (tenants []string, leadErr error, fatalErr error) {
	deals, err := s.portal().FindByPartyAccountID(ctx, accountID)
	if err != nil {
		return nil, nil, err
	}

	tenants = make([]string, 0, len(deals))
	seenTenant := map[string]bool{}
	add := func(tenantID string) {
		tenantID = strings.TrimSpace(tenantID)
		if tenantID == "" || seenTenant[tenantID] {
			return
		}
		seenTenant[tenantID] = true
		tenants = append(tenants, tenantID)
	}
	for _, d := range deals {
		add(d.TenantID)
	}

	if s.leadRepo != nil {
		leadTenants, lerr := s.leadRepo.ClientLeadTenants(ctx, accountID)
		if lerr != nil {
			logger.Warn("portal.list_my_channels_lead_tenants_failed",
				"event", "portal.list_my_channels_lead_tenants_failed",
				"account_id", accountID, "error", lerr.Error())
			leadErr = lerr
		} else {
			for _, tenantID := range leadTenants {
				add(tenantID)
			}
		}
	}

	return tenants, leadErr, nil
}

func (s *DealService) GetMyDealChannel(ctx context.Context, accountID, email, dealID string) (*clients.ChannelResponse, error) {
	foundDeal, err := s.findMyDeal(ctx, accountID, email, dealID)
	if err != nil {
		return nil, err
	}

	var participants []string
	participants = append(participants, accountID)
	if foundDeal.BrokerID != "" {
		participants = append(participants, foundDeal.BrokerID)
	}

	tenantID, err := domain.NewTenantID(foundDeal.TenantID)
	if err != nil {
		return nil, fmt.Errorf("deal %s has invalid tenant_id: %w", foundDeal.ID, err)
	}

	req := clients.EnsureChannelRequest{
		TenantID:     tenantID,
		DealID:       foundDeal.ID,
		Participants: participants,
	}

	return s.commClient.EnsureChannelForDeal(ctx, req)
}

func (s *DealService) GetMyDealAppointments(ctx context.Context, accountID, email, dealID string) ([]*clients.Appointment, error) {
	foundDeal, err := s.findMyDeal(ctx, accountID, email, dealID)
	if err != nil {
		return nil, err
	}

	if s.calendarClient == nil {
		return nil, clients.ErrCalendarUnavailable
	}
	appointments, err := s.calendarClient.ListAppointmentsByDeal(ctx, foundDeal.ID, clients.DealAppointmentScope{
		TenantID: foundDeal.TenantID,
		LeadIDs:  portalAppointmentLeadIDs(foundDeal, accountID, email),
	})
	if err != nil {
		return nil, err
	}
	for _, a := range appointments {
		if a != nil {
			a.CanManage = portalAppointmentOwned(foundDeal, a, accountID, email) &&
				portalAppointmentEditableIn(timeutil.LocationForTenant(ctx, foundDeal.TenantID), a)
		}
	}
	return appointments, nil
}

// Preserve pre-negotiation visits for the authenticated client's own leads.
func portalAppointmentLeadIDs(d *deal.Deal, accountID, email string) []string {
	ids := []string{}
	seen := map[string]bool{}
	add := func(id string) {
		if id != "" && !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	matches := func(account, address string) bool {
		return (accountID != "" && account == accountID) || (email != "" && strings.EqualFold(strings.TrimSpace(address), strings.TrimSpace(email)))
	}
	for _, party := range d.Parties {
		if matches(party.AccountID, party.Email) {
			add(party.LeadID)
		}
	}
	// Legacy deals may have signers without the newer party/lead linkage.
	if len(ids) == 0 {
		for _, signer := range d.Signers {
			if matches(signer.AccountID, signer.Email) && (signer.Role == deal.SignatoryRoleLocatario || signer.Role == deal.SignatoryRoleInquilino || signer.Role == deal.SignatoryRoleComprador) {
				if d.LeadID != "" {
					add(d.LeadID)
				} else {
					add(d.CustomerID)
				}
			}
		}
	}
	return ids
}

func (s *DealService) GetMyDealCharges(ctx context.Context, accountID, email, dealID string) ([]*portal.Charge, error) {
	foundDeal, err := s.findMyDeal(ctx, accountID, email, dealID)
	if err != nil {
		return nil, err
	}

	role := string(deal.SignatoryRoleLocatario)
	// Find customer role
	for _, signer := range foundDeal.Signers {
		if signer.AccountID == accountID || signer.Email == email {
			role = string(signer.Role)
			break
		}
	}

	// O tenant do NEGÓCIO vai explícito: a rota interna do toca-financial resolve
	// o tenant pela credencial e recusa chamada de serviço sem ele (achado A3).
	// Sem isto a tela Financeiro do portal recebia 403 do próprio handler, que
	// traduzia o 400 de parâmetro ausente como "acesso negado".
	charges, err := s.financialClient.ListChargesByCustomer(ctx, foundDeal.TenantID, foundDeal.ID, accountID, role)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPortalUpstreamUnavailable, err)
	}
	return mapPortalCharges(charges, time.Now()), nil
}

// mapPortalCharges converte a cobrança da rota interna do toca-financial no que
// o portal do cliente pode ver (G32 do plano de inadimplência).
//
// Multa (`late_fee`), juros (`interest`) e desconto (`discount`) vêm prontos da
// origem — o domínio `charge.Charge` já os expõe e a rota interna devolve o
// JSON do domínio direto, então aqui é mapeamento, não cálculo de encargo.
//
// `days_overdue` é a exceção: a origem só tem o MÉTODO `Charge.DaysOverdue()`,
// e método não é serializado. O atraso é derivado de `due_date` + `status` com
// a mesma régua do domínio; se um dia a origem passar a mandar `days_overdue`,
// o valor dela vence a derivação (ver portalChargeDaysOverdue).
//
// Campos que o portal não deve ver (payments, reminders_sent, applied_event_ids,
// tenant_id, …) não são copiados: o DTO é a allowlist do que vai para o cliente.
func mapPortalCharges(charges []*clients.Charge, now time.Time) []*portal.Charge {
	out := make([]*portal.Charge, 0, len(charges))
	for _, c := range charges {
		if c == nil {
			continue
		}
		out = append(out, &portal.Charge{
			ID:             c.ID,
			Description:    c.Description,
			DueDate:        c.DueDate,
			Amount:         c.Amount,
			LateFee:        c.LateFee,
			Interest:       c.Interest,
			Discount:       c.Discount,
			TotalAmount:    c.TotalAmount,
			DaysOverdue:    portalChargeDaysOverdue(c, now),
			Status:         c.Status,
			PaymentURL:     c.PaymentURL,
			PaidAt:         c.PaidAt,
			ReferenceMonth: c.ReferenceMonth,
		})
	}
	return out
}

// portalChargeDaysOverdue espelha `charge.Charge.DaysOverdue()` do
// toca-financial: zero fora dos estados em aberto (PENDING, OVERDUE,
// PARTIALLY_PAID), zero antes do vencimento e dias corridos depois. Manter a
// mesma régua garante que o "vencida há N dias" do portal bata com o número que
// a régua de cobrança usa.
func portalChargeDaysOverdue(c *clients.Charge, now time.Time) int {
	if c.DaysOverdue > 0 {
		return c.DaysOverdue
	}
	switch c.Status {
	case "PENDING", "OVERDUE", "PARTIALLY_PAID":
	default:
		return 0
	}
	if c.DueDate.IsZero() || now.Before(c.DueDate) {
		return 0
	}
	return int(now.Sub(c.DueDate).Hours() / 24)
}

func (s *DealService) ListMyDealFavorites(ctx context.Context, accountID, email, dealID string) ([]string, error) {
	foundDeal, err := s.findMyDeal(ctx, accountID, email, dealID)
	if err != nil {
		return nil, err
	}

	d, err := s.portal().GetByID(ctx, foundDeal.TenantID, foundDeal.ID)
	if err != nil {
		return nil, err
	}

	var out []string
	for _, f := range d.Favorites {
		if f.AccountID == accountID {
			out = append(out, f.PropertyID)
		}
	}
	return out, nil
}

func (s *DealService) AddMyDealFavorite(ctx context.Context, accountID, email, dealID, propertyID string) (*deal.Deal, error) {
	foundDeal, err := s.findMyDeal(ctx, accountID, email, dealID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(propertyID) == "" {
		return nil, errors.New("property_id is required")
	}

	d, err := s.portal().GetByID(ctx, foundDeal.TenantID, foundDeal.ID)
	if err != nil {
		return nil, err
	}

	for _, f := range d.Favorites {
		if f.AccountID == accountID && f.PropertyID == propertyID {
			return d, nil
		}
	}

	d.Favorites = append(d.Favorites, deal.Favorite{
		PropertyID: propertyID,
		AccountID:  accountID,
		CreatedAt:  time.Now(),
	})
	if err := s.portal().Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *DealService) RemoveMyDealFavorite(ctx context.Context, accountID, email, dealID, propertyID string) (*deal.Deal, error) {
	foundDeal, err := s.findMyDeal(ctx, accountID, email, dealID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(propertyID) == "" {
		return nil, errors.New("property_id is required")
	}

	d, err := s.portal().GetByID(ctx, foundDeal.TenantID, foundDeal.ID)
	if err != nil {
		return nil, err
	}

	var filtered []deal.Favorite
	for _, f := range d.Favorites {
		if f.AccountID == accountID && f.PropertyID == propertyID {
			continue
		}
		filtered = append(filtered, f)
	}
	d.Favorites = filtered
	if err := s.portal().Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *DealService) SearchMyDealProperties(ctx context.Context, accountID, email, dealID string, params repository.SearchParams) ([]*portal.PropertySummary, error) {
	foundDeal, err := s.findMyDeal(ctx, accountID, email, dealID)
	if err != nil {
		return nil, err
	}
	if s.propertyRepo == nil {
		return nil, errors.New("property repository not configured")
	}

	if params.Limit <= 0 {
		params.Limit = 20
	}

	return s.propertyRepo.Search(ctx, foundDeal.TenantID, foundDeal.Type, params)
}

func (s *DealService) GetMyDealMaintenanceTickets(ctx context.Context, accountID, email, dealID string) ([]*portal.MaintenanceTicket, error) {
	foundDeal, err := s.findMyDeal(ctx, accountID, email, dealID)
	if err != nil {
		return nil, err
	}

	// A3: o tenant do próprio negócio acompanha a chamada interna.
	tickets, err := s.opsClient.ListMaintenanceTicketsByProperty(ctx, foundDeal.TenantID, foundDeal.PropertyID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPortalUpstreamUnavailable, err)
	}

	out := make([]*portal.MaintenanceTicket, 0, len(tickets))
	for _, t := range tickets {
		if t == nil {
			continue
		}
		attachments, err := s.presignAttachmentKeys(ctx, "toca-documents", t.Attachments)
		if err != nil {
			return nil, err
		}
		out = append(out, &portal.MaintenanceTicket{
			ID:            t.ID,
			Type:          t.Type,
			Category:      t.Category,
			Status:        t.Status,
			Priority:      t.Priority,
			Description:   t.Description,
			Attachments:   attachments,
			PropertyTitle: t.PropertyTitle,
			CreatedAt:     t.CreatedAt,
			CompletedAt:   t.CompletedAt,
		})
	}
	return out, nil
}

func (s *DealService) CreateMyDealMaintenanceTicket(ctx context.Context, accountID, email, name, dealID string, req portal.CreateMaintenanceTicketRequest) (*portal.MaintenanceTicket, error) {
	foundDeal, err := s.findMyDeal(ctx, accountID, email, dealID)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(req.Description) == "" {
		return nil, errors.New("description is required")
	}

	clientReq := clients.CreateMaintenanceTicketRequest{
		TenantID:      foundDeal.TenantID,
		Type:          strings.TrimSpace(req.Type),
		Category:      strings.TrimSpace(req.Category),
		PropertyID:    foundDeal.PropertyID,
		PropertyTitle: "",
		Requester:     name,
		Status:        "Em Análise",
		Priority:      strings.TrimSpace(req.Priority),
		Description:   strings.TrimSpace(req.Description),
		Attachments:   req.Attachments,
	}

	if clientReq.Type == "" {
		clientReq.Type = "Manutenção"
	}
	if clientReq.Category == "" {
		clientReq.Category = "Geral"
	}
	if clientReq.Priority == "" {
		clientReq.Priority = "Media"
	}
	if strings.TrimSpace(name) == "" {
		clientReq.Requester = email
	}

	ticket, err := s.opsClient.CreateMaintenanceTicket(ctx, clientReq)
	if err != nil {
		return nil, err
	}

	if s.eventBus != nil {
		evt := ClientTicketCreatedEvent{
			TicketID:          ticket.ID,
			DealID:            foundDeal.ID,
			TenantID:          foundDeal.TenantID,
			PropertyID:        foundDeal.PropertyID,
			CustomerAccountID: accountID,
			Priority:          ticket.Priority,
			Category:          ticket.Category,
			Timestamp:         time.Now(),
		}
		if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
			log.Printf("WARN: failed to enqueue client.ticket.created event for ticket=%s: %v", ticket.ID, err)
		}
	}

	attachments, err := s.presignAttachmentKeys(ctx, "toca-documents", ticket.Attachments)
	if err != nil {
		return nil, err
	}
	return &portal.MaintenanceTicket{
		ID:            ticket.ID,
		Type:          ticket.Type,
		Category:      ticket.Category,
		Status:        ticket.Status,
		Priority:      ticket.Priority,
		Description:   ticket.Description,
		Attachments:   attachments,
		PropertyTitle: ticket.PropertyTitle,
		CreatedAt:     ticket.CreatedAt,
		CompletedAt:   ticket.CompletedAt,
	}, nil
}

func (s *DealService) presignAttachmentKeys(ctx context.Context, bucket string, keys []string) ([]string, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	if s.storage == nil {
		return keys, nil
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if legacyKey, ok := storage.ExtractUploadObjectKey(bucket, k); ok {
			k = legacyKey
		} else if strings.Contains(k, "://") || strings.HasPrefix(k, "/") {
			// Preserve already resolved/external URLs. Only storage keys should
			// be sent to the provider's signing endpoint.
			out = append(out, k)
			continue
		}
		url, err := s.storage.GetPresignedURL(ctx, bucket, k, presignedURLTTL)
		if err != nil {
			return nil, err
		}
		out = append(out, url)
	}
	return out, nil
}

func (s *DealService) presignTicketCommentAttachments(ctx context.Context, t *clients.Ticket) error {
	if t == nil || len(t.Comments) == 0 {
		return nil
	}
	for i := range t.Comments {
		attachments, err := s.presignAttachmentKeys(ctx, "toca-documents", t.Comments[i].Attachments)
		if err != nil {
			return err
		}
		t.Comments[i].Attachments = attachments
	}
	return nil
}

func (s *DealService) GetMyDealTickets(ctx context.Context, accountID, email, dealID string) ([]*clients.Ticket, error) {
	foundDeal, err := s.findMyDeal(ctx, accountID, email, dealID)
	if err != nil {
		return nil, err
	}

	tickets, err := s.opsClient.ListTicketsByRequester(ctx, foundDeal.TenantID, accountID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPortalUpstreamUnavailable, err)
	}

	filtered := []*clients.Ticket{}
	for _, t := range tickets {
		if t != nil && t.DealID == foundDeal.ID {
			// Privacy guard idem GetMyTicket: striping notas internas antes
			// de presignar attachments (evita gerar URL de anexo interno).
			t.Comments = filterPublicTicketComments(t.Comments)
			if err := s.presignTicketCommentAttachments(ctx, t); err != nil {
				return nil, err
			}
			filtered = append(filtered, t)
		}
	}
	return filtered, nil
}

func (s *DealService) CreateMyDealTicket(ctx context.Context, accountID, email, name, dealID string, req portal.CreateTicketRequest) (*clients.Ticket, error) {
	foundDeal, err := s.findMyDeal(ctx, accountID, email, dealID)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(req.Subject) == "" {
		return nil, errors.New("subject is required")
	}
	if strings.TrimSpace(req.Description) == "" {
		return nil, errors.New("description is required")
	}

	tType := strings.TrimSpace(req.Type)
	if tType == "" {
		tType = "support"
	}
	category := strings.TrimSpace(req.Category)
	if category == "" {
		category = "general"
	}
	priority := strings.TrimSpace(req.Priority)
	if priority == "" {
		priority = "medium"
	}

	t, err := s.opsClient.CreateTicket(ctx, foundDeal.TenantID, clients.CreateTicketRequest{
		DealID:             foundDeal.ID,
		PropertyID:         foundDeal.PropertyID,
		Type:               tType,
		Category:           category,
		Subject:            strings.TrimSpace(req.Subject),
		Description:        strings.TrimSpace(req.Description),
		Priority:           priority,
		RequesterAccountID: accountID,
		RequesterName:      name,
		RequesterEmail:     email,
	})
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (s *DealService) GetMyTicket(ctx context.Context, accountID, email, ticketID string) (*clients.Ticket, error) {
	deals, err := s.portal().FindBySignerIdentity(ctx, accountID, email)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	for _, d := range deals {
		if d == nil || d.TenantID == "" {
			continue
		}
		if seen[d.TenantID] {
			continue
		}
		seen[d.TenantID] = true
		t, err := s.opsClient.GetTicketByID(ctx, d.TenantID, ticketID)
		if err != nil {
			continue
		}
		if t != nil && t.RequesterAccountID == accountID {
			// Privacy guard: o portal cliente nunca deve enxergar notas
			// internas (operadores comentam questões privadas — ex.: "ligar
			// para o jurídico antes de responder"). Stripamos antes de
			// presignar attachments para evitar gerar URLs de arquivos
			// anexados em notas internas.
			t.Comments = filterPublicTicketComments(t.Comments)
			if err := s.presignTicketCommentAttachments(ctx, t); err != nil {
				return nil, err
			}
			return t, nil
		}
	}
	return nil, ErrPortalTicketAccessDenied
}

// filterPublicTicketComments retorna apenas comentários com visibilidade
// pública (ou vazia, considerada pública por backward-compat com comentários
// legacy criados antes da feature). Usado pelas rotas /client-portal/* para
// nunca vazar notas internas do operador para o requester.
func filterPublicTicketComments(comments []clients.TicketComment) []clients.TicketComment {
	out := make([]clients.TicketComment, 0, len(comments))
	for _, c := range comments {
		// Vazio = legacy = public (preserva comentários antigos)
		if c.Visibility == "" || c.Visibility == "public" {
			out = append(out, c)
		}
	}
	return out
}

func (s *DealService) AddMyTicketComment(ctx context.Context, accountID, email, name, ticketID string, req portal.AddTicketCommentRequest) (*clients.TicketComment, error) {
	t, err := s.GetMyTicket(ctx, accountID, email, ticketID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Content) == "" {
		return nil, errors.New("content is required")
	}

	comment, err := s.opsClient.AddTicketComment(ctx, t.TenantID, t.ID, clients.AddTicketCommentRequest{
		AuthorID:   accountID,
		AuthorName: name,
		AuthorType: "client",
		// Forçamos "public" — o portal cliente nunca pode marcar uma nota
		// como interna, mesmo se o frontend tentar enviar visibility=internal.
		Visibility:  "public",
		Content:     strings.TrimSpace(req.Content),
		Attachments: req.Attachments,
		// Issue #453: idempotency token propaga do portal para o ops.
		// Frontend gera UUID v4 por clique no botão "Enviar"; retry de
		// rede com o mesmo crid retorna o existing em vez de duplicar.
		ClientRequestID: strings.TrimSpace(req.ClientRequestID),
	})
	if err != nil {
		return nil, err
	}
	if comment != nil {
		attachments, err := s.presignAttachmentKeys(ctx, "toca-documents", comment.Attachments)
		if err != nil {
			return nil, err
		}
		comment.Attachments = attachments
	}
	return comment, nil
}

func (s *DealService) CreateMyDealAppointment(ctx context.Context, accountID, email, name, dealID string, req portal.CreateAppointmentRequest) (*clients.Appointment, error) {
	foundDeal, err := s.findMyDeal(ctx, accountID, email, dealID)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(req.Date) == "" || strings.TrimSpace(req.Time) == "" || strings.TrimSpace(req.EndTime) == "" {
		return nil, errors.New("date, time and end_time are required")
	}
	// Fuso configurado pela imobiliária (Configurações → "Fuso horário"): a
	// data/hora do agendamento é um dia civil local, não UTC.
	location := timeutil.LocationForTenant(ctx, foundDeal.TenantID)
	start, err := time.ParseInLocation("2006-01-02 15:04", req.Date+" "+req.Time, location)
	if err != nil || !start.After(time.Now()) {
		return nil, clients.ErrCalendarInvalid
	}

	normalizeAppointmentType := func(raw string) string {
		v := strings.TrimSpace(raw)
		if v == "" {
			return ""
		}
		switch strings.ToUpper(v) {
		case "VISIT", "VISITA":
			return "Visita"
		case "MEETING", "REUNIAO", "REUNIÃO":
			return "Reuniao"
		case "INSPECTION", "VISTORIA":
			return "Vistoria"
		case "SIGNATURE", "ASSINATURA":
			return "Assinatura"
		case "CALL", "LIGACAO", "LIGAÇÃO":
			return "Ligacao"
		case "REMINDER", "LEMBRETE":
			return "Lembrete"
		case "OTHER", "OUTRO":
			return "Outro"
		default:
			return v
		}
	}

	formatPropertyLabel := func(p *portal.PropertySummary) string {
		if p == nil {
			return ""
		}
		parts := make([]string, 0, 3)
		title := strings.TrimSpace(p.Title)
		if title != "" {
			parts = append(parts, title)
		}
		address := strings.TrimSpace(p.Address)
		if address != "" {
			parts = append(parts, address)
		}
		location := make([]string, 0, 3)
		if strings.TrimSpace(p.Neighborhood) != "" {
			location = append(location, strings.TrimSpace(p.Neighborhood))
		}
		if strings.TrimSpace(p.City) != "" {
			location = append(location, strings.TrimSpace(p.City))
		}
		if strings.TrimSpace(p.State) != "" {
			location = append(location, strings.TrimSpace(p.State))
		}
		if len(location) > 0 {
			parts = append(parts, strings.Join(location, ", "))
		}
		return strings.Join(parts, " · ")
	}

	tenantID, err := domain.NewTenantID(foundDeal.TenantID)
	if err != nil {
		return nil, fmt.Errorf("deal %s has invalid tenant_id: %w", foundDeal.ID, err)
	}

	propertyID := foundDeal.PropertyID
	if req.PropertyID != "" {
		propertyID = req.PropertyID
	}
	propertyLabel := ""
	if req.PropertyID != "" && req.PropertyID != foundDeal.PropertyID && s.propertyRepo == nil {
		return nil, errors.New("property lookup unavailable")
	}
	if s.propertyRepo != nil && propertyID != "" {
		summary, err := s.propertyRepo.GetSummaryByID(ctx, foundDeal.TenantID, propertyID, foundDeal.Type)
		if req.PropertyID != "" && req.PropertyID != foundDeal.PropertyID && (err != nil || summary == nil) {
			return nil, errors.New("property not found")
		}
		if err == nil && summary != nil {
			propertyLabel = formatPropertyLabel(summary)
		}
	}

	clientReq := clients.CreateAppointmentRequest{
		TenantID:          tenantID,
		Title:             strings.TrimSpace(req.Title),
		Date:              strings.TrimSpace(req.Date),
		Time:              strings.TrimSpace(req.Time),
		EndTime:           strings.TrimSpace(req.EndTime),
		Type:              normalizeAppointmentType(req.Type),
		Priority:          "Media",
		Client:            strings.TrimSpace(name),
		LeadID:            foundDeal.CustomerID,
		CustomerAccountID: accountID,
		DealID:            foundDeal.ID,
		PropertyID:        propertyID,
		Property:          propertyLabel,
		Agent:             "Portal",
		AgentID:           foundDeal.BrokerID,
		Status:            "Pendente",
		Description:       strings.TrimSpace(req.Description),
	}

	if clientReq.Title == "" {
		clientReq.Title = "Visita"
	}
	if clientReq.Type == "" {
		clientReq.Type = "Visita"
	}
	if clientReq.Client == "" {
		clientReq.Client = email
	}

	appt, err := s.calendarClient.CreateAppointment(ctx, clientReq)
	if err != nil {
		return nil, err
	}

	if s.eventBus != nil {
		evt := ClientAppointmentRequestedEvent{
			AppointmentID:     appt.ID,
			DealID:            foundDeal.ID,
			TenantID:          foundDeal.TenantID,
			Date:              clientReq.Date,
			PropertyID:        propertyID,
			CustomerAccountID: accountID,
			BrokerID:          foundDeal.BrokerID,
			Timestamp:         time.Now(),
		}
		if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
			log.Printf("WARN: failed to enqueue client.appointment.requested event for appointment=%s: %v", appt.ID, err)
		}
	}

	return appt, nil
}

// SetRecommendedProperty registra o imóvel recomendado pelo corretor ao cliente.
func (s *DealService) SetRecommendedProperty(ctx context.Context, tenantID, dealID, propertyID, brokerID string) error {
	d, err := s.GetByID(ctx, tenantID, dealID, brokerID)
	if err != nil {
		return err
	}
	if d == nil {
		return errors.New("deal not found")
	}
	d.RecommendedPropertyID = propertyID
	d.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, d); err != nil {
		return err
	}

	// Beta de e-mail transacional (F3): publica a recomendação para o
	// toca-comm notificar as partes (in-app + e-mail "Sugestão do seu
	// corretor"). Snapshot vem do resumo do imóvel; falha de publicação não
	// desfaz a recomendação gravada.
	if s.eventBus != nil {
		payload := contracts.RecommendedPropertyEventPayload{
			TenantID:   tenantID,
			DealID:     dealID,
			PropertyID: propertyID,
			BrokerID:   brokerID,
			Timestamp:  contracts.RecommendedPropertyTimestamp(time.Now()),
		}
		accounts := []string{}
		for _, p := range d.Parties {
			if (p.Role == deal.PartyRoleBuyer || p.Role == deal.PartyRoleTenant) && strings.TrimSpace(p.AccountID) != "" {
				accounts = append(accounts, p.AccountID)
			}
		}
		payload.CustomerAccountIDs = accounts
		if s.propertyRepo != nil {
			if summary, serr := s.propertyRepo.GetSummaryByID(ctx, tenantID, propertyID, d.Type); serr == nil && summary != nil {
				payload.Title = summary.Title
				payload.Neighborhood = summary.Neighborhood
				payload.City = summary.City
				payload.State = summary.State
				payload.ListingType = summary.ListingType
				payload.ImageURL = summary.ImageURL
				if d.Type == deal.TypeRent {
					payload.RentPrice = summary.Price
				} else {
					payload.SalePrice = summary.Price
				}
			}
		}
		evt := contracts.DealRecommendedPropertyEvent{Data: payload}
		if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
			log.Printf("WARN: failed to enqueue deal.recommended_property_set for deal=%s: %v", dealID, err)
		}
	}
	return nil
}

// CreateDealAppointment agenda uma visita pelo operador/corretor para um
// negócio (o cliente acompanha no portal). Reusa o agendamento de calendário.
func (s *DealService) CreateDealAppointment(ctx context.Context, tenantID, dealID string, req portal.CreateAppointmentRequest) (*clients.Appointment, error) {
	d, err := s.repo.GetByID(ctx, tenantID, dealID)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, errors.New("deal not found")
	}
	if strings.TrimSpace(req.Date) == "" || strings.TrimSpace(req.Time) == "" || strings.TrimSpace(req.EndTime) == "" {
		return nil, errors.New("date, time and end_time are required")
	}
	normalizeType := func(raw string) string {
		switch strings.ToUpper(strings.TrimSpace(raw)) {
		case "VISIT", "VISITA":
			return "Visita"
		case "MEETING", "REUNIAO", "REUNIÃO":
			return "Reuniao"
		case "INSPECTION", "VISTORIA":
			return "Vistoria"
		case "SIGNATURE", "ASSINATURA":
			return "Assinatura"
		case "CALL", "LIGACAO", "LIGAÇÃO":
			return "Ligacao"
		default:
			return "Outro"
		}
	}
	tid, err := domain.NewTenantID(d.TenantID)
	if err != nil {
		return nil, fmt.Errorf("deal %s has invalid tenant_id: %w", d.ID, err)
	}
	propertyLabel := ""
	if s.propertyRepo != nil && d.PropertyID != "" {
		if summary, perr := s.propertyRepo.GetSummaryByID(ctx, d.TenantID, d.PropertyID, d.Type); perr == nil && summary != nil {
			propertyLabel = fmt.Sprintf("%s · %s", summary.Title, summary.Address)
		}
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "Visita"
	}
	apptType := normalizeType(req.Type)
	if apptType == "Outro" && title != "" {
		apptType = title
	}
	// Resolve o nome do cliente (comprador/inquilino) a partir das partes do negócio.
	customerName := ""
	for _, p := range d.Parties {
		if p.Role == deal.PartyRoleBuyer || p.Role == deal.PartyRoleTenant {
			customerName = p.Name
			break
		}
	}
	clientReq := clients.CreateAppointmentRequest{
		TenantID:    tid,
		Title:       title,
		Date:        strings.TrimSpace(req.Date),
		Time:        strings.TrimSpace(req.Time),
		EndTime:     strings.TrimSpace(req.EndTime),
		Type:        apptType,
		Priority:    "Media",
		Client:      customerName,
		LeadID:      d.CustomerID,
		DealID:      d.ID,
		PropertyID:  d.PropertyID,
		Property:    propertyLabel,
		Agent:       "Operador",
		AgentID:     d.BrokerID,
		Status:      "Agendado",
		Description: strings.TrimSpace(req.Description),
	}
	return s.calendarClient.CreateAppointment(ctx, clientReq)
}

func (s *DealService) UploadMyChecklistItem(ctx context.Context, accountID, email, name, dealID, itemID, filename, contentType string, size int64, content io.Reader) (*deal.Deal, error) {
	foundDeal, err := s.findMyDeal(ctx, accountID, email, dealID)
	if err != nil {
		return nil, err
	}

	d, err := s.portal().GetByID(ctx, foundDeal.TenantID, foundDeal.ID)
	if err != nil {
		return nil, err
	}

	uploadedBy := strings.TrimSpace(name)
	if uploadedBy == "" {
		uploadedBy = email
	}

	fileURL := ""
	var finalStorageKey string
	if content != nil {
		if s.storage == nil {
			return nil, errors.New("storage service not configured")
		}
		ext := filepath.Ext(filename)
		base := storage.SanitizeFilenameBase(strings.TrimSuffix(filename, ext))
		storageKey := fmt.Sprintf("%s/checklist/%s/%s_%s%s", d.TenantID, d.ID, uuid.New().String()[:8], base, ext)

		// P1-13: o upload do portal passa pelo MESMO antivírus do funil de
		// documentos (fail-closed). Spool em memória: o stream do multipart
		// não é seekable e o scanner + storage precisam dos mesmos bytes.
		if s.virusScanner == nil {
			return nil, fmt.Errorf("%w: virus scanner not configured for uploads", virusscan.ErrScannerUnavailable)
		}
		// Teto do funil: recusa antes de trazer o arquivo para a memória (e o
		// LimitReader cobre o caso do tamanho declarado mentir).
		if err := requireUploadSize(size); err != nil {
			return nil, err
		}
		spooled, err := io.ReadAll(io.LimitReader(content, virusscan.MaxUploadBytes+1))
		if err != nil {
			return nil, fmt.Errorf("failed to read upload stream: %w", err)
		}
		if err := requireUploadSize(int64(len(spooled))); err != nil {
			return nil, err
		}
		if _, err := s.virusScanner.Scan(ctx, bytes.NewReader(spooled)); err != nil {
			if errors.Is(err, virusscan.ErrVirusFound) {
				audit.Record(ctx, audit.Entry{
					Action:     "document.virus_rejected",
					EntityType: "deal",
					EntityID:   d.ID,
					Metadata: map[string]any{
						"filename": filename,
						"error":    err.Error(),
						"tenant":   d.TenantID,
					},
				})
			}
			return nil, fmt.Errorf("virus scan failed for %s: %w", filename, err)
		}
		url, err := s.storage.Upload(ctx, "toca-documents", storageKey, contentType, bytes.NewReader(spooled))
		if err != nil {
			return nil, err
		}
		fileURL = url
		finalStorageKey = storageKey
	}

	now := time.Now()
	found := false
	for i := range d.Checklist {
		if d.Checklist[i].ID == itemID {
			d.Checklist[i].Status = deal.ChecklistUploaded
			d.Checklist[i].RejectionReason = ""
			d.Checklist[i].UploadedAt = &now

			// Always set StorageKey when uploading
			if finalStorageKey != "" {
				d.Checklist[i].StorageKey = finalStorageKey
				d.Checklist[i].FileName = filename
				d.Checklist[i].ContentType = contentType
				d.Checklist[i].FileSize = size
				d.Checklist[i].UploadedBy = uploadedBy

				// Set FileURL dynamically (it might be empty on private S3)
				// We'll expose it directly via gateway
				d.Checklist[i].FileURL = fmt.Sprintf("/api/v1/deals/%s/checklist/%s/download", d.ID, itemID)
			} else if fileURL != "" {
				d.Checklist[i].FileURL = fileURL
			}

			found = true
			break
		}
	}
	if !found {
		return nil, errors.New("checklist item not found")
	}

	d.AddHistory(d.Status, accountID, "Checklist item uploaded via client portal")
	if err := s.portal().Update(ctx, d); err != nil {
		return nil, err
	}

	// Espelha o arquivo no painel de documentos da imobiliária (o upload vive
	// no metadata do negócio; o painel lê deal.documents).
	if item := d.ChecklistItemByID(itemID); item != nil {
		s.mirrorChecklistDocument(ctx, d, item, uploadedBy)
	}

	audit.Record(ctx, audit.Entry{
		Action:     "checklist.uploaded",
		EntityType: "ChecklistItem",
		EntityID:   itemID,
		After:      map[string]any{"status": string(deal.ChecklistUploaded)},
		Metadata: map[string]any{
			"deal_id":      d.ID,
			"file_name":    filename,
			"content_type": contentType,
			"file_size":    size,
			"source":       "client_portal",
			"uploaded_by":  uploadedBy,
		},
	})

	if s.eventBus != nil {
		evt := ClientDocumentUploadedEvent{
			DealID:            d.ID,
			ItemID:            itemID,
			FileName:          filename,
			TenantID:          d.TenantID,
			CustomerAccountID: accountID,
			Timestamp:         time.Now(),
		}
		if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
			log.Printf("WARN: failed to enqueue client.document.uploaded event for deal=%s item=%s: %v", d.ID, itemID, err)
		}
	}

	return d, nil
}

// formSubmissionValues monta o snapshot apenas com os campos pedidos no item,
// usando os valores que o cliente enviou (não o valor posterior da party).
func formSubmissionValues(
	requested []deal.ChecklistRequestedField,
	name, document, birthDate, phone string,
	customFields map[string]string,
) map[string]string {
	values := make(map[string]string, len(requested))
	for _, field := range requested {
		key := strings.TrimSpace(field.Key)
		if key == "" {
			continue
		}
		switch key {
		case "full_name":
			values[key] = strings.TrimSpace(name)
		case "cpf":
			values[key] = strings.TrimSpace(document)
		case "birth_date":
			values[key] = strings.TrimSpace(birthDate)
		case "phone":
			values[key] = strings.TrimSpace(phone)
		default:
			values[key] = strings.TrimSpace(customFields[key])
		}
	}
	return values
}

func (s *DealService) UpdateMyChecklistPartyFields(ctx context.Context, accountID, email, dealID, itemID string, name string, document string, birthDate string, phone string, customFields map[string]string) (*deal.Deal, error) {
	foundDeal, err := s.findMyDeal(ctx, accountID, email, dealID)
	if err != nil {
		return nil, err
	}

	d, err := s.portal().GetByID(ctx, foundDeal.TenantID, foundDeal.ID)
	if err != nil {
		return nil, err
	}

	resolveMyPartyID := func() string {
		if accountID != "" {
			for i := range d.Parties {
				if d.Parties[i].AccountID == accountID {
					return d.Parties[i].ID
				}
			}
		}
		if email != "" {
			for i := range d.Parties {
				if strings.EqualFold(d.Parties[i].Email, email) {
					return d.Parties[i].ID
				}
			}
		}
		if accountID != "" || email != "" {
			for _, s := range d.Signers {
				if accountID != "" && s.AccountID == accountID {
					return s.ID
				}
				if email != "" && strings.EqualFold(s.Email, email) {
					return s.ID
				}
			}
		}
		return ""
	}

	myPartyID := resolveMyPartyID()
	if myPartyID == "" {
		return nil, errors.New("party not found or access denied")
	}

	var item *deal.ChecklistItem
	for i := range d.Checklist {
		if d.Checklist[i].ID == itemID {
			item = &d.Checklist[i]
			break
		}
	}
	if item == nil {
		return nil, errors.New("checklist item not found")
	}
	if item.PartyID == "" || item.PartyID != myPartyID {
		return nil, errors.New("access denied")
	}

	partyIdx := -1
	for i := range d.Parties {
		if d.Parties[i].ID == myPartyID {
			partyIdx = i
			break
		}
	}
	if partyIdx == -1 {
		return nil, errors.New("party not found")
	}

	if strings.TrimSpace(name) != "" {
		d.Parties[partyIdx].Name = strings.TrimSpace(name)
	}
	if strings.TrimSpace(document) != "" {
		d.Parties[partyIdx].Document = strings.TrimSpace(document)
	}
	if strings.TrimSpace(birthDate) != "" {
		d.Parties[partyIdx].BirthDate = strings.TrimSpace(birthDate)
	}
	if strings.TrimSpace(phone) != "" {
		d.Parties[partyIdx].Phone = strings.TrimSpace(phone)
	}
	if customFields != nil && len(customFields) > 0 {
		if d.Parties[partyIdx].CustomFields == nil {
			d.Parties[partyIdx].CustomFields = map[string]string{}
		}
		for k, v := range customFields {
			key := strings.TrimSpace(k)
			val := strings.TrimSpace(v)
			if key == "" {
				continue
			}
			d.Parties[partyIdx].CustomFields[key] = val
		}
	}

	hasField := func(key string) bool {
		switch key {
		case "full_name":
			return strings.TrimSpace(d.Parties[partyIdx].Name) != ""
		case "cpf":
			return strings.TrimSpace(d.Parties[partyIdx].Document) != ""
		case "birth_date":
			return strings.TrimSpace(d.Parties[partyIdx].BirthDate) != ""
		case "phone":
			return strings.TrimSpace(d.Parties[partyIdx].Phone) != ""
		default:
			if d.Parties[partyIdx].CustomFields == nil {
				return false
			}
			return strings.TrimSpace(d.Parties[partyIdx].CustomFields[key]) != ""
		}
	}

	for _, f := range item.RequestedFields {
		if f.Required && strings.TrimSpace(f.Key) != "" && !hasField(strings.TrimSpace(f.Key)) {
			return nil, fmt.Errorf("field '%s' is required", f.Key)
		}
	}

	// Solicitações "apenas preenchimento" (request_mode = form) são
	// cumpridas com os campos preenchidos — sem anexo. Quando todos os
	// campos obrigatórios existirem na parte, o item vai para UPLOADED
	// para o operador aprovar/rejeitar como nos demais.
	if item.RequestMode == deal.RequestModeForm && (item.Status == deal.ChecklistPending || item.Status == deal.ChecklistRejected) {
		allFilled := true
		for _, f := range item.RequestedFields {
			key := strings.TrimSpace(f.Key)
			if key != "" && !hasField(key) {
				allFilled = false
			}
		}
		if allFilled {
			now := time.Now()
			item.Status = deal.ChecklistUploaded
			uploadedBy := email
			if accountID != "" {
				uploadedBy = "account:" + accountID
			}
			item.UploadedBy = uploadedBy
			item.UploadedAt = &now
			item.FileURL = "" // preenchimento puro — não há arquivo
			item.StorageKey = ""
			item.RejectionReason = ""
			item.FormSubmission = &deal.ChecklistFormSubmission{
				Values:          formSubmissionValues(item.RequestedFields, name, document, birthDate, phone, customFields),
				RequestedFields: append([]deal.ChecklistRequestedField(nil), item.RequestedFields...),
				SubmittedBy:     uploadedBy,
				SubmittedAt:     now,
			}
			if d.Parties[partyIdx].FieldProvenance == nil {
				d.Parties[partyIdx].FieldProvenance = map[string]deal.FieldProvenance{}
			}
			markClient := func(key string) {
				d.Parties[partyIdx].FieldProvenance[key] = deal.FieldProvenance{
					Source:    deal.ProvenanceClient,
					UpdatedAt: now,
				}
			}
			if strings.TrimSpace(name) != "" {
				markClient("name")
			}
			if strings.TrimSpace(document) != "" {
				markClient("document")
			}
			if strings.TrimSpace(birthDate) != "" {
				markClient("birth_date")
			}
			if strings.TrimSpace(phone) != "" {
				markClient("phone")
			}
			for key := range customFields {
				if clean := strings.TrimSpace(key); clean != "" {
					markClient(clean)
				}
			}
			d.AddHistory(d.Status, uploadedBy, "Checklist item submitted via form: "+item.Name)
		}
	}

	d.UpdatedAt = time.Now()
	if err := s.portal().Update(ctx, d); err != nil {
		return nil, err
	}

	audit.Record(ctx, audit.Entry{
		Action:     "party.fields_updated",
		EntityType: "ChecklistItem",
		EntityID:   itemID,
		Metadata: map[string]any{
			"deal_id":  d.ID,
			"party_id": myPartyID,
			"source":   "client_portal",
		},
	})

	return d, nil
}

type ChecklistItemUploadURL struct {
	UploadURL   string    `json:"upload_url"`
	Bucket      string    `json:"bucket"`
	StorageKey  string    `json:"storage_key"`
	ExpiresAt   time.Time `json:"expires_at"`
	ContentType string    `json:"content_type"`
	FileName    string    `json:"file_name"`
	FileSize    int64     `json:"file_size"`
}

type AttachmentUploadURL struct {
	UploadURL   string    `json:"upload_url"`
	Bucket      string    `json:"bucket"`
	StorageKey  string    `json:"storage_key"`
	ExpiresAt   time.Time `json:"expires_at"`
	ContentType string    `json:"content_type"`
	FileName    string    `json:"file_name"`
	FileSize    int64     `json:"file_size"`
}

type AttachmentUploadComplete struct {
	StorageKey string `json:"storage_key"`
}

func (s *DealService) GetMyDealMaintenanceAttachmentUploadURL(ctx context.Context, accountID, email, dealID, filename, contentType string, size int64) (*AttachmentUploadURL, error) {
	foundDeal, err := s.findMyDeal(ctx, accountID, email, dealID)
	if err != nil {
		return nil, err
	}
	if s.storage == nil {
		return nil, errors.New("storage service not configured")
	}

	ext := filepath.Ext(filename)
	base := storage.SanitizeFilenameBase(strings.TrimSuffix(filename, ext))

	bucket := "toca-documents"
	storageKey := fmt.Sprintf("%s/maintenance/%s/%s_%s%s", foundDeal.TenantID, foundDeal.ID, uuid.New().String()[:8], base, ext)

	expiry := 15 * time.Minute
	uploadURL, err := s.storage.GetPresignedPutURL(ctx, bucket, storageKey, expiry)
	if err != nil {
		return nil, err
	}

	return &AttachmentUploadURL{
		UploadURL:   uploadURL,
		Bucket:      bucket,
		StorageKey:  storageKey,
		ExpiresAt:   time.Now().Add(expiry),
		ContentType: contentType,
		FileName:    filename,
		FileSize:    size,
	}, nil
}

func (s *DealService) CompleteMyDealMaintenanceAttachmentUpload(ctx context.Context, accountID, email, dealID, storageKey string) (*AttachmentUploadComplete, error) {
	foundDeal, err := s.findMyDeal(ctx, accountID, email, dealID)
	if err != nil {
		return nil, err
	}
	prefix := fmt.Sprintf("%s/maintenance/%s/", foundDeal.TenantID, foundDeal.ID)
	if !strings.HasPrefix(storageKey, prefix) {
		return nil, fmt.Errorf("%w: storage key does not belong to this deal", ErrPortalAccessDenied)
	}
	if err := s.scanStoredUpload(ctx, "toca-documents", storageKey, 0); err != nil {
		return nil, fmt.Errorf("virus scan failed for %s: %w", storageKey, err)
	}
	return &AttachmentUploadComplete{StorageKey: storageKey}, nil
}

func (s *DealService) GetMyTicketAttachmentUploadURL(ctx context.Context, accountID, email, ticketID, filename, contentType string, size int64) (*AttachmentUploadURL, error) {
	t, err := s.GetMyTicket(ctx, accountID, email, ticketID)
	if err != nil {
		return nil, err
	}
	if s.storage == nil {
		return nil, errors.New("storage service not configured")
	}

	ext := filepath.Ext(filename)
	base := storage.SanitizeFilenameBase(strings.TrimSuffix(filename, ext))

	bucket := "toca-documents"
	storageKey := fmt.Sprintf("%s/tickets/%s/%s_%s%s", t.TenantID, ticketID, uuid.New().String()[:8], base, ext)

	expiry := 15 * time.Minute
	uploadURL, err := s.storage.GetPresignedPutURL(ctx, bucket, storageKey, expiry)
	if err != nil {
		return nil, err
	}

	return &AttachmentUploadURL{
		UploadURL:   uploadURL,
		Bucket:      bucket,
		StorageKey:  storageKey,
		ExpiresAt:   time.Now().Add(expiry),
		ContentType: contentType,
		FileName:    filename,
		FileSize:    size,
	}, nil
}

func (s *DealService) CompleteMyTicketAttachmentUpload(ctx context.Context, accountID, email, ticketID, storageKey string) (*AttachmentUploadComplete, error) {
	if _, err := s.GetMyTicket(ctx, accountID, email, ticketID); err != nil {
		return nil, err
	}
	if !strings.Contains(storageKey, "/tickets/"+ticketID+"/") {
		return nil, fmt.Errorf("%w: storage key does not belong to this ticket", ErrPortalAccessDenied)
	}
	if err := s.scanStoredUpload(ctx, "toca-documents", storageKey, 0); err != nil {
		return nil, fmt.Errorf("virus scan failed for %s: %w", storageKey, err)
	}
	return &AttachmentUploadComplete{StorageKey: storageKey}, nil
}

func (s *DealService) GetMyChecklistItemUploadURL(ctx context.Context, accountID, email, dealID, itemID, filename, contentType string, size int64) (*ChecklistItemUploadURL, error) {
	foundDeal, err := s.findMyDeal(ctx, accountID, email, dealID)
	if err != nil {
		return nil, err
	}

	// O portal é cross-tenant e roda sem claims de tenant: a leitura precisa ser
	// a mesma do resto do portal (pool de serviço, autorizada pelo vínculo do
	// signatário). Com o repositório do pool de aplicação a RLS não devolvia a
	// linha e o /upload-url respondia 500 em vez de gerar a URL.
	d, err := s.portal().GetByID(ctx, foundDeal.TenantID, foundDeal.ID)
	if err != nil {
		return nil, err
	}

	found := false
	for i := range d.Checklist {
		if d.Checklist[i].ID == itemID {
			found = true
			break
		}
	}
	if !found {
		return nil, errors.New("checklist item not found")
	}

	if s.storage == nil {
		return nil, errors.New("storage service not configured")
	}

	ext := filepath.Ext(filename)
	base := storage.SanitizeFilenameBase(strings.TrimSuffix(filename, ext))

	bucket := "toca-documents"
	storageKey := fmt.Sprintf("%s/checklist/%s/%s_%s%s", d.TenantID, d.ID, uuid.New().String()[:8], base, ext)

	expiry := 15 * time.Minute
	uploadURL, err := s.storage.GetPresignedPutURL(ctx, bucket, storageKey, expiry)
	if err != nil {
		return nil, err
	}

	return &ChecklistItemUploadURL{
		UploadURL:   uploadURL,
		Bucket:      bucket,
		StorageKey:  storageKey,
		ExpiresAt:   time.Now().Add(expiry),
		ContentType: contentType,
		FileName:    filename,
		FileSize:    size,
	}, nil
}

// checklistStoragePrefix é o prefixo do objeto que o portal gera para o item.
// O storage_key chega do cliente no upload-complete: sem essa checagem um
// cliente poderia apontar o item para o arquivo de outro negócio.
func checklistStoragePrefix(tenantID, dealID string) string {
	return fmt.Sprintf("%s/checklist/%s/", tenantID, dealID)
}

// requireUploadSize recusa cedo (413) arquivo acima do teto do funil de upload,
// antes de ler o corpo inteiro para a memória.
func requireUploadSize(size int64) error {
	if size > virusscan.MaxUploadBytes {
		return fmt.Errorf("%w: %d bytes (limite %d)", virusscan.ErrFileTooLarge, size, virusscan.MaxUploadBytes)
	}
	return nil
}

// scanStoredUpload garante que o objeto enviado direto ao storage (fluxo
// presigned) passa pelo MESMO antivírus do upload server-side (P1-13). Objeto
// infectado é removido do storage e a conclusão falha; scanner indisponível
// também recusa a conclusão (fail-closed), então o item nunca fica marcado como
// enviado sem verificação.
func (s *DealService) scanStoredUpload(ctx context.Context, bucket, storageKey string, size int64) error {
	if err := requireUploadSize(size); err != nil {
		return err
	}
	if s.virusScanner == nil {
		return fmt.Errorf("%w: virus scanner not configured for uploads", virusscan.ErrScannerUnavailable)
	}
	downloader, ok := s.storage.(storage.DownloadService)
	if !ok {
		return fmt.Errorf("%w: storage cannot read back presigned uploads", virusscan.ErrScannerUnavailable)
	}
	reader, _, _, err := downloader.Download(ctx, bucket, storageKey)
	if err != nil {
		return fmt.Errorf("failed to read presigned upload for scanning: %w", err)
	}
	defer reader.Close()

	if _, err := s.virusScanner.Scan(ctx, reader); err != nil {
		if errors.Is(err, virusscan.ErrVirusFound) {
			audit.Record(ctx, audit.Entry{
				Action:     "document.virus_rejected",
				EntityType: "deal",
				EntityID:   storageKey,
				Metadata:   map[string]any{"filename": storageKey, "error": err.Error(), "origin": "presigned_upload"},
			})
			if deleteErr := s.storage.Delete(ctx, bucket, storageKey); deleteErr != nil {
				log.Printf("WARN: failed to delete infected presigned upload %s: %v", storageKey, deleteErr)
			}
		}
		return err
	}
	return nil
}

func (s *DealService) CompleteMyChecklistItemUpload(ctx context.Context, accountID, email, name, dealID, itemID, storageKey, filename, contentType string, size int64) (*deal.Deal, error) {
	foundDeal, err := s.findMyDeal(ctx, accountID, email, dealID)
	if err != nil {
		return nil, err
	}

	d, err := s.portal().GetByID(ctx, foundDeal.TenantID, foundDeal.ID)
	if err != nil {
		return nil, err
	}

	if !strings.HasPrefix(storageKey, checklistStoragePrefix(d.TenantID, d.ID)) {
		return nil, fmt.Errorf("%w: storage key does not belong to this deal", ErrPortalAccessDenied)
	}
	if err := s.scanStoredUpload(ctx, "toca-documents", storageKey, size); err != nil {
		return nil, fmt.Errorf("virus scan failed for %s: %w", filename, err)
	}

	uploadedBy := strings.TrimSpace(name)
	if uploadedBy == "" {
		uploadedBy = email
	}

	now := time.Now()
	found := false
	for i := range d.Checklist {
		if d.Checklist[i].ID == itemID {
			d.Checklist[i].Status = deal.ChecklistUploaded
			d.Checklist[i].RejectionReason = ""
			d.Checklist[i].UploadedAt = &now
			d.Checklist[i].StorageKey = storageKey
			d.Checklist[i].FileName = filename
			d.Checklist[i].ContentType = contentType
			d.Checklist[i].FileSize = size
			d.Checklist[i].UploadedBy = uploadedBy
			d.Checklist[i].FileURL = fmt.Sprintf("/api/v1/deals/%s/checklist/%s/download", d.ID, itemID)
			found = true
			break
		}
	}
	if !found {
		return nil, errors.New("checklist item not found")
	}

	d.AddHistory(d.Status, accountID, "Checklist item uploaded via client portal (presigned)")
	if err := s.portal().Update(ctx, d); err != nil {
		return nil, err
	}

	if item := d.ChecklistItemByID(itemID); item != nil {
		s.mirrorChecklistDocument(ctx, d, item, uploadedBy)
	}

	if s.eventBus != nil {
		evt := ClientDocumentUploadedEvent{
			DealID:            d.ID,
			ItemID:            itemID,
			FileName:          filename,
			TenantID:          d.TenantID,
			CustomerAccountID: accountID,
			Timestamp:         time.Now(),
		}
		if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
			log.Printf("WARN: failed to enqueue client.document.uploaded event (presigned) for deal=%s item=%s: %v", d.ID, itemID, err)
		}
	}

	return d, nil
}

// PortalUploadChecklistItem marks an item as uploaded (used by portal without full auth).
func (s *DealService) PortalUploadChecklistItem(ctx context.Context, tenantID, dealID, itemID string) (*deal.Deal, error) {
	d, err := s.portal().GetByID(ctx, tenantID, dealID)
	if err != nil {
		return nil, err
	}
	d.UpdateChecklistItemStatus(itemID, deal.ChecklistUploaded, "")
	d.AddHistory(d.Status, "portal", "Document uploaded via client portal")
	if err := s.portal().Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}
