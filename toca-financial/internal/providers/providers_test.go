package providers

import (
	"context"
	"errors"
	"testing"
)

func TestSandboxNfseProvider_DeterministicReceipt(t *testing.T) {
	p := NewSandboxNfseProvider()
	in := NfseIssueInput{
		TenantID:           "tenant-1",
		ReferenceMonth:     "2026-07",
		RecipientName:      "recipient-1",
		AmountCents:        125050,
		ServiceDescription: "Intermediação imobiliária",
		IdempotencyKey:     "key-1",
	}

	first, err := p.IssueNfse(context.Background(), in)
	if err != nil {
		t.Fatalf("IssueNfse: %v", err)
	}
	second, err := p.IssueNfse(context.Background(), in)
	if err != nil {
		t.Fatalf("IssueNfse (replay): %v", err)
	}
	if first.AuthorityNumber != second.AuthorityNumber || first.AuthorityURL != second.AuthorityURL {
		t.Fatalf("mesma entrada deveria gerar o mesmo recibo: %+v vs %+v", first, second)
	}
	if first.AuthorityNumber == "" || first.AuthorityURL == "" {
		t.Fatal("recibo sem número/url de autoridade")
	}

	other := in
	other.AmountCents = 99999
	third, err := p.IssueNfse(context.Background(), other)
	if err != nil {
		t.Fatalf("IssueNfse (other): %v", err)
	}
	if third.AuthorityNumber == first.AuthorityNumber {
		t.Fatal("entradas diferentes deveriam gerar recibos diferentes")
	}
}

func TestSandboxNfseProvider_ConfiguredFailure(t *testing.T) {
	p := NewSandboxNfseProvider()
	p.FailWith = ErrNfseProviderRejected

	_, err := p.IssueNfse(context.Background(), NfseIssueInput{TenantID: "t"})
	if !errors.Is(err, ErrNfseProviderRejected) {
		t.Fatalf("err = %v, want ErrNfseProviderRejected", err)
	}
}

func TestSandboxPaymentProvider_DeterministicLink(t *testing.T) {
	p := NewSandboxPaymentProvider()
	in := PaymentLinkInput{TenantID: "tenant-1", ChargeID: "charge-1", AmountCents: 10000, CustomerEmail: "a@b.com"}

	first, err := p.CreatePaymentLink(context.Background(), in)
	if err != nil {
		t.Fatalf("CreatePaymentLink: %v", err)
	}
	second, err := p.CreatePaymentLink(context.Background(), in)
	if err != nil {
		t.Fatalf("CreatePaymentLink (replay): %v", err)
	}
	if first.URL != second.URL || first.ExternalID != second.ExternalID {
		t.Fatalf("mesma cobrança deveria gerar o mesmo link: %+v vs %+v", first, second)
	}
	if first.Provider != "sandbox" || first.ExpiresAt.IsZero() {
		t.Fatalf("link sandbox sem provider/expiração: %+v", first)
	}
}

func TestSandboxPaymentProvider_ConfiguredFailure(t *testing.T) {
	p := NewSandboxPaymentProvider()
	p.FailWith = ErrPaymentProviderUnavailable

	_, err := p.CreatePaymentLink(context.Background(), PaymentLinkInput{TenantID: "t"})
	if !errors.Is(err, ErrPaymentProviderUnavailable) {
		t.Fatalf("err = %v, want ErrPaymentProviderUnavailable", err)
	}
}
