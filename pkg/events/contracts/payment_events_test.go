package contracts

import (
	"encoding/json"
	"testing"
)

func TestChargePaidEventContractHasNoSyntheticInvoiceID(t *testing.T) {
	event := ChargePaidEvent{Data: ChargePaidEventPayload{
		ChargeID: "charge-1", TenantID: "tenant-1", ContractID: "contract-1",
		Amount: 100, Currency: "BRL", PaidAt: "2026-08-13T12:00:00Z",
	}}
	payload, err := json.Marshal(event.Payload())
	if err != nil {
		t.Fatalf("marshal charge.paid payload: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode charge.paid payload: %v", err)
	}
	if event.Name() != "charge.paid" || decoded["charge_id"] != "charge-1" {
		t.Fatalf("unexpected charge.paid contract: name=%s payload=%v", event.Name(), decoded)
	}
	if _, exists := decoded["invoice_id"]; exists {
		t.Fatalf("charge.paid fabricated invoice_id: %v", decoded)
	}
}

func TestInvoicePaidEventContractCarriesRealInvoiceAndChargeIDs(t *testing.T) {
	event := InvoicePaidEvent{Data: InvoicePaidEventPayload{
		SchemaVersion: 2,
		InvoiceID:     "invoice-1", ChargeID: "charge-1", TenantID: "tenant-1",
		ReferenceID: "contract-1", Amount: 40, Currency: "BRL", PaidAt: "2026-08-13T12:00:00Z",
	}}
	payload, err := json.Marshal(event.Payload())
	if err != nil {
		t.Fatalf("marshal invoice.paid payload: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode invoice.paid payload: %v", err)
	}
	if event.Name() != "invoice.paid" || decoded["schema_version"] != float64(2) || decoded["invoice_id"] != "invoice-1" || decoded["charge_id"] != "charge-1" || decoded["amount"] != float64(40) {
		t.Fatalf("unexpected invoice.paid contract: name=%s payload=%v", event.Name(), decoded)
	}
}
