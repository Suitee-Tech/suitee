package handlers

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/oginoino/toca/toca-portal/internal/domain/integration"
	"github.com/oginoino/toca/toca-portal/internal/domain/lead"
	psync "github.com/oginoino/toca/toca-portal/internal/domain/sync"
)

// Fakes usados pelos testes P0 (C4/C5). Eles emulam fielmente a semântica do
// repositório Postgres real no momento em que os testes foram escritos, para
// que um teste que passa aqui signifique que a camada de cima (handler/serviço)
// está passando o tenant do chamador. O teste de SQL de verdade vive em
// internal/repositories/integration_p0_tenant_isolation_test.go.

// fakeIntegrationRepo emula portal.integrations com a semântica PÓS-correção:
// o tenant do chamador é sempre o primeiro filtro (espelha o
// `WHERE id = $1 AND tenant_id = $2` do repositório Postgres).
//
// O mesmo arquivo, na versão pré-correção, não recebia tenant nenhum: GetByID
// buscava só por id, Update filtrava pelo tenant do próprio registro e Delete
// era incondicional — é o que o output vermelho em
// docs/reviews/evidence/p0/C4-C5-portal-before-red.txt documenta.
type fakeIntegrationRepo struct {
	mu         sync.Mutex
	rows       map[string]*integration.PortalIntegration
	gone       map[string]bool
	calls      []string
	tenantArgs []string
	// failWith, quando definido, é devolvido por GetByID/Update/Delete — usado
	// para provar que erro de driver não chega ao cliente.
	failWith error
}

func newFakeIntegrationRepo() *fakeIntegrationRepo {
	return &fakeIntegrationRepo{
		rows: map[string]*integration.PortalIntegration{},
		gone: map[string]bool{},
	}
}

func (f *fakeIntegrationRepo) seed(integ *integration.PortalIntegration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows[integ.ID] = cloneIntegration(integ)
}

// raw devolve uma cópia da linha como ela está no "banco" (nil se não existe).
func (f *fakeIntegrationRepo) raw(id string) *integration.PortalIntegration {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.rows[id]
	if !ok {
		return nil
	}
	return cloneIntegration(row)
}

func (f *fakeIntegrationRepo) isDeleted(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gone[id]
}

// tenantArgsReceived devolve, em ordem, o tenant que o handler repassou ao
// repositório nas chamadas tenant-scoped.
func (f *fakeIntegrationRepo) tenantArgsReceived() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.tenantArgs))
	copy(out, f.tenantArgs)
	return out
}

func (f *fakeIntegrationRepo) recordedCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	copy(out, f.calls)
	return out
}

func (f *fakeIntegrationRepo) record(format string, args ...any) {
	f.calls = append(f.calls, fmt.Sprintf(format, args...))
}

func (f *fakeIntegrationRepo) Create(_ context.Context, integ *integration.PortalIntegration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("Create")
	f.rows[integ.ID] = cloneIntegration(integ)
	return nil
}

func (f *fakeIntegrationRepo) GetByID(_ context.Context, tenantID, id string) (*integration.PortalIntegration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("GetByID(tenant=%s,id=%s)", tenantID, id)
	f.tenantArgs = append(f.tenantArgs, tenantID)
	if f.failWith != nil {
		return nil, f.failWith
	}
	row, ok := f.rows[id]
	if !ok || f.gone[id] || strings.TrimSpace(tenantID) == "" || row.TenantID != tenantID {
		return nil, integration.ErrNotFound
	}
	return cloneIntegration(row), nil
}

func (f *fakeIntegrationRepo) GetByIDAnyTenant(_ context.Context, id string) (*integration.PortalIntegration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("GetByIDAnyTenant(id=%s)", id)
	row, ok := f.rows[id]
	if !ok || f.gone[id] {
		return nil, integration.ErrNotFound
	}
	return cloneIntegration(row), nil
}

func (f *fakeIntegrationRepo) GetByFeedToken(_ context.Context, token string) (*integration.PortalIntegration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, row := range f.rows {
		if row.FeedToken == token {
			return cloneIntegration(row), nil
		}
	}
	return nil, fmt.Errorf("integration not found: sql: no rows in result set")
}

func (f *fakeIntegrationRepo) GetByTenantAndPortal(_ context.Context, tenantID string, portal integration.PortalType) (*integration.PortalIntegration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, row := range f.rows {
		if row.TenantID == tenantID && row.Portal == portal {
			return cloneIntegration(row), nil
		}
	}
	return nil, nil
}

func (f *fakeIntegrationRepo) Update(_ context.Context, tenantID string, integ *integration.PortalIntegration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("Update(tenant=%s,id=%s)", tenantID, integ.ID)
	f.tenantArgs = append(f.tenantArgs, tenantID)
	if f.failWith != nil {
		return f.failWith
	}
	row, ok := f.rows[integ.ID]
	if !ok || f.gone[integ.ID] || row.TenantID != tenantID || integ.TenantID != tenantID {
		return integration.ErrNotFound // 0 linhas afetadas = não encontrado
	}
	f.rows[integ.ID] = cloneIntegration(integ)
	return nil
}

func (f *fakeIntegrationRepo) Delete(_ context.Context, tenantID, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("Delete(tenant=%s,id=%s)", tenantID, id)
	f.tenantArgs = append(f.tenantArgs, tenantID)
	if f.failWith != nil {
		return f.failWith
	}
	row, ok := f.rows[id]
	if !ok || f.gone[id] || row.TenantID != tenantID {
		return integration.ErrNotFound
	}
	f.gone[id] = true
	return nil
}

func (f *fakeIntegrationRepo) ListByTenant(_ context.Context, tenantID string) ([]*integration.PortalIntegration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []*integration.PortalIntegration{}
	for id, row := range f.rows {
		if row.TenantID == tenantID && !f.gone[id] {
			out = append(out, cloneIntegration(row))
		}
	}
	return out, nil
}

func (f *fakeIntegrationRepo) ListActive(_ context.Context) ([]*integration.PortalIntegration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []*integration.PortalIntegration{}
	for id, row := range f.rows {
		if row.Status == integration.IntegrationActive && !f.gone[id] {
			out = append(out, cloneIntegration(row))
		}
	}
	return out, nil
}

func cloneIntegration(in *integration.PortalIntegration) *integration.PortalIntegration {
	if in == nil {
		return nil
	}
	cp := *in
	return &cp
}

// fakeLeadRepo emula portal.leads e guarda o que foi persistido.
type fakeLeadRepo struct {
	mu    sync.Mutex
	leads map[string]*lead.PortalLead
}

func newFakeLeadRepo() *fakeLeadRepo {
	return &fakeLeadRepo{leads: map[string]*lead.PortalLead{}}
}

func (f *fakeLeadRepo) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.leads)
}

func (f *fakeLeadRepo) all() []*lead.PortalLead {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*lead.PortalLead, 0, len(f.leads))
	for _, l := range f.leads {
		cp := *l
		out = append(out, &cp)
	}
	return out
}

func (f *fakeLeadRepo) Create(_ context.Context, pl *lead.PortalLead) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *pl
	f.leads[pl.ID] = &cp
	return nil
}

func (f *fakeLeadRepo) Update(_ context.Context, pl *lead.PortalLead) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *pl
	f.leads[pl.ID] = &cp
	return nil
}

func (f *fakeLeadRepo) GetByPortalAndContact(_ context.Context, _, _, _ string) (*lead.PortalLead, error) {
	return nil, nil
}

func (f *fakeLeadRepo) List(_ context.Context, _, _, _ string, _, _ int64) ([]*lead.PortalLead, int64, error) {
	return nil, 0, nil
}

func (f *fakeLeadRepo) ListByTenant(_ context.Context, _ string, _, _ int64) ([]*lead.PortalLead, int64, error) {
	return nil, 0, nil
}

func (f *fakeLeadRepo) ListByIntegration(_ context.Context, _, _ string, _, _ int64) ([]*lead.PortalLead, int64, error) {
	return nil, 0, nil
}

func (f *fakeLeadRepo) FindDuplicate(_ context.Context, _, _, _, _, _ string) (*lead.PortalLead, error) {
	return nil, nil
}

func (f *fakeLeadRepo) GetStats(_ context.Context, _ string) (int64, map[string]int64, error) {
	return 0, map[string]int64{}, nil
}

// fakeSyncRepo emula portal.sync_history (só o suficiente para o caminho de
// sync dos handlers).
type fakeSyncRepo struct {
	mu       sync.Mutex
	byTenant map[string]int
}

func newFakeSyncRepo() *fakeSyncRepo {
	return &fakeSyncRepo{byTenant: map[string]int{}}
}

func (f *fakeSyncRepo) Create(_ context.Context, s *psync.PortalSync) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byTenant[s.TenantID]++
	return nil
}

func (f *fakeSyncRepo) Update(_ context.Context, _ *psync.PortalSync) error { return nil }

func (f *fakeSyncRepo) GetLatestByIntegrationID(_ context.Context, tenantID, integrationID string) (*psync.PortalSync, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &psync.PortalSync{ID: "sync-1", TenantID: tenantID, IntegrationID: integrationID}, nil
}

func (f *fakeSyncRepo) ListByIntegrationID(_ context.Context, tenantID, _ string, _, _ int64) ([]*psync.PortalSync, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.byTenant[tenantID] == 0 {
		return []*psync.PortalSync{}, 0, nil
	}
	return []*psync.PortalSync{{ID: "sync-1", TenantID: tenantID}}, 1, nil
}
