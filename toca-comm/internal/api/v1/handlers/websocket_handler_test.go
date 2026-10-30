package handlers

import (
	"net/http"
	"testing"
)

func TestBuildUpgrader_CheckOrigin(t *testing.T) {
	tests := []struct {
		name           string
		allowedOrigins []string
		requestOrigin  string
		wantAllowed    bool
	}{
		{
			name:           "empty list allows all (dev mode)",
			allowedOrigins: []string{},
			requestOrigin:  "https://evil.com",
			wantAllowed:    true,
		},
		{
			name:           "wildcard allows all",
			allowedOrigins: []string{"*"},
			requestOrigin:  "https://evil.com",
			wantAllowed:    true,
		},
		{
			name:           "wildcard mixed with explicit still allows all",
			allowedOrigins: []string{"https://app.example.com", "*"},
			requestOrigin:  "https://random.example",
			wantAllowed:    true,
		},
		{
			name:           "allows configured origin",
			allowedOrigins: []string{"https://app.example.com", "https://portal.example.com"},
			requestOrigin:  "https://app.example.com",
			wantAllowed:    true,
		},
		{
			name:           "rejects unconfigured origin",
			allowedOrigins: []string{"https://app.example.com"},
			requestOrigin:  "https://evil.com",
			wantAllowed:    false,
		},
		{
			name:           "rejects subdomain not in list",
			allowedOrigins: []string{"https://app.example.com"},
			requestOrigin:  "https://other.suitee.com.br",
			wantAllowed:    false,
		},
		{
			name:           "empty Origin header allowed (non-browser client)",
			allowedOrigins: []string{"https://app.example.com"},
			requestOrigin:  "",
			wantAllowed:    true,
		},
		{
			name:           "case-sensitive match (strict)",
			allowedOrigins: []string{"https://app.example.com"},
			requestOrigin:  "https://app.example.com",
			wantAllowed:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up := buildUpgrader(tt.allowedOrigins)
			req := &http.Request{Header: http.Header{}}
			if tt.requestOrigin != "" {
				req.Header.Set("Origin", tt.requestOrigin)
			}
			got := up.CheckOrigin(req)
			if got != tt.wantAllowed {
				t.Errorf("CheckOrigin(origin=%q, allowed=%v) = %v, want %v",
					tt.requestOrigin, tt.allowedOrigins, got, tt.wantAllowed)
			}
		})
	}
}
