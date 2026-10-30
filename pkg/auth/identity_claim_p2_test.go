package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// P2/M9 — identidade × tenant para o `cliente`.
//
// A revisão de permissões de 2026-09-19 (achado M9) registrou a conflação: o
// convite de cliente gravava a CONTA LOCAL em `app_metadata.tenant_id`, e o
// middleware usava o `tenant_id` como identidade sempre que o papel era
// `cliente`. Resultado: um cliente convidado por uma empresa passava a ter como
// identidade o **id da empresa**, e `/me`, `/lgpd/me/account` e a checagem de
// vínculo operavam sobre ela.
//
// Agora a identidade tem claim própria (`app_metadata.account_id`); tokens
// antigos (sem a claim) seguem funcionando por até 1h, com aviso.

// signSupabaseStyleToken monta um token como o Supabase emite para estes fluxos:
// `sub` é o usuário do Supabase e o produto vive em `app_metadata` (sem os
// campos legados de topo que os outros helpers preenchem).
func signSupabaseStyleToken(t *testing.T, sub string, appMeta AppMetadata) string {
	t.Helper()
	if authTestJWKSInstance == nil {
		t.Fatal("JWKS de teste não inicializado")
	}
	claims := Claims{
		TokenType:   TokenTypeAccess,
		AppMetadata: &appMeta,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = authTestJWKSInstance.kid
	signed, err := tok.SignedString(authTestJWKSInstance.key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

// identityEngine devolve um router que expõe o que o middleware gravou.
func identityEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(authTestMiddleware(t))
	engine.GET("/identity", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"user_id":    c.GetString("user_id"),
			"account_id": c.GetString("account_id"),
			"tenant_id":  c.GetString("tenant_id"),
			"role":       c.GetString("functional_role"),
		})
	})
	return engine
}

func doIdentityRequest(t *testing.T, engine *gin.Engine, token string) (int, map[string]string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/identity", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	body := map[string]string{}
	if recorder.Code == http.StatusOK {
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
	}
	return recorder.Code, body
}

func TestMiddleware_ClienteIdentityComesFromDedicatedClaim(t *testing.T) {
	engine := identityEngine(t)

	// Cliente convidado por uma empresa: identidade = conta local, tenant = empresa.
	token := signSupabaseStyleToken(t, "supabase-user-1", AppMetadata{
		AccountID:      "local-acc-1",
		TenantID:       "company-1",
		FunctionalRole: "cliente",
		Permission:     "cliente",
	})

	code, body := doIdentityRequest(t, engine, token)
	if code != http.StatusOK {
		t.Fatalf("status %d, want 200", code)
	}
	if body["user_id"] != "local-acc-1" {
		t.Fatalf("user_id = %q, want a conta local (a identidade NÃO pode ser o id da empresa)", body["user_id"])
	}
	if body["account_id"] != "local-acc-1" {
		t.Fatalf("account_id = %q, want a conta local (B9: chave publicada com o mesmo valor de user_id)", body["account_id"])
	}
	if body["tenant_id"] != "company-1" {
		t.Fatalf("tenant_id = %q, want company-1 (o tenant continua sendo a empresa)", body["tenant_id"])
	}
}

// Token emitido antes do P2/M9 (sem a claim dedicada): continua valendo durante
// a transição, usando o `tenant_id` como conta local — exatamente o que o
// cliente standalone recebia.
func TestMiddleware_LegacyClienteTokenStillResolvesIdentity(t *testing.T) {
	engine := identityEngine(t)

	token := signSupabaseStyleToken(t, "supabase-user-2", AppMetadata{
		TenantID:       "local-acc-2",
		FunctionalRole: "cliente",
	})

	code, body := doIdentityRequest(t, engine, token)
	if code != http.StatusOK {
		t.Fatalf("token legado recusado: %d", code)
	}
	if body["user_id"] != "local-acc-2" {
		t.Fatalf("user_id = %q, want local-acc-2 (fallback transitório)", body["user_id"])
	}
}

// A claim dedicada vale para qualquer papel, não só `cliente`.
func TestMiddleware_DedicatedClaimAppliesToOperatorTokens(t *testing.T) {
	engine := identityEngine(t)

	token := signSupabaseStyleToken(t, "supabase-user-3", AppMetadata{
		AccountID:      "acc-3",
		TenantID:       "company-3",
		FunctionalRole: "gestor",
	})

	code, body := doIdentityRequest(t, engine, token)
	if code != http.StatusOK {
		t.Fatalf("status %d, want 200", code)
	}
	if body["user_id"] != "acc-3" || body["tenant_id"] != "company-3" {
		t.Fatalf("identidade/tenant errados: %+v", body)
	}
}

// Sem identidade alguma o middleware recusa (fail-closed preservado).
func TestMiddleware_WithoutIdentityIsUnauthorized(t *testing.T) {
	engine := identityEngine(t)

	token := signSupabaseStyleToken(t, "", AppMetadata{TenantID: "company-1", FunctionalRole: "gestor"})
	code, _ := doIdentityRequest(t, engine, token)
	if code != http.StatusUnauthorized {
		t.Fatalf("token sem identidade devolveu %d, want 401", code)
	}
}
