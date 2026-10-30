package handlers

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/config"
	appctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-calendar/internal/models/appointment"
	"github.com/oginoino/toca/toca-calendar/internal/services"
)

// P1 / A3 — as rotas /api/v1/calendar/internal/* não aceitam mais o tenant
// escolhido pelo chamador.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A3) mostrou que os handlers internos liam o tenant do `?tenant_id=`
// (ou do corpo) com a chave interna compartilhada como única credencial:
// qualquer portador da chave lia/criava/apagava compromissos de qualquer tenant.
//
// Agora o grupo usa `auth.InternalOrUserMiddleware` e os handlers resolvem o
// tenant com `auth.ResolveTenantForInternalRequest` (helper internalTenant):
// credencial de serviço com allowlist só atende os tenants dela; chamada com JWT
// resolve pelo token e pedir outro tenant é 403. O dublê de repositório registra
// o tenant com que foi consultado — a asserção não é só o status, é a prova de
// que o tenant estrangeiro nunca chegou ao repositório.

// internalServiceIdentity injeta a identidade de um chamador de serviço já
// autenticado — o mesmo estado que `auth.InternalOrUserMiddleware` deixa no
// contexto em produção. Usado pelos testes que montam a rota sem o middleware.
func internalServiceIdentity(tenants ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(auth.CtxInternalAuthKind, auth.InternalAuthService)
		c.Set(auth.CtxInternalServiceName, "toca-crm")
		c.Set(auth.CtxInternalAllowedScope, tenants)
		c.Next()
	}
}

// recordingAppointmentRepo registra o tenant de cada acesso (dublê por
// embedding: só os métodos observados são sobrescritos).
type recordingAppointmentRepo struct {
	appointment.Repository
	mu             sync.Mutex
	rows           map[string]*appointment.Appointment
	listTenants    []string
	createdTenants []string
	updatedTenants []string
	ctxTenants     []string
	deletedIDs     []string
}

func newRecordingAppointmentRepo(seed ...*appointment.Appointment) *recordingAppointmentRepo {
	r := &recordingAppointmentRepo{rows: map[string]*appointment.Appointment{}}
	for _, a := range seed {
		r.rows[a.ID] = a
	}
	return r
}

func (r *recordingAppointmentRepo) record(dst *[]string, value string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Dedupe consecutivo: um mesmo request pode consultar o repositório duas
	// vezes (ex.: GetByID + Update) e as asserções continuam exatas.
	if n := len(*dst); n > 0 && (*dst)[n-1] == value {
		return
	}
	*dst = append(*dst, value)
}

func (r *recordingAppointmentRepo) snapshot(dst *[]string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), *dst...)
}

func (r *recordingAppointmentRepo) GetByID(ctx context.Context, id string) (*appointment.Appointment, error) {
	tenant, _ := appctx.GetTenantID(ctx)
	r.record(&r.ctxTenants, tenant)
	r.mu.Lock()
	row := r.rows[id]
	r.mu.Unlock()
	if row == nil {
		return nil, sql.ErrNoRows
	}
	cp := *row
	return &cp, nil
}

func (r *recordingAppointmentRepo) List(_ context.Context, tenantID string, _ appointment.ListFilters) ([]*appointment.Appointment, int64, error) {
	r.record(&r.listTenants, tenantID)
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []*appointment.Appointment{}
	for _, row := range r.rows {
		if row.TenantID != tenantID {
			continue
		}
		cp := *row
		out = append(out, &cp)
	}
	return out, int64(len(out)), nil
}

func (r *recordingAppointmentRepo) Create(_ context.Context, a *appointment.Appointment) error {
	r.record(&r.createdTenants, a.TenantID)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rows == nil {
		r.rows = map[string]*appointment.Appointment{}
	}
	cp := *a
	r.rows[a.ID] = &cp
	return nil
}

func (r *recordingAppointmentRepo) Update(_ context.Context, a *appointment.Appointment) error {
	r.record(&r.updatedTenants, a.TenantID)
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *a
	r.rows[a.ID] = &cp
	return nil
}

func (r *recordingAppointmentRepo) Delete(_ context.Context, tenantID, id string) error {
	r.record(&r.deletedIDs, id)
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rows, id)
	return nil
}

func (r *recordingAppointmentRepo) FindConflicts(_ context.Context, tenantID, _, _, _, _, _ string) ([]*appointment.Appointment, error) {
	r.record(&r.listTenants, tenantID)
	return nil, nil
}

// withNamedServiceKey publica uma credencial NOMEADA com allowlist de tenants no
// registro global lido pelo middleware (mesmo caminho de INTERNAL_SERVICE_KEYS em
// produção) e devolve a chave a apresentar no header X-Internal-Key.
func withNamedServiceKey(t *testing.T, service, key string, tenants ...string) string {
	t.Helper()
	entry := service + ":" + key
	if len(tenants) > 0 {
		entry += ":" + strings.Join(tenants, "|")
	}
	prev, had := os.LookupEnv("INTERNAL_SERVICE_KEYS")
	if err := os.Setenv("INTERNAL_SERVICE_KEYS", entry); err != nil {
		t.Fatalf("set INTERNAL_SERVICE_KEYS: %v", err)
	}
	auth.ReloadServiceKeyRegistry()
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("INTERNAL_SERVICE_KEYS", prev)
		} else {
			_ = os.Unsetenv("INTERNAL_SERVICE_KEYS")
		}
		auth.ReloadServiceKeyRegistry()
	})
	return key
}

// calendarTestJWKS serve um JWKS local e assina tokens no formato que o
// auth.Middleware valida.
type calendarTestJWKS struct {
	server *httptest.Server
	key    *rsa.PrivateKey
	kid    string
}

func newCalendarTestJWKS(t *testing.T) *calendarTestJWKS {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	j := &calendarTestJWKS{key: key, kid: "toca-calendar-test-kid"}
	payload := map[string]any{"keys": []map[string]any{{
		"kty": "RSA",
		"kid": j.kid,
		"use": "sig",
		"alg": "RS256",
		"n":   base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
	}}}
	j.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	return j
}

func (j *calendarTestJWKS) token(t *testing.T, tenantID string) string {
	t.Helper()
	claims := auth.Claims{
		AccountID:      "acc-1",
		Email:          "corretor@example.com",
		Name:           "Corretor",
		Role:           "member",
		Permission:     "member",
		FunctionalRole: "corretor",
		Type:           "company",
		TenantID:       tenantID,
		TokenType:      auth.TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "acc-1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = j.kid
	signed, err := tok.SignedString(j.key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

// calendarRouter sobe o router REAL do serviço (SetupRouter): rotas, middlewares
// e handlers de produção, com repositório dublê.
func calendarRouter(t *testing.T, jwksURL string, repo *recordingAppointmentRepo) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := config.BaseConfig{}
	cfg.JWT.SupabaseJWKSURL = jwksURL
	return SetupRouter(cfg, services.NewAppointmentService(repo, nil))
}

func seedAppointment(id, tenantID string) *appointment.Appointment {
	a := appointment.New(tenantID, "Visita", "2099-10-20", "09:00", "10:00", appointment.TypeVisita, appointment.PriorityMedia, "Cliente", "Corretor")
	a.ID = id
	return a
}

func doCalendarRequest(t *testing.T, r *gin.Engine, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// Sem chave interna e sem JWT a rota interna não responde nada de negócio:
// prova que o tenant não vem do query param.
func TestInternalCalendar_RejectsRequestWithoutInternalIdentity(t *testing.T) {
	repo := newRecordingAppointmentRepo(seedAppointment("appt-b", "tenant-vitima"))
	r := calendarRouter(t, "", repo)

	w := doCalendarRequest(t, r, http.MethodGet, "/api/v1/calendar/internal/appointments/appt-b?tenant_id=tenant-vitima", "", nil)

	if w.Code != http.StatusUnauthorized && w.Code != http.StatusForbidden {
		t.Fatalf("rota interna respondeu %d sem identidade de serviço/usuário: %s", w.Code, w.Body.String())
	}
	if seen := repo.snapshot(&repo.ctxTenants); len(seen) != 0 {
		t.Fatalf("o repositório foi consultado (%v) sem identidade", seen)
	}
}

// (a) Credencial de SERVIÇO restrita a tenant-a pedindo tenant-vitima: 403 e o
// repositório NUNCA vê o tenant estrangeiro — o cenário exato do A3.
func TestInternalAppointments_GetByID_ServiceCredentialCannotPickForeignTenant(t *testing.T) {
	repo := newRecordingAppointmentRepo(seedAppointment("appt-b", "tenant-vitima"))
	r := calendarRouter(t, "", repo)
	key := withNamedServiceKey(t, "toca-crm", "key-crm", "tenant-a")

	w := doCalendarRequest(t, r, http.MethodGet, "/api/v1/calendar/internal/appointments/appt-b?tenant_id=tenant-vitima", "",
		map[string]string{"X-Internal-Key": key})

	if w.Code != http.StatusForbidden {
		t.Fatalf("VAZAMENTO CROSS-TENANT: credencial restrita a tenant-a leu compromisso de tenant-vitima e recebeu %d (tenants consultados: %v): %s",
			w.Code, repo.snapshot(&repo.ctxTenants), w.Body.String())
	}
	if seen := repo.snapshot(&repo.ctxTenants); len(seen) != 0 {
		t.Fatalf("o repositório foi consultado com %v — a requisição nem deveria chegar lá", seen)
	}
}

// (a-legítimo) A mesma credencial, no tenant DA ALLOWLIST, continua funcionando e
// o handler propaga o tenant certo até o repositório.
func TestInternalAppointments_GetByID_ServiceCredentialOwnTenant_ReachesRepository(t *testing.T) {
	repo := newRecordingAppointmentRepo(seedAppointment("appt-a", "tenant-a"))
	r := calendarRouter(t, "", repo)
	key := withNamedServiceKey(t, "toca-crm", "key-crm", "tenant-a")

	w := doCalendarRequest(t, r, http.MethodGet, "/api/v1/calendar/internal/appointments/appt-a?tenant_id=tenant-a", "",
		map[string]string{"X-Internal-Key": key})

	if w.Code != http.StatusOK {
		t.Fatalf("chamada legítima deveria responder 200, veio %d: %s", w.Code, w.Body.String())
	}
	if seen := repo.snapshot(&repo.ctxTenants); len(seen) != 1 || seen[0] != "tenant-a" {
		t.Fatalf("o repositório deveria ser consultado com tenant-a, viu %v", seen)
	}
}

// (b) JWT de usuário: o tenant do TOKEN manda. Pedir outro é 403; o mesmo tenant
// (ou nenhum tenant no query) segue funcionando.
func TestInternalAppointments_UserTokenTenantWins(t *testing.T) {
	jwks := newCalendarTestJWKS(t)
	defer jwks.server.Close()

	repo := newRecordingAppointmentRepo(seedAppointment("appt-b", "tenant-vitima"), seedAppointment("appt-token", "tenant-do-token"))
	r := calendarRouter(t, jwks.server.URL, repo)
	token := jwks.token(t, "tenant-do-token")

	w := doCalendarRequest(t, r, http.MethodGet, "/api/v1/calendar/internal/appointments/appt-b?tenant_id=tenant-vitima", "",
		map[string]string{"Authorization": "Bearer " + token})
	if w.Code != http.StatusForbidden {
		t.Fatalf("VAZAMENTO CROSS-TENANT: token de tenant-do-token pediu tenant-vitima e recebeu %d (tenants consultados: %v)",
			w.Code, repo.snapshot(&repo.ctxTenants))
	}
	if seen := repo.snapshot(&repo.ctxTenants); len(seen) != 0 {
		t.Fatalf("o repositório foi consultado com %v — a requisição nem deveria chegar lá", seen)
	}

	w = doCalendarRequest(t, r, http.MethodGet, "/api/v1/calendar/internal/appointments/appt-token", "",
		map[string]string{"Authorization": "Bearer " + token})
	if w.Code != http.StatusOK {
		t.Fatalf("chamada de usuário sem tenant no query deveria usar o do token, veio %d: %s", w.Code, w.Body.String())
	}
	if seen := repo.snapshot(&repo.ctxTenants); len(seen) != 1 || seen[0] != "tenant-do-token" {
		t.Fatalf("o repositório deveria ser consultado com o tenant do TOKEN, viu %v", seen)
	}
}

// GET /calendar/internal/deals/:id/appointments — tenant no query.
func TestInternalAppointments_ListByDeal_TenantIsEnforced(t *testing.T) {
	t.Run("serviço com tenant estrangeiro", func(t *testing.T) {
		repo := newRecordingAppointmentRepo(seedAppointment("appt-b", "tenant-vitima"))
		r := calendarRouter(t, "", repo)
		key := withNamedServiceKey(t, "toca-deal", "key-deal", "tenant-a")

		w := doCalendarRequest(t, r, http.MethodGet, "/api/v1/calendar/internal/deals/deal-1/appointments?tenant_id=tenant-vitima", "",
			map[string]string{"X-Internal-Key": key})

		if w.Code != http.StatusForbidden {
			t.Fatalf("VAZAMENTO CROSS-TENANT: credencial restrita a tenant-a listou a agenda de tenant-vitima e recebeu %d (tenants consultados: %v)",
				w.Code, repo.snapshot(&repo.listTenants))
		}
		if seen := repo.snapshot(&repo.listTenants); len(seen) != 0 {
			t.Fatalf("o repositório foi consultado com %v — a requisição nem deveria chegar lá", seen)
		}
	})

	t.Run("serviço no próprio tenant", func(t *testing.T) {
		repo := newRecordingAppointmentRepo(seedAppointment("appt-a", "tenant-a"))
		r := calendarRouter(t, "", repo)
		key := withNamedServiceKey(t, "toca-deal", "key-deal", "tenant-a")

		w := doCalendarRequest(t, r, http.MethodGet, "/api/v1/calendar/internal/deals/deal-1/appointments?tenant_id=tenant-a", "",
			map[string]string{"X-Internal-Key": key})

		if w.Code != http.StatusOK {
			t.Fatalf("chamada legítima deveria responder 200, veio %d: %s", w.Code, w.Body.String())
		}
		if seen := repo.snapshot(&repo.listTenants); len(seen) != 1 || seen[0] != "tenant-a" {
			t.Fatalf("o repositório deveria ser consultado com tenant-a, viu %v", seen)
		}
	})

	t.Run("serviço sem tenant explícito", func(t *testing.T) {
		repo := newRecordingAppointmentRepo()
		r := calendarRouter(t, "", repo)
		key := withNamedServiceKey(t, "toca-deal", "key-deal")

		w := doCalendarRequest(t, r, http.MethodGet, "/api/v1/calendar/internal/deals/deal-1/appointments", "",
			map[string]string{"X-Internal-Key": key})

		if w.Code != http.StatusBadRequest && w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("serviço sem tenant explícito deveria ser recusado como validação, veio %d", w.Code)
		}
		if seen := repo.snapshot(&repo.listTenants); len(seen) != 0 {
			t.Fatalf("nenhuma consulta deveria acontecer, viu %v", seen)
		}
	})
}

// POST /calendar/internal/appointments — o tenant chega no CORPO.
func TestInternalAppointments_Create_BodyTenantIsEnforced(t *testing.T) {
	const body = `{"tenant_id":"%s","title":"Visita","date":"2099-10-20","time":"09:00","end_time":"10:00","type":"Visita","client":"Cliente","agent":"Corretor"}`

	t.Run("serviço com tenant estrangeiro no corpo", func(t *testing.T) {
		repo := newRecordingAppointmentRepo()
		r := calendarRouter(t, "", repo)
		key := withNamedServiceKey(t, "toca-crm", "key-crm", "tenant-a")

		w := doCalendarRequest(t, r, http.MethodPost, "/api/v1/calendar/internal/appointments",
			strings.Replace(body, "%s", "tenant-vitima", 1), map[string]string{"X-Internal-Key": key})

		if w.Code != http.StatusForbidden {
			t.Fatalf("VAZAMENTO CROSS-TENANT: credencial restrita a tenant-a criou compromisso em tenant-vitima (%d, tenants consultados: %v)",
				w.Code, repo.snapshot(&repo.createdTenants))
		}
		if seen := repo.snapshot(&repo.createdTenants); len(seen) != 0 {
			t.Fatalf("nenhuma gravação deveria acontecer, viu %v", seen)
		}
	})

	t.Run("serviço no próprio tenant", func(t *testing.T) {
		repo := newRecordingAppointmentRepo()
		r := calendarRouter(t, "", repo)
		key := withNamedServiceKey(t, "toca-crm", "key-crm", "tenant-a")

		w := doCalendarRequest(t, r, http.MethodPost, "/api/v1/calendar/internal/appointments",
			strings.Replace(body, "%s", "tenant-a", 1), map[string]string{"X-Internal-Key": key})

		if w.Code != http.StatusCreated {
			t.Fatalf("criação legítima deveria responder 201, veio %d: %s", w.Code, w.Body.String())
		}
		if seen := repo.snapshot(&repo.createdTenants); len(seen) != 1 || seen[0] != "tenant-a" {
			t.Fatalf("o compromisso deveria ser gravado em tenant-a, viu %v", seen)
		}
	})

	t.Run("usuário pedindo outro tenant no corpo", func(t *testing.T) {
		jwks := newCalendarTestJWKS(t)
		defer jwks.server.Close()

		repo := newRecordingAppointmentRepo()
		r := calendarRouter(t, jwks.server.URL, repo)
		token := jwks.token(t, "tenant-a")

		w := doCalendarRequest(t, r, http.MethodPost, "/api/v1/calendar/internal/appointments",
			strings.Replace(body, "%s", "tenant-vitima", 1), map[string]string{"Authorization": "Bearer " + token})

		if w.Code != http.StatusForbidden {
			t.Fatalf("VAZAMENTO CROSS-TENANT: token de tenant-a criou compromisso em tenant-vitima (%d, tenants consultados: %v)",
				w.Code, repo.snapshot(&repo.createdTenants))
		}
		if seen := repo.snapshot(&repo.createdTenants); len(seen) != 0 {
			t.Fatalf("nenhuma gravação deveria acontecer, viu %v", seen)
		}
	})

	t.Run("usuário no próprio tenant", func(t *testing.T) {
		jwks := newCalendarTestJWKS(t)
		defer jwks.server.Close()

		repo := newRecordingAppointmentRepo()
		r := calendarRouter(t, jwks.server.URL, repo)
		token := jwks.token(t, "tenant-a")

		w := doCalendarRequest(t, r, http.MethodPost, "/api/v1/calendar/internal/appointments",
			strings.Replace(body, "%s", "tenant-a", 1), map[string]string{"Authorization": "Bearer " + token})

		if w.Code != http.StatusCreated {
			t.Fatalf("criação legítima por usuário deveria responder 201, veio %d: %s", w.Code, w.Body.String())
		}
		if seen := repo.snapshot(&repo.createdTenants); len(seen) != 1 || seen[0] != "tenant-a" {
			t.Fatalf("o compromisso deveria ser gravado em tenant-a, viu %v", seen)
		}
	})
}

// PUT /calendar/internal/appointments/:id — tenant no corpo.
func TestInternalAppointments_Update_BodyTenantIsEnforced(t *testing.T) {
	t.Run("serviço com tenant estrangeiro no corpo", func(t *testing.T) {
		repo := newRecordingAppointmentRepo(seedAppointment("appt-b", "tenant-vitima"))
		r := calendarRouter(t, "", repo)
		key := withNamedServiceKey(t, "toca-crm", "key-crm", "tenant-a")

		w := doCalendarRequest(t, r, http.MethodPut, "/api/v1/calendar/internal/appointments/appt-b",
			`{"tenant_id":"tenant-vitima","title":"Invadido"}`, map[string]string{"X-Internal-Key": key})

		if w.Code != http.StatusForbidden {
			t.Fatalf("VAZAMENTO CROSS-TENANT: credencial restrita a tenant-a alterou compromisso de tenant-vitima (%d, tenants gravados: %v)",
				w.Code, repo.snapshot(&repo.updatedTenants))
		}
		if seen := repo.snapshot(&repo.updatedTenants); len(seen) != 0 {
			t.Fatalf("nenhuma gravação deveria acontecer, viu %v", seen)
		}
	})

	t.Run("serviço no próprio tenant", func(t *testing.T) {
		repo := newRecordingAppointmentRepo(seedAppointment("appt-a", "tenant-a"))
		r := calendarRouter(t, "", repo)
		key := withNamedServiceKey(t, "toca-crm", "key-crm", "tenant-a")

		w := doCalendarRequest(t, r, http.MethodPut, "/api/v1/calendar/internal/appointments/appt-a",
			`{"tenant_id":"tenant-a","title":"Remarcada"}`, map[string]string{"X-Internal-Key": key})

		if w.Code != http.StatusOK {
			t.Fatalf("alteração legítima deveria responder 200, veio %d: %s", w.Code, w.Body.String())
		}
		if seen := repo.snapshot(&repo.updatedTenants); len(seen) != 1 || seen[0] != "tenant-a" {
			t.Fatalf("a gravação deveria acontecer em tenant-a, viu %v", seen)
		}
	})

	t.Run("usuário pedindo outro tenant no corpo", func(t *testing.T) {
		jwks := newCalendarTestJWKS(t)
		defer jwks.server.Close()

		repo := newRecordingAppointmentRepo(seedAppointment("appt-b", "tenant-vitima"))
		r := calendarRouter(t, jwks.server.URL, repo)
		token := jwks.token(t, "tenant-a")

		w := doCalendarRequest(t, r, http.MethodPut, "/api/v1/calendar/internal/appointments/appt-b",
			`{"tenant_id":"tenant-vitima","title":"Invadido"}`, map[string]string{"Authorization": "Bearer " + token})

		if w.Code != http.StatusForbidden {
			t.Fatalf("VAZAMENTO CROSS-TENANT: token de tenant-a alterou compromisso de tenant-vitima (%d, tenants gravados: %v)",
				w.Code, repo.snapshot(&repo.updatedTenants))
		}
		if seen := repo.snapshot(&repo.updatedTenants); len(seen) != 0 {
			t.Fatalf("nenhuma gravação deveria acontecer, viu %v", seen)
		}
	})
}

// DELETE /calendar/internal/appointments/:id — o tenant não vinha do request (o
// handler usava o tenant DO REGISTRO): agora a credencial precisa dizer o tenant
// e ele tem de bater com o do compromisso.
func TestInternalAppointments_Delete_RequiresTenantOfTheRow(t *testing.T) {
	t.Run("tenant estrangeiro no query", func(t *testing.T) {
		repo := newRecordingAppointmentRepo(seedAppointment("appt-b", "tenant-vitima"))
		r := calendarRouter(t, "", repo)
		key := withNamedServiceKey(t, "toca-crm", "key-crm", "tenant-a")

		w := doCalendarRequest(t, r, http.MethodDelete, "/api/v1/calendar/internal/appointments/appt-b?tenant_id=tenant-vitima", "",
			map[string]string{"X-Internal-Key": key})

		if w.Code != http.StatusForbidden {
			t.Fatalf("VAZAMENTO CROSS-TENANT: credencial restrita a tenant-a apagou compromisso de tenant-vitima (%d, apagados: %v)",
				w.Code, repo.snapshot(&repo.deletedIDs))
		}
		if seen := repo.snapshot(&repo.deletedIDs); len(seen) != 0 {
			t.Fatalf("nenhuma exclusão deveria acontecer, viu %v", seen)
		}
	})

	t.Run("tenant próprio com registro de outro tenant", func(t *testing.T) {
		repo := newRecordingAppointmentRepo(seedAppointment("appt-b", "tenant-vitima"))
		r := calendarRouter(t, "", repo)
		key := withNamedServiceKey(t, "toca-crm", "key-crm", "tenant-a")

		w := doCalendarRequest(t, r, http.MethodDelete, "/api/v1/calendar/internal/appointments/appt-b?tenant_id=tenant-a", "",
			map[string]string{"X-Internal-Key": key})

		if w.Code != http.StatusForbidden {
			t.Fatalf("registro de outro tenant não pode ser apagado por tenant-a (%d, apagados: %v)",
				w.Code, repo.snapshot(&repo.deletedIDs))
		}
		if seen := repo.snapshot(&repo.deletedIDs); len(seen) != 0 {
			t.Fatalf("nenhuma exclusão deveria acontecer, viu %v", seen)
		}
	})

	t.Run("tenant próprio com registro próprio", func(t *testing.T) {
		repo := newRecordingAppointmentRepo(seedAppointment("appt-a", "tenant-a"))
		r := calendarRouter(t, "", repo)
		key := withNamedServiceKey(t, "toca-crm", "key-crm", "tenant-a")

		w := doCalendarRequest(t, r, http.MethodDelete, "/api/v1/calendar/internal/appointments/appt-a?tenant_id=tenant-a", "",
			map[string]string{"X-Internal-Key": key})

		if w.Code != http.StatusOK {
			t.Fatalf("exclusão legítima deveria responder 200, veio %d: %s", w.Code, w.Body.String())
		}
		if seen := repo.snapshot(&repo.deletedIDs); len(seen) != 1 || seen[0] != "appt-a" {
			t.Fatalf("o compromisso appt-a deveria ter sido apagado, viu %v", seen)
		}
	})
}
