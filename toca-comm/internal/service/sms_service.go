package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/oginoino/toca/pkg/clients"
)

// SMSService — fina camada de orquestração sobre o
// clients.SMSZenviaClient. Issue #444. Mantém o mesmo formato do
// WhatsAppService para que NotificationService.Send roteie por type sem
// surpresa.
//
// Por que separar do pkg/clients?
//   - pkg/clients é o transport (HTTP para Zenvia)
//   - SMSService aqui é onde futuras políticas vivem: choose-provider
//     per tenant (Zenvia hoje, talvez Twilio em outros mercados),
//     fallback gracioso quando o provider está down, dedup por
//     (recipient, body, day) etc. Por ora é uma fina passagem.

type SMSSendResult struct {
	MessageID string
	Recipient string
}

type SMSService struct {
	client *clients.SMSZenviaClient
}

type SMSServiceConfig struct {
	Client *clients.SMSZenviaClient
}

func NewSMSService(cfg SMSServiceConfig) *SMSService {
	return &SMSService{client: cfg.Client}
}

// NewSMSServiceFromEnv tenta construir o service via env vars. Retorna
// (nil, nil) quando o ambiente não configurou SMS — caller trata como
// "feature desligada", sem erro. Erros reais (config parcial) sobem.
func NewSMSServiceFromEnv() (*SMSService, error) {
	client, err := clients.NewSMSZenviaClientFromEnv()
	if err != nil {
		if errors.Is(err, clients.ErrSMSNotConfigured) {
			return nil, nil
		}
		return nil, err
	}
	return NewSMSService(SMSServiceConfig{Client: client}), nil
}

// IsConfigured reporta se o service tem um client real. Nil-safe.
func (s *SMSService) IsConfigured() bool {
	return s != nil && s.client != nil
}

// Send dispatcha um SMS de texto. Sem conceito de template (SMS é só
// texto livre); body deve ser ≤ 160 chars — o client valida e devolve
// ErrSMSBodyTooLong se exceder.
func (s *SMSService) Send(ctx context.Context, toPhone, body string) (*SMSSendResult, error) {
	if !s.IsConfigured() {
		return nil, fmt.Errorf("%w: sms service not configured", clients.ErrSMSNotConfigured)
	}
	res, err := s.client.SendText(ctx, toPhone, body)
	if err != nil {
		return nil, err
	}
	return &SMSSendResult{
		MessageID: res.MessageID,
		Recipient: res.Recipient,
	}, nil
}
