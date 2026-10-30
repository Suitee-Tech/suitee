package providers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// ── Pagamentos ─────────────────────────────────────────────────────────────

var (
	// ErrPaymentProviderRejected marca rejeição do provedor (cobrança fora de
	// política, dados inválidos).
	ErrPaymentProviderRejected = errors.New("payment provider rejected the link request")
	// ErrPaymentProviderUnavailable marca falha de infraestrutura do provedor.
	ErrPaymentProviderUnavailable = errors.New("payment provider unavailable")
)

// PaymentLinkInput é o contrato de entrada da criação de link de pagamento.
type PaymentLinkInput struct {
	TenantID      string
	ChargeID      string
	AmountCents   int64
	DueDate       time.Time
	CustomerName  string
	CustomerEmail string
}

// PaymentLink é o link de cobrança devolvido pelo provedor.
type PaymentLink struct {
	URL        string
	ExternalID string
	Provider   string
	ExpiresAt  time.Time
}

// PaymentProvider cria links de cobrança (Pix/boleto) em provedor externo.
//
// Contrato para a integração futura de gateway. Hoje a baixa de cobranças é
// manual (MarkPaidIdempotent); quando o gateway for escolhido, a
// implementação real persiste o link na cobrança e consome o webhook de
// confirmação — este contrato é o ponto de costura.
type PaymentProvider interface {
	CreatePaymentLink(ctx context.Context, in PaymentLinkInput) (*PaymentLink, error)
}

// SandboxPaymentProvider gera links determinísticos e explicitamente
// marcados como sandbox: a mesma cobrança gera SEMPRE o mesmo link, o que
// torna testes e reentregas estáveis. Nunca é confundido com cobrança real.
type SandboxPaymentProvider struct {
	// FailWith configura falha para testes (mesma semântica do NFS-e).
	FailWith error
}

func NewSandboxPaymentProvider() *SandboxPaymentProvider { return &SandboxPaymentProvider{} }

func (p *SandboxPaymentProvider) CreatePaymentLink(ctx context.Context, in PaymentLinkInput) (*PaymentLink, error) {
	if p.FailWith != nil {
		return nil, p.FailWith
	}
	if ctx.Err() != nil {
		return nil, fmt.Errorf("%w: %v", ErrPaymentProviderUnavailable, ctx.Err())
	}

	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d|%s", in.TenantID, in.ChargeID, in.AmountCents, in.CustomerEmail)))
	digest := hex.EncodeToString(sum[:])
	externalID := "SBX-PAY-" + digest[:16]
	return &PaymentLink{
		URL:        fmt.Sprintf("https://pay.sandbox.suitee.com.br/%s/%s?amount=%d", in.TenantID, in.ChargeID, in.AmountCents),
		ExternalID: externalID,
		Provider:   "sandbox",
		ExpiresAt:  time.Now().UTC().Add(24 * time.Hour),
	}, nil
}
