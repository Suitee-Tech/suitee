package handlers

import (
	"testing"

	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
)

// TestCanOwnerAccessTransfer cobre o guard LGPD que evita vazamento de
// transfers entre owners do mesmo tenant. Regressão direta de #455/#465.
func TestCanOwnerAccessTransfer(t *testing.T) {
	tests := []struct {
		name           string
		transfer       *transfer.OwnerTransfer
		accountLeadIDs []string
		want           bool
	}{
		{
			name:           "owner can access own transfer",
			transfer:       &transfer.OwnerTransfer{OwnerID: "lead-a"},
			accountLeadIDs: []string{"lead-a", "lead-b"},
			want:           true,
		},
		{
			name:           "owner CANNOT access other owner's transfer (PRIVACY)",
			transfer:       &transfer.OwnerTransfer{OwnerID: "lead-c"},
			accountLeadIDs: []string{"lead-a", "lead-b"},
			want:           false,
		},
		{
			name:           "account with multiple leads — match any one",
			transfer:       &transfer.OwnerTransfer{OwnerID: "lead-b"},
			accountLeadIDs: []string{"lead-a", "lead-b", "lead-c"},
			want:           true,
		},
		{
			name:           "empty lead set always denies",
			transfer:       &transfer.OwnerTransfer{OwnerID: "lead-a"},
			accountLeadIDs: []string{},
			want:           false,
		},
		{
			name:           "nil lead set always denies",
			transfer:       &transfer.OwnerTransfer{OwnerID: "lead-a"},
			accountLeadIDs: nil,
			want:           false,
		},
		{
			name:           "nil transfer denies (defensive)",
			transfer:       nil,
			accountLeadIDs: []string{"lead-a"},
			want:           false,
		},
		{
			name:           "transfer without owner_id denies (data integrity)",
			transfer:       &transfer.OwnerTransfer{OwnerID: ""},
			accountLeadIDs: []string{"lead-a"},
			want:           false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canOwnerAccessTransfer(tt.transfer, tt.accountLeadIDs)
			if got != tt.want {
				t.Errorf("canOwnerAccessTransfer(owner=%q, leads=%v) = %v, want %v",
					ownerID(tt.transfer), tt.accountLeadIDs, got, tt.want)
			}
		})
	}
}

func ownerID(t *transfer.OwnerTransfer) string {
	if t == nil {
		return "<nil>"
	}
	return t.OwnerID
}
