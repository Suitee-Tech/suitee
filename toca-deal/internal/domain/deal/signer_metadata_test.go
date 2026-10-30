package deal_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// TestSigner_MetadataFieldsRoundTrip pins the JSON contract for the
// Phase 3/S8 forensic signing metadata. Production reads / writes of
// `Signer` go through JSON (REST API + JSONB column in Postgres) so a
// silent rename breaks both the FE and the toca-signature subscriber
// that populates these fields.
func TestSigner_MetadataFieldsRoundTrip(t *testing.T) {
	s := deal.Signer{
		ID:                "signer-1",
		Name:              "Maria",
		Email:             "maria@example.com",
		SignedIP:          "203.0.113.5",
		AuthMethodUsed:    "icp_brasil",
		CertificateSerial: "A1B2C3D4E5F6",
		WidgetEventLogURL: "https://clicksign.invalid/widget-events/abc",
		AuthEvidenceURL:   "https://clicksign.invalid/evidence/abc",
	}

	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	expected := []string{
		`"signed_ip":"203.0.113.5"`,
		`"auth_method_used":"icp_brasil"`,
		`"certificate_serial":"A1B2C3D4E5F6"`,
		`"widget_event_log_url":"https://clicksign.invalid/widget-events/abc"`,
		`"auth_evidence_url":"https://clicksign.invalid/evidence/abc"`,
	}
	for _, want := range expected {
		if !strings.Contains(string(raw), want) {
			t.Errorf("expected JSON to contain %s, got %s", want, raw)
		}
	}

	var back deal.Signer
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.SignedIP != s.SignedIP ||
		back.AuthMethodUsed != s.AuthMethodUsed ||
		back.CertificateSerial != s.CertificateSerial ||
		back.WidgetEventLogURL != s.WidgetEventLogURL ||
		back.AuthEvidenceURL != s.AuthEvidenceURL {
		t.Errorf("round-trip lost data: got %+v want %+v", back, s)
	}
}

// TestSigner_MetadataFieldsAreOmitemptyWhenEmpty pins the wire
// behaviour: legacy signers (created before this struct grew) must not
// emit empty placeholder strings, so old consumers parsing the JSON
// don't suddenly see new fields with empty values.
func TestSigner_MetadataFieldsAreOmitemptyWhenEmpty(t *testing.T) {
	s := deal.Signer{ID: "signer-2", Name: "João", Email: "j@x.com"}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	disallowed := []string{
		"signed_ip",
		"auth_method_used",
		"certificate_serial",
		"widget_event_log_url",
		"auth_evidence_url",
	}
	for _, key := range disallowed {
		if strings.Contains(string(raw), key) {
			t.Errorf("empty signer must omit %q field, got %s", key, raw)
		}
	}
}
