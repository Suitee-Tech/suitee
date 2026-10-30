package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
)

// accessEnvelope monta a resposta do endpoint interno de acesso do plano.
func accessEnvelope(controlEnabled, entitled bool, status, reason string, features ...string) string {
	body := map[string]any{
		"success": true,
		"data": map[string]any{
			"access_control_enabled": controlEnabled,
			"entitled":               entitled,
			"status":                 status,
			"reason":                 reason,
			"features":               features,
			"plan":                   map[string]any{"tier": "pro"},
		},
	}
	raw, _ := json.Marshal(body)
	return string(raw)
}

// accessServer sobe um toca-financial falso e conta as chamadas.
func accessServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	return server, &calls
}

func runRequest(t *testing.T, middleware gin.HandlerFunc, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware)
	router.Handle(method, path, func(c *gin.Context) { c.String(http.StatusOK, "proxied") })
	request := httptest.NewRequest(method, path, nil)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestRequireActiveSubscription_BlocksWritesAndPreservesReads(t *testing.T) {
	entitled := map[string]string{"Authorization": "Bearer user-token"}
	server, calls := accessServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer user-token" {
			t.Errorf("token do usuário não foi repassado: %q", got)
		}
		_, _ = w.Write([]byte(accessEnvelope(true, false, "EXPIRED", "trial_or_paid_period_ended")))
	})

	middleware := RequireActiveSubscription(server.URL, true)
	if recorder := runRequest(t, middleware, http.MethodGet, "/api/v1/leads", entitled); recorder.Code != http.StatusOK {
		t.Fatalf("leitura deveria passar após o fim do período, veio %d", recorder.Code)
	}
	if got := atomic.LoadInt32(calls); got != 0 {
		t.Fatalf("leitura não deveria consultar o plano (evita hop extra), chamadas=%d", got)
	}

	recorder := runRequest(t, middleware, http.MethodPost, "/api/v1/leads", entitled)
	if recorder.Code != http.StatusPaymentRequired {
		t.Fatalf("gravação deveria ser bloqueada com 402, veio %d", recorder.Code)
	}
	var payload struct {
		Code string `json:"code"`
		Data struct {
			Feature    string `json:"feature"`
			UpgradeURL string `json:"upgrade_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("corpo inválido: %v (%s)", err, recorder.Body.String())
	}
	if payload.Code != "plan_required" || payload.Data.UpgradeURL != "/assinatura" {
		t.Fatalf("resposta de bloqueio fora do contrato: %s", recorder.Body.String())
	}
	if payload.Data.Feature != "subscription" {
		t.Fatalf("feature do bloqueio = %q, esperado subscription", payload.Data.Feature)
	}
}

func TestRequireActiveSubscription_AllowsEntitledWritesAndSkipsExemptPaths(t *testing.T) {
	server, calls := accessServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(accessEnvelope(true, true, "TRIALING", "", "reports")))
	})
	middleware := RequireActiveSubscription(server.URL, true)

	if recorder := runRequest(t, middleware, http.MethodPost, "/api/v1/leads", map[string]string{"Authorization": "Bearer t"}); recorder.Code != http.StatusOK {
		t.Fatalf("assinatura vigente deveria gravar, veio %d", recorder.Code)
	}
	// Assinatura, sessão e portal ficam fora do bloqueio de gravação: é por onde
	// o cliente regulariza e por onde o portal do cliente continua funcionando.
	for _, path := range []string{"/api/v1/platform-subscription/checkout", "/api/v1/auth/refresh", "/api/v1/client-portal/invites"} {
		if recorder := runRequest(t, middleware, http.MethodPost, path, map[string]string{"Authorization": "Bearer t"}); recorder.Code != http.StatusOK {
			t.Errorf("%s deveria passar sem consultar o plano, veio %d", path, recorder.Code)
		}
	}
	// Webhooks do provedor não carregam token de usuário: quem valida é o
	// segredo/assinatura do próprio provedor, não o plano.
	if recorder := runRequest(t, middleware, http.MethodPost, "/webhooks/abacatepay", nil); recorder.Code != http.StatusOK {
		t.Fatalf("webhook deveria passar, veio %d", recorder.Code)
	}
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("só a gravação autenticada deveria consultar o plano, chamadas=%d", got)
	}
}

func TestRequireActiveSubscription_FailsClosedWhenAccessIsUnknown(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"serviço indisponível", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }},
		{"envelope inválido", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"success":false}`)) }},
		{"json quebrado", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{`)) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, _ := accessServer(t, tc.handler)
			middleware := RequireActiveSubscription(server.URL, true)
			// A regressão que este caso cobre: envelope ausente não pode virar
			// nil dereference (panic no gateway) nem gravação liberada.
			recorder := runRequest(t, middleware, http.MethodPost, "/api/v1/leads", map[string]string{"Authorization": "Bearer t"})
			if recorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("esperava 503, veio %d (%s)", recorder.Code, recorder.Body.String())
			}
			var payload struct {
				Code string `json:"code"`
			}
			_ = json.Unmarshal(recorder.Body.Bytes(), &payload)
			if payload.Code != "plan_access_unavailable" {
				t.Fatalf("código = %q, esperado plan_access_unavailable", payload.Code)
			}
		})
	}
}

func TestRequireActiveSubscription_DisabledFlagNeverConsultsThePlan(t *testing.T) {
	server, calls := accessServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("com o enforcement desligado o plano não deve ser consultado")
	})
	middleware := RequireActiveSubscription(server.URL, false)
	if recorder := runRequest(t, middleware, http.MethodPost, "/api/v1/leads", map[string]string{"Authorization": "Bearer t"}); recorder.Code != http.StatusOK {
		t.Fatalf("enforcement desligado deveria liberar, veio %d", recorder.Code)
	}
	if got := atomic.LoadInt32(calls); got != 0 {
		t.Fatalf("chamadas inesperadas: %d", got)
	}
}

func TestRequirePlanFeature_SeparatesStarterFromPro(t *testing.T) {
	entitled := map[string]string{"Authorization": "Bearer t"}
	server, calls := accessServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(accessEnvelope(true, true, "ACTIVE", "", "crm", "finance", "rentals")))
	})
	middleware := RequirePlanFeature(server.URL, "reports", true, false)

	recorder := runRequest(t, middleware, http.MethodGet, "/api/v1/reports", entitled)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("Starter não tem relatórios: esperava 403, veio %d", recorder.Code)
	}
	var payload struct {
		Code string `json:"code"`
		Data struct {
			Feature    string `json:"feature"`
			UpgradeURL string `json:"upgrade_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("corpo inválido: %v", err)
	}
	if payload.Code != "plan_upgrade_required" || payload.Data.Feature != "reports" || payload.Data.UpgradeURL != "/assinatura" {
		t.Fatalf("contrato do gate fora do esperado: %s", recorder.Body.String())
	}
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("esperava uma consulta ao plano, veio %d", got)
	}
}

func TestRequirePlanFeature_ExpiredTenantGetsPlanRequiredAndMutatingOnlySkipsReads(t *testing.T) {
	server, calls := accessServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(accessEnvelope(true, false, "EXPIRED", "trial_or_paid_period_ended")))
	})
	entitled := map[string]string{"Authorization": "Bearer t"}

	// Leitura de recurso Pro com período encerrado: precisa dizer que o acesso
	// terminou (402), não que o plano é outro (403).
	readGate := RequirePlanFeature(server.URL, "reports", true, false)
	if recorder := runRequest(t, readGate, http.MethodGet, "/api/v1/reports", entitled); recorder.Code != http.StatusPaymentRequired {
		t.Fatalf("esperava 402 para acesso encerrado, veio %d", recorder.Code)
	}

	// Gate de mutação (assinatura digital): a leitura do recurso continua
	// disponível para o cliente ver o que já existe.
	mutationGate := RequirePlanFeature(server.URL, "electronic_signature", true, true)
	if recorder := runRequest(t, mutationGate, http.MethodGet, "/api/v1/signature/envelopes", entitled); recorder.Code != http.StatusOK {
		t.Fatalf("leitura deveria passar no gate de mutação, veio %d", recorder.Code)
	}
	if recorder := runRequest(t, mutationGate, http.MethodPost, "/api/v1/signature/envelopes", entitled); recorder.Code != http.StatusPaymentRequired {
		t.Fatalf("mutação deveria ser bloqueada, veio %d", recorder.Code)
	}
	if got := atomic.LoadInt32(calls); got != 2 {
		t.Fatalf("esperava duas consultas (leitura de recurso + mutação), veio %d", got)
	}
}

func TestRequirePlanFeature_ControlDisabledOrMissingTokenPassesThrough(t *testing.T) {
	// Enforcement desligado no serviço de assinatura: a decisão chega com
	// control_enabled=false e nada é bloqueado, mesmo para quem não tem o plano.
	server, calls := accessServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(accessEnvelope(false, true, "UNRESTRICTED", "", "crm")))
	})
	middleware := RequirePlanFeature(server.URL, "reports", true, false)
	if recorder := runRequest(t, middleware, http.MethodGet, "/api/v1/reports", map[string]string{"Authorization": "Bearer t"}); recorder.Code != http.StatusOK {
		t.Fatalf("controle desligado deveria liberar, veio %d", recorder.Code)
	}
	// Sem token, a autenticação é problema do serviço de destino (401 dele).
	if recorder := runRequest(t, middleware, http.MethodGet, "/api/v1/reports", nil); recorder.Code != http.StatusOK {
		t.Fatalf("sem token deveria seguir para o serviço, veio %d", recorder.Code)
	}
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("só a chamada autenticada consulta o plano, veio %d", got)
	}
}

// O lookup do plano é da EQUIPE: o endpoint de assinatura recusa o papel
// `cliente` e exige token válido. Quando ele recusa o próprio token do
// chamador, a regra de plano não se aplica — quem decide é o serviço de
// destino. Sem isso, o cliente no portal recebia 403 ao salvar as preferências
// dele (PATCH /settings/preferences) e o portal ficava inutilizável para
// gravações que não são da equipe.
func TestRequireActiveSubscription_PassesThroughWhenIdentityRefusesTheCaller(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
	}{
		{"cliente no portal (403 do lookup)", http.StatusForbidden},
		{"token expirado (401 do lookup)", http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, _ := accessServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status) })
			middleware := RequireActiveSubscription(server.URL, true)
			recorder := runRequest(t, middleware, http.MethodPatch, "/api/v1/settings/preferences", map[string]string{"Authorization": "Bearer portal"})
			if recorder.Code != http.StatusOK {
				t.Fatalf("esperava a requisição seguir para o serviço de destino, veio %d (%s)", recorder.Code, recorder.Body.String())
			}
			if body := recorder.Body.String(); body != "proxied" {
				t.Fatalf("corpo = %q, esperado o do serviço de destino", body)
			}
		})
	}
}

// O gate de recurso segue a mesma regra: o cliente do portal resolve a
// autorização no serviço de destino (permissão do recurso), não no plano.
func TestRequirePlanFeature_PassesThroughWhenIdentityRefusesTheCaller(t *testing.T) {
	server, _ := accessServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) })
	middleware := RequirePlanFeature(server.URL, "reports", true, false)
	recorder := runRequest(t, middleware, http.MethodGet, "/api/v1/reports/financial/kpis", map[string]string{"Authorization": "Bearer portal"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("esperava seguir para o destino, veio %d (%s)", recorder.Code, recorder.Body.String())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Incidente dos relatórios financeiros (out/2026): /api/v1/reports/financial/*
// devolvia 503 em beta e produção porque a consulta de plano (chamada HTTP
// interna ao serviço de assinatura) respondeu algo que não era 2xx. A tela era
// de LEITURA e ficava refém do serviço de billing.
//
// Regra nova: falha de INFRAESTRUTURA na consulta de plano não derruba leitura
// — a requisição segue para o serviço de destino, que revalida a permissão do
// recurso (toca-financial exige reports:read/finance:read). Escrita continua
// fechada, e uma decisão REAL de plano (402/403) continua bloqueando.
// ─────────────────────────────────────────────────────────────────────────────

// leituraSobreviveAFalhaDePlano cobre os três modos de falha de infraestrutura
// que antes viravam 503 numa tela de leitura.
func TestRequirePlanFeature_AllowsReadWhenPlanServiceIsUnavailable(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"serviço responde 500", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }},
		{"serviço responde 502", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadGateway) }},
		{"envelope inválido", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"success":false}`)) }},
		{"json quebrado", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{`)) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, _ := accessServer(t, tc.handler)
			middleware := RequirePlanFeature(server.URL, "reports", true, false)

			recorder := runRequest(t, middleware, http.MethodGet, "/api/v1/reports/financial/kpis", map[string]string{"Authorization": "Bearer t"})

			if recorder.Code != http.StatusOK {
				t.Fatalf("leitura deveria seguir para o destino, veio %d (%s)", recorder.Code, recorder.Body.String())
			}
			if body := recorder.Body.String(); body != "proxied" {
				t.Fatalf("corpo = %q, esperado o do serviço de destino", body)
			}
		})
	}
}

// A liberação é só de leitura: a mesma falha não pode liberar escrita, senão o
// bloqueio de assinatura (a razão de o gate existir) deixa de valer.
func TestRequirePlanFeature_StillBlocksWriteWhenPlanServiceIsUnavailable(t *testing.T) {
	server, _ := accessServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) })
	// mutatingOnly=false: o gate avalia todos os métodos, então a escrita
	// chega na consulta de plano e precisa continuar fechada.
	middleware := RequirePlanFeature(server.URL, "electronic_signature", true, false)

	recorder := runRequest(t, middleware, http.MethodPost, "/api/v1/signature/envelopes", map[string]string{"Authorization": "Bearer t"})

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("escrita deveria continuar fechada com 503, veio %d (%s)", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(recorder.Body.Bytes(), &payload)
	if payload.Code != "plan_access_unavailable" {
		t.Fatalf("código = %q, esperado plan_access_unavailable", payload.Code)
	}
}

// O fallback não pode mascarar decisão real de plano: com envelope válido
// dizendo "não tem o recurso", a leitura continua bloqueada com 403 e o código
// de upgrade. Sem este caso, "falha aberto" viraria "Pro de graça".
func TestRequirePlanFeature_RealDenialStillBlocksRead(t *testing.T) {
	server, _ := accessServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(accessEnvelope(true, true, "ACTIVE", "", "client_portal")))
	})
	middleware := RequirePlanFeature(server.URL, "reports", true, false)

	recorder := runRequest(t, middleware, http.MethodGet, "/api/v1/reports/financial/kpis", map[string]string{"Authorization": "Bearer t"})

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("esperava 403 (recurso fora do plano), veio %d (%s)", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(recorder.Body.Bytes(), &payload)
	if payload.Code != "plan_upgrade_required" {
		t.Fatalf("código = %q, esperado plan_upgrade_required", payload.Code)
	}
}

// O 503 do incidente não dizia QUAL status o upstream devolveu — diagnosticar
// exigia entrar no host. O corpo agora carrega o status real.
func TestRequirePlanFeature_ErrorExposesUpstreamStatus(t *testing.T) {
	server, _ := accessServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadGateway) })
	middleware := RequirePlanFeature(server.URL, "reports", true, false)

	recorder := runRequest(t, middleware, http.MethodPost, "/api/v1/reports/financial/kpis", map[string]string{"Authorization": "Bearer t"})

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("esperava 503, veio %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "plan_access_unavailable") {
		t.Fatalf("corpo deveria identificar a falha de plano, veio %s", recorder.Body.String())
	}
}

// A mesma regra vale para o bloqueio global de assinatura: leitura preservada.
func TestRequireActiveSubscription_AllowsReadWhenPlanServiceIsUnavailable(t *testing.T) {
	server, _ := accessServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) })
	middleware := RequireActiveSubscription(server.URL, true)

	recorder := runRequest(t, middleware, http.MethodGet, "/api/v1/leads", map[string]string{"Authorization": "Bearer t"})

	if recorder.Code != http.StatusOK {
		t.Fatalf("leitura deveria seguir para o destino, veio %d (%s)", recorder.Code, recorder.Body.String())
	}
}
