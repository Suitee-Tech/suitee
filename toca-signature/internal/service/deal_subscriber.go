package service

import (
	"context"
	"errors"
	"fmt"
	"log"

	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-signature/internal/domain/envelope"
)

// NewDealSubscriber returns an EventHandler for the `signature.requested`
// topic. The published payload (see pkg/events/contracts/signature_events.go)
// arrives here as a map[string]interface{} (json.Unmarshal into interface{}),
// so we type-assert defensively rather than re-marshal/unmarshal.
//
// On unrecoverable parse errors we return nil + log: the message is acked so
// it doesn't loop forever in the DLQ. On EnvelopeService errors we return the
// error so RabbitMQ retries / sends to DLQ.
func NewDealSubscriber(svc *EnvelopeService) events.EventHandler {
	return func(ctx context.Context, evt events.Event) error {
		payload, ok := evt.Payload().(map[string]interface{})
		if !ok {
			log.Printf("signature.requested: unexpected payload type %T, skipping", evt.Payload())
			return nil
		}

		tenantID := stringField(payload, "tenant_id")
		contractID := stringField(payload, "contract_id")
		if tenantID == "" || contractID == "" {
			log.Printf("signature.requested: missing tenant_id or contract_id, payload=%v", payload)
			return nil
		}

		// The bus already injects tenant_id from AMQP headers, but be
		// explicit so a malformed header doesn't silently drop us into
		// an empty tenant.
		ctx = pkgctx.WithTenantID(ctx, tenantID)

		// Distrato (#436): documento e signatários vêm do próprio evento;
		// roteia para CreateForDistrato em vez de buscar o contrato.
		if stringField(payload, "kind") == string(envelope.EnvelopeKindDistrato) {
			signers := signersFromPayload(payload)
			if len(signers) == 0 {
				log.Printf("signature.requested[distrato]: no signers for contract %s, skipping", contractID)
				return nil
			}
			if _, err := svc.CreateForDistrato(ctx, tenantID, contractID,
				stringField(payload, "deal_id"), stringField(payload, "document_url"), signers); err != nil {
				return fmt.Errorf("create distrato envelope for contract %s: %w", contractID, err)
			}
			return nil
		}

		opts := CreateOptions{
			Locale: stringField(payload, "locale"),
		}
		opts.SignerAuth = parseSignerAuth(payload)

		if _, err := svc.CreateForContract(ctx, tenantID, contractID, opts); err != nil {
			if errors.Is(err, ErrContractNotReadyForSignature) {
				// A regenerated draft requires a new explicit send. A delayed
				// request for the previous draft must not send it automatically.
				return nil
			}
			return fmt.Errorf("create envelope for contract %s: %w", contractID, err)
		}
		return nil
	}
}

// signersFromPayload constrói os envelope.Signer completos a partir do
// array `signers` do payload do evento — usado no fluxo de distrato, em
// que os signatários vêm do evento e não do contrato. Espelha
// signersFromContract, mas lendo o map cru entregue pelo RabbitMQ.
func signersFromPayload(payload map[string]interface{}) []envelope.Signer {
	raw, ok := payload["signers"].([]interface{})
	if !ok {
		return nil
	}
	out := make([]envelope.Signer, 0, len(raw))
	for _, r := range raw {
		s, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		role := envelope.SignerRoleSign
		if rv := stringField(s, "role"); rv == "witness" || rv == "testemunha" {
			role = envelope.SignerRoleWitness
		}
		var auth []envelope.AuthMethod
		if av, ok := s["auth"].([]interface{}); ok {
			for _, a := range av {
				if str, ok := a.(string); ok && str != "" {
					auth = append(auth, envelope.AuthMethod(str))
				}
			}
		}
		if len(auth) == 0 {
			auth = []envelope.AuthMethod{envelope.AuthEmailToken}
		}
		out = append(out, envelope.Signer{
			SignerID:      stringField(s, "signer_id"),
			Name:          stringField(s, "name"),
			Email:         stringField(s, "email"),
			PhoneNumber:   stringField(s, "phone_number"),
			Documentation: stringField(s, "documentation"),
			Birthday:      stringField(s, "birthday"),
			Role:          role,
			Auth:          auth,
			Status:        envelope.SignerStatusPending,
		})
	}
	return out
}

// parseSignerAuth pulls signer→[auth] map out of the raw payload. The publisher
// sends a `signers` array of objects, each with `signer_id` and `auth` (slice
// of strings). Unknown auth strings are passed through (clients/types map
// to the Clicksign field as-is).
func parseSignerAuth(payload map[string]interface{}) map[string][]envelope.AuthMethod {
	signers, ok := payload["signers"].([]interface{})
	if !ok {
		return nil
	}
	out := make(map[string][]envelope.AuthMethod, len(signers))
	for _, raw := range signers {
		s, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		id := stringField(s, "signer_id")
		if id == "" {
			continue
		}
		authVals, ok := s["auth"].([]interface{})
		if !ok {
			continue
		}
		methods := make([]envelope.AuthMethod, 0, len(authVals))
		for _, av := range authVals {
			str, ok := av.(string)
			if !ok || str == "" {
				continue
			}
			methods = append(methods, envelope.AuthMethod(str))
		}
		out[id] = methods
	}
	return out
}

func stringField(m map[string]interface{}, key string) string {
	v, ok := m[key].(string)
	if !ok {
		return ""
	}
	return v
}
