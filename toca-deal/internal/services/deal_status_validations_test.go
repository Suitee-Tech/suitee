package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// inMemoryDealRepo é o mínimo que UpdateStatus precisa — apenas GetByID
// e Update funcionam. Os outros são stubs no-op só para satisfazer a
// interface deal.Repository.
type inMemoryDealRepo struct {
	deals map[string]*deal.Deal
	// partyAccountID é o account que "participa" dos deals semeados.
	partyAccountID string
}

func newInMemoryDealRepo(seed ...*deal.Deal) *inMemoryDealRepo {
	r := &inMemoryDealRepo{deals: map[string]*deal.Deal{}}
	for _, d := range seed {
		r.deals[d.ID] = d
	}
	return r
}

func (r *inMemoryDealRepo) Create(ctx context.Context, d *deal.Deal) error {
	r.deals[d.ID] = d
	return nil
}
func (r *inMemoryDealRepo) GetByID(ctx context.Context, tenantID, id string) (*deal.Deal, error) {
	if d, ok := r.deals[id]; ok && (tenantID == "" || d.TenantID == tenantID) {
		return d, nil
	}
	return nil, nil
}
func (r *inMemoryDealRepo) GetByIDScoped(ctx context.Context, tenantID, id, accountID string) (*deal.Deal, error) {
	return r.GetByID(ctx, tenantID, id)
}
func (r *inMemoryDealRepo) GetByIDInTx(ctx context.Context, tx *sqlx.Tx, tenantID, id string) (*deal.Deal, error) {
	return r.GetByID(ctx, tenantID, id)
}
func (r *inMemoryDealRepo) Update(ctx context.Context, d *deal.Deal) error {
	r.deals[d.ID] = d
	return nil
}
func (r *inMemoryDealRepo) UpdateInTx(ctx context.Context, tx *sqlx.Tx, d *deal.Deal) error {
	return r.Update(ctx, d)
}
func (r *inMemoryDealRepo) Delete(ctx context.Context, tenantID, id string) error {
	delete(r.deals, id)
	return nil
}
func (r *inMemoryDealRepo) DeleteScoped(ctx context.Context, tenantID, id, accountID string) error {
	delete(r.deals, id)
	return nil
}
func (r *inMemoryDealRepo) List(ctx context.Context, tenantID string, limit, offset int64) ([]*deal.Deal, int64, error) {
	return nil, 0, nil
}
func (r *inMemoryDealRepo) ListScoped(ctx context.Context, tenantID, accountID string, limit, offset int64) ([]*deal.Deal, int64, error) {
	return nil, 0, nil
}
func (r *inMemoryDealRepo) FindByPartyAccountID(ctx context.Context, accountID string) ([]*deal.Deal, error) {
	// P1/A3: o dublê precisa honrar o account — devolver tudo (ou nada) escondia
	// justamente o escopo que o serviço passou a derivar do cliente.
	if r.partyAccountID == "" || r.partyAccountID != accountID {
		return nil, nil
	}
	out := make([]*deal.Deal, 0, len(r.deals))
	for _, d := range r.deals {
		out = append(out, d)
	}
	return out, nil
}
func (r *inMemoryDealRepo) FindByPartyEmail(ctx context.Context, email string) ([]*deal.Deal, error) {
	return nil, nil
}
func (r *inMemoryDealRepo) FindByPartyLeadID(ctx context.Context, leadID string) ([]*deal.Deal, error) {
	return nil, nil
}
func (r *inMemoryDealRepo) ListByCustomerIDs(ctx context.Context, tenantID string, customerIDs []string, limit, offset int64) ([]*deal.Deal, int64, error) {
	return nil, 0, nil
}
func (r *inMemoryDealRepo) ListByCustomerIDsScoped(ctx context.Context, tenantID, accountID string, customerIDs []string, limit, offset int64) ([]*deal.Deal, int64, error) {
	return nil, 0, nil
}
func (r *inMemoryDealRepo) FindBySignerEmail(ctx context.Context, email string) ([]*deal.Deal, error) {
	return nil, nil
}
func (r *inMemoryDealRepo) FindBySignerIdentity(ctx context.Context, document, email string) ([]*deal.Deal, error) {
	return nil, nil
}
func (r *inMemoryDealRepo) GetStats(ctx context.Context, tenantID string) (int64, float64, error) {
	return 0, 0, nil
}
func (r *inMemoryDealRepo) GetStatsScoped(ctx context.Context, tenantID, accountID string) (int64, float64, error) {
	return 0, 0, nil
}
func (r *inMemoryDealRepo) Search(ctx context.Context, tenantID string, filter deal.DealFilter) ([]*deal.Deal, int64, error) {
	return nil, 0, nil
}

// helpers ────────────────────────────────────────────────────────────────

func boolPtr(v bool) *bool { return &v }

func dealReadyForContract() *deal.Deal {
	return &deal.Deal{
		ID:         "deal-1",
		TenantID:   "tenant-A",
		Type:       deal.TypeRent,
		Status:     deal.StatusAnalysis,
		PropertyID: "prop-1",
		BrokerID:   "broker-1",
		Proposals: []deal.Proposal{
			{
				ID:        "prop-1",
				Value:     1000,
				Status:    deal.ProposalStatusAccepted,
				CreatedAt: time.Now(),
			},
		},
		Parties: []deal.Party{
			{
				ID:                "p-tenant",
				Name:              "Inquilino Teste",
				Email:             "tenant@example.com",
				Document:          "111.111.111-11",
				Role:              deal.PartyRoleTenant,
				IsPrimary:         true,
				SignatureRequired: boolPtr(true),
				NoSpouseDeclared:  true,
			},
			{
				ID:                "p-owner",
				Name:              "Proprietário Teste",
				Email:             "owner@example.com",
				Document:          "222.222.222-22",
				Role:              deal.PartyRoleOwner,
				IsPrimary:         true,
				SignatureRequired: boolPtr(true),
				NoSpouseDeclared:  true,
			},
		},
	}
}

func testService(d *deal.Deal) *DealService {
	repo := newInMemoryDealRepo(d)
	return &DealService{
		repo: repo,
	}
}

// Tests ─────────────────────────────────────────────────────────────────

func TestUpdateStatus_HappyPath_AnalysisToContract(t *testing.T) {
	d := dealReadyForContract()
	svc := testService(d)
	got, err := svc.UpdateStatusTrusted(context.Background(), d.TenantID, d.ID, "", deal.StatusContract, "user-1", "ok")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if got.Status != deal.StatusContract {
		t.Errorf("expected CONTRACT, got %q", got.Status)
	}
}

func TestUpdateStatus_ProposalNotApproved_NoProposals(t *testing.T) {
	d := dealReadyForContract()
	d.Proposals = nil
	svc := testService(d)
	_, err := svc.UpdateStatusTrusted(context.Background(), d.TenantID, d.ID, "", deal.StatusContract, "user-1", "")
	if !errors.Is(err, ErrProposalNotApproved) {
		t.Errorf("expected ErrProposalNotApproved, got %v", err)
	}
}

func TestUpdateStatus_ProposalNotApproved_LatestNotAccepted(t *testing.T) {
	d := dealReadyForContract()
	d.Proposals[0].Status = deal.ProposalStatusCounter
	svc := testService(d)
	_, err := svc.UpdateStatusTrusted(context.Background(), d.TenantID, d.ID, "", deal.StatusContract, "user-1", "")
	if !errors.Is(err, ErrProposalNotApproved) {
		t.Errorf("expected ErrProposalNotApproved when latest=COUNTER, got %v", err)
	}
}

func TestUpdateStatus_PartiesPending_MissingEmail(t *testing.T) {
	d := dealReadyForContract()
	d.Parties[0].Email = ""
	svc := testService(d)
	_, err := svc.UpdateStatusTrusted(context.Background(), d.TenantID, d.ID, "", deal.StatusContract, "user-1", "")
	if !errors.Is(err, ErrPartiesPending) {
		t.Errorf("expected ErrPartiesPending for missing email, got %v", err)
	}
}

func TestUpdateStatus_PartiesPending_MissingDocument(t *testing.T) {
	d := dealReadyForContract()
	d.Parties[1].Document = ""
	svc := testService(d)
	_, err := svc.UpdateStatusTrusted(context.Background(), d.TenantID, d.ID, "", deal.StatusContract, "user-1", "")
	if !errors.Is(err, ErrPartiesPending) {
		t.Errorf("expected ErrPartiesPending for missing document, got %v", err)
	}
}

func TestUpdateStatus_PartiesPending_WhitespaceOnly(t *testing.T) {
	d := dealReadyForContract()
	d.Parties[0].Name = "   "
	svc := testService(d)
	_, err := svc.UpdateStatusTrusted(context.Background(), d.TenantID, d.ID, "", deal.StatusContract, "user-1", "")
	if !errors.Is(err, ErrPartiesPending) {
		t.Errorf("expected ErrPartiesPending for whitespace-only name, got %v", err)
	}
}

func TestUpdateStatus_SpouseMissing(t *testing.T) {
	d := dealReadyForContract()
	d.Parties[1].NoSpouseDeclared = false
	svc := testService(d)
	_, err := svc.UpdateStatusTrusted(context.Background(), d.TenantID, d.ID, "", deal.StatusContract, "user-1", "")
	if !errors.Is(err, ErrSpouseMissing) {
		t.Errorf("expected ErrSpouseMissing, got %v", err)
	}
}

func TestUpdateStatus_SpouseDeclared_NoError(t *testing.T) {
	d := dealReadyForContract()
	d.Parties[1].NoSpouseDeclared = false
	d.Parties = append(d.Parties, deal.Party{
		ID:               "p-spouse",
		Name:             "Cônjuge Teste",
		Email:            "spouse@example.com",
		Document:         "333.333.333-33",
		Role:             deal.PartyRoleSpouse,
		SpouseOfPartyID:  "p-owner",
		NoSpouseDeclared: true,
	})
	svc := testService(d)
	if _, err := svc.UpdateStatusTrusted(context.Background(), d.TenantID, d.ID, "", deal.StatusContract, "user-1", ""); err != nil {
		t.Fatalf("expected success when spouse is registered, got %v", err)
	}
}

func TestUpdateStatus_SpouseRole_NotChecked_ForPartiesPending(t *testing.T) {
	// Spouse com dados incompletos NÃO deve disparar ErrPartiesPending —
	// regras de cônjuge são validadas pelo spouse-check, não pelo
	// completude-de-dados. (No mundo real podemos exigir, mas o issue #412
	// pediu apenas para campos do principal.)
	d := dealReadyForContract()
	d.Parties[1].NoSpouseDeclared = false
	d.Parties = append(d.Parties, deal.Party{
		ID:                "p-spouse",
		Name:              "",
		Email:             "",
		Document:          "",
		Role:              deal.PartyRoleSpouse,
		SpouseOfPartyID:   "p-owner",
		SignatureRequired: boolPtr(true), // mesmo required, spouse é skip
		NoSpouseDeclared:  true,
	})
	svc := testService(d)
	if _, err := svc.UpdateStatusTrusted(context.Background(), d.TenantID, d.ID, "", deal.StatusContract, "user-1", ""); err != nil {
		// Pode falhar por outra razão — só queremos garantir que NÃO é
		// ErrPartiesPending por causa do spouse.
		if errors.Is(err, ErrPartiesPending) {
			t.Errorf("spouse role should not trigger ErrPartiesPending, got %v", err)
		}
	}
}

func TestUpdateStatus_TypedErrorsAreWrapped(t *testing.T) {
	// Verifica que errors.Is funciona — handlers HTTP dependem disso para
	// mapear 409 vs 422.
	d := dealReadyForContract()
	d.Parties[0].Email = ""
	svc := testService(d)
	_, err := svc.UpdateStatusTrusted(context.Background(), d.TenantID, d.ID, "", deal.StatusContract, "user-1", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrPartiesPending) {
		t.Errorf("errors.Is should match ErrPartiesPending, got %v", err)
	}
	// Mensagem deve incluir contexto (party_id) — facilita debug pelo operador
	if !contains(err.Error(), "party_id=p-tenant") {
		t.Errorf("error message should include party_id for debug, got %q", err.Error())
	}
}

// ─────────── gate de documento: ANALYSIS vs CONTRACT ───────────

func TestUpdateStatus_Analysis_AllowsMissingDocument(t *testing.T) {
	// Mover Proposta → Análise NÃO exige o documento das partes: é
	// justamente na etapa "Análise (Doc)" que o CPF/CNPJ é coletado. O
	// gate de documento vive só na transição para Contrato.
	d := dealReadyForContract()
	d.Status = deal.StatusProposal
	d.Parties[0].Document = ""
	d.Parties[1].Document = ""
	svc := testService(d)
	got, err := svc.UpdateStatusTrusted(context.Background(), d.TenantID, d.ID, "", deal.StatusAnalysis, "user-1", "ok")
	if err != nil {
		t.Fatalf("expected success moving to ANALYSIS without document, got %v", err)
	}
	if got.Status != deal.StatusAnalysis {
		t.Errorf("expected ANALYSIS, got %q", got.Status)
	}
}

func TestUpdateStatus_Analysis_StillRequiresNameAndEmail(t *testing.T) {
	// Nome e e-mail seguem obrigatórios já na entrada em Análise — eles
	// identificam a parte e vêm do cadastro do lead.
	d := dealReadyForContract()
	d.Status = deal.StatusProposal
	d.Parties[0].Email = ""
	svc := testService(d)
	_, err := svc.UpdateStatusTrusted(context.Background(), d.TenantID, d.ID, "", deal.StatusAnalysis, "user-1", "")
	if !errors.Is(err, ErrPartiesPending) {
		t.Errorf("expected ErrPartiesPending for missing email at ANALYSIS, got %v", err)
	}
}

// ─────────── UpdateParty: edição de identidade da parte ───────────

func TestUpdateParty_SetsIdentityFields(t *testing.T) {
	// O operador completa o cadastro de uma parte pelo painel da Esteira —
	// em especial o documento, que é o que destrava o gate de Contrato.
	d := dealReadyForContract()
	d.Parties[0].Document = ""
	svc := testService(d)
	doc := "111.444.777-35"
	name := "Inquilino Corrigido"
	birthDate := "1993-04-06"
	custom := map[string]string{"profissao": "Engenheiro"}
	updated, err := svc.UpdateParty(context.Background(), d.TenantID, d.ID, "", "p-tenant",
		nil, nil, nil, nil,
		&PartyIdentityPatch{Name: &name, Document: &doc, BirthDate: &birthDate, CustomFields: custom}, "user-1")
	if err != nil {
		t.Fatalf("UpdateParty: %v", err)
	}
	var tenant *deal.Party
	for i := range updated.Parties {
		if updated.Parties[i].ID == "p-tenant" {
			tenant = &updated.Parties[i]
		}
	}
	if tenant == nil {
		t.Fatal("tenant party not found after UpdateParty")
	}
	if tenant.Document != doc {
		t.Errorf("Document = %q, want %q", tenant.Document, doc)
	}
	if tenant.Name != name {
		t.Errorf("Name = %q, want %q", tenant.Name, name)
	}
	if tenant.BirthDate != birthDate {
		t.Errorf("BirthDate = %q, want %q", tenant.BirthDate, birthDate)
	}
	if tenant.CustomFields["profissao"] != "Engenheiro" {
		t.Errorf("CustomFields[profissao] = %q, want Engenheiro", tenant.CustomFields["profissao"])
	}
	if prov := tenant.FieldProvenance["profissao"]; prov.Source != deal.ProvenanceManual {
		t.Errorf("provenance profissao = %q, want manual", prov.Source)
	}
	if prov := tenant.FieldProvenance["name"]; prov.Source != deal.ProvenanceManual {
		t.Errorf("provenance name = %q, want manual", prov.Source)
	}
	if _, touched := tenant.FieldProvenance["email"]; touched {
		t.Error("email não foi alterado; não deveria ganhar proveniência manual")
	}
	// Campos não enviados no patch permanecem intactos.
	if tenant.Email != "tenant@example.com" {
		t.Errorf("Email should be untouched, got %q", tenant.Email)
	}

	removed, err := svc.UpdateParty(context.Background(), d.TenantID, d.ID, "", "p-tenant",
		nil, nil, nil, nil,
		&PartyIdentityPatch{RemovedCustomFields: []string{"profissao"}}, "user-2")
	if err != nil {
		t.Fatalf("UpdateParty remove custom field: %v", err)
	}
	for i := range removed.Parties {
		if removed.Parties[i].ID != "p-tenant" {
			continue
		}
		if _, ok := removed.Parties[i].CustomFields["profissao"]; ok {
			t.Error("custom field profissao deveria ter sido removido")
		}
		if _, ok := removed.Parties[i].FieldProvenance["profissao"]; ok {
			t.Error("proveniência de profissao deveria ter sido removida")
		}
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Relato do operador: com o aceite de TODAS as partes marcado como opcional nos
// "Parâmetros da Proposta Digital" (a tela anuncia "Sem partes obrigatórias, o
// deal avança sem aceite formal da proposta"), o avanço continuava bloqueado
// pedindo proposta aceita — o gate ignorava a configuração.
func TestUpdateStatus_ProposalOptional_AdvancesWithoutAcceptance(t *testing.T) {
	d := dealReadyForContract()
	optional := false
	for i := range d.Parties {
		d.Parties[i].SignatureRequired = &optional
	}
	// Nem proposta aceita: o operador optou por não exigir aceite formal.
	d.Proposals[0].Status = deal.ProposalStatusSent
	svc := testService(d)
	got, err := svc.UpdateStatusTrusted(context.Background(), d.TenantID, d.ID, "", deal.StatusContract, "user-1", "")
	if err != nil {
		t.Fatalf("aceite opcional deveria permitir avançar, veio %v", err)
	}
	if got.Status != deal.StatusContract {
		t.Errorf("expected CONTRACT, got %q", got.Status)
	}
}

// A exceção é só quando a configuração é EXPLÍCITA: sem nenhuma parte marcada
// (campo ausente, padrão = parte primária) o gate antigo continua valendo.
func TestUpdateStatus_ProposalStillRequiredWhenNobodyIsConfigured(t *testing.T) {
	d := dealReadyForContract()
	for i := range d.Parties {
		d.Parties[i].SignatureRequired = nil
	}
	d.Proposals[0].Status = deal.ProposalStatusSent
	svc := testService(d)
	_, err := svc.UpdateStatusTrusted(context.Background(), d.TenantID, d.ID, "", deal.StatusContract, "user-1", "")
	if !errors.Is(err, ErrProposalNotApproved) {
		t.Errorf("sem configuração explícita o gate deve continuar: veio %v", err)
	}
}

// Basta UMA parte obrigatória para o aceite formal voltar a ser exigido.
func TestUpdateStatus_ProposalRequiredWhenAnyPartyIsMandatory(t *testing.T) {
	d := dealReadyForContract()
	optional, required := false, true
	d.Parties[0].SignatureRequired = &optional
	d.Parties[1].SignatureRequired = &required
	d.Proposals[0].Status = deal.ProposalStatusSent
	svc := testService(d)
	_, err := svc.UpdateStatusTrusted(context.Background(), d.TenantID, d.ID, "", deal.StatusContract, "user-1", "")
	if !errors.Is(err, ErrProposalNotApproved) {
		t.Errorf("com parte obrigatória o gate deve valer: veio %v", err)
	}
}
