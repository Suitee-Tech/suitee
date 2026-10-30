package handlers

import (
	"strings"
	"testing"

	"github.com/oginoino/toca/pkg/audit"
)

// TestFiltersForLog confirma que o snapshot do filtro preserva todos
// os campos — útil pra investigação de quem rodou qual query.
func TestFiltersForLog(t *testing.T) {
	f := audit.ListFilters{
		TenantID:      "t-1",
		Action:        "auth.banking_updated",
		ActionPrefix:  "auth.",
		EntityType:    "User",
		EntityID:      "u-1",
		ActorID:       "actor-1",
		From:          "2026-05-01T00:00:00Z",
		To:            "2026-05-31T23:59:59Z",
		CreatedBefore: "2026-05-19T10:00:00Z",
		Limit:         200,
	}
	m := filtersForLog(f)

	expected := map[string]any{
		"tenant_id":      "t-1",
		"action":         "auth.banking_updated",
		"action_prefix":  "auth.",
		"entity_type":    "User",
		"entity_id":      "u-1",
		"actor_id":       "actor-1",
		"from":           "2026-05-01T00:00:00Z",
		"to":             "2026-05-31T23:59:59Z",
		"created_before": "2026-05-19T10:00:00Z",
		"limit":          200,
	}
	for k, v := range expected {
		if m[k] != v {
			t.Errorf("filtersForLog[%s] = %v, want %v", k, m[k], v)
		}
	}
}

// TestFiltersForLog_EmptyValuesPreserved garante que valores vazios
// também aparecem (audit precisa do log "explicitamente vazio" vs.
// "não enviado" para reconstituir consultas).
func TestFiltersForLog_EmptyValuesPreserved(t *testing.T) {
	m := filtersForLog(audit.ListFilters{})
	if len(m) != 10 {
		t.Errorf("expected 10 keys (mesmo que empty), got %d", len(m))
	}
}

// Acceptable smoke test: documenta a estrutura esperada do snapshot
// pra alguém que abra o handler depois e queira entender o shape.
func TestFiltersForLog_ContainsAllExpectedKeys(t *testing.T) {
	m := filtersForLog(audit.ListFilters{})
	expected := []string{
		"tenant_id", "action", "action_prefix",
		"entity_type", "entity_id", "actor_id",
		"from", "to", "created_before", "limit",
	}
	for _, k := range expected {
		if _, ok := m[k]; !ok {
			t.Errorf("filtersForLog missing key %q. Existing keys: %s", k, joinKeys(m))
		}
	}
}

func joinKeys(m map[string]any) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}
