package services

import (
	"context"
	access "github.com/oginoino/toca/pkg/partnershipaccess"
	"github.com/oginoino/toca/toca-deal/internal/repositories"
	"testing"
)

type reviewCrossLookup struct {
	CrossTenantLookup
	eligible bool
}

func (*reviewCrossLookup) PropertyOwnerTenant(context.Context, string) (string, error) {
	return "owner-company", nil
}
func (*reviewCrossLookup) LeadOwnerTenant(context.Context, string) (string, error) {
	return "owner-company", nil
}
func (*reviewCrossLookup) ActivePartnershipBetween(context.Context, string, string) (*repositories.CrossPartnership, error) {
	return &repositories.CrossPartnership{ID: "agreement", RequesterID: "owner-company", TargetID: "partner-company", Status: "active", Permissions: []repositories.CrossPartnershipPermission{{Scope: "shared_portfolio", AccessLevel: "view"}, {Scope: "leads", AccessLevel: "view"}}, Sharing: repositories.CrossPartnershipSharing{AllowCoBrokering: true, SharedPropertyTypes: []string{"house"}, SharedRegions: []string{"Recife"}}}, nil
}

func (r *reviewCrossLookup) EligibleSharedResource(context.Context, string, string, string, access.Grant) (bool, error) {
	return r.eligible, nil
}
func TestCoBrokeringRequiresEligibleResourceAndActorPermission(t *testing.T) {
	for _, resourceEligible := range []bool{false, true} {
		for _, actorAllowed := range []bool{false, true} {
			s := &DealService{crossRepo: &reviewCrossLookup{eligible: resourceEligible}}
			for _, authorize := range []func(context.Context, string, string, bool) (coBrokeringDecision, error){s.authorizeCrossTenantProperty, s.authorizeCrossTenantLead} {
				decision, err := authorize(context.Background(), "resource", "partner-company", actorAllowed)
				if resourceEligible && actorAllowed {
					if err != nil || !decision.coBrokering || decision.snapshot == nil || decision.snapshot.ID != "agreement" {
						t.Fatal("eligible resource denied or snapshot missing", err)
					}
				} else if err == nil {
					t.Fatal("ineligible resource or actor authorized")
				}
			}
		}
	}
}
