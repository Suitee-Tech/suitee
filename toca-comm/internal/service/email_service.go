package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/logger"
)

const defaultResendBaseURL = "https://api.resend.com"

type EmailService struct {
	client *clients.HTTPClient
	cfg    config.EmailConfig
}

func NewEmailService(cfg config.EmailConfig) *EmailService {
	return newEmailService(cfg, defaultResendBaseURL, 10*time.Second)
}

// newEmailService keeps the production constructor strict while allowing
// contract tests to point at an httptest server. HTTPClient supplies the
// explicit timeout and circuit breaker used by every Resend operation
// (templates and raw HTML alike). Email POSTs use one attempt to avoid
// duplicate delivery after an ambiguous upstream failure.
func newEmailService(cfg config.EmailConfig, baseURL string, timeout time.Duration, opts ...clients.HTTPClientOption) *EmailService {
	clientOpts := []clients.HTTPClientOption{
		clients.WithTimeout(timeout),
		clients.WithMaxAttempts(1),
		clients.WithHeaders(func(h http.Header) {
			h.Set("Authorization", "Bearer "+cfg.ResendAPIKey)
			h.Set("Accept", "application/json")
		}),
	}
	clientOpts = append(clientOpts, opts...)
	return &EmailService{
		client: clients.NewHTTPClient(strings.TrimRight(baseURL, "/"), clientOpts...),
		cfg:    cfg,
	}
}

type resendTemplate struct {
	ID        string                 `json:"id"`
	Variables map[string]interface{} `json:"variables"`
}

type resendEmailRequest struct {
	From     string          `json:"from"`
	To       []string        `json:"to"`
	Subject  string          `json:"subject,omitempty"`
	HTML     string          `json:"html,omitempty"`
	Template *resendTemplate `json:"template,omitempty"`
}

// sendEmail escolhe o transporte. SMTP_HOST é usado no local (Inbucket do
// Supabase); sem ele, o provider de produção é o Resend. Templates do Resend
// não têm representação em SMTP, então continuam pelo provider.
func (s *EmailService) sendEmail(ctx context.Context, payload resendEmailRequest) error {
	if strings.TrimSpace(s.cfg.SMTPHost) != "" && payload.Template == nil {
		return s.sendSMTPEmail(ctx, payload)
	}
	return s.sendResendEmail(ctx, payload)
}

func (s *EmailService) sendSMTPEmail(ctx context.Context, payload resendEmailRequest) error {
	port := strings.TrimSpace(s.cfg.SMTPPort)
	if port == "" {
		port = "587"
	}
	host := strings.TrimSpace(s.cfg.SMTPHost)
	addr := net.JoinHostPort(host, port)

	from := strings.TrimSpace(s.cfg.SMTPFrom)
	if from == "" {
		from = s.cfg.FromAddress
	}
	fromAddress, err := mail.ParseAddress(from)
	if err != nil {
		return fmt.Errorf("invalid SMTP from address %q: %w", from, err)
	}

	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("connect SMTP %s: %w", addr, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("SMTP handshake: %w", err)
	}
	defer client.Close()

	if user := strings.TrimSpace(s.cfg.SMTPUsername); user != "" {
		auth := smtp.PlainAuth("", user, s.cfg.SMTPPassword, host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP auth: %w", err)
		}
	}
	if err := client.Mail(fromAddress.Address); err != nil {
		return fmt.Errorf("SMTP MAIL FROM: %w", err)
	}
	for _, to := range payload.To {
		if err := client.Rcpt(to); err != nil {
			return fmt.Errorf("SMTP RCPT TO %s: %w", to, err)
		}
	}

	data, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA: %w", err)
	}

	subject := strings.NewReplacer("\r", " ", "\n", " ").Replace(payload.Subject)
	body := strings.ReplaceAll(payload.HTML, "\n", "\r\n")
	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: %s\r\n", fromAddress.String())
	fmt.Fprintf(&msg, "To: %s\r\n", strings.Join(payload.To, ", "))
	if subject != "" {
		fmt.Fprintf(&msg, "Subject: %s\r\n", mime.QEncoding.Encode("UTF-8", subject))
	}
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	msg.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	msg.WriteString(body)

	if _, err := data.Write(msg.Bytes()); err != nil {
		return fmt.Errorf("SMTP write: %w", err)
	}
	if err := data.Close(); err != nil {
		return fmt.Errorf("SMTP close data: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("SMTP quit: %w", err)
	}

	logger.Info("Email sent via SMTP", "to", payload.To, "subject", payload.Subject)
	return nil
}

func (s *EmailService) sendResendEmail(ctx context.Context, payload resendEmailRequest) error {
	resp, err := s.client.Do(ctx, http.MethodPost, "/emails", payload)
	if err != nil {
		return fmt.Errorf("failed to send email through Resend: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errResp map[string]interface{}
		if err := json.NewDecoder(io.LimitReader(resp.Body, 32_000)).Decode(&errResp); err == nil {
			return fmt.Errorf("resend api error (status %d): %v", resp.StatusCode, errResp)
		}
		return fmt.Errorf("resend api error: status %d", resp.StatusCode)
	}

	var successResp struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&successResp); err == nil {
		logger.Info("Email sent successfully", "id", successResp.ID, "to", payload.To)
	}

	return nil
}

// sendTemplateEmail sends an email using the current Resend template contract.
func (s *EmailService) sendTemplateEmail(ctx context.Context, to, templateID string, variables map[string]interface{}) error {
	return s.sendResendEmail(ctx, resendEmailRequest{
		From: s.cfg.FromAddress,
		To:   []string{to},
		Template: &resendTemplate{
			ID:        templateID,
			Variables: variables,
		},
	})
}

func (s *EmailService) SendWelcomeEmail(ctx context.Context, email, name, appURL string) error {
	// Template ID: fc0c800d-ebad-46bf-96f5-279b9f08ef5b
	variables := map[string]interface{}{
		"NAME":       name,
		"SUITEE_URL": appURL,
		"LOGO_URL":   s.logoURL(),
	}
	return s.sendTemplateEmail(ctx, email, "fc0c800d-ebad-46bf-96f5-279b9f08ef5b", variables)
}

func (s *EmailService) SendPasswordResetEmail(ctx context.Context, email, resetURL string) error {
	// Template ID: 91bf153d-a3d5-4f84-af68-9e49ee710613
	variables := map[string]interface{}{
		"SUITEE_URL": resetURL,
		"LOGO_URL":   s.logoURL(),
	}
	if err := s.sendTemplateEmail(ctx, email, "91bf153d-a3d5-4f84-af68-9e49ee710613", variables); err == nil {
		return nil
	}

	htmlContent := s.buildEmailHTML(
		"Recuperação de senha",
		"<p style='margin: 0 0 24px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>Recebemos uma solicitação para redefinir sua senha. Clique no botão abaixo para criar uma nova senha.</p>",
		"Redefinir senha",
		resetURL,
		"Se você não solicitou a redefinição de senha, ignore este e-mail.",
	)
	return s.SendRawEmail(ctx, email, "Recuperação de senha", htmlContent)
}

func (s *EmailService) SendInvitationEmail(ctx context.Context, email, companyName, inviteURL string) error {
	// Template ID: 0551da37-6822-4675-b08f-9d2b9d0ee87b
	variables := map[string]interface{}{
		"NAME":         "Convidado(a)",
		"COMPANY_NAME": companyName,
		"SUITEE_URL":   inviteURL,
		"LOGO_URL":     s.logoURL(),
	}
	return s.sendTemplateEmail(ctx, email, "0551da37-6822-4675-b08f-9d2b9d0ee87b", variables)
}

// SendClientInvitationEmail sends an email to the client with a magic link
func (s *EmailService) SendClientInvitationEmail(ctx context.Context, to, name string) error {
	base := strings.TrimRight(s.resolvePortalBase(), "/")
	if base == "" {
		logger.Error("FRONTEND_URL not configured; cannot build portal invite link")
		return fmt.Errorf("FRONTEND_URL not configured")
	}
	portalURL := base + "/portal/login?email=" + url.QueryEscape(to)

	htmlContent := s.buildEmailHTML(
		"Você foi convidado para o Portal do Cliente",
		fmt.Sprintf("<p style='margin: 0 0 24px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>Olá <strong>%s</strong>,</p><p style='margin: 0 0 32px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>Você foi convidado para acompanhar seu processo. Acesse o portal clicando no botão abaixo:</p>", name),
		"Acessar Portal",
		portalURL,
		"Se você desconhece este convite, por favor, ignore esta mensagem. Para suporte, entre em contato com a imobiliária responsável.",
	)

	err := s.sendEmail(ctx, resendEmailRequest{
		From:    s.cfg.FromAddress,
		To:      []string{to},
		Subject: "Você foi convidado para o Portal do Cliente",
		HTML:    htmlContent,
	})
	if err != nil {
		logger.Error("Failed to send client invitation email", "error", err)
		return err
	}

	return nil
}

// frontendLink builds an absolute URL for a relative app path (e.g.
// "/parcerias") using the configured frontend base (FRONTEND_URL). When no
// base is configured it returns the relative path unchanged — the email still
// renders, the link is just relative (best-effort, mirrors how other emails
// degrade when FRONTEND_URL is missing).
func (s *EmailService) frontendLink(path string) string {
	base := strings.TrimRight(s.resolvePortalBase(), "/")
	if base == "" {
		return path
	}
	if path == "" {
		return base
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

func (s *EmailService) resolvePortalBase() string {
	if v := os.Getenv("FRONTEND_URL"); v != "" {
		return v
	}
	if s.cfg.FrontendURL != "" {
		return s.cfg.FrontendURL
	}

	if v := os.Getenv("PORTAL_URL"); v != "" {
		logger.Warn("PORTAL_URL is deprecated; use FRONTEND_URL")
		return v
	}
	if s.cfg.PortalURL != "" {
		logger.Warn("EmailConfig.PortalURL is deprecated; use FrontendURL")
		return s.cfg.PortalURL
	}
	return ""
}

func (s *EmailService) SendOTPEmail(ctx context.Context, to string, code string) error {
	htmlContent := s.buildEmailHTML(
		"Seu código de acesso",
		fmt.Sprintf("<p style='margin: 0 0 24px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>Aqui está o seu código de verificação para acessar o Portal do Cliente:</p><div style='background-color: #f3f4f6; border-radius: 8px; padding: 16px; text-align: center; margin-bottom: 32px;'><span style='font-size: 32px; font-weight: 700; color: #111827; letter-spacing: 4px;'>%s</span></div>", code),
		"", // No button
		"",
		"Este código é válido por 10 minutos. Se você não solicitou este acesso, por favor, ignore esta mensagem.",
	)

	err := s.sendEmail(ctx, resendEmailRequest{
		From:    s.cfg.FromAddress,
		To:      []string{to},
		Subject: "Seu código de acesso",
		HTML:    htmlContent,
	})
	if err != nil {
		logger.Error("Failed to send otp email", "error", err)
		return err
	}

	return nil
}

func (s *EmailService) SendRawEmail(ctx context.Context, to string, subject string, html string) error {
	err := s.sendEmail(ctx, resendEmailRequest{
		From:    s.cfg.FromAddress,
		To:      []string{to},
		Subject: subject,
		HTML:    html,
	})
	if err != nil {
		logger.Error("Failed to send raw email", "error", err)
		return err
	}
	return nil
}

// logoURL devolve a URL absoluta do logo para e-mails. Segue o frontend do
// ambiente (FRONTEND_URL) para que beta/produção apontem para os próprios
// assets; e-mails não resolvem URL relativa, então sem base configurada cai
// para o domínio de produção.
func (s *EmailService) logoURL() string {
	logoURL := s.frontendLink("/email/suitee-logo.png")
	if logoURL == "/email/suitee-logo.png" {
		logoURL = "https://example.com/email/suitee-logo.png"
	}
	return logoURL
}

func (s *EmailService) buildEmailHTML(title, bodyContent, btnText, btnURL, disclaimer string) string {
	return s.buildEmailHTMLWithSecondary(title, bodyContent, btnText, btnURL, "", "", disclaimer)
}

// buildEmailHTMLWithSecondary acrescenta um link secundário abaixo do botão:
// o convite principal (ex.: adicionar ao calendário) não pode esconder o
// portal, que continua sendo o caminho para acompanhar o compromisso.
func (s *EmailService) buildEmailHTMLWithSecondary(title, bodyContent, btnText, btnURL, secondaryText, secondaryURL, disclaimer string) string {
	logoURL := s.logoURL()

	btnHTML := ""
	if btnText != "" && btnURL != "" {
		btnHTML = fmt.Sprintf(`
                    <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="margin: 0 0 32px 0;">
                      <tr>
                        <td align="center">
                          <table role="presentation" cellpadding="0" cellspacing="0" style="border-collapse: separate; width: auto;">
                            <tr>
                              <td align="center" bgcolor="#1C905B" style="border-radius: 8px;">
                                <a class="btn" href="%s" target="_blank" style="display: block; box-sizing: border-box; max-width: 100%%; padding: 14px 28px; font-size: 16px; font-weight: 600; color: #ffffff; text-decoration: none; border-radius: 8px; background-color: #1C905B; border: 1px solid #1C905B; mso-padding-alt: 14px 28px; white-space: normal; word-break: break-word;">
                                  %s
                                </a>
                              </td>
                            </tr>
                          </table>
                        </td>
                      </tr>
                    </table>`, btnURL, btnText)
	}

	if secondaryText != "" && secondaryURL != "" {
		btnHTML += fmt.Sprintf(`
                    <p style="margin: 0 0 32px 0; font-size: 14px; line-height: 1.6; text-align: center;">
                      <a href="%s" target="_blank" style="color: #1C905B; text-decoration: underline;">%s</a>
                    </p>`, secondaryURL, secondaryText)
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="pt-BR">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>%s</title>
  <!--[if mso]>
  <style type="text/css">
    body, table, td {font-family: Arial, Helvetica, sans-serif !important;}
  </style>
  <![endif]-->
  <style>
    body { margin: 0; padding: 0; background-color: #f9fafb; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased; word-break: break-word; }
    table { border-spacing: 0; border-collapse: collapse; width: 100%%; table-layout: fixed; }
    td { padding: 0; word-break: break-word; }
    img { border: 0; max-width: 100%%; height: auto; }
    .wrapper { width: 100%%; table-layout: fixed; background-color: #f9fafb; padding-bottom: 60px; }
    .container { max-width: 600px; margin: 0 auto; width: 100%%; }
    .card { background-color: #ffffff; border-radius: 12px; border: 1px solid #e5e7eb; overflow: hidden; box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.05); }
    .content-pad { padding: 40px 32px; }
    @media screen and (max-width: 600px) {
      .content-pad { padding: 32px 20px !important; }
      .container { width: 100%% !important; max-width: 100%% !important; }
      .btn { display: block !important; width: 100%% !important; box-sizing: border-box !important; text-align: center !important; }
      body { padding: 20px 10px !important; }
    }
  </style>
</head>
<body style="background-color: #f9fafb; margin: 0; padding: 40px 20px;">
  <table role="presentation" class="wrapper">
    <tr>
      <td align="center">
        <table role="presentation" class="container" style="max-width: 600px; margin: 0 auto;">
          <tr>
            <td align="center" style="padding-bottom: 24px;">
              <img src="%s" alt="Suitee Logo" width="160" style="display: block; width: 160px; max-width: 100%%; height: auto;">
            </td>
          </tr>
          <tr>
            <td class="card" style="background-color: #ffffff; border-radius: 12px; border: 1px solid #e5e7eb;">
              <table role="presentation" style="width: 100%%;">
                <tr>
                  <td class="content-pad" style="padding: 40px 32px;">
                    <h1 style="margin: 0 0 16px 0; font-size: 24px; font-weight: 700; color: #111827; line-height: 1.25;">
                      %s
                    </h1>
                    %s
                    %s
                    <hr style="border: none; border-top: 1px solid #e5e7eb; margin: 0 0 24px 0;">
                    <p style="margin: 0; font-size: 14px; color: #6b7280; line-height: 1.5;">
                      %s
                    </p>
                  </td>
                </tr>
              </table>
            </td>
          </tr>
          <tr>
            <td align="center" style="padding-top: 32px; padding-bottom: 32px;">
              <p style="margin: 0 0 8px 0; font-size: 12px; color: #9ca3af; line-height: 1.5;">
                &copy; %d Suitee Inteligência Imobiliária. Todos os direitos reservados.
              </p>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`, title, logoURL, title, bodyContent, btnHTML, disclaimer, time.Now().Year())
}
