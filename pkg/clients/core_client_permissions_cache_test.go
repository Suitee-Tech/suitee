package clients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	appCtx "github.com/oginoino/toca/pkg/context"
)

// P1 / A5 — invalidação do cache de permissões na escrita da matriz.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A5) mostrou que `/me/permissions` era cacheado por 30 s SEM nenhuma
// invalidação ao salvar a matriz: uma revogação de permissão levava até 30 s
// para valer em todos os serviços.
//
// Correção: o cache guarda a VERSÃO da matriz junto do payload; o toca-core
// incrementa `permsver:{tenant}` a cada salvamento, então a próxima leitura de
// qualquer usuário daquele tenant já relê do core.
//
// Este teste usa o Redis local (mesmo padrão dos testes de banco QA: ele se
// declara pulado quando o Redis não está acessível).

func testRedis(t *testing.T) *redis.Client {
	t.Helper()
	uri := os.Getenv("REDIS_URI")
	if uri == "" {
		uri = "redis://127.0.0.1:6379/0"
	}
	opts, err := redis.ParseURL(uri)
	if err != nil {
		t.Skipf("REDIS_URI inválido (%v)", err)
	}
	rdb := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis indisponível em %s: %v", uri, err)
	}
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb
}

func newTestCoreClient(t *testing.T, rdb *redis.Client, calls *int64) *CoreClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(MyPermissions{
			FunctionalRole: "corretor",
			Profile:        "member",
			Permissions:    []string{"leads:read"},
		})
	}))
	t.Cleanup(srv.Close)
	return &CoreClient{BaseURL: srv.URL, HTTPClient: srv.Client(), rdb: rdb}
}

func authCtx(tenantID, accountID, token string) context.Context {
	ctx := appCtx.WithTenantID(context.Background(), tenantID)
	ctx = appCtx.WithAccountID(ctx, accountID)
	return appCtx.WithAuthToken(ctx, token)
}

func TestGetMyPermissions_CacheHitDentroDaMesmaVersao(t *testing.T) {
	rdb := testRedis(t)
	var calls int64
	c := newTestCoreClient(t, rdb, &calls)

	const tenant, account = "tenant-cache", "user-cache"
	ctx := authCtx(tenant, account, "token")
	t.Cleanup(func() {
		_ = c.InvalidatePermissionsCache(context.Background(), tenant, account)
		_ = rdb.Del(context.Background(), permissionsCacheVersionKey(tenant)).Err()
	})
	_ = c.InvalidatePermissionsCache(context.Background(), tenant, account)

	for i := 0; i < 3; i++ {
		got, err := c.GetMyPermissions(ctx)
		if err != nil {
			t.Fatalf("chamada %d: %v", i, err)
		}
		if len(got.Permissions) != 1 {
			t.Fatalf("payload inesperado: %+v", got)
		}
	}
	if calls != 1 {
		t.Fatalf("chamadas HTTP ao core = %d, want 1 (cache deveria servir as demais)", calls)
	}
}

// O teste central do achado A5: salvar a matriz (bump da versão) precisa
// invalidar o cache imediatamente, não em 30 s.
func TestGetMyPermissions_VersionBumpInvalidaNaHora(t *testing.T) {
	rdb := testRedis(t)
	var calls int64
	c := newTestCoreClient(t, rdb, &calls)

	const tenant, account = "tenant-bump", "user-bump"
	ctx := authCtx(tenant, account, "token")
	t.Cleanup(func() {
		_ = c.InvalidatePermissionsCache(context.Background(), tenant, account)
		_ = rdb.Del(context.Background(), permissionsCacheVersionKey(tenant)).Err()
	})
	_ = c.InvalidatePermissionsCache(context.Background(), tenant, account)
	_ = rdb.Del(context.Background(), permissionsCacheVersionKey(tenant)).Err()

	if _, err := c.GetMyPermissions(ctx); err != nil { // popula o cache
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("primeira chamada deveria consultar o core (calls=%d)", calls)
	}

	// Simula o PUT /company-settings/permissions: o toca-core incrementa a versão.
	if err := c.BumpPermissionsVersion(context.Background(), tenant); err != nil {
		t.Fatalf("bump: %v", err)
	}

	if _, err := c.GetMyPermissions(ctx); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("REVOGAÇÃO ATRASADA: após o bump o cache continuou servindo (calls=%d, want 2)", calls)
	}
}

// Sem versão gravada (ex.: toca-core sem Redis) o comportamento continua o
// antigo — degradação segura, nunca cache eterno nem erro.
func TestGetMyPermissions_SemVersaoContinuaCacheandoComTTL(t *testing.T) {
	rdb := testRedis(t)
	var calls int64
	c := newTestCoreClient(t, rdb, &calls)

	const tenant, account = "tenant-nover", "user-nover"
	ctx := authCtx(tenant, account, "token")
	t.Cleanup(func() { _ = c.InvalidatePermissionsCache(context.Background(), tenant, account) })
	_ = rdb.Del(context.Background(), permissionsCacheVersionKey(tenant)).Err()
	_ = c.InvalidatePermissionsCache(context.Background(), tenant, account)

	for i := 0; i < 2; i++ {
		if _, err := c.GetMyPermissions(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("sem versão o cache deveria continuar servindo (calls=%d, want 1)", calls)
	}
}
