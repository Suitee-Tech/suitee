//go:build integration

package database

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/config"
)

// M1 — coleta das capacidades de RLS dos dois pools, contra banco real.
//
// Rode com um banco descartável cujo role de aplicação NÃO tenha BYPASSRLS:
//
//	createdb toca_review_qa_rls
//	psql -c "CREATE ROLE toca_rls_app LOGIN PASSWORD 'toca' NOBYPASSRLS"
//	psql -d toca_review_qa_rls -c "GRANT CONNECT, USAGE ON SCHEMA public TO toca_rls_app"
//	DATABASE_URI_TEST=postgres://toca_rls_app:toca@localhost:54322/toca_review_qa_rls?sslmode=disable \
//	DATABASE_URI_SERVICE_TEST=postgres://postgres:postgres@localhost:54322/toca_review_qa_rls?sslmode=disable \
//	go test -tags=integration ./pkg/database/ -run TestRLSBackstop
//
// Sem as variáveis o teste é pulado (a suíte padrão não depende de banco).
func TestRLSBackstopReportsApplicationPoolCapability(t *testing.T) {
	uri := strings.TrimSpace(os.Getenv("DATABASE_URI_TEST"))
	if uri == "" {
		t.Skip("set DATABASE_URI_TEST (role sem BYPASSRLS) to run the RLS backstop test")
	}
	serviceURI := strings.TrimSpace(os.Getenv("DATABASE_URI_SERVICE_TEST"))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := config.DatabaseConfig{URI: uri, ServiceURI: serviceURI}
	db, err := ConnectPostgres(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer db.Close()

	backstop := db.RLSBackstop()
	if backstop.UserRoleBypassesRLS {
		t.Fatalf("DATABASE_URI_TEST role bypassa RLS; o teste exige um role de aplicação sem BYPASSRLS (%s)", uri)
	}
	if !backstop.Active() {
		t.Fatal("backstop deveria estar ativo com role de aplicação sem BYPASSRLS")
	}
	if decision := EvaluateRLSBackstop(backstop); decision.Event != "database.rls_backstop_active" {
		t.Fatalf("decisão = %s, want database.rls_backstop_active", decision.Event)
	}
}

// O caminho fail-closed: com o role de aplicação bypassando RLS e
// REQUIRE_RLS_BACKSTOP=true em release, o boot precisa falhar.
func TestRLSBackstopFailsBootWhenRequired(t *testing.T) {
	uri := strings.TrimSpace(os.Getenv("DATABASE_URI_SERVICE_TEST"))
	if uri == "" {
		t.Skip("set DATABASE_URI_SERVICE_TEST (role COM BYPASSRLS) to run the fail-closed test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_MODE", "release")
	t.Setenv("REQUIRE_RLS_BACKSTOP", "true")

	cfg := config.DatabaseConfig{URI: uri, ServiceURI: ""}
	db, err := ConnectPostgres(ctx, cfg)
	if db != nil {
		db.Close()
	}
	if !errors.Is(err, ErrRLSBackstopRequired) {
		t.Fatalf("boot com role BYPASSRLS e REQUIRE_RLS_BACKSTOP=true devolveu %v, want ErrRLSBackstopRequired", err)
	}
}
