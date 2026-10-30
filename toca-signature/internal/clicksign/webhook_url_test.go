package clicksign

import "testing"

func TestPublicWebhookURL(t *testing.T) {
	cases := []struct {
		name     string
		gateway  string
		override string
		want     string
	}{
		{name: "from gateway url", gateway: "https://abc.ngrok-free.app", want: "https://abc.ngrok-free.app/webhooks/clicksign"},
		{name: "trims trailing slash", gateway: "https://api.example.com/", want: "https://api.example.com/webhooks/clicksign"},
		{name: "override beats gateway", gateway: "https://gateway.example", override: "https://cdn.example", want: "https://cdn.example/webhooks/clicksign"},
		{name: "empty when neither set", want: ""},
		{name: "empty gateway falls back to nothing", gateway: "  ", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PUBLIC_GATEWAY_URL", tc.gateway)
			t.Setenv("WEBHOOK_PUBLIC_URL", tc.override)
			got := PublicWebhookURL()
			if got != tc.want {
				t.Errorf("PublicWebhookURL() = %q, want %q", got, tc.want)
			}
		})
	}
}
