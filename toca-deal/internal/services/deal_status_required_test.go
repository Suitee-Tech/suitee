package services_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	service "github.com/oginoino/toca/toca-deal/internal/services"
)

// boolPtr returns a pointer to the given bool literal — used to set the
// optional SignatureRequired / IsRequired fields on Party / Signer.
func boolPtr(v bool) *bool { return &v }

// dealWithRequiredFlow builds a SALE deal stuck at Contract stage with one
// signed required signer and one optional unsigned signer. Used to exercise
// the StatusConverted gating logic.
func dealWithRequiredFlow(t *testing.T) *deal.Deal {
	t.Helper()
	d, err := deal.New(deal.TypeSale, "prop-1", "owner-acc", "buyer-acc", "broker-1", 100000, "tenant-1")
	if err != nil {
		t.Fatalf("new deal: %v", err)
	}
	d.Status = deal.StatusContract

	// Two parties — primary buyer + primary owner — required by default.
	if err := d.AddParty(deal.Party{
		ID: "party-buyer", Role: deal.PartyRoleBuyer, Name: "Buyer", Email: "b@x.com",
		IsPrimary: true, NoSpouseDeclared: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("add buyer party: %v", err)
	}
	if err := d.AddParty(deal.Party{
		ID: "party-owner", Role: deal.PartyRoleOwner, Name: "Owner", Email: "o@x.com",
		IsPrimary: true, NoSpouseDeclared: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("add owner party: %v", err)
	}

	// Accepted proposal so UpdateStatus → StatusContract checks pass.
	d.Proposals = append(d.Proposals, deal.Proposal{
		ID:     "prop-1",
		Status: deal.ProposalStatusAccepted,
		AcceptedBy: []deal.ProposalAcceptance{
			{PartyID: "party-buyer", Role: deal.PartyRoleBuyer, AcceptedAt: time.Now()},
			{PartyID: "party-owner", Role: deal.PartyRoleOwner, AcceptedAt: time.Now()},
		},
	})

	// Required signer — already signed.
	d.Signers = append(d.Signers, deal.Signer{
		ID: "signer-buyer", Name: "Buyer", Email: "b@x.com",
		Role: deal.SignatoryRoleComprador, Status: deal.SignerStatusSigned,
	})
	// Optional signer — pending.
	d.Signers = append(d.Signers, deal.Signer{
		ID: "signer-witness", Name: "Witness", Email: "w@x.com",
		Role: deal.SignatoryRoleTestemunha, Status: deal.SignerStatusPending,
		IsRequired: boolPtr(false),
	})
	return d
}

func TestUpdateStatus_OptionalSignerDoesNotBlock(t *testing.T) {
	d := dealWithRequiredFlow(t)
	repo := &mockDealRepo{d: d}
	svc := service.NewDealService(repo, nil, nil, &mockEventBus{}, nil)

	got, err := svc.UpdateStatusTrusted(context.Background(), d.TenantID, d.ID, "", deal.StatusConverted, "user-1", "")
	if err != nil {
		t.Fatalf("expected to advance with only optional signer pending, got err: %v", err)
	}
	if got.Status != deal.StatusConverted {
		t.Errorf("expected StatusConverted, got %v", got.Status)
	}
}

func TestUpdateStatus_RequiredSignerBlocks(t *testing.T) {
	d := dealWithRequiredFlow(t)
	// Flip witness to required + flip buyer to pending → required signer pending.
	d.Signers[0].Status = deal.SignerStatusPending
	d.Signers[1].IsRequired = boolPtr(true)
	d.Signers[1].Status = deal.SignerStatusPending
	repo := &mockDealRepo{d: d}
	svc := service.NewDealService(repo, nil, nil, &mockEventBus{}, nil)

	_, err := svc.UpdateStatusTrusted(context.Background(), d.TenantID, d.ID, "", deal.StatusConverted, "user-1", "")
	if err == nil {
		t.Fatal("expected error blocking conversion when required signer pending, got nil")
	}
	if !errors.Is(err, service.ErrSignersNotComplete) {
		t.Errorf("expected ErrSignersNotComplete, got %v", err)
	}
}

// stubSyncer simulates a contract that's already SIGNED — calling
// SyncDealSignersFromActiveContract flips each Signer to SIGNED on the
// in-memory deal, just like the real ContractService would after a
// Clicksign envelope_finished event.
type stubSyncer struct {
	d      *deal.Deal
	called int
}

func (s *stubSyncer) SyncDealSignersFromActiveContract(_ context.Context, _, _ string) error {
	s.called++
	if s.d == nil {
		return nil
	}
	for i := range s.d.Signers {
		s.d.Signers[i].Status = deal.SignerStatusSigned
	}
	for i := range s.d.Parties {
		s.d.Parties[i].SignStatus = deal.SignerStatusSigned
	}
	return nil
}

func TestUpdateStatus_AutoHealsStaleSignersFromContract(t *testing.T) {
	d := dealWithRequiredFlow(t)
	// Reset both signers to PENDING — happy path is contract.Status=SIGNED but
	// the deal projection never caught up (the production bug).
	d.Signers[0].Status = deal.SignerStatusPending
	d.Signers[1].IsRequired = boolPtr(true)
	d.Signers[1].Status = deal.SignerStatusPending

	repo := &mockDealRepo{d: d}
	syncer := &stubSyncer{d: d}
	svc := service.NewDealService(repo, nil, nil, &mockEventBus{}, nil).WithSignerSyncer(syncer)

	got, err := svc.UpdateStatusTrusted(context.Background(), d.TenantID, d.ID, "", deal.StatusConverted, "user-1", "")
	if err != nil {
		t.Fatalf("expected auto-heal to recover, got err: %v", err)
	}
	if got.Status != deal.StatusConverted {
		t.Errorf("expected StatusConverted after auto-heal, got %v", got.Status)
	}
	if syncer.called != 1 {
		t.Errorf("expected syncer to be called exactly once, got %d", syncer.called)
	}
}

func TestUpdateStatus_SyncerStillBlocksWhenContractAlsoStale(t *testing.T) {
	d := dealWithRequiredFlow(t)
	d.Signers[0].Status = deal.SignerStatusPending
	d.Signers[1].IsRequired = boolPtr(true)
	d.Signers[1].Status = deal.SignerStatusPending

	repo := &mockDealRepo{d: d}
	// Syncer no-op — contract didn't have updates either.
	syncer := &stubSyncer{d: nil}
	svc := service.NewDealService(repo, nil, nil, &mockEventBus{}, nil).WithSignerSyncer(syncer)

	_, err := svc.UpdateStatusTrusted(context.Background(), d.TenantID, d.ID, "", deal.StatusConverted, "user-1", "")
	if err == nil {
		t.Fatal("expected gate to still block when contract is also stale")
	}
	if !errors.Is(err, service.ErrSignersNotComplete) {
		t.Errorf("expected ErrSignersNotComplete, got %v", err)
	}
	if syncer.called != 1 {
		t.Errorf("syncer should be tried once before failing, got %d", syncer.called)
	}
}

func TestUpdateStatus_SkipsSyncerWhenAllSignersAlreadySigned(t *testing.T) {
	d := dealWithRequiredFlow(t)
	// All required signers already signed — happy path, syncer must NOT run
	// (avoids an extra contract fetch on every successful advance).
	syncer := &stubSyncer{}
	svc := service.NewDealService(&mockDealRepo{d: d}, nil, nil, &mockEventBus{}, nil).WithSignerSyncer(syncer)

	_, err := svc.UpdateStatusTrusted(context.Background(), d.TenantID, d.ID, "", deal.StatusConverted, "user-1", "")
	if err != nil {
		t.Fatalf("expected happy path success, got err: %v", err)
	}
	if syncer.called != 0 {
		t.Errorf("syncer should not run when projection is already in sync, got %d calls", syncer.called)
	}
}

func TestParty_IsSignatureRequiredFallback(t *testing.T) {
	cases := []struct {
		name string
		p    deal.Party
		want bool
	}{
		{"unset + primary", deal.Party{IsPrimary: true}, true},
		{"unset + secondary", deal.Party{IsPrimary: false}, false},
		{"explicit true overrides primary=false", deal.Party{IsPrimary: false, SignatureRequired: boolPtr(true)}, true},
		{"explicit false overrides primary=true", deal.Party{IsPrimary: true, SignatureRequired: boolPtr(false)}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.p.IsSignatureRequired(); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSigner_IsSignerRequiredFallback(t *testing.T) {
	cases := []struct {
		name string
		s    deal.Signer
		want bool
	}{
		{"unset defaults to true", deal.Signer{}, true},
		{"explicit true", deal.Signer{IsRequired: boolPtr(true)}, true},
		{"explicit false", deal.Signer{IsRequired: boolPtr(false)}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.s.IsSignerRequired(); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
