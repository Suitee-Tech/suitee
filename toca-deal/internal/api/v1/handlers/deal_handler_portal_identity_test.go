package handlers

import (
	"testing"

	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

func TestFindMyPartyMatchesPartyWithoutSigner(t *testing.T) {
	d := &deal.Deal{
		Parties: []deal.Party{
			{ID: "party-1", AccountID: "account-1", Email: "cliente@example.test", Role: deal.PartyRole("buyer")},
		},
	}
	if got := findMyParty(d, "account-1", ""); got == nil || got.ID != "party-1" {
		t.Fatalf("parte por account_id não encontrada: %#v", got)
	}
	if got := findMyParty(d, "", "CLIENTE@EXAMPLE.TEST"); got == nil || got.ID != "party-1" {
		t.Fatalf("parte por e-mail (case-insensitive) não encontrada: %#v", got)
	}
	if got := findMyParty(d, "other", "other@example.test"); got != nil {
		t.Fatalf("não pode encontrar parte de outra pessoa: %#v", got)
	}
}
