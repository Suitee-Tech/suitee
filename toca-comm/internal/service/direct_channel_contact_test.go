package service

import (
	"context"
	"errors"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-comm/internal/domain/message"
	"reflect"
	"testing"
)

type contactRepo struct {
	*a9Repo
	found        *message.Channel
	findErr      error
	tenant       string
	participants []string
	created      []*message.Channel
}

func (r *contactRepo) FindDirectChannel(_ context.Context, tenant string, participants []string) (*message.Channel, error) {
	r.tenant = tenant
	r.participants = participants
	return r.found, r.findErr
}
func (r *contactRepo) CreateChannel(_ context.Context, ch *message.Channel) error {
	r.created = append(r.created, ch)
	return nil
}

func TestDirectContactSharesCanonicalParticipantsAndReusesHistory(t *testing.T) {
	for _, fromPanel := range []bool{false, true} {
		old := canalNomeadoPeloLead()
		repo := &contactRepo{a9Repo: repoDoIncidente(old), found: old}
		svc := NewMessageService(repo, nil, nil, nil)
		ctx := pkgcontext.WithTenantID(pkgcontext.WithAccountID(context.Background(), corretorDoCanal), tenantDoCanal)
		var ch *message.Channel
		var err error
		if fromPanel {
			ch, err = svc.CreateChannel(ctx, "Cliente", message.ChannelTypeDirect, []string{leadDoCliente, contaDoCliente}, "", "")
		} else {
			ch, err = svc.EnsureDirectChannel(ctx, tenantDoCanal, "Cliente", []string{leadDoCliente, contaDoCliente})
		}
		if err != nil || ch.ID != old.ID || len(repo.created) != 0 {
			t.Fatalf("history not reused: %v %+v", err, ch)
		}
		if repo.tenant != tenantDoCanal || !reflect.DeepEqual(repo.participants, []string{contaDoCliente, corretorDoCanal}) {
			t.Fatalf("wrong scope/identity: %s %v", repo.tenant, repo.participants)
		}
		if repo.replacedTo != contaDoCliente {
			t.Fatal("legacy participant not healed")
		}
	}
}
func TestDirectContactDoesNotCreateOnLookupFailure(t *testing.T) {
	repo := &contactRepo{a9Repo: repoDoIncidente(nil), findErr: errors.New("database unavailable")}
	svc := NewMessageService(repo, nil, nil, nil)
	if _, err := svc.EnsureDirectChannel(context.Background(), tenantDoCanal, "Cliente", []string{contaDoCliente, corretorDoCanal}); err == nil {
		t.Fatal("failure hidden")
	}
	if len(repo.created) != 0 {
		t.Fatal("created duplicate after failed lookup")
	}
}

type scopedContactRepo struct {
	*a9Repo
	readTenant, writeTenant string
}

func (r *scopedContactRepo) GetMessages(ctx context.Context, channelID, tenantID string, limit, offset int) ([]*message.Message, int, error) {
	r.readTenant, _ = pkgcontext.GetTenantID(ctx)
	return nil, 0, nil
}
func (r *scopedContactRepo) SaveMessage(ctx context.Context, msg *message.Message) error {
	r.writeTenant, _ = pkgcontext.GetTenantID(ctx)
	return nil
}
func TestPortalContactReadsAndWritesInAuthorizedChannelTenant(t *testing.T) {
	ch := canalNomeadoPeloLead()
	ch.Participants = []string{corretorDoCanal, contaDoCliente}
	repo := &scopedContactRepo{a9Repo: repoDoIncidente(ch)}
	svc := NewMessageService(repo, nil, nil, nil)
	ctx := pkgcontext.WithTenantID(pkgcontext.WithAccountID(context.Background(), contaDoCliente), contaDoCliente)
	if _, _, err := svc.GetMessages(ctx, ch.ID, 50, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SendMessage(ctx, ch.ID, "teste"); err != nil {
		t.Fatal(err)
	}
	if repo.readTenant != tenantDoCanal || repo.writeTenant != tenantDoCanal {
		t.Fatalf("wrong database scope: read=%s write=%s", repo.readTenant, repo.writeTenant)
	}
	if original, _ := pkgcontext.GetTenantID(ctx); original != contaDoCliente {
		t.Fatal("changed caller scope")
	}
}

func TestDealContactReusesReferencedChannelBeforeAddingClient(t *testing.T) {
	existing := &message.Channel{ID: "old-deal-channel", TenantID: tenantDoCanal, Type: message.ChannelTypeDeal, Participants: []string{corretorDoCanal}}
	repo := &stubMessageRepo{getByRef: func(tenant, ref, kind string) (*message.Channel, error) {
		if tenant != tenantDoCanal || ref != "deal" || kind != "deal" {
			t.Fatalf("wrong reference %s %s %s", tenant, ref, kind)
		}
		return existing, nil
	}, createCh: func(*message.Channel) error { t.Fatal("created a second deal conversation"); return nil }}
	svc := NewMessageService(repo, nil, nil, nil)
	for i := 0; i < 2; i++ {
		ch, err := svc.EnsureChannelForDeal(context.Background(), tenantDoCanal, "deal", []string{contaDoCliente, corretorDoCanal})
		if err != nil || ch.ID != existing.ID {
			t.Fatalf("wrong channel %+v %v", ch, err)
		}
	}
	if repo.updateParticipantsCalls != 1 || len(existing.Participants) != 2 {
		t.Fatalf("membership not idempotent: %+v", repo)
	}
}

func TestPanelCanCreateSeparateDraftsBeforeChoosingAClient(t *testing.T) {
	repo := &contactRepo{a9Repo: repoDoIncidente(nil), found: &message.Channel{ID: "old-empty-draft"}}
	svc := NewMessageService(repo, nil, nil, nil)
	ctx := pkgcontext.WithTenantID(pkgcontext.WithAccountID(context.Background(), corretorDoCanal), tenantDoCanal)
	for _, name := range []string{"Atendimento A", "Atendimento B"} {
		ch, err := svc.CreateChannel(ctx, name, message.ChannelTypeDirect, nil, "", "")
		if err != nil || ch.Name != name {
			t.Fatalf("draft was incorrectly reused: %+v %v", ch, err)
		}
	}
	if len(repo.created) != 2 {
		t.Fatal("panel drafts were merged before selecting a client")
	}
}
