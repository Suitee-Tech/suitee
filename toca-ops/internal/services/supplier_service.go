package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/oginoino/toca/toca-ops/internal/models/supplier"
)

// SupplierService mantém o cadastro de profissionais/fornecedores da imobiliária
// e é a fonte de verdade para reutilizar o mesmo prestador em chamados e
// vistorias.
type SupplierService struct {
	repo supplier.Repository
}

func NewSupplierService(repo supplier.Repository) *SupplierService {
	return &SupplierService{repo: repo}
}

// Create cadastra o prestador. Documento ou contato já cadastrado devolve o
// registro existente em vez de criar um segundo — reutilizar é o objetivo.
func (s *SupplierService) Create(ctx context.Context, v *supplier.Supplier) (*supplier.Supplier, bool, error) {
	if v == nil {
		return nil, false, fmt.Errorf("profissional é obrigatório")
	}
	v.Normalize()
	if err := v.Validate(); err != nil {
		return nil, false, err
	}

	existing, err := s.repo.FindDuplicate(ctx, v.TenantID, v.Document, v.Phone, v.Email)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		return existing, true, nil
	}
	if err := s.repo.Create(ctx, v); err != nil {
		return nil, false, err
	}
	return v, false, nil
}

func (s *SupplierService) GetByID(ctx context.Context, tenantID, id string) (*supplier.Supplier, error) {
	if strings.TrimSpace(id) == "" {
		return nil, supplier.ErrNotFound
	}
	return s.repo.GetByID(ctx, tenantID, id)
}

// Update aplica as mudanças sobre o cadastro existente, preservando o que não
// foi informado — a tela edita um campo por vez.
func (s *SupplierService) Update(ctx context.Context, tenantID, id string, changes supplier.Changes) (*supplier.Supplier, error) {
	current, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	if changes.Name != nil {
		current.Name = *changes.Name
	}
	if changes.ServiceType != nil {
		current.ServiceType = *changes.ServiceType
	}
	if changes.Kind != nil {
		current.Kind = *changes.Kind
	}
	if changes.Document != nil {
		current.Document = *changes.Document
	}
	if changes.Email != nil {
		current.Email = *changes.Email
	}
	if changes.Phone != nil {
		current.Phone = *changes.Phone
	}
	if changes.Notes != nil {
		current.Notes = *changes.Notes
	}
	if changes.City != nil {
		current.City = *changes.City
	}
	if changes.Active != nil {
		current.Active = *changes.Active
	}

	current.Normalize()
	if err := current.Validate(); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, current); err != nil {
		return nil, err
	}
	return current, nil
}

// Deactivate tira o prestador da seleção sem apagar o histórico.
func (s *SupplierService) Deactivate(ctx context.Context, tenantID, id string) (*supplier.Supplier, error) {
	current, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	current.Active = false
	if err := s.repo.Update(ctx, current); err != nil {
		return nil, err
	}
	return current, nil
}

func (s *SupplierService) Delete(ctx context.Context, tenantID, id string) error {
	return s.repo.Delete(ctx, tenantID, id)
}

func (s *SupplierService) List(ctx context.Context, tenantID string, filter supplier.Filter) ([]*supplier.Supplier, int64, error) {
	return s.repo.List(ctx, tenantID, filter)
}

// ServiceTypes devolve os tipos de serviço já cadastrados, para o formulário
// reutilizar a nomenclatura da imobiliária em vez de inventar outra.
func (s *SupplierService) ServiceTypes(ctx context.Context, tenantID string) ([]string, error) {
	all, _, err := s.repo.List(ctx, tenantID, supplier.Filter{Limit: 200})
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(all))
	out := make([]string, 0, len(all))
	for _, v := range all {
		if v == nil || v.ServiceType == "" || seen[v.ServiceType] {
			continue
		}
		seen[v.ServiceType] = true
		out = append(out, v.ServiceType)
	}
	return out, nil
}
