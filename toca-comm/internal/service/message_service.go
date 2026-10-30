package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"regexp"
	"time"

	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/storage"
	"github.com/oginoino/toca/toca-comm/internal/domain/message"
	"github.com/oginoino/toca/toca-comm/internal/repository"
)

// MessageService handles channel and message operations.
type MessageService struct {
	repo         repository.MessageRepository
	hub          *Hub
	storage      storage.Service
	inAppService *InAppNotificationService
}

func NewMessageService(repo repository.MessageRepository, hub *Hub, storageService storage.Service, inAppService *InAppNotificationService) *MessageService {
	return &MessageService{
		repo:         repo,
		hub:          hub,
		storage:      storageService,
		inAppService: inAppService,
	}
}

// Hub returns the WebSocket hub for event broadcasting
func (s *MessageService) Hub() *Hub {
	return s.hub
}

func attachmentDownloadPath(channelID, attachmentID string) string {
	return "/api/v1/channels/" + url.PathEscape(channelID) + "/messages/attachments/" + url.PathEscape(attachmentID) + "/download"
}

// CreateChannel creates a new messaging channel for a tenant
func (s *MessageService) CreateChannel(ctx context.Context, name string, channelType message.ChannelType, participants []string, entityID, entityType string) (*message.Channel, error) {
	tenantID, err := pkgcontext.GetTenantID(ctx)
	if err != nil {
		return nil, errors.New("tenant ID is required")
	}
	senderID, _ := pkgcontext.GetAccountID(ctx)

	// O painel escolhe o LEAD do cliente; o canal guarda a CONTA (identidade com
	// que o portal autentica). Ver canonicalParticipant.
	participants, err = s.canonicalParticipants(ctx, tenantID, participants)
	if err != nil {
		return nil, err
	}

	// Ensure sender is in participants
	hasSender := false
	for _, p := range participants {
		if p == senderID {
			hasSender = true
			break
		}
	}
	if !hasSender {
		participants = append(participants, senderID)
	}

	if channelType == message.ChannelTypeDirect && len(participants) > 1 {
		return s.EnsureDirectChannel(ctx, tenantID, name, participants)
	}

	ch := message.NewChannel(tenantID, name, channelType, senderID, participants)
	ch.EntityID = entityID
	ch.EntityType = entityType

	if err := s.repo.CreateChannel(ctx, ch); err != nil {
		return nil, err
	}

	if s.hub != nil && len(ch.Participants) > 0 {
		s.hub.BroadcastEvent(tenantID, ch.Participants, "channel_created", ch)
	}

	return ch, nil
}

func (s *MessageService) EnsureChannelForDeal(ctx context.Context, tenantID, dealID string, participants []string) (*message.Channel, error) {
	if tenantID == "" || dealID == "" {
		return nil, errors.New("tenant and deal IDs are required")
	}
	ctx = pkgcontext.WithTenantID(ctx, tenantID)
	participants, err := s.canonicalParticipants(ctx, tenantID, participants)
	if err != nil {
		return nil, err
	}

	// A tenant's channel list is participant-scoped; an empty account cannot
	// find the deal's channel. Resolve its explicit reference instead.
	existing, err := s.repo.GetChannelByReference(ctx, tenantID, dealID, "deal")
	if err != nil && !errors.Is(err, message.ErrChannelNotFound) {
		return nil, err
	}
	if existing != nil {
		changed := false
		present := make(map[string]bool)
		for _, p := range existing.Participants {
			present[p] = true
		}
		for _, p := range participants {
			if !present[p] {
				existing.Participants = append(existing.Participants, p)
				present[p] = true
				changed = true
			}
		}
		if changed {
			if err := s.repo.UpdateChannelParticipants(ctx, existing.ID, tenantID, existing.Participants); err != nil {
				return nil, err
			}
			if s.hub != nil {
				s.hub.BroadcastEvent(tenantID, existing.Participants, "channel_created", existing)
			}
		}
		return existing, nil
	}

	// Create new channel
	name := "Negociação " + dealID
	ch := message.NewChannel(tenantID, name, message.ChannelTypeDeal, "system", participants)
	ch.EntityID = dealID
	ch.EntityType = "deal"

	if err := s.repo.CreateChannel(ctx, ch); err != nil {
		return nil, err
	}

	if s.hub != nil {
		s.hub.BroadcastEvent(tenantID, ch.Participants, "channel_created", ch)
	}

	return ch, nil
}

// EnsureChannelForDealWithInfo is like EnsureChannelForDeal but also sets
// initial info sections on a newly-created channel (no-op on existing ones).
func (s *MessageService) EnsureChannelForDealWithInfo(ctx context.Context, tenantID, dealID string, participants []string, info *message.ChannelInfo) (*message.Channel, error) {
	ch, err := s.EnsureChannelForDeal(ctx, tenantID, dealID, participants)
	if err != nil {
		return nil, err
	}

	if info != nil && len(info.Sections) > 0 && ch.Info == nil {
		ch.Info = info
		updates := map[string]interface{}{"info": info}
		return s.repo.UpdateChannel(ctx, ch.ID, tenantID, updates)
	}

	return ch, nil
}

// EnsureChannelForSupport — Issue #452. Cria (ou retorna existente) um
// canal de suporte vinculado a um contrato. Disparado automaticamente
// no consumer de `deal.contract.signed`: o cliente acabou de assinar,
// queremos um canal aberto onde ele consiga falar com a plataforma sem
// precisar criar nada manualmente.
//
// Idempotência: usa GetChannelByReference (refType="contract") como
// chave de unicidade. Eventos redelivered não duplicam o canal — o
// índice é refID+refType na tabela. Quando já existe, faz merge dos
// participants (mesma lógica do EnsureChannelForDeal) para o caso de
// callbacks com participantes diferentes (cliente principal + fiador,
// p.ex.).
//
// CreatedBy = "system" porque a operação parte do consumer, não de um
// usuário. Esse é o mesmo marcador usado por EnsureChannelForDeal.
func (s *MessageService) EnsureChannelForSupport(ctx context.Context, tenantID, contractID string, participants []string) (*message.Channel, error) {
	if tenantID == "" || contractID == "" {
		return nil, errors.New("tenantID and contractID are required")
	}

	if existing, err := s.repo.GetChannelByReference(ctx, tenantID, contractID, "contract"); err == nil && existing != nil {
		// Merge participants idempotentemente — o caller pode mandar uma
		// lista maior em redeliveries (ex.: fiador adicionado depois).
		current := make(map[string]bool, len(existing.Participants))
		for _, p := range existing.Participants {
			current[p] = true
		}
		updated := false
		for _, p := range participants {
			if p == "" || current[p] {
				continue
			}
			existing.Participants = append(existing.Participants, p)
			current[p] = true
			updated = true
		}
		if updated {
			_ = s.repo.UpdateChannelParticipants(ctx, existing.ID, existing.TenantID, existing.Participants)
		}
		return existing, nil
	}

	// Dedup participants antes de salvar — payload pode trazer ids
	// repetidos (mesmo cliente listado como signer e como contato).
	dedupParticipants := make([]string, 0, len(participants))
	seen := make(map[string]bool, len(participants))
	for _, p := range participants {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		dedupParticipants = append(dedupParticipants, p)
	}

	name := "Suporte Contrato " + contractID
	ch := message.NewChannel(tenantID, name, message.ChannelTypeSupport, "system", dedupParticipants)
	ch.EntityID = contractID
	ch.EntityType = "contract"

	if err := s.repo.CreateChannel(ctx, ch); err != nil {
		return nil, err
	}

	// Broadcast para os participantes em real-time: o canal de suporte
	// aparece imediatamente na lista do cliente sem precisar refetch.
	if s.hub != nil && len(ch.Participants) > 0 {
		s.hub.BroadcastEvent(tenantID, ch.Participants, "channel_created", ch)
	}

	return ch, nil
}

// EnsureDirectChannel acha ou cria um canal direto (1-a-1) entre os
// participantes (ex. cliente + corretor). Se já existir um canal "direct"
// com exatamente os mesmos participantes (em qualquer ordem), retorna ele.
func (s *MessageService) EnsureDirectChannel(ctx context.Context, tenantID string, name string, participants []string) (*message.Channel, error) {
	if tenantID == "" {
		return nil, errors.New("tenant ID is required")
	}
	ctx = pkgcontext.WithTenantID(ctx, tenantID)
	var err error
	participants, err = s.canonicalParticipants(ctx, tenantID, participants)
	if err != nil {
		return nil, err
	}
	if len(participants) == 0 {
		return nil, errors.New("participants are required")
	}
	senderID, _ := pkgcontext.GetAccountID(ctx)
	hasSender := false
	for _, p := range participants {
		if p == senderID {
			hasSender = true
			break
		}
	}
	if !hasSender && senderID != "" {
		participants = append(participants, senderID)
	}

	// Reutiliza o canal direct já existente entre os MESMOS participantes
	// (mesmo corretor + mesmo cliente) — não cria um canal por sessão.
	existing, err := s.repo.FindDirectChannel(ctx, tenantID, participants)
	if err == nil && existing != nil {
		for _, p := range existing.Participants {
			canonical, resolveErr := s.canonicalParticipant(ctx, tenantID, p)
			if resolveErr != nil {
				return nil, resolveErr
			}
			if canonical != p {
				s.healParticipantIdentity(ctx, existing, p, canonical)
			}
		}
		s.enrichChannels(ctx, []*message.Channel{existing})
		return existing, nil
	}
	if err != nil && !errors.Is(err, message.ErrChannelNotFound) {
		return nil, err
	}

	ch := message.NewChannel(tenantID, name, message.ChannelTypeDirect, senderID, participants)
	ch.EntityType = "direct"
	if err := s.repo.CreateChannel(ctx, ch); err != nil {
		return nil, err
	}
	if s.hub != nil && len(ch.Participants) > 0 {
		s.hub.BroadcastEvent(tenantID, ch.Participants, "channel_created", ch)
	}
	return ch, nil
}

// ListChannelsByParticipant lista os canais de um participante.
//
// SECURITY (achados A3/A9 da revisão de permissões de 2026-09-19): tenantID
// vazio = sem recorte de tenant, permitido apenas quando a autorização É a
// própria participação (chamador pedindo os próprios canais — o cliente do
// portal tem tenant no token igual à própria conta e participa de canais do
// tenant do negócio). Quem decide isso é o handler, a partir da credencial.
//
// O recorte por participação usa o CONJUNTO de identidades do cliente (conta +
// leads vinculados, ver participantIdentities): o canal aberto pelo painel nomeia
// o lead, e o portal autentica pela conta.
func (s *MessageService) ListChannelsByParticipant(ctx context.Context, tenantID, accountID string) ([]*message.Channel, error) {
	identities, err := s.participantIdentities(ctx, tenantID, accountID)
	if err != nil {
		return nil, err
	}
	channels, err := s.repo.ListChannelsByParticipant(ctx, tenantID, identities)
	if err != nil {
		return nil, err
	}
	for _, ch := range channels {
		for _, p := range ch.Participants {
			for _, identity := range identities {
				if p == identity && p != accountID {
					s.healParticipantIdentity(ctx, ch, p, accountID)
					break
				}
			}
		}
	}

	s.enrichChannels(ctx, channels)
	return channels, nil
}

// participantIdentities devolve todas as identidades do cliente DENTRO do
// tenant: a conta do portal (com que ele autentica) e os leads vinculados a ela
// (com que o painel da imobiliária o nomeia).
//
// tenantID vazio (cliente do portal, cujo token não carrega o tenant da
// imobiliária) devolve a conta e os leads dela em QUALQUER tenant: continuam
// sendo as identidades do próprio chamador, que é o que a autorização por
// participação exige.
func (s *MessageService) participantIdentities(ctx context.Context, tenantID, accountID string) ([]string, error) {
	identities := []string{accountID}
	if accountID == "" {
		return identities, nil
	}
	leads, err := s.repo.LeadIDsForAccount(ctx, tenantID, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve participant identities: %w", err)
	}
	for _, leadID := range leads {
		if leadID != "" && leadID != accountID {
			identities = append(identities, leadID)
		}
	}
	return identities, nil
}

// canonicalParticipant traduz a identidade escolhida pelo painel (lead) para a
// identidade com que o cliente autentica (conta do portal).
//
// O corretor abre a conversa pelo LEAD — é o id que a busca de participantes
// devolve — mas quem lê/escreve no portal é a CONTA (`lead.AccountID`). Gravar o
// lead deixava o canal invisível para o próprio cliente (incidente 2026-09-21).
// Sem conta vinculada (cliente ainda não convidado) o lead é mantido como
// participante e `checkParticipant` reconhece o dono quando o convite acontece.
func (s *MessageService) canonicalParticipant(ctx context.Context, tenantID, participantID string) (string, error) {
	if tenantID == "" || participantID == "" {
		return participantID, nil
	}
	accountID, err := s.repo.ClientAccountForLead(ctx, tenantID, participantID)
	if err != nil {
		return "", fmt.Errorf("failed to resolve participant account: %w", err)
	}
	if accountID == "" {
		return participantID, nil
	}
	return accountID, nil
}

// canonicalParticipants aplica canonicalParticipant a uma lista, sem duplicar.
func (s *MessageService) canonicalParticipants(ctx context.Context, tenantID string, participants []string) ([]string, error) {
	out := make([]string, 0, len(participants))
	seen := make(map[string]bool, len(participants))
	for _, p := range participants {
		canonical, err := s.canonicalParticipant(ctx, tenantID, p)
		if err != nil {
			return nil, err
		}
		if canonical == "" || seen[canonical] {
			continue
		}
		seen[canonical] = true
		out = append(out, canonical)
	}
	return out, nil
}

// healParticipantIdentity troca, no canal, a identidade antiga (lead) pela
// conta do cliente autenticado — a mesma tradução da escrita, aplicada aos
// canais que nasceram antes dela.
//
// Só é chamada quando a autorização foi provada pela vinculação
// (lead.AccountID = conta do chamador), então a troca não amplia acesso: ela
// apenas normaliza o participante para o id que o portal usa para listar,
// enviar e receber em tempo real. Falha aqui não bloqueia a operação — a
// leitura seguinte tenta de novo.
func (s *MessageService) healParticipantIdentity(ctx context.Context, ch *message.Channel, leadID, accountID string) {
	if ch == nil || leadID == "" || accountID == "" || ch.TenantID == "" {
		return
	}
	if err := s.repo.ReplaceChannelParticipant(pkgcontext.WithTenantID(ctx, ch.TenantID), ch.ID, ch.TenantID, leadID, accountID); err != nil {
		// 0 linhas = o canal não nomeia mais o lead (cura concorrente ou já
		// feita): a cópia em memória ainda precisa acompanhar o banco.
		if !errors.Is(err, message.ErrChannelNotFound) {
			logger.Warn("comm.participant_identity_heal_failed",
				"event", "comm.participant_identity_heal_failed",
				"channel_id", ch.ID, "lead_id", leadID, "account_id", accountID, "error", err.Error())
			return
		}
	}

	// A cópia em memória alimenta o broadcast desta chamada (envio de mensagem,
	// por exemplo): sem atualizá-la o cliente continuaria fora do fan-out.
	healed := make([]string, 0, len(ch.Participants))
	seen := make(map[string]bool, len(ch.Participants))
	for _, p := range ch.Participants {
		if p == leadID {
			p = accountID
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		healed = append(healed, p)
	}
	ch.Participants = healed
}

// checkParticipant exige que o account do contexto seja participante do canal.
//
// SECURITY (achado A9 da revisão de permissões de 2026-09-19): a versão anterior
// devolvia `nil` (autorizado) quando não havia account id no contexto —
// "system call or no user context". Como estes métodos só são alcançados por
// rotas de USUÁRIO (`/api/v1/channels/:id/...`), account id vazio é anomalia ou
// bug, nunca chamada de sistema: agora falha fechado. Os fluxos de serviço
// (criação/garantia de canal vinda do toca-deal e do toca-financial) não passam
// por aqui.
//
// A participação é decidida pelo CONJUNTO de identidades do chamador: o canal
// aberto pelo painel nomeia o lead do cliente, e o portal autentica pela conta.
// As duas são o mesmo principal (`crm.leads.AccountID` = conta autenticada), e a
// autorização por lead é curada na hora (healParticipantIdentity) para que o
// canal volte a nomear a conta. Continuam de fora, como antes, os canais de
// outros clientes.
func (s *MessageService) checkParticipant(ctx context.Context, ch *message.Channel) error {
	userID, _ := pkgcontext.GetAccountID(ctx)
	if userID == "" {
		return errors.New("authenticated user is required to access a channel")
	}

	for _, p := range ch.Participants {
		if p == userID {
			return nil
		}
	}

	if ch.TenantID == "" {
		return errors.New("user is not a participant of this channel")
	}
	leadIDs, err := s.repo.LeadIDsForAccount(ctx, ch.TenantID, userID)
	if err != nil {
		// Falha fechado: sem saber quem é o chamador não se autoriza.
		return fmt.Errorf("failed to resolve participant identities: %w", err)
	}
	for _, leadID := range leadIDs {
		for _, p := range ch.Participants {
			if p == leadID {
				s.healParticipantIdentity(ctx, ch, leadID, userID)
				return nil
			}
		}
	}

	return errors.New("user is not a participant of this channel")
}

// GetChannel retrieves a channel by ID, scoped to the tenant
func (s *MessageService) GetChannel(ctx context.Context, channelID string) (*message.Channel, error) {
	tenantID, err := pkgcontext.GetTenantID(ctx)
	// Remove the strict tenant check here, let checkParticipant decide if they can view it.
	if err != nil {
		tenantID = ""
	}

	ch, err := s.resolveChannel(ctx, channelID, tenantID)
	if err != nil {
		return nil, err
	}

	if err := s.checkParticipant(ctx, ch); err != nil {
		return nil, err
	}
	ctx = pkgcontext.WithTenantID(ctx, ch.TenantID)

	// Self-healing: ensure creator is in participants
	hasCreator := false
	for _, p := range ch.Participants {
		if p == ch.CreatedBy {
			hasCreator = true
			break
		}
	}
	if !hasCreator && ch.CreatedBy != "" {
		ch.Participants = append(ch.Participants, ch.CreatedBy)
		go func(tenantID string, participants []string) {
			_ = s.repo.UpdateChannelParticipants(context.Background(), channelID, tenantID, participants)
		}(ch.TenantID, ch.Participants)
	}

	s.enrichChannels(ctx, []*message.Channel{ch})
	return ch, nil
}

// ListChannels returns all channels for the current tenant, optionally filtered by entity
func (s *MessageService) ListChannels(ctx context.Context, entityID string) ([]*message.Channel, error) {
	tenantID, err := pkgcontext.GetTenantID(ctx)
	if err != nil {
		return nil, errors.New("tenant ID is required")
	}
	userID, _ := pkgcontext.GetAccountID(ctx)

	channels, err := s.repo.ListChannels(ctx, tenantID, userID, entityID)
	if err != nil {
		return nil, err
	}

	// Self-healing: ensure creator is in participants
	// Skip "system" — it's not a real user and shouldn't be added as participant.
	for _, ch := range channels {
		hasCreator := false
		for _, p := range ch.Participants {
			if p == ch.CreatedBy {
				hasCreator = true
				break
			}
		}
		if !hasCreator && ch.CreatedBy != "" && ch.CreatedBy != "system" {
			ch.Participants = append(ch.Participants, ch.CreatedBy)
			go func(chID, tenantID string, participants []string) {
				_ = s.repo.UpdateChannelParticipants(context.Background(), chID, tenantID, participants)
			}(ch.ID, ch.TenantID, ch.Participants)
		}
	}

	s.enrichChannels(ctx, channels)
	return channels, nil
}

// UpdateChannel updates channel metadata (name, description)
func (s *MessageService) UpdateChannel(ctx context.Context, channelID string, updates map[string]interface{}) (*message.Channel, error) {
	tenantID, err := pkgcontext.GetTenantID(ctx)
	if err != nil {
		return nil, errors.New("tenant ID is required")
	}

	ch, err := s.resolveChannel(ctx, channelID, tenantID)
	if err != nil {
		return nil, err
	}

	if err := s.checkParticipant(ctx, ch); err != nil {
		return nil, err
	}
	ctx = pkgcontext.WithTenantID(ctx, ch.TenantID)

	updated, err := s.repo.UpdateChannel(ctx, channelID, ch.TenantID, updates)
	if err != nil {
		return nil, err
	}

	if s.hub != nil {
		s.hub.BroadcastEvent(ch.TenantID, ch.Participants, "channel_updated", map[string]interface{}{
			"channel_id": channelID,
		})
	}

	return updated, nil
}

// EditMessage edits the content of an existing message
func (s *MessageService) EditMessage(ctx context.Context, channelID, messageID, content string) (*message.Message, error) {
	tenantID, err := pkgcontext.GetTenantID(ctx)
	if err != nil {
		return nil, errors.New("tenant ID is required")
	}
	senderID, _ := pkgcontext.GetAccountID(ctx)

	ch, err := s.resolveChannel(ctx, channelID, tenantID)
	if err != nil {
		return nil, err
	}

	if err := s.checkParticipant(ctx, ch); err != nil {
		return nil, err
	}
	ctx = pkgcontext.WithTenantID(ctx, ch.TenantID)

	msg, err := s.repo.GetMessage(ctx, messageID, ch.TenantID, senderID)
	if err != nil {
		return nil, err
	}

	if msg.SenderID != senderID {
		return nil, errors.New("only the sender can edit this message")
	}

	if err := s.repo.UpdateMessageContent(ctx, messageID, ch.TenantID, senderID, content); err != nil {
		return nil, err
	}

	msg.Content = content
	msg.IsEdited = true

	if s.hub != nil {
		s.hub.BroadcastEvent(ch.TenantID, ch.Participants, "message_edited", map[string]interface{}{
			"channel_id": channelID,
			"message_id": messageID,
			"content":    content,
		})
	}

	return msg, nil
}

// DeleteMessage deletes a message from a channel
func (s *MessageService) DeleteMessage(ctx context.Context, channelID, messageID string) error {
	tenantID, err := pkgcontext.GetTenantID(ctx)
	if err != nil {
		return errors.New("tenant ID is required")
	}
	senderID, _ := pkgcontext.GetAccountID(ctx)

	ch, err := s.resolveChannel(ctx, channelID, tenantID)
	if err != nil {
		return err
	}

	if err := s.checkParticipant(ctx, ch); err != nil {
		return err
	}
	ctx = pkgcontext.WithTenantID(ctx, ch.TenantID)

	msg, err := s.repo.GetMessage(ctx, messageID, ch.TenantID, senderID)
	if err != nil {
		return err
	}

	if msg.SenderID != senderID {
		return errors.New("only the sender can delete this message")
	}

	if err := s.repo.DeleteMessage(ctx, messageID, ch.TenantID, senderID); err != nil {
		return err
	}

	if s.hub != nil {
		s.hub.BroadcastEvent(ch.TenantID, ch.Participants, "message_deleted", map[string]interface{}{
			"channel_id": channelID,
			"message_id": messageID,
		})
	}

	return nil
}

// SendMessage adds a message to a channel

// resolveChannel busca o canal pelo tenant do chamador e, quando o chamador tem
// tenant diferente do canal (cliente do portal, cujo `tenant_id` no token é a
// própria conta), cai para a busca por id — que EXIGE participação no próprio
// SQL (`GetChannelForParticipant`).
//
// SECURITY (achado A9 da revisão de permissões de 2026-09-19): antes o fallback
// era `GetChannelByID` (busca crua por id) e acontecia em QUALQUER erro de
// `GetChannel` — inclusive falha de banco —, com a participação validada só
// quando havia `senderID`. Agora:
//
//   - só há fallback quando o canal realmente não existe para aquele tenant
//     (`message.ErrChannelNotFound`); erro de infraestrutura sobe;
//   - o account do contexto é sempre exigido, e o repositório filtra por
//     participação — um id de canal adivinhado não devolve nada.
func (s *MessageService) resolveChannel(ctx context.Context, channelID, tenantID string) (*message.Channel, error) {
	ch, err := s.repo.GetChannel(ctx, channelID, tenantID)
	if err == nil {
		return ch, nil
	}
	if !errors.Is(err, message.ErrChannelNotFound) {
		return nil, err
	}

	accountID, _ := pkgcontext.GetAccountID(ctx)
	if accountID == "" {
		return nil, message.ErrChannelNotFound
	}
	// Aqui o tenant do canal ainda é desconhecido (é o caso do cliente do
	// portal, cujo token não carrega o tenant da imobiliária): as identidades
	// são resolvidas sem recorte de tenant, restritas ao próprio chamador.
	identities, err := s.participantIdentities(ctx, "", accountID)
	if err != nil {
		return nil, err
	}
	return s.repo.GetChannelForParticipant(ctx, channelID, identities)
}

func (s *MessageService) SendMessage(ctx context.Context, channelID, content string) (*message.Message, error) {
	tenantID, err := pkgcontext.GetTenantID(ctx)
	if err != nil {
		tenantID = ""
	}
	senderID, _ := pkgcontext.GetAccountID(ctx)

	ch, err := s.resolveChannel(ctx, channelID, tenantID)
	if err != nil {
		return nil, err
	}
	if err := s.checkParticipant(ctx, ch); err != nil {
		return nil, err
	}
	ctx = pkgcontext.WithTenantID(ctx, ch.TenantID)
	if ch.Status == message.ChannelStatusClosed {
		return nil, errors.New("channel is closed")
	}

	// Sempre usa o tenant do CANAL para a mensagem — o chamador (ex. cliente
	// do portal) pode ter um tenant no token diferente do canal, o que faria
	// a mensagem sumir na listagem (que filtra por tenant do canal).
	tenantID = ch.TenantID

	// Get sender name if provided in context, else fallback to ID
	senderName := senderID
	if name, ok := ctx.Value(CtxSenderName).(string); ok && name != "" {
		senderName = name
	}

	msg := message.NewMessage(channelID, tenantID, senderID, senderName, content)

	// Add sender avatar URL if provided in context
	if avatarURL, ok := ctx.Value(CtxSenderAvatarURL).(string); ok && avatarURL != "" {
		msg.SenderAvatarURL = avatarURL
	}

	applySenderProfile(msg, s.participantProfiles(ctx, ch.TenantID, []string{senderID}))
	// Add support for replying
	if replyToID, ok := ctx.Value(CtxReplyToID).(string); ok && replyToID != "" {
		msg.ReplyToID = replyToID
	}

	// Add support for content_type (e.g., file/image)
	if contentType, ok := ctx.Value(CtxContentType).(string); ok && contentType != "" {
		msg.ContentType = contentType
	}

	// Add support for attachments
	if attachments, ok := ctx.Value(CtxAttachments).([]message.Attachment); ok && len(attachments) > 0 {
		msg.Attachments = attachments
	}

	// Execute Slash Commands
	if len(content) > 0 && content[0] == '/' {
		// Command execution usually replaces the message sending
		// In a real app we would check permissions and execute actions
		return s.handleSlashCommand(ctx, channelID, content, msg)
	}

	// Extract Mentions and Hashtags
	msg.Mentions = s.extractMentions(content)
	msg.Hashtags = s.extractHashtags(content)

	// Moderation check
	if s.isMessageFlagged(content) {
		msg.IsFlagged = true
	}

	if err := s.repo.SaveMessage(ctx, msg); err != nil {
		return nil, err
	}

	// Trigger notifications for mentions
	if len(msg.Mentions) > 0 && s.hub != nil {
		for _, userID := range msg.Mentions {
			s.hub.triggerWebhook(tenantID, userID, msg)
		}
	}

	// Broadcast the new message via WebSockets to all channel participants
	if s.hub != nil && len(ch.Participants) > 0 {
		s.hub.BroadcastMessage(tenantID, ch.Participants, msg)
	}

	// Create in-app notifications for other participants
	if s.inAppService != nil {
		for _, p := range ch.Participants {
			if p != senderID {
				_ = s.inAppService.Create(
					ctx,
					tenantID,
					p,
					"chat_message", // notification.InAppTypeChatMessage
					"normal",
					"Nova mensagem",
					senderName+": "+content,
					"/messages/"+channelID,
					map[string]interface{}{
						"channel_id": channelID,
						"message_id": msg.ID,
					},
				)
			}
		}
	}

	// Update last message on channel
	if err := s.repo.UpdateChannelLastMessage(ctx, ch.TenantID, channelID, msg); err != nil {
		// Log error but don't fail message send
		log.Printf("Failed to update last message for channel %s: %v", channelID, err)
	}

	return msg, nil
}

// GetMessages returns paginated messages for a channel
func (s *MessageService) GetMessages(ctx context.Context, channelID string, limit, offset int) ([]*message.Message, int, error) {
	tenantID, err := pkgcontext.GetTenantID(ctx)
	if err != nil {
		tenantID = ""
	}

	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	// Ensure channel exists and belongs to tenant (fallback por participação)
	ch, err := s.resolveChannel(ctx, channelID, tenantID)
	if err != nil {
		return nil, 0, err
	}
	if err := s.checkParticipant(ctx, ch); err != nil {
		return nil, 0, err
	}
	ctx = pkgcontext.WithTenantID(ctx, ch.TenantID)

	// Usa sempre o tenant do canal p/ listar (chamador pode ter tenant diferente).
	tenantID = ch.TenantID

	// The repository now returns newest first. Let's reverse them to chronological for the UI.
	msgs, total, err := s.repo.GetMessages(ctx, channelID, tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	// Reverse to return oldest first (chronological)
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}

	profiles := s.participantProfiles(ctx, ch.TenantID, ch.Participants)
	for _, msg := range msgs {
		applySenderProfile(msg, profiles)
		for i := range msg.Attachments {
			msg.Attachments[i].URL = attachmentDownloadPath(channelID, msg.Attachments[i].ID)
		}
	}

	return msgs, total, nil
}

// MarkRead marks messages in a channel as read by the calling user
func (s *MessageService) MarkRead(ctx context.Context, channelID string) error {
	tenantID, err := pkgcontext.GetTenantID(ctx)
	if err != nil {
		tenantID = ""
	}
	userID, _ := pkgcontext.GetAccountID(ctx)

	// Ensure channel exists (fallback por id p/ cliente com tenant divergente)
	ch, err := s.resolveChannel(ctx, channelID, tenantID)
	if err != nil {
		return err
	}
	if err := s.checkParticipant(ctx, ch); err != nil {
		return err
	}
	ctx = pkgcontext.WithTenantID(ctx, ch.TenantID)

	tenantID = ch.TenantID
	if err := s.repo.MarkMessagesAsRead(ctx, channelID, tenantID, userID); err != nil {
		return err
	}
	if s.hub != nil {
		s.hub.BroadcastEvent(tenantID, ch.Participants, "messages_read", map[string]interface{}{
			"channel_id": channelID,
			"user_id":    userID,
		})
	}
	return nil
}

// AddReaction adds an emoji reaction to a message
func (s *MessageService) AddReaction(ctx context.Context, channelID, messageID, emoji string) error {
	tenantID, err := pkgcontext.GetTenantID(ctx)
	if err != nil {
		return errors.New("tenant ID is required")
	}
	userID, _ := pkgcontext.GetAccountID(ctx)

	ch, err := s.resolveChannel(ctx, channelID, tenantID)
	if err != nil {
		return err
	}
	if err := s.checkParticipant(ctx, ch); err != nil {
		return err
	}
	ctx = pkgcontext.WithTenantID(ctx, ch.TenantID)

	msg, err := s.repo.GetMessage(ctx, messageID, ch.TenantID, userID)
	if err != nil {
		return err
	}

	var found bool
	for i, r := range msg.Reactions {
		if r.Emoji == emoji {
			found = true
			// Check if user already reacted
			for _, uid := range r.UserIDs {
				if uid == userID {
					return nil // Already reacted
				}
			}
			msg.Reactions[i].UserIDs = append(msg.Reactions[i].UserIDs, userID)
			break
		}
	}

	if !found {
		msg.Reactions = append(msg.Reactions, message.Reaction{
			Emoji:   emoji,
			UserIDs: []string{userID},
		})
	}

	if err := s.repo.UpdateMessageReactions(ctx, messageID, ch.TenantID, userID, msg.Reactions); err != nil {
		return err
	}

	if s.hub != nil {
		s.hub.BroadcastEvent(ch.TenantID, ch.Participants, "reaction_added", map[string]interface{}{
			"channel_id": channelID,
			"message_id": messageID,
			"emoji":      emoji,
			"user_id":    userID,
		})
	}

	return nil
}

// CreateTemplate creates a new message template
func (s *MessageService) CreateTemplate(ctx context.Context, name, category, content string, variables []string) (*message.Template, error) {
	tenantID, err := pkgcontext.GetTenantID(ctx)
	if err != nil {
		return nil, errors.New("tenant ID is required")
	}
	userID, _ := pkgcontext.GetAccountID(ctx)

	tmpl := message.NewTemplate(tenantID, name, category, content, variables, userID)
	if err := s.repo.CreateTemplate(ctx, tmpl); err != nil {
		return nil, err
	}

	return tmpl, nil
}

// UploadAttachment handles file upload and returns an attachment object
func (s *MessageService) UploadAttachment(ctx context.Context, channelID string, filename string, displayName string, contentType string, size int64, content io.Reader) (*message.Attachment, error) {
	tenantID, err := pkgcontext.GetTenantID(ctx)
	if err != nil {
		return nil, errors.New("tenant ID is required")
	}

	// Verify channel exists
	ch, err := s.resolveChannel(ctx, channelID, tenantID)
	if err != nil {
		return nil, err
	}
	if err := s.checkParticipant(ctx, ch); err != nil {
		return nil, err
	}
	ctx = pkgcontext.WithTenantID(ctx, ch.TenantID)

	// Upload to storage
	bucket := "toca-message-attachments"
	_, err = s.storage.Upload(ctx, bucket, filename, contentType, content)
	if err != nil {
		return nil, err
	}

	attachment := &message.Attachment{
		ID:   filename, // Could use a UUID here
		Name: displayName,
		URL:  attachmentDownloadPath(channelID, filename),
		Type: contentType,
		Size: size,
	}

	return attachment, nil
}

func (s *MessageService) GetAttachmentDownloadURL(ctx context.Context, channelID, attachmentID string) (string, error) {
	tenantID, err := pkgcontext.GetTenantID(ctx)
	if err != nil {
		tenantID = ""
	}

	ch, err := s.resolveChannel(ctx, channelID, tenantID)
	if err != nil {
		return "", err
	}
	if err := s.checkParticipant(ctx, ch); err != nil {
		return "", err
	}
	ctx = pkgcontext.WithTenantID(ctx, ch.TenantID)

	storageURL, err := s.storage.GetPresignedURL(ctx, "toca-message-attachments", attachmentID, 15*time.Minute)
	if err != nil {
		return "", err
	}
	return storageURL, nil
}
func (s *MessageService) ListTemplates(ctx context.Context, category string) ([]*message.Template, error) {
	tenantID, err := pkgcontext.GetTenantID(ctx)
	if err != nil {
		return nil, errors.New("tenant ID is required")
	}

	return s.repo.ListTemplates(ctx, tenantID, category)
}

// GetMetrics returns chat metrics for the dashboard
func (s *MessageService) GetMetrics(ctx context.Context) (*message.Metrics, error) {
	tenantID, err := pkgcontext.GetTenantID(ctx)
	if err != nil {
		return nil, errors.New("tenant ID is required")
	}

	return s.repo.GetMetrics(ctx, tenantID)
}

// SearchMessages performs a global search across channels
func (s *MessageService) SearchMessages(ctx context.Context, query, channelID, hashtag, mention string, limit, offset int) ([]*message.Message, int, error) {
	tenantID, err := pkgcontext.GetTenantID(ctx)
	if err != nil {
		return nil, 0, errors.New("tenant ID is required")
	}

	if limit <= 0 {
		limit = 50
	}

	userID, _ := pkgcontext.GetAccountID(ctx)

	var channelIDs []string
	effectiveTenantID := tenantID
	if channelID != "" {
		// Validate that the user is in this specific channel
		ch, err := s.resolveChannel(ctx, channelID, tenantID)
		if err != nil {
			return nil, 0, err
		}
		if err := s.checkParticipant(ctx, ch); err != nil {
			return nil, 0, err
		}
		ctx = pkgcontext.WithTenantID(ctx, ch.TenantID)
		effectiveTenantID = ch.TenantID
		channelIDs = []string{channelID}
	} else {
		// Get all channels for user
		channels, err := s.repo.ListChannels(ctx, tenantID, userID, "")
		if err != nil {
			return nil, 0, err
		}
		for _, ch := range channels {
			channelIDs = append(channelIDs, ch.ID)
		}
		if len(channelIDs) == 0 {
			return make([]*message.Message, 0), 0, nil
		}
	}

	return s.repo.SearchMessages(ctx, effectiveTenantID, userID, channelIDs, query, hashtag, mention, limit, offset)
}

// TogglePinMessage pins or unpins a message
func (s *MessageService) TogglePinMessage(ctx context.Context, channelID, messageID string, pin bool) error {
	tenantID, err := pkgcontext.GetTenantID(ctx)
	if err != nil {
		return errors.New("tenant ID is required")
	}

	userID, _ := pkgcontext.GetAccountID(ctx)
	ch, err := s.resolveChannel(ctx, channelID, tenantID)
	if err != nil {
		return err
	}
	if err := s.checkParticipant(ctx, ch); err != nil {
		return err
	}
	ctx = pkgcontext.WithTenantID(ctx, ch.TenantID)

	if err := s.repo.UpdateMessagePinStatus(ctx, messageID, ch.TenantID, userID, pin); err != nil {
		return err
	}

	// Broadcast the update
	if s.hub != nil {
		eventType := "message_pinned"
		if !pin {
			eventType = "message_unpinned"
		}
		s.hub.BroadcastEvent(ch.TenantID, ch.Participants, eventType, map[string]interface{}{
			"channel_id": channelID,
			"message_id": messageID,
			"is_pinned":  pin,
		})
	}

	return nil
}
func (s *MessageService) isMessageFlagged(content string) bool {
	// Simple mock implementation
	// In production, this would use a more sophisticated algorithm or external service
	blockedWords := []string{"idiota", "merda", "burro", "golpe", "fraude"}
	for _, word := range blockedWords {
		if contains(content, word) {
			return true
		}
	}
	return false
}

func (s *MessageService) extractMentions(content string) []string {
	re := regexp.MustCompile(`@([a-zA-Z0-9_-]+)`)
	matches := re.FindAllStringSubmatch(content, -1)

	mentions := make([]string, 0)
	seen := make(map[string]bool)

	for _, match := range matches {
		if len(match) > 1 {
			id := match[1]
			if !seen[id] {
				seen[id] = true
				mentions = append(mentions, id)
			}
		}
	}
	return mentions
}

func (s *MessageService) extractHashtags(content string) []string {
	re := regexp.MustCompile(`#([a-zA-Z0-9_]+)`)
	matches := re.FindAllStringSubmatch(content, -1)

	tags := make([]string, 0)
	seen := make(map[string]bool)

	for _, match := range matches {
		if len(match) > 1 {
			tag := match[1]
			if !seen[tag] {
				seen[tag] = true
				tags = append(tags, tag)
			}
		}
	}
	return tags
}

// TogglePinChannel pins or unpins a channel for the current user
func (s *MessageService) TogglePinChannel(ctx context.Context, channelID string, pin bool) error {
	tenantID, err := pkgcontext.GetTenantID(ctx)
	if err != nil {
		return errors.New("tenant ID is required")
	}
	userID, _ := pkgcontext.GetAccountID(ctx)

	ch, err := s.resolveChannel(ctx, channelID, tenantID)
	if err != nil {
		return err
	}
	if err := s.checkParticipant(ctx, ch); err != nil {
		return err
	}
	ctx = pkgcontext.WithTenantID(ctx, ch.TenantID)

	return s.repo.UpdateChannelPinned(ctx, channelID, ch.TenantID, userID, pin)
}

// ToggleMuteChannel mutes or unmutes a channel for the current user
func (s *MessageService) ToggleMuteChannel(ctx context.Context, channelID string, mute bool) error {
	tenantID, err := pkgcontext.GetTenantID(ctx)
	if err != nil {
		return errors.New("tenant ID is required")
	}
	userID, _ := pkgcontext.GetAccountID(ctx)

	ch, err := s.resolveChannel(ctx, channelID, tenantID)
	if err != nil {
		return err
	}
	if err := s.checkParticipant(ctx, ch); err != nil {
		return err
	}
	ctx = pkgcontext.WithTenantID(ctx, ch.TenantID)

	return s.repo.UpdateChannelMuted(ctx, channelID, ch.TenantID, userID, mute)
}

func (s *MessageService) handleSlashCommand(ctx context.Context, channelID, content string, msg *message.Message) (*message.Message, error) {
	// Parse command
	parts := regexp.MustCompile(`\s+`).Split(content, 2)
	cmd := parts[0]

	// Create system message to show command result
	msg.ContentType = "system"

	switch cmd {
	case "/encerrar":
		if err := s.UpdateChannelStatus(ctx, channelID, message.ChannelStatusClosed); err != nil {
			return nil, err
		}
		msg.Content = "Canal encerrado pelo usuário."
	case "/arquivar":
		// Assuming we had an archived status
		if err := s.UpdateChannelStatus(ctx, channelID, "archived"); err != nil {
			return nil, err
		}
		msg.Content = "Canal arquivado pelo usuário."
	case "/ping":
		msg.Content = "Pong! O sistema está online."
	default:
		msg.Content = "Comando não reconhecido: " + cmd
	}

	if err := s.repo.SaveMessage(ctx, msg); err != nil {
		return nil, err
	}

	// Broadcast
	ch, _ := s.GetChannel(ctx, channelID)
	if s.hub != nil && ch != nil {
		s.hub.BroadcastMessage(msg.TenantID, ch.Participants, msg)
	}

	return msg, nil
}

func contains(s, substr string) bool {
	// Simple contains check (case-insensitive in a real app)
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// RemoveReaction removes an emoji reaction from a message
func (s *MessageService) RemoveReaction(ctx context.Context, channelID, messageID, emoji string) error {
	tenantID, err := pkgcontext.GetTenantID(ctx)
	if err != nil {
		return errors.New("tenant ID is required")
	}
	userID, _ := pkgcontext.GetAccountID(ctx)

	ch, err := s.resolveChannel(ctx, channelID, tenantID)
	if err != nil {
		return err
	}
	if err := s.checkParticipant(ctx, ch); err != nil {
		return err
	}
	ctx = pkgcontext.WithTenantID(ctx, ch.TenantID)

	tenantID = ch.TenantID
	msg, err := s.repo.GetMessage(ctx, messageID, tenantID, userID)
	if err != nil {
		return err
	}
	if msg.ChannelID != channelID {
		return errors.New("message not found")
	}
	if msg.ChannelID != channelID {
		return errors.New("message not found")
	}

	for i, r := range msg.Reactions {
		if r.Emoji == emoji {
			var newUsers []string
			for _, uid := range r.UserIDs {
				if uid != userID {
					newUsers = append(newUsers, uid)
				}
			}
			msg.Reactions[i].UserIDs = newUsers

			// Remove reaction entirely if no users left
			if len(newUsers) == 0 {
				msg.Reactions = append(msg.Reactions[:i], msg.Reactions[i+1:]...)
			}
			break
		}
	}

	if err := s.repo.UpdateMessageReactions(ctx, messageID, tenantID, userID, msg.Reactions); err != nil {
		return err
	}

	if s.hub != nil {
		s.hub.BroadcastEvent(ch.TenantID, ch.Participants, "reaction_removed", map[string]interface{}{
			"channel_id": channelID,
			"message_id": messageID,
			"emoji":      emoji,
			"user_id":    userID,
		})
	}

	return nil
}

// SendTyping broadcasts a typing event to channel participants
func (s *MessageService) SendTyping(ctx context.Context, channelID string) error {
	tenantID, err := pkgcontext.GetTenantID(ctx)
	if err != nil {
		return errors.New("tenant ID is required")
	}
	userID, _ := pkgcontext.GetAccountID(ctx)
	senderName := userID
	if name, ok := ctx.Value(CtxSenderName).(string); ok && name != "" {
		senderName = name
	}

	ch, err := s.resolveChannel(ctx, channelID, tenantID)
	if err != nil {
		return err
	}
	if err := s.checkParticipant(ctx, ch); err != nil {
		return err
	}
	ctx = pkgcontext.WithTenantID(ctx, ch.TenantID)

	if s.hub != nil {
		s.hub.BroadcastEvent(ch.TenantID, ch.Participants, "typing", map[string]interface{}{
			"channel_id":  channelID,
			"user_id":     userID,
			"sender_name": senderName,
		})
	}

	return nil
}
