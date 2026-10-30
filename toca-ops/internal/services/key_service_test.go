package services_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-ops/internal/models/key"
	"github.com/oginoino/toca/toca-ops/internal/services"
	"github.com/stretchr/testify/assert"
)

// inMemoryKeyRepo espelha o repositório Postgres: GetByID é escopado pelo
// tenant do chamador e falha fechado quando ele está vazio (auditoria A8/ops).
type inMemoryKeyRepo struct {
	mu   sync.Mutex
	keys map[string]*key.Key
}

func newInMemoryKeyRepo() *inMemoryKeyRepo {
	return &inMemoryKeyRepo{keys: make(map[string]*key.Key)}
}

func (r *inMemoryKeyRepo) Create(_ context.Context, k *key.Key) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *k
	r.keys[k.ID] = &cp
	return nil
}

func (r *inMemoryKeyRepo) GetByID(_ context.Context, tenantID, id string) (*key.Key, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if strings.TrimSpace(tenantID) == "" {
		return nil, nil
	}
	k, ok := r.keys[id]
	if !ok || k.TenantID != tenantID {
		return nil, nil
	}
	cp := *k
	return &cp, nil
}

func (r *inMemoryKeyRepo) Update(_ context.Context, k *key.Key) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *k
	r.keys[k.ID] = &cp
	return nil
}

func (r *inMemoryKeyRepo) Delete(_ context.Context, tenantID, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.keys, id)
	return nil
}

func (r *inMemoryKeyRepo) List(_ context.Context, tenantID, propertyID, status string, limit, offset int64) ([]*key.Key, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*key.Key, 0)
	for _, k := range r.keys {
		if k.TenantID != tenantID {
			continue
		}
		cp := *k
		out = append(out, &cp)
	}
	return out, int64(len(out)), nil
}

func TestKeyService_GetByID_RejectsForeignTenant(t *testing.T) {
	repo := newInMemoryKeyRepo()
	svc := services.NewKeyService(repo)

	k := key.New("prop-B", "K-01", "Portaria", "", "tenant-B")
	assert.NoError(t, repo.Create(context.Background(), k))

	got, err := svc.GetByID(context.Background(), "tenant-A", k.ID)
	assert.Nil(t, got)
	assert.Error(t, err)

	own, err := svc.GetByID(context.Background(), "tenant-B", k.ID)
	assert.NoError(t, err)
	assert.NotNil(t, own)
}

func TestKeyService_GetByID_FailsClosedWithoutTenant(t *testing.T) {
	repo := newInMemoryKeyRepo()
	svc := services.NewKeyService(repo)

	k := key.New("prop-1", "K-01", "Portaria", "", "tenant-1")
	assert.NoError(t, repo.Create(context.Background(), k))

	got, err := svc.GetByID(context.Background(), "", k.ID)
	assert.Nil(t, got)
	assert.Error(t, err)
}

func TestKeyService_CheckOut_RejectsForeignTenant(t *testing.T) {
	repo := newInMemoryKeyRepo()
	svc := services.NewKeyService(repo)

	k := key.New("prop-B", "K-01", "Portaria", "", "tenant-B")
	assert.NoError(t, repo.Create(context.Background(), k))

	_, err := svc.CheckOut(context.Background(), "tenant-A", k.ID, "intruso")
	assert.Error(t, err)

	stored, err := svc.GetByID(context.Background(), "tenant-B", k.ID)
	assert.NoError(t, err)
	if assert.NotNil(t, stored) {
		assert.Equal(t, key.StatusAvailable, stored.Status, "chave de outro tenant não pode sofrer check-out")
		assert.Empty(t, stored.CheckedOutBy)
	}

	_, err = svc.CheckIn(context.Background(), "tenant-A", k.ID)
	assert.Error(t, err)
	_ = time.Now()
}
