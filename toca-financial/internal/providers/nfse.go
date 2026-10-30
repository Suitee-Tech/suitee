// Package providers reúne os CONTRATOS de integração externa do financeiro
// (P1-05): NFS-e e pagamentos. Cada provider segue o mesmo padrão dos demais
// provedores da frota (Clicksign, Zenvia, Resend, DeepSeek): contrato
// determinístico, timeout no chamador, erros tipados e implementação de
// sandbox in-process para testes e ambientes sem provedor real.
//
// Escolher um provedor real (ex.: PlugNotas/eNotas para NFS-e; Asaas/Stark
// para pagamentos) é decisão de produto; quando houver, basta implementar a
// interface com um cliente HTTP (pkg/clients.BaseClient dá timeout + circuit
// breaker) e trocar o wiring no router/main.
package providers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ── NFS-e ──────────────────────────────────────────────────────────────────

var (
	// ErrNfseProviderRejected marca rejeição do provedor (dados/regime
	// municipal recusados). A NFS-e fica em ERROR e o operador corrige.
	ErrNfseProviderRejected = errors.New("nfse provider rejected the issue request")
	// ErrNfseProviderUnavailable marca falha de infraestrutura do provedor
	// (rede/timeout/5xx). A NFS-e fica em ERROR e a emissão é retryable.
	ErrNfseProviderUnavailable = errors.New("nfse provider unavailable")
)

// NfseIssueInput é o contrato de entrada da emissão de NFS-e.
type NfseIssueInput struct {
	TenantID           string
	ReferenceMonth     string
	RecipientName      string
	RecipientDocument  string
	AmountCents        int64
	ServiceDescription string
	// IdempotencyKey é derivado pelo chamador de forma determinística; o
	// provedor pode usá-lo para não emitir duas notas na reentrega.
	IdempotencyKey string
}

// NfseReceipt é o recibo de autoridade devolvido pelo provedor.
type NfseReceipt struct {
	AuthorityNumber string
	AuthorityURL    string
	XMLURL          string
	IssuedAt        time.Time
}

// NfseProvider emite NFS-e em provedor externo.
type NfseProvider interface {
	IssueNfse(ctx context.Context, in NfseIssueInput) (*NfseReceipt, error)
}

// SandboxNfseProvider é a implementação in-process determinística: a mesma
// entrada gera SEMPRE o mesmo recibo (número de autorização derivado de um
// hash estável), o que torna reentregas idempotentes e testes determinísticos.
// O recibo é explicitamente marcado como sandbox — nunca é confundido com
// nota fiscal real.
type SandboxNfseProvider struct {
	// FailWith configura falha para testes: nil = nunca falha;
	// ErrNfseProviderRejected/Unavailable = sempre falha com o erro dado.
	FailWith error
}

func NewSandboxNfseProvider() *SandboxNfseProvider { return &SandboxNfseProvider{} }

func (p *SandboxNfseProvider) IssueNfse(ctx context.Context, in NfseIssueInput) (*NfseReceipt, error) {
	if p.FailWith != nil {
		return nil, p.FailWith
	}
	if ctx.Err() != nil {
		return nil, fmt.Errorf("%w: %v", ErrNfseProviderUnavailable, ctx.Err())
	}

	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%s|%d|%s",
		in.TenantID, in.ReferenceMonth, in.RecipientDocument, in.IdempotencyKey, in.AmountCents, in.ServiceDescription)))
	digest := hex.EncodeToString(sum[:])
	number := "SBX-" + strings.ToUpper(digest[:12])
	return &NfseReceipt{
		AuthorityNumber: number,
		AuthorityURL:    "https://sandbox.nfse.suitee.com.br/danfe/" + digest,
		XMLURL:          "https://sandbox.nfse.suitee.com.br/xml/" + digest,
		IssuedAt:        time.Now().UTC(),
	}, nil
}
