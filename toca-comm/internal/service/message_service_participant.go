package service

import (
	"context"
	"errors"
	"fmt"

	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-comm/internal/domain/message"
)

// ErrParticipantNotInTenant — o account informado não é conhecido no tenant do
// canal (nem como lead, nem como corretor). Ver AddParticipant.
var ErrParticipantNotInTenant = errors.New("participant does not belong to this tenant")

// AddParticipant adds a new participant to a channel.
//
// SECURITY (achado A9 da revisão de permissões de 2026-09-19): a versão anterior
// só exigia que o CHAMADOR fosse membro do canal e aceitava qualquer
// `participant_id` — o UPDATE era sem predicado de tenant. Qualquer membro
// adicionava uma conta de OUTRO tenant (um cúmplice) e ela passava a ler o
// histórico completo, porque a leitura tem a participação como único controle.
//
// Agora: (a) o canal é resolvido com tenant + participação do chamador; (b) o
// novo participante precisa pertencer ao tenant do canal (lead por id/AccountID
// ou corretor — as mesmas fontes que resolvem perfis de participantes); (c) a
// escrita carrega o tenant do canal resolvido.
//
// Identidade (incidente "lead convidado não vê a mensagem do corretor",
// 2026-09-21): o painel da imobiliária adiciona o LEAD, mas o portal autentica
// pela CONTA do cliente. O participante é traduzido para a conta quando ela já
// existe (canonicalParticipant); sem conta (cliente ainda não convidado) o lead
// é aceito e reconhecido depois, na autorização.
func (s *MessageService) AddParticipant(ctx context.Context, channelID, participantID string) error {
	tenantID, err := pkgcontext.GetTenantID(ctx)
	if err != nil {
		return errors.New("tenant ID is required")
	}
	if participantID == "" {
		return errors.New("participant_id is required")
	}

	ch, err := s.resolveChannel(ctx, channelID, tenantID)
	if err != nil {
		return err
	}
	if err := s.checkParticipant(ctx, ch); err != nil {
		return err
	}

	canonicalID, err := s.canonicalParticipant(ctx, ch.TenantID, participantID)
	if err != nil {
		return err
	}

	allowed, err := s.repo.AccountBelongsToTenant(ctx, ch.TenantID, canonicalID)
	if err != nil {
		return fmt.Errorf("failed to validate participant: %w", err)
	}
	if !allowed {
		return fmt.Errorf("%w: %s", ErrParticipantNotInTenant, participantID)
	}

	// Check if already in channel — nas duas identidades: com o lead já presente
	// (canal anterior ao convite) e com a conta, a adição é no-op depois da cura.
	for _, p := range ch.Participants {
		if p == canonicalID {
			return nil // Already a participant
		}
		if p == participantID && canonicalID != participantID {
			return nil
		}
	}

	ch.Participants = append(ch.Participants, canonicalID)
	err = s.repo.UpdateChannelParticipants(ctx, channelID, ch.TenantID, ch.Participants)
	if err != nil {
		return err
	}

	if s.hub != nil {
		// Broadcast to all participants (including the new one)
		s.hub.BroadcastEvent(ch.TenantID, ch.Participants, "participant_added", map[string]interface{}{
			"channel_id":     channelID,
			"participant_id": canonicalID,
		})
		// For the new participant, they also need to know the channel exists
		s.hub.BroadcastEvent(ch.TenantID, []string{canonicalID}, "channel_created", ch)
	}

	return nil
}

// RemoveParticipant removes a participant from a channel.
//
// A remoção continua sendo uma operação de membro do canal (o corretor tira um
// participante do canal do negócio), mas agora com o tenant do canal resolvido
// no predicado da escrita — antes o UPDATE era sem tenant (achado A9).
func (s *MessageService) RemoveParticipant(ctx context.Context, channelID, participantID string) error {
	tenantID, err := pkgcontext.GetTenantID(ctx)
	if err != nil {
		return errors.New("tenant ID is required")
	}

	ch, err := s.resolveChannel(ctx, channelID, tenantID)
	if err != nil {
		return err
	}
	if err := s.checkParticipant(ctx, ch); err != nil {
		return err
	}

	var newParticipants []string
	for _, p := range ch.Participants {
		if p != participantID {
			newParticipants = append(newParticipants, p)
		}
	}

	if len(newParticipants) != len(ch.Participants) {
		err = s.repo.UpdateChannelParticipants(ctx, channelID, ch.TenantID, newParticipants)
		if err != nil {
			return err
		}
		if s.hub != nil {
			// Notify remaining participants
			if len(newParticipants) > 0 {
				s.hub.BroadcastEvent(ch.TenantID, newParticipants, "participant_removed", map[string]interface{}{
					"channel_id":     channelID,
					"participant_id": participantID,
				})
			}
			// Notify the removed participant to close the channel
			s.hub.BroadcastEvent(ch.TenantID, []string{participantID}, "channel_removed", map[string]interface{}{
				"channel_id": channelID,
			})
		}
		return nil
	}

	return nil
}

// UpdateChannelStatus updates the status of a channel
func (s *MessageService) UpdateChannelStatus(ctx context.Context, channelID string, status message.ChannelStatus) error {
	tenantID, err := pkgcontext.GetTenantID(ctx)
	if err != nil {
		return errors.New("tenant ID is required")
	}

	ch, err := s.resolveChannel(ctx, channelID, tenantID)
	if err != nil {
		return err
	}
	if err := s.checkParticipant(ctx, ch); err != nil {
		return err
	}

	if err := s.repo.UpdateChannelStatus(ctx, channelID, ch.TenantID, status); err != nil {
		return err
	}

	if s.hub != nil {
		s.hub.BroadcastEvent(ch.TenantID, ch.Participants, "channel_updated", map[string]interface{}{
			"channel_id": channelID,
			"status":     status,
		})
	}
	return nil
}
