package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/toca-comm/internal/domain/notification"
)

type AuthEventHandler struct {
	emailService *EmailService
	inAppService *InAppNotificationService
}

func NewAuthEventHandler(emailService *EmailService, inAppService *InAppNotificationService) *AuthEventHandler {
	return &AuthEventHandler{
		emailService: emailService,
		inAppService: inAppService,
	}
}

// ClientVisitRequestedData é o payload do evento client.visit.requested.
type ClientVisitRequestedData struct {
	LeadID        string `json:"lead_id"`
	BrokerID      string `json:"broker_id"`
	TenantID      string `json:"tenant_id"`
	ClientName    string `json:"client_name"`
	ClientEmail   string `json:"client_email"`
	PropertyID    string `json:"property_id"`
	Date          string `json:"date"`
	Time          string `json:"time"`
	AppointmentID string `json:"appointment_id"`
}

// HandleClientVisitRequested notifica o corretor quando o cliente solicita uma visita.
func (h *AuthEventHandler) HandleClientVisitRequested(ctx context.Context, event events.Event) error {
	payloadBytes, err := json.Marshal(event.Payload())
	if err != nil {
		return err
	}
	var data ClientVisitRequestedData
	if err := json.Unmarshal(payloadBytes, &data); err != nil {
		return err
	}
	logger.Info("Client visit requested", "lead", data.LeadID, "broker", data.BrokerID, "date", data.Date, "time", data.Time)
	if h.inAppService == nil || data.BrokerID == "" || data.TenantID == "" {
		return nil
	}
	title := "Visita solicitada pelo cliente"
	content := fmt.Sprintf("%s solicitou uma visita para %s às %s.", data.ClientName, data.Date, data.Time)
	link := fmt.Sprintf("/agenda?compromisso=%s", data.AppointmentID)
	if data.AppointmentID == "" {
		link = "/agenda"
	}
	_ = h.inAppService.Create(ctx, data.TenantID, data.BrokerID, notification.InAppTypeSystem, notification.PriorityImportant, title, content, link, map[string]interface{}{"lead_id": data.LeadID, "visit_date": data.Date, "appointment_id": data.AppointmentID})
	return nil
}

// UserRegisteredEvent matches toca-auth domain events
type UserRegisteredEvent struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	TenantID  string `json:"tenant_id"`
	AppURL    string `json:"app_url"`
}

func (h *AuthEventHandler) HandleUserRegistered(ctx context.Context, event events.Event) error {
	payloadBytes, err := json.Marshal(event.Payload())
	if err != nil {
		return err
	}

	var data UserRegisteredEvent
	if err := json.Unmarshal(payloadBytes, &data); err != nil {
		return err
	}

	logger.Info("Processing user registered event", "email", data.Email)
	return h.emailService.SendWelcomeEmail(ctx, data.Email, data.FirstName, data.AppURL)
}

type PasswordResetRequestedEvent struct {
	Email    string `json:"email"`
	Token    string `json:"token"`
	ResetURL string `json:"reset_url"`
}

func (h *AuthEventHandler) HandlePasswordResetRequested(ctx context.Context, event events.Event) error {
	payloadBytes, err := json.Marshal(event.Payload())
	if err != nil {
		return err
	}

	var data PasswordResetRequestedEvent
	if err := json.Unmarshal(payloadBytes, &data); err != nil {
		return err
	}

	logger.Info("Processing password reset event", "email", data.Email)
	return h.emailService.SendPasswordResetEmail(ctx, data.Email, data.ResetURL)
}

type InvitationCreatedEvent struct {
	Email       string `json:"email"`
	CompanyID   string `json:"company_id"`
	CompanyName string `json:"company_name"`
	Role        string `json:"role"`
	Token       string `json:"token"`
	InviteURL   string `json:"invite_url"`
}

func (h *AuthEventHandler) HandleInvitationCreated(ctx context.Context, event events.Event) error {
	payloadBytes, err := json.Marshal(event.Payload())
	if err != nil {
		return err
	}

	var data InvitationCreatedEvent
	if err := json.Unmarshal(payloadBytes, &data); err != nil {
		return err
	}

	logger.Info("Processing invitation event", "email", data.Email)
	return h.emailService.SendInvitationEmail(ctx, data.Email, data.CompanyName, data.InviteURL)
}

type OTPRequestedEvent struct {
	Contact string `json:"contact"`
	Method  string `json:"method"`
	Code    string `json:"code"`
}

func (h *AuthEventHandler) HandleOTPRequested(ctx context.Context, event events.Event) error {
	payloadBytes, err := json.Marshal(event.Payload())
	if err != nil {
		return err
	}

	var data OTPRequestedEvent
	if err := json.Unmarshal(payloadBytes, &data); err != nil {
		return err
	}

	logger.Info("Processing OTP request event", "contact", data.Contact, "method", data.Method)
	if data.Method == "email" {
		return h.emailService.SendOTPEmail(ctx, data.Contact, data.Code)
	}
	// Future: implement WhatsApp OTP
	logger.Info("WhatsApp OTP not yet implemented", "contact", data.Contact)
	return nil
}

type ClientInvitedEvent struct {
	Email      string `json:"email"`
	ClientName string `json:"name"`
	Phone      string `json:"phone"`
}

func (h *AuthEventHandler) HandleClientInvited(ctx context.Context, event events.Event) error {
	payloadBytes, err := json.Marshal(event.Payload())
	if err != nil {
		return err
	}

	var data ClientInvitedEvent
	if err := json.Unmarshal(payloadBytes, &data); err != nil {
		return err
	}

	logger.Info("Processing client invited event", "email", data.Email)
	return h.emailService.SendClientInvitationEmail(ctx, data.Email, data.ClientName)
}
