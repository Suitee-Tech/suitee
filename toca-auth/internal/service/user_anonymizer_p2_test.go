package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// M13 — a exclusão LGPD era um no-op silencioso com resposta de sucesso.
//
// A revisão de permissões de 2026-09-19 registrou que o `UserAnonymizer`
// consultava `auth.accounts` (schema que não existe — a tabela é
// `toca_auth.accounts`) e engolia QUALQUER erro de query como "conta
// inexistente". O `DELETE /lgpd/me/account` devolvia "Account anonymized" sem
// tocar em nenhuma linha: atestação falsa de conformidade.
//
// Os testes abaixo fixam o contrato fail-loud e o schema correto.

type fakeAccountStore struct {
	metadata   []byte
	selectErr  error
	execErr    error
	execTag    pgconn.CommandTag
	selectSQL  string
	updateSQL  string
	execArgs   []any
	selectCall int
}

func (f *fakeAccountStore) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	f.selectSQL = sql
	f.selectCall++
	if f.selectErr != nil {
		return fakeRow{err: f.selectErr}
	}
	if f.metadata == nil {
		return fakeRow{err: pgx.ErrNoRows}
	}
	return fakeRow{value: f.metadata}
}

func (f *fakeAccountStore) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.updateSQL = sql
	f.execArgs = args
	if f.execErr != nil {
		return pgconn.CommandTag{}, f.execErr
	}
	return f.execTag, nil
}

type fakeRow struct {
	value []byte
	err   error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != 1 {
		return errors.New("unexpected scan arity")
	}
	target, ok := dest[0].(*[]byte)
	if !ok {
		return errors.New("unexpected scan target")
	}
	*target = r.value
	return nil
}

func accountMetadata(t *testing.T, payload map[string]any) []byte {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}

func okTag() pgconn.CommandTag { return pgconn.NewCommandTag("UPDATE 1") }

// O SQL precisa apontar para o schema REAL. Este teste é o que teria pegado o
// achado: `auth.accounts` não existe e o erro era engolido.
func TestUserAnonymizer_UsesRealSchema(t *testing.T) {
	store := &fakeAccountStore{
		metadata: accountMetadata(t, map[string]any{"type": "individual", "individual": map[string]any{"email": "titular@example.com"}}),
		execTag:  okTag(),
	}

	if _, err := newUserAnonymizerWithStore(store).Anonymize(context.Background(), "acc-1"); err != nil {
		t.Fatalf("anonimização falhou: %v", err)
	}
	if !strings.Contains(store.selectSQL, "toca_auth.accounts") {
		t.Fatalf("SELECT não usa toca_auth.accounts: %s", store.selectSQL)
	}
	if !strings.Contains(store.updateSQL, "toca_auth.accounts") {
		t.Fatalf("UPDATE não usa toca_auth.accounts: %s", store.updateSQL)
	}
	if strings.Contains(store.selectSQL, "auth.accounts") && !strings.Contains(store.selectSQL, "toca_auth.accounts") {
		t.Fatalf("SELECT ainda aponta para o schema inexistente: %s", store.selectSQL)
	}
}

// Erro de banco NÃO pode virar sucesso silencioso.
func TestUserAnonymizer_DatabaseErrorIsLoud(t *testing.T) {
	store := &fakeAccountStore{selectErr: errors.New(`ERROR: relation "auth.accounts" does not exist (SQLSTATE 42P01)`)}

	touched, err := newUserAnonymizerWithStore(store).Anonymize(context.Background(), "acc-1")
	if err == nil {
		t.Fatalf("erro de banco engolido (touched=%d): atestação falsa de conformidade", touched)
	}
	if !strings.Contains(err.Error(), "42P01") {
		t.Fatalf("erro perdeu a causa original: %v", err)
	}
}

func TestUserAnonymizer_AccountNotFoundIsError(t *testing.T) {
	store := &fakeAccountStore{} // QueryRow devolve pgx.ErrNoRows

	_, err := newUserAnonymizerWithStore(store).Anonymize(context.Background(), "acc-inexistente")
	if !errors.Is(err, ErrAnonymizedAccountNotFound) {
		t.Fatalf("conta inexistente devolveu %v, want ErrAnonymizedAccountNotFound", err)
	}
}

func TestUserAnonymizer_WithoutDatabaseIsError(t *testing.T) {
	_, err := (&UserAnonymizer{}).Anonymize(context.Background(), "acc-1")
	if !errors.Is(err, ErrAnonymizerNotConfigured) {
		t.Fatalf("anonimizador sem banco devolveu %v, want ErrAnonymizerNotConfigured", err)
	}
}

// Conta existe, mas não tem bloco de identidade pessoal: não há o que excluir,
// e isso NÃO pode ser reportado como sucesso.
func TestUserAnonymizer_CompanyAccountWithoutIndividualIsError(t *testing.T) {
	store := &fakeAccountStore{
		metadata: accountMetadata(t, map[string]any{"type": "company"}),
		execTag:  okTag(),
	}

	touched, err := newUserAnonymizerWithStore(store).Anonymize(context.Background(), "company-1")
	if err == nil || touched != 0 {
		t.Fatalf("conta sem dados pessoais devolveu (%d, %v); want erro explícito", touched, err)
	}
	if store.updateSQL != "" {
		t.Fatal("UPDATE executado sem nada para anonimizar")
	}
}

func TestUserAnonymizer_AnonymizesPIIAndDeactivates(t *testing.T) {
	store := &fakeAccountStore{
		metadata: accountMetadata(t, map[string]any{
			"type": "individual",
			"individual": map[string]any{
				"firstname":  "Maria",
				"lastname":   "Silva",
				"fullname":   "Maria Silva",
				"email":      "maria@example.com",
				"phone":      "11999998888",
				"bio":        "corretora",
				"avatar_url": "https://cdn.example.com/a.png",
				"address":    map[string]any{"city": "São Paulo"},
				"banking":    map[string]any{"iban": "BR00"},
				"is_active":  true,
			},
		}),
		execTag: okTag(),
	}

	touched, err := newUserAnonymizerWithStore(store).Anonymize(context.Background(), "acc-1")
	if err != nil {
		t.Fatalf("anonimizar: %v", err)
	}
	if touched != 9 {
		t.Fatalf("touched = %d, want 9 (6 textos + address + banking + avatar)", touched)
	}
	if len(store.execArgs) != 2 {
		t.Fatalf("UPDATE com args inesperados: %+v", store.execArgs)
	}
	payload, _ := store.execArgs[1].(string)
	for _, pii := range []string{"Maria", "Silva", "maria@example.com", "11999998888"} {
		if strings.Contains(payload, pii) {
			t.Fatalf("PII %q sobreviveu à anonimização: %s", pii, payload)
		}
	}
	for _, cleared := range []string{`"address":{}`, `"banking":{}`, `"avatar_url":""`} {
		if !strings.Contains(payload, cleared) {
			t.Fatalf("campo %s não foi limpo: %s", cleared, payload)
		}
	}
	if !strings.Contains(payload, `"is_active":false`) {
		t.Fatalf("conta anonimizada não foi marcada inativa: %s", payload)
	}
}

// Idempotência: rodar de novo numa linha já anonimizada é no-op com sucesso.
func TestUserAnonymizer_AlreadyAnonymizedIsIdempotent(t *testing.T) {
	store := &fakeAccountStore{
		metadata: accountMetadata(t, map[string]any{
			"type":          "individual",
			"anonymized_at": "2026-09-19T00:00:00Z",
			"individual":    map[string]any{"email": "hash$abc"},
		}),
		execTag: okTag(),
	}

	touched, err := newUserAnonymizerWithStore(store).Anonymize(context.Background(), "acc-1")
	if err != nil || touched != 0 {
		t.Fatalf("segunda execução devolveu (%d, %v), want (0, nil)", touched, err)
	}
	if store.updateSQL != "" {
		t.Fatal("UPDATE executado numa linha já anonimizada")
	}
}

// UPDATE sem efeito não pode ser sucesso (linha sumiu entre SELECT e UPDATE).
func TestUserAnonymizer_NoRowsUpdatedIsError(t *testing.T) {
	store := &fakeAccountStore{
		metadata: accountMetadata(t, map[string]any{"type": "individual", "individual": map[string]any{"email": "a@b.com"}}),
		execTag:  pgconn.NewCommandTag("UPDATE 0"),
	}

	_, err := newUserAnonymizerWithStore(store).Anonymize(context.Background(), "acc-1")
	if err == nil || !strings.Contains(err.Error(), "touched 0 rows") {
		t.Fatalf("UPDATE sem linhas devolveu %v, want erro", err)
	}
}
