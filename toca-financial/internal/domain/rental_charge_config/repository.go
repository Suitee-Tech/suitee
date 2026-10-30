package rental_charge_config

import (
	"context"
	"errors"
)

var ErrAlreadyExists = errors.New("rental charge config already exists for contract")

type Repository interface {
	Create(ctx context.Context, config *RentalChargeConfig) error
	GetByID(ctx context.Context, tenantID, id string) (*RentalChargeConfig, error)
	GetByContractID(ctx context.Context, tenantID, contractID string) (*RentalChargeConfig, error)
	Update(ctx context.Context, config *RentalChargeConfig) error
	Delete(ctx context.Context, tenantID, id string) error
}
