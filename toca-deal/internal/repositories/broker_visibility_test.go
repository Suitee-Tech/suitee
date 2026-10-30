package repositories

import (
	"strings"
	"testing"
)

func TestBrokerVisibilityCoversResponsibleAndParticipants(t *testing.T) {
	sql := brokerVisibility("metadata", "$3")
	// O responsável continua entrando pelo `broker_id`.
	if !strings.Contains(sql, "(metadata->>'broker_id') = $3") {
		t.Fatalf("predicado sem o responsável: %s", sql)
	}
	// E quem participa da divisão entra pela lista de participações.
	if !strings.Contains(sql, "jsonb_array_elements(COALESCE(metadata->'broker_participations'") {
		t.Fatalf("predicado sem as participações: %s", sql)
	}
	if !strings.Contains(sql, "p->>'broker_id' = $3") {
		t.Fatalf("participação não casa com o corretor: %s", sql)
	}
	// A linha da imobiliária tem broker_id vazio e não pode casar com ninguém.
	if !strings.Contains(sql, "COALESCE(p->>'broker_id','') <> ''") {
		t.Fatalf("predicado aceitaria a linha da imobiliária: %s", sql)
	}
}
