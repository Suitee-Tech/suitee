// Authentication fixture and HTTP assertions for the disposable Compose stack.
// It exposes public JWKS only: tokens are signed by `run` via a container-local
// private key. Business requests always use the real application services.
package main

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"
)

const (
	tenant    = "11111111-1111-4111-8111-111111111101"
	account   = "11111111-1111-4111-8111-111111111102"
	leadID    = "11111111-1111-4111-8111-111111111103"
	property  = "11111111-1111-4111-8111-111111111104"
	keyFile   = "/tmp/test-jwt-key.pem"
	stateFile = "/tmp/test-visit-state.json"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("expected serve, run, verify-restart, financial, verify-financial-restart, deal-pipeline or roulette-pipeline")
	}
	var err error
	switch os.Args[1] {
	case "serve":
		err = serve()
	case "run":
		err = journey()
	case "verify-restart":
		err = verifyRestart()
	case "financial":
		err = financialJourney()
	case "verify-financial-restart":
		err = verifyFinancialRestart()
	case "deal-pipeline":
		err = dealPipelineJourney()
	case "roulette-pipeline":
		err = roulettePipelineJourney()
	default:
		err = fmt.Errorf("unknown fixture command")
	}
	if err != nil {
		log.Fatal(err)
	}
}

func serve() error {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600); err != nil {
		return err
	}
	jwk := map[string]any{"kty": "RSA", "use": "sig", "alg": "RS256", "kid": "isolated-stack-test", "n": encode(key.N.Bytes()), "e": "AQAB"}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /auth/v1/.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{jwk}})
	})
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	return (&http.Server{Addr: ":9000", Handler: mux, ReadHeaderTimeout: 5 * time.Second}).ListenAndServe()
}

func encode(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func token(id, role, tenantID string) (string, error) {
	return tokenWithEmail(id, role, tenantID, id+"@example.invalid")
}

func tokenWithEmail(id, role, tenantID, email string) (string, error) {
	b, err := os.ReadFile(keyFile)
	if err != nil {
		return "", err
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return "", fmt.Errorf("invalid fixture key")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return "", err
	}
	claims, _ := json.Marshal(map[string]any{"sub": id, "account_id": id, "tenant_id": tenantID,
		"email": email, "functional_role": role, "role": "authenticated", "permission": "owner", "token_type": "access", "exp": time.Now().Add(time.Hour).Unix()})
	unsigned := encode([]byte(`{"alg":"RS256","kid":"isolated-stack-test","typ":"JWT"}`)) + "." + encode(claims)
	hash := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		return "", err
	}
	return unsigned + "." + encode(sig), nil
}

type visit struct {
	AppointmentID string `json:"appointment_id"`
	Date          string `json:"date"`
	Time          string `json:"time"`
	Status        string `json:"status"`
}
type overviewLead struct {
	ID    string `json:"id"`
	Phone string `json:"phone"`
	Visit *visit `json:"visit_request"`
}
type appointment struct {
	Title             string `json:"title"`
	UpdatedAt         string `json:"updated_at"`
	ID                string `json:"id"`
	LeadID            string `json:"lead_id"`
	TenantID          string `json:"tenant_id"`
	CustomerAccountID string `json:"customer_account_id"`
	Date              string `json:"date"`
	Time              string `json:"time"`
	Status            string `json:"status"`
}

func request(method, path, jwt string, body any, expected int, out any) error {
	base := os.Getenv("GATEWAY_URL")
	if strings.HasPrefix(path, "/contracts/") && os.Getenv("DEAL_SERVICE_URL") != "" {
		base = os.Getenv("DEAL_SERVICE_URL")
	}
	if base == "" {
		return fmt.Errorf("GATEWAY_URL required")
	}
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(method, base+"/api/v1"+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "isolated-financial-receipt")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != expected {
		return fmt.Errorf("%s %s: expected %d, received %d: %s", method, path, expected, resp.StatusCode, raw)
	}
	if out != nil {
		var envelope struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return err
		}
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return err
		}
	}
	return nil
}

func portal(jwt string) (overviewLead, error) {
	var leads []overviewLead
	if err := request("GET", "/client-portal/overview", jwt, nil, 200, &leads); err != nil {
		return overviewLead{}, err
	}
	if len(leads) != 1 || leads[0].ID != leadID {
		return overviewLead{}, fmt.Errorf("portal lead identity mismatch: %+v", leads)
	}
	return leads[0], nil
}

func journey() error {
	client, err := token(account, "cliente", account)
	if err != nil {
		return err
	}
	agent, err := token("11111111-1111-4111-8111-111111111105", "corretor", tenant)
	if err != nil {
		return err
	}
	outsider, err := token("11111111-1111-4111-8111-111111111106", "cliente", "11111111-1111-4111-8111-111111111106")
	if err != nil {
		return err
	}
	path := "/client-portal/leads/" + leadID
	date := time.Now().UTC().AddDate(0, 0, 7).Format("2006-01-02")
	slot := map[string]any{"property_id": property, "date": date, "time": "10:00", "end_time": "11:00", "note": "Jornada isolada"}
	if err := request("POST", path+"/visit-request", outsider, slot, 403, nil); err != nil {
		return err
	}
	if err := request("POST", path+"/channel", outsider, nil, 403, nil); err != nil {
		return err
	}
	for i := 0; i < 2; i++ {
		if err := request("POST", path+"/visit-request", client, slot, 200, nil); err != nil {
			return err
		}
	}
	p, err := portal(client)
	if err != nil {
		return err
	}
	if p.Visit == nil || p.Visit.AppointmentID == "" || p.Visit.Status != "Aguardando" {
		return fmt.Errorf("pending visit missing: %+v", p)
	}
	id := p.Visit.AppointmentID
	var a appointment
	if err := request("GET", "/appointments/"+id, agent, nil, 200, &a); err != nil {
		return err
	}
	if a.LeadID != leadID || a.TenantID != tenant || a.CustomerAccountID != account || a.Status != "Pendente" {
		return fmt.Errorf("agenda linkage mismatch: %+v", a)
	}
	if err := request("GET", "/appointments/"+id, client, nil, 403, nil); err != nil {
		return err
	}
	otherAgent, err := token("11111111-1111-4111-8111-111111111107", "corretor", "11111111-1111-4111-8111-111111111108")
	if err != nil {
		return err
	}
	if err := request("PUT", "/appointments/"+id, otherAgent, map[string]any{"status": "Confirmado"}, 403, nil); err != nil {
		return err
	}
	if a.UpdatedAt == "" {
		return fmt.Errorf("agenda did not expose revision")
	}
	staleRevision := a.UpdatedAt
	if err := request("PUT", "/appointments/"+id, agent, map[string]any{"title": "Título atualizado na agenda", "expected_updated_at": staleRevision}, 200, nil); err != nil {
		return err
	}
	if err := request("PUT", "/appointments/"+id, agent, map[string]any{"status": "Confirmado", "expected_updated_at": staleRevision}, 409, nil); err != nil {
		return err
	}
	if err := request("GET", "/appointments/"+id, agent, nil, 200, &a); err != nil {
		return err
	}
	if a.Title != "Título atualizado na agenda" || a.Status != "Pendente" {
		return fmt.Errorf("stale patch overwrote newer title/status: %+v", a)
	}
	fmt.Println("[test-stack] PASS stale calendar revision rejected; concurrent title preserved")
	if err := request("PUT", "/appointments/"+id, agent, map[string]any{"status": "Confirmado"}, 200, nil); err != nil {
		return err
	}
	p, err = portal(client)
	if err != nil {
		return err
	}
	if p.Visit == nil || p.Visit.Status != "Confirmado" {
		return fmt.Errorf("confirmation not visible in portal: %+v", p.Visit)
	}
	if err := request("PUT", path+"/visit", client, map[string]any{"date": date, "time": "12:00", "end_time": "13:00"}, 200, nil); err != nil {
		return err
	}
	p, err = portal(client)
	if err != nil {
		return err
	}
	if p.Visit == nil || p.Visit.AppointmentID != id || p.Visit.Time != "12:00" {
		return fmt.Errorf("reschedule duplicated or lost appointment: %+v", p.Visit)
	}
	if err := request("DELETE", path+"/visit", client, nil, 200, nil); err != nil {
		return err
	}
	p, err = portal(client)
	if err != nil {
		return err
	}
	if p.Visit == nil || p.Visit.Status != "Cancelado" {
		return fmt.Errorf("cancellation not visible")
	}
	if err := request("GET", "/appointments/"+id, agent, nil, 200, &a); err != nil {
		return err
	}
	if a.Status != "Cancelado" || a.Time != "12:00" {
		return fmt.Errorf("agenda cancellation mismatch: %+v", a)
	}
	b, err := json.Marshal(p.Visit)
	if err != nil {
		return err
	}
	if err := os.WriteFile(stateFile, b, 0600); err != nil {
		return err
	}
	fmt.Println("[test-stack] PASS portal -> agenda -> portal; retry, identity, agency isolation, confirmation, reschedule and cancellation")
	return nil
}

func verifyRestart() error {
	b, err := os.ReadFile(stateFile)
	if err != nil {
		return err
	}
	var expected visit
	if err := json.Unmarshal(b, &expected); err != nil {
		return err
	}
	client, err := token(account, "cliente", account)
	if err != nil {
		return err
	}
	p, err := portal(client)
	if err != nil {
		return err
	}
	if p.Visit == nil || *p.Visit != expected {
		return fmt.Errorf("visit changed after restart: %+v", p.Visit)
	}
	fmt.Println("[test-stack] PASS persisted visit after CRM/calendar restart")
	return nil
}

const qaContract = "33333333-3333-4333-8333-333333333303"
const financialStateFile = "/tmp/test-financial-state.json"

type financialCharge struct {
	ID          string            `json:"id"`
	ContractID  string            `json:"contract_id"`
	Status      string            `json:"status"`
	TotalAmount float64           `json:"total_amount"`
	PaidAmount  float64           `json:"paid_amount"`
	DueDate     string            `json:"due_date"`
	Payments    []json.RawMessage `json:"payments"`
}

func financialJourney() error {
	agent, err := token("11111111-1111-4111-8111-111111111105", "corretor", tenant)
	if err != nil {
		return err
	}
	for i := 0; i < 2; i++ {
		var c struct {
			Status string `json:"status"`
		}
		if err := request("POST", "/contracts/"+qaContract+"/sign", agent, map[string]any{"signatory_id": "33333333-3333-4333-8333-333333333304", "signature": "synthetic-test-signature"}, 200, &c); err != nil {
			return err
		}
		if c.Status != "SIGNED" {
			return fmt.Errorf("contract did not reach SIGNED: %s", c.Status)
		}
	}
	var charges []financialCharge
	var commissions []struct {
		ID         string  `json:"id"`
		ContractID string  `json:"contract_id"`
		Amount     float64 `json:"amount"`
		Status     string  `json:"status"`
	}
	for attempt := 0; attempt < 30; attempt++ {
		if err := request("GET", "/charges?contract_id="+qaContract+"&limit=100", agent, nil, 200, &charges); err != nil {
			return err
		}
		if err := request("GET", "/commissions/by-deal/33333333-3333-4333-8333-333333333301", agent, nil, 200, &commissions); err != nil {
			return err
		}
		if len(charges) > 0 && len(commissions) > 0 {
			break
		}
		time.Sleep(time.Second)
	}
	if len(charges) == 0 || len(commissions) != 1 || commissions[0].ContractID != qaContract || commissions[0].Amount != 2000 || commissions[0].Status != "PENDING" {
		return fmt.Errorf("contract obligations not provisioned once: charges=%d commissions=%+v", len(charges), commissions)
	}
	slices.SortFunc(charges, func(a, b financialCharge) int { return strings.Compare(a.DueDate, b.DueDate) })
	ch := charges[0]
	if ch.ContractID != qaContract || ch.Status != "PENDING" || ch.TotalAmount != 2000 {
		return fmt.Errorf("unexpected receivable: %+v", ch)
	}
	other, err := token("11111111-1111-4111-8111-111111111107", "corretor", "11111111-1111-4111-8111-111111111108")
	if err != nil {
		return err
	}
	if err := request("GET", "/charges/"+ch.ID, other, nil, 404, nil); err != nil {
		return err
	}
	paidAt := time.Now().UTC().Format(time.RFC3339Nano)
	for i := 0; i < 2; i++ {
		if err := request("POST", "/charges/"+ch.ID+"/reconcile-manual", agent, map[string]any{"paid_at": paidAt, "paid_amount": 2000, "payment_method": "PIX", "proof_url": "https://example.invalid/qa-proof", "note": "Recebimento sintético de teste"}, 200, &ch); err != nil {
			return err
		}
		if ch.Status != "PAID" || ch.PaidAmount != 2000 || len(ch.Payments) != 1 {
			return fmt.Errorf("manual receipt duplicated or not persisted: %+v", ch)
		}
	}
	b, err := json.Marshal(ch)
	if err != nil {
		return err
	}
	if err := os.WriteFile(financialStateFile, b, 0600); err != nil {
		return err
	}
	fmt.Println("[financial-integration] PASS contract -> worker obligations -> manual receipt; signing/receipt retry and agency isolation")
	return nil
}

func verifyFinancialRestart() error {
	b, err := os.ReadFile(financialStateFile)
	if err != nil {
		return err
	}
	var expected, actual financialCharge
	if err := json.Unmarshal(b, &expected); err != nil {
		return err
	}
	agent, err := token("11111111-1111-4111-8111-111111111105", "corretor", tenant)
	if err != nil {
		return err
	}
	if err := request("GET", "/charges/"+expected.ID, agent, nil, 200, &actual); err != nil {
		return err
	}
	if actual.ID != expected.ID || actual.ContractID != expected.ContractID || actual.Status != "PAID" || actual.PaidAmount != 2000 || len(actual.Payments) != 1 {
		return fmt.Errorf("manual receipt changed after restart: %+v", actual)
	}
	fmt.Println("[financial-integration] PASS manual receipt after deal/financial restart")
	return nil
}

const ownerLead = "11111111-1111-4111-8111-111111111110"
const agentAccount = "11111111-1111-4111-8111-111111111105"
const adminAccount = "11111111-1111-4111-8111-111111111109"

// dealPipelineJourney percorre a esteira completa sem provedor externo:
// negócio de locação → proposta → aceite do cliente (portal) → análise →
// contrato em minuta. Não chega ao Clicksign de propósito: a assinatura é
// dependência externa e o contrato assinado dispara efeitos financeiros.
func dealPipelineJourney() error {
	agent, err := token(agentAccount, "corretor", tenant)
	if err != nil {
		return err
	}
	admin, err := token(adminAccount, "admin", tenant)
	if err != nil {
		return err
	}
	client, err := token(account, "cliente", account)
	if err != nil {
		return err
	}

	// 1. O corretor abre o negócio com as partes (tenant + owner).
	var created struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	createBody := map[string]any{
		"type": "RENT", "property_id": property, "owner_id": ownerLead,
		"customer_id": leadID, "lead_id": leadID, "broker_id": agentAccount,
		"value": 2500.0,
		"parties": []map[string]any{
			{"role": "tenant", "lead_id": leadID, "account_id": account, "name": "Cliente de teste", "email": "client@example.invalid", "document": "12345678901", "no_spouse_declared": true, "is_primary": true},
			{"role": "owner", "lead_id": ownerLead, "name": "Proprietário de teste", "email": "owner@example.invalid", "document": "10987654321", "no_spouse_declared": true, "is_primary": true},
		},
	}
	if err := request("POST", "/deals", agent, createBody, 201, &created); err != nil {
		return fmt.Errorf("deal-pipeline: criar negócio: %w", err)
	}
	if created.ID == "" {
		return fmt.Errorf("deal-pipeline: negócio sem id: %+v", created)
	}

	// 2. Proposta do operador (avança o negócio para PROPOSAL).
	var withProposal struct {
		Status    string `json:"status"`
		Proposals []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"proposals"`
	}
	proposalBody := map[string]any{"value": 2500.0, "payment_method": "pix", "installments": 1}
	if err := request("POST", "/deals/"+created.ID+"/proposals", agent, proposalBody, 200, &withProposal); err != nil {
		return fmt.Errorf("deal-pipeline: propor: %w", err)
	}
	if withProposal.Status != "PROPOSAL" || len(withProposal.Proposals) == 0 {
		return fmt.Errorf("deal-pipeline: proposta não avançou para PROPOSAL: %+v", withProposal)
	}
	proposalID := withProposal.Proposals[len(withProposal.Proposals)-1].ID

	// 3. As partes obrigatórias aceitam a proposta pelo portal: o inquilino
	// (por account_id) e o proprietário (por email — lead sem conta).
	if err := request("POST", "/client-portal/deals/"+created.ID+"/proposals/"+proposalID+"/accept", client, nil, 200, nil); err != nil {
		return fmt.Errorf("deal-pipeline: aceite do cliente: %w", err)
	}
	owner, err := tokenWithEmail(ownerLead, "cliente", ownerLead, "owner@example.invalid")
	if err != nil {
		return err
	}
	if err := request("POST", "/client-portal/deals/"+created.ID+"/proposals/"+proposalID+"/accept", owner, nil, 200, nil); err != nil {
		return fmt.Errorf("deal-pipeline: aceite do proprietário: %w", err)
	}

	// 4. Análise (documentos conferidos nas partes) e contrato.
	for _, stage := range []string{"ANALYSIS", "CONTRACT"} {
		var d struct {
			Status string `json:"status"`
		}
		if err := request("PATCH", "/deals/"+created.ID+"/status", agent, map[string]any{"status": stage, "notes": "jornada isolada"}, 200, &d); err != nil {
			return fmt.Errorf("deal-pipeline: mover para %s: %w", stage, err)
		}
		if d.Status != stage {
			return fmt.Errorf("deal-pipeline: status após PATCH = %q, want %q", d.Status, stage)
		}
	}

	// 5. Minuta de contrato de locação (admin tem contracts:write).
	// RENT não passa pelo guard de plano SALE (T-01 é exclusivo de venda).
	var contract struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	contractBody := map[string]any{
		"deal_id": created.ID,
		"type":    "locacao",
		"signatories": []map[string]any{
			{"id": "sig-fixture-tenant", "name": "Cliente de teste", "email": "client@example.invalid", "role": "inquilino"},
			{"id": "sig-fixture-owner", "name": "Proprietário de teste", "email": "owner@example.invalid", "role": "proprietario"},
		},
		"content":    "Minuta de contrato da jornada isolada (deal-pipeline)",
		"start_date": time.Now().UTC().Format(time.RFC3339),
	}
	if err := request("POST", "/contracts", admin, contractBody, 201, &contract); err != nil {
		return fmt.Errorf("deal-pipeline: criar minuta: %w", err)
	}
	if contract.ID == "" || contract.Status != "DRAFT" {
		return fmt.Errorf("deal-pipeline: minuta divergente: %+v", contract)
	}
	var readBack struct {
		ID string `json:"id"`
	}
	if err := request("GET", "/contracts/"+contract.ID, admin, nil, 200, &readBack); err != nil {
		return fmt.Errorf("deal-pipeline: ler minuta: %w", err)
	}
	if readBack.ID != contract.ID {
		return fmt.Errorf("deal-pipeline: minuta ilegível: %+v", readBack)
	}
	fmt.Println("[test-stack] PASS deal-pipeline: negócio → proposta → aceite do cliente → análise → contrato (minuta)")
	return nil
}

// roulettePipelineJourney valida a configuração, a simulação determinística
// (mesma semente → mesma distribuição) e o histórico persistido da roleta.
func roulettePipelineJourney() error {
	admin, err := token(adminAccount, "admin", tenant)
	if err != nil {
		return err
	}

	var cfg struct {
		Enabled bool `json:"enabled"`
	}
	if err := request("PUT", "/leads/roulette", admin, map[string]any{
		"enabled":        true,
		"active_brokers": []string{agentAccount},
	}, 200, &cfg); err != nil {
		return fmt.Errorf("roulette-pipeline: configurar: %w", err)
	}
	if !cfg.Enabled {
		return fmt.Errorf("roulette-pipeline: config não ficou habilitada: %+v", cfg)
	}

	// Simulação determinística: mesma semente → mesma distribuição.
	type simulation struct {
		PoolSize    int `json:"pool_size"`
		Assignments []struct {
			N        int    `json:"n"`
			BrokerID string `json:"broker_id"`
		} `json:"assignments"`
	}
	var first, second simulation
	simBody := map[string]any{"count": 5, "seed": 42, "persist": false}
	if err := request("POST", "/leads/roulette/simulate", admin, simBody, 200, &first); err != nil {
		return fmt.Errorf("roulette-pipeline: simular (1): %w", err)
	}
	if err := request("POST", "/leads/roulette/simulate", admin, simBody, 200, &second); err != nil {
		return fmt.Errorf("roulette-pipeline: simular (2): %w", err)
	}
	if first.PoolSize != 1 || len(first.Assignments) != 5 {
		return fmt.Errorf("roulette-pipeline: distribuição divergente do pool: %+v", first)
	}
	for i := range first.Assignments {
		if first.Assignments[i].BrokerID != second.Assignments[i].BrokerID || first.Assignments[i].N != second.Assignments[i].N {
			return fmt.Errorf("roulette-pipeline: mesma semente deveria dar a mesma distribuição")
		}
	}
	if first.Assignments[0].BrokerID != agentAccount {
		return fmt.Errorf("roulette-pipeline: broker ativo não recebeu: %+v", first.Assignments[0])
	}

	// O ponteiro da roleta persiste entre simulações (CAS).
	if err := request("POST", "/leads/roulette/simulate", admin, map[string]any{"count": 2, "seed": 7, "persist": true}, 200, nil); err != nil {
		return fmt.Errorf("roulette-pipeline: simular persistente: %w", err)
	}

	// O pipeline real: um lead novo SEM corretor é distribuído pela roleta
	// (AssignByRoulette roda na criação) e o histórico é gravado.
	var created struct {
		ID               string `json:"id"`
		AssignedBrokerID string `json:"assigned_broker_id"`
	}
	leadBody := map[string]any{
		"name": "Lead da roleta isolada", "email": "roulette@example.invalid",
		"phone": "11900000099", "source": "MANUAL",
	}
	if err := request("POST", "/leads", admin, leadBody, 201, &created); err != nil {
		return fmt.Errorf("roulette-pipeline: criar lead: %w", err)
	}
	if created.ID == "" {
		return fmt.Errorf("roulette-pipeline: lead sem id: %+v", created)
	}
	// A distribuição roda em DOIS caminhos concorrentes (síncrono no create e
	// o consumidor de lead.created via RabbitMQ) — o DTO do create pode perder
	// a corrida. O estado real está no lead depois de persistido.
	assigned := ""
	for attempt := 0; attempt < 20; attempt++ {
		var got struct {
			AssignedBrokerID string `json:"assigned_broker_id"`
		}
		if err := request("GET", "/leads/"+created.ID, admin, nil, 200, &got); err != nil {
			return fmt.Errorf("roulette-pipeline: ler lead: %w", err)
		}
		assigned = got.AssignedBrokerID
		if assigned == agentAccount {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if assigned != agentAccount {
		return fmt.Errorf("roulette-pipeline: lead não distribuído ao corretor ativo: %+v", created)
	}

	var history struct {
		Items []json.RawMessage `json:"items"`
		Total int               `json:"total"`
	}
	if err := request("GET", "/leads/roulette/history", admin, nil, 200, &history); err != nil {
		return fmt.Errorf("roulette-pipeline: histórico: %w", err)
	}
	if len(history.Items) == 0 {
		return fmt.Errorf("roulette-pipeline: histórico vazio após distribuição real")
	}
	fmt.Println("[test-stack] PASS roulette-pipeline: config → simulação determinística → lead distribuído pela roleta → histórico")
	return nil
}
