package contracts

import (
	"encoding/json"
	"sort"
	"testing"
)

// Contract shape tests — Onda 5.2.
//
// These tests assert the wire shape (JSON field names + nesting) of
// every event type that crosses a service boundary. If a publisher
// renames a field without coordination, the test that the consumer
// owns will fail loudly — preventing the silent-break class of bugs
// where one side keeps working but the other quietly stops getting
// the data it expects.
//
// Pattern: marshal a populated zero-equivalent struct, then assert
// the resulting JSON has exactly the expected top-level keys. We
// don't pin the values — only the keyset — because that's the
// contract surface a downstream actually cares about.
//
// When the contract DOES change intentionally:
//   1. Bump a version field (or introduce a parallel _v2 event)
//   2. Update the expected keyset here
//   3. Update consumer(s) in lockstep
//   4. Land both PRs together; never as separate deploys

func topLevelKeys(t *testing.T, v interface{}) []string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func assertKeys(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("key count: got=%v want=%v", got, want)
	}
	for i, g := range got {
		if g != want[i] {
			t.Fatalf("key[%d]: got=%q want=%q (all got=%v)", i, g, want[i], got)
		}
	}
}

func TestSignatureRequestedEventPayloadShape(t *testing.T) {
	// Populate every optional field so omitempty doesn't drop them from
	// the snapshot — we want the FULL contract shape, not the
	// minimum-required subset.
	p := SignatureRequestedEventPayload{
		TenantID:    "t",
		ContractID:  "c",
		DealID:      "d",
		DisplayID:   "x",
		Signers:     []SignatureRequestedSigner{},
		DocumentURL: "u",
		Locale:      "pt-BR",
		Deadline:    "2026-01-01",
		Timestamp:   "now",
	}
	want := []string{
		"contract_id", "deadline", "deal_id", "display_id",
		"document_url", "locale", "signers", "tenant_id", "timestamp",
	}
	assertKeys(t, topLevelKeys(t, p), want)
}

func TestSignatureSignerSignedEventPayloadShape(t *testing.T) {
	p := SignatureSignerSignedEventPayload{
		TenantID:           "t",
		ContractID:         "c",
		EnvelopeID:         "e",
		ExternalEnvelopeID: "ee",
		SignerID:           "s",
		SignerEmail:        "x@y.io",
		SignedAt:           "now",
		EvidenceURL:        "u",
		Timestamp:          "now",
	}
	want := []string{
		"contract_id", "envelope_id", "evidence_url", "external_envelope_id",
		"signed_at", "signer_email", "signer_id", "tenant_id", "timestamp",
	}
	assertKeys(t, topLevelKeys(t, p), want)
}

func TestSignatureEnvelopeFinishedEventPayloadShape(t *testing.T) {
	p := SignatureEnvelopeFinishedEventPayload{
		TenantID:           "t",
		ContractID:         "c",
		EnvelopeID:         "e",
		ExternalEnvelopeID: "ee",
		FinishedAt:         "now",
		SignedDocumentURL:  "u",
		Timestamp:          "now",
	}
	want := []string{
		"contract_id", "envelope_id", "external_envelope_id",
		"finished_at", "signed_document_url", "tenant_id", "timestamp",
	}
	assertKeys(t, topLevelKeys(t, p), want)
}

func TestSignatureRefusedEventPayloadShape(t *testing.T) {
	p := SignatureRefusedEventPayload{
		TenantID:           "t",
		ContractID:         "c",
		EnvelopeID:         "e",
		ExternalEnvelopeID: "ee",
		SignerID:           "s",
		Reason:             "reason",
		RefusedAt:          "now",
		Timestamp:          "now",
	}
	want := []string{
		"contract_id", "envelope_id", "external_envelope_id",
		"reason", "refused_at", "signer_id", "tenant_id", "timestamp",
	}
	assertKeys(t, topLevelKeys(t, p), want)
}

func TestRentalCreatedEventPayloadShape(t *testing.T) {
	// toca-financial publishes this; consumers are toca-comm and analytics.
	p := RentalCreatedEventPayload{
		RentalID:     "r",
		TenantID:     "t",
		DealID:       "d",
		ContractID:   "c",
		PropertyID:   "p",
		TenantLeadID: "tl",
		OwnerLeadID:  "ol",
		BrokerID:     "b",
		Timestamp:    "now",
	}
	want := []string{
		"broker_id", "contract_id", "deal_id", "owner_lead_id",
		"property_id", "rental_id", "tenant_id", "tenant_lead_id", "timestamp",
	}
	assertKeys(t, topLevelKeys(t, p), want)
}

func TestContractSignedEventPayloadShape(t *testing.T) {
	// Critical event — toca-financial creates Rental from this. Any
	// field rename would break the entire post-signature pipeline.
	p := ContractSignedEventPayload{
		DealID:        "d",
		ContractID:    "c",
		TenantID:      "t",
		SignedAt:      "now",
		StartDate:     "2026-01-01",
		EndDate:       "2027-01-01",
		Value:         1000,
		CommissionPct: 6,
		Type:          "rent",
		CustomerID:    "cu",
		OwnerID:       "o",
		PropertyID:    "p",
	}
	want := []string{
		"broker_participations", "commission_pct", "contract_id", "customer_id", "deal_id",
		"end_date", "owner_id", "property_id", "signed_at", "start_date",
		"tenant_id", "type", "value",
	}
	assertKeys(t, topLevelKeys(t, p), want)
}

func TestProposalSentEventPayloadShape(t *testing.T) {
	p := ProposalSentEventPayload{
		DealID:             "d",
		TenantID:           "t",
		ProposalID:         "pr",
		Value:              500,
		CustomerAccountIDs: []string{"a", "b"},
		CustomerEmails:     []string{"cliente@example.com"},
	}
	want := []string{"customer_account_ids", "customer_emails", "deal_id", "proposal_id", "tenant_id", "value"}
	assertKeys(t, topLevelKeys(t, p), want)
}

func TestPartnerDealInitiatedEventPayloadShape(t *testing.T) {
	// toca-deal publishes this when a co-brokering deal is started against a
	// partner-shared resource; toca-comm consumes it to notify the owner
	// company. A field rename would silently drop the notification.
	p := PartnerDealInitiatedEventPayload{
		DealID:            "d",
		ResourceType:      "property",
		ResourceID:        "r",
		InitiatorTenantID: "i",
		OwnerTenantID:     "o",
		CreatedAt:         "now",
	}
	want := []string{
		"created_at", "deal_id", "initiator_tenant_id",
		"owner_tenant_id", "resource_id", "resource_type",
	}
	assertKeys(t, topLevelKeys(t, p), want)
}

func TestLeadUpdatedEventPayloadShape(t *testing.T) {
	// toca-deal subscribes to refresh Contract.PropertyTitle and party
	// snapshots. Adding a field here without coordination silently
	// drops it in the projection refresh.
	p := LeadUpdatedEventPayload{
		LeadID:    "l",
		TenantID:  "t",
		Name:      "n",
		Email:     "e",
		Phone:     "p",
		Document:  "d",
		Role:      "r",
		UpdatedAt: "now",
	}
	want := []string{
		"document", "email", "lead_id", "name",
		"phone", "role", "tenant_id", "updated_at",
	}
	assertKeys(t, topLevelKeys(t, p), want)
}
