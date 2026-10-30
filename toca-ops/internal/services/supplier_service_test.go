package services_test

import (
	"context"
	"testing"

	"github.com/oginoino/toca/toca-ops/internal/models/supplier"
	service "github.com/oginoino/toca/toca-ops/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSupplierRepo struct {
	items     []*supplier.Supplier
	createErr error
}

func (r *fakeSupplierRepo) Create(_ context.Context, v *supplier.Supplier) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.items = append(r.items, v)
	return nil
}

func (r *fakeSupplierRepo) GetByID(_ context.Context, tenantID, id string) (*supplier.Supplier, error) {
	for _, item := range r.items {
		if item.ID == id && item.TenantID == tenantID {
			return item, nil
		}
	}
	return nil, supplier.ErrNotFound
}

func (r *fakeSupplierRepo) Update(_ context.Context, v *supplier.Supplier) error {
	for index, item := range r.items {
		if item.ID == v.ID {
			r.items[index] = v
		}
	}
	return nil
}

func (r *fakeSupplierRepo) Delete(_ context.Context, tenantID, id string) error {
	for index, item := range r.items {
		if item.ID == id && item.TenantID == tenantID {
			r.items = append(r.items[:index], r.items[index+1:]...)
			return nil
		}
	}
	return supplier.ErrNotFound
}

func (r *fakeSupplierRepo) List(_ context.Context, tenantID string, filter supplier.Filter) ([]*supplier.Supplier, int64, error) {
	out := make([]*supplier.Supplier, 0)
	for _, item := range r.items {
		if item.TenantID != tenantID {
			continue
		}
		if filter.ServiceType != "" && item.ServiceType != filter.ServiceType {
			continue
		}
		if filter.ActiveOnly && !item.Active {
			continue
		}
		out = append(out, item)
	}
	return out, int64(len(out)), nil
}

func (r *fakeSupplierRepo) FindDuplicate(_ context.Context, tenantID, document, phone, email string) (*supplier.Supplier, error) {
	for _, item := range r.items {
		if item.TenantID != tenantID {
			continue
		}
		if document != "" && item.Document == document {
			return item, nil
		}
		if phone != "" && item.Phone == phone {
			return item, nil
		}
		if email != "" && item.Email == email {
			return item, nil
		}
	}
	return nil, nil
}

func TestSupplierCreateRequiresContactAndServiceType(t *testing.T) {
	svc := service.NewSupplierService(&fakeSupplierRepo{})

	_, _, err := svc.Create(context.Background(), supplier.New("Carlos Silva", "", "externo", "", "carlos@ex.com", "", "", "", "tenant-1"))
	require.ErrorIs(t, err, supplier.ErrServiceTypeRequired)

	_, _, err = svc.Create(context.Background(), supplier.New("Carlos Silva", "Elétrica", "externo", "", "", "", "", "", "tenant-1"))
	require.ErrorIs(t, err, supplier.ErrContactRequired)
}

func TestSupplierCreateReusesExistingRegistration(t *testing.T) {
	repo := &fakeSupplierRepo{}
	svc := service.NewSupplierService(repo)

	first, reused, err := svc.Create(context.Background(), supplier.New("Carlos Silva", "Elétrica", "externo", "", "", "11999990000", "Atende zona sul", "", "tenant-1"))
	require.NoError(t, err)
	require.False(t, reused)
	require.Len(t, repo.items, 1)

	// Mesmo telefone: reutiliza em vez de duplicar o prestador.
	second, reused, err := svc.Create(context.Background(), supplier.New("Carlos S.", "Elétrica", "externo", "", "", "11999990000", "", "", "tenant-1"))
	require.NoError(t, err)
	assert.True(t, reused, "cadastro repetido deve reutilizar o prestador existente")
	assert.Equal(t, first.ID, second.ID)
	assert.Len(t, repo.items, 1, "nenhum duplicado é criado")
}

func TestSupplierServiceTypesComeFromRegistry(t *testing.T) {
	repo := &fakeSupplierRepo{}
	svc := service.NewSupplierService(repo)
	for _, v := range []*supplier.Supplier{
		supplier.New("Carlos Silva", "Elétrica", "externo", "", "", "11999990000", "", "", "tenant-1"),
		supplier.New("João Santos", "Hidráulica", "externo", "", "", "11999990001", "", "", "tenant-1"),
		supplier.New("Roberto", "Elétrica", "interno", "", "roberto@imob.com", "", "", "", "tenant-1"),
	} {
		_, _, err := svc.Create(context.Background(), v)
		require.NoError(t, err)
	}

	types, err := svc.ServiceTypes(context.Background(), "tenant-1")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"Elétrica", "Hidráulica"}, types, "tipos repetidos não se duplicam")
}

func TestSupplierUpdatePreservesFieldsNotInformed(t *testing.T) {
	repo := &fakeSupplierRepo{}
	svc := service.NewSupplierService(repo)
	created, _, err := svc.Create(context.Background(), supplier.New("Carlos Silva", "Elétrica", "externo", "52998224725", "carlos@ex.com", "11999990000", "Atende zona sul", "São Paulo", "tenant-1"))
	require.NoError(t, err)

	notes := "Agora atende também zona norte"
	updated, err := svc.Update(context.Background(), "tenant-1", created.ID, supplier.Changes{Notes: &notes})
	require.NoError(t, err)

	assert.Equal(t, "Agora atende também zona norte", updated.Notes)
	assert.Equal(t, "Carlos Silva", updated.Name)
	assert.Equal(t, "Elétrica", updated.ServiceType)
	assert.Equal(t, "+5511999990000", updated.Phone, "contato permanece")
	assert.True(t, updated.Active)
}

func TestSupplierDeactivateKeepsRecordForHistory(t *testing.T) {
	repo := &fakeSupplierRepo{}
	svc := service.NewSupplierService(repo)
	created, _, err := svc.Create(context.Background(), supplier.New("João Santos", "Hidráulica", "externo", "", "", "11999990001", "", "", "tenant-1"))
	require.NoError(t, err)

	deactivated, err := svc.Deactivate(context.Background(), "tenant-1", created.ID)
	require.NoError(t, err)
	assert.False(t, deactivated.Active)

	// Continua existindo (histórico), mas sai da seleção de ativos.
	stored, err := svc.GetByID(context.Background(), "tenant-1", created.ID)
	require.NoError(t, err)
	assert.False(t, stored.Active)
	active, _, err := svc.List(context.Background(), "tenant-1", supplier.Filter{ActiveOnly: true})
	require.NoError(t, err)
	assert.Empty(t, active)
}

func TestSupplierDisplayIncludesServiceType(t *testing.T) {
	v := supplier.New("Carlos Silva", "Elétrica", "externo", "", "", "11999990000", "", "", "tenant-1")
	assert.Equal(t, "Carlos Silva (Elétrica)", v.Display())
}
