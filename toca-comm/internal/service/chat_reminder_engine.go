package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/metrics"
	"github.com/oginoino/toca/toca-comm/internal/repository"
)

// ChatReminderEngine é o motor de lembretes de mensagens não lidas
// (beta F3 — docs/features/comunicacao/notificacoes-email-beta.md §8).
//
// A cada tick (padrão 5 min, protegido por advisory lock Postgres para
// deploy multi-pod — mesmo padrão do toca-financial #445), lista as
// conversas com mensagem parada há ≥ minAge, filtra online/mutados já
// cobertos pelo SQL, e envia UM e-mail por (canal, participante) por hora
// via EmailNotifier (categoria chat_reminders, com opt-out/cooldown/dedup).
type ChatReminderEngine struct {
	repo     repository.ChatReminderRepository
	resolver repository.EmailRecipientResolver
	notifier *EmailNotifier
	hub      *Hub
	db       *database.PostgresDB

	tickInterval time.Duration
	minAge       time.Duration // idade mínima da mensagem não lida (30min)
	maxAge       time.Duration // janela máxima de busca (24h)

	stopCh chan struct{}
	doneCh chan struct{}
	once   sync.Once
}

// chatReminderLockKey coordena múltiplas instâncias do toca-comm no mesmo
// banco (FNV-1a no pkg/database, estável entre processos).
const chatReminderLockKey = "toca-comm.chat-reminder.tick"

// chatReminderGroupKey identifica UM grupo de mensagens não lidas de uma
// conversa/participante: canal + participante + a mensagem mais recente ainda
// não lida. Enquanto o topo do não lido não mudar, o grupo é o mesmo e o
// lembrete não se repete.
func chatReminderGroupKey(channelID, participantID, latestUnreadMessageID string) string {
	return fmt.Sprintf("%s:%s:%s", channelID, participantID, latestUnreadMessageID)
}

const (
	defaultChatReminderTickInterval = 5 * time.Minute
	defaultChatReminderMinAge       = 30 * time.Minute
	defaultChatReminderMaxAge       = 24 * time.Hour
)

// NewChatReminderEngine monta o motor. minAge <= 0 usa o default (30 min).
func NewChatReminderEngine(repo repository.ChatReminderRepository, resolver repository.EmailRecipientResolver, notifier *EmailNotifier, hub *Hub, minAge time.Duration) *ChatReminderEngine {
	if minAge <= 0 {
		minAge = defaultChatReminderMinAge
	}
	return &ChatReminderEngine{
		repo:         repo,
		resolver:     resolver,
		notifier:     notifier,
		hub:          hub,
		tickInterval: defaultChatReminderTickInterval,
		minAge:       minAge,
		maxAge:       defaultChatReminderMaxAge,
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
	}
}

// WithDB wires o advisory lock multi-pod. Sem db o lock é pulado
// (modo single-instance, dev/test).
func (e *ChatReminderEngine) WithDB(db *database.PostgresDB) *ChatReminderEngine {
	e.db = db
	return e
}

// Start dispara o ticker em background. Idempotente.
func (e *ChatReminderEngine) Start() {
	e.once.Do(func() {
		go e.loop()
		log.Printf("[ChatReminder] engine started (tick=%s minAge=%s)", e.tickInterval, e.minAge)
	})
}

// Stop encerra o ticker e espera o loop atual terminar.
func (e *ChatReminderEngine) Stop() {
	select {
	case <-e.stopCh:
		// Já parado.
	default:
		close(e.stopCh)
		<-e.doneCh
	}
}

func (e *ChatReminderEngine) loop() {
	defer close(e.doneCh)
	ticker := time.NewTicker(e.tickInterval)
	defer ticker.Stop()

	// Primeiro tick imediato? Não — o motor espera o primeiro intervalo para
	// não competir com o boot do serviço. Lembretes são reforço, não SLA.
	for {
		select {
		case <-e.stopCh:
			return
		case <-ticker.C:
			e.runTick()
		}
	}
}

func (e *ChatReminderEngine) runTick() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	e.runWithLock(ctx, func(ctx context.Context) {
		candidates, err := e.repo.ListUnreadCandidates(ctx, e.minAge, e.maxAge, 200)
		if err != nil {
			logger.Warn("chat reminder: candidate query failed", "error", err.Error())
			return
		}
		// Dedup por (canal, participante) mantendo a mensagem mais recente.
		latest := map[string]repository.ChatReminderCandidate{}
		for _, cand := range candidates {
			key := cand.ChannelID + "|" + cand.ParticipantID
			if prev, ok := latest[key]; !ok || cand.SentAt.After(prev.SentAt) {
				latest[key] = cand
			}
		}
		sent := 0
		for _, cand := range latest {
			if e.sendReminder(ctx, cand) {
				sent++
			}
		}
		metrics.AddChatReminderCandidates(uint64(len(latest)))
		if len(latest) > 0 {
			logger.Info("chat reminder: tick done",
				"candidates", len(latest), "sent", sent,
				"skipped_online_mute_unresolved", len(latest)-sent)
		}
	})
}

// sendReminder envia o lembrete para um candidato. Devolve true quando o
// e-mail foi disparado.
func (e *ChatReminderEngine) sendReminder(ctx context.Context, cand repository.ChatReminderCandidate) bool {
	if e.notifier == nil || e.resolver == nil {
		return false
	}
	if cand.TenantID == "" || cand.ChannelID == "" || cand.ParticipantID == "" {
		return false
	}
	// Presença: usuário online já recebeu a notificação in-app em tempo real.
	if e.hub != nil && e.hub.IsUserOnline(cand.TenantID, cand.ParticipantID) {
		return false
	}
	email, err := e.resolver.ParticipantEmailByID(ctx, cand.TenantID, cand.ParticipantID)
	if err != nil {
		logger.Warn("chat reminder: failed to resolve email",
			"tenant_id", cand.TenantID, "participant", cand.ParticipantID, "error", err.Error())
		return false
	}
	if email == "" {
		return false
	}

	// Idempotência ancorada no GRUPO de mensagens não lidas: a chave usa a
	// mensagem mais recente que o participante ainda não leu. Assim o lembrete
	// sai UMA única vez por grupo — ticks seguintes com a mesma mensagem no
	// topo colapsam na chave já registrada, em vez de repetir a cada janela.
	// Quando chega mensagem nova, o grupo muda (chave nova) e o participante
	// pode ser lembrado de novo (respeitando o cooldown de anti-spam).
	groupKey := chatReminderGroupKey(cand.ChannelID, cand.ParticipantID, cand.MessageID)
	eventID := "chat_reminder:" + groupKey

	sender := cand.SenderName
	if sender == "" {
		sender = "Alguém"
	}
	channelName := cand.ChannelName
	if channelName == "" {
		channelName = "uma conversa"
	}
	snippet := strings.TrimSpace(cand.Snippet)
	body := fmt.Sprintf("<p style='margin: 0 0 24px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>Você tem uma mensagem não lida de <strong>%s</strong> em <strong>%s</strong>.</p>", sender, channelName)
	if snippet != "" {
		body += fmt.Sprintf("<p style='margin: 0 0 24px 0; font-size: 14px; color: #6b7280;'>\"%s\"</p>", snippet)
	}

	dispatched, _ := e.notifier.Notify(ctx, NotifyOptions{
		Category:  CategoryChatReminders,
		TenantID:  cand.TenantID,
		UserID:    cand.ParticipantID,
		Recipient: email,
		Subject:   "Mensagem não lida em " + channelName,
		BodyHTML:  e.notifier.BuildEmail("Você tem uma mensagem não lida", body, "Responder", e.notifier.FrontendLink("/messages/"+cand.ChannelID), internalEmailDisclaimer()),
		EventID:   eventID,
	})
	if dispatched {
		metrics.IncChatReminderSent()
	}
	return dispatched
}

// runWithLock espelha o padrão do toca-financial (#445): protege o tick com
// pg_try_advisory_lock quando o db está wired; sem lock adquirido (outro pod
// processando) o tick é pulado silenciosamente.
func (e *ChatReminderEngine) runWithLock(ctx context.Context, fn func(context.Context)) {
	if e.db == nil || !e.db.CanHoldSessionLockDuringWork() {
		fn(ctx)
		return
	}
	lockRes, releaser, err := e.db.TryAdvisoryLock(ctx, chatReminderLockKey)
	if err != nil {
		logger.Warn("chat reminder: advisory lock error; skipping tick", "error", err.Error())
		return
	}
	defer func() {
		if relErr := releaser.Release(ctx); relErr != nil {
			logger.Warn("chat reminder: advisory unlock error", "error", relErr.Error())
		}
	}()
	if !lockRes.Acquired {
		log.Printf("[ChatReminder] tick skipped — another instance holds the lock (key=%s)", lockRes.Key)
		return
	}
	fn(ctx)
}
