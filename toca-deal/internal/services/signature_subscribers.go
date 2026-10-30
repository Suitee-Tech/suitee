package services

import (
	"context"
	"fmt"
	"log"
	"time"

	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/events"
)

// NewEnvelopeStartedHandler handles signature.envelope_started. Moves the
// contract from DRAFT to PENDING_SIGNATURE so the UI reflects "Aguardando
// assinatura" the moment Clicksign starts notifying signers (before any
// single signer.signed event fires).
func NewEnvelopeStartedHandler(svc *ContractService) events.EventHandler {
	return func(ctx context.Context, evt events.Event) error {
		payload, ok := evt.Payload().(map[string]interface{})
		if !ok {
			log.Printf("signature.envelope_started: unexpected payload type %T", evt.Payload())
			return nil
		}
		tenantID := stringField(payload, "tenant_id")
		contractID := stringField(payload, "contract_id")
		externalID := stringField(payload, "external_envelope_id")
		if tenantID == "" || contractID == "" {
			log.Printf("signature.envelope_started: missing required fields, payload=%v", payload)
			return nil
		}
		ctx = pkgctx.WithTenantID(ctx, tenantID)
		if err := svc.MarkEnvelopeStartedExternally(ctx, tenantID, contractID, externalID); err != nil {
			return fmt.Errorf("mark envelope started %s: %w", contractID, err)
		}
		return nil
	}
}

// NewSignerViewedHandler handles signature.signer_viewed. Updates the
// signatory's Status to VIEWED so admin/portal can show "Visualizado" when
// the signer opened the link but hasn't acted yet.
func NewSignerViewedHandler(svc *ContractService) events.EventHandler {
	return func(ctx context.Context, evt events.Event) error {
		payload, ok := evt.Payload().(map[string]interface{})
		if !ok {
			log.Printf("signature.signer_viewed: unexpected payload type %T", evt.Payload())
			return nil
		}
		tenantID := stringField(payload, "tenant_id")
		contractID := stringField(payload, "contract_id")
		signerID := stringField(payload, "signer_id")
		if tenantID == "" || contractID == "" || signerID == "" {
			log.Printf("signature.signer_viewed: missing required fields, payload=%v", payload)
			return nil
		}
		ctx = pkgctx.WithTenantID(ctx, tenantID)
		viewedAt := parseRFC3339(stringField(payload, "viewed_at"))
		if err := svc.MarkSignerViewedExternally(ctx, tenantID, contractID, signerID, viewedAt); err != nil {
			return fmt.Errorf("mark signer viewed %s/%s: %w", contractID, signerID, err)
		}
		return nil
	}
}

// NewSignerSignedHandler returns the EventBus handler for signature.signer_signed.
// The wire payload (see pkg/events/contracts/signature_events.go
// SignatureSignerSignedEventPayload) arrives as map[string]interface{} —
// json.Unmarshal of an arbitrary payload into interface{}.
func NewSignerSignedHandler(svc *ContractService) events.EventHandler {
	return func(ctx context.Context, evt events.Event) error {
		payload, ok := evt.Payload().(map[string]interface{})
		if !ok {
			log.Printf("signature.signer_signed: unexpected payload type %T", evt.Payload())
			return nil
		}
		tenantID := stringField(payload, "tenant_id")
		contractID := stringField(payload, "contract_id")
		signerID := stringField(payload, "signer_id")
		if tenantID == "" || contractID == "" || signerID == "" {
			log.Printf("signature.signer_signed: missing required fields, payload=%v", payload)
			return nil
		}
		ctx = pkgctx.WithTenantID(ctx, tenantID)
		signedAt := parseRFC3339(stringField(payload, "signed_at"))
		evidence := stringField(payload, "evidence_url")
		if err := svc.MarkSignerSignedExternally(ctx, tenantID, contractID, signerID, signedAt, evidence, stringField(payload, "external_envelope_id")); err != nil {
			return fmt.Errorf("mark signer signed for %s/%s: %w", contractID, signerID, err)
		}
		return nil
	}
}

// NewEnvelopeFinishedHandler handles signature.envelope_finished. Flips the
// contract to SIGNED and re-emits the existing ContractSignedEvent (which
// toca-financial consumes to create the Rental aggregate).
func NewEnvelopeFinishedHandler(svc *ContractService) events.EventHandler {
	return func(ctx context.Context, evt events.Event) error {
		payload, ok := evt.Payload().(map[string]interface{})
		if !ok {
			log.Printf("signature.envelope_finished: unexpected payload type %T", evt.Payload())
			return nil
		}
		tenantID := stringField(payload, "tenant_id")
		contractID := stringField(payload, "contract_id")
		externalID := stringField(payload, "external_envelope_id")
		if tenantID == "" || contractID == "" {
			log.Printf("signature.envelope_finished: missing required fields, payload=%v", payload)
			return nil
		}
		ctx = pkgctx.WithTenantID(ctx, tenantID)
		finishedAt := parseRFC3339(stringField(payload, "finished_at"))
		signedDoc := stringField(payload, "signed_document_url")
		if _, err := svc.MarkSignedExternally(ctx, tenantID, contractID, externalID, finishedAt, signedDoc); err != nil {
			return fmt.Errorf("mark contract %s signed externally: %w", contractID, err)
		}
		return nil
	}
}

// NewSignatureRefusedHandler handles signature.refused. The contract goes
// back to DRAFT with the lock cleared, so the operator can fix and resend.
func NewSignatureRefusedHandler(svc *ContractService) events.EventHandler {
	return func(ctx context.Context, evt events.Event) error {
		payload, ok := evt.Payload().(map[string]interface{})
		if !ok {
			log.Printf("signature.refused: unexpected payload type %T", evt.Payload())
			return nil
		}
		tenantID := stringField(payload, "tenant_id")
		contractID := stringField(payload, "contract_id")
		if tenantID == "" || contractID == "" {
			log.Printf("signature.refused: missing required fields, payload=%v", payload)
			return nil
		}
		ctx = pkgctx.WithTenantID(ctx, tenantID)
		signerID := stringField(payload, "signer_id")
		reason := stringField(payload, "reason")
		if err := svc.HandleSignatureRefused(ctx, tenantID, contractID, stringField(payload, "external_envelope_id"), signerID, reason); err != nil {
			return fmt.Errorf("handle refusal for %s: %w", contractID, err)
		}
		return nil
	}
}

// NewSignatureExpiredHandler handles signature.expired (envelope deadline
// reached on Clicksign). Same recovery as a refusal — contract returns to
// DRAFT with the lock cleared — but distinct as an event so dashboards can
// distinguish "rejected by signer" from "ran out of time".
func NewSignatureExpiredHandler(svc *ContractService) events.EventHandler {
	return func(ctx context.Context, evt events.Event) error {
		payload, ok := evt.Payload().(map[string]interface{})
		if !ok {
			log.Printf("signature.expired: unexpected payload type %T", evt.Payload())
			return nil
		}
		tenantID := stringField(payload, "tenant_id")
		contractID := stringField(payload, "contract_id")
		if tenantID == "" || contractID == "" {
			log.Printf("signature.expired: missing required fields, payload=%v", payload)
			return nil
		}
		ctx = pkgctx.WithTenantID(ctx, tenantID)
		if err := svc.HandleSignatureRefused(ctx, tenantID, contractID, stringField(payload, "external_envelope_id"), "", "envelope expired (deadline)"); err != nil {
			return fmt.Errorf("handle expiration for %s: %w", contractID, err)
		}
		return nil
	}
}

// AmendmentLookup is the narrow surface NewSignedPDFPersistHandler needs
// from ContractService to know whether a freshly-signed contract is an
// amendment. The signed Document is reclassified from CategoryContrato to
// CategoryAditivo so the Central de Documentos groups it correctly.
type AmendmentLookup interface {
	IsAmendment(ctx context.Context, tenantID, contractID string) (bool, error)
}

// NewSignedPDFPersistHandler handles signature.envelope_finished on a second
// queue dedicated to persisting the signed PDF in MinIO. Separated from
// NewEnvelopeFinishedHandler so the slower HTTP-download + storage upload
// has its own retry budget — a transient storage outage shouldn't keep
// retrying the contract status flip too.
//
// amendmentLookup is optional. When wired, this handler reclassifies the
// signed Document from "contrato" to "aditivo" if the contract carries an
// AmendsContractID. Nil lookup keeps the legacy "always contrato" path.
func NewSignedPDFPersistHandler(docs *DocumentService, amendmentLookup AmendmentLookup) events.EventHandler {
	return func(ctx context.Context, evt events.Event) error {
		payload, ok := evt.Payload().(map[string]interface{})
		if !ok {
			log.Printf("signature.envelope_finished[persist]: unexpected payload type %T", evt.Payload())
			return nil
		}
		tenantID := stringField(payload, "tenant_id")
		contractID := stringField(payload, "contract_id")
		signedDoc := stringField(payload, "signed_document_url")
		if tenantID == "" || contractID == "" {
			log.Printf("signature.envelope_finished[persist]: missing required fields, payload=%v", payload)
			return nil
		}
		if signedDoc == "" {
			// Clicksign sometimes emits envelope_finished without the
			// signed file URL ready (race with their download CDN). No-op
			// here; a follow-up event or manual re-sync will fill it.
			return nil
		}
		ctx = pkgctx.WithTenantID(ctx, tenantID)
		if err := docs.PersistSignedContractPDF(ctx, tenantID, contractID, signedDoc); err != nil {
			return fmt.Errorf("persist signed PDF for contract %s: %w", contractID, err)
		}
		if amendmentLookup != nil {
			isAmendment, lookupErr := amendmentLookup.IsAmendment(ctx, tenantID, contractID)
			if lookupErr != nil {
				log.Printf("signature.envelope_finished[persist]: amendment lookup failed for %s: %v", contractID, lookupErr)
			} else if isAmendment {
				if err := docs.ReclassifyContractAsAmendment(ctx, tenantID, contractID); err != nil {
					log.Printf("signature.envelope_finished[persist]: reclassify aditivo failed for %s: %v", contractID, err)
				}
			}
		}
		return nil
	}
}

func stringField(m map[string]interface{}, key string) string {
	v, ok := m[key].(string)
	if !ok {
		return ""
	}
	return v
}

func parseRFC3339(s string) time.Time {
	if s == "" {
		return time.Now()
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Now()
	}
	return t
}
