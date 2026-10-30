package service

import (
	"context"
	access "github.com/oginoino/toca/pkg/partnershipaccess"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
	"github.com/oginoino/toca/toca-crm/internal/repository"
	"testing"
)

type sharedLeadRepo struct{ lead.Repository }

func (*sharedLeadRepo) ListSharedForPartner(context.Context, string) ([]*lead.Lead, error) {
	return []*lead.Lead{{ID: "lead", Name: "Nome privado", Email: "private@example.test"}}, nil
}

type sharedLeadCross struct {
	repository.CrossTenantRepository
	p *repository.PartnershipSharing
}

func (r *sharedLeadCross) ActivePartnershipsForCompany(context.Context, string) ([]*repository.PartnershipSharing, error) {
	return []*repository.PartnershipSharing{r.p}, nil
}
func TestLeadContactNeedsOwnerDirectionalConsent(t *testing.T) {
	permissions := []access.Permission{{Scope: "leads", AccessLevel: "view"}}
	for _, consent := range []bool{false, true} {
		p := &repository.PartnershipSharing{ID: "agreement", RequesterID: "owner", TargetID: "recipient", Status: "active", CompanyNames: map[string]string{"owner": "Parceiro"}, Grants: map[string]access.Grant{"owner": {Permissions: permissions, Sharing: access.Sharing{ShareOwnerContact: true, ShareLeadContact: consent}}, "recipient": {Permissions: permissions, Sharing: access.Sharing{ShareLeadContact: true}}}}
		s := &LeadService{repo: &sharedLeadRepo{}, crossRepo: &sharedLeadCross{p: p}}
		out, err := s.SharedLeadsForCompany(context.Background(), "recipient")
		if err != nil || len(out) != 1 {
			t.Fatal(out, err)
		}
		if (out[0].Lead.Email != "") != consent {
			t.Fatalf("contact visibility bypassed owner consent: %+v", out[0].Lead)
		}
		if out[0].SourceCompanyName != "Parceiro" {
			t.Fatal("missing public identity")
		}
		delete(p.Grants, "owner")
		out, err = s.SharedLeadsForCompany(context.Background(), "recipient")
		if err != nil || len(out) != 0 {
			t.Fatal("recipient self-granted lead access", out, err)
		}
	}
}
