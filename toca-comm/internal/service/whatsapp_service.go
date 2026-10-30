package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/oginoino/toca/pkg/clients"
)

// WhatsAppService — fina camada de orquestração sobre o
// clients.WhatsAppBusinessClient (Meta Cloud API). Decide se a mensagem
// vai como template ou texto livre baseado na config do tenant/ambiente,
// e expõe uma API estável para os outros serviços (NotificationService,
// EventConsumer) que não precisam saber de detalhes da Meta. Issue #443.
//
// Por que separar do pkg/clients?
//   - pkg/clients/whatsapp_business_client.go é a primitiva HTTP — só
//     envia o JSON certo para o endpoint certo.
//   - WhatsAppService aqui é onde fica o policy: qual template usar,
//     qual o language code, como mapear (subject, body) para
//     (templateName, params), bypass para texto livre em dev/test, etc.
//
// Mantém um único ponto de configuração — todos os callers podem evoluir
// para "WhatsApp message v2" mudando só este arquivo, sem tocar nas
// dependências.

// SendResult resume o envio para callers que precisam do WAMID (audit,
// webhook de delivery, etc.).
type SendResult struct {
	WAMID        string
	RecipientWA  string
	UsedTemplate bool
	TemplateName string
}

// WhatsAppService — dispatcher concreto. Quando `client` é nil, todas as
// chamadas retornam erro — sinal para o caller fazer fallback (Noop +
// log).
type WhatsAppService struct {
	client *clients.WhatsAppBusinessClient

	// useTemplate quando true força modo template usando templateName/
	// templateLanguage como default. False → texto livre (24h window).
	// Lido de WHATSAPP_USE_TEMPLATE (default: true em produção quando
	// templateName está setado).
	useTemplate      bool
	templateName     string
	templateLanguage string
}

// WhatsAppServiceConfig parametriza a construção. Use NewWhatsAppServiceFromEnv
// no main.go; construtor explícito serve para testes.
type WhatsAppServiceConfig struct {
	Client           *clients.WhatsAppBusinessClient
	UseTemplate      bool
	TemplateName     string
	TemplateLanguage string
}

// NewWhatsAppService — construtor explícito (testes).
func NewWhatsAppService(cfg WhatsAppServiceConfig) *WhatsAppService {
	lang := cfg.TemplateLanguage
	if lang == "" {
		lang = "pt_BR"
	}
	return &WhatsAppService{
		client:           cfg.Client,
		useTemplate:      cfg.UseTemplate,
		templateName:     cfg.TemplateName,
		templateLanguage: lang,
	}
}

// NewWhatsAppServiceFromEnv tenta construir o service a partir das env
// vars. Retorna (nil, nil) quando o ambiente NÃO configurou WhatsApp —
// é um caminho legítimo (dev local, tenants que ainda não habilitaram).
// Retorna (nil, error) só quando há config parcial inconsistente (token
// presente, phone_number_id ausente — sinal de erro de deploy).
//
// Env vars lidas (além das do client primitivo):
//
//	WHATSAPP_USE_TEMPLATE        "true" (default) | "false"
//	WHATSAPP_TEMPLATE_NAME       template aprovado pela Meta (ex.: "cobranca_aluguel")
//	WHATSAPP_TEMPLATE_LANGUAGE   default "pt_BR"
func NewWhatsAppServiceFromEnv() (*WhatsAppService, error) {
	client, err := clients.NewWhatsAppBusinessClientFromEnv()
	if err != nil {
		// Não configurado: trate como "feature desligada", não erro.
		// Caller verifica nil-receiver via IsConfigured().
		if errors.Is(err, clients.ErrWhatsAppNotConfigured) {
			return nil, nil
		}
		return nil, err
	}

	useTemplate := true
	if v := os.Getenv("WHATSAPP_USE_TEMPLATE"); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err == nil {
			useTemplate = parsed
		}
	}

	return NewWhatsAppService(WhatsAppServiceConfig{
		Client:           client,
		UseTemplate:      useTemplate,
		TemplateName:     os.Getenv("WHATSAPP_TEMPLATE_NAME"),
		TemplateLanguage: os.Getenv("WHATSAPP_TEMPLATE_LANGUAGE"),
	}), nil
}

// IsConfigured reporta se o service tem um client real para usar. Útil
// para callers fazerem fallback (Noop + log) quando o ambiente não
// habilita WhatsApp, sem precisar capturar erro a cada chamada.
func (s *WhatsAppService) IsConfigured() bool {
	return s != nil && s.client != nil
}

// Send dispatcha a mensagem usando o modo configurado (template ou texto).
//
// `subject` é tratado como hint:
//   - quando vazio: usa templateName default do env
//   - quando setado e useTemplate=true: vira o nome do template
//     (permite que a régua de cobrança escolha "cobranca_atraso" vs
//     "cobranca_vencimento_proximo" sem mudar config global)
//
// `body` para modo template: é o primeiro parâmetro ({{1}}) do template.
// Para modo texto: é o corpo da mensagem.
func (s *WhatsAppService) Send(ctx context.Context, toPhone, subject, body string) (*SendResult, error) {
	if !s.IsConfigured() {
		return nil, fmt.Errorf("%w: whatsapp service not configured", clients.ErrWhatsAppNotConfigured)
	}

	if s.useTemplate {
		template := strings.TrimSpace(subject)
		if template == "" {
			template = s.templateName
		}
		if template == "" {
			return nil, fmt.Errorf("%w: useTemplate=true but neither subject nor WHATSAPP_TEMPLATE_NAME provided", clients.ErrMetaAPIError)
		}
		// Para o primeiro pass: passa o body inteiro como o parâmetro
		// {{1}} do template. Templates futuros com múltiplos placeholders
		// devem usar uma estrutura custom (Send + SendStructured) — fora
		// de escopo aqui.
		params := []string{}
		if body != "" {
			params = append(params, body)
		}
		res, err := s.client.SendTemplate(ctx, toPhone, template, s.templateLanguage, params)
		if err != nil {
			return nil, err
		}
		return &SendResult{
			WAMID:        res.WAMID,
			RecipientWA:  res.RecipientWA,
			UsedTemplate: true,
			TemplateName: template,
		}, nil
	}

	res, err := s.client.SendText(ctx, toPhone, body)
	if err != nil {
		return nil, err
	}
	return &SendResult{
		WAMID:       res.WAMID,
		RecipientWA: res.RecipientWA,
	}, nil
}
