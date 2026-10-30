package services

import (
	"context"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SECURITY (achado A13 da revisão de permissões de 2026-09-19): comissão avulsa
// sem campo de autor. O agregado passou a guardar quem pediu (RequestedBy) e
// quem aprovou (ApprovedBy), e o ciclo recusa criador = aprovador e aprovador =
// pagador. Estes testes provam as duas regras, a dispensa por autoridade
// financeira e o caminho de exceção (campo vazio não cria conflito).
func TestStandaloneCommissionRequesterCannotApproveAndApproverCannotPay(t *testing.T) {
	item := commission.New("deal-a13", "broker-a13", 100, "tenant-a13")
	item.RequestedBy = "user-requester"
	repo := &commissionLifecycleRepo{commission: item}
	service := NewCommissionService(repo)

	// criador ≠ aprovador
	_, err := service.Approve(context.Background(), item.TenantID, item.ID, "user-requester")
	require.ErrorIs(t, err, ErrSegregationOfDutiesRequesterCannotApprove)
	assert.Equal(t, commission.StatusPending, repo.commission.Status)
	assert.Zero(t, repo.casWrites)
	assert.Empty(t, repo.commission.ApprovedBy, "recusa não pode gravar aprovador")

	approved, err := service.Approve(context.Background(), item.TenantID, item.ID, "user-approver")
	require.NoError(t, err)
	assert.Equal(t, commission.StatusApproved, approved.Status)
	assert.Equal(t, "user-approver", approved.ApprovedBy, "a aprovação deixa o aprovador no agregado")
	assert.Equal(t, "user-approver", repo.commission.ApprovedBy, "o aprovador é persistido junto da transição")

	// aprovador ≠ pagador
	_, err = service.MarkPaidWithProof(context.Background(), item.TenantID, item.ID,
		"https://files.example/a13.pdf", "PIX conferido", "user-approver")
	require.ErrorIs(t, err, ErrSegregationOfDutiesApproverCannotPay)
	assert.Equal(t, commission.StatusApproved, repo.commission.Status)

	paid, err := service.MarkPaidWithProof(context.Background(), item.TenantID, item.ID,
		"https://files.example/a13.pdf", "PIX conferido", "user-payer")
	require.NoError(t, err)
	assert.Equal(t, commission.StatusPaid, paid.Status)
}

func TestStandaloneCommissionWaiverAllowsSingleActor(t *testing.T) {
	item := commission.New("deal-a13-waiver", "broker-a13-waiver", 100, "tenant-a13-waiver")
	item.RequestedBy = "user-admin"
	repo := &commissionLifecycleRepo{commission: item}
	service := NewCommissionService(repo)
	ctx := WithSeparationOfDutiesWaiver(context.Background())

	approved, err := service.Approve(ctx, item.TenantID, item.ID, "user-admin")
	require.NoError(t, err)
	assert.Equal(t, commission.StatusApproved, approved.Status)
	assert.Equal(t, "user-admin", approved.ApprovedBy)

	paid, err := service.MarkPaidWithProof(ctx, item.TenantID, item.ID,
		"https://files.example/a13-waiver.pdf", "PIX conferido", "user-admin")
	require.NoError(t, err)
	assert.Equal(t, commission.StatusPaid, paid.Status)
}

// Comissão ligada a payout order continua sob a segregação do payout: o
// lifecycle próprio é recusado antes de qualquer comparação de ator.
func TestLinkedCommissionStillOwnedByPayoutOrder(t *testing.T) {
	item := commission.New("deal-a13-linked", "broker-a13-linked", 100, "tenant-a13-linked")
	item.PayoutOrderID = "payout-a13"
	item.RequestedBy = "user-requester"
	repo := &commissionLifecycleRepo{commission: item}
	service := NewCommissionService(repo)

	_, err := service.Approve(context.Background(), item.TenantID, item.ID, "user-requester")
	require.ErrorIs(t, err, commission.ErrLinkedPayoutLifecycleManaged)
	assert.NotErrorIs(t, err, ErrSegregationOfDutiesRequesterCannotApprove)
}

// O solicitante gravado no lançamento manual vem do contexto AUTENTICADO
// (account_id), e não de um parâmetro que o chamador escolhe. Sem identidade
// (worker, import, teste) o campo fica vazio e não cria conflito.
func TestBatchCreatePostingRecordsRequesterFromAuthenticatedContext(t *testing.T) {
	newService := func(commissionRepo *storingManualCommissionRepo) *TransactionService {
		return NewTransactionService(newStoringTxRepo()).
			WithManualCommissionService(NewCommissionService(commissionRepo)).
			WithJournalService(NewJournalService(&recordingManualJournalRepo{}))
	}
	items := func() []BatchCreateInput {
		return []BatchCreateInput{{
			ContractID: "contract-a13", DealID: "deal-a13", Type: string(transaction.TypeComissao),
			Description: "Comissão Avulsa - Corretor", Value: 6_000, DateStr: "2026-08-29",
			BrokerID: "broker-cap", Role: "captador",
		}}
	}
	commissions := func() []ManualCommissionInput {
		return []ManualCommissionInput{{
			DealID: "deal-a13", ContractID: "contract-a13", BrokerID: "broker-cap",
			Role: "captador", Percentage: 100, Amount: 6_000,
		}}
	}

	t.Run("identidade autenticada vira solicitante", func(t *testing.T) {
		repo := newStoringManualCommissionRepo()
		ctx := pkgcontext.WithAccountID(database.WithTx(context.Background(), &sqlx.Tx{}), "user-requester")
		if _, err := newService(repo).BatchCreatePosting(ctx, "tenant-a13", "avulsa-com-autor", items(), commissions(), nil); err != nil {
			t.Fatalf("batch posting: %v", err)
		}
		require.Len(t, repo.byID, 1)
		for _, created := range repo.byID {
			assert.Equal(t, "user-requester", created.RequestedBy)
		}
	})

	t.Run("sem identidade o campo fica vazio", func(t *testing.T) {
		repo := newStoringManualCommissionRepo()
		ctx := database.WithTx(context.Background(), &sqlx.Tx{})
		if _, err := newService(repo).BatchCreatePosting(ctx, "tenant-a13", "avulsa-sem-autor", items(), commissions(), nil); err != nil {
			t.Fatalf("batch posting: %v", err)
		}
		require.Len(t, repo.byID, 1)
		for _, created := range repo.byID {
			assert.Empty(t, created.RequestedBy)
		}
	})
}

// A comissão criada pela API (Create*) carrega o solicitante e recusa a
// autoaprovação — é o caminho do POST /commissions.
func TestCreateWithRoleRecordsRequesterAndBlocksSelfApproval(t *testing.T) {
	repo := newStoringManualCommissionRepo()
	service := NewCommissionService(repo)

	created, err := service.CreateWithRole(context.Background(), "deal-api", "broker-api",
		"contract-api", "captador", 100, 500, "tenant-api", "user-requester")
	require.NoError(t, err)
	assert.Equal(t, "user-requester", created.RequestedBy)

	_, err = service.Approve(context.Background(), "tenant-api", created.ID, "user-requester")
	require.True(t, errors.Is(err, ErrSegregationOfDutiesRequesterCannotApprove), "got %v", err)
}
