package models

import (
	"time"

	"github.com/oginoino/toca-core/internal/domain/core/partnership"
)

type PartnershipModel struct {
	ID              string                        `bson:"_id,omitempty"`
	RequesterID     string                        `bson:"requester_id"`
	TargetID        string                        `bson:"target_id"`
	Type            partnership.PartnershipType   `bson:"type"`
	Status          partnership.PartnershipStatus `bson:"status"`
	FinancialConfig partnership.FinancialConfig   `bson:"financial_config"`
	Permissions     []partnership.Permission      `bson:"permissions"`
	AuditLogs       []partnership.AuditLog        `bson:"audit_logs,omitempty"`
	CreatedAt       time.Time                     `bson:"created_at"`
	UpdatedAt       time.Time                     `bson:"updated_at"`
}
