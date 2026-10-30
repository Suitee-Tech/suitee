package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	pkgvalidation "github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

func TestParseLeadListLimit(t *testing.T) {
	cases := []struct {
		name      string
		scope     string
		raw       string
		wantLimit int64
		wantErr   string
	}{
		{name: "empty broker scope returns default", scope: "broker", raw: "", wantLimit: DefaultLeadListLimit},
		{name: "empty tenant scope returns tenant cap", scope: "tenant", raw: "", wantLimit: MaxTenantScopeLimit},
		{name: "explicit small value passes through", scope: "broker", raw: "5", wantLimit: 5},
		{name: "tenant scope above cap silently capped", scope: "tenant", raw: "50", wantLimit: MaxTenantScopeLimit},
		{name: "broker scope at max passes", scope: "broker", raw: "100", wantLimit: MaxLeadListLimit},
		{name: "above max rejected", scope: "broker", raw: "500", wantErr: "limit exceeds maximum of 100"},
		{name: "non-numeric rejected", scope: "broker", raw: "abc", wantErr: "invalid limit value"},
		{name: "zero rejected", scope: "broker", raw: "0", wantErr: "invalid limit value"},
		{name: "negative rejected", scope: "broker", raw: "-1", wantErr: "invalid limit value"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotLimit, gotErr := ParseLeadListLimit(tc.scope, tc.raw)
			if gotErr != tc.wantErr {
				t.Fatalf("error: got %q want %q", gotErr, tc.wantErr)
			}
			if gotLimit != tc.wantLimit {
				t.Fatalf("limit: got %d want %d", gotLimit, tc.wantLimit)
			}
		})
	}
}

func TestIsValidationError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "invalid phone with prefix", err: errAt("invalid phone: invalid Brazilian phone number"), want: true},
		{name: "invalid cpf", err: errAt("invalid cpf"), want: true},
		{name: "invalid cnpj uppercase ignored", err: errAt("INVALID CNPJ"), want: true},
		{name: "validation error generic", err: errAt("field validation failed"), want: true},
		{name: "underscored code", err: errAt("invalid_email: bad address"), want: true},
		{name: "connection refused not validation", err: errAt("connection refused"), want: false},
		{name: "sql null not validation", err: errAt("sql: no rows in result set"), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isValidationError(tc.err); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

type stringErr string

func (s stringErr) Error() string { return string(s) }
func errAt(s string) error        { return stringErr(s) }

func TestFormatLeadETag(t *testing.T) {
	l := &lead.Lead{UpdatedSeq: 42, UpdatedAt: time.Date(2026, 5, 17, 12, 34, 56, 789000000, time.UTC)}
	got := formatLeadETag(l)
	want := `W/"42:2026-05-17T12:34:56.789Z"`
	if got != want {
		t.Fatalf("etag: got %q want %q", got, want)
	}
	if formatLeadETag(nil) != "" {
		t.Fatalf("nil lead should yield empty etag")
	}
}

func TestParseIfMatchLead(t *testing.T) {
	mustTime := func(s string) time.Time {
		tt, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			t.Fatalf("bad fixture: %v", err)
		}
		return tt
	}

	t.Run("empty returns nil", func(t *testing.T) {
		if got := parseIfMatchLead(""); got != nil {
			t.Fatalf("want nil, got %#v", got)
		}
		if got := parseIfMatchLead("   "); got != nil {
			t.Fatalf("want nil, got %#v", got)
		}
	})

	t.Run("seq-only form", func(t *testing.T) {
		got := parseIfMatchLead(`W/"7"`)
		if got == nil || got.UpdatedSeq == nil || *got.UpdatedSeq != 7 {
			t.Fatalf("seq parse failed: %#v", got)
		}
		if got.UpdatedAt != nil {
			t.Fatalf("expected no UpdatedAt")
		}
	})

	t.Run("combined seq+timestamp", func(t *testing.T) {
		got := parseIfMatchLead(`W/"7:2026-05-17T12:34:56.789Z"`)
		if got == nil || got.UpdatedSeq == nil || *got.UpdatedSeq != 7 {
			t.Fatalf("seq parse failed: %#v", got)
		}
		want := mustTime("2026-05-17T12:34:56.789Z")
		if got.UpdatedAt == nil || !got.UpdatedAt.Equal(want) {
			t.Fatalf("ts parse failed: %#v", got)
		}
	})

	t.Run("legacy bare rfc3339nano", func(t *testing.T) {
		got := parseIfMatchLead("2026-05-17T12:34:56.789Z")
		if got == nil || got.UpdatedSeq != nil {
			t.Fatalf("expected only UpdatedAt: %#v", got)
		}
		want := mustTime("2026-05-17T12:34:56.789Z")
		if got.UpdatedAt == nil || !got.UpdatedAt.Equal(want) {
			t.Fatalf("ts parse failed: %#v", got)
		}
	})

	t.Run("legacy quoted timestamp", func(t *testing.T) {
		got := parseIfMatchLead(`"2026-05-17T12:34:56.789Z"`)
		if got == nil || got.UpdatedAt == nil {
			t.Fatalf("expected UpdatedAt: %#v", got)
		}
	})

	t.Run("unparseable returns nil", func(t *testing.T) {
		if got := parseIfMatchLead("not-a-thing"); got != nil {
			t.Fatalf("expected nil, got %#v", got)
		}
	})
}

// O cadastro de lead devolvia 500 para um telefone inválido (o formulário
// aceitava DDD + 8 dígitos e a API recusava sem dizer o motivo). Agora criar e
// atualizar compartilham o mapeamento: validação é 400 com a mensagem.
func TestRespondLeadMutationError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "telefone inválido vira 400 com o motivo",
			err:  fmt.Errorf("invalid phone: %w", pkgvalidation.ErrInvalidBRPhone),
			want: http.StatusBadRequest,
		},
		{
			name: "validação genérica vira 400",
			err:  errAt("validation failed: nome obrigatório"),
			want: http.StatusBadRequest,
		},
		{
			name: "falha inesperada continua 500",
			err:  errAt("connection refused"),
			want: http.StatusInternalServerError,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/leads", nil)

			respondLeadMutationError(c, tc.err)

			if recorder.Code != tc.want {
				t.Fatalf("status = %d, esperado %d (corpo: %s)", recorder.Code, tc.want, recorder.Body.String())
			}
			if tc.want == http.StatusBadRequest {
				if body := recorder.Body.String(); !strings.Contains(body, "invalid phone") && !strings.Contains(body, "validation") {
					t.Fatalf("corpo do 400 deve trazer o motivo, veio %s", body)
				}
			}
		})
	}
}
