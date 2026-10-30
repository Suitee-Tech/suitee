package partnershipaccess

import "testing"

func TestPropertyEligibility(t *testing.T) {
	g := Grant{Sharing: Sharing{SharedPropertyTypes: []string{"house"}, SharedRegions: []string{"Recife"}}, Permissions: []Permission{{Scope: "shared_portfolio", AccessLevel: "view"}}}
	p := Property{Shared: true, Type: "house"}
	p.Address.City = "Recife"
	if !g.IncludesProperty(p) {
		t.Fatal("eligible property rejected")
	}
	for _, change := range []func(*Property){func(p *Property) { p.Shared = false }, func(p *Property) { p.Exclusivity = "exclusive" }, func(p *Property) { p.Type = "apartment" }, func(p *Property) { p.Address.City = "Salvador" }} {
		v := p
		change(&v)
		if g.IncludesProperty(v) {
			t.Fatal("private property allowed")
		}
	}
	if Resolve(map[string]Grant{"a": g}, "b", Sharing{}, g.Permissions).Allows("shared_portfolio") {
		t.Fatal("missing directional grant fell back to legacy")
	}
	if Resolve(nil, "a", Sharing{ShareOwnerContact: true}, nil).Sharing.ShareLeadContact {
		t.Fatal("owner consent leaked lead contact")
	}
}
