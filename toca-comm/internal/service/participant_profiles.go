package service

import (
	"context"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-comm/internal/domain/message"
	"log"
)

type participantProfileRepository interface {
	GetParticipantProfiles(context.Context, string, []string) ([]message.ParticipantProfile, error)
}

func (s *MessageService) participantProfiles(ctx context.Context, tenantID string, ids []string) []message.ParticipantProfile {
	repo, ok := s.repo.(participantProfileRepository)
	if !ok {
		return nil
	}
	profiles, err := repo.GetParticipantProfiles(pkgcontext.WithTenantID(ctx, tenantID), tenantID, ids)
	if err != nil {
		log.Printf("Unable to load chat participant profiles: %v", err)
		return nil
	}
	return profiles
}

func applySenderProfile(msg *message.Message, profiles []message.ParticipantProfile) {
	if msg == nil {
		return
	}
	for _, p := range profiles {
		if p.ID != msg.SenderID {
			continue
		}
		if p.Name != "" {
			msg.SenderName = p.Name
		}
		if p.AvatarURL != "" {
			msg.SenderAvatarURL = p.AvatarURL
		}
		return
	}
}

func (s *MessageService) enrichChannels(ctx context.Context, channels []*message.Channel) {
	byTenant := map[string][]string{}
	for _, ch := range channels {
		if ch != nil {
			byTenant[ch.TenantID] = append(byTenant[ch.TenantID], ch.Participants...)
		}
	}
	for tenantID, ids := range byTenant {
		unique := []string{}
		seen := map[string]bool{}
		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				unique = append(unique, id)
			}
		}
		profiles := s.participantProfiles(ctx, tenantID, unique)
		for _, ch := range channels {
			if ch == nil || ch.TenantID != tenantID {
				continue
			}
			ch.ParticipantProfiles = nil
			for _, id := range ch.Participants {
				for _, p := range profiles {
					if p.ID == id {
						ch.ParticipantProfiles = append(ch.ParticipantProfiles, p)
						break
					}
				}
			}
			applySenderProfile(ch.LastMessage, ch.ParticipantProfiles)
		}
	}
}
