package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

// inMemoryAcceptLeadRepo é o mínimo de Lead Repository para os testes —
// só o que AcceptAssignment/RejectAssignment usa: GetByID + Update.
type inMemoryAcceptLeadRepo struct {
	leads map[string]*lead.Lead
}

func newAcceptRepo(seed ...*lead.Lead) *inMemoryAcceptLeadRepo {
	r := &inMemoryAcceptLeadRepo{leads: map[string]*lead.Lead{}}
	for _, l := range seed {
		r.leads[l.ID] = l
	}
	return r
}

func (r *inMemoryAcceptLeadRepo) Create(ctx context.Context, l *lead.Lead) error {
	r.leads[l.ID] = l
	return nil
}

// GetByID espelha o contrato do repositório real (achado C9): filtra por
// tenant. O tenant efetivo é o do contexto quando presente; os testes chamam
// com o ctx do chamador.
func (r *inMemoryAcceptLeadRepo) GetByID(ctx context.Context, tenantID, id string) (*lead.Lead, error) {
	// Espelha o repositório real (achado C9): tenant vazio é recusado
	// (fail-closed) e tenant diferente do dono não encontra o lead.
	if strings.TrimSpace(tenantID) == "" {
		return nil, nil
	}
	l, ok := r.leads[id]
	if !ok || (l.TenantID != "" && l.TenantID != tenantID) {
		return nil, nil
	}
	return l, nil
}

// GetByIDForTenantService espelha a leitura interna pelo pool de serviço:
// mesmo escopo do GetByID (tenant explícito), sem passar pela política de RLS.
func (r *inMemoryAcceptLeadRepo) GetByIDForTenantService(ctx context.Context, tenantID, id string) (*lead.Lead, error) {
	return r.GetByID(ctx, tenantID, id)
}

func (r *inMemoryAcceptLeadRepo) GetByIDAnyTenant(ctx context.Context, id string) (*lead.Lead, error) {
	if l, ok := r.leads[id]; ok {
		return l, nil
	}
	return nil, nil
}
func (r *inMemoryAcceptLeadRepo) GetByEmail(ctx context.Context, tenantID, email string) (*lead.Lead, error) {
	return nil, nil
}
func (r *inMemoryAcceptLeadRepo) GetByPhone(ctx context.Context, tenantID, phone string) (*lead.Lead, error) {
	return nil, nil
}
func (r *inMemoryAcceptLeadRepo) GetByDisplayID(ctx context.Context, tenantID, displayID string) (*lead.Lead, error) {
	return nil, nil
}
func (r *inMemoryAcceptLeadRepo) ListByAccountID(ctx context.Context, tenantID, accountID string, limit int64) ([]*lead.Lead, error) {
	return nil, nil
}
func (r *inMemoryAcceptLeadRepo) Update(ctx context.Context, l *lead.Lead) error {
	r.leads[l.ID] = l
	return nil
}
func (r *inMemoryAcceptLeadRepo) Patch(ctx context.Context, leadID, tenantID string, expected *lead.LeadExpectedVersion, updates map[string]interface{}) (*lead.Lead, bool, error) {
	l := r.leads[leadID]
	if l == nil || l.TenantID != tenantID {
		return nil, false, nil
	}
	if expected != nil && ((expected.UpdatedSeq != nil && *expected.UpdatedSeq != l.UpdatedSeq) || (expected.UpdatedAt != nil && !expected.UpdatedAt.Equal(l.UpdatedAt))) {
		return nil, false, nil
	}
	data, err := json.Marshal(l)
	if err != nil {
		return nil, false, err
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, false, err
	}
	for key, value := range updates {
		metadata[key] = value
	}
	data, err = json.Marshal(metadata)
	if err != nil {
		return nil, false, err
	}
	var next lead.Lead
	if err := json.Unmarshal(data, &next); err != nil {
		return nil, false, err
	}
	next.UpdatedSeq, next.UpdatedAt = l.UpdatedSeq+1, time.Now()
	r.leads[leadID] = &next
	return &next, true, nil
}
func (r *inMemoryAcceptLeadRepo) AssignBrokerIfUnassigned(ctx context.Context, leadID, tenantID, brokerID, details string) (bool, error) {
	return false, nil
}
func (r *inMemoryAcceptLeadRepo) Delete(ctx context.Context, tenantID, id string) error {
	l, ok := r.leads[id]
	// Espelha o repositório real (achado C9): nada afetado => sql.ErrNoRows,
	// para o serviço não responder sucesso silencioso em lead de outro tenant.
	if !ok || (tenantID != "" && l.TenantID != "" && l.TenantID != tenantID) {
		return sql.ErrNoRows
	}
	delete(r.leads, id)
	return nil
}
func (r *inMemoryAcceptLeadRepo) FindByEmailAnyTenant(ctx context.Context, email string, limit int64) ([]*lead.Lead, error) {
	// Espelha o contrato do repositório real (achado A19): caminho cross-tenant
	// explícito, usado só pelo portal do cliente.
	out := []*lead.Lead{}
	for _, l := range r.leads {
		if strings.EqualFold(l.Email, email) {
			out = append(out, l)
		}
	}
	return out, nil
}

func (r *inMemoryAcceptLeadRepo) List(ctx context.Context, tenantID string, filter lead.LeadFilter) ([]*lead.Lead, int64, error) {
	return nil, 0, nil
}
func (r *inMemoryAcceptLeadRepo) ListUnassigned(ctx context.Context, tenantID string, limit int64) ([]*lead.Lead, error) {
	return nil, nil
}
func (r *inMemoryAcceptLeadRepo) CountUnassigned(ctx context.Context, tenantID string) (int64, error) {
	return 0, nil
}
func (r *inMemoryAcceptLeadRepo) GetStats(ctx context.Context, tenantID, brokerID string) (int64, map[string]int64, float64, error) {
	return 0, nil, 0, nil
}
func (r *inMemoryAcceptLeadRepo) GetActiveLeadCountsByBroker(ctx context.Context, tenantID string, brokerIDs []string) (map[string]int, error) {
	return nil, nil
}
func (r *inMemoryAcceptLeadRepo) ListForSLARedirect(ctx context.Context, tenantID string, statuses []string, maxRetries int, cutoff time.Time, limit int64) ([]*lead.Lead, error) {
	return nil, nil
}
func (r *inMemoryAcceptLeadRepo) ListForFollowupSLA(ctx context.Context, tenantID string, statuses []string, cutoff time.Time, limit int64) ([]*lead.Lead, error) {
	return nil, nil
}
func (r *inMemoryAcceptLeadRepo) ListFirstContactSLABreaches(ctx context.Context, tenantID string, cutoff time.Time, limit int64) ([]*lead.Lead, error) {
	return nil, nil
}
func (r *inMemoryAcceptLeadRepo) ListSharedForPartner(ctx context.Context, ownerTenantID string) ([]*lead.Lead, error) {
	return nil, nil
}
func (r *inMemoryAcceptLeadRepo) ReassignBrokerIfCurrent(ctx context.Context, leadID, tenantID, expectedBrokerID, newBrokerID, tierID, details string) (bool, error) {
	return false, nil
}

// pendingLead helper — lead em PENDING_ACCEPTANCE atribuído ao broker
// passado, com AssignedAt agora-1min.
func pendingLead(id, tenantID, brokerID string) *lead.Lead {
	return &lead.Lead{
		ID:               id,
		TenantID:         tenantID,
		Name:             "Test Lead",
		Email:            "test@example.com",
		Status:           lead.LeadStatusPendingAcceptance,
		AssignedBrokerID: brokerID,
		AssignedAt:       time.Now().Add(-1 * time.Minute),
		ExpiresAt:        time.Now().Add(4 * time.Minute),
	}
}

func newAcceptTestService(repo lead.Repository) *LeadService {
	return &LeadService{
		repo:          repo,
		bootstrapping: map[string]bool{},
	}
}

func TestAcceptAssignment_Success(t *testing.T) {
	ctx := pkgcontext.WithTenantID(context.Background(), "tenant-A")
	l := pendingLead("lead-1", "tenant-A", "broker-1")
	repo := newAcceptRepo(l)
	svc := newAcceptTestService(repo)

	out, err := svc.AcceptAssignment(ctx, "lead-1", "broker-1")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if out.Status != lead.LeadStatusContacted {
		t.Errorf("expected status CONTACTED, got %q", out.Status)
	}
	if !out.ExpiresAt.IsZero() {
		t.Errorf("expected ExpiresAt to be zeroed, got %v", out.ExpiresAt)
	}
	if out.AssignedBrokerID != "broker-1" {
		t.Errorf("AssignedBrokerID should be preserved, got %q", out.AssignedBrokerID)
	}
}

func TestAcceptAssignment_WrongBroker(t *testing.T) {
	ctx := pkgcontext.WithTenantID(context.Background(), "tenant-A")
	l := pendingLead("lead-1", "tenant-A", "broker-A")
	repo := newAcceptRepo(l)
	svc := newAcceptTestService(repo)

	_, err := svc.AcceptAssignment(ctx, "lead-1", "broker-B")
	if err == nil {
		t.Fatal("expected error when wrong broker accepts, got nil")
	}
	if err.Error() != "lead is not assigned to caller" {
		t.Errorf("unexpected error message: %q", err.Error())
	}
	// Sanity: lead status not mutated.
	got, _ := repo.GetByID(ctx, "tenant-A", "lead-1")
	if got.Status != lead.LeadStatusPendingAcceptance {
		t.Errorf("status should remain PENDING after failed accept, got %q", got.Status)
	}
}

func TestAcceptAssignment_NotPending(t *testing.T) {
	ctx := context.Background()
	l := pendingLead("lead-1", "tenant-A", "broker-1")
	l.Status = lead.LeadStatusContacted
	repo := newAcceptRepo(l)
	svc := newAcceptTestService(repo)

	_, err := svc.AcceptAssignment(ctx, "lead-1", "broker-1")
	if err == nil {
		t.Fatal("expected error when status != PENDING, got nil")
	}
}

func TestAcceptAssignment_NotFound(t *testing.T) {
	ctx := pkgcontext.WithTenantID(context.Background(), "tenant-A")
	repo := newAcceptRepo()
	svc := newAcceptTestService(repo)

	_, err := svc.AcceptAssignment(ctx, "lead-missing", "broker-1")
	if err == nil || err.Error() != "lead not found" {
		t.Errorf("expected 'lead not found', got %v", err)
	}
}

func TestRejectAssignment_Success(t *testing.T) {
	ctx := pkgcontext.WithTenantID(context.Background(), "tenant-A")
	l := pendingLead("lead-1", "tenant-A", "broker-1")
	repo := newAcceptRepo(l)
	svc := newAcceptTestService(repo)

	out, err := svc.RejectAssignment(ctx, "lead-1", "broker-1", "sem capacity")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if out.Status != lead.LeadStatusNew {
		t.Errorf("expected status NEW after reject, got %q", out.Status)
	}
	if out.AssignedBrokerID != "" {
		t.Errorf("expected assigned_broker_id cleared, got %q", out.AssignedBrokerID)
	}
	// AssignedAt PRESERVADO para métricas históricas (RouletteHistory).
	if out.AssignedAt.IsZero() {
		t.Errorf("AssignedAt should be preserved for historical metrics")
	}
}

func TestRejectAssignment_MissingReason(t *testing.T) {
	ctx := context.Background()
	l := pendingLead("lead-1", "tenant-A", "broker-1")
	repo := newAcceptRepo(l)
	svc := newAcceptTestService(repo)

	_, err := svc.RejectAssignment(ctx, "lead-1", "broker-1", "")
	if err == nil || err.Error() != "reason is required for reject" {
		t.Errorf("expected 'reason is required', got %v", err)
	}
	got, _ := repo.GetByID(ctx, "tenant-A", "lead-1")
	if got.Status != lead.LeadStatusPendingAcceptance {
		t.Error("status should remain PENDING when reject is rejected")
	}
}

func TestRejectAssignment_WrongBroker(t *testing.T) {
	ctx := pkgcontext.WithTenantID(context.Background(), "tenant-A")
	l := pendingLead("lead-1", "tenant-A", "broker-A")
	repo := newAcceptRepo(l)
	svc := newAcceptTestService(repo)

	_, err := svc.RejectAssignment(ctx, "lead-1", "broker-B", "qualquer")
	if err == nil || err.Error() != "lead is not assigned to caller" {
		t.Errorf("expected guard violation, got %v", err)
	}
}

func (r *inMemoryAcceptLeadRepo) FindByAccountID(_ context.Context, accountID string, _ int64) ([]*lead.Lead, error) {
	out := []*lead.Lead{}
	for _, l := range r.leads {
		if l.AccountID == accountID {
			copy := *l
			out = append(out, &copy)
		}
	}
	return out, nil
}
func (r *inMemoryAcceptLeadRepo) FindByEmailGlobal(_ context.Context, email string, _ int64) ([]*lead.Lead, error) {
	out := []*lead.Lead{}
	for _, l := range r.leads {
		if l.Email == email {
			copy := *l
			out = append(out, &copy)
		}
	}
	return out, nil
}
