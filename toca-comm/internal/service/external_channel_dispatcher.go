package service

import (
	"context"

	"github.com/oginoino/toca/pkg/logger"
)

type ExternalChannelDispatcher interface {
	DispatchWhatsApp(ctx context.Context, tenantID, userID, subject, body string) error
	DispatchSMS(ctx context.Context, tenantID, userID, body string) error
	DispatchEmail(ctx context.Context, tenantID, userID, subject, bodyHTML string) error
}

type NoopExternalDispatcher struct{}

func (d *NoopExternalDispatcher) DispatchWhatsApp(ctx context.Context, tenantID, userID, subject, body string) error {
	logger.Info("noop external dispatch: whatsapp", "tenant_id", tenantID, "user_id", userID, "subject", subject)
	return nil
}

func (d *NoopExternalDispatcher) DispatchSMS(ctx context.Context, tenantID, userID, body string) error {
	logger.Info("noop external dispatch: sms", "tenant_id", tenantID, "user_id", userID)
	return nil
}

func (d *NoopExternalDispatcher) DispatchEmail(ctx context.Context, tenantID, userID, subject, bodyHTML string) error {
	logger.Info("noop external dispatch: email", "tenant_id", tenantID, "user_id", userID, "subject", subject)
	return nil
}
