package repositories

import (
	"strings"
	"testing"
)

func TestPortalIdentityMatchesPartiesAndSigners(t *testing.T) {
	clauses, args := portalIdentityMatchClauses("account-1", "Cliente@Example.test")
	if len(clauses) != 2 {
		t.Fatalf("expected parties and signers clauses, got %d", len(clauses))
	}
	joined := strings.Join(clauses, " ")
	if !strings.Contains(joined, "'parties'") || !strings.Contains(joined, "'signers'") {
		t.Fatalf("identity must search parties and signers: %s", joined)
	}
	if !strings.Contains(joined, "lower(COALESCE(value->>'email'") {
		t.Fatalf("email match must be case-insensitive: %s", joined)
	}
	if len(args) != 2 || args[0] != "account-1" || args[1] != "Cliente@Example.test" {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestPortalIdentityWithoutIdentifiersDoesNotQuery(t *testing.T) {
	clauses, args := portalIdentityMatchClauses("", "")
	if len(clauses) != 0 || len(args) != 0 {
		t.Fatalf("empty identity must not query another tenant's deals: %#v %#v", clauses, args)
	}
}
