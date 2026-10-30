package services

import (
	"context"

	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
)

type TemplateService struct {
	repo contract.TemplateRepository
}

func NewTemplateService(repo contract.TemplateRepository) *TemplateService {
	return &TemplateService{
		repo: repo,
	}
}

func (s *TemplateService) CreateTemplate(ctx context.Context, tpl *contract.ContractTemplate) error {
	return s.repo.CreateTemplate(ctx, tpl)
}

func (s *TemplateService) GetTemplate(ctx context.Context, tenantID, id string) (*contract.ContractTemplate, error) {
	return s.repo.GetTemplateByID(ctx, tenantID, id)
}

func (s *TemplateService) ListTemplates(ctx context.Context, tenantID string) ([]*contract.ContractTemplate, error) {
	return s.repo.ListTemplates(ctx, tenantID)
}

func (s *TemplateService) UpdateTemplate(ctx context.Context, tpl *contract.ContractTemplate) error {
	return s.repo.UpdateTemplate(ctx, tpl)
}

func (s *TemplateService) DeleteTemplate(ctx context.Context, id, tenantID string) error {
	return s.repo.DeleteTemplate(ctx, id, tenantID)
}
