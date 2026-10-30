package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// authStub registra os contatos que chegaram ao ensure-client e devolve um
// account_id determinístico — é o contrato do toca-auth que o provisionamento
// usa para deixar o cliente apto a pedir o código do portal.
type authStub struct {
	mu       sync.Mutex
	calls    []map[string]string
	accounts map[string]string
	fail     bool
}

func newAuthStub(fail bool) *authStub {
	return &authStub{accounts: map[string]string{}, fail: fail}
}

func newAuthStubServer(t *testing.T, stub *authStub) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Key") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		accountID, failing := stub.record(body)
		if failing {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    map[string]string{"account_id": accountID},
		})
	}))
	t.Cleanup(server.Close)
	return server
}

func withAuthStub(t *testing.T, stub *authStub) {
	t.Helper()
	server := newAuthStubServer(t, stub)
	t.Setenv("AUTH_SERVICE_URL", server.URL)
	t.Setenv("INTERNAL_API_KEY", "test-internal-key")
}

// record guarda a chamada e resolve o account_id (idempotente por e-mail).
func (s *authStub) record(body map[string]string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, body)
	accountID := s.accounts[body["email"]]
	if accountID == "" && !s.fail {
		accountID = "acc-" + body["email"]
		s.accounts[body["email"]] = accountID
	}
	return accountID, s.fail
}

func (s *authStub) recorded() []map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]map[string]string, len(s.calls))
	copy(out, s.calls)
	return out
}

func provisioningDeal() *deal.Deal {
	return &deal.Deal{
		ID:       "deal-1",
		TenantID: "tenant-1",
		Parties: []deal.Party{
			{ID: "party-buyer", Role: deal.PartyRoleBuyer, Name: "Maria Souza", Email: "maria@example.com", Phone: "88999999999"},
			{ID: "party-owner", Role: deal.PartyRoleOwner, Name: "Ayla Mizuke", Email: "ayla@example.com"},
			{ID: "party-noemail", Role: deal.PartyRoleBuyer, Name: "Sem Email"},
			{ID: "party-has-account", Role: deal.PartyRoleTenant, Name: "Já Tem Conta", Email: "conta@example.com", AccountID: "acc-existente"},
		},
		Signers: []deal.Signer{
			{ID: "party-buyer", Name: "Maria Souza", Email: "maria@example.com"},
		},
	}
}

// A criação do negócio precisa deixar TODA parte com e-mail com conta de
// portal: sem ela o pedido de código não encontra o contato e a pessoa não
// recebe nada (o toca-auth responde 200 genérico e descarta o pedido) — foi o
// caso do vendedor, que não era provisionado.
func TestProvisionPortalAccounts_CreatesAccountForEveryContact(t *testing.T) {
	stub := newAuthStub(false)
	withAuthStub(t, stub)
	repo := newInMemoryDealRepo()
	svc := &DealService{repo: repo}
	d := provisioningDeal()

	svc.provisionPortalAccounts(context.Background(), d)

	calls := stub.recorded()
	if len(calls) != 2 {
		t.Fatalf("esperava uma chamada por contato sem conta (comprador e vendedor), veio %d: %v", len(calls), calls)
	}
	if calls[0]["email"] != "maria@example.com" || calls[0]["tenant_id"] != "tenant-1" || calls[0]["phone"] != "88999999999" {
		t.Fatalf("payload do ensure-client fora do esperado: %v", calls[0])
	}
	if calls[1]["email"] != "ayla@example.com" {
		t.Fatalf("vendedor (owner) também precisa de conta de portal: %v", calls[1])
	}

	if d.Parties[0].AccountID != "acc-maria@example.com" {
		t.Fatalf("comprador deveria receber a conta, veio %q", d.Parties[0].AccountID)
	}
	if d.Parties[1].AccountID != "acc-ayla@example.com" {
		t.Fatalf("vendedor deveria receber a conta, veio %q", d.Parties[1].AccountID)
	}
	if d.Signers[0].AccountID != "acc-maria@example.com" {
		t.Fatalf("signatário com o mesmo e-mail deveria receber a conta, veio %q", d.Signers[0].AccountID)
	}
	if d.Parties[2].AccountID != "" {
		t.Fatalf("parte sem e-mail não tem identidade de portal: %q", d.Parties[2].AccountID)
	}
	if d.Parties[3].AccountID != "acc-existente" {
		t.Fatalf("conta existente não pode ser trocada: %q", d.Parties[3].AccountID)
	}
	if got := repo.deals["deal-1"]; got == nil || got.Parties[1].AccountID != "acc-ayla@example.com" {
		t.Fatalf("provisionamento deveria ser persistido: %+v", got)
	}

	// Reexecutar não duplica chamada nem troca a conta já gravada.
	svc.provisionPortalAccounts(context.Background(), d)
	if got := len(stub.recorded()); got != 2 {
		t.Fatalf("segunda passada não deveria consultar o auth de novo, veio %d", got)
	}
}

// O mesmo e-mail em papéis diferentes (parte + signatário, cônjuge, etc.) é a
// mesma pessoa: uma chamada só e o account_id replicado.
func TestProvisionPortalAccounts_DeduplicatesSameEmail(t *testing.T) {
	stub := newAuthStub(false)
	withAuthStub(t, stub)
	svc := &DealService{repo: newInMemoryDealRepo()}
	d := &deal.Deal{
		ID:       "deal-1",
		TenantID: "tenant-1",
		Parties: []deal.Party{
			{ID: "p1", Role: deal.PartyRoleBuyer, Name: "Maria Souza", Email: "Maria@Example.com"},
			{ID: "p2", Role: deal.PartyRoleSpouse, Name: "Maria Souza", Email: "maria@example.com"},
		},
		Signers: []deal.Signer{{ID: "p1", Name: "Maria Souza", Email: " maria@example.com "}},
	}

	if !svc.ProvisionPortalAccounts(context.Background(), d) {
		t.Fatal("esperava mudança no agregado")
	}

	if got := len(stub.recorded()); got != 1 {
		t.Fatalf("e-mail repetido deve gerar uma chamada, veio %d: %v", got, stub.recorded())
	}
	for i, party := range d.Parties {
		if !strings.EqualFold(party.AccountID, "acc-maria@example.com") {
			t.Fatalf("party %d sem conta replicada: %q", i, party.AccountID)
		}
	}
	if !strings.EqualFold(d.Signers[0].AccountID, "acc-maria@example.com") {
		t.Fatalf("signatário sem conta replicada: %q", d.Signers[0].AccountID)
	}
}

// Signatário cadastrado direto na Esteira (sem parte correspondente) também é
// gente do negócio e assina pelo portal.
func TestProvisionPortalAccounts_CoversSignerWithoutParty(t *testing.T) {
	stub := newAuthStub(false)
	withAuthStub(t, stub)
	svc := &DealService{repo: newInMemoryDealRepo()}
	d := &deal.Deal{
		ID:       "deal-1",
		TenantID: "tenant-1",
		Parties:  []deal.Party{{ID: "p1", Role: deal.PartyRoleBuyer, Name: "Maria Souza", Email: "maria@example.com", AccountID: "acc-maria"}},
		Signers:  []deal.Signer{{ID: "s9", Name: "Testemunha Um", Email: "testemunha@example.com"}},
	}

	if !svc.ProvisionPortalAccounts(context.Background(), d) {
		t.Fatal("esperava mudança no agregado")
	}

	if got := len(stub.recorded()); got != 1 || stub.recorded()[0]["email"] != "testemunha@example.com" {
		t.Fatalf("signatário sem parte deveria ser provisionado: %v", stub.recorded())
	}
	if d.Signers[0].AccountID != "acc-testemunha@example.com" {
		t.Fatalf("signatário sem conta: %q", d.Signers[0].AccountID)
	}
}

// Parte adicionada depois da criação (fluxo da Esteira) entra com conta.
func TestAddParty_ProvisionsPortalAccount(t *testing.T) {
	stub := newAuthStub(false)
	withAuthStub(t, stub)
	svc, repo := newServiceWithFakeRepo()
	existing := &deal.Deal{
		ID:       "deal-1",
		TenantID: "tenant-1",
		Type:     deal.TypeSale,
		Parties:  []deal.Party{{ID: "p1", Role: deal.PartyRoleBuyer, Name: "Maria Souza", Email: "maria@example.com", AccountID: "acc-maria"}},
	}
	repo.deals["deal-1"] = existing

	if _, err := svc.AddParty(context.Background(), "tenant-1", "deal-1", "", deal.Party{
		ID: "p2", Role: deal.PartyRoleOwner, Name: "Ayla Mizuke", Email: "ayla@example.com",
	}, "user-1"); err != nil {
		t.Fatalf("AddParty() error = %v", err)
	}

	if len(stub.recorded()) != 1 || stub.recorded()[0]["email"] != "ayla@example.com" {
		t.Fatalf("nova parte deveria ser provisionada: %v", stub.recorded())
	}
	stored := repo.deals["deal-1"]
	found := false
	for _, party := range stored.Parties {
		if party.Email == "ayla@example.com" && party.AccountID == "acc-ayla@example.com" {
			found = true
		}
	}
	if !found {
		t.Fatalf("conta da nova parte não foi persistida: %+v", stored.Parties)
	}
}

// Signatário cadastrado depois da criação entra com conta.
func TestAddSigner_ProvisionsPortalAccount(t *testing.T) {
	stub := newAuthStub(false)
	withAuthStub(t, stub)
	svc, repo := newServiceWithFakeRepo()
	repo.deals["deal-1"] = &deal.Deal{ID: "deal-1", TenantID: "tenant-1", Type: deal.TypeSale}

	if _, err := svc.AddSigner(context.Background(), "tenant-1", "deal-1", "", "Testemunha Um", "testemunha@example.com", deal.SignatoryRoleTestemunha, nil, "user-1"); err != nil {
		t.Fatalf("AddSigner() error = %v", err)
	}

	if len(stub.recorded()) != 1 || stub.recorded()[0]["email"] != "testemunha@example.com" {
		t.Fatalf("novo signatário deveria ser provisionado: %v", stub.recorded())
	}
	signers := repo.deals["deal-1"].Signers
	if len(signers) != 1 || signers[0].AccountID != "acc-testemunha@example.com" {
		t.Fatalf("signatário sem conta de portal: %+v", signers)
	}
}

// E-mail completado depois (cadastro incompleto do lead) também provisiona: o
// operador corrige o contato e a pessoa passa a conseguir entrar.
func TestUpdateParty_ProvisionsWhenEmailArrives(t *testing.T) {
	stub := newAuthStub(false)
	withAuthStub(t, stub)
	svc, repo := newServiceWithFakeRepo()
	repo.deals["deal-1"] = &deal.Deal{
		ID:       "deal-1",
		TenantID: "tenant-1",
		Type:     deal.TypeSale,
		Parties:  []deal.Party{{ID: "p1", Role: deal.PartyRoleOwner, Name: "Ayla Mizuke"}},
	}
	email := "ayla@example.com"

	if _, err := svc.UpdateParty(context.Background(), "tenant-1", "deal-1", "", "p1", nil, nil, nil, nil, &PartyIdentityPatch{Email: &email}, "user-1"); err != nil {
		t.Fatalf("UpdateParty() error = %v", err)
	}

	if len(stub.recorded()) != 1 || stub.recorded()[0]["email"] != "ayla@example.com" {
		t.Fatalf("e-mail novo deveria provisionar conta: %v", stub.recorded())
	}
	if got := repo.deals["deal-1"].Parties[0].AccountID; got != "acc-ayla@example.com" {
		t.Fatalf("conta não persistida na parte: %q", got)
	}
}

// Indisponibilidade do serviço de identidade não pode derrubar a criação do
// negócio — a pessoa fica sem conta e o convite manual segue como reparo.
func TestProvisionPortalAccounts_AuthFailureDoesNotBreakTheDeal(t *testing.T) {
	stub := newAuthStub(true)
	withAuthStub(t, stub)
	repo := newInMemoryDealRepo()
	svc := &DealService{repo: repo}
	d := provisioningDeal()

	svc.provisionPortalAccounts(context.Background(), d)

	if d.Parties[0].AccountID != "" {
		t.Fatalf("sem auth não há conta: %q", d.Parties[0].AccountID)
	}
	if len(stub.recorded()) != 2 {
		t.Fatalf("esperava uma tentativa por contato, veio %d", len(stub.recorded()))
	}
}

// Sem configuração do serviço de identidade o provisionamento é inerte (não
// entra em pânico nem inventa conta) — o negócio continua sendo criado.
func TestProvisionPortalAccounts_UnconfiguredAuthIsNoop(t *testing.T) {
	t.Setenv("AUTH_SERVICE_URL", "")
	t.Setenv("INTERNAL_API_KEY", "")
	svc := &DealService{repo: newInMemoryDealRepo()}
	d := provisioningDeal()

	svc.provisionPortalAccounts(context.Background(), d)

	if d.Parties[0].AccountID != "" {
		t.Fatalf("sem serviço configurado não há conta: %q", d.Parties[0].AccountID)
	}
}

// A criação do negócio pela esteira chama o provisionamento: é o caminho que o
// operador usa ao iniciar um negócio novo.
func TestCreate_ProvisionsClientPortalAccount(t *testing.T) {
	stub := newAuthStub(false)
	withAuthStub(t, stub)
	svc, repo := newServiceWithFakeRepo()

	// Nome completo: AddSigner valida o nome da pessoa (o cadastro real usa o
	// nome do lead; "Cliente" de um só nome é recusado pela validação).
	d, err := svc.Create(context.Background(), deal.TypeSale,
		"prop-1", "owner-1", "customer-1", "broker-1", "customer-1",
		500000, "tenant-1", "Maria Souza", "cliente@example.com", "", "comprador",
		PartyProfileHints{}, false)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	created := repo.deals[d.ID]
	if created == nil {
		t.Fatal("negócio não foi persistido")
	}
	if len(stub.recorded()) != 1 {
		t.Fatalf("criação deveria provisionar o cliente uma vez, veio %d: %v", len(stub.recorded()), stub.recorded())
	}
	if stub.recorded()[0]["email"] != "cliente@example.com" {
		t.Fatalf("e-mail provisionado = %q", stub.recorded()[0]["email"])
	}
	provisioned := false
	for _, party := range created.Parties {
		if party.Email == "cliente@example.com" && party.AccountID != "" {
			provisioned = true
		}
	}
	if !provisioned {
		t.Fatalf("parte do cliente deveria sair com conta de portal: %+v", created.Parties)
	}
	// O evento de criação precisa carregar a conta: é por ela que o toca-comm
	// vincula o cliente ao canal do negócio.
	if evt := NewDealCreatedEvent(created); evt.CustomerAccountID == "" {
		t.Fatalf("deal.created deveria levar customer_account_id: %+v", evt)
	}
}
