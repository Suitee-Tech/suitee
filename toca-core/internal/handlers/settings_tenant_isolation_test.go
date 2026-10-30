package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca-core/internal/domain/core/settings"
	service "github.com/oginoino/toca-core/internal/services"
)

// P0 / C6 — isolamento de tenant no DELETE de webhooks e no REVOKE de API keys.
//
// O handler tem o tenant no contexto e precisa repassá-lo até o repositório;
// um repositório que não filtra por tenant deixa o tenant A apagar recursos do
// tenant B pelo UUID. Estes testes usam fakes que replicam fielmente o
// contrato do banco: 0 linhas afetadas => sql.ErrNoRows => 404.
//
// P0 / A1 — o segredo de assinatura do webhook só pode aparecer UMA vez, na
// resposta do POST de criação. O GET de listagem nunca pode serializá-lo.

// --- Fakes dos repositórios de settings -------------------------------------

type fakeWebhookRepo struct {
	rows map[string]settings.WebhookEndpoint
}

func newFakeWebhookRepo(rows ...settings.WebhookEndpoint) *fakeWebhookRepo {
	m := make(map[string]settings.WebhookEndpoint, len(rows))
	for _, r := range rows {
		m[r.ID.(string)] = r
	}
	return &fakeWebhookRepo{rows: m}
}

func (r *fakeWebhookRepo) Create(_ context.Context, w *settings.WebhookEndpoint) error {
	if w.ID == nil {
		w.ID = "wh-generated"
	}
	r.rows[w.ID.(string)] = *w
	return nil
}

func (r *fakeWebhookRepo) GetByID(_ context.Context, id string) (*settings.WebhookEndpoint, error) {
	w, ok := r.rows[id]
	if !ok {
		return nil, nil
	}
	return &w, nil
}

func (r *fakeWebhookRepo) ListByTenantID(_ context.Context, tenantID string) ([]settings.WebhookEndpoint, error) {
	out := make([]settings.WebhookEndpoint, 0)
	for _, w := range r.rows {
		if w.TenantID == tenantID {
			out = append(out, w)
		}
	}
	return out, nil
}

func (r *fakeWebhookRepo) Update(_ context.Context, w *settings.WebhookEndpoint) error {
	id, _ := w.ID.(string)
	cur, ok := r.rows[id]
	if !ok || cur.TenantID != w.TenantID {
		return sql.ErrNoRows
	}
	r.rows[id] = *w
	return nil
}

// Delete replica o SQL CORRIGIDO: o tenant_id entra no WHERE junto com o id e
// 0 linhas afetadas devolve sql.ErrNoRows (=> 404 no handler).
func (r *fakeWebhookRepo) Delete(_ context.Context, tenantID, id string) error {
	cur, ok := r.rows[id]
	if !ok || cur.TenantID != tenantID {
		return sql.ErrNoRows
	}
	delete(r.rows, id)
	return nil
}

type fakeAPIKeyRepo struct {
	rows map[string]settings.APIKey
}

func newFakeAPIKeyRepo(rows ...settings.APIKey) *fakeAPIKeyRepo {
	m := make(map[string]settings.APIKey, len(rows))
	for _, r := range rows {
		m[r.ID.(string)] = r
	}
	return &fakeAPIKeyRepo{rows: m}
}

func (r *fakeAPIKeyRepo) Create(_ context.Context, k *settings.APIKey) error {
	if k.ID == nil {
		k.ID = "key-generated"
	}
	r.rows[k.ID.(string)] = *k
	return nil
}

func (r *fakeAPIKeyRepo) ListByTenantID(_ context.Context, tenantID string) ([]settings.APIKey, error) {
	out := make([]settings.APIKey, 0)
	for _, k := range r.rows {
		if k.TenantID == tenantID {
			out = append(out, k)
		}
	}
	return out, nil
}

// Revoke replica o SQL CORRIGIDO: tenant_id no WHERE e 0 linhas => ErrNoRows.
func (r *fakeAPIKeyRepo) Revoke(_ context.Context, tenantID, id string) error {
	k, ok := r.rows[id]
	if !ok || k.TenantID != tenantID {
		return sql.ErrNoRows
	}
	k.IsActive = false
	r.rows[id] = k
	return nil
}

// fakePermissionsRepo devolve a matriz pedida e nada mais.
type fakePermissionsRepo struct {
	matrix *settings.RolePermissions
}

func (r *fakePermissionsRepo) GetByTenantID(_ context.Context, tenantID string) (*settings.RolePermissions, error) {
	if r.matrix == nil {
		return nil, nil
	}
	m := *r.matrix
	m.TenantID = tenantID
	return &m, nil
}

func (r *fakePermissionsRepo) Upsert(_ context.Context, _ *settings.RolePermissions) error {
	return nil
}

// --- Helpers ---------------------------------------------------------------

const (
	tenantAtacante = "tenant-atacante"
	tenantVitima   = "tenant-vitima"
)

func withTenant(tenantID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("tenant_id", tenantID)
		c.Next()
	}
}

func newSettingsHandlerForTest(t *testing.T, webhooks *fakeWebhookRepo, keys *fakeAPIKeyRepo) *SettingsHandler {
	t.Helper()
	svc := service.NewSettingsService(nil, nil, &fakePermissionsRepo{}, nil, webhooks, keys, nil)
	return NewSettingsHandler(svc)
}

// --- C6: webhooks -----------------------------------------------------------

func TestDeleteWebhook_NaoApagaWebhookDeOutroTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)

	vitima := settings.WebhookEndpoint{ID: "wh-vitima", TenantID: tenantVitima, URL: "https://vitima.example/hook", Secret: "whsec_vitima"}
	webhooks := newFakeWebhookRepo(vitima)
	h := newSettingsHandlerForTest(t, webhooks, newFakeAPIKeyRepo())

	r := gin.New()
	r.DELETE("/company-settings/webhooks/:id", withTenant(tenantAtacante), h.DeleteWebhook)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/company-settings/webhooks/wh-vitima", nil))

	if _, aindaExiste := webhooks.rows["wh-vitima"]; !aindaExiste {
		t.Fatalf("VAZAMENTO CROSS-TENANT: o tenant %q apagou o webhook do tenant %q (status=%d body=%s)",
			tenantAtacante, tenantVitima, rec.Code, rec.Body.String())
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("esperava 404 (nada afetado no próprio tenant), veio %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteWebhook_DoProprioTenantFunciona(t *testing.T) {
	gin.SetMode(gin.TestMode)

	webhooks := newFakeWebhookRepo(settings.WebhookEndpoint{ID: "wh-proprio", TenantID: tenantAtacante, URL: "https://a.example/hook"})
	h := newSettingsHandlerForTest(t, webhooks, newFakeAPIKeyRepo())

	r := gin.New()
	r.DELETE("/company-settings/webhooks/:id", withTenant(tenantAtacante), h.DeleteWebhook)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/company-settings/webhooks/wh-proprio", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200 no próprio tenant, veio %d: %s", rec.Code, rec.Body.String())
	}
	if _, aindaExiste := webhooks.rows["wh-proprio"]; aindaExiste {
		t.Fatal("o webhook do próprio tenant deveria ter sido removido")
	}
}

func TestDeleteWebhook_NoOpRetorna404(t *testing.T) {
	gin.SetMode(gin.TestMode)

	webhooks := newFakeWebhookRepo()
	h := newSettingsHandlerForTest(t, webhooks, newFakeAPIKeyRepo())

	r := gin.New()
	r.DELETE("/company-settings/webhooks/:id", withTenant(tenantAtacante), h.DeleteWebhook)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/company-settings/webhooks/inexistente", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("0 linhas afetadas deveria responder 404, veio %d: %s", rec.Code, rec.Body.String())
	}
}

// --- C6: API keys -----------------------------------------------------------

func TestRevokeAPIKey_NaoRevogaChaveDeOutroTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)

	vitima := settings.APIKey{ID: "key-vitima", TenantID: tenantVitima, Name: "chave da vítima", IsActive: true}
	keys := newFakeAPIKeyRepo(vitima)
	h := newSettingsHandlerForTest(t, newFakeWebhookRepo(), keys)

	r := gin.New()
	r.DELETE("/company-settings/api-keys/:id", withTenant(tenantAtacante), h.RevokeAPIKey)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/company-settings/api-keys/key-vitima", nil))

	if !keys.rows["key-vitima"].IsActive {
		t.Fatalf("VAZAMENTO CROSS-TENANT: o tenant %q revogou a API key do tenant %q (status=%d body=%s)",
			tenantAtacante, tenantVitima, rec.Code, rec.Body.String())
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("esperava 404 (nada afetado no próprio tenant), veio %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRevokeAPIKey_NoOpRetorna404(t *testing.T) {
	gin.SetMode(gin.TestMode)

	keys := newFakeAPIKeyRepo()
	h := newSettingsHandlerForTest(t, newFakeWebhookRepo(), keys)

	r := gin.New()
	r.DELETE("/company-settings/api-keys/:id", withTenant(tenantAtacante), h.RevokeAPIKey)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/company-settings/api-keys/inexistente", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("0 linhas afetadas deveria responder 404, veio %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRevokeAPIKey_DoProprioTenantFunciona(t *testing.T) {
	gin.SetMode(gin.TestMode)

	keys := newFakeAPIKeyRepo(settings.APIKey{ID: "key-propria", TenantID: tenantAtacante, IsActive: true})
	h := newSettingsHandlerForTest(t, newFakeWebhookRepo(), keys)

	r := gin.New()
	r.DELETE("/company-settings/api-keys/:id", withTenant(tenantAtacante), h.RevokeAPIKey)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/company-settings/api-keys/key-propria", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200 no próprio tenant, veio %d: %s", rec.Code, rec.Body.String())
	}
	if keys.rows["key-propria"].IsActive {
		t.Fatal("a API key do próprio tenant deveria ter sido revogada")
	}
}

// --- A1: o segredo do webhook é write-only ----------------------------------

func TestListWebhooks_NaoSerializaSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)

	webhooks := newFakeWebhookRepo(settings.WebhookEndpoint{
		ID: "wh-1", TenantID: tenantAtacante, URL: "https://a.example/hook",
		Events: []string{"lead.created"}, Active: true, Secret: "whsec_super_secreto",
	})
	h := newSettingsHandlerForTest(t, webhooks, newFakeAPIKeyRepo())

	r := gin.New()
	r.GET("/company-settings/webhooks", withTenant(tenantAtacante), h.ListWebhooks)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/company-settings/webhooks", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, veio %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "whsec_super_secreto") {
		t.Fatalf("VAZAMENTO DE SEGREDO: GET /company-settings/webhooks devolveu o signing secret: %s", rec.Body.String())
	}

	var lista []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &lista); err != nil {
		t.Fatalf("resposta não é uma lista JSON: %v (%s)", err, rec.Body.String())
	}
	if len(lista) != 1 {
		t.Fatalf("esperava 1 webhook, veio %d", len(lista))
	}
	if _, existe := lista[0]["secret"]; existe {
		t.Fatalf("a chave \"secret\" não pode aparecer na listagem: %s", rec.Body.String())
	}
	if lista[0]["url"] != "https://a.example/hook" {
		t.Fatalf("o restante do recurso deve continuar sendo serializado: %s", rec.Body.String())
	}
}

func TestCreateWebhook_DevolveSecretUmaVezEPersiste(t *testing.T) {
	gin.SetMode(gin.TestMode)

	webhooks := newFakeWebhookRepo()
	h := newSettingsHandlerForTest(t, webhooks, newFakeAPIKeyRepo())

	r := gin.New()
	r.POST("/company-settings/webhooks", withTenant(tenantAtacante), h.CreateWebhook)
	r.GET("/company-settings/webhooks", withTenant(tenantAtacante), h.ListWebhooks)

	body := bytes.NewBufferString(`{"url":"https://a.example/hook","events":["lead.created"],"active":true}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/company-settings/webhooks", body)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("esperava 201, veio %d: %s", rec.Code, rec.Body.String())
	}
	var criado map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &criado); err != nil {
		t.Fatalf("resposta de criação inválida: %v (%s)", err, rec.Body.String())
	}
	secret, _ := criado["secret"].(string)
	if !strings.HasPrefix(secret, "whsec_") {
		t.Fatalf("a resposta de criação deve devolver o secret uma única vez: %s", rec.Body.String())
	}

	// O segredo precisa continuar sendo PERSISTIDO (é o que permite assinar as
	// entregas); só não pode ser serializado na API.
	armazenado, ok := webhooks.rows[criado["id"].(string)]
	if !ok {
		t.Fatalf("webhook não foi persistido: %v", criado)
	}
	if armazenado.Secret != secret {
		t.Fatalf("o secret persistido difere do devolvido na criação: %q != %q", armazenado.Secret, secret)
	}

	// ...e nunca mais pode aparecer.
	recList := httptest.NewRecorder()
	r.ServeHTTP(recList, httptest.NewRequest(http.MethodGet, "/company-settings/webhooks", nil))
	if strings.Contains(recList.Body.String(), secret) {
		t.Fatalf("o secret reapareceu na listagem: %s", recList.Body.String())
	}
}
