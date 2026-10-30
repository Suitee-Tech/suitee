package services

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/payout_order"
	"github.com/oginoino/toca/toca-financial/internal/repositories"
	"github.com/stretchr/testify/require"
)

func TestSaleCommissionFundingPostgresAtomicConcurrentAndIdempotent(t *testing.T) {
	db := openFinancialConsistencyPostgres(t)
	tenantID, contractID, dealID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	ctx, cancel := context.WithTimeout(pkgcontext.WithTenantID(context.Background(), tenantID), 45*time.Second)
	defer cancel()
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(pkgcontext.WithTenantID(context.Background(), tenantID), 10*time.Second)
		defer cancel()
		for _, table := range []string{"payout_orders", "commissions"} {
			_, err := db.Exec(cleanupCtx, "DELETE FROM financial."+table+" WHERE tenant_id = $1", tenantID)
			if err != nil {
				t.Errorf("cleanup %s: %v", table, err)
			}
		}
	})
	commRepo := repositories.NewCommissionPostgresRepository(db)
	payoutRepo := repositories.NewPayoutOrderPostgresRepository(db)
	comm := commission.NewWithRole(dealID, uuid.NewString(), contractID, "captador", 100, 250, tenantID)
	comm.PlanHash = "frozen-sale-plan"
	_, err := comm.EnableFundingGate(25000)
	require.NoError(t, err)
	order, err := payout_order.New(tenantID, payout_order.SourceManual, contractID, payout_order.RecipientBroker, comm.BrokerID, "Corretor", 25000, "system")
	require.NoError(t, err)
	order.ContractID = contractID
	_, err = order.EnableFundingGate(25000)
	require.NoError(t, err)
	comm.PayoutOrderID = order.ID
	require.NoError(t, payoutRepo.Create(ctx, order))
	require.NoError(t, commRepo.Create(ctx, comm))
	svc := NewSaleCommissionFundingService(NewCommissionService(commRepo), payoutRepo).RequirePaymentTransaction()
	c := newSaleFundingCharge(t, uuid.NewString(), "2026-09")
	c.TenantID, c.ContractID, c.DealID, c.OwnerID = tenantID, contractID, dealID, uuid.NewString()
	for i := range c.LineItems {
		c.LineItems[i].EconomicOwnerID = tenantID
		if c.LineItems[i].AllocationType == charge.AllocationTypeOwnerPayable {
			c.LineItems[i].EconomicOwnerID = c.OwnerID
		}
	}
	paidAt := time.Now().UTC().Truncate(time.Microsecond)
	secondChargeID := uuid.NewString()
	apply := func(paymentID string, rollback bool) error {
		installment := *c
		installment.LineItems = append([]charge.LineItem(nil), c.LineItems...)
		if paymentID == "receipt-1" {
			installment.ID = secondChargeID
			installment.ReferenceMonth = "2026-10"
		}
		_, allocations, err := NewEconomicAllocationService(nil).BuildPaymentAllocations(ctx, &installment, paymentID, 50000)
		if err != nil {
			return err
		}
		return db.RunInTx(ctx, func(tx *sqlx.Tx) error {
			result, err := svc.FundAllocations(database.WithTx(ctx, tx), &installment, paymentID, paidAt, allocations)
			if err != nil {
				return err
			}
			if err := charge.ValidateAllocations(result, 50000); err != nil {
				return err
			}
			if rollback {
				return errors.New("rollback-sale-funding")
			}
			return nil
		})
	}
	require.EqualError(t, apply("rollback", true), "rollback-sale-funding")
	afterRollback, err := commRepo.GetByID(ctx, tenantID, comm.ID)
	require.NoError(t, err)
	require.Zero(t, afterRollback.FundedAmountCents)
	payoutAfterRollback, err := payoutRepo.GetByID(ctx, tenantID, order.ID)
	require.NoError(t, err)
	require.Zero(t, payoutAfterRollback.FundedAmountCents)

	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		paymentID := fmt.Sprintf("receipt-%d", i)
		go func() { <-start; results <- apply(paymentID, false) }()
	}
	close(start)
	for i := 0; i < 2; i++ {
		require.NoError(t, <-results)
	}
	require.NoError(t, apply("receipt-0", false))
	funded, err := commRepo.GetByID(ctx, tenantID, comm.ID)
	require.NoError(t, err)
	payout, err := payoutRepo.GetByID(ctx, tenantID, order.ID)
	require.NoError(t, err)
	require.EqualValues(t, 25000, funded.FundedAmountCents)
	require.EqualValues(t, 25000, payout.FundedAmountCents)
	require.Len(t, funded.FundingEntries, 2)
	require.Len(t, payout.FundingEntries, 2)
	require.Equal(t, payout_order.StatusPendingReview, payout.Status)
	require.NoError(t, funded.Approve())
	require.NoError(t, payout.Approve("operator"))
}
