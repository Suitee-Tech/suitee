package database

import (
	"testing"
)

// M1 — o retrato de RLS dos dois pools precisa virar log/erro determinístico.
// A decisão é pura justamente para poder ser testada sem um banco com roles
// especiais; a coleta das capacidades é exercitada pelos testes de integração
// (`-tags=integration`, pkg/database/rls_test.go).
func TestEvaluateRLSBackstop(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		backstop  RLSBackstop
		wantEvent string
		wantLevel string
		wantFatal bool
	}{
		{
			name:      "pool de aplicação sujeito a RLS",
			backstop:  RLSBackstop{UserRoleBypassesRLS: false, ServiceRoleBypassesRLS: true, DedicatedServiceRole: true},
			wantEvent: "database.rls_backstop_active",
			wantLevel: "info",
		},
		{
			name:      "release com backstop desligado é erro (fail-loud, não fatal por padrão)",
			env:       map[string]string{"APP_ENV": "production", "APP_MODE": "release"},
			backstop:  RLSBackstop{UserRoleBypassesRLS: true, ServiceRoleBypassesRLS: true, DedicatedServiceRole: true},
			wantEvent: "database.rls_backstop_disabled",
			wantLevel: "error",
		},
		{
			name:      "fora de release o mesmo estado é aviso",
			env:       map[string]string{"APP_ENV": "development", "APP_MODE": "debug"},
			backstop:  RLSBackstop{UserRoleBypassesRLS: true, ServiceRoleBypassesRLS: true},
			wantEvent: "database.rls_backstop_disabled",
			wantLevel: "warn",
		},
		{
			name: "REQUIRE_RLS_BACKSTOP=true torna o boot fatal em release",
			env: map[string]string{
				"APP_ENV": "production", "APP_MODE": "release", "REQUIRE_RLS_BACKSTOP": "true",
			},
			backstop:  RLSBackstop{UserRoleBypassesRLS: true, ServiceRoleBypassesRLS: true},
			wantEvent: "database.rls_backstop_required",
			wantLevel: "error",
			wantFatal: true,
		},
		{
			name: "REQUIRE_RLS_BACKSTOP fora de release não derruba o boot local",
			env: map[string]string{
				"APP_ENV": "development", "REQUIRE_RLS_BACKSTOP": "true",
			},
			backstop:  RLSBackstop{UserRoleBypassesRLS: true},
			wantEvent: "database.rls_backstop_disabled",
			wantLevel: "warn",
		},
		{
			name: "backstop ativo com REQUIRE_RLS_BACKSTOP=true segue verde",
			env: map[string]string{
				"APP_ENV": "production", "APP_MODE": "release", "REQUIRE_RLS_BACKSTOP": "true",
			},
			backstop:  RLSBackstop{UserRoleBypassesRLS: false, ServiceRoleBypassesRLS: true, DedicatedServiceRole: true},
			wantEvent: "database.rls_backstop_active",
			wantLevel: "info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Ambiente limpo por caso: as duas chaves sempre definidas evita
			// herdar APP_ENV/APP_MODE do processo de teste.
			t.Setenv("APP_ENV", "")
			t.Setenv("APP_MODE", "")
			t.Setenv("REQUIRE_RLS_BACKSTOP", "")
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			decision := EvaluateRLSBackstop(tt.backstop)
			if decision.Event != tt.wantEvent {
				t.Fatalf("Event = %q, want %q", decision.Event, tt.wantEvent)
			}
			if decision.Level != tt.wantLevel {
				t.Fatalf("Level = %q, want %q", decision.Level, tt.wantLevel)
			}
			if decision.Fatal != tt.wantFatal {
				t.Fatalf("Fatal = %v, want %v", decision.Fatal, tt.wantFatal)
			}
			if decision.Message == "" {
				t.Fatal("mensagem vazia: o log precisa dizer o que fazer")
			}
		})
	}
}

func TestRLSBackstopActive(t *testing.T) {
	if !(RLSBackstop{}).Active() {
		t.Fatal("pool de aplicação sem BYPASSRLS deveria ter o backstop ativo")
	}
	if (RLSBackstop{UserRoleBypassesRLS: true}).Active() {
		t.Fatal("pool de aplicação com BYPASSRLS não pode ser considerado ativo")
	}
}
