package repositories

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// A busca de imóveis do portal do cliente é a vitrine da imobiliária. Dois
// defeitos conviviam nela (revisão de acesso a imóveis, 2026-09-19):
//
//  1. NÃO filtrava disponibilidade — devolvia imóvel vendido, pausado ou em
//     rascunho, que a recomendação de lead (`ListRecommendations`) já escondia;
//  2. finalidade por igualdade exata em `listing_type` — um imóvel marcado como
//     disponível também para aluguel (`available_for`) aparecia na recomendação
//     e sumia da busca; e o valor caía para o preço da OUTRA finalidade
//     (`sale_price` → `rent_price`), exibindo aluguel como preço de venda.
//
// Rodar:
//
//	createdb toca_review_qa_portal_imov
//	DEAL_REVIEW_TEST_DATABASE_URL=postgres://.../toca_review_qa_portal_imov \
//	  go test ./internal/repositories/ -run TestPortalPropertySearch -v
func TestPortalPropertySearchAvailabilityAndPrice(t *testing.T) {
	uri := os.Getenv("DEAL_REVIEW_TEST_DATABASE_URL")
	if uri == "" {
		t.Skip("set DEAL_REVIEW_TEST_DATABASE_URL to a disposable toca_review_qa_* database")
	}
	db, err := sqlx.Connect("pgx", uri)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var name string
	if err := db.Get(&name, "SELECT current_database()"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(name, "toca_review_qa_") {
		t.Fatal("refusing non-QA database")
	}

	if _, err := db.Exec(`CREATE SCHEMA IF NOT EXISTS core;
		CREATE TABLE IF NOT EXISTS core.properties (
			id uuid PRIMARY KEY,
			tenant_id uuid NOT NULL,
			metadata jsonb NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now(),
			deleted_at timestamptz
		)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec("DELETE FROM core.properties WHERE tenant_id IN ($1::uuid, $2::uuid)",
			"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"); err != nil {
			t.Errorf("cleanup QA properties: %v", err)
		}
	})

	tenant := "11111111-1111-4111-8111-111111111111"
	otherTenant := "22222222-2222-4222-8222-222222222222"

	seed := func(id, tenantID, metadata string) {
		t.Helper()
		if _, err := db.Exec(
			`INSERT INTO core.properties (id, tenant_id, metadata) VALUES ($1::uuid, $2::uuid, $3::jsonb)`,
			id, tenantID, metadata); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	property := func(title, listingType, status string, extra string) string {
		return `{"title":"` + title + `","listing_type":"` + listingType + `","status":"` + status + `",` +
			`"pricing":{"sale_price":{"current_amount":0},"rent_price":{"current_amount":0}}` + extra + `}`
	}

	seed("aaaaaaaa-0000-4000-8000-000000000001", tenant, property("Venda disponível", "sale", "published",
		`,"pricing":{"sale_price":{"current_amount":450000},"rent_price":{"current_amount":0}}`))
	seed("aaaaaaaa-0000-4000-8000-000000000002", tenant, property("Só aluguel", "rent", "published",
		`,"pricing":{"sale_price":{"current_amount":0},"rent_price":{"current_amount":2500}}`))
	seed("aaaaaaaa-0000-4000-8000-000000000003", tenant, property("Vendido", "sale", "sold",
		`,"pricing":{"sale_price":{"current_amount":300000},"rent_price":{"current_amount":0}}`))
	seed("aaaaaaaa-0000-4000-8000-000000000004", tenant, property("Rascunho", "sale", "draft",
		`,"pricing":{"sale_price":{"current_amount":100000},"rent_price":{"current_amount":0}}`))
	seed("aaaaaaaa-0000-4000-8000-000000000005", tenant, `{"title":"Venda e aluguel","listing_type":"sale","status":"in_portfolio",
		"available_for":["sale","rent"],"pricing":{"sale_price":{"current_amount":500000},"rent_price":{"current_amount":3000}}}`)
	seed("aaaaaaaa-0000-4000-8000-000000000006", otherTenant, property("De outro tenant", "sale", "published",
		`,"pricing":{"sale_price":{"current_amount":999999},"rent_price":{"current_amount":0}}`))
	// Sem preço de venda, mas com aluguel cadastrado: na busca de VENDA o valor
	// não pode ser o do aluguel (era o que o COALESCE cruzado fazia).
	// O cadastro pode simplesmente NÃO ter a chave do preço de venda (imóvel
	// captado para locação e depois marcado como disponível para venda): é aí que
	// o COALESCE cruzado devolvia o aluguel como preço de venda.
	seed("aaaaaaaa-0000-4000-8000-000000000007", tenant, `{"title":"Venda sem preço definido","listing_type":"sale","status":"published",
		"available_for":["sale"],"pricing":{"rent_price":{"current_amount":9000}}}`)

	repo := NewPropertyPostgresRepository(&database.PostgresDB{DB: db})
	ctx := context.Background()

	type row struct {
		id      string
		price   float64
		listing string
	}
	search := func(dealType deal.Type, listingType string) map[string]row {
		t.Helper()
		found, err := repo.Search(ctx, tenant, dealType, SearchParams{ListingType: listingType, Limit: 50})
		if err != nil {
			t.Fatalf("search(%s, %s): %v", dealType, listingType, err)
		}
		out := map[string]row{}
		for _, p := range found {
			out[p.ID] = row{id: p.ID, price: p.Price, listing: p.ListingType}
		}
		return out
	}

	sale := search(deal.TypeSale, "sale")
	if len(sale) != 3 {
		t.Fatalf("busca de venda devolveu %d imóveis, want 3: %+v", len(sale), sale)
	}
	if got := sale["aaaaaaaa-0000-4000-8000-000000000007"]; got.price != 0 {
		t.Fatalf("venda sem preço definido exibiu %.2f — valor do ALUGUEL apresentado como preço de venda", got.price)
	}
	if got := sale["aaaaaaaa-0000-4000-8000-000000000001"]; got.price != 450000 || got.listing != "sale" {
		t.Fatalf("imóvel à venda: %+v, want preço 450000 e listing sale", got)
	}
	if _, ok := sale["aaaaaaaa-0000-4000-8000-000000000002"]; ok {
		t.Fatal("imóvel SÓ DE ALUGUEL apareceu na busca de venda (valor cruzado: aluguel como preço de venda)")
	}
	if _, ok := sale["aaaaaaaa-0000-4000-8000-000000000003"]; ok {
		t.Fatal("imóvel VENDIDO apareceu na busca do portal")
	}
	if _, ok := sale["aaaaaaaa-0000-4000-8000-000000000004"]; ok {
		t.Fatal("imóvel em RASCUNHO apareceu na busca do portal")
	}
	if _, ok := sale["aaaaaaaa-0000-4000-8000-000000000006"]; ok {
		t.Fatal("imóvel de outro tenant apareceu na busca do portal")
	}
	if got := sale["aaaaaaaa-0000-4000-8000-000000000005"]; got.price != 500000 {
		t.Fatalf("imóvel venda&aluguel na busca de venda: %+v, want preço de VENDA 500000", got)
	}

	// Aluguel: "Só aluguel" + "Venda e aluguel" (available_for). Vendido e
	// rascunho ficam fora; o imóvel só de venda também.
	rent := search(deal.TypeRent, "rent")
	if len(rent) != 2 {
		t.Fatalf("busca de aluguel devolveu %d imóveis, want 2: %+v", len(rent), rent)
	}
	if got := rent["aaaaaaaa-0000-4000-8000-000000000002"]; got.price != 2500 || got.listing != "rent" {
		t.Fatalf("imóvel de aluguel: %+v, want preço 2500 e listing rent", got)
	}
	if got := rent["aaaaaaaa-0000-4000-8000-000000000005"]; got.price != 3000 {
		t.Fatalf("imóvel venda&aluguel na busca de aluguel: %+v, want preço de ALUGUEL 3000", got)
	}

	// Sem `listing_type` explícito, a finalidade vem do tipo do negócio.
	byDeal := search(deal.TypeRent, "")
	if len(byDeal) != len(rent) {
		t.Fatalf("negócio de aluguel sem filtro explícito devolveu %d, want %d", len(byDeal), len(rent))
	}
}
