package services_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	service "github.com/oginoino/toca/toca-deal/internal/services"
)

type regenerationSignature struct {
	fakeSignatureClient
	env       *clients.SignatureEnvelopeDTO
	lookupErr error
}

func (s *regenerationSignature) GetByContract(context.Context, string, string) (*clients.SignatureEnvelopeDTO, error) {
	return s.env, s.lookupErr
}

func TestRegenerateContract_RecoversFailedSendWithoutDuplicatingContract(t *testing.T) {
	c := sampleSignedContract()
	c.UpdatedAt = time.Now()
	c.Content = "original-docx"
	c.Status = contract.StatusPending
	c.SignatureLockedAt = &c.UpdatedAt
	c.SentForSignatureAt = &c.UpdatedAt
	c.Signatories[0].Documentation = "12345678909"
	c.Signatories[0].PhoneNumber = "11999999999"
	c.Signatories[0].AuthMethods = []string{"email"}
	c.Signatories[0].Group = 2
	repo := &mockContractRepo{c: c}
	sig := &regenerationSignature{env: &clients.SignatureEnvelopeDTO{ID: "failed-attempt", Status: "ERROR"}}
	bus := &recordingBus{}
	svc := service.NewContractService(repo, &mockDealRepo{}, bus, nil, sig)
	req := service.RegenerateContractRequest{
		Content: "revised-docx", ExpectedUpdatedAt: c.UpdatedAt,
		Signatories: []contract.Signatory{{ID: "sig-1", Name: "Nome Revisado", Email: "inq@x.com", Role: contract.RoleInquilino}},
	}
	out, err := svc.RegenerateContract(context.Background(), c.TenantID, c.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	if out.ID != c.ID || out.DealID != c.DealID || out.Status != contract.StatusDraft || out.Content != req.Content {
		t.Fatalf("regeneration must reuse the contract and return the new draft: %+v", out)
	}
	if out.SignatureLockedAt != nil || out.SentForSignatureAt != nil {
		t.Fatal("draft is still locked")
	}
	if !out.StartDate.Equal(c.StartDate) || out.Value != c.Value {
		t.Fatal("financial terms changed")
	}
	if len(out.DraftRevisions) != 1 || out.DraftRevisions[0].Content != c.Content || out.DraftRevisions[0].Signatories[0].Name != "Inquilino" {
		t.Fatal("original document/signatories were not preserved")
	}
	if out.Signatories[0].Documentation != "12345678909" || out.Signatories[0].Group != 2 || out.Signatories[0].PhoneNumber == "" {
		t.Fatal("previously collected identification/order was lost")
	}
	if sig.calls != 0 || len(bus.published) != 0 {
		t.Fatal("regeneration must not send the contract")
	}
	if sig.env.Status != "ERROR" {
		t.Fatal("failed attempt history changed")
	}
	// Explicit send is available after regeneration. O envelope é criado pelo
	// worker: a requisição apenas publica o evento, sem chamar o provedor (a
	// renderização do PDF leva ~27s e não cabe no tempo de um request).
	sent, err := svc.SendForSignature(context.Background(), c.TenantID, c.ID, service.SendForSignatureRequest{})
	if err != nil || sent.Status != contract.StatusPending {
		t.Fatalf("explicit retry failed: %v", err)
	}
	if sig.calls != 0 {
		t.Fatalf("o envio não deve chamar o provedor na requisição, calls = %d", sig.calls)
	}
	if len(bus.published) != 1 {
		t.Fatalf("o envio deve publicar o evento para o worker, published = %d", len(bus.published))
	}
}

func TestRegenerateContract_RecoversCanceledEnvelopeBeforeSignature(t *testing.T) {
	c := sampleSignedContract()
	c.UpdatedAt = time.Now()
	c.Content = "original-docx"
	c.Status = contract.StatusPending
	c.ExternalEnvelopeID = "cs-canceled"
	c.SignatureLockedAt = &c.UpdatedAt
	c.SentForSignatureAt = &c.UpdatedAt
	repo := &mockContractRepo{c: c}
	sig := &regenerationSignature{env: &clients.SignatureEnvelopeDTO{
		ID: "canceled-attempt", Status: "CANCELED", ExternalEnvelopeID: "cs-canceled",
	}}
	d, _ := deal.New(deal.TypeRent, "prop-1", "owner-1", "cust-1", "broker-1", 2500, c.TenantID)
	d.Signers = []deal.Signer{{ID: "sig-1", Name: "Nome Corrigido", Email: "corrigido@example.com", Role: deal.SignatoryRole(contract.RoleInquilino)}}
	svc := service.NewContractService(repo, &mockDealRepo{d: d}, nil, nil, sig)
	req := service.RegenerateContractRequest{
		Content: "corrected-docx", ExpectedUpdatedAt: c.UpdatedAt,
		Signatories: []contract.Signatory{{ID: "sig-1", Name: "Nome Revisado", Email: "inq@x.com", Role: contract.RoleInquilino}},
	}
	out, err := svc.RegenerateContract(context.Background(), c.TenantID, c.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	if out.ID != c.ID || out.Status != contract.StatusDraft || out.ExternalEnvelopeID != "" {
		t.Fatalf("contrato não voltou a rascunho: %+v", out)
	}
	if out.SignatureLockedAt != nil || out.SentForSignatureAt != nil {
		t.Fatal("lock de assinatura deveria ser limpo")
	}
	if out.Signatories[0].Email != "corrigido@example.com" || out.Signatories[0].Name != "Nome Corrigido" {
		t.Fatalf("regeneração ignorou a identidade corrigida no deal: %+v", out.Signatories[0])
	}
	if len(out.DraftRevisions) != 1 || out.DraftRevisions[0].Content != "original-docx" {
		t.Fatalf("versão anterior não preservada: %+v", out.DraftRevisions)
	}
	if sig.calls != 0 {
		t.Fatal("regeneração não pode enviar assinatura")
	}
}

func TestRegenerateContract_BlocksUnsafeOrUnknownState(t *testing.T) {
	tests := []struct {
		name      string
		status    contract.Status
		env       *clients.SignatureEnvelopeDTO
		lookupErr error
		signed    bool
	}{
		{name: "signed", status: contract.StatusSigned},
		{name: "partially signed", status: contract.StatusPartiallySigned},
		{name: "signature evidence", status: contract.StatusDraft, signed: true},
		{name: "running", status: contract.StatusPending, env: &clients.SignatureEnvelopeDTO{Status: "RUNNING", ExternalEnvelopeID: "external"}},
		{name: "failed after creation", status: contract.StatusDraft, env: &clients.SignatureEnvelopeDTO{Status: "ERROR", ExternalEnvelopeID: "external"}},
		{name: "pending without envelope", status: contract.StatusPending},
		{name: "lookup failed", status: contract.StatusDraft, lookupErr: errors.New("unavailable")},
		{name: "envelope signed", status: contract.StatusDraft, env: &clients.SignatureEnvelopeDTO{Status: "ERROR", Signers: []clients.SignatureEnvelopeSignerDTO{{Status: "SIGNED"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := sampleSignedContract()
			c.UpdatedAt = time.Now()
			c.Status = tt.status
			c.Content = "original"
			if tt.signed {
				c.Signatories[0].SignedAt = &c.UpdatedAt
			}
			repo := &mockContractRepo{c: c}
			sig := &regenerationSignature{env: tt.env, lookupErr: tt.lookupErr}
			svc := service.NewContractService(repo, &mockDealRepo{}, nil, nil, sig)
			_, err := svc.RegenerateContract(context.Background(), c.TenantID, c.ID, service.RegenerateContractRequest{Content: "new", Signatories: c.Signatories, ExpectedUpdatedAt: c.UpdatedAt})
			if err == nil {
				t.Fatal("unsafe regeneration allowed")
			}
			if repo.c.Content != "original" || len(repo.c.DraftRevisions) != 0 || sig.calls != 0 {
				t.Fatal("failed regeneration changed state")
			}
		})
	}
}

type conflictingContractRepo struct{ *mockContractRepo }

func (r *conflictingContractRepo) UpdateIfUnchanged(context.Context, *contract.Contract, time.Time) error {
	return contract.ErrConcurrentModification
}

func TestRegenerateContract_RejectsConcurrentChanges(t *testing.T) {
	for _, duringSave := range []bool{false, true} {
		t.Run(map[bool]string{false: "stale preview", true: "send during save"}[duringSave], func(t *testing.T) {
			c := sampleSignedContract()
			c.UpdatedAt = time.Now()
			c.Content = "original"
			base := &mockContractRepo{c: c}
			var repo contract.Repository = base
			expected := c.UpdatedAt.Add(-time.Second)
			if duringSave {
				repo = &conflictingContractRepo{base}
				expected = c.UpdatedAt
			}
			svc := service.NewContractService(repo, &mockDealRepo{}, nil, nil, &regenerationSignature{})
			_, err := svc.RegenerateContract(context.Background(), c.TenantID, c.ID, service.RegenerateContractRequest{Content: "new", Signatories: c.Signatories, ExpectedUpdatedAt: expected})
			if !errors.Is(err, contract.ErrConcurrentModification) {
				t.Fatalf("expected conflict, got %v", err)
			}
			if base.c.Content != "original" || len(base.c.DraftRevisions) != 0 {
				t.Fatal("conflict overwrote original")
			}
		})
	}
}
