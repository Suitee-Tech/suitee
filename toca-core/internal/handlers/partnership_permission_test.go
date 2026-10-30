package handlers

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca-core/internal/app"
	"github.com/oginoino/toca-core/internal/domain/core/partnership"
	"github.com/oginoino/toca-core/internal/domain/core/property"
	repository "github.com/oginoino/toca-core/internal/repositories"
	service "github.com/oginoino/toca-core/internal/services"
	"github.com/oginoino/toca/pkg/config"
	"testing"
)

type reviewRoutePartnershipRepo struct {
	repository.PartnershipRepository
}

func (*reviewRoutePartnershipRepo) GetByID(context.Context, string) (*partnership.Partnership, error) {
	return &partnership.Partnership{ID: "agreement", RequesterID: tenantTeste, TargetID: "other-company", Status: partnership.PartnershipStatusActive, Permissions: []partnership.Permission{{Scope: partnership.PermissionScopeSharedPortfolio, AccessLevel: partnership.AccessLevelView}}}, nil
}
func (*reviewRoutePartnershipRepo) AddAuditLog(context.Context, string, string, partnership.AuditLog) error {
	return nil
}

type reviewRoutePropertyRepo struct{ repository.PropertyRepository }

func (*reviewRoutePropertyRepo) ListSharedForPartner(context.Context, string, []string, []string) ([]*property.Property, error) {
	return []*property.Property{{ID: "synthetic-shared-property"}}, nil
}
func TestSharedPortfolioRequiresActorPermissionOnBothRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("INTERNAL_API_KEY", internalKeyTest)
	matrix := permissionMatrix()
	for i := range matrix.Permissions {
		matrix.Permissions[i].Roles["service"] = false
	}
	settingsSvc := service.NewSettingsService(fakeCompanyRepo{}, fakeAppearanceRepo{}, &fakePermissionsRepo{matrix: matrix}, fakeNotificationRepo{}, newFakeWebhookRepo(), newFakeAPIKeyRepo(), nil)
	partnershipSvc := service.NewPartnershipService(&reviewRoutePartnershipRepo{})
	partnershipSvc.SetPropertyRepository(&reviewRoutePropertyRepo{})
	cfg := &app.Config{BaseConfig: config.BaseConfig{AppMode: "test", JWT: config.JWTConfig{SupabaseJWKSURL: ""}, RateLimit: config.RateLimitConfig{RPS: 1000, Burst: 1000}}}
	router := SetupRouter(cfg, nil, partnershipSvc, settingsSvc, nil, nil)
	aggregate := doInternalGet(t, router, "/api/v1/partnerships/shared/properties")
	specific := doInternalGet(t, router, "/api/v1/partnerships/agreement/shared-portfolio")
	if aggregate.Code != 403 || specific.Code != 403 {
		t.Fatalf("not reproduced: aggregate=%d specific=%d %s", aggregate.Code, specific.Code, specific.Body.String())
	}
}
