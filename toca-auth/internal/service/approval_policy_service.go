package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/oginoino/toca-auth/internal/domain/tenantsettings"
	"github.com/oginoino/toca/pkg/audit"
)

type ApprovalPolicyRepository interface {
	GetApprovalPolicy(ctx context.Context, tenantID string) (tenantsettings.ApprovalPolicy, bool, error)
	UpsertApprovalPolicy(ctx context.Context, tenantID string, enabled bool, thresholdBRL float64, updatedBy string) (tenantsettings.ApprovalPolicy, error)
}

type UpdateApprovalPolicyInput struct {
	Enabled      bool
	ThresholdBRL float64
	UpdatedBy    string
}

type ApprovalPolicyService struct {
	repo ApprovalPolicyRepository
}

func NewApprovalPolicyService(repo ApprovalPolicyRepository) *ApprovalPolicyService {
	return &ApprovalPolicyService{repo: repo}
}

func (s *ApprovalPolicyService) Get(ctx context.Context, tenantID string) (tenantsettings.ApprovalPolicy, error) {
	if _, err := uuid.Parse(strings.TrimSpace(tenantID)); err != nil {
		return tenantsettings.ApprovalPolicy{}, errors.New("valid tenant_id is required")
	}
	if s == nil || s.repo == nil {
		return tenantsettings.ApprovalPolicy{}, errors.New("approval policy repository is unavailable")
	}

	policy, found, err := s.repo.GetApprovalPolicy(ctx, tenantID)
	if err != nil {
		return tenantsettings.ApprovalPolicy{}, err
	}
	if !found {
		return tenantsettings.DefaultApprovalPolicy(), nil
	}
	if err := policy.Validate(); err != nil {
		return tenantsettings.ApprovalPolicy{}, fmt.Errorf("invalid tenant approval policy: %w", err)
	}
	return policy, nil
}

func (s *ApprovalPolicyService) Update(ctx context.Context, tenantID string, input UpdateApprovalPolicyInput) (tenantsettings.ApprovalPolicy, error) {
	if _, err := uuid.Parse(strings.TrimSpace(tenantID)); err != nil {
		return tenantsettings.ApprovalPolicy{}, errors.New("valid tenant_id is required")
	}
	if s == nil || s.repo == nil {
		return tenantsettings.ApprovalPolicy{}, errors.New("approval policy repository is unavailable")
	}

	candidate := tenantsettings.DefaultApprovalPolicy()
	candidate.Enabled = input.Enabled
	candidate.ThresholdBRL = input.ThresholdBRL
	if err := candidate.Validate(); err != nil {
		return tenantsettings.ApprovalPolicy{}, err
	}

	before, found, err := s.repo.GetApprovalPolicy(ctx, tenantID)
	if err != nil {
		return tenantsettings.ApprovalPolicy{}, err
	}
	if !found {
		before = tenantsettings.DefaultApprovalPolicy()
	}

	after, err := s.repo.UpsertApprovalPolicy(ctx, tenantID, input.Enabled, input.ThresholdBRL, strings.TrimSpace(input.UpdatedBy))
	if err != nil {
		return tenantsettings.ApprovalPolicy{}, err
	}

	audit.Record(ctx, audit.Entry{
		Action:     "tenant.approval_policy_updated",
		EntityType: "TenantSettings",
		EntityID:   tenantID,
		Before:     approvalPolicyAuditSnapshot(before),
		After:      approvalPolicyAuditSnapshot(after),
		Metadata: map[string]any{
			"policy_schema_version": after.SchemaVersion,
			"policy_revision":       after.Revision,
		},
	})

	return after, nil
}

func approvalPolicyAuditSnapshot(policy tenantsettings.ApprovalPolicy) map[string]any {
	return map[string]any{
		"schema_version":      policy.SchemaVersion,
		"revision":            policy.Revision,
		"enabled":             policy.Enabled,
		"threshold_brl":       policy.ThresholdBRL,
		"target_statuses":     policy.TargetStatuses,
		"required_permission": policy.RequiredPermission,
		"source":              policy.Source,
	}
}
