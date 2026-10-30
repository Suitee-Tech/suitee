package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/oginoino/toca/pkg/events"
	pkgevents "github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-signature/internal/clicksign"
	"github.com/oginoino/toca/toca-signature/internal/domain/envelope"
	"github.com/oginoino/toca/toca-signature/internal/service"
)

// recordingBus guarda o que foi publicado — os testes abaixo provam que o
// cancelamento avisa o toca-deal (é o evento que destrava o contrato) em vez de
// só mudar o status local.
type recordingBus struct {
	published []events.Event
	failWith  error
}

func (b *recordingBus) Publish(_ context.Context, event events.Event) error {
	if b.failWith != nil {
		return b.failWith
	}
	b.published = append(b.published, event)
	return nil
}

func (b *recordingBus) PublishReliable(ctx context.Context, event events.Event) error {
	return b.Publish(ctx, event)
}

func (b *recordingBus) Subscribe(string, events.EventHandler) error { return nil }

func (b *recordingBus) SubscribeWithQueue(string, string, events.EventHandler) error { return nil }

func (b *recordingBus) refused() []pkgevents.SignatureRefusedEvent {
	var out []pkgevents.SignatureRefusedEvent
	for _, evt := range b.published {
		if refused, ok := evt.(pkgevents.SignatureRefusedEvent); ok {
			out = append(out, refused)
		}
	}
	return out
}

func runningEnvelope(t *testing.T, repo *fakeRepo) *envelope.SignatureEnvelope {
	t.Helper()
	env := mustNewEnvelope(t, "tenant-1", "contract-1", []envelope.Signer{
		{SignerID: "sig-1", Name: "n", Email: "e@x", Role: envelope.SignerRoleSign, Auth: []envelope.AuthMethod{envelope.AuthEmailToken}},
	})
	env.Status = envelope.StatusRunning
	env.ExternalEnvelopeID = "cs-env-1"
	repo.byID[env.ID] = env
	return env
}

// O defeito de 2026-10-05: o operador cancelava, o envelope ficava CANCELED do
// nosso lado e o toca-deal nunca sabia — o contrato seguia em
// PENDING_SIGNATURE, travado para reenvio, sem botão na tela. Cancelar precisa
// publicar o mesmo evento que o webhook `cancel` publica.
func TestCancel_PublishesRefusedSoContractIsReleased(t *testing.T) {
	repo := newFakeRepo()
	env := runningEnvelope(t, repo)
	bus := &recordingBus{}
	svc := service.NewEnvelopeService(repo, &fakeClicksign{}, &stubCreds{}, &fakeDeal{contract: sampleContract()}, &stubPDF{}, bus)

	got, err := svc.Cancel(context.Background(), "tenant-1", env.ID)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if got.Status != envelope.StatusCanceled {
		t.Errorf("status = %s, want CANCELED", got.Status)
	}
	if got.TerminalEventPublishedAt == nil {
		t.Error("TerminalEventPublishedAt vazio: a varredura não tem como saber que o desfecho já foi anunciado")
	}
	refusals := bus.refused()
	if len(refusals) != 1 {
		t.Fatalf("eventos signature.refused = %d, want 1", len(refusals))
	}
	if refusals[0].Data.ContractID != env.ContractID {
		t.Errorf("contract_id = %s, want %s", refusals[0].Data.ContractID, env.ContractID)
	}
	if refusals[0].Data.ExternalEnvelopeID != env.ExternalEnvelopeID {
		t.Errorf("external_envelope_id = %s, want %s", refusals[0].Data.ExternalEnvelopeID, env.ExternalEnvelopeID)
	}
	if refusals[0].Data.SignerID != "" {
		t.Errorf("signer_id = %q, want vazio (cancelamento não é recusa de um signatário)", refusals[0].Data.SignerID)
	}
}

func TestCancel_IsIdempotentAndDoesNotRepublish(t *testing.T) {
	repo := newFakeRepo()
	env := runningEnvelope(t, repo)
	bus := &recordingBus{}
	svc := service.NewEnvelopeService(repo, &fakeClicksign{}, &stubCreds{}, &fakeDeal{contract: sampleContract()}, &stubPDF{}, bus)

	if _, err := svc.Cancel(context.Background(), "tenant-1", env.ID); err != nil {
		t.Fatalf("primeiro Cancel: %v", err)
	}
	stored := repo.byID[env.ID]
	if _, err := svc.Cancel(context.Background(), "tenant-1", stored.ID); err != nil {
		t.Fatalf("segundo Cancel: %v", err)
	}
	if refusals := bus.refused(); len(refusals) != 1 {
		t.Errorf("eventos signature.refused = %d, want 1 (o segundo cancelamento não republica)", len(refusals))
	}
}

// Cancelar com o provedor recusando NÃO pode marcar o envelope como cancelado:
// o link de assinatura continua válido e a tela diria "cancelado". Antes desta
// correção a falha era engolida em last_error e a API devolvia sucesso.
func TestCancel_ProviderFailureDoesNotLieAboutStatus(t *testing.T) {
	repo := newFakeRepo()
	env := runningEnvelope(t, repo)
	bus := &recordingBus{}
	cs := &fakeClicksign{cancelErr: errors.New("clicksign PATCH ... status=400")}
	svc := service.NewEnvelopeService(repo, cs, &stubCreds{}, &fakeDeal{contract: sampleContract()}, &stubPDF{}, bus)

	got, err := svc.Cancel(context.Background(), "tenant-1", env.ID)
	if err == nil {
		t.Fatal("Cancel devolveu sucesso com o provedor recusando o cancelamento")
	}
	if !errors.Is(err, service.ErrCancelProviderFailed) {
		t.Errorf("erro = %v, want ErrCancelProviderFailed (o handler traduz para 502)", err)
	}
	if got != nil {
		t.Errorf("envelope devolvido = %v, want nil", got)
	}
	stored := repo.byID[env.ID]
	if stored.Status != envelope.StatusRunning {
		t.Errorf("status local = %s, want RUNNING (o link segue válido no provedor)", stored.Status)
	}
	if stored.LastError == "" {
		t.Error("last_error vazio: o operador não tem como saber por que o cancelamento falhou")
	}
	if refusals := bus.refused(); len(refusals) != 0 {
		t.Errorf("eventos signature.refused = %d, want 0 (o contrato não pode ser liberado com o link vivo)", len(refusals))
	}
}

// O estado preso exato do incidente: envelope já CANCELED local (o operador
// tentou antes) e o desfecho nunca anunciado. Clicar cancelar de novo precisa
// curar, não devolver "já cancelado".
func TestCancel_HealsCanceledEnvelopeNeverAnnounced(t *testing.T) {
	repo := newFakeRepo()
	env := runningEnvelope(t, repo)
	env.Status = envelope.StatusCanceled
	env.TerminalEventPublishedAt = nil
	repo.byID[env.ID] = env

	bus := &recordingBus{}
	svc := service.NewEnvelopeService(repo, &fakeClicksign{getStatus: "canceled"}, &stubCreds{}, &fakeDeal{contract: sampleContract()}, &stubPDF{}, bus)

	got, err := svc.Cancel(context.Background(), "tenant-1", env.ID)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if got.TerminalEventPublishedAt == nil {
		t.Error("TerminalEventPublishedAt continua vazio: o contrato segue travado")
	}
	if refusals := bus.refused(); len(refusals) != 1 {
		t.Fatalf("eventos signature.refused = %d, want 1", len(refusals))
	}
}

// A reconciliação é a rede de segurança: envelope cancelado no provedor e não
// anunciado aqui precisa publicar o desfecho mesmo já estando CANCELED local.
// Antes, o ramo `canceled` voltava cedo com "já alinhado" e o contrato ficava
// preso para sempre (a varredura nem olhava envelopes terminais).
func TestReconcile_CanceledLocalWithoutAnnouncementHeals(t *testing.T) {
	repo := newFakeRepo()
	env := runningEnvelope(t, repo)
	env.Status = envelope.StatusCanceled
	repo.byID[env.ID] = env

	bus := &recordingBus{}
	cs := &fakeClicksign{getStatus: "canceled"}
	svc := service.NewEnvelopeService(repo, cs, &stubCreds{}, &fakeDeal{contract: sampleContract()}, &stubPDF{}, bus)

	if _, err := svc.ReconcileServiceScope(context.Background(), "tenant-1", env.ID); err != nil {
		t.Fatalf("ReconcileServiceScope: %v", err)
	}
	if refusals := bus.refused(); len(refusals) != 1 {
		t.Fatalf("eventos signature.refused = %d, want 1 — sem isso o contrato nunca volta para rascunho", len(refusals))
	}
	if stored := repo.byID[env.ID]; stored.TerminalEventPublishedAt == nil {
		t.Error("TerminalEventPublishedAt vazio depois da reconciliação")
	}
}

// E, uma vez anunciado, a reconciliação não republica a cada ciclo da varredura.
func TestReconcile_CanceledAndAnnouncedDoesNotRepublish(t *testing.T) {
	repo := newFakeRepo()
	env := runningEnvelope(t, repo)
	env.Status = envelope.StatusCanceled
	announced := env.UpdatedAt
	env.TerminalEventPublishedAt = &announced
	repo.byID[env.ID] = env

	bus := &recordingBus{}
	svc := service.NewEnvelopeService(repo, &fakeClicksign{getStatus: "canceled"}, &stubCreds{}, &fakeDeal{contract: sampleContract()}, &stubPDF{}, bus)

	if _, err := svc.ReconcileServiceScope(context.Background(), "tenant-1", env.ID); err != nil {
		t.Fatalf("ReconcileServiceScope: %v", err)
	}
	if refusals := bus.refused(); len(refusals) != 0 {
		t.Errorf("eventos signature.refused = %d, want 0", len(refusals))
	}
}

// O webhook da Clicksign é chamador de sistema (HMAC, sem JWT): a escrita
// precisa carregar o tenant do envelope no contexto, senão o pool de app
// instala claims vazios, a RLS esconde a linha e TODO webhook morre em
// "envelope not found" — foi o que aconteceu no beta com o webhook `cancel`,
// deixando o contrato preso.
func TestWebhook_WriteCarriesEnvelopeTenantInContext(t *testing.T) {
	repo := newFakeRepo()
	env := runningEnvelope(t, repo)
	env.Status = envelope.StatusRunning
	repo.byID[env.ID] = env
	repo.byExternalID[env.ExternalEnvelopeID] = env

	bus := &recordingBus{}
	svc := service.NewWebhookService(repo, bus)

	payload := clicksign.WebhookPayload{
		Event:    clicksign.WebhookEvent{Name: "cancel"},
		Envelope: &clicksign.WebhookEnvelope{ID: env.ExternalEnvelopeID},
	}
	if err := svc.Process(context.Background(), payload, []byte(`{"event":{"name":"cancel"}}`), "delivery-1", "127.0.0.1", "sig"); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(repo.writeTenants) == 0 {
		t.Fatal("nenhuma escrita registrada — o webhook não chegou ao repositório")
	}
	for _, tenantID := range repo.writeTenants {
		if tenantID != env.TenantID {
			t.Errorf("tenant no contexto da escrita = %q, want %q", tenantID, env.TenantID)
		}
	}
}
