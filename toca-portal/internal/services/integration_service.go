package services

import (
	"context"
	"fmt"

	"github.com/oginoino/toca/toca-portal/internal/domain/integration"
)

type IntegrationService struct {
	repo        integration.Repository
	feedBaseURL string
}

func NewIntegrationService(repo integration.Repository, feedBaseURL string) *IntegrationService {
	return &IntegrationService{
		repo:        repo,
		feedBaseURL: feedBaseURL,
	}
}

func (s *IntegrationService) Create(ctx context.Context, tenantID string, portal integration.PortalType, contactName, contactEmail, contactPhone, syncFrequency, publicationType string, propertyFilter integration.FeedPropertyFilter) (*integration.PortalIntegration, error) {
	// Check if integration already exists for this tenant + portal
	// (GetByTenantAndPortal devolve nil, nil quando não existe).
	existing, err := s.repo.GetByTenantAndPortal(ctx, tenantID, portal)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("integration already exists for portal %s", portal)
	}

	integ, err := integration.NewPortalIntegration(tenantID, portal, contactName, contactEmail, contactPhone, syncFrequency, publicationType, s.feedBaseURL)
	if err != nil {
		return nil, err
	}

	integ.PropertyFilter = propertyFilter

	// Generate webhook URL
	integ.WebhookURL = s.feedBaseURL + "/api/v1/portals/webhook/" + integ.ID

	if err := s.repo.Create(ctx, integ); err != nil {
		return nil, err
	}

	return integ, nil
}

// GetByID devolve a integração do tenant do chamador. Integração de outro
// tenant responde integration.ErrNotFound (o handler traduz para 404).
func (s *IntegrationService) GetByID(ctx context.Context, tenantID, id string) (*integration.PortalIntegration, error) {
	return s.repo.GetByID(ctx, tenantID, id)
}

func (s *IntegrationService) GetByFeedToken(ctx context.Context, token string) (*integration.PortalIntegration, error) {
	return s.repo.GetByFeedToken(ctx, token)
}

func (s *IntegrationService) ListByTenant(ctx context.Context, tenantID string) ([]*integration.PortalIntegration, error) {
	return s.repo.ListByTenant(ctx, tenantID)
}

func (s *IntegrationService) ListActive(ctx context.Context) ([]*integration.PortalIntegration, error) {
	return s.repo.ListActive(ctx)
}

// Update grava apenas na integração do tenant do chamador.
func (s *IntegrationService) Update(ctx context.Context, tenantID string, integ *integration.PortalIntegration) error {
	return s.repo.Update(ctx, tenantID, integ)
}

// Delete faz soft-delete apenas na integração do tenant do chamador.
func (s *IntegrationService) Delete(ctx context.Context, tenantID, id string) error {
	return s.repo.Delete(ctx, tenantID, id)
}
