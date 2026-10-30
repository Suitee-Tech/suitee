package handlers

import (
	"testing"

	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
)

// TestCanOwnerAccessCharge cobre o guard LGPD que decide se uma charge é
// visível ao caller. Análogo ao test de TestCanOwnerAccessTransfer (#465)
// — função pura, fácil de manter e cobre todos os edge cases.
func TestCanOwnerAccessCharge(t *testing.T) {
	tests := []struct {
		name           string
		charge         *charge.Charge
		accountLeadIDs []string
		want           bool
	}{
		{
			name:           "tenant lead matches lead_id",
			charge:         &charge.Charge{LeadID: "lead-tenant-1", OwnerID: "lead-owner-1"},
			accountLeadIDs: []string{"lead-tenant-1"},
			want:           true,
		},
		{
			name:           "owner lead matches owner_id",
			charge:         &charge.Charge{LeadID: "lead-tenant-1", OwnerID: "lead-owner-1"},
			accountLeadIDs: []string{"lead-owner-1"},
			want:           true,
		},
		{
			name:           "no overlap — third-party DENIED (LGPD)",
			charge:         &charge.Charge{LeadID: "lead-tenant-1", OwnerID: "lead-owner-1"},
			accountLeadIDs: []string{"lead-tenant-2", "lead-owner-2"},
			want:           false,
		},
		{
			name:           "account with multiple leads — one matches owner",
			charge:         &charge.Charge{LeadID: "lead-tenant-X", OwnerID: "lead-owner-A"},
			accountLeadIDs: []string{"lead-owner-A", "lead-owner-B", "lead-tenant-A"},
			want:           true,
		},
		{
			name:           "empty leads always denies",
			charge:         &charge.Charge{LeadID: "lead-tenant-1"},
			accountLeadIDs: []string{},
			want:           false,
		},
		{
			name:           "nil leads denies",
			charge:         &charge.Charge{LeadID: "lead-tenant-1"},
			accountLeadIDs: nil,
			want:           false,
		},
		{
			name:           "nil charge denies (defensive)",
			charge:         nil,
			accountLeadIDs: []string{"lead-tenant-1"},
			want:           false,
		},
		{
			name:           "empty string in leadIDs ignored — no false positive against empty fields",
			charge:         &charge.Charge{LeadID: "", OwnerID: ""},
			accountLeadIDs: []string{""},
			want:           false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canOwnerAccessCharge(tt.charge, tt.accountLeadIDs)
			if got != tt.want {
				t.Errorf("canOwnerAccessCharge(%+v, %v) = %v, want %v",
					tt.charge, tt.accountLeadIDs, got, tt.want)
			}
		})
	}
}
