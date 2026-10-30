package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// fakeRepo implementa deal.Repository e a guarda opcional de criação, com
// injeção de falha para simular a corrida de idempotência.
type fakeRepo struct {
	*inMemoryDealRepo
	failCreateWith conflictErr
}

type conflictErr bool

func (f *fakeRepo) Create(ctx context.Context, d *deal.Deal) error {
	if f.failCreateWith {
		return deal.ErrIdempotencyConflict
	}
	return f.inMemoryDealRepo.Create(ctx, d)
}

func (f *fakeRepo) GetByIdempotencyKey(ctx context.Context, tenantID, key string) (*deal.Deal, error) {
	for _, d := range f.deals {
		if d.TenantID == tenantID && d.IdempotencyKey == key && d.Status != deal.StatusCancelled {
			return d, nil
		}
	}
	return nil, nil
}

func (f *fakeRepo) FindRecentDuplicate(ctx context.Context, tenantID, propertyID, customerID string, dealType deal.Type, value float64, since time.Time) (*deal.Deal, error) {
	var newest *deal.Deal
	for _, d := range f.deals {
		if d.TenantID != tenantID || d.PropertyID != propertyID || d.CustomerID != customerID || d.Type != dealType {
			continue
		}
		if d.Status == deal.StatusCancelled || d.CreatedAt.Before(since) {
			continue
		}
		if value-d.Value > 0.005 || d.Value-value > 0.005 {
			continue
		}
		if newest == nil || d.CreatedAt.After(newest.CreatedAt) {
			newest = d
		}
	}
	return newest, nil
}

func newServiceWithFakeRepo() (*DealService, *fakeRepo) {
	base := newInMemoryDealRepo()
	repo := &fakeRepo{inMemoryDealRepo: base}
	return &DealService{repo: repo}, repo
}

func mustCreateDeal(t *testing.T, s *DealService, tenantID string, value float64, opts ...CreateDealOption) *deal.Deal {
	t.Helper()
	d, err := s.Create(context.Background(), deal.TypeSale,
		"prop-1", "owner-1", "customer-1", "broker-1", "customer-1",
		value, tenantID, "Cliente", "cliente@example.com", "", "comprador",
		PartyProfileHints{}, false, opts...)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	return d
}

// A chave de idempotência transforma reenvios do mesmo formulário em replay:
// a segunda chamada devolve o negócio criado na primeira.
func TestCreateReplaysIdempotentIntent(t *testing.T) {
	s, repo := newServiceWithFakeRepo()
	key := "intent:create:prop-1:customer-1:SALE:1000000.00"

	first := mustCreateDeal(t, s, "tenant-1", 1_000_000, WithIdempotencyKey(key))
	second := mustCreateDeal(t, s, "tenant-1", 1_000_000, WithIdempotencyKey(key))

	if first.ID != second.ID {
		t.Fatalf("replay criou outro negócio: %s vs %s", first.ID, second.ID)
	}
	if len(repo.deals) != 1 {
		t.Fatalf("criações persistidas = %d, want 1", len(repo.deals))
	}
}

// A mesma chave com payload diferente é conflito, não replay silencioso.
func TestCreateRejectsIdempotencyKeyReuseWithDifferentPayload(t *testing.T) {
	s, _ := newServiceWithFakeRepo()
	key := "intent:create:prop-1:customer-1:SALE:1000000.00"

	mustCreateDeal(t, s, "tenant-1", 1_000_000, WithIdempotencyKey(key))
	_, err := s.Create(context.Background(), deal.TypeSale,
		"prop-1", "owner-1", "customer-1", "broker-1", "customer-1",
		999_000, "tenant-1", "Cliente", "cliente@example.com", "", "comprador",
		PartyProfileHints{}, false, WithIdempotencyKey(key))
	if err == nil || !strings.Contains(err.Error(), "different payload") {
		t.Fatalf("Create() error = %v, want different payload conflict", err)
	}
}

// Negócio cancelado libera a chave: recriar depois de cancelar é uma intenção
// nova e válida.
func TestCancelledDealReleasesIdempotencyKey(t *testing.T) {
	s, repo := newServiceWithFakeRepo()
	key := "intent:create:prop-1:customer-1:SALE:1000000.00"

	first := mustCreateDeal(t, s, "tenant-1", 1_000_000, WithIdempotencyKey(key))
	first.Status = deal.StatusCancelled

	second := mustCreateDeal(t, s, "tenant-1", 1_000_000, WithIdempotencyKey(key))
	if first.ID == second.ID {
		t.Fatalf("negócio cancelado não liberou a chave")
	}
	if len(repo.deals) != 2 {
		t.Fatalf("criações persistidas = %d, want 2", len(repo.deals))
	}
}

// Cliente sem chave é protegido pela janela: mesmo imóvel + cliente + tipo +
// valor em segundos é devolvido como duplicata (409 na borda HTTP).
func TestCreateBlocksRecentDuplicateWithoutKey(t *testing.T) {
	s, repo := newServiceWithFakeRepo()
	older := &deal.Deal{
		ID: "older", TenantID: "tenant-1", Type: deal.TypeSale,
		PropertyID: "prop-1", CustomerID: "customer-1", Value: 1_000_000,
		Status: deal.StatusNegotiation, CreatedAt: time.Now().Add(-5 * time.Second),
	}
	repo.deals[older.ID] = older

	_, err := s.Create(context.Background(), deal.TypeSale,
		"prop-1", "owner-1", "customer-1", "broker-1", "customer-1",
		1_000_000, "tenant-1", "Cliente", "cliente@example.com", "", "comprador",
		PartyProfileHints{}, false)
	var dupErr *DuplicateDealError
	if err == nil || !asDuplicateDealError(err, &dupErr) || dupErr.ExistingID != "older" {
		t.Fatalf("Create() error = %v, want DuplicateDealError{older}", err)
	}
	if len(repo.deals) != 1 {
		t.Fatalf("criações persistidas = %d, want 1", len(repo.deals))
	}
}

// A janela não bloqueia negociação legítima fora dela, nem valor diferente.
func TestCreateAllowsDifferentIntentOutsideWindow(t *testing.T) {
	s, repo := newServiceWithFakeRepo()
	old := &deal.Deal{
		ID: "old", TenantID: "tenant-1", Type: deal.TypeSale,
		PropertyID: "prop-1", CustomerID: "customer-1", Value: 1_000_000,
		Status: deal.StatusNegotiation, CreatedAt: time.Now().Add(-2 * time.Minute),
	}
	repo.deals[old.ID] = old

	created := mustCreateDeal(t, s, "tenant-1", 1_100_000) // outro valor, outra intenção
	if created.ID == old.ID {
		t.Fatalf("valor diferente deveria criar um novo negócio")
	}
}

// A corrida real: dois envios simultâneos com a mesma chave, o perdedor do
// INSERT relê o vencedor em vez de falhar.
func TestCreateResolvesInsertConflictByKey(t *testing.T) {
	s, repo := newServiceWithFakeRepo()
	key := "intent:create:prop-1:customer-1:SALE:1000000.00"

	winner := mustCreateDeal(t, s, "tenant-1", 1_000_000, WithIdempotencyKey(key))

	repo.failCreateWith = true
	replay := mustCreateDeal(t, s, "tenant-1", 1_000_000, WithIdempotencyKey(key))
	if replay.ID != winner.ID {
		t.Fatalf("conflito de inserção deveria devolver o vencedor %s, veio %s", winner.ID, replay.ID)
	}
}

func asDuplicateDealError(err error, target **DuplicateDealError) bool {
	if err == nil {
		return false
	}
	dup, ok := err.(*DuplicateDealError)
	if ok {
		*target = dup
	}
	return ok
}
