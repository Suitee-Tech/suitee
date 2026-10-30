package service

import (
	"context"
	"errors"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-comm/internal/domain/message"
	"testing"
)

type profileMessageRepo struct {
	*portalMessageMutationRepo
	profileTenant string
	profileIDs    []string
	failProfiles  bool
}

func (r *profileMessageRepo) GetParticipantProfiles(_ context.Context, tenant string, ids []string) ([]message.ParticipantProfile, error) {
	r.profileTenant = tenant
	r.profileIDs = ids
	if r.failProfiles {
		return nil, errors.New("profiles unavailable")
	}
	return []message.ParticipantProfile{
		{ID: "portal-client", Name: "Lucas Cliente", AvatarURL: "/lucas.png"},
		{ID: "operator-1", Name: "Juliana Cabral", AvatarURL: "/juliana.png"},
	}, nil
}
func (r *profileMessageRepo) GetMessages(context.Context, string, string, int, int) ([]*message.Message, int, error) {
	return []*message.Message{r.msg}, 1, nil
}

func TestPortalChatProfilesUseChannelTenantAndRestoreHistoricalSender(t *testing.T) {
	repo := &profileMessageRepo{portalMessageMutationRepo: newPortalMessageMutationRepo()}
	repo.msg.SenderID = "operator-1"
	repo.msg.SenderName = "operator-1"
	svc := NewMessageService(repo, nil, nil, nil)
	ctx := pkgcontext.WithAccountID(pkgcontext.WithTenantID(context.Background(), "client-tenant"), "portal-client")
	channel, err := svc.GetChannel(ctx, repo.channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if repo.profileTenant != "company-tenant" || len(channel.ParticipantProfiles) != 2 {
		t.Fatalf("profiles use wrong tenant or are missing: %q %#v", repo.profileTenant, channel.ParticipantProfiles)
	}
	msgs, _, err := svc.GetMessages(ctx, channel.ID, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if msgs[0].SenderName != "Juliana Cabral" || msgs[0].SenderAvatarURL != "/juliana.png" {
		t.Fatalf("historical broker identity missing: %#v", msgs[0])
	}
	sent, err := svc.SendMessage(ctx, channel.ID, "Olá")
	if err != nil {
		t.Fatal(err)
	}
	if sent.SenderName != "Lucas Cliente" || sent.SenderAvatarURL != "/lucas.png" {
		t.Fatalf("new client identity missing: %#v", sent)
	}
}

func TestChatProfilesDoNotResolveUnauthorizedChannels(t *testing.T) {
	repo := &profileMessageRepo{portalMessageMutationRepo: newPortalMessageMutationRepo()}
	svc := NewMessageService(repo, nil, nil, nil)
	ctx := pkgcontext.WithAccountID(pkgcontext.WithTenantID(context.Background(), "client-tenant"), "outsider")
	if _, err := svc.GetChannel(ctx, repo.channel.ID); err == nil {
		t.Fatal("expected denied access")
	}
	if repo.profileTenant != "" {
		t.Fatal("resolved identities before authorizing channel access")
	}
}

func TestChatStillLoadsWhenProfilesAreUnavailable(t *testing.T) {
	repo := &profileMessageRepo{portalMessageMutationRepo: newPortalMessageMutationRepo(), failProfiles: true}
	svc := NewMessageService(repo, nil, nil, nil)
	ctx := pkgcontext.WithAccountID(pkgcontext.WithTenantID(context.Background(), "client-tenant"), "portal-client")
	if _, _, err := svc.GetMessages(ctx, repo.channel.ID, 50, 0); err != nil {
		t.Fatal(err)
	}
}
