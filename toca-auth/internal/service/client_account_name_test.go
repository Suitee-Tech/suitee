package service

import (
	"context"
	"testing"

	"github.com/oginoino/toca-auth/internal/domain/account"
	"github.com/oginoino/toca-auth/internal/domain/auth"
)

type clientNameAccountRepo struct {
	fakeAccountRepo
}

func (r *clientNameAccountRepo) Create(ctx context.Context, acc *account.Account) error {
	acc.ID = "client-account"
	return r.fakeAccountRepo.Create(ctx, acc)
}

// CreateService delegates to the overridden Create so the fixed "client-account"
// ID also applies on the pre-auth (service-role) path exercised by
// EnsureClientAccount.
func (r *clientNameAccountRepo) CreateService(ctx context.Context, acc *account.Account) error {
	return r.Create(ctx, acc)
}

type clientNameCredentialRepo struct {
	fakeCredentialRepo
}

func (r *clientNameCredentialRepo) Create(_ context.Context, cred *auth.Credential) error {
	if r.byEmail == nil {
		r.byEmail = map[string]*auth.Credential{}
	}
	r.byEmail[cred.Email] = cred
	return nil
}

func (r *clientNameCredentialRepo) CreateService(ctx context.Context, cred *auth.Credential) error {
	return r.Create(ctx, cred)
}

func TestEnsureClientAccountSeparatesName(t *testing.T) {
	for _, tc := range []struct {
		name, first, last, full string
	}{
		{"Lucas Mendes Barros", "Lucas", "Mendes Barros", "Lucas Mendes Barros"},
		{"  Maria   Clara de Souza  ", "Maria", "Clara de Souza", "Maria Clara de Souza"},
		{"Lucas", "Lucas", "", "Lucas"},
		{"", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			accounts := &clientNameAccountRepo{}
			credentials := &clientNameCredentialRepo{}
			svc := &AuthService{
				accountRepo:           accounts,
				credentialRepo:        credentials,
				accountServiceRepo:    accounts,
				credentialServiceRepo: credentials,
			}
			id, err := svc.EnsureClientAccount(context.Background(), "client@example.test", tc.name, "")
			if err != nil {
				t.Fatal(err)
			}
			acc, _ := accounts.GetByID(context.Background(), id)
			if acc.Individual.FirstName != tc.first || acc.Individual.LastName != tc.last || acc.Individual.FullName != tc.full {
				t.Fatalf("unexpected name fields: %#v", acc.Individual)
			}
			// Reopening the invitation must reuse the account without replacing its name.
			if reused, err := svc.EnsureClientAccount(context.Background(), "client@example.test", "Other Name", ""); err != nil || reused != id {
				t.Fatalf("account was not reused: id=%q err=%v", reused, err)
			}
			if acc.Individual.FirstName != tc.first || acc.Individual.LastName != tc.last {
				t.Fatal("reusing the account replaced its name")
			}
		})
	}
}
