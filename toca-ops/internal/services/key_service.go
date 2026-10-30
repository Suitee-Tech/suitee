package services

import (
	"context"
	"fmt"
	"time"

	"github.com/oginoino/toca/toca-ops/internal/models/key"
)

type KeyService struct {
	repo key.Repository
}

func NewKeyService(repo key.Repository) *KeyService {
	return &KeyService{repo: repo}
}

func (s *KeyService) Create(ctx context.Context, k *key.Key) error {
	if k.PropertyID == "" {
		return fmt.Errorf("property_id is required")
	}
	if k.Code == "" {
		return fmt.Errorf("code is required")
	}
	return s.repo.Create(ctx, k)
}

func (s *KeyService) GetByID(ctx context.Context, tenantID, id string) (*key.Key, error) {
	k, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if k == nil {
		return nil, fmt.Errorf("key not found")
	}
	return k, nil
}

func (s *KeyService) Delete(ctx context.Context, tenantID, id string) error {
	return s.repo.Delete(ctx, tenantID, id)
}

func (s *KeyService) List(ctx context.Context, tenantID, propertyID, status string, limit, offset int64) ([]*key.Key, int64, error) {
	return s.repo.List(ctx, tenantID, propertyID, status, limit, offset)
}

func (s *KeyService) CheckOut(ctx context.Context, tenantID, id, checkedOutBy string) (*key.Key, error) {
	k, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if k == nil {
		return nil, fmt.Errorf("key not found")
	}
	if k.Status == key.StatusCheckedOut {
		return nil, fmt.Errorf("key is already checked out")
	}

	now := time.Now()
	k.Status = key.StatusCheckedOut
	k.CheckedOutBy = checkedOutBy
	k.CheckedOutAt = &now
	k.ReturnedAt = nil

	if err := s.repo.Update(ctx, k); err != nil {
		return nil, err
	}
	return k, nil
}

func (s *KeyService) CheckIn(ctx context.Context, tenantID, id string) (*key.Key, error) {
	k, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if k == nil {
		return nil, fmt.Errorf("key not found")
	}
	if k.Status == key.StatusAvailable {
		return nil, fmt.Errorf("key is already available")
	}

	now := time.Now()
	k.Status = key.StatusAvailable
	k.CheckedOutBy = ""
	k.CheckedOutAt = nil
	k.ReturnedAt = &now

	if err := s.repo.Update(ctx, k); err != nil {
		return nil, err
	}
	return k, nil
}
