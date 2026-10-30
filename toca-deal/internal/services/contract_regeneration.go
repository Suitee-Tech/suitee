package services

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
)

var ErrContractCannotRegenerate = errors.New("Não é possível regerar: existe assinatura ou envelope externo. Reconcilie a assinatura antes de continuar.")
var ErrInvalidRegeneration = errors.New("Informe a nova minuta, os signatários e a versão atual do contrato.")

type RegenerateContractRequest struct {
	Content           string               `json:"content" binding:"required"`
	Signatories       []contract.Signatory `json:"signatories" binding:"required,min=1"`
	ExpectedUpdatedAt time.Time            `json:"expected_updated_at" binding:"required"`
}

// RegenerateContract replaces an unsigned draft in place. It never creates or
// cancels an external envelope, changes the financial plan, or sends invitations.
func (s *ContractService) RegenerateContract(ctx context.Context, tenantID, id string, req RegenerateContractRequest) (*contract.Contract, error) {
	c, err := s.contractRepo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, errors.New("contract not found")
	}
	if strings.TrimSpace(req.Content) == "" || len(req.Signatories) == 0 || req.ExpectedUpdatedAt.IsZero() {
		return nil, ErrInvalidRegeneration
	}
	if !req.ExpectedUpdatedAt.Equal(c.UpdatedAt) {
		return nil, contract.ErrConcurrentModification
	}
	if (c.Status != contract.StatusDraft && c.Status != contract.StatusPending) || c.SignedDocumentURL != "" {
		return nil, ErrContractCannotRegenerate
	}
	for _, sig := range c.Signatories {
		if sig.SignedAt != nil || sig.Signature != "" || sig.Status == contract.SignatoryStatusSigned {
			return nil, ErrContractCannotRegenerate
		}
	}
	if s.signature == nil {
		return nil, errors.New("Serviço de assinatura indisponível para verificar o contrato.")
	}
	env, err := s.signature.GetByContract(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if env == nil {
		if c.Status != contract.StatusDraft || c.SignatureLockedAt != nil || c.SentForSignatureAt != nil || c.ExternalEnvelopeID != "" {
			return nil, ErrContractCannotRegenerate
		}
	} else {
		// ERROR sem external é falha antes de criar o envelope; CANCELED (e os
		// demais terminais sem assinatura) é uma tentativa encerrada que pode
		// ser corrigida. Em ambos os casos, o novo envio cria outro envelope.
		terminalWithoutSignature := env.Status == "CANCELED" || env.Status == "EXPIRED" || env.Status == "REFUSED"
		failedBeforeEnvelope := env.Status == "ERROR" && env.ExternalEnvelopeID == ""
		if !terminalWithoutSignature && !failedBeforeEnvelope {
			return nil, ErrContractCannotRegenerate
		}
		if env.FinishedAt != "" {
			return nil, ErrContractCannotRegenerate
		}
		for _, sig := range env.Signers {
			if sig.Status == "SIGNED" || sig.SignedAt != "" {
				return nil, ErrContractCannotRegenerate
			}
		}
	}
	// Mesmo que o cliente envie o snapshot antigo, o deal é a fonte de
	// verdade da identidade (nome/e-mail) corrigida na esteira.
	if err := s.syncSignatoriesFromDeal(ctx, tenantID, c.DealID, req.Signatories); err != nil {
		return nil, err
	}
	if err := contract.ValidateSignatories(c.Type, req.Signatories); err != nil {
		return nil, err
	}
	previous := c
	copyContract := *c
	c = &copyContract
	signatories := make([]contract.Signatory, len(req.Signatories))
	seen := make(map[string]bool)
	for i, sig := range req.Signatories {
		if sig.ID == "" || seen[sig.ID] || strings.TrimSpace(sig.Name) == "" || strings.TrimSpace(sig.Email) == "" {
			return nil, ErrInvalidRegeneration
		}
		seen[sig.ID] = true
		// Recover identification/auth fields collected in the previous send.
		// Never copy evidence or accept signature state from the browser.
		for _, old := range previous.Signatories {
			if old.ID == sig.ID && strings.EqualFold(old.Email, sig.Email) {
				if sig.Documentation == "" {
					sig.Documentation = old.Documentation
				}
				if sig.Birthday == "" {
					sig.Birthday = old.Birthday
				}
				if sig.PhoneNumber == "" {
					sig.PhoneNumber = old.PhoneNumber
				}
				if len(sig.AuthMethods) == 0 {
					sig.AuthMethods = old.AuthMethods
				}
				sig.Group = old.Group
			}
		}
		sig.Status = contract.SignatoryStatusPending
		sig.SignedAt, sig.ViewedAt = nil, nil
		sig.Signature, sig.EvidenceURL, sig.ClicksignSignerKey = "", "", ""
		signatories[i] = sig
	}
	c.DraftRevisions = append(append([]contract.DraftRevision(nil), previous.DraftRevisions...), contract.DraftRevision{
		Content: previous.Content, Signatories: previous.Signatories, ReplacedAt: time.Now(), ReplacedBy: actorFromContext(ctx),
	})
	c.Content, c.Signatories = req.Content, signatories
	c.Status = contract.StatusDraft
	c.SignatureLockedAt, c.SentForSignatureAt = nil, nil
	c.SignatureProvider, c.ExternalEnvelopeID = "", ""
	if err := s.contractRepo.UpdateIfUnchanged(ctx, c, previous.UpdatedAt); err != nil {
		return nil, err
	}
	s.syncDocuments(ctx, c)
	if s.documents != nil {
		if bytes, err := decodeContractContent(c.Content); err == nil && len(bytes) > 0 {
			// Projection failures must not turn a saved regeneration into a
			// failed operation and encourage the user to regenerate twice.
			_ = s.documents.UploadDocxForContract(ctx, tenantID, c.ID, bytes, actorFromContext(ctx))
		}
	}
	return c, nil
}

// syncSignatoriesFromDeal atualiza nome/e-mail dos signatários com os dados
// atuais do deal. A minuta é um snapshot; uma correção feita no lead/parte não
// pode ficar presa em um contrato/cancelamento antigo.
func (s *ContractService) syncSignatoriesFromDeal(ctx context.Context, tenantID, dealID string, signatories []contract.Signatory) error {
	if s.dealRepo == nil || dealID == "" || len(signatories) == 0 {
		return nil
	}
	d, err := s.dealRepo.GetByID(ctx, tenantID, dealID)
	if err != nil {
		return err
	}
	if d == nil {
		return nil
	}
	for i := range signatories {
		for _, ds := range d.Signers {
			if ds.ID != signatories[i].ID {
				continue
			}
			if name := strings.TrimSpace(ds.Name); name != "" {
				signatories[i].Name = name
			}
			if email := strings.TrimSpace(ds.Email); email != "" {
				signatories[i].Email = email
			}
			break
		}
	}
	return nil
}
