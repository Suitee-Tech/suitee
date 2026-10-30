//go:build integration

package service

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/database"
)

// M13 — prova de runtime: o anonimizador LGPD fala com o schema REAL.
//
// A versão anterior consultava `auth.accounts` (inexistente) e engolia o erro:
// sucesso sem tocar em nenhuma linha. Este teste roda contra um banco de verdade
// (Supabase local ou QA descartável), insere uma conta sintética e verifica o
// efeito no banco.
//
//	LGPD_REVIEW_TEST_DATABASE_URL=postgres://postgres:postgres@localhost:54322/postgres?sslmode=disable \
//	go test -tags=integration ./internal/service/ -run TestUserAnonymizerIntegration
func TestUserAnonymizerIntegration(t *testing.T) {
	uri := strings.TrimSpace(os.Getenv("LGPD_REVIEW_TEST_DATABASE_URL"))
	if uri == "" {
		t.Skip("set LGPD_REVIEW_TEST_DATABASE_URL to run the LGPD anonymizer integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := database.ConnectPostgres(ctx, config.DatabaseConfig{URI: uri})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer db.Close()

	accountID := uuid.NewString()
	metadata, _ := json.Marshal(map[string]any{
		"type": "individual",
		"individual": map[string]any{
			"firstname": "Titular",
			"lastname":  "Sintético",
			"fullname":  "Titular Sintético",
			"email":     "titular.sintetico@example.com",
			"phone":     "11999990000",
			"is_active": true,
		},
	})
	if _, err := db.ServicePool.Exec(ctx,
		`INSERT INTO toca_auth.accounts (id, metadata) VALUES ($1::uuid, $2::jsonb)`, accountID, string(metadata),
	); err != nil {
		t.Fatalf("insert synthetic account: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = db.ServicePool.Exec(cleanupCtx, `DELETE FROM toca_auth.accounts WHERE id = $1::uuid`, accountID)
	})

	anonymizer := NewUserAnonymizer(db)
	touched, err := anonymizer.Anonymize(ctx, accountID)
	if err != nil {
		t.Fatalf("Anonymize: %v", err)
	}
	if touched == 0 {
		t.Fatal("nenhum campo anonimizado — a regressão do M13 (no-op silencioso) voltou")
	}

	var stored []byte
	if err := db.ServicePool.QueryRow(ctx, `SELECT metadata FROM toca_auth.accounts WHERE id = $1::uuid`, accountID).Scan(&stored); err != nil {
		t.Fatalf("reload account: %v", err)
	}
	payload := string(stored)
	for _, pii := range []string{"Titular", "Sintético", "titular.sintetico@example.com", "11999990000"} {
		if strings.Contains(payload, pii) {
			t.Fatalf("PII %q continua no banco após a anonimização: %s", pii, payload)
		}
	}
	var blob map[string]any
	if err := json.Unmarshal(stored, &blob); err != nil {
		t.Fatalf("parse stored metadata: %v", err)
	}
	if _, ok := blob["anonymized_at"]; !ok {
		t.Fatalf("anonymized_at ausente: %s", payload)
	}
	if active, _ := blob["is_active"].(bool); active {
		t.Fatalf("conta anonimizada continua ativa: %s", payload)
	}

	// Idempotência no banco real.
	second, err := anonymizer.Anonymize(ctx, accountID)
	if err != nil || second != 0 {
		t.Fatalf("segunda execução devolveu (%d, %v), want (0, nil)", second, err)
	}
}
