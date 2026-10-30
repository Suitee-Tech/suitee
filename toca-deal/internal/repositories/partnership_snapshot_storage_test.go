package repositories

import (
	"encoding/json"
	access "github.com/oginoino/toca/pkg/partnershipaccess"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	"testing"
)

func TestPartnershipSnapshotIsPersistedButNotExposedInDealJSON(t *testing.T) {
	original := &deal.Deal{ID: "deal", TenantID: "closing", PartnershipSnapshot: &access.Snapshot{ID: "agreement", Version: 3, FinancialConfig: json.RawMessage(`{"admin_fee":123}`)}}
	stored, err := marshalStoredDeal(original)
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(stored, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["partnership_snapshot"] == nil || metadata["id"] != "deal" {
		t.Fatal("missing storage fields", string(stored))
	}
	var loaded deal.Deal
	if err := unmarshalStoredDeal(stored, &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.PartnershipSnapshot == nil || loaded.PartnershipSnapshot.Version != 3 {
		t.Fatal("snapshot lost on load")
	}
	loaded.Status = deal.Status("NEGOTIATION")
	updated, err := marshalStoredDeal(&loaded)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded deal.Deal
	if err := unmarshalStoredDeal(updated, &reloaded); err != nil || reloaded.PartnershipSnapshot == nil {
		t.Fatal("snapshot lost on update", err)
	}
	public, err := json.Marshal(&loaded)
	if err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	json.Unmarshal(public, &response)
	if _, ok := response["partnership_snapshot"]; ok {
		t.Fatal("financial snapshot exposed to generic deal reader")
	}
}
