package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/oginoino/toca/toca-comm/internal/domain/notification"
)

// NotificationService roteia uma Notification (email, whatsapp, sms,
// etc.) para o serviço de canal correspondente. Mantém um único ponto
// de entrada para todos os callers HTTP — o switch por tipo fica aqui
// em vez de espalhar pelos handlers.
type NotificationService struct {
	emailService    *EmailService
	whatsappService *WhatsAppService
	smsService      *SMSService
}

// NewNotificationService aceita serviços de canal opcionais via
// setters. O construtor pega só o email para preservar a API legada;
// WhatsApp é wirado via WithWhatsAppService a partir do main.go. Issue #443.
func NewNotificationService(emailService *EmailService) *NotificationService {
	return &NotificationService{emailService: emailService}
}

// WithWhatsAppService wires o dispatcher WhatsApp Business — opcional.
// Quando não wirado (ou wirado como nil), envios de TypeWhatsApp falham
// com erro descritivo. Caller (charge_service em toca-financial) deve
// tratar como "canal indisponível" e logar SkippedNote em vez de quebrar
// o pipeline inteiro.
func (s *NotificationService) WithWhatsAppService(svc *WhatsAppService) *NotificationService {
	s.whatsappService = svc
	return s
}

// WithSMSService wires o dispatcher SMS Zenvia — opcional, mesmo
// padrão do WhatsApp. Quando ausente, envios TypeSMS falham com erro
// descritivo, sem quebrar pipelines (charge_service etc. devem tratar
// como canal indisponível). Issue #444.
func (s *NotificationService) WithSMSService(svc *SMSService) *NotificationService {
	s.smsService = svc
	return s
}

func (s *NotificationService) Send(ctx context.Context, n *notification.Notification) error {
	switch n.Type {
	case notification.TypeEmail:
		return s.sendEmail(ctx, n)
	case notification.TypeWhatsApp:
		return s.sendWhatsApp(ctx, n)
	case notification.TypeSMS:
		return s.sendSMS(ctx, n)
	default:
		return fmt.Errorf("notification type not supported: %s", n.Type)
	}
}

func (s *NotificationService) sendEmail(ctx context.Context, n *notification.Notification) error {
	if s.emailService == nil {
		return errors.New("email service not configured")
	}
	if err := s.emailService.SendRawEmail(ctx, n.Recipient, n.Subject, n.Content); err != nil {
		n.Status = notification.StatusFailed
		n.Error = err.Error()
		return err
	}
	n.Status = notification.StatusSent
	now := time.Now()
	n.SentAt = &now
	return nil
}

// sendWhatsApp dispatcha via Meta Cloud API. Recipient é o telefone do
// destinatário (LeadDTO.Phone) — normalização para E.164 é
// responsabilidade do client primitivo (NormalizeBrazilianPhone). Issue #443.
func (s *NotificationService) sendWhatsApp(ctx context.Context, n *notification.Notification) error {
	if s.whatsappService == nil || !s.whatsappService.IsConfigured() {
		return errors.New("whatsapp service not configured")
	}
	res, err := s.whatsappService.Send(ctx, n.Recipient, n.Subject, n.Content)
	if err != nil {
		n.Status = notification.StatusFailed
		n.Error = err.Error()
		return err
	}
	n.Status = notification.StatusSent
	now := time.Now()
	n.SentAt = &now
	// O ProviderMessageID do Meta (wamid) vai no campo Error apenas como
	// vestígio em logs; sem campo dedicado na Notification ainda. Quando
	// o webhook de delivery aterrissar (follow-up), aí persistimos
	// explicitamente.
	if res != nil && res.WAMID != "" {
		n.Error = "" // limpa qualquer marcação anterior
		// futuro: n.ProviderMessageID = res.WAMID — exige migração de schema
	}
	return nil
}

// sendSMS dispatcha via Zenvia REST. Recipient é o telefone (mesma
// normalização do WhatsApp, no client primitivo). Subject é ignorado
// (SMS não tem subject). Content é o body — limitado a 160 chars pelo
// client; se exceder, o client devolve ErrSMSBodyTooLong e a
// Notification fica StatusFailed (caller decide se retry/skip). Issue #444.
func (s *NotificationService) sendSMS(ctx context.Context, n *notification.Notification) error {
	if s.smsService == nil || !s.smsService.IsConfigured() {
		return errors.New("sms service not configured")
	}
	res, err := s.smsService.Send(ctx, n.Recipient, n.Content)
	if err != nil {
		n.Status = notification.StatusFailed
		n.Error = err.Error()
		return err
	}
	n.Status = notification.StatusSent
	now := time.Now()
	n.SentAt = &now
	// MessageID do Zenvia (id da Zenvia, correlação com webhook de
	// delivery futuro). Mesmo limite do WhatsApp: ainda sem coluna
	// dedicada na Notification, então só limpa erro residual.
	if res != nil && res.MessageID != "" {
		n.Error = ""
	}
	return nil
}
