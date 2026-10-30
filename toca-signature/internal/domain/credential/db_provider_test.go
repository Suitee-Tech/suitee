package credential

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCryptoRoundTrip(t *testing.T) {
	key, err := KeyFromHex(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatalf("KeyFromHex: %v", err)
	}
	secret := "token-secreto-🚀"
	envelope, err := Encrypt(key, secret)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if strings.Contains(envelope, secret) {
		t.Fatal("envelope vaza o texto puro")
	}
	got, err := Decrypt(key, envelope)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != secret {
		t.Fatalf("round-trip divergente: %q", got)
	}
}

func TestCrypto_TamperedEnvelope(t *testing.T) {
	key, _ := KeyFromHex(strings.Repeat("cd", 32))
	envelope, _ := Encrypt(key, "segredo")
	tampered := envelope[:len(envelope)-2] + "!!"
	if _, err := Decrypt(key, tampered); !errors.Is(err, ErrCredentialCryptoInvalid) {
		t.Fatalf("err = %v, want ErrCredentialCryptoInvalid", err)
	}
}

func TestCrypto_WrongKeyFails(t *testing.T) {
	key1, _ := KeyFromHex(strings.Repeat("ab", 32))
	key2, _ := KeyFromHex(strings.Repeat("cd", 32))
	envelope, _ := Encrypt(key1, "segredo")
	if _, err := Decrypt(key2, envelope); !errors.Is(err, ErrCredentialCryptoInvalid) {
		t.Fatalf("err = %v, want ErrCredentialCryptoInvalid", err)
	}
}

func TestKeyFromHex_Validation(t *testing.T) {
	if _, err := KeyFromHex(""); !errors.Is(err, ErrCredentialCryptoKeyMissing) {
		t.Fatalf("vazio: err = %v", err)
	}
	if _, err := KeyFromHex("abc123"); !errors.Is(err, ErrCredentialCryptoKeyMissing) {
		t.Fatalf("curta: err = %v", err)
	}
}

// fakeCredentialRepo implementa o CredentialRepository em memória.
type fakeCredentialRepo struct {
	rows      map[string]*CredentialRow
	rotations map[string][]*CredentialRotation
}

func newFakeCredentialRepo() *fakeCredentialRepo {
	return &fakeCredentialRepo{rows: map[string]*CredentialRow{}, rotations: map[string][]*CredentialRotation{}}
}

func (f *fakeCredentialRepo) GetByTenant(_ context.Context, tenantID string) (*CredentialRow, error) {
	row, ok := f.rows[tenantID]
	if !ok {
		return nil, ErrCredentialTenantNotFound
	}
	return row, nil
}

func (f *fakeCredentialRepo) Upsert(_ context.Context, tenantID string, row *CredentialRow) (int, error) {
	revision := 1
	if prior, ok := f.rows[tenantID]; ok {
		revision = prior.Revision + 1
	}
	row.TenantID = tenantID
	row.Revision = revision
	f.rows[tenantID] = row
	return revision, nil
}

func (f *fakeCredentialRepo) AppendRotation(_ context.Context, tenantID string, revision int, rotatedBy, note string) error {
	f.rotations[tenantID] = append(f.rotations[tenantID], &CredentialRotation{
		TenantID: tenantID, Revision: revision, RotatedBy: rotatedBy, Note: note,
	})
	return nil
}

func (f *fakeCredentialRepo) ListRotations(_ context.Context, tenantID string, limit int) ([]*CredentialRotation, error) {
	return f.rotations[tenantID], nil
}

// envCreds é o fallback global usado nos testes do provedor por tenant.
type envCreds struct{ creds *Credentials }

func (e *envCreds) Get(_ context.Context, _ string) (*Credentials, error) {
	if e.creds == nil {
		return nil, ErrCredentialsNotFound
	}
	return e.creds, nil
}

func dbProviderFixture(t *testing.T) (*DBCredentialProvider, *fakeCredentialRepo) {
	t.Helper()
	key, err := KeyFromHex(strings.Repeat("ef", 32))
	if err != nil {
		t.Fatalf("KeyFromHex: %v", err)
	}
	repo := newFakeCredentialRepo()
	return NewDBCredentialProvider(repo, key, &envCreds{creds: &Credentials{BaseURL: "https://fallback", AccessToken: "global"}}), repo
}

func TestDBCredentialProvider_TenantRowWins(t *testing.T) {
	provider, _ := dbProviderFixture(t)
	ctx := context.Background()

	if _, err := provider.Store(ctx, "tenant-1", "https://tenant.clicksign.com", "token-do-tenant", "segredo-webhook", "production", "bearer", "admin-1", "primeira carga"); err != nil {
		t.Fatalf("Store: %v", err)
	}

	creds, err := provider.Get(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if creds.AccessToken != "token-do-tenant" || creds.WebhookSecret != "segredo-webhook" {
		t.Fatalf("credencial do tenant divergente: %+v", creds)
	}
	if creds.BaseURL != "https://tenant.clicksign.com" || creds.Environment != "production" || creds.AuthScheme != AuthSchemeBearer {
		t.Fatalf("configuração do tenant divergente: %+v", creds)
	}

	// Tenant SEM linha cai no fallback global.
	fallback, err := provider.Get(ctx, "tenant-2")
	if err != nil {
		t.Fatalf("Get fallback: %v", err)
	}
	if fallback.AccessToken != "global" {
		t.Fatalf("fallback divergente: %+v", fallback)
	}
}

func TestDBCredentialProvider_RotationBumpsRevisionAndAudits(t *testing.T) {
	provider, repo := dbProviderFixture(t)
	ctx := context.Background()

	rev1, err := provider.Store(ctx, "tenant-1", "https://x", "token-1", "", "sandbox", "raw", "admin-1", "")
	if err != nil {
		t.Fatalf("Store 1: %v", err)
	}
	rev2, err := provider.Store(ctx, "tenant-1", "https://x", "token-2", "", "sandbox", "raw", "admin-1", "rotação")
	if err != nil {
		t.Fatalf("Store 2: %v", err)
	}
	if rev1 != 1 || rev2 != 2 {
		t.Fatalf("revisões = %d, %d; want 1, 2", rev1, rev2)
	}
	if len(repo.rotations["tenant-1"]) != 2 {
		t.Fatalf("trilha de rotação = %d entradas", len(repo.rotations["tenant-1"]))
	}

	info, err := provider.Info(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if !info.HasToken || info.Revision != 2 || info.HasWebhook {
		t.Fatalf("info mascarada divergente: %+v", info)
	}
	var dumped bytes.Buffer
	dumped.WriteString(info.BaseURL + info.Environment + info.AuthScheme + info.UpdatedAt)
	if strings.Contains(dumped.String(), "token-2") {
		t.Fatal("info mascarada vazou segredo")
	}
}

func TestDBCredentialProvider_InfoUsesFallbackWithoutRow(t *testing.T) {
	provider, _ := dbProviderFixture(t)
	info, err := provider.Info(context.Background(), "tenant-sem-linha")
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if !info.UsesFallback || info.HasToken {
		t.Fatalf("info sem linha divergente: %+v", info)
	}
}

func TestDBCredentialProvider_StoreRequiresKeyAndToken(t *testing.T) {
	repo := newFakeCredentialRepo()
	noKey := NewDBCredentialProvider(repo, nil, nil)
	if _, err := noKey.Store(context.Background(), "t", "https://x", "token", "", "sandbox", "raw", "a", ""); !errors.Is(err, ErrCredentialCryptoKeyMissing) {
		t.Fatalf("sem chave: err = %v", err)
	}

	key, _ := KeyFromHex(strings.Repeat("ef", 32))
	withKey := NewDBCredentialProvider(repo, key, nil)
	if _, err := withKey.Store(context.Background(), "t", "https://x", "   ", "", "sandbox", "raw", "a", ""); err == nil {
		t.Fatal("token vazio deveria falhar")
	}
}

func TestDBCredentialProvider_GetRowWithoutKeyFailsClosed(t *testing.T) {
	repo := newFakeCredentialRepo()
	repo.rows["tenant-1"] = &CredentialRow{
		TenantID: "tenant-1", BaseURL: "https://x", AccessTokenEncrypted: "v1:xxxx:yyyy",
		Environment: "sandbox", AuthScheme: "raw", Revision: 1,
	}
	noKey := NewDBCredentialProvider(repo, nil, nil)
	if _, err := noKey.Get(context.Background(), "tenant-1"); !errors.Is(err, ErrCredentialCryptoKeyMissing) {
		t.Fatalf("linha persistida sem chave deveria falhar fechado, err = %v", err)
	}
}
