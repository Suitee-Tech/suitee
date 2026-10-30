package services_test

import (
	"context"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	service "github.com/oginoino/toca/toca-deal/internal/services"
)

// Reproduces the production bug reported by the user: after Clicksign emits the
// envelope finished event, the contract status flips to SIGNED on screen but
// the per-signer rows in "Assinatura Eletrônica" stay PENDING because the
// deal's Signers[] / Parties[] arrays never get mirrored. The current
// implementation already mirrors via syncAllDealSignersFromContract, so these
// tests pin down the expected behavior and catch regressions.
func TestMarkSignedExternally_SyncsDealSignersToSigned(t *testing.T) {
	// Build a deal that already has a signer + party with matching IDs (this
	// is the post-Send-for-signature state).
	d, _ := deal.New(deal.TypeRent, "prop-1", "owner-1", "cust-1", "broker-1", 2500, "tenant-1")
	d.ID = "deal-1"
	if err := d.AddSigner("João Inquilino", "joao@example.com", deal.SignatoryRoleLocatario); err != nil {
		t.Fatalf("AddSigner: %v", err)
	}
	signerID := d.Signers[0].ID
	if signerID == "" {
		t.Fatal("signer should have a generated ID")
	}

	// Contract mirrors the signer.
	c := &contract.Contract{
		ID:                 "contract-1",
		DealID:             d.ID,
		TenantID:           "tenant-1",
		Status:             contract.StatusPending,
		ExternalEnvelopeID: "ext-env-123",
		Type:               contract.TypeLocacao,
		StartDate:          time.Now(),
		Signatories: []contract.Signatory{
			{ID: signerID, Name: "João Inquilino", Email: "joao@example.com", Role: contract.RoleLocatario},
		},
	}

	dealRepo := &mockDealRepo{d: d}
	contractRepo := &mockContractRepo{c: c}
	svc := service.NewContractService(contractRepo, dealRepo, &recordingBus{}, nil)

	finished := time.Date(2026, 5, 14, 18, 0, 0, 0, time.UTC)
	if _, err := svc.MarkSignedExternally(context.Background(), "tenant-1", "contract-1", "ext-env-123", finished, ""); err != nil {
		t.Fatalf("MarkSignedExternally: %v", err)
	}

	if contractRepo.c.Status != contract.StatusSigned {
		t.Errorf("contract.Status = %q, want %q", contractRepo.c.Status, contract.StatusSigned)
	}
	if got := d.Signers[0].Status; got != deal.SignerStatusSigned {
		t.Errorf("deal.Signers[0].Status = %q, want %q (this is the bug surface)", got, deal.SignerStatusSigned)
	}
	if d.Signers[0].SignedAt == nil {
		t.Error("deal.Signers[0].SignedAt should be stamped")
	}
	// The party row is what PartiesPanel / DealPartiesSection render in the
	// "Partes envolvidas" portal section. It must also flip.
	if got := d.Parties[0].SignStatus; got != deal.SignerStatusSigned {
		t.Errorf("deal.Parties[0].SignStatus = %q, want %q (portal would show PENDING)", got, deal.SignerStatusSigned)
	}
	if d.Parties[0].SignedAt == nil {
		t.Error("deal.Parties[0].SignedAt should be stamped")
	}
}

func TestMarkSignedExternally_SyncsTwoSigners(t *testing.T) {
	d, _ := deal.New(deal.TypeRent, "prop-1", "owner-1", "cust-1", "broker-1", 2500, "tenant-1")
	d.ID = "deal-1"
	_ = d.AddSigner("João Inquilino", "joao@example.com", deal.SignatoryRoleLocatario)
	_ = d.AddSigner("Maria Locador", "maria@example.com", deal.SignatoryRoleLocador)
	idTenant := d.Signers[0].ID
	idOwner := d.Signers[1].ID

	c := &contract.Contract{
		ID:                 "contract-1",
		DealID:             d.ID,
		TenantID:           "tenant-1",
		Status:             contract.StatusPending,
		ExternalEnvelopeID: "ext-env-123",
		Type:               contract.TypeLocacao,
		StartDate:          time.Now(),
		Signatories: []contract.Signatory{
			{ID: idTenant, Name: "João Inquilino", Email: "joao@example.com", Role: contract.RoleLocatario},
			{ID: idOwner, Name: "Maria Locador", Email: "maria@example.com", Role: contract.RoleLocador},
		},
	}

	svc := service.NewContractService(&mockContractRepo{c: c}, &mockDealRepo{d: d}, &recordingBus{}, nil)
	if _, err := svc.MarkSignedExternally(context.Background(), "tenant-1", "contract-1", "ext-env-123", time.Now(), ""); err != nil {
		t.Fatalf("MarkSignedExternally: %v", err)
	}

	for i, s := range d.Signers {
		if s.Status != deal.SignerStatusSigned {
			t.Errorf("deal.Signers[%d].Status = %q, want %q", i, s.Status, deal.SignerStatusSigned)
		}
	}
	for i, p := range d.Parties {
		if p.SignStatus != deal.SignerStatusSigned {
			t.Errorf("deal.Parties[%d].SignStatus = %q, want %q", i, p.SignStatus, deal.SignerStatusSigned)
		}
	}
}

func TestMarkSignerSignedExternally_SyncsOneSigner(t *testing.T) {
	d, _ := deal.New(deal.TypeRent, "prop-1", "owner-1", "cust-1", "broker-1", 2500, "tenant-1")
	d.ID = "deal-1"
	_ = d.AddSigner("João Inquilino", "joao@example.com", deal.SignatoryRoleLocatario)
	_ = d.AddSigner("Maria Locador", "maria@example.com", deal.SignatoryRoleLocador)
	idTenant := d.Signers[0].ID
	idOwner := d.Signers[1].ID

	c := &contract.Contract{
		ID:                 "contract-1",
		DealID:             d.ID,
		TenantID:           "tenant-1",
		Status:             contract.StatusPending,
		ExternalEnvelopeID: "ext-env-123",
		Type:               contract.TypeLocacao,
		StartDate:          time.Now(),
		Signatories: []contract.Signatory{
			{ID: idTenant, Name: "João Inquilino", Email: "joao@example.com", Role: contract.RoleLocatario},
			{ID: idOwner, Name: "Maria Locador", Email: "maria@example.com", Role: contract.RoleLocador},
		},
	}

	svc := service.NewContractService(&mockContractRepo{c: c}, &mockDealRepo{d: d}, &recordingBus{}, nil)
	if err := svc.MarkSignerSignedExternally(context.Background(), "tenant-1", "contract-1", idTenant, time.Now(), "", "ext-env-123"); err != nil {
		t.Fatalf("MarkSignerSignedExternally: %v", err)
	}

	// Only the tenant should flip; the owner must remain pending.
	if d.Signers[0].Status != deal.SignerStatusSigned {
		t.Errorf("tenant Signer should be SIGNED, got %q", d.Signers[0].Status)
	}
	if d.Parties[0].SignStatus != deal.SignerStatusSigned {
		t.Errorf("tenant Party.SignStatus should be SIGNED, got %q", d.Parties[0].SignStatus)
	}
	if d.Signers[1].Status == deal.SignerStatusSigned {
		t.Errorf("owner Signer should still be PENDING, got %q", d.Signers[1].Status)
	}
	if d.Parties[1].SignStatus == deal.SignerStatusSigned {
		t.Errorf("owner Party.SignStatus should still be PENDING, got %q", d.Parties[1].SignStatus)
	}
}

// Production drift scenario: the Contract.Signatory.ID does NOT match the
// Deal.Signer.ID, but the emails do. This is what happens when a contract
// is created from an envelope that was edited or when a Reconcile re-emits
// events with the envelope SignerID instead of the original contract
// signatory ID. The fix is the email fallback in UpdateSignerStatusByIDOrEmail.
func TestMarkSignedExternally_FallsBackToEmailWhenIDsDrift(t *testing.T) {
	d, _ := deal.New(deal.TypeRent, "prop-1", "owner-1", "cust-1", "broker-1", 2500, "tenant-1")
	d.ID = "deal-1"
	_ = d.AddSigner("João Inquilino", "joao@example.com", deal.SignatoryRoleLocatario)

	// Contract.Signatory.ID is intentionally different from d.Signers[0].ID.
	// This mirrors a real production state where the contract was re-created
	// or the envelope was edited mid-flight.
	c := &contract.Contract{
		ID:                 "contract-1",
		DealID:             d.ID,
		TenantID:           "tenant-1",
		Status:             contract.StatusPending,
		ExternalEnvelopeID: "ext-env-123",
		Type:               contract.TypeLocacao,
		StartDate:          time.Now(),
		Signatories: []contract.Signatory{
			{ID: "different-id-xyz", Name: "João Inquilino", Email: "joao@example.com", Role: contract.RoleLocatario},
		},
	}

	svc := service.NewContractService(&mockContractRepo{c: c}, &mockDealRepo{d: d}, &recordingBus{}, nil)
	if _, err := svc.MarkSignedExternally(context.Background(), "tenant-1", "contract-1", "ext-env-123", time.Now(), ""); err != nil {
		t.Fatalf("MarkSignedExternally: %v", err)
	}

	if d.Signers[0].Status != deal.SignerStatusSigned {
		t.Errorf("deal.Signers[0].Status = %q, want %q — email fallback should have matched", d.Signers[0].Status, deal.SignerStatusSigned)
	}
	if d.Parties[0].SignStatus != deal.SignerStatusSigned {
		t.Errorf("deal.Parties[0].SignStatus = %q, want %q — email fallback should have matched", d.Parties[0].SignStatus, deal.SignerStatusSigned)
	}
}

func TestMarkSignerSignedExternally_FallsBackToEmail(t *testing.T) {
	d, _ := deal.New(deal.TypeRent, "prop-1", "owner-1", "cust-1", "broker-1", 2500, "tenant-1")
	d.ID = "deal-1"
	_ = d.AddSigner("João Inquilino", "joao@example.com", deal.SignatoryRoleLocatario)

	c := &contract.Contract{
		ID:                 "contract-1",
		DealID:             d.ID,
		TenantID:           "tenant-1",
		Status:             contract.StatusPending,
		ExternalEnvelopeID: "ext-env-123",
		Type:               contract.TypeLocacao,
		StartDate:          time.Now(),
		Signatories: []contract.Signatory{
			{ID: "drift-id", Name: "João", Email: "joao@example.com", Role: contract.RoleLocatario},
		},
	}

	svc := service.NewContractService(&mockContractRepo{c: c}, &mockDealRepo{d: d}, &recordingBus{}, nil)
	if err := svc.MarkSignerSignedExternally(context.Background(), "tenant-1", "contract-1", "drift-id", time.Now(), "", "ext-env-123"); err != nil {
		t.Fatalf("MarkSignerSignedExternally: %v", err)
	}

	if d.Signers[0].Status != deal.SignerStatusSigned {
		t.Errorf("deal.Signers[0].Status = %q, want %q", d.Signers[0].Status, deal.SignerStatusSigned)
	}
	if d.Parties[0].SignStatus != deal.SignerStatusSigned {
		t.Errorf("deal.Parties[0].SignStatus = %q, want %q", d.Parties[0].SignStatus, deal.SignerStatusSigned)
	}
}

// Edge case: SignContract is the in-app fallback (used when the operator
// signs internally without Clicksign). It already flips contract.Status when
// all signed, but it does NOT mirror the deal's Signers/Parties — same bug
// surface, different path. This test pins the regression so a fix lands here
// too when we get to it.
func TestSignContract_SyncsDealSignersWhenComplete(t *testing.T) {
	d, _ := deal.New(deal.TypeRent, "prop-1", "owner-1", "cust-1", "broker-1", 2500, "tenant-1")
	d.ID = "deal-1"
	_ = d.AddSigner("João Inquilino", "joao@example.com", deal.SignatoryRoleLocatario)
	signerID := d.Signers[0].ID

	c := &contract.Contract{
		ID:                 "contract-1",
		DealID:             d.ID,
		TenantID:           "tenant-1",
		Status:             contract.StatusPending,
		ExternalEnvelopeID: "ext-env-123",
		Type:               contract.TypeLocacao,
		StartDate:          time.Now(),
		Signatories: []contract.Signatory{
			{ID: signerID, Name: "João Inquilino", Email: "joao@example.com", Role: contract.RoleLocatario},
		},
	}

	svc := service.NewContractService(&mockContractRepo{c: c}, &mockDealRepo{d: d}, &recordingBus{}, nil)
	// SECURITY (C12): quem assina é o próprio signatário — o teste representa o
	// chamador legítimo (mesmo id do signatário, mesmo e-mail).
	if _, err := svc.SignContract(context.Background(), "tenant-1", "contract-1", signerID, "signature-bytes",
		service.SignerIdentity{AccountID: signerID, Email: "joao@example.com"}); err != nil {
		t.Fatalf("SignContract: %v", err)
	}

	if c.Status != contract.StatusSigned {
		t.Errorf("contract.Status = %q, want %q", c.Status, contract.StatusSigned)
	}
	if d.Signers[0].Status != deal.SignerStatusSigned {
		t.Errorf("deal.Signers[0].Status = %q, want %q (SignContract should sync the deal)", d.Signers[0].Status, deal.SignerStatusSigned)
	}
	if d.Parties[0].SignStatus != deal.SignerStatusSigned {
		t.Errorf("deal.Parties[0].SignStatus = %q, want %q", d.Parties[0].SignStatus, deal.SignerStatusSigned)
	}
}

func TestSignContract_RetryAfterSignedDoesNotRepublishFinancialEvent(t *testing.T) {
	d, _ := deal.New(deal.TypeSale, "prop-1", "owner-1", "cust-1", "broker-1", 500000, "tenant-1")
	d.ID = "deal-1"
	_ = d.AddSigner("Maria Compradora", "maria@example.com", deal.SignatoryRoleComprador)
	signerID := d.Signers[0].ID

	c := &contract.Contract{
		ID:                 "contract-1",
		DealID:             d.ID,
		TenantID:           "tenant-1",
		Status:             contract.StatusPending,
		ExternalEnvelopeID: "ext-env-123",
		Type:               contract.TypeVenda,
		StartDate:          time.Now(),
		Signatories: []contract.Signatory{
			{ID: signerID, Name: "Maria Compradora", Email: "maria@example.com", Role: contract.RoleComprador},
		},
	}

	bus := &recordingBus{}
	svc := service.NewContractService(&mockContractRepo{c: c}, &mockDealRepo{d: d}, bus, nil)
	ctx := context.Background()
	// SECURITY (C12): identidade do chamador = identidade do signatário.
	signer := service.SignerIdentity{AccountID: signerID, Email: "maria@example.com"}

	if _, err := svc.SignContract(ctx, "tenant-1", c.ID, signerID, "signature-bytes", signer); err != nil {
		t.Fatalf("first SignContract: %v", err)
	}
	if _, err := svc.SignContract(ctx, "tenant-1", c.ID, signerID, "signature-bytes", signer); err != nil {
		t.Fatalf("retry SignContract: %v", err)
	}

	if got := len(bus.published); got != 1 {
		t.Fatalf("published events = %d, want 1 contract.signed after retry", got)
	}
	if got := bus.published[0].Name(); got != "deal.contract.signed" {
		t.Fatalf("published event = %q, want deal.contract.signed", got)
	}
}
