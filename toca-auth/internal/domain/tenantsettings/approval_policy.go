package tenantsettings

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"time"
)

const (
	ApprovalPolicySchemaVersion = 1
	DefaultApprovalThresholdBRL = 500_000.0
	MaxApprovalThresholdBRL     = 1_000_000_000_000.0
	ApprovalPermission          = "deals:approve"
)

var DefaultApprovalTargetStatuses = []string{"ANALYSIS", "CONTRACT"}

// ApprovalPolicy is the versioned tenant contract consumed by toca-deal.
// SchemaVersion versions the JSON shape, while Revision is incremented for
// every tenant change so caches and audit records can identify the exact rule
// used for a decision.
type ApprovalPolicy struct {
	SchemaVersion      int       `json:"schema_version"`
	Revision           int64     `json:"revision"`
	Enabled            bool      `json:"enabled"`
	ThresholdBRL       float64   `json:"threshold_brl"`
	TargetStatuses     []string  `json:"target_statuses"`
	RequiredPermission string    `json:"required_permission"`
	UpdatedAt          time.Time `json:"updated_at"`
	UpdatedBy          string    `json:"updated_by,omitempty"`
	Source             string    `json:"source"`
}

func DefaultApprovalPolicy() ApprovalPolicy {
	return ApprovalPolicy{
		SchemaVersion:      ApprovalPolicySchemaVersion,
		Revision:           0,
		Enabled:            true,
		ThresholdBRL:       DefaultApprovalThresholdBRL,
		TargetStatuses:     slices.Clone(DefaultApprovalTargetStatuses),
		RequiredPermission: ApprovalPermission,
		Source:             "platform_default",
	}
}

// Validate rejects policy shapes that toca-deal cannot safely interpret.
// In particular, permission and target stages are contract constants in v1;
// tenants can tune the monetary threshold or explicitly disable the rule.
func (p ApprovalPolicy) Validate() error {
	if p.SchemaVersion != ApprovalPolicySchemaVersion {
		return fmt.Errorf("unsupported approval policy schema_version %d", p.SchemaVersion)
	}
	if p.Revision < 0 {
		return errors.New("approval policy revision cannot be negative")
	}
	if math.IsNaN(p.ThresholdBRL) || math.IsInf(p.ThresholdBRL, 0) || p.ThresholdBRL < 0 || p.ThresholdBRL > MaxApprovalThresholdBRL {
		return fmt.Errorf("approval threshold must be between 0 and %.0f BRL", MaxApprovalThresholdBRL)
	}
	if p.Enabled && p.ThresholdBRL <= 0 {
		return errors.New("enabled approval policy requires a positive threshold_brl")
	}
	if p.RequiredPermission != ApprovalPermission {
		return fmt.Errorf("approval policy required_permission must be %q", ApprovalPermission)
	}
	if !slices.Equal(p.TargetStatuses, DefaultApprovalTargetStatuses) {
		return fmt.Errorf("approval policy target_statuses must be %v for schema v1", DefaultApprovalTargetStatuses)
	}
	return nil
}
