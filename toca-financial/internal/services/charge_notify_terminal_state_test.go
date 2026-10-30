package services

import (
	"context"
	"errors"
	"testing"

	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
)

func TestChargeServiceNotifyRejectsEveryNonOutstandingState(t *testing.T) {
	for _, status := range []charge.Status{
		charge.StatusPaid,
		charge.StatusCancelled,
		charge.StatusReversed,
		charge.Status("UNKNOWN"),
	} {
		t.Run(string(status), func(t *testing.T) {
			c := newPendingCharge(t)
			c.Status = status
			svc := NewChargeService(&mockChargeRepo{ch: c}, nil)

			result, err := svc.NotifyWithOptions(context.Background(), c.TenantID, c.ID, NotifyOptions{})
			if result != nil || !errors.Is(err, charge.ErrInvalidState) {
				t.Fatalf("NotifyWithOptions() result=%+v error=%v, want ErrInvalidState", result, err)
			}
		})
	}
}
