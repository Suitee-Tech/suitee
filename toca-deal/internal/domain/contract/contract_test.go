package contract_test

import (
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
)

func TestValidateSignatories(t *testing.T) {
	tests := []struct {
		name        string
		cType       contract.ContractType
		signatories []contract.Signatory
		wantErr     bool
	}{
		{
			name:  "Valid Venda",
			cType: contract.TypeVenda,
			signatories: []contract.Signatory{
				{Role: contract.RoleComprador},
				{Role: contract.RoleVendedor},
				{Role: contract.RoleTestemunha},
			},
			wantErr: false,
		},
		{
			name:  "Invalid Venda - Inquilino not allowed",
			cType: contract.TypeVenda,
			signatories: []contract.Signatory{
				{Role: contract.RoleComprador},
				{Role: contract.RoleInquilino},
			},
			wantErr: true,
		},
		{
			name:  "Valid Venda - spouse and representatives",
			cType: contract.TypeVenda,
			signatories: []contract.Signatory{
				{Role: contract.RoleConjuge},
				{Role: contract.RoleProcurador},
				{Role: contract.RoleRepresentanteLegal},
				{Role: contract.RoleTutor},
				{Role: contract.RoleCurador},
			},
			wantErr: false,
		},
		{
			name:  "Valid Locacao",
			cType: contract.TypeLocacao,
			signatories: []contract.Signatory{
				{Role: contract.RoleInquilino},
				{Role: contract.RoleProprietario},
				{Role: contract.RoleFiador},
			},
			wantErr: false,
		},
		{
			name:  "Invalid Locacao - Comprador not allowed",
			cType: contract.TypeLocacao,
			signatories: []contract.Signatory{
				{Role: contract.RoleInquilino},
				{Role: contract.RoleComprador},
			},
			wantErr: true,
		},
		{
			name:  "Valid Locacao - spouse and representatives",
			cType: contract.TypeLocacao,
			signatories: []contract.Signatory{
				{Role: contract.RoleConjuge},
				{Role: contract.RoleProcurador},
				{Role: contract.RoleRepresentanteLegal},
				{Role: contract.RoleTutor},
				{Role: contract.RoleCurador},
			},
			wantErr: false,
		},
		{
			name:        "Invalid contract type",
			cType:       contract.ContractType("cessao"),
			signatories: nil,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := contract.ValidateSignatories(tt.cType, tt.signatories)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSignatories() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContractValidationErrorsAreClassified(t *testing.T) {
	t.Run("signatory role", func(t *testing.T) {
		err := contract.ValidateSignatories(contract.TypeVenda, []contract.Signatory{{Role: contract.RoleInquilino}})
		if !errors.Is(err, contract.ErrInvalidSignatoryRole) || !contract.IsValidationError(err) {
			t.Fatalf("expected invalid signatory role validation error, got %v", err)
		}
	})

	t.Run("contract type", func(t *testing.T) {
		err := contract.ValidateSignatories(contract.ContractType("cessao"), nil)
		if !errors.Is(err, contract.ErrInvalidContractType) || !contract.IsValidationError(err) {
			t.Fatalf("expected invalid contract type validation error, got %v", err)
		}
	})

	t.Run("contract period", func(t *testing.T) {
		start := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
		end := start.Add(-24 * time.Hour)
		_, err := contract.New("deal-1", "content", nil, start, &end, "tenant-1", contract.TypeLocacao)
		if !errors.Is(err, contract.ErrInvalidContractPeriod) || !contract.IsValidationError(err) {
			t.Fatalf("expected invalid contract period validation error, got %v", err)
		}
	})
}

func TestNew_Locacao_DefaultEndDate30Months(t *testing.T) {
	start := time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC)
	c, err := contract.New("deal-1", "content", nil, start, nil, "tenant-1", contract.TypeLocacao)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.EndDate == nil {
		t.Fatalf("expected end_date, got nil")
	}
	want := start.AddDate(0, contract.DefaultLeaseTermMonths, 0)
	if !c.EndDate.Equal(want) {
		t.Fatalf("expected %s, got %s", want.Format(time.RFC3339), c.EndDate.Format(time.RFC3339))
	}
}
