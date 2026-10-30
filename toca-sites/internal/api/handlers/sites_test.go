package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/oginoino/toca/toca-sites/internal/domain/site"
	"github.com/oginoino/toca/toca-sites/internal/services"
)

func recorderFor(method string, target string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, target, nil)
	return c, w
}

func TestWriteErrorMapsDomainErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"validação por campo", services.ValidationError{Field: "slug", Message: "marca reservada"}, http.StatusBadRequest},
		{"validação simples", services.ErrValidation, http.StatusBadRequest},
		{"não encontrado", site.ErrNotFound, http.StatusNotFound},
		{"site desligado vira 404", site.ErrSiteDisabled, http.StatusNotFound},
		{"marca em uso", site.ErrSlugTaken, http.StatusConflict},
		{"marca inválida", site.ErrSlugInvalid, http.StatusBadRequest},
		{"desconhecido", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, w := recorderFor(http.MethodGet, "/")
			writeError(c, tc.err)
			if w.Code != tc.want {
				t.Fatalf("status=%d want %d (body %s)", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

func TestWriteErrorDiscardsBotSilently(t *testing.T) {
	c, w := recorderFor(http.MethodPost, "/")
	writeError(c, services.ErrDiscarded)
	if w.Code != http.StatusAccepted {
		t.Fatalf("honeypot deveria responder 202, veio %d", w.Code)
	}
}

func TestWriteErrorFieldCarriesFieldName(t *testing.T) {
	c, w := recorderFor(http.MethodPost, "/")
	writeError(c, services.ValidationError{Field: "accepted_contact", Message: "aceite o contato"})
	if body := w.Body.String(); !strings.Contains(body, "accepted_contact") {
		t.Fatalf("resposta sem o campo: %s", body)
	}
}

func TestSiteHostPrefersExplicitHeader(t *testing.T) {
	c, _ := recorderFor(http.MethodPost, "/api/v1/sites/public/views")
	c.Request.Host = "api.example.com"
	c.Request.Header.Set("X-Forwarded-Host", "proxy.suitee.com.br")
	if got := siteHost(c); got != "proxy.suitee.com.br" {
		t.Fatalf("atrás do proxy deve usar X-Forwarded-Host: %q", got)
	}
	c.Request.Header.Del("X-Forwarded-Host")
	if got := siteHost(c); got != "api.example.com" {
		t.Fatalf("sem headers de proxy deve usar o Host: %q", got)
	}
	c.Request.Header.Set("X-Site-Host", "Exemplo.imoveis.suitee.com.br")
	if got := siteHost(c); got != "Exemplo.imoveis.suitee.com.br" {
		t.Fatalf("X-Site-Host deve vencer: %q", got)
	}
}
