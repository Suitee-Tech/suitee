package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/gdpr"
)

// UserAnonymizer implements gdpr.Anonymizer para toca_auth.accounts. Substitui
// PII no User struct (FirstName/LastName/FullName/Email/Phone) pelo
// hash determinístico de gdpr.HashIdentifier. Issue #476.
//
// Por que aqui (no toca-auth) e não cross-service?
//   - toca_auth.accounts é a fonte canônica de identidade pessoal — quando
//     anonimizamos AccountID, o User (LeadAnonymizer já cobre crm.leads)
//     se aposenta sem deixar pegada PII.
//   - Outros services usam o accountID como FK; preservamos o ID (assim
//     joins históricos não quebram) e só zeramos os campos identificáveis.
//
// Campos anonimizados (quando preenchidos):
//   - metadata->'individual'->>'firstname'/'lastname'/'fullname'
//   - metadata->'individual'->>'email'
//   - metadata->'individual'->>'phone'
//   - metadata->'individual'->>'bio'
//   - metadata->'individual'->'address' completo (zerado)
//   - metadata->'individual'->'banking' completo (zerado)
//
// Campos preservados:
//   - id (account_id)
//   - type (Individual vs Company)
//   - timestamps
//
// Idempotente: re-runs com mesmo accountID já anonimizado é no-op.
//
// SECURITY (achado M13 da revisão de permissões de 2026-09-19): a versão
// anterior consultava `auth.accounts` — schema que NÃO EXISTE neste banco (a
// tabela é `toca_auth.accounts`) — e engolia qualquer erro de query como "conta
// inexistente":
//
//	err := ...QueryRow(...).Scan(&raw)
//	if err != nil { return 0, nil }   // ⇐ atestação falsa de conformidade
//
// Resultado: `DELETE /lgpd/me/account` respondia sucesso ("conta anonimizada")
// sem tocar em uma única linha. Agora o schema é o correto e a falha é ALTA:
// erro de banco vira erro do domínio (o handler devolve o que falhou), conta
// inexistente é erro explícito e "nada a anonimizar" só é sucesso quando a linha
// já estava anonimizada (idempotência).
type UserAnonymizer struct {
	store accountStore
}

// accountStore é o mínimo que o anonimizador precisa do banco. Existe para que a
// regra seja testável sem Postgres (o *pgxpool.Pool satisfaz a interface).
type accountStore interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// ErrAnonymizerNotConfigured é devolvido quando o anonimizador não tem banco —
// antes esse estado também era silencioso (`return 0, nil`).
var ErrAnonymizerNotConfigured = errors.New("user anonymizer has no database configured")

// ErrAnonymizedAccountNotFound — a conta alvo não existe. Antes era no-op com
// sucesso; para uma atestação de LGPD, "não achei a conta" precisa ser visível.
var ErrAnonymizedAccountNotFound = errors.New("account not found for anonymization")

func NewUserAnonymizer(db *database.PostgresDB) *UserAnonymizer {
	if db == nil || db.ServicePool == nil {
		return &UserAnonymizer{}
	}
	return &UserAnonymizer{store: db.ServicePool}
}

// newUserAnonymizerWithStore permite injetar um dublê (testes).
func newUserAnonymizerWithStore(store accountStore) *UserAnonymizer {
	return &UserAnonymizer{store: store}
}

func (a *UserAnonymizer) Domain() string { return "auth.account" }

func (a *UserAnonymizer) Anonymize(ctx context.Context, accountID string) (int, error) {
	if a == nil || a.store == nil {
		return 0, ErrAnonymizerNotConfigured
	}
	if strings.TrimSpace(accountID) == "" {
		return 0, fmt.Errorf("account_id required")
	}

	// Lê o metadata JSON inteiro pra preservar campos não-PII (que
	// queremos manter — type, is_active, audit timestamps etc.) e
	// substituir cirurgicamente os campos PII em sub-paths conhecidos.
	const selectQuery = `
		SELECT metadata
		  FROM toca_auth.accounts
		 WHERE id = $1::uuid
	`
	var raw []byte
	err := a.store.QueryRow(ctx, selectQuery, accountID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrAnonymizedAccountNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("load account %s: %w", accountID, err)
	}

	var blob map[string]interface{}
	if err := json.Unmarshal(raw, &blob); err != nil {
		return 0, fmt.Errorf("parse account metadata: %w", err)
	}

	if already, _ := blob["anonymized_at"].(string); already != "" {
		// Idempotência real: a linha já foi anonimizada numa chamada anterior.
		return 0, nil
	}

	touched := 0
	if individual, ok := blob["individual"].(map[string]interface{}); ok {
		// PII text fields → hash determinístico.
		for _, key := range []string{"firstname", "lastname", "fullname", "email", "phone", "bio"} {
			if v, ok := individual[key].(string); ok && v != "" && !gdpr.IsAnonymized(v) {
				individual[key] = gdpr.HashIdentifier(v)
				touched++
			}
		}
		// Address e Banking — limpar inteiro (não fazem sentido como hash).
		// Banking especificamente é sensível: zerar evita re-uso indevido.
		if _, ok := individual["address"]; ok {
			individual["address"] = map[string]interface{}{}
			touched++
		}
		if _, ok := individual["banking"]; ok {
			individual["banking"] = map[string]interface{}{}
			touched++
		}
		// avatar_url → zerar (era PII via storage).
		if v, ok := individual["avatar_url"].(string); ok && v != "" {
			individual["avatar_url"] = ""
			touched++
		}
		// Marcar inativo — login bloqueado (e agora EFETIVAMENTE lido: P2/M7).
		individual["is_active"] = false
		blob["individual"] = individual
	} else {
		// A linha existe, não está marcada como anonimizada e não tem bloco de
		// identidade pessoal (ex.: conta `company`). Sucesso silencioso aqui
		// seria de novo atestação falsa: o titular pediu exclusão e nada foi
		// excluído.
		return 0, fmt.Errorf("account %s has no individual data to anonymize (type=%v)", accountID, blob["type"])
	}

	// Tag a nível de account pra audit/queries futuras saberem que o
	// row foi anonimizado e não tentar usar como dado vivo.
	blob["anonymized_at"] = "now()" // será substituído pelo SQL abaixo
	blob["is_active"] = false

	newRaw, err := json.Marshal(blob)
	if err != nil {
		return 0, fmt.Errorf("marshal updated metadata: %w", err)
	}

	const updateQuery = `
		UPDATE toca_auth.accounts
		   SET metadata = $2::jsonb || jsonb_build_object('anonymized_at', NOW()),
		       updated_at = NOW()
		 WHERE id = $1::uuid
	`
	tag, err := a.store.Exec(ctx, updateQuery, accountID, string(newRaw))
	if err != nil {
		return touched, fmt.Errorf("update toca_auth.accounts: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// A linha existia no SELECT e o UPDATE não pegou nada: estado inesperado
		// (concorrência/remoção). Falha alta em vez de sucesso silencioso.
		return touched, fmt.Errorf("anonymization touched 0 rows for account %s", accountID)
	}
	return touched, nil
}
