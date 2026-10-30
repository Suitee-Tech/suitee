package services

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// leadTenantsRepo é o dublê do repositório de leads usado pela lista de canais
// do portal (`ClientLeadTenants`): o vínculo cliente ↔ imobiliária que existe
// pelo ATENDIMENTO, sem negócio nenhum.
type leadTenantsRepo struct {
	byAccount map[string][]string
	err       error
}

func (r *leadTenantsRepo) GetInterest(context.Context, string, string) (interface{}, error) {
	return nil, nil
}

func (r *leadTenantsRepo) Exists(context.Context, string, string) (bool, error) { return false, nil }

func (r *leadTenantsRepo) ClientLeadTenants(_ context.Context, accountID string) ([]string, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.byAccount[accountID], nil
}

// P1/A3+A9 — a lista de canais do portal passa a ser tenant-scoped.
//
// O `ListChannelsByParticipant` do toca-comm era `?participant=` sem tenant
// ("across all tenants"): com a chave interna, listava canais de qualquer
// tenant. Como o `tenant_id` do cliente do portal é a própria conta (VerifyOTP),
// o tenant do canal tem de vir dos NEGÓCIOS em que ele é parte — é o que este
// teste trava: um tenant por negócio, união por participante, sem chamar o
// toca-comm sem tenant.
func TestListMyChannelsQueriesOnlyTenantsFromTheClientsDeals(t *testing.T) {
	t.Setenv("COMM_SERVICE_URL", "http://comm.test")
	t.Setenv("INTERNAL_API_KEY", "test-key")
	const account = "acc-cliente"
	tenantA := "11111111-1111-4111-8111-111111111111"
	tenantB := "22222222-2222-4222-8222-222222222222"

	repo := newInMemoryDealRepo(
		&deal.Deal{ID: "deal-a", TenantID: tenantA},
		&deal.Deal{ID: "deal-b", TenantID: tenantA}, // mesmo tenant: uma consulta só
		&deal.Deal{ID: "deal-c", TenantID: tenantB},
	)
	repo.partyAccountID = account

	var paths []string
	comm := clients.NewCommClient()
	comm.BaseClient.HTTPClient.Transport = portalVisitTransport(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path+"?"+r.URL.RawQuery)
		if !strings.Contains(r.URL.RawQuery, "tenant_id=") {
			t.Errorf("chamada ao toca-comm SEM tenant: %s", r.URL.RawQuery)
		}
		tenant := r.URL.Query().Get("tenant_id")
		body, _ := json.Marshal(map[string]any{
			"success": true,
			"data": []map[string]any{
				{"id": "canal-" + tenant[:4], "tenant_id": tenant, "participants": []string{account}, "updated_at": "2026-09-19T10:00:00Z"},
				{"id": "canal-duplicado", "tenant_id": tenant, "participants": []string{account}, "updated_at": "2026-09-19T09:00:00Z"},
			},
		})
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
	})

	svc := &DealService{repo: repo, commClient: comm}
	channels, err := svc.ListMyChannels(context.Background(), account)
	if err != nil {
		t.Fatalf("ListMyChannels: %v", err)
	}

	if len(paths) != 2 {
		t.Fatalf("esperava 2 consultas (um por tenant distinto), veio %d: %v", len(paths), paths)
	}
	for _, p := range paths {
		if !strings.Contains(p, "participant="+account) {
			t.Errorf("consulta sem o participante: %s", p)
		}
	}

	// O canal "duplicado" aparece nos dois tenants: a união não pode repetir.
	ids := map[string]int{}
	for _, ch := range channels {
		ids[ch.ID]++
	}
	for id, n := range ids {
		if n > 1 {
			t.Errorf("canal %s apareceu %d vezes na lista do portal", id, n)
		}
	}
	if len(channels) == 0 {
		t.Fatal("nenhum canal devolvido")
	}

	// Ordenação: mais recente primeiro (o portal depende disso para a lista).
	if !sort.SliceIsSorted(channels, func(i, j int) bool { return channels[i].UpdatedAt > channels[j].UpdatedAt }) {
		t.Errorf("canais fora de ordem de atualização: %+v", channels)
	}
}

// Sem negócio E sem atendimento, nenhuma consulta ao toca-comm — e nenhuma lista
// cross-tenant.
func TestListMyChannelsWithoutAnyRelationshipDoesNotQueryComm(t *testing.T) {
	t.Setenv("COMM_SERVICE_URL", "http://comm.test")
	repo := newInMemoryDealRepo()
	leadRepo := &leadTenantsRepo{byAccount: map[string][]string{}}

	called := false
	comm := clients.NewCommClient()
	comm.BaseClient.HTTPClient.Transport = portalVisitTransport(func(*http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"success":true,"data":[]}`)), Header: make(http.Header)}, nil
	})

	svc := &DealService{repo: repo, leadRepo: leadRepo, commClient: comm}
	channels, err := svc.ListMyChannels(context.Background(), "acc-sem-negocio")
	if err != nil {
		t.Fatalf("ListMyChannels: %v", err)
	}
	if called {
		t.Fatal("o toca-comm foi consultado sem nenhum tenant derivável")
	}
	if len(channels) != 0 {
		t.Fatalf("esperava lista vazia, veio %+v", channels)
	}
}

// Incidente "corretor inicia a mensagem com o lead e o cliente convidado não vê
// nada no portal" (2026-09-21): sem NEGÓCIO nenhum, o vínculo do cliente com a
// imobiliária existe só no ATENDIMENTO (lead do proprietário). Derivar os
// tenants apenas de `deal.deals` fazia a lista sair vazia sem sequer consultar o
// toca-comm — a conversa do corretor ficava invisível para o dono.
func TestListMyChannelsDerivesTenantsFromTheClientsLeads(t *testing.T) {
	t.Setenv("COMM_SERVICE_URL", "http://comm.test")
	t.Setenv("INTERNAL_API_KEY", "test-key")
	const account = "5b11063c-a7df-4a2e-b1fd-cfe9cdab31e6"
	const leadID = "4de1b561-919b-45b8-ad01-432859a7c57a"
	tenantDaImobiliaria := "de1c7bb5-b93f-4d4d-86e7-9430f94c404b"

	// Nenhum negócio: só o atendimento (lead) liga o cliente à imobiliária.
	repo := newInMemoryDealRepo()
	repo.partyAccountID = account
	leadRepo := &leadTenantsRepo{byAccount: map[string][]string{account: {tenantDaImobiliaria}}}

	var paths []string
	comm := clients.NewCommClient()
	comm.BaseClient.HTTPClient.Transport = portalVisitTransport(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path+"?"+r.URL.RawQuery)
		if got := r.URL.Query().Get("tenant_id"); got != tenantDaImobiliaria {
			t.Errorf("tenant consultado = %q, esperado %q", got, tenantDaImobiliaria)
		}
		if got := r.URL.Query().Get("participant"); got != account {
			t.Errorf("participante consultado = %q, esperado a conta do portal %q", got, account)
		}
		// O canal existe e nomeia o LEAD: quem traduz lead → conta é o toca-comm.
		body, _ := json.Marshal(map[string]any{
			"success": true,
			"data": []map[string]any{{
				"id":           "canal-anuncio-marcelo",
				"tenant_id":    tenantDaImobiliaria,
				"name":         "Anúncio Marcelo",
				"type":         "direct",
				"participants": []string{"c7f105dd-3e68-435b-a4cc-ea456ea52551", leadID},
				"updated_at":   "2026-09-21T00:19:59Z",
			}},
		})
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
	})

	svc := &DealService{repo: repo, leadRepo: leadRepo, commClient: comm}
	channels, err := svc.ListMyChannels(context.Background(), account)
	if err != nil {
		t.Fatalf("ListMyChannels: %v", err)
	}

	if len(paths) != 1 {
		t.Fatalf("esperava 1 consulta (tenant do atendimento), veio %d: %v", len(paths), paths)
	}
	if len(channels) != 1 || channels[0].ID != "canal-anuncio-marcelo" {
		t.Fatalf("conversa do corretor não chegou ao portal: %+v", channels)
	}
}

// Falha ao ler os ATENDIMENTOS não pode virar "você não tem canais" quando ela é
// a única explicação para a lista vazia.
func TestListMyChannelsSurfacesLeadReadFailureWhenNothingElseExplains(t *testing.T) {
	t.Setenv("COMM_SERVICE_URL", "http://comm.test")
	repo := newInMemoryDealRepo()
	leadRepo := &leadTenantsRepo{err: context.DeadlineExceeded}

	svc := &DealService{repo: repo, leadRepo: leadRepo, commClient: clients.NewCommClient()}
	if _, err := svc.ListMyChannels(context.Background(), "acc-cliente"); err == nil {
		t.Fatal("falha ao ler os atendimentos foi apresentada como lista vazia")
	}
}

// Indisponibilidade do toca-comm não pode virar "você não tem canais": sem
// nenhum dado, o erro sobe (a tela tem estado de erro); com dados parciais, a
// lista parcial é mostrada.
func TestListMyChannelsSurfacesCommFailureWhenNothingCouldBeRead(t *testing.T) {
	t.Setenv("COMM_SERVICE_URL", "http://comm.test")
	t.Setenv("INTERNAL_API_KEY", "test-key")
	const account = "acc-cliente"

	repo := newInMemoryDealRepo(&deal.Deal{ID: "deal-a", TenantID: "11111111-1111-4111-8111-111111111111"})
	repo.partyAccountID = account

	comm := clients.NewCommClient()
	comm.BaseClient.HTTPClient.Transport = portalVisitTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader(`{"message":"boom"}`)), Header: http.Header{}}, nil
	})

	svc := &DealService{repo: repo, commClient: comm}
	if _, err := svc.ListMyChannels(context.Background(), account); err == nil {
		t.Fatal("falha do toca-comm foi apresentada como lista vazia")
	}
}

func TestMyDealContactIncludesClientAndResponsibleBroker(t *testing.T) {
	const tenant = "11111111-1111-4111-8111-111111111111"
	selected := &deal.Deal{ID: "my-deal", TenantID: tenant, BrokerID: "responsible-broker"}
	var request clients.EnsureChannelRequest
	base := clients.NewBaseClient("http://comm.test")
	base.InternalAPIKey = "test-key"
	base.HTTPClient.Transport = portalVisitTransport(func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"data":{"id":"channel"}}`)), Header: make(http.Header)}, nil
	})
	svc := &DealService{repo: portalVisitDeals{selected: selected}, commClient: &clients.CommClient{BaseClient: base}}
	ch, err := svc.GetMyDealChannel(context.Background(), "portal-client", "", "my-deal")
	if err != nil || ch.ID != "channel" {
		t.Fatalf("contact failed: %+v %v", ch, err)
	}
	if len(request.Participants) != 2 || request.Participants[0] != "portal-client" || request.Participants[1] != "responsible-broker" {
		t.Fatalf("conversation missing recipient: %v", request.Participants)
	}
	if _, err = svc.GetMyDealChannel(context.Background(), "portal-client", "", "other-deal"); err == nil {
		t.Fatal("opened unrelated deal")
	}
}
