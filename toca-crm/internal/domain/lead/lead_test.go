package lead

import (
	"strings"
	"testing"
)

func TestNewLeadWithOptionalTenant(t *testing.T) {
	t.Run("Should create lead with valid tenant ID (UUID)", func(t *testing.T) {
		tenantID := "123e4567-e89b-12d3-a456-426614174000"
		lead, err := NewLeadWithOptionalTenant("John Doe", "john@example.com", "1234567890", "", LeadSourceWebsite, LeadStatusNew, nil, "", tenantID)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if lead == nil {
			t.Fatal("Expected lead to be created")
		}
		if lead.TenantID != tenantID {
			t.Errorf("Expected tenant ID %s, got %s", tenantID, lead.TenantID)
		}
	})

	t.Run("Should create lead with valid tenant ID (Non-UUID)", func(t *testing.T) {
		tenantID := "custom-id-123"
		lead, err := NewLeadWithOptionalTenant("Jane Doe", "jane@example.com", "0987654321", "", LeadSourceWebsite, LeadStatusNew, nil, "", tenantID)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if lead == nil {
			t.Fatal("Expected lead to be created")
		}
		if lead.TenantID != tenantID {
			t.Errorf("Expected tenant ID %s, got %s", tenantID, lead.TenantID)
		}
	})

	t.Run("Should create lead with empty tenant ID", func(t *testing.T) {
		lead, err := NewLeadWithOptionalTenant("Bob Smith", "bob@example.com", "1122334455", "", LeadSourceWebsite, LeadStatusNew, nil, "", "")

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if lead == nil {
			t.Fatal("Expected lead to be created")
		}
		if lead.TenantID != "" {
			t.Errorf("Expected empty tenant ID, got %s", lead.TenantID)
		}
	})

	t.Run("Should validate required fields", func(t *testing.T) {
		_, err := NewLeadWithOptionalTenant("", "", "", "", LeadSourceWebsite, LeadStatusNew, nil, "", "")
		if err == nil {
			t.Error("Expected error for missing required fields")
		}
	})
}

// O lead alimenta a parte do negócio: a regra de nome completo vale já no
// cadastro, senão o operador só descobre ao iniciar o negócio na esteira
// ("Nome do signatário precisa incluir sobrenome"). Mesma regra de
// `pkg/validation/person_name.go`.
func TestNewLeadRequiresFirstNameAndSurname(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr string // vazio = deve passar
	}{
		{"nome e sobrenome", "Maria Souza", ""},
		{"nome composto", "José da Silva", ""},
		{"normaliza espaços", "  Maria   Souza  ", ""},
		{"monônimo", "Maria", "precisa incluir sobrenome"},
		{"vazio", "   ", "não pode estar vazio"},
		{"um caractere", "M", "muito curto"},
		{"com número", "Maria 2 Souza", "não pode conter números"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l, err := NewLeadWithOptionalTenant(tc.input, "maria@example.com", "11999999999", "",
				LeadSourceWebsite, LeadStatusNew, nil, "", "123e4567-e89b-12d3-a456-426614174000")
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("não esperava erro, veio %v", err)
				}
				if l.Name != "Maria Souza" && tc.input == "  Maria   Souza  " {
					t.Fatalf("nome deveria sair normalizado, veio %q", l.Name)
				}
				return
			}
			if err == nil {
				t.Fatalf("esperava erro contendo %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("erro %q não contém %q", err.Error(), tc.wantErr)
			}
		})
	}
}
