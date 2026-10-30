package services

import (
	"context"
	"log"
	"os"
	"slices"

	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// ApprovalPolicyProvider is deliberately tiny so tests can inject a policy
// without starting toca-auth. *clients.AuthClient implements it.
type ApprovalPolicyProvider interface {
	GetApprovalPolicy(ctx context.Context, tenantID string) (clients.ApprovalPolicy, error)
}

type dealAuditRecorder func(context.Context, audit.Entry)

func (s *DealService) recordDealAudit(ctx context.Context, entry audit.Entry) {
	if s != nil && s.auditRecorder != nil {
		s.auditRecorder(ctx, entry)
		return
	}
	audit.Record(ctx, entry)
}

func (s *DealService) WithApprovalPolicyProvider(provider ApprovalPolicyProvider) *DealService {
	s.approvalPolicy = provider
	return s
}

// RequiresApproval keeps the historic pure helper and evaluates the explicit
// environment/default fallback. Runtime transitions use the policy-aware
// helper below after resolving toca-auth.
func RequiresApproval(d *deal.Deal, target deal.Status) bool {
	return requiresApprovalWithPolicy(d, target, fallbackApprovalPolicy())
}

func requiresApprovalWithPolicy(d *deal.Deal, target deal.Status, policy clients.ApprovalPolicy) bool {
	if d == nil || !policy.Enabled || policy.ThresholdBRL <= 0 {
		return false
	}
	if !slices.Contains(policy.TargetStatuses, string(target)) {
		return false
	}
	return d.Value > policy.ThresholdBRL
}

func (s *DealService) resolveApprovalPolicy(ctx context.Context, tenantID string) clients.ApprovalPolicy {
	if s != nil && s.approvalPolicy != nil {
		policy, err := s.approvalPolicy.GetApprovalPolicy(ctx, tenantID)
		if err == nil {
			if validationErr := policy.Validate(); validationErr == nil {
				return policy
			} else {
				log.Printf("WARN: invalid tenant approval policy; using explicit fallback tenant=%s: %v", tenantID, validationErr)
			}
		} else {
			log.Printf("WARN: approval policy unavailable; using explicit fallback tenant=%s: %v", tenantID, err)
		}
	}
	return fallbackApprovalPolicy()
}

func fallbackApprovalPolicy() clients.ApprovalPolicy {
	source := "platform_default_fallback"
	if os.Getenv("TOCA_DEAL_APPROVAL_THRESHOLD_BRL") != "" {
		source = "environment_fallback"
	}
	return clients.ApprovalPolicy{
		SchemaVersion:      1,
		Revision:           0,
		Enabled:            approvalThresholdBRL > 0,
		ThresholdBRL:       approvalThresholdBRL,
		TargetStatuses:     []string{string(deal.StatusAnalysis), string(deal.StatusContract)},
		RequiredPermission: "deals:approve",
		Source:             source,
		ResolutionSource:   source,
	}
}
