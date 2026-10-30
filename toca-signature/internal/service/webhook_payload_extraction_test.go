package service

import (
	"testing"

	"github.com/oginoino/toca/toca-signature/internal/clicksign"
)

func TestExtractExternalEnvelopeID(t *testing.T) {
	cases := []struct {
		name    string
		payload clicksign.WebhookPayload
		want    string
	}{
		{
			name: "v1 shape: envelope.id present",
			payload: clicksign.WebhookPayload{
				Envelope: &clicksign.WebhookEnvelope{ID: "env-from-envelope"},
			},
			want: "env-from-envelope",
		},
		{
			name: "v3 shape: only document.key present",
			payload: clicksign.WebhookPayload{
				Document: &clicksign.WebhookDocument{Key: "env-from-document"},
			},
			want: "env-from-document",
		},
		{
			name: "envelope wins over document if both set",
			payload: clicksign.WebhookPayload{
				Envelope: &clicksign.WebhookEnvelope{ID: "from-envelope"},
				Document: &clicksign.WebhookDocument{Key: "from-document"},
			},
			want: "from-envelope",
		},
		{
			name:    "neither present → empty",
			payload: clicksign.WebhookPayload{},
			want:    "",
		},
		{
			name: "envelope present but ID empty → fall back to document",
			payload: clicksign.WebhookPayload{
				Envelope: &clicksign.WebhookEnvelope{ID: ""},
				Document: &clicksign.WebhookDocument{Key: "fallback"},
			},
			want: "fallback",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractExternalEnvelopeID(c.payload)
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestNormalizeEventName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"sign", "sign"},
		{"auto_close", "auto_close"},
		{"Event::Sign", "sign"},
		{"Event::AutoClose", "auto_close"},
		{"Event::DocumentClosed", "document_closed"},
		{"Event::SignatureStarted", "signature_started"},
		{"Event::AddSigner", "add_signer"},
		{"", ""},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got := normalizeEventName(c.in)
			if got != c.want {
				t.Errorf("normalizeEventName(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
