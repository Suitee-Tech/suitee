package service

import (
	"context"
	"fmt"
	"time"

	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/metrics"
	"github.com/oginoino/toca/toca-comm/internal/repository"
)

// EmailCategory identifica a categoria de e-mail transacional. O mesmo valor
// é usado no opt-out (comm.notification_prefs.category), nas env flags e como
// chave de cooldown/dedup.
type EmailCategory string

const (
	CategoryLeadAssigned    EmailCategory = "lead_assigned"
	CategoryDealUpdates     EmailCategory = "deal_updates"
	CategoryProposals       EmailCategory = "proposals"
	CategoryAppointments    EmailCategory = "appointments"
	CategoryRecommendations EmailCategory = "recommendations"
	CategoryChatReminders   EmailCategory = "chat_reminders"
)

// NotifyOptions carrega tudo que um e-mail transacional precisa para passar
// pelos gates do beta (opt-out, presença, cooldown, idempotência).
type NotifyOptions struct {
	// Category é obrigatória — define opt-out, flag e cooldown.
	Category EmailCategory
	// TenantID + UserID são a chave de identidade (prefs, presença, dedup).
	// UserID é a identidade de domínio do destinatário (broker id ou account
	// id) e também o recipient_id do ledger comm.sent_externals.
	TenantID string
	UserID   string
	// Recipient é o endereço de e-mail resolvido. Vazio → envio é pulado
	// (destinatário sem e-mail não trava o pipeline).
	Recipient string
	Subject   string
	BodyHTML  string
	// EventID é o id do evento de origem — idempotência entre redeliveries.
	EventID string
	// SkipIfOnline pula o envio quando o usuário está online (a notificação
	// in-app já chegou em tempo real). Force ignora presença E cooldown,
	// mas respeita opt-out e dedup — para marcos críticos (lead atribuído,
	// contrato assinado).
	SkipIfOnline bool
	Force        bool
}

// EmailNotifier aplica as regras transversais do beta
// (docs/features/comunicacao/notificacoes-email-beta.md §3) e dispara o
// e-mail via Resend. Nunca retorna erro de ENVIO para o chamador — falha é
// logada e o pipeline do evento segue (a notificação in-app é o caminho
// primário; e-mail é reforço).
type EmailNotifier struct {
	emailService  *EmailService
	prefs         repository.EmailPrefsRepository
	sentExternals repository.SentExternalsRepository
	hub           *Hub
	cfg           config.EmailConfig
}

func NewEmailNotifier(emailService *EmailService, prefs repository.EmailPrefsRepository, sentExternals repository.SentExternalsRepository, hub *Hub, cfg config.EmailConfig) *EmailNotifier {
	return &EmailNotifier{
		emailService:  emailService,
		prefs:         prefs,
		sentExternals: sentExternals,
		hub:           hub,
		cfg:           cfg,
	}
}

// categoryEnabled mapeia a categoria para a env flag correspondente.
func (n *EmailNotifier) categoryEnabled(category EmailCategory) bool {
	switch category {
	case CategoryLeadAssigned:
		return n.cfg.EmailLeadAssigned
	case CategoryDealUpdates:
		return n.cfg.EmailDealUpdates
	case CategoryProposals:
		return n.cfg.EmailProposals
	case CategoryAppointments:
		return n.cfg.EmailAppointments
	case CategoryRecommendations:
		return n.cfg.EmailRecommendations
	case CategoryChatReminders:
		return n.cfg.EmailChatReminders
	default:
		return false
	}
}

// BuildEmail monta o HTML no layout padrão Suitee (logo, botão CTA,
// disclaimer). Devolve "" quando o notifier não tem EmailService wired.
func (n *EmailNotifier) BuildEmail(title, bodyHTML, btnText, btnURL, disclaimer string) string {
	if n == nil || n.emailService == nil {
		return ""
	}
	return n.emailService.buildEmailHTML(title, bodyHTML, btnText, btnURL, disclaimer)
}

// BuildEmailWithSecondary monta o HTML com um link secundário abaixo do botão
// principal. O link secundário é usado como veio (não passa por FrontendLink):
// o download do calendário já é uma URL absoluta assinada.
func (n *EmailNotifier) BuildEmailWithSecondary(title, bodyHTML, btnText, btnURL, secondaryText, secondaryURL, disclaimer string) string {
	if n == nil || n.emailService == nil {
		return ""
	}
	return n.emailService.buildEmailHTMLWithSecondary(title, bodyHTML, btnText, btnURL, secondaryText, secondaryURL, disclaimer)
}

// FrontendLink monta um link absoluto para um path do app (admin/portal)
// usando FRONTEND_URL. Degrada para o path relativo quando a base não está
// configurada — mesma semântica do EmailService.frontendLink.
func (n *EmailNotifier) FrontendLink(path string) string {
	if n == nil || n.emailService == nil {
		return path
	}
	return n.emailService.frontendLink(path)
}

// Notify envia um e-mail transacional passando por todos os gates. Devolve
// dispatched=true apenas quando o envio foi realmente submetido ao Resend
// (supressões por flag/opt-out/presença/cooldown/dedup devolvem false sem
// erro). Erro só para opts inválidas (identidade obrigatória).
func (n *EmailNotifier) Notify(ctx context.Context, opts NotifyOptions) (bool, error) {
	if n == nil || n.emailService == nil {
		return false, nil
	}
	if opts.Category == "" || opts.UserID == "" || opts.TenantID == "" {
		return false, fmt.Errorf("email notify: category, tenant_id and user_id are required")
	}

	// Gate 0 — kill-switch global e flag da categoria.
	if !n.cfg.TransactionalEmailEnabled {
		metrics.IncEmailSkipped("flag_off")
		return false, nil
	}
	if !n.categoryEnabled(opts.Category) {
		metrics.IncEmailSkipped("flag_off")
		return false, nil
	}

	// Gate 1 — opt-out do usuário. Sem repo wired, default ligado.
	if n.prefs != nil {
		enabled, err := n.prefs.IsEnabled(ctx, opts.UserID, opts.TenantID, string(opts.Category))
		if err != nil {
			logger.Warn("email notify: prefs lookup failed; defaulting to enabled",
				"category", string(opts.Category), "user_id", opts.UserID, "error", err.Error())
		} else if !enabled {
			metrics.IncEmailSkipped("optout")
			logger.Info("email suppressed: user opted out",
				"category", string(opts.Category), "tenant_id", opts.TenantID, "user_id", opts.UserID)
			return false, nil
		}
	}

	// Gate 2 — presença. Usuário online recebe em tempo real no app.
	if !opts.Force && opts.SkipIfOnline && n.hub != nil && n.hub.IsUserOnline(opts.TenantID, opts.UserID) {
		metrics.IncEmailSkipped("online")
		logger.Info("email suppressed: user online",
			"category", string(opts.Category), "tenant_id", opts.TenantID, "user_id", opts.UserID)
		return false, nil
	}

	// Gate 3 — cooldown por categoria/destinatário (0 = desligado).
	if !opts.Force && n.cfg.EmailCooldownMinutes > 0 && n.sentExternals != nil {
		recent, err := n.sentExternals.HasRecentDispatch(ctx, opts.TenantID, "email", opts.UserID, string(opts.Category),
			time.Duration(n.cfg.EmailCooldownMinutes)*time.Minute)
		if err != nil {
			logger.Warn("email notify: cooldown check failed; proceeding",
				"category", string(opts.Category), "user_id", opts.UserID, "error", err.Error())
		} else if recent {
			metrics.IncEmailSkipped("cooldown")
			logger.Info("email suppressed: cooldown",
				"category", string(opts.Category), "tenant_id", opts.TenantID, "user_id", opts.UserID)
			return false, nil
		}
	}

	// Gate 4 — destinatário sem e-mail resolvido.
	if opts.Recipient == "" {
		metrics.IncEmailSkipped("no_email")
		logger.Info("email skipped: recipient has no email",
			"category", string(opts.Category), "tenant_id", opts.TenantID, "user_id", opts.UserID)
		return false, nil
	}

	// Gate 5 — idempotência (event_id + channel + recipient_id). Registro
	// falho = skip (sub-entregar é recuperável; duplicar não é).
	first := true
	if n.sentExternals != nil && opts.EventID != "" {
		ok, err := n.sentExternals.RegisterIfFirst(ctx, opts.EventID, "email", opts.UserID, opts.TenantID, string(opts.Category))
		if err != nil {
			metrics.IncEmailSkipped("ledger_error")
			logger.Warn("email notify: sent_externals register failed; skipping dispatch",
				"category", string(opts.Category), "event_id", opts.EventID, "error", err.Error())
			return false, nil
		}
		first = ok
	}
	if !first {
		metrics.IncEmailSkipped("dedup")
		logger.Info("email suppressed: already dispatched",
			"category", string(opts.Category), "event_id", opts.EventID, "user_id", opts.UserID)
		return false, nil
	}

	// Envio best-effort: erro não propaga para o consumidor do evento.
	if err := n.emailService.SendRawEmail(ctx, opts.Recipient, opts.Subject, opts.BodyHTML); err != nil {
		metrics.IncEmailSkipped("send_failed")
		logger.Warn("email send failed",
			"category", string(opts.Category), "tenant_id", opts.TenantID,
			"user_id", opts.UserID, "event_id", opts.EventID, "error", err.Error())
		return false, nil
	}
	metrics.IncEmailSent(string(opts.Category))
	logger.Info("email sent",
		"category", string(opts.Category), "tenant_id", opts.TenantID,
		"user_id", opts.UserID, "event_id", opts.EventID)
	return true, nil
}
