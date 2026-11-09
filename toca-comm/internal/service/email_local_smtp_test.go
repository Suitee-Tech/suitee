package service

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/config"
)

// smtpFake is deliberately tiny: enough of RFC 5321 for net/smtp to deliver
// one message locally. It lets us prove that template-based flows (welcome,
// reset and invitation) are rendered and delivered through Mailpit's SMTP
// transport instead of calling Resend.
func smtpFake(t *testing.T) (host, port string, received <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	host, port, err = net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	messages := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		r := bufio.NewReader(conn)
		w := bufio.NewWriter(conn)
		write := func(s string) { _, _ = w.WriteString(s); _ = w.Flush() }
		write("220 localhost ESMTP\r\n")
		var data strings.Builder
		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if inData {
				if line == ".\r\n" {
					inData = false
					write("250 OK\r\n")
					messages <- data.String()
					continue
				}
				data.WriteString(line)
				continue
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				write("250-localhost\r\n250 PIPELINING\r\n")
			case strings.HasPrefix(cmd, "MAIL FROM"), strings.HasPrefix(cmd, "RCPT TO"):
				write("250 OK\r\n")
			case strings.HasPrefix(cmd, "DATA"):
				inData = true
				write("354 End data with <CR><LF>.<CR><LF>\r\n")
			case strings.HasPrefix(cmd, "QUIT"):
				write("221 Bye\r\n")
				return
			default:
				write("250 OK\r\n")
			}
		}
	}()
	return host, port, messages
}

func localEmailService(t *testing.T) (*EmailService, <-chan string) {
	t.Helper()
	host, port, messages := smtpFake(t)
	return NewEmailService(config.EmailConfig{
		SMTPHost:    host,
		SMTPPort:    port,
		SMTPFrom:    "no-reply@toca.local",
		FrontendURL: "http://localhost:3000",
	}), messages
}

func assertLocalMessage(t *testing.T, messages <-chan string, wantSubject, wantBody string) {
	t.Helper()
	select {
	case raw := <-messages:
		if !strings.Contains(raw, wantSubject) {
			t.Fatalf("message missing subject %q:\n%s", wantSubject, raw)
		}
		if !strings.Contains(raw, wantBody) {
			t.Fatalf("message missing body %q:\n%s", wantBody, raw)
		}
		if strings.Contains(raw, "template") {
			t.Fatalf("local message must not carry a Resend template:\n%s", raw)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SMTP did not receive the message")
	}
}

func TestLocalSMTP_WelcomePasswordResetAndInvitation(t *testing.T) {
	t.Run("welcome", func(t *testing.T) {
		svc, messages := localEmailService(t)
		if err := svc.SendWelcomeEmail(context.Background(), "ana@example.com", "Ana", "http://localhost:3000"); err != nil {
			t.Fatalf("SendWelcomeEmail: %v", err)
		}
		assertLocalMessage(t, messages, "Bem-vindo", "Ana")
	})

	t.Run("password reset", func(t *testing.T) {
		svc, messages := localEmailService(t)
		if err := svc.SendPasswordResetEmail(context.Background(), "ana@example.com", "http://localhost:3000/reset-password"); err != nil {
			t.Fatalf("SendPasswordResetEmail: %v", err)
		}
		assertLocalMessage(t, messages, "Recuperação de senha", "reset-password")
	})

	t.Run("invitation", func(t *testing.T) {
		svc, messages := localEmailService(t)
		if err := svc.SendInvitationEmail(context.Background(), "ana@example.com", "Imobiliária Exemplo", "http://localhost:3000/invite?token=abc"); err != nil {
			t.Fatalf("SendInvitationEmail: %v", err)
		}
		assertLocalMessage(t, messages, "Convite", "Imobiliária Exemplo")
	})

	t.Run("otp", func(t *testing.T) {
		svc, messages := localEmailService(t)
		if err := svc.SendOTPEmail(context.Background(), "ana@example.com", "123456"); err != nil {
			t.Fatalf("SendOTPEmail: %v", err)
		}
		assertLocalMessage(t, messages, "Seu código de acesso", "123456")
	})

	t.Run("client invitation", func(t *testing.T) {
		svc, messages := localEmailService(t)
		if err := svc.SendClientInvitationEmail(context.Background(), "cliente@example.com", "Ana"); err != nil {
			t.Fatalf("SendClientInvitationEmail: %v", err)
		}
		assertLocalMessage(t, messages, "Portal do Cliente", "/portal/login")
	})
}
