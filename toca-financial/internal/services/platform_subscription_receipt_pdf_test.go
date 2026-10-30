package services

import (
	"bytes"
	"testing"
	"time"

	ps "github.com/oginoino/toca/toca-financial/internal/domain/platform_subscription"
	"github.com/stretchr/testify/require"
)

func TestPlatformSubscriptionReceiptFitsOnePageAndFormatsTheDocument(t *testing.T) {
	pdf, err := RenderPlatformSubscriptionReceiptPDF(PlatformSubscriptionReceiptContext{
		Payment: ps.Payment{ID: "card_1", SubscriptionID: "subs_1", AmountCents: 49700,
			PaidAt: time.Date(2026, 10, 3, 22, 46, 0, 0, time.UTC)},
		Plan:          &ps.Plan{Tier: "starter", Name: "Starter", Cycle: "MONTHLY", AmountCents: 49700},
		PaymentMethod: &ps.PaymentMethod{Kind: "CARD", Brand: "VISA", Last4: "4242"},
		TenantName:    "Exemplo Imóveis", TenantDocument: "74684712000103", Sandbox: true,
	})
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(pdf, []byte("%PDF")))
	// A short receipt belongs to a single page: the footer used to overflow.
	// The text stream is compressed, so the page tree is what can be asserted.
	require.Contains(t, string(pdf), "/Count 1")
}

func TestPlatformSubscriptionReceiptFormatsTaxIDs(t *testing.T) {
	require.Equal(t, "74.684.712/0001-03", formatTaxID("74684712000103"))
	require.Equal(t, "529.982.247-25", formatTaxID("52998224725"))
	require.Equal(t, "74.684.712/0001-03", formatTaxID("74.684.712/0001-03"))
	// Anything unexpected is printed as received.
	require.Equal(t, "sem documento", formatTaxID("sem documento"))
}

func TestPlatformSubscriptionReceiptRefusesAnEmptyPayment(t *testing.T) {
	_, err := RenderPlatformSubscriptionReceiptPDF(PlatformSubscriptionReceiptContext{})
	require.Error(t, err)
}
