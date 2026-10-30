package lead

import "testing"

func TestDeriveTags_LeadTypeAliases(t *testing.T) {
	l := &Lead{
		Source: LeadSourceWebsite,
		Role:   ClientRoleBuyer,
	}
	tags := l.DeriveTags()
	if !containsTag(tags, "COMPRA_VENDA") {
		t.Fatalf("expected COMPRA_VENDA in tags, got %v", tags)
	}

	l.Role = ClientRoleTenant
	tags = l.DeriveTags()
	if !containsTag(tags, "LOCACAO") {
		t.Fatalf("expected LOCACAO in tags, got %v", tags)
	}

	l.Role = ClientRoleOwner
	tags = l.DeriveTags()
	if !containsTag(tags, "CAPTACAO") {
		t.Fatalf("expected CAPTACAO in tags, got %v", tags)
	}
}

func containsTag(tags []string, v string) bool {
	for _, t := range tags {
		if t == v {
			return true
		}
	}
	return false
}
