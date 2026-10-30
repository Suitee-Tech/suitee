package services

import (
	"context"
	"errors"
	"log"

	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// signatureAttemptMatches checks the provider envelope, not just the contract
// ID. When events arrive before envelope_started, read the latest envelope
// and its creation date so a previous attempt cannot unlock a new send.
func (s *ContractService) signatureAttemptMatches(ctx context.Context, c *contract.Contract, externalID string) (bool, error) {
	if externalID == "" {
		log.Printf("ignored uncorrelated signature event for contract %s", c.ID)
		return false, nil
	}
	if c.ExternalEnvelopeID != "" {
		return c.ExternalEnvelopeID == externalID, nil
	}
	if s.signature == nil {
		return false, errors.New("signature attempt is not synchronized")
	}
	env, err := s.signature.GetByContract(ctx, c.TenantID, c.ID)
	if err != nil {
		return false, err // Retry rather than discard an event during an outage.
	}
	if env == nil {
		return false, errors.New("signature envelope is not synchronized")
	}
	if env.ContractID != c.ID || env.ExternalEnvelopeID != externalID {
		return false, nil
	}
	if c.SentForSignatureAt != nil {
		if env.CreatedAt.IsZero() {
			return false, errors.New("signature envelope creation date is unavailable")
		}
		if env.CreatedAt.Before(*c.SentForSignatureAt) {
			return false, nil
		}
	}
	return true, nil
}

func (s *ContractService) syncSignatureRefusal(ctx context.Context, c *contract.Contract, signerID string) error {
	if signerID != "" {
		for _, signer := range c.Signatories {
			if signer.ID == signerID && signer.Status == contract.SignatoryStatusRefused {
				s.syncDocuments(ctx, c)
				return s.syncDealSignerStatus(ctx, c.TenantID, c.DealID, signerID, deal.SignerStatusRejected)
			}
		}
		return nil
	}
	s.syncDocuments(ctx, c)
	return s.syncAllDealSignersFromContract(ctx, c)
}
