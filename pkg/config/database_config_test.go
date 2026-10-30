package config

import "testing"

func TestResolveServiceURI(t *testing.T) {
	tests := []struct {
		name       string
		appEnv     string
		appMode    string
		configured string
		want       string
		wantErr    bool
	}{
		{name: "development may share pool", appEnv: "development", configured: "", want: "tenant"},
		{name: "test may share pool", appEnv: "test", configured: "tenant", want: "tenant"},
		{name: "production requires explicit URI", appEnv: "production", configured: "", wantErr: true},
		{name: "release mode requires explicit URI", appEnv: "development", appMode: "release", configured: "", wantErr: true},
		{name: "production rejects same credential", appEnv: "production", configured: "tenant", wantErr: true},
		{name: "production accepts distinct service credential", appEnv: "production", configured: "service", want: "service"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveServiceURI(tt.appEnv, tt.appMode, "tenant", tt.configured)
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveServiceURI() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("resolveServiceURI() = %q, want %q", got, tt.want)
			}
		})
	}
}
