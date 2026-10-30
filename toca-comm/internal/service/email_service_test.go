package service

import (
	"strings"
	"testing"

	"github.com/oginoino/toca/pkg/config"
)

func TestEmailService_ResolvePortalBase(t *testing.T) {
	cases := []struct {
		name    string
		envVars map[string]string
		cfg     config.EmailConfig
		want    string
	}{
		{
			name:    "FRONTEND_URL env wins",
			envVars: map[string]string{"FRONTEND_URL": "https://frontend.example.com"},
			cfg:     config.EmailConfig{FrontendURL: "https://cfg.example.com"},
			want:    "https://frontend.example.com",
		},
		{
			name: "EmailConfig.FrontendURL when env not set",
			cfg:  config.EmailConfig{FrontendURL: "https://cfg.example.com"},
			want: "https://cfg.example.com",
		},
		{
			name:    "PORTAL_URL deprecated fallback",
			envVars: map[string]string{"PORTAL_URL": "https://portal.example.com"},
			want:    "https://portal.example.com",
		},
		{
			name: "EmailConfig.PortalURL deprecated fallback",
			cfg:  config.EmailConfig{PortalURL: "https://cfg-portal.example.com"},
			want: "https://cfg-portal.example.com",
		},
		{
			name: "empty when nothing configured",
			want: "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Always unset relevant envs so previous tests do not leak.
			t.Setenv("FRONTEND_URL", "")
			t.Setenv("PORTAL_URL", "")
			for k, v := range c.envVars {
				t.Setenv(k, v)
			}
			s := &EmailService{cfg: c.cfg}
			if got := s.resolvePortalBase(); got != c.want {
				t.Errorf("resolvePortalBase() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestEmailService_BuildEmailHTML_IncludesTitleAndBody(t *testing.T) {
	s := &EmailService{}
	html := s.buildEmailHTML("Bem-vindo", "<p>Conteúdo do corpo</p>", "Acessar", "https://example.com", "Texto legal")

	if !strings.Contains(html, "Bem-vindo") {
		t.Error("html should contain title")
	}
	if !strings.Contains(html, "Conteúdo do corpo") {
		t.Error("html should contain body content")
	}
	if !strings.Contains(html, "https://example.com") {
		t.Error("html should contain button URL")
	}
	if !strings.Contains(html, "Acessar") {
		t.Error("html should contain button text")
	}
	if !strings.Contains(html, "Texto legal") {
		t.Error("html should contain disclaimer")
	}
}

func TestEmailService_BuildEmailHTML_NoButtonWhenEmpty(t *testing.T) {
	s := &EmailService{}
	html := s.buildEmailHTML("Título", "<p>Corpo</p>", "", "", "")

	// Button block uses #1C905B background color — if the button is omitted,
	// that hex should NOT appear in the rendered HTML.
	if strings.Contains(html, "#1C905B") {
		t.Error("html should not contain button styling when btnText/btnURL are empty")
	}
}

func TestEmailService_BuildEmailHTML_NoButtonWhenOnlyURL(t *testing.T) {
	s := &EmailService{}
	html := s.buildEmailHTML("Título", "<p>Corpo</p>", "", "https://example.com", "")

	if strings.Contains(html, ">https://example.com<") {
		t.Error("html should not render button when btnText is empty even if URL is provided")
	}
}
