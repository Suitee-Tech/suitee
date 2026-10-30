package services

import (
	"testing"

	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

func TestCustomerEmailsWithoutAccount(t *testing.T) {
	d := &deal.Deal{}
	d.Parties = []deal.Party{
		{Role: deal.PartyRoleBuyer, Email: "luana@example.com", AccountID: ""},
		{Role: deal.PartyRoleTenant, Email: " joao@example.com ", AccountID: ""},
		{Role: deal.PartyRoleOwner, Email: "ayla@example.com", AccountID: ""},
		{Role: deal.PartyRoleBuyer, Email: "portal@example.com", AccountID: "acc-1"},
		{Role: deal.PartyRoleBuyer, Email: "luana@example.com", AccountID: ""}, // duplicata
		{Role: deal.PartyRoleBuyer, Email: "", AccountID: ""},
	}

	got := CustomerEmailsWithoutAccount(d)
	want := []string{"luana@example.com", "joao@example.com"}
	if len(got) != len(want) {
		t.Fatalf("emails = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("emails = %v, want %v", got, want)
		}
	}

	if got := CustomerEmailsWithoutAccount(nil); got != nil {
		t.Fatalf("nil deal must return nil, got %v", got)
	}
}
