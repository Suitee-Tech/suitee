package services

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/collection"
	"github.com/oginoino/toca/toca-financial/internal/insurer"
)

// fakeCollectionRepo é um collection.RepositoryInterface de teste com
// estado mínimo: guarda uma Collection e a devolve em GetByID. Update e
// Create sobrescrevem o estado. Os demais métodos são no-op — não são
// exercitados por SetJudicialProcess.
type fakeCollectionRepo struct {
	stored               *collection.Collection
	forceVersionConflict bool
	versionedUpdates     int
}

func cloneCollectionForTest(c *collection.Collection) *collection.Collection {
	if c == nil {
		return nil
	}
	b, _ := json.Marshal(c)
	var out collection.Collection
	_ = json.Unmarshal(b, &out)
	return &out
}

func (f *fakeCollectionRepo) Create(ctx context.Context, c *collection.Collection) error {
	f.stored = c
	return nil
}
func (f *fakeCollectionRepo) CreateIdempotent(ctx context.Context, c *collection.Collection) error {
	f.stored = c
	return nil
}
func (f *fakeCollectionRepo) GetByID(ctx context.Context, tenantID, id string) (*collection.Collection, error) {
	if f.stored == nil || f.stored.ID != id {
		return nil, nil
	}
	return cloneCollectionForTest(f.stored), nil
}
func (f *fakeCollectionRepo) Update(ctx context.Context, c *collection.Collection) error {
	f.stored = c
	return nil
}
func (f *fakeCollectionRepo) UpdateIfVersion(_ context.Context, c *collection.Collection, expected time.Time) (bool, error) {
	f.versionedUpdates++
	if f.forceVersionConflict || f.stored == nil || !f.stored.UpdatedAt.Equal(expected) {
		return false, nil
	}
	c.UpdatedAt = time.Now().Add(time.Microsecond)
	f.stored = cloneCollectionForTest(c)
	return true, nil
}
func (f *fakeCollectionRepo) List(ctx context.Context, tenantID, status string, limit, offset int64) ([]*collection.Collection, int64, error) {
	return nil, 0, nil
}
func (f *fakeCollectionRepo) AddAction(ctx context.Context, tenantID, id string, action collection.Action) error {
	return nil
}
func (f *fakeCollectionRepo) FindByChargeID(ctx context.Context, tenantID, chargeID string) (*collection.Collection, error) {
	return nil, nil
}
func (f *fakeCollectionRepo) FindByContract(ctx context.Context, contractID, tenantID string) ([]*collection.Collection, error) {
	return nil, nil
}
func (f *fakeCollectionRepo) FindActiveByDebtor(ctx context.Context, debtorID, tenantID string) (*collection.Collection, error) {
	return nil, nil
}
func (f *fakeCollectionRepo) GetStats(ctx context.Context, tenantID string) (*collection.CollectionStats, error) {
	return nil, nil
}

func newJudicialTestCollection() *collection.Collection {
	return collection.New("contract-1", "debtor-1", "Fulano de Tal", "tenant-1", 5000, 90, []string{"chg-1"})
}

func TestSetJudicialProcess_Success(t *testing.T) {
	coll := newJudicialTestCollection()
	repo := &fakeCollectionRepo{stored: coll}
	svc := NewCollectionService(repo)

	jp := collection.JudicialProcess{
		CaseNumber: "0001234-56.2024.8.26.0100",
		Court:      "2ª Vara Cível de São Paulo",
		Lawyer:     collection.Lawyer{Name: "Dra. Advogada", OAB: "SP123456"},
		ClaimValue: 12000,
		Status:     collection.JudicialStatusEmTramite,
	}
	out, err := svc.SetJudicialProcess(context.Background(), "tenant-1", coll.ID, jp, "user-1")
	if err != nil {
		t.Fatalf("SetJudicialProcess: %v", err)
	}
	if out.Judicial == nil {
		t.Fatal("Judicial não foi gravado na collection")
	}
	if out.Judicial.CaseNumber != "0001234-56.2024.8.26.0100" {
		t.Errorf("CaseNumber = %q", out.Judicial.CaseNumber)
	}
	if out.Status != collection.StatusJudicial {
		t.Errorf("status = %q, want JUDICIAL", out.Status)
	}
	if out.Judicial.CreatedAt.IsZero() || out.Judicial.UpdatedAt.IsZero() {
		t.Error("timestamps do processo judicial não preenchidos")
	}
	found := false
	for _, a := range out.Actions {
		if a.Type == "judicial_process" {
			found = true
		}
	}
	if !found {
		t.Error("nenhuma Action judicial_process registrada no histórico")
	}
}

func TestUpdateStatusRejectsResolvedReopen(t *testing.T) {
	coll := newJudicialTestCollection()
	coll.Status = collection.StatusResolved
	repo := &fakeCollectionRepo{stored: coll}
	svc := NewCollectionService(repo)

	_, err := svc.UpdateStatus(context.Background(), coll.TenantID, coll.ID, collection.StatusActive, "operator")
	if !errors.Is(err, collection.ErrInvalidStatusTransition) {
		t.Fatalf("UpdateStatus() error = %v, want ErrInvalidStatusTransition", err)
	}
	if repo.versionedUpdates != 0 || repo.stored.Status != collection.StatusResolved {
		t.Fatalf("resolved collection was written/reopened: calls=%d status=%s", repo.versionedUpdates, repo.stored.Status)
	}
}

func TestUpdateStatusRejectsConcurrentWrite(t *testing.T) {
	coll := newJudicialTestCollection()
	repo := &fakeCollectionRepo{stored: coll, forceVersionConflict: true}
	svc := NewCollectionService(repo)

	_, err := svc.UpdateStatus(context.Background(), coll.TenantID, coll.ID, collection.StatusNegotiated, "operator")
	if !errors.Is(err, collection.ErrConcurrentModification) {
		t.Fatalf("UpdateStatus() error = %v, want ErrConcurrentModification", err)
	}
	if repo.stored.Status != collection.StatusActive {
		t.Fatalf("concurrent failure mutated stored status: %s", repo.stored.Status)
	}
}

func TestSetJudicialProcess_InvalidCNJ(t *testing.T) {
	coll := newJudicialTestCollection()
	svc := NewCollectionService(&fakeCollectionRepo{stored: coll})

	jp := collection.JudicialProcess{CaseNumber: "123", Court: "Vara X"}
	if _, err := svc.SetJudicialProcess(context.Background(), "tenant-1", coll.ID, jp, "user-1"); !errors.Is(err, ErrInvalidJudicial) {
		t.Errorf("esperava ErrInvalidJudicial para CNJ malformado, veio %v", err)
	}
}

func TestSetJudicialProcess_DefaultsStatusEmTramite(t *testing.T) {
	coll := newJudicialTestCollection()
	svc := NewCollectionService(&fakeCollectionRepo{stored: coll})

	// Status vazio no input → EM_TRAMITE.
	jp := collection.JudicialProcess{CaseNumber: "00012345620248260100", Court: "Vara X"}
	out, err := svc.SetJudicialProcess(context.Background(), "tenant-1", coll.ID, jp, "user-1")
	if err != nil {
		t.Fatalf("SetJudicialProcess: %v", err)
	}
	if out.Judicial.Status != collection.JudicialStatusEmTramite {
		t.Errorf("status default = %q, want EM_TRAMITE", out.Judicial.Status)
	}
}

func TestSetJudicialProcess_NotFound(t *testing.T) {
	svc := NewCollectionService(&fakeCollectionRepo{}) // nada armazenado
	jp := collection.JudicialProcess{CaseNumber: "0001234-56.2024.8.26.0100", Court: "Vara X"}
	if _, err := svc.SetJudicialProcess(context.Background(), "tenant-1", "missing", jp, "user-1"); err == nil || err.Error() != "collection not found" {
		t.Errorf("esperava 'collection not found', veio %v", err)
	}
}

func TestSetJudicialProcess_RegisterPreservesCreatedAt(t *testing.T) {
	coll := newJudicialTestCollection()
	svc := NewCollectionService(&fakeCollectionRepo{stored: coll})

	jp := collection.JudicialProcess{CaseNumber: "0001234-56.2024.8.26.0100", Court: "Vara X"}
	first, err := svc.SetJudicialProcess(context.Background(), "tenant-1", coll.ID, jp, "user-1")
	if err != nil {
		t.Fatalf("primeira chamada: %v", err)
	}
	originalCreated := first.Judicial.CreatedAt

	// Re-registrar com outros dados preserva o CreatedAt do processo.
	jp2 := collection.JudicialProcess{
		CaseNumber: "0001234-56.2024.8.26.0100",
		Court:      "Vara Y",
		Status:     collection.JudicialStatusAcordo,
	}
	second, err := svc.SetJudicialProcess(context.Background(), "tenant-1", coll.ID, jp2, "user-1")
	if err != nil {
		t.Fatalf("segunda chamada: %v", err)
	}
	if !second.Judicial.CreatedAt.Equal(originalCreated) {
		t.Errorf("CreatedAt mudou ao atualizar: %v != %v", second.Judicial.CreatedAt, originalCreated)
	}
	if second.Judicial.Court != "Vara Y" {
		t.Errorf("Court não atualizado: %q", second.Judicial.Court)
	}
}

// fakeInsurer — adaptador de seguradora de teste com resposta configurável.
type fakeInsurer struct {
	accepted bool
}

func (fakeInsurer) Name() string { return "fake" }
func (f fakeInsurer) Activate(ctx context.Context, req insurer.Request) (insurer.Result, error) {
	return insurer.Result{
		InsurerName: "fake-seguradora",
		Protocol:    "PROT-123",
		Message:     "acionamento recebido",
		Accepted:    f.accepted,
	}, nil
}

func TestActivateGuarantee_SeguroFiancaUsesInsurer(t *testing.T) {
	coll := newJudicialTestCollection()
	svc := NewCollectionService(&fakeCollectionRepo{stored: coll})

	out, err := svc.ActivateGuarantee(context.Background(), "tenant-1", coll.ID, collection.GuaranteeTypeSeguroFianca, 8000, "inadimplência > 90 dias", "user-1")
	if err != nil {
		t.Fatalf("ActivateGuarantee: %v", err)
	}
	if out.Guarantee == nil {
		t.Fatal("Guarantee não foi gravada na collection")
	}
	if out.Status != collection.StatusGuaranteeActivated {
		t.Errorf("status da cobrança = %q, want GUARANTEE_ACTIVATED", out.Status)
	}
	// Sem WithInsurer → adaptador manual: protocolo pendente, REQUESTED.
	if out.Guarantee.Status != collection.GuaranteeStatusRequested {
		t.Errorf("status do acionamento = %q, want REQUESTED", out.Guarantee.Status)
	}
	if out.Guarantee.InsurerName == "" || out.Guarantee.InsurerResponse == "" {
		t.Error("Seguro Fiança deveria preencher InsurerName/InsurerResponse via adaptador")
	}
}

func TestActivateGuarantee_WithInsurerAccepted(t *testing.T) {
	coll := newJudicialTestCollection()
	svc := NewCollectionService(&fakeCollectionRepo{stored: coll}).WithInsurer(fakeInsurer{accepted: true})

	out, err := svc.ActivateGuarantee(context.Background(), "tenant-1", coll.ID, collection.GuaranteeTypeSeguroFianca, 8000, "", "user-1")
	if err != nil {
		t.Fatalf("ActivateGuarantee: %v", err)
	}
	if out.Guarantee.Status != collection.GuaranteeStatusAccepted {
		t.Errorf("seguradora aceitou → status = %q, want ACCEPTED", out.Guarantee.Status)
	}
	if out.Guarantee.InsurerProtocol != "PROT-123" {
		t.Errorf("protocolo da seguradora = %q, want PROT-123", out.Guarantee.InsurerProtocol)
	}
}

func TestActivateGuarantee_FiadorSkipsInsurer(t *testing.T) {
	coll := newJudicialTestCollection()
	svc := NewCollectionService(&fakeCollectionRepo{stored: coll})

	out, err := svc.ActivateGuarantee(context.Background(), "tenant-1", coll.ID, collection.GuaranteeTypeFiador, 3000, "", "user-1")
	if err != nil {
		t.Fatalf("ActivateGuarantee: %v", err)
	}
	if out.Guarantee.InsurerName != "" || out.Guarantee.InsurerResponse != "" {
		t.Error("Fiador não envolve seguradora — campos de insurer deveriam ficar vazios")
	}
	if out.Guarantee.Status != collection.GuaranteeStatusRequested {
		t.Errorf("status do acionamento = %q, want REQUESTED", out.Guarantee.Status)
	}
}

func TestActivateGuarantee_InvalidType(t *testing.T) {
	coll := newJudicialTestCollection()
	svc := NewCollectionService(&fakeCollectionRepo{stored: coll})
	if _, err := svc.ActivateGuarantee(context.Background(), "tenant-1", coll.ID, collection.GuaranteeType("OUTRO"), 1000, "", "u"); !errors.Is(err, ErrInvalidGuarantee) {
		t.Errorf("esperava ErrInvalidGuarantee para tipo inválido, veio %v", err)
	}
}

func TestActivateGuarantee_NonPositiveAmount(t *testing.T) {
	coll := newJudicialTestCollection()
	svc := NewCollectionService(&fakeCollectionRepo{stored: coll})
	if _, err := svc.ActivateGuarantee(context.Background(), "tenant-1", coll.ID, collection.GuaranteeTypeCaucao, 0, "", "u"); !errors.Is(err, ErrInvalidGuarantee) {
		t.Errorf("esperava ErrInvalidGuarantee para valor zero, veio %v", err)
	}
}

func TestUpdateGuaranteeStatus_TerminalSetsResolvedAt(t *testing.T) {
	coll := newJudicialTestCollection()
	svc := NewCollectionService(&fakeCollectionRepo{stored: coll})

	if _, err := svc.ActivateGuarantee(context.Background(), "tenant-1", coll.ID, collection.GuaranteeTypeCaucao, 3000, "", "u"); err != nil {
		t.Fatalf("ActivateGuarantee: %v", err)
	}
	out, err := svc.UpdateGuaranteeStatus(context.Background(), "tenant-1", coll.ID, collection.GuaranteeStatusPaid, "valor recuperado da caução", "u")
	if err != nil {
		t.Fatalf("UpdateGuaranteeStatus: %v", err)
	}
	if out.Guarantee.Status != collection.GuaranteeStatusPaid {
		t.Errorf("status do acionamento = %q, want PAID", out.Guarantee.Status)
	}
	if out.Guarantee.ResolvedAt == nil {
		t.Error("status terminal (PAID) deveria preencher ResolvedAt")
	}
}

func TestUpdateGuaranteeStatus_DoesNotReopenTerminalActivation(t *testing.T) {
	coll := newJudicialTestCollection()
	repo := &fakeCollectionRepo{stored: coll}
	svc := NewCollectionService(repo)
	if _, err := svc.ActivateGuarantee(context.Background(), "tenant-1", coll.ID, collection.GuaranteeTypeCaucao, 3000, "", "u"); err != nil {
		t.Fatalf("ActivateGuarantee: %v", err)
	}
	if _, err := svc.UpdateGuaranteeStatus(context.Background(), "tenant-1", coll.ID, collection.GuaranteeStatusPaid, "paid", "u"); err != nil {
		t.Fatalf("terminal update: %v", err)
	}

	_, err := svc.UpdateGuaranteeStatus(context.Background(), "tenant-1", coll.ID, collection.GuaranteeStatusRequested, "retry", "u")
	if !errors.Is(err, ErrInvalidGuarantee) {
		t.Fatalf("terminal reopen error = %v, want ErrInvalidGuarantee", err)
	}
	if repo.stored.Guarantee.Status != collection.GuaranteeStatusPaid {
		t.Fatalf("terminal guarantee reopened to %s", repo.stored.Guarantee.Status)
	}
}

func TestUpdateGuaranteeStatus_NoActivation(t *testing.T) {
	coll := newJudicialTestCollection()
	svc := NewCollectionService(&fakeCollectionRepo{stored: coll})
	if _, err := svc.UpdateGuaranteeStatus(context.Background(), "tenant-1", coll.ID, collection.GuaranteeStatusPaid, "", "u"); !errors.Is(err, ErrInvalidGuarantee) {
		t.Errorf("esperava ErrInvalidGuarantee quando não há acionamento, veio %v", err)
	}
}
