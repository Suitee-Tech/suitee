package services_test

import (
	"context"
	"testing"

	service "github.com/oginoino/toca/toca-financial/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPayoutRecomputeForChargeIsDisabledBeforeRepositoryAccess(t *testing.T) {
	// Nil repositories make this a regression guard for the destructive ordering:
	// the method must return the explicit error before listing or cancelling any
	// persisted payout order.
	payoutService := service.NewPayoutOrchestrationService(nil, nil, nil)

	cancelled, err := payoutService.RecomputeForCharge(
		context.Background(), "tenant-recompute", "charge-recompute", "admin-recompute",
	)

	require.ErrorIs(t, err, service.ErrPayoutRecomputeDisabled)
	assert.Zero(t, cancelled)
}
