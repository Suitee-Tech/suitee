package services

import (
	"context"
	"testing"

	"github.com/oginoino/toca/toca-financial/internal/domain/utility_account"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type utilityAccountMemoryRepo struct {
	items   map[string]*utility_account.UtilityAccount
	updates int
}

func newUtilityAccountMemoryRepo() *utilityAccountMemoryRepo {
	return &utilityAccountMemoryRepo{items: make(map[string]*utility_account.UtilityAccount)}
}

func (r *utilityAccountMemoryRepo) Create(_ context.Context, account *utility_account.UtilityAccount) error {
	copy := *account
	r.items[account.ID] = &copy
	return nil
}

func (r *utilityAccountMemoryRepo) GetByID(_ context.Context, tenantID, id string) (*utility_account.UtilityAccount, error) {
	account := r.items[id]
	if account == nil || account.TenantID != tenantID {
		return nil, nil
	}
	copy := *account
	return &copy, nil
}

func (r *utilityAccountMemoryRepo) ListByContract(_ context.Context, tenantID, contractID string) ([]*utility_account.UtilityAccount, error) {
	items := make([]*utility_account.UtilityAccount, 0)
	for _, account := range r.items {
		if account.TenantID == tenantID && account.ContractID == contractID {
			copy := *account
			items = append(items, &copy)
		}
	}
	return items, nil
}

func (r *utilityAccountMemoryRepo) Update(_ context.Context, account *utility_account.UtilityAccount) error {
	r.updates++
	copy := *account
	r.items[account.ID] = &copy
	return nil
}

func (r *utilityAccountMemoryRepo) Delete(_ context.Context, tenantID, id string) error {
	if account := r.items[id]; account != nil && account.TenantID == tenantID {
		delete(r.items, id)
	}
	return nil
}

func baseUtilityAccountInput() CreateUtilityAccountInput {
	return CreateUtilityAccountInput{
		ContractID:       "contract-1",
		PropertyID:       "property-1",
		Type:             utility_account.TypeEnergia,
		Provider:         "Enel",
		ResponsibleParty: utility_account.ResponsibleTenant,
		EstimatedAmount:  19_990,
	}
}

func TestUtilityAccountCreatePersistsBillingMode(t *testing.T) {
	repo := newUtilityAccountMemoryRepo()
	service := NewUtilityAccountService(repo)

	in := baseUtilityAccountInput()
	in.BillingMode = utility_account.BillingModeVariableSeparate
	created, err := service.Create(context.Background(), "tenant-1", in)

	require.NoError(t, err)
	assert.Equal(t, utility_account.BillingModeVariableSeparate, created.BillingMode)
	assert.Empty(t, created.BundledItemKey)
	assert.Equal(t, utility_account.BillingModeVariableSeparate, repo.items[created.ID].BillingMode)
}

func TestUtilityAccountCreateDefaultsToInformationalMode(t *testing.T) {
	repo := newUtilityAccountMemoryRepo()
	service := NewUtilityAccountService(repo)

	created, err := service.Create(context.Background(), "tenant-1", baseUtilityAccountInput())

	require.NoError(t, err)
	assert.Equal(t, utility_account.BillingModeInformational, created.BillingMode)
}

func TestUtilityAccountBundledModeRequiresAndPersistsItemKey(t *testing.T) {
	repo := newUtilityAccountMemoryRepo()
	service := NewUtilityAccountService(repo)
	in := baseUtilityAccountInput()
	in.BillingMode = utility_account.BillingModeBundled

	_, err := service.Create(context.Background(), "tenant-1", in)
	require.EqualError(t, err, "bundled_item_key is required for BUNDLED billing mode")

	in.BundledItemKey = "  condominio  "
	created, err := service.Create(context.Background(), "tenant-1", in)
	require.NoError(t, err)
	assert.Equal(t, "condominio", created.BundledItemKey)
}

func TestUtilityAccountUpdateKeepsBillingModeCoherent(t *testing.T) {
	repo := newUtilityAccountMemoryRepo()
	service := NewUtilityAccountService(repo)
	in := baseUtilityAccountInput()
	in.BillingMode = utility_account.BillingModeBundled
	in.BundledItemKey = "iptu"
	created, err := service.Create(context.Background(), "tenant-1", in)
	require.NoError(t, err)

	mode := utility_account.BillingModeVariableSeparate
	updated, err := service.Update(context.Background(), "tenant-1", created.ID, UpdateUtilityAccountInput{
		BillingMode: &mode,
	})

	require.NoError(t, err)
	assert.Equal(t, utility_account.BillingModeVariableSeparate, updated.BillingMode)
	assert.Empty(t, updated.BundledItemKey)
}

func TestUtilityAccountUpdateRejectsBundledModeWithoutItemKey(t *testing.T) {
	repo := newUtilityAccountMemoryRepo()
	service := NewUtilityAccountService(repo)
	created, err := service.Create(context.Background(), "tenant-1", baseUtilityAccountInput())
	require.NoError(t, err)

	mode := utility_account.BillingModeBundled
	_, err = service.Update(context.Background(), "tenant-1", created.ID, UpdateUtilityAccountInput{
		BillingMode: &mode,
	})

	require.EqualError(t, err, "bundled_item_key is required for BUNDLED billing mode")
	assert.Zero(t, repo.updates)
}
