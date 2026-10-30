package services

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/oginoino/toca-core/internal/domain/core/partnership"
	repository "github.com/oginoino/toca-core/internal/repositories"
	access "github.com/oginoino/toca/pkg/partnershipaccess"
)

type agreementRepo struct {
	repository.PartnershipRepository
	p      *partnership.Partnership
	writes int
}

func copyAgreement(p *partnership.Partnership) *partnership.Partnership {
	b, _ := json.Marshal(p)
	var c partnership.Partnership
	_ = json.Unmarshal(b, &c)
	return &c
}
func (r *agreementRepo) GetByID(context.Context, string) (*partnership.Partnership, error) {
	return copyAgreement(r.p), nil
}
func (r *agreementRepo) Create(_ context.Context, p *partnership.Partnership) error {
	r.p = copyAgreement(p)
	r.writes++
	return nil
}
func (r *agreementRepo) Update(_ context.Context, _ string, p *partnership.Partnership) error {
	if r.p.Version != p.Version-1 {
		return partnership.ErrConflict
	}
	r.p = copyAgreement(p)
	r.writes++
	return nil
}
func agreementFixture() (*PartnershipService, *agreementRepo) {
	r := &agreementRepo{p: &partnership.Partnership{ID: "p", RequesterID: "a", TargetID: "b", Type: "partner", Status: "active", Version: 1, FinancialVersion: 1, FinancialConfig: partnership.FinancialConfig{CommissionModel: partnership.CommissionModel{OwnerCompanyPct: 50, ClosingCompanyPct: 50}}, SharingConfig: partnership.SharingConfig{}, Permissions: []partnership.Permission{{Scope: "shared_portfolio", AccessLevel: "view"}}}}
	return NewPartnershipService(r), r
}
func command(p *partnership.Partnership, action string) AgreementChange {
	v := p.Version
	return AgreementChange{ExpectedVersion: &v, Action: action}
}
func TestAgreementOnlyChangesActorGrant(t *testing.T) {
	s, r := agreementFixture()
	c := command(r.p, "save")
	c.Grant = &access.Grant{Sharing: access.Sharing{ShareOwnerContact: true, ShareLeadContact: true, AllowCoBrokering: true}, Permissions: []access.Permission{{Scope: "leads", AccessLevel: "view"}}}
	p, err := s.ChangeAgreement(context.Background(), "p", "b", "user-b", c)
	if err != nil {
		t.Fatal(err)
	}
	if p.GrantFrom("a").Sharing.ShareOwnerContact || p.GrantFrom("a").Allows("leads") {
		t.Fatal("recipient expanded access to owner")
	}
	if !p.GrantFrom("b").Sharing.ShareLeadContact || !p.GrantFrom("b").Allows("leads") {
		t.Fatal("actor grant not updated")
	}
	if len(p.AuditLogs) != 1 || p.AuditLogs[0].UserID != "user-b" || p.AuditLogs[0].TenantID != "b" {
		t.Fatal("missing actor audit")
	}
	if _, err := s.ChangeAgreement(context.Background(), "p", "a", "user-a", c); !errors.Is(err, partnership.ErrConflict) {
		t.Fatal("stale version accepted", err)
	}
	if _, err := s.ChangeAgreement(context.Background(), "p", "c", "user-c", command(p, "save")); !errors.Is(err, ErrAgreementAccess) {
		t.Fatal("third company accepted", err)
	}
}
func TestFinancialProposalRequiresOtherCompanyAcceptance(t *testing.T) {
	s, r := agreementFixture()
	c := command(r.p, "save")
	c.FinancialConfig = &partnership.FinancialConfig{CommissionModel: partnership.CommissionModel{OwnerCompanyPct: 60, ClosingCompanyPct: 40}}
	p, err := s.ChangeAgreement(context.Background(), "p", "a", "u", c)
	if err != nil {
		t.Fatal(err)
	}
	if p.FinancialConfig.CommissionModel.OwnerCompanyPct != 50 || p.PendingFinancial == nil {
		t.Fatal("terms changed before consent")
	}
	if _, err := s.ChangeAgreement(context.Background(), "p", "a", "u", command(p, "accept_financial")); !errors.Is(err, ErrAgreementAccess) {
		t.Fatal("self acceptance allowed")
	}
	p, err = s.ChangeAgreement(context.Background(), "p", "b", "v", command(p, "accept_financial"))
	if err != nil {
		t.Fatal(err)
	}
	if p.PendingFinancial != nil || p.FinancialConfig.CommissionModel.OwnerCompanyPct != 60 || p.FinancialVersion != 3 {
		t.Fatal("accepted version missing")
	}
}
func TestInvalidFinancialUpdateIsAtomic(t *testing.T) {
	for _, config := range []partnership.FinancialConfig{{RoyaltyPercentage: 200}, {AdminFee: -1}, {CommissionModel: partnership.CommissionModel{OwnerCompanyPct: 80, ClosingCompanyPct: 80}}} {
		s, r := agreementFixture()
		c := command(r.p, "save")
		c.FinancialConfig = &config
		c.Grant = &access.Grant{Sharing: access.Sharing{ShareOwnerContact: true}}
		if _, err := s.ChangeAgreement(context.Background(), "p", "b", "u", c); err == nil {
			t.Fatal("invalid financial config accepted")
		}
		if r.writes != 0 {
			t.Fatal("partial write")
		}
	}
}
func TestAgreementLifecycleAndReviewedVersion(t *testing.T) {
	s, r := agreementFixture()
	r.p.Status = "pending"
	r.p.Grants = map[string]access.Grant{"a": {Permissions: []access.Permission{{Scope: "shared_portfolio", AccessLevel: "view"}}}, "b": {}}
	if _, err := s.ChangeAgreement(context.Background(), "p", "b", "u", AgreementChange{Action: "accept"}); !errors.Is(err, ErrAgreementVersion) {
		t.Fatal("unreviewed accept", err)
	}
	c := command(r.p, "accept")
	c.Grant = &access.Grant{}
	if _, err := s.ChangeAgreement(context.Background(), "p", "a", "u", c); !errors.Is(err, ErrAgreementAccess) {
		t.Fatal("initiator accepted")
	}
	p, err := s.ChangeAgreement(context.Background(), "p", "b", "u", c)
	if err != nil {
		t.Fatal(err)
	}
	c = command(p, "suspend")
	c.Reason = "Pausa acordada"
	p, err = s.ChangeAgreement(context.Background(), "p", "a", "u", c)
	if err != nil || p.Status != "suspended" {
		t.Fatal("suspension failed", err)
	}
	c = command(p, "save")
	c.Grant = &access.Grant{Permissions: []access.Permission{{Scope: "shared_portfolio", AccessLevel: "view"}}}
	if _, err := s.ChangeAgreement(context.Background(), "p", "b", "u", c); err == nil {
		t.Fatal("suspended agreement modified")
	}
	c = command(p, "terminate")
	c.Reason = "Encerramento"
	p, err = s.ChangeAgreement(context.Background(), "p", "b", "u", c)
	if err != nil || p.Status != "inactive" {
		t.Fatal("termination failed", err)
	}
	if _, err := s.SharedPortfolioForPartnership(context.Background(), "p", "a"); !errors.Is(err, ErrSharedPortfolioUnavailable) {
		t.Fatal("revocation failed", err)
	}
}
func TestInvitationOnlyGrantsInvitersData(t *testing.T) {
	s, r := agreementFixture()
	p := copyAgreement(r.p)
	p.Status = "pending"
	if err := s.CreatePartnership(context.Background(), p, "user-a"); err != nil {
		t.Fatal(err)
	}
	if p.GrantFrom("b").Allows("shared_portfolio") {
		t.Fatal("inviter grants other company data")
	}
	p.TargetID = p.RequesterID
	if err := s.CreatePartnership(context.Background(), p, "u"); err == nil {
		t.Fatal("self invitation accepted")
	}
}

func TestEquivalentLegacyFinancialTermsDoNotCreateProposal(t *testing.T) {
	s, r := agreementFixture()
	r.p.FinancialConfig = partnership.FinancialConfig{CommissionSplit: "60/40"}
	c := command(r.p, "save")
	c.FinancialConfig = &partnership.FinancialConfig{CommissionModel: partnership.CommissionModel{OwnerCompanyPct: 60, ClosingCompanyPct: 40}, RoyaltyBase: partnership.RoyaltyBaseGrossCommission}
	p, err := s.ChangeAgreement(context.Background(), "p", "a", "u", c)
	if err != nil || p.PendingFinancial != nil {
		t.Fatalf("equivalent defaults produced proposal: %+v %v", p, err)
	}
}
func TestFeeRuleMustBeAcceptedExplicitly(t *testing.T) {
	s, r := agreementFixture()
	r.p.FinancialConfig.AdminFee = 100
	r.p.Status = partnership.PartnershipStatusPending
	c := command(r.p, "accept")
	c.Grant = &access.Grant{}
	if _, err := s.ChangeAgreement(context.Background(), "p", "b", "user", c); !errors.Is(err, ErrAgreementValidation) {
		t.Fatal("accepted fees without payer", err)
	}
	if r.writes != 0 {
		t.Fatal("invalid acceptance persisted")
	}
	r.p.FinancialConfig.FeePayer = access.FeeClosingPaysSource
	if _, err := s.ChangeAgreement(context.Background(), "p", "b", "user", c); err != nil {
		t.Fatal(err)
	}
	next := r.p.FinancialConfig
	next.FeePayer = access.FeeSourcePaysClosing
	c = command(r.p, "save")
	c.FinancialConfig = &next
	if _, err := s.ChangeAgreement(context.Background(), "p", "a", "user", c); err != nil {
		t.Fatal(err)
	}
	if r.p.FinancialConfig.FeePayer != access.FeeClosingPaysSource || r.p.PendingFinancial == nil {
		t.Fatal("payer changed without consent")
	}
	if _, err := s.ChangeAgreement(context.Background(), "p", "b", "user", command(r.p, "accept_financial")); err != nil {
		t.Fatal(err)
	}
	if r.p.FinancialConfig.FeePayer != access.FeeSourcePaysClosing {
		t.Fatal("payer change not accepted")
	}
}
