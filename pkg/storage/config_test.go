package storage

import "testing"

func TestNewFromEnvFailsClosedForInvalidProviderConfiguration(t *testing.T) {
	t.Run("supabase credentials are required", func(t *testing.T) {
		t.Setenv("STORAGE_PROVIDER", "supabase")
		t.Setenv("SUPABASE_URL", "")
		t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "")
		if _, err := NewFromEnv(); err == nil {
			t.Fatal("NewFromEnv() should reject incomplete Supabase configuration")
		}
	})

	t.Run("unknown provider is rejected", func(t *testing.T) {
		t.Setenv("STORAGE_PROVIDER", "unknown")
		if _, err := NewFromEnv(); err == nil {
			t.Fatal("NewFromEnv() should reject an unsupported provider")
		}
	})
}

func TestLocalUploadsURLUsesActiveEnvironment(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantURL string
	}{
		{
			name:    "explicit uploads URL",
			env:     map[string]string{"PUBLIC_UPLOADS_URL": "https://beta.example/uploads"},
			wantURL: "https://beta.example/uploads",
		},
		{
			name:    "gateway URL",
			env:     map[string]string{"PUBLIC_GATEWAY_URL": "https://beta.example"},
			wantURL: "https://beta.example/uploads",
		},
		{
			name:    "API URL",
			env:     map[string]string{"PUBLIC_API_URL": "https://beta.example/api/v1"},
			wantURL: "https://beta.example/uploads",
		},
		{
			name:    "local default",
			env:     map[string]string{},
			wantURL: "http://localhost:8080/uploads",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PUBLIC_UPLOADS_URL", "")
			t.Setenv("PUBLIC_GATEWAY_URL", "")
			t.Setenv("PUBLIC_API_URL", "")
			for key, value := range tt.env {
				t.Setenv(key, value)
			}
			if got := localUploadsURL(); got != tt.wantURL {
				t.Fatalf("localUploadsURL() = %q, want %q", got, tt.wantURL)
			}
		})
	}
}
