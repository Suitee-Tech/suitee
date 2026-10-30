package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/toca-signature/internal/domain/envelope"
)

// inertEnvelopeRepo registra o tenant com que o handler consultou e devolve um
// envelope vazio. Sem ele os testes de escopo morreriam em panic (repo nil) em
// vez de mostrar o vazamento na asserção.
type inertEnvelopeRepo struct {
	envelope.Repository
	tenantRecebido string
}

func (r *inertEnvelopeRepo) GetByContractID(_ context.Context, tenantID, _ string) (*envelope.SignatureEnvelope, error) {
	r.tenantRecebido = tenantID
	return &envelope.SignatureEnvelope{ID: "env-1", TenantID: tenantID}, nil
}

// P1 / A3 — rota interna não aceita tenant escolhido pelo chamador.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A3) mostrou que as rotas `/api/v1/signature/internal/*` liam o tenant
// de `?tenant_id=` com a chave interna compartilhada como única credencial:
// qualquer portador da chave lia/criava envelopes de qualquer tenant.
//
// Agora a rota aceita chave interna OU JWT de usuário, e o tenant efetivo vem da
// credencial (`auth.ResolveTenantForInternalRequest`).

func internalEnvelopeRouter(h *EnvelopeHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/internal/envelopes/by-contract/:contractId", h.GetByContractInternal)
	return r
}

// sem middleware interno nenhum, o resolver recusa — prova que a rota depende
// da identidade e não do query param.
func TestInternalRoute_RejectsRequestWithoutInternalIdentity(t *testing.T) {
	h := &EnvelopeHandler{repo: &inertEnvelopeRepo{}}
	r := internalEnvelopeRouter(h)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/internal/envelopes/by-contract/c1?tenant_id=tenant-vitima", nil)
	r.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Fatalf("rota interna respondeu 200 sem identidade de serviço/usuário: %s", w.Body.String())
	}
	if w.Code != http.StatusUnauthorized && w.Code != http.StatusForbidden {
		t.Fatalf("esperava 401/403, veio %d", w.Code)
	}
}

// Chamador de SERVIÇO com credencial restrita: tenant fora da allowlist é 403 —
// era exatamente o cenário do A3 (chave interna + tenant arbitrário).
func TestInternalRoute_ServiceCredentialCannotPickForeignTenant(t *testing.T) {
	repo := &inertEnvelopeRepo{}
	h := &EnvelopeHandler{repo: repo}
	r := gin.New()
	gin.SetMode(gin.TestMode)
	r.Use(func(c *gin.Context) {
		c.Set(auth.CtxInternalAuthKind, auth.InternalAuthService)
		c.Set(auth.CtxInternalServiceName, "toca-deal")
		c.Set(auth.CtxInternalAllowedScope, []string{"tenant-a"})
		c.Next()
	})
	r.GET("/internal/envelopes/by-contract/:contractId", h.GetByContractInternal)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/internal/envelopes/by-contract/c1?tenant_id=tenant-vitima", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("VAZAMENTO CROSS-TENANT: credencial restrita a tenant-a pediu tenant-vitima e recebeu %d (tenant consultado: %q): %s",
			w.Code, repo.tenantRecebido, w.Body.String())
	}
	if repo.tenantRecebido != "" {
		t.Fatalf("o repositório foi consultado com %q — a requisição nem deveria chegar lá", repo.tenantRecebido)
	}
}

// Chamada com JWT: o tenant do TOKEN manda; pedir outro é 403.
func TestInternalRoute_UserTokenTenantWins(t *testing.T) {
	repo := &inertEnvelopeRepo{}
	h := &EnvelopeHandler{repo: repo}
	r := gin.New()
	gin.SetMode(gin.TestMode)
	r.Use(func(c *gin.Context) {
		c.Set(auth.CtxInternalAuthKind, auth.InternalAuthUser)
		c.Set("tenant_id", "tenant-do-token")
		c.Next()
	})
	r.GET("/internal/envelopes/by-contract/:contractId", h.GetByContractInternal)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/internal/envelopes/by-contract/c1?tenant_id=tenant-vitima", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("VAZAMENTO CROSS-TENANT: token de tenant-do-token pediu tenant-vitima e recebeu %d (tenant consultado: %q)",
			w.Code, repo.tenantRecebido)
	}
	if repo.tenantRecebido != "" {
		t.Fatalf("o repositório foi consultado com %q — a requisição nem deveria chegar lá", repo.tenantRecebido)
	}
}

// Serviço precisa dizer o tenant (antes o handler respondia 422 "required";
// segue obrigatório, agora pelo resolver).
func TestInternalRoute_ServiceWithoutTenantIsRejected(t *testing.T) {
	h := &EnvelopeHandler{repo: &inertEnvelopeRepo{}}
	r := gin.New()
	gin.SetMode(gin.TestMode)
	r.Use(func(c *gin.Context) {
		c.Set(auth.CtxInternalAuthKind, auth.InternalAuthService)
		c.Set(auth.CtxInternalAllowedScope, []string{})
		c.Next()
	})
	r.GET("/internal/envelopes/by-contract/:contractId", h.GetByContractInternal)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/internal/envelopes/by-contract/c1", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity && w.Code != http.StatusBadRequest {
		t.Fatalf("serviço sem tenant deveria ser recusado como validação, veio %d", w.Code)
	}
}
