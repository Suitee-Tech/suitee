package postgres

import (
	"context"
	"strings"
	"testing"
)

// As três buscas por id abaixo eram `WHERE id = $1 AND deleted_at IS NULL`,
// sem tenant_id (auditorias C8 e A8/ops). Como a role de banco tem BYPASSRLS,
// não há backstop: se o filtro sumir da query, o isolamento de tenant some
// junto. Este teste fixa o contrato do SQL sem precisar de Postgres.
func TestGetByIDQueriesAreTenantScoped(t *testing.T) {
	cases := []struct {
		name  string
		query string
	}{
		{"ops.inspections", inspectionGetByIDQuery},
		{"ops.maintenance", maintenanceGetByIDQuery},
		{"ops.keys", keyGetByIDQuery},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(tc.query, "id = $1") {
				t.Errorf("%s: query deve continuar filtrando por id: %s", tc.name, tc.query)
			}
			if !strings.Contains(tc.query, "tenant_id = $2") {
				t.Errorf("%s: query SEM tenant_id (leitura cross-tenant): %s", tc.name, tc.query)
			}
			if !strings.Contains(tc.query, "deleted_at IS NULL") {
				t.Errorf("%s: query deve ignorar registros soft-deleted: %s", tc.name, tc.query)
			}
		})
	}
}

// Fail closed: sem tenant não existe escopo de autorização, então o repositório
// deve recusar ANTES de tocar o banco. O db é nil de propósito — se a checagem
// de tenant não acontecer primeiro, o teste falha com panic em vez de passar.
func TestGetByIDFailsClosedWithoutTenant(t *testing.T) {
	ctx := context.Background()
	blank := []string{"", "   "}

	for _, tenantID := range blank {
		if _, err := (&inspectionRepository{}).GetByID(ctx, tenantID, "insp-1"); err == nil {
			t.Errorf("inspectionRepository.GetByID(tenant=%q) deveria falhar fechado", tenantID)
		}
		if _, err := (&maintenanceRepository{}).GetByID(ctx, tenantID, "mt-1"); err == nil {
			t.Errorf("maintenanceRepository.GetByID(tenant=%q) deveria falhar fechado", tenantID)
		}
		if _, err := (&keyRepository{}).GetByID(ctx, tenantID, "key-1"); err == nil {
			t.Errorf("keyRepository.GetByID(tenant=%q) deveria falhar fechado", tenantID)
		}
	}

	// Id vazio também não pode virar busca sem filtro.
	if _, err := (&inspectionRepository{}).GetByID(ctx, "tenant-1", ""); err == nil {
		t.Error("inspectionRepository.GetByID(id vazio) deveria falhar")
	}
}
