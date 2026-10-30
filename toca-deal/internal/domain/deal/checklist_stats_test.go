package deal_test

import (
	"testing"

	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

func TestComputeChecklistStats_EmptyChecklist(t *testing.T) {
	d := &deal.Deal{}
	stats := d.ComputeChecklistStats()
	if stats.Total != 0 {
		t.Errorf("Total = %d, want 0", stats.Total)
	}
	if !stats.RequiredComplete {
		t.Errorf("RequiredComplete = false, want true (vacuous truth when nothing required)")
	}
}

func TestComputeChecklistStats_MixedStatuses(t *testing.T) {
	d := &deal.Deal{
		Checklist: []deal.ChecklistItem{
			{Name: "RG", Required: true, Status: deal.ChecklistApproved},
			{Name: "CNH", Required: true, Status: deal.ChecklistPending},
			{Name: "Comprovante de residência", Required: true, Status: deal.ChecklistUploaded},
			{Name: "Matrícula", Required: true, Status: deal.ChecklistRejected},
			{Name: "Foto", Required: false, Status: deal.ChecklistApproved},
		},
	}
	stats := d.ComputeChecklistStats()
	cases := []struct {
		name string
		got  int
		want int
	}{
		{"Total", stats.Total, 5},
		{"Required", stats.Required, 4},
		{"Approved", stats.Approved, 2},                 // RG + Foto
		{"RequiredApproved", stats.RequiredApproved, 1}, // only RG
		{"Pending", stats.Pending, 1},
		{"Uploaded", stats.Uploaded, 1},
		{"Rejected", stats.Rejected, 1},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s = %d, want %d", tc.name, tc.got, tc.want)
		}
	}
	if stats.RequiredComplete {
		t.Errorf("RequiredComplete = true, want false (only 1/4 required approved)")
	}
}

func TestComputeChecklistStats_AllRequiredApproved(t *testing.T) {
	d := &deal.Deal{
		Checklist: []deal.ChecklistItem{
			{Required: true, Status: deal.ChecklistApproved},
			{Required: true, Status: deal.ChecklistApproved},
			{Required: false, Status: deal.ChecklistPending}, // optional, still pending
		},
	}
	stats := d.ComputeChecklistStats()
	if !stats.RequiredComplete {
		t.Errorf("RequiredComplete = false, want true (all required approved; " +
			"optional pending should not block)")
	}
	if stats.RequiredApproved != 2 {
		t.Errorf("RequiredApproved = %d, want 2", stats.RequiredApproved)
	}
}

func TestRequiredChecklistComplete(t *testing.T) {
	tests := []struct {
		name  string
		items []deal.ChecklistItem
		want  bool
	}{
		{"empty checklist is vacuously complete", nil, true},
		{
			"all required approved",
			[]deal.ChecklistItem{
				{Required: true, Status: deal.ChecklistApproved},
				{Required: true, Status: deal.ChecklistApproved},
			},
			true,
		},
		{
			"one required uploaded but not yet approved",
			[]deal.ChecklistItem{
				{Required: true, Status: deal.ChecklistApproved},
				{Required: true, Status: deal.ChecklistUploaded},
			},
			false,
		},
		{
			"required rejected blocks completion",
			[]deal.ChecklistItem{
				{Required: true, Status: deal.ChecklistRejected},
			},
			false,
		},
		{
			"optional pending does not block",
			[]deal.ChecklistItem{
				{Required: true, Status: deal.ChecklistApproved},
				{Required: false, Status: deal.ChecklistPending},
			},
			true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := &deal.Deal{Checklist: tc.items}
			if got := d.RequiredChecklistComplete(); got != tc.want {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestPopulateChecklistStats_SetsField(t *testing.T) {
	d := &deal.Deal{
		Checklist: []deal.ChecklistItem{
			{Required: true, Status: deal.ChecklistApproved},
		},
	}
	if d.ChecklistStats != nil {
		t.Fatalf("expected ChecklistStats nil before populate, got %+v", d.ChecklistStats)
	}
	d.PopulateChecklistStats()
	if d.ChecklistStats == nil {
		t.Fatal("expected ChecklistStats populated")
	}
	if d.ChecklistStats.Total != 1 || !d.ChecklistStats.RequiredComplete {
		t.Errorf("unexpected stats: %+v", d.ChecklistStats)
	}
}
