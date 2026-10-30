package services

import (
	"context"
	"fmt"
	"math"

	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
)

func (s *TransactionService) createPartnershipShare(ctx context.Context, tx *transaction.Transaction) error {
	if reader, ok := s.repo.(interface {
		PartnershipShare(context.Context, string, string, string) (*transaction.Transaction, error)
	}); ok {
		old, err := reader.PartnershipShare(ctx, tx.TenantID, tx.DealID, tx.ContractID)
		if err != nil {
			return err
		}
		if old != nil {
			if math.Round(old.Value*100) != math.Round(tx.Value*100) {
				return fmt.Errorf("comissão de parceria divergente: reconcilie o histórico antes de provisionar")
			}
			return nil
		}
	}
	persisted, err := s.CreateFromRequest(ctx, events.DeriveNotificationID("partnership_commission", tx.DealID, tx.TenantID), tx)
	if err != nil {
		return err
	}
	if persisted == nil || math.Round(persisted.Value*100) != math.Round(tx.Value*100) {
		return fmt.Errorf("comissão de parceria divergente: reconcilie o histórico antes de provisionar")
	}
	return nil
}
