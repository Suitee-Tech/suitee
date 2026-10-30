package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/collection"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
)

// agreementRepoMemoria guarda os acordos em memória, respeitando a regra de um
// acordo vigente por caso (a mesma do índice único parcial da migration).
type agreementRepoMemoria struct {
	acordos []*collection.Agreement
}

func (r *agreementRepoMemoria) Create(_ context.Context, a *collection.Agreement) error {
	r.acordos = append(r.acordos, a)
	return nil
}

func (r *agreementRepoMemoria) GetByID(_ context.Context, _, id string) (*collection.Agreement, error) {
	for _, a := range r.acordos {
		if a.ID == id {
			return a, nil
		}
	}
	return nil, nil
}

func (r *agreementRepoMemoria) GetActiveByCollection(_ context.Context, _, collectionID string) (*collection.Agreement, error) {
	for _, a := range r.acordos {
		if a.CollectionID == collectionID && (a.Status == collection.AgreementActive || a.Status == collection.AgreementDraft) {
			return a, nil
		}
	}
	return nil, nil
}

func (r *agreementRepoMemoria) Update(_ context.Context, a *collection.Agreement) error {
	for i, existente := range r.acordos {
		if existente.ID == a.ID {
			r.acordos[i] = a
			return nil
		}
	}
	return collection.ErrAgreementNotFound
}

// casoRepoMemoria serve o CollectionService nos testes da Fase 2: guarda um caso
// e registra a última versão persistida.
type casoRepoMemoria struct {
	*stubCollectionRepo
	caso       *collection.Collection
	persistido *collection.Collection
	criados    []*collection.Collection
}

// GetByID devolve uma CÓPIA, como o repositório Postgres (que desserializa o
// JSONB a cada leitura). Devolver o mesmo ponteiro escondia bugs em que o serviço
// relê o caso no meio do fluxo e perde o que ainda não foi persistido — foi assim
// que o crédito do excedente da garantia sumia sem nenhum teste reclamar.
func (r *casoRepoMemoria) GetByID(_ context.Context, _, _ string) (*collection.Collection, error) {
	if r.caso == nil {
		return nil, nil
	}
	copia := *r.caso
	copia.ChargeIDs = append([]string(nil), r.caso.ChargeIDs...)
	copia.Actions = append([]collection.Action(nil), r.caso.Actions...)
	return &copia, nil
}

func (r *casoRepoMemoria) Update(_ context.Context, c *collection.Collection) error {
	r.persistido = c
	r.caso = c
	return nil
}

func (r *casoRepoMemoria) UpdateIfVersion(_ context.Context, c *collection.Collection, _ time.Time) (bool, error) {
	r.persistido = c
	r.caso = c
	return true, nil
}

func (r *casoRepoMemoria) AddAction(_ context.Context, _, _ string, a collection.Action) error {
	r.caso.Actions = append(r.caso.Actions, a)
	return nil
}

func (r *casoRepoMemoria) Create(_ context.Context, c *collection.Collection) error {
	r.criados = append(r.criados, c)
	return nil
}

func (r *casoRepoMemoria) CreateIdempotent(_ context.Context, c *collection.Collection) error {
	r.criados = append(r.criados, c)
	return nil
}

// chargeRepoFase2 devolve as cobranças pedidas por ID e registra as criadas.
type chargeRepoFase2 struct {
	*mockChargeRepo
	porID   map[string]*charge.Charge
	criadas []*charge.Charge
}

func (r *chargeRepoFase2) GetByID(_ context.Context, _, id string) (*charge.Charge, error) {
	return r.porID[id], nil
}

func (r *chargeRepoFase2) ListByIDs(_ context.Context, _ string, ids []string) ([]*charge.Charge, error) {
	out := make([]*charge.Charge, 0, len(ids))
	for _, id := range ids {
		if c, ok := r.porID[id]; ok {
			out = append(out, c)
		}
	}
	return out, nil
}

func (r *chargeRepoFase2) Create(_ context.Context, c *charge.Charge) error {
	r.criadas = append(r.criadas, c)
	r.porID[c.ID] = c
	return nil
}

func (r *chargeRepoFase2) Update(_ context.Context, c *charge.Charge) error {
	r.porID[c.ID] = c
	return nil
}

func (r *chargeRepoFase2) UpdateIfVersion(_ context.Context, c *charge.Charge, _ time.Time) (bool, error) {
	r.porID[c.ID] = c
	return true, nil
}

func novaCobrancaFase2(t *testing.T, id string, vencimento time.Time, valor float64, status charge.Status) *charge.Charge {
	t.Helper()
	c, err := charge.New("c-1", "lead-inquilino", "lead-proprietario", "prop-1", "t-1", charge.TypeRent, "Aluguel", vencimento.Format("2006-01"), valor, vencimento)
	if err != nil {
		t.Fatalf("charge.New: %v", err)
	}
	c.ID = id
	c.Status = status
	return c
}

func novoServicoFase2(t *testing.T, caso *collection.Collection, cobrancas ...*charge.Charge) (*CollectionService, *casoRepoMemoria, *agreementRepoMemoria, *chargeRepoFase2) {
	t.Helper()
	porID := make(map[string]*charge.Charge, len(cobrancas))
	for _, c := range cobrancas {
		porID[c.ID] = c
	}
	chargeRepo := &chargeRepoFase2{mockChargeRepo: &mockChargeRepo{}, porID: porID}
	casoRepo := &casoRepoMemoria{stubCollectionRepo: &stubCollectionRepo{}, caso: caso}
	agreementRepo := &agreementRepoMemoria{}
	svc := NewCollectionService(casoRepo).
		WithChargeRepository(chargeRepo).
		WithAgreementRepository(agreementRepo).
		WithChargeService(NewChargeService(chargeRepo, nil)).
		WithApprovalThreshold(5000)
	return svc, casoRepo, agreementRepo, chargeRepo
}

func casoComCobrancas(t *testing.T, ids ...string) (*collection.Collection, []*charge.Charge) {
	t.Helper()
	cobrancas := make([]*charge.Charge, 0, len(ids))
	total := 0.0
	for i, id := range ids {
		c := novaCobrancaFase2(t, id, time.Now().AddDate(0, 0, -30*(i+1)), 1000, charge.StatusOverdue)
		cobrancas = append(cobrancas, c)
		total += c.TotalAmount.Float()
	}
	caso := &collection.Collection{
		ID:          "coll-1",
		TenantID:    "t-1",
		ContractID:  "c-1",
		DebtorID:    "lead-inquilino",
		Status:      collection.StatusActive,
		TotalDebt:   total,
		DaysOverdue: 30,
		ChargeIDs:   ids,
	}
	return caso, cobrancas
}

// O acordo precisa materializar as parcelas como cobranças de verdade,
// substituir as originais no caso e colocá-lo em NEGOTIATED (G2).
func TestCreateAgreementGeraParcelasESubstituiOriginais(t *testing.T) {
	caso, cobrancas := casoComCobrancas(t, "ch-1", "ch-2")
	svc, casoRepo, agreementRepo, chargeRepo := novoServicoFase2(t, caso, cobrancas...)

	atualizado, acordo, err := svc.CreateAgreement(context.Background(), "t-1", "coll-1", collection.NewAgreementInput{
		TotalAmount:  1800,
		Discount:     200,
		DownPayment:  0,
		Installments: 3,
		IntervalDays: 30,
		FirstDueDate: time.Now().AddDate(0, 0, 10),
		Notes:        "acordo extrajudicial",
	}, "conta-operador", false)
	if err != nil {
		t.Fatalf("CreateAgreement() erro = %v", err)
	}

	if acordo == nil || acordo.Status != collection.AgreementActive {
		t.Fatalf("acordo = %+v, esperado ACTIVE", acordo)
	}
	if len(acordo.OriginalChargeIDs) != 2 {
		t.Fatalf("original_charge_ids = %v, esperado as 2 cobranças do caso", acordo.OriginalChargeIDs)
	}
	if len(chargeRepo.criadas) != 3 {
		t.Fatalf("cobranças de parcela criadas = %d, esperado 3", len(chargeRepo.criadas))
	}
	soma := 0.0
	for _, c := range chargeRepo.criadas {
		soma += c.Amount.Float()
		if c.SourceEventID == "" {
			t.Fatalf("parcela %s sem chave de idempotência", c.ID)
		}
	}
	if soma != 1800 {
		t.Fatalf("soma das parcelas = %.2f, esperado 1800.00 (valor acordado)", soma)
	}
	// As parcelas substituem as originais: sem o mesmo dono/imóvel o pagamento
	// não geraria repasse ao proprietário nem alocação da locação.
	for _, c := range chargeRepo.criadas {
		if c.OwnerID != "lead-proprietario" || c.PropertyID != "prop-1" || c.TenantPartyID != "lead-inquilino" {
			t.Fatalf("parcela %s sem vínculo econômico da original: owner=%q property=%q tenant_party=%q",
				c.ID, c.OwnerID, c.PropertyID, c.TenantPartyID)
		}
		if c.Type != charge.TypeRent {
			t.Fatalf("parcela %s tipo = %s, esperado o mesmo da original (RENT)", c.ID, c.Type)
		}
	}

	if atualizado.Status != collection.StatusNegotiated {
		t.Fatalf("status do caso = %s, esperado NEGOTIATED", atualizado.Status)
	}
	if len(atualizado.ChargeIDs) != 3 || atualizado.ChargeIDs[0] != chargeRepo.criadas[0].ID {
		t.Fatalf("charge_ids do caso = %v, esperado as 3 parcelas", atualizado.ChargeIDs)
	}
	if atualizado.NegotiatedAmount == nil || *atualizado.NegotiatedAmount != 1800 {
		t.Fatalf("negotiated_amount = %v, esperado 1800", atualizado.NegotiatedAmount)
	}
	// As originais saem do caso e são canceladas.
	for _, c := range cobrancas {
		if c.Status != charge.StatusCancelled {
			t.Fatalf("cobrança original %s = %s, esperado CANCELLED", c.ID, c.Status)
		}
	}
	if casoRepo.persistido == nil {
		t.Fatalf("caso não foi persistido")
	}
	if len(agreementRepo.acordos) != 1 {
		t.Fatalf("acordos gravados = %d, esperado 1", len(agreementRepo.acordos))
	}
}

func TestCreateAgreementExigeAlcadaAcimaDoLimiar(t *testing.T) {
	caso, cobrancas := casoComCobrancas(t, "ch-1")
	svc, _, _, _ := novoServicoFase2(t, caso, cobrancas...)

	_, _, err := svc.CreateAgreement(context.Background(), "t-1", "coll-1", collection.NewAgreementInput{
		TotalAmount:  10000,
		Discount:     6000, // acima do limiar de 5.000
		Installments: 3,
		IntervalDays: 30,
		FirstDueDate: time.Now().AddDate(0, 0, 10),
		Notes:        "desconto grande",
	}, "conta-operador", false)

	if !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("erro = %v, esperado ErrApprovalRequired", err)
	}

	// Com a permissão de aprovação, o acordo passa e fica registrado quem aprovou.
	_, acordo, err := svc.CreateAgreement(context.Background(), "t-1", "coll-1", collection.NewAgreementInput{
		TotalAmount:  10000,
		Discount:     6000,
		Installments: 3,
		IntervalDays: 30,
		FirstDueDate: time.Now().AddDate(0, 0, 10),
		Notes:        "desconto grande",
	}, "conta-gerente", true)
	if err != nil {
		t.Fatalf("com alçada o acordo deveria passar: %v", err)
	}
	if !acordo.ApprovalRequired || acordo.ApprovedBy != "conta-gerente" {
		t.Fatalf("aprovação não registrada: %+v", acordo)
	}
}

func TestCreateAgreementRejeitaSegundoAcordoVigente(t *testing.T) {
	caso, cobrancas := casoComCobrancas(t, "ch-1")
	svc, _, _, _ := novoServicoFase2(t, caso, cobrancas...)
	entrada := collection.NewAgreementInput{
		TotalAmount: 1000, Installments: 2, IntervalDays: 30,
		FirstDueDate: time.Now().AddDate(0, 0, 5), Notes: "primeiro",
	}
	if _, _, err := svc.CreateAgreement(context.Background(), "t-1", "coll-1", entrada, "u1", false); err != nil {
		t.Fatalf("primeiro acordo falhou: %v", err)
	}
	_, _, err := svc.CreateAgreement(context.Background(), "t-1", "coll-1", entrada, "u1", false)
	if !errors.Is(err, ErrAgreementConflict) {
		t.Fatalf("erro = %v, esperado ErrAgreementConflict", err)
	}
}

// Quebrar o acordo devolve o caso à cobrança ativa e NÃO inventa dívida nova
// quando `restore_discount` não é pedido.
func TestBreakAgreementDevolveCasoParaAtivo(t *testing.T) {
	caso, cobrancas := casoComCobrancas(t, "ch-1")
	svc, casoRepo, agreementRepo, chargeRepo := novoServicoFase2(t, caso, cobrancas...)

	if _, _, err := svc.CreateAgreement(context.Background(), "t-1", "coll-1", collection.NewAgreementInput{
		TotalAmount: 900, Discount: 100, Installments: 3, IntervalDays: 30,
		FirstDueDate: time.Now().AddDate(0, 0, 5), Notes: "acordo",
	}, "u1", false); err != nil {
		t.Fatalf("acordo falhou: %v", err)
	}
	criadasAntes := len(chargeRepo.criadas)

	atualizado, acordo, err := svc.BreakAgreement(context.Background(), "t-1", "coll-1", "parcela 1 não paga", "u1", false)
	if err != nil {
		t.Fatalf("BreakAgreement() erro = %v", err)
	}
	if acordo.Status != collection.AgreementBroken {
		t.Fatalf("acordo = %s, esperado BROKEN", acordo.Status)
	}
	if atualizado.Status != collection.StatusActive {
		t.Fatalf("caso = %s, esperado ACTIVE", atualizado.Status)
	}
	// As parcelas em aberto saem e a dívida volta como UMA cobrança restabelecida:
	// sem isso o devedor seguiria sendo cobrado pelo plano de um acordo encerrado.
	for _, id := range acordo.InstallmentChargeIDs {
		if c := chargeRepo.porID[id]; c != nil && charge.IsOutstandingStatus(c.Status) {
			t.Fatalf("parcela %s continuou aberta (%s) depois de o acordo ser encerrado", id, c.Status)
		}
	}
	if len(chargeRepo.criadas) != criadasAntes+1 {
		t.Fatalf("esperava a cobrança de dívida restabelecida (criadas %d → %d)", criadasAntes, len(chargeRepo.criadas))
	}
	restabelecida := chargeRepo.criadas[len(chargeRepo.criadas)-1]
	// O caso tinha 1.000; o acordo fechou em 900 (desconto de 100) e nada foi pago.
	// Sem revogar o desconto, a dívida que volta é o valor NEGOCIADO: 900.
	if restabelecida.Amount.Float() != 900 {
		t.Fatalf("dívida restabelecida = %.2f, esperado 900.00 (valor negociado, nada pago)", restabelecida.Amount.Float())
	}
	if len(atualizado.ChargeIDs) != 1 || atualizado.ChargeIDs[0] != restabelecida.ID {
		t.Fatalf("caso deveria apontar para a dívida restabelecida: %v", atualizado.ChargeIDs)
	}
	if casoRepo.persistido.Status != collection.StatusActive {
		t.Fatalf("caso persistido = %s, esperado ACTIVE", casoRepo.persistido.Status)
	}
	if len(agreementRepo.acordos) != 1 || agreementRepo.acordos[0].BreakReason == "" {
		t.Fatalf("motivo da quebra não registrado")
	}
}

// A baixa por garantia aplica o valor da cobrança mais antiga para a mais nova,
// credita o excedente e resolve o caso quando quita tudo (G1).
func TestSettleGuaranteeBaixaMaisAntigaPrimeiroECreditaExcedente(t *testing.T) {
	antiga := novaCobrancaFase2(t, "ch-antiga", time.Now().AddDate(0, 0, -60), 1000, charge.StatusOverdue)
	nova := novaCobrancaFase2(t, "ch-nova", time.Now().AddDate(0, 0, -10), 1000, charge.StatusOverdue)
	tipo := collection.GuaranteeTypeSeguroFianca
	caso := &collection.Collection{
		ID: "coll-1", TenantID: "t-1", ContractID: "c-1", DebtorID: "lead-inquilino",
		Status: collection.StatusGuaranteeActivated, TotalDebt: 2000, DaysOverdue: 60,
		ChargeIDs: []string{"ch-antiga", "ch-nova"},
		Guarantee: &collection.GuaranteeActivation{
			Type: tipo, Amount: 2000, Status: collection.GuaranteeStatusAccepted,
			RequestedAt: time.Now().AddDate(0, 0, -5),
		},
	}
	svc, casoRepo, _, _ := novoServicoFase2(t, caso, antiga, nova)

	atualizado, resultado, err := svc.SettleGuarantee(context.Background(), "t-1", "coll-1", 2500, "sinistro-123", "pagamento integral", "u1")
	if err != nil {
		t.Fatalf("SettleGuarantee() erro = %v", err)
	}

	if antiga.Status != charge.StatusPaid {
		t.Fatalf("cobrança mais antiga = %s, esperado PAID", antiga.Status)
	}
	if nova.Status != charge.StatusPaid {
		t.Fatalf("segunda cobrança = %s, esperado PAID", nova.Status)
	}
	if resultado.AppliedAmount != 2000 {
		t.Fatalf("aplicado = %.2f, esperado 2000.00 (a dívida)", resultado.AppliedAmount)
	}
	if resultado.CreditAmount != 500 {
		t.Fatalf("crédito = %.2f, esperado 500.00 (excedente)", resultado.CreditAmount)
	}
	if atualizado.CreditBalance != 500 {
		t.Fatalf("credit_balance do caso = %.2f, esperado 500.00", atualizado.CreditBalance)
	}
	// O que importa é o que ficou GRAVADO: o serviço relê o caso no meio da baixa
	// e o crédito precisa sobreviver à releitura.
	if casoRepo.persistido == nil || casoRepo.persistido.CreditBalance != 500 {
		t.Fatalf("credit_balance persistido = %+v, esperado 500.00", casoRepo.persistido)
	}
	if atualizado.Status != collection.StatusResolved {
		t.Fatalf("caso = %s, esperado RESOLVED (dívida quitada pela garantia)", atualizado.Status)
	}
	if resultado.DepositDeductionID != "" {
		t.Fatalf("garantia que não é caução não deve registrar dedução de depósito")
	}
}

// Caução: o valor sai do depósito do contrato, então a dedução precisa ficar
// registrada para o acerto da rescisão (G25).
func TestSettleGuaranteeCaucaoRegistraDeducao(t *testing.T) {
	cobranca := novaCobrancaFase2(t, "ch-1", time.Now().AddDate(0, 0, -20), 1000, charge.StatusOverdue)
	tipo := collection.GuaranteeTypeCaucao
	caso := &collection.Collection{
		ID: "coll-1", TenantID: "t-1", ContractID: "c-1", DebtorID: "lead-inquilino",
		Status: collection.StatusGuaranteeActivated, TotalDebt: 1000, DaysOverdue: 20,
		ChargeIDs: []string{"ch-1"},
		Guarantee: &collection.GuaranteeActivation{Type: tipo, Amount: 1000, Status: collection.GuaranteeStatusAccepted},
	}
	svc, _, _, chargeRepo := novoServicoFase2(t, caso, cobranca)

	_, resultado, err := svc.SettleGuarantee(context.Background(), "t-1", "coll-1", 1000, "dep-1", "", "u1")
	if err != nil {
		t.Fatalf("SettleGuarantee() erro = %v", err)
	}
	if resultado.DepositDeductionID == "" {
		t.Fatalf("caução deveria registrar a dedução do depósito")
	}
	var deducao *charge.Charge
	for _, c := range chargeRepo.criadas {
		if c.ID == resultado.DepositDeductionID {
			deducao = c
		}
	}
	if deducao == nil {
		t.Fatalf("cobrança de dedução não encontrada entre as criadas")
	}
	if deducao.Status != charge.StatusPaid {
		t.Fatalf("dedução = %s, esperado PAID (o valor saiu do depósito)", deducao.Status)
	}
	if !isDepositDeduction(deducao) {
		t.Fatalf("dedução não é reconhecida pelo acerto da rescisão: %q", deducao.Description)
	}
}

// Sentença improcedente devolve o caso à cobrança; arquivado também. As custas
// viram despesa lançada no financeiro (G12).
func TestRegisterJudicialOutcomeImprocedenteRetomaCobranca(t *testing.T) {
	cobranca := novaCobrancaFase2(t, "ch-1", time.Now().AddDate(0, 0, -100), 3000, charge.StatusOverdue)
	caso := &collection.Collection{
		ID: "coll-1", TenantID: "t-1", ContractID: "c-1", DebtorID: "lead-inquilino",
		Status: collection.StatusJudicial, TotalDebt: 3000, DaysOverdue: 100,
		ChargeIDs: []string{"ch-1"},
		Judicial: &collection.JudicialProcess{
			CaseNumber: "1234567-89.2026.8.26.0100", Court: "5ª Vara Cível",
			Status: collection.JudicialStatusEmTramite, CreatedAt: time.Now().AddDate(0, 0, -30),
		},
	}
	svc, _, _, _ := novoServicoFase2(t, caso, cobranca)
	txRepo := &txRepoMemoria{}
	svc.WithTransactionService(NewTransactionService(txRepo))

	atualizado, err := svc.RegisterJudicialOutcome(context.Background(), "t-1", "coll-1",
		collection.JudicialStatusSentencaImprocedente, time.Now(), "sem prova do débito",
		[]collection.JudicialCost{{Description: "Custas processuais", Amount: 350.5}}, "u1")
	if err != nil {
		t.Fatalf("RegisterJudicialOutcome() erro = %v", err)
	}
	if atualizado.Status != collection.StatusActive {
		t.Fatalf("caso = %s, esperado ACTIVE (cobrança retomada)", atualizado.Status)
	}
	if atualizado.Judicial.Status != collection.JudicialStatusSentencaImprocedente {
		t.Fatalf("status judicial = %s", atualizado.Judicial.Status)
	}
	if atualizado.Judicial.DecidedAt == nil {
		t.Fatalf("data da decisão não registrada")
	}
	if len(atualizado.Judicial.Costs) != 1 || atualizado.Judicial.Costs[0].Amount != 350.5 {
		t.Fatalf("custas não registradas no processo: %+v", atualizado.Judicial.Costs)
	}
	if len(txRepo.criadas) != 1 {
		t.Fatalf("lançamentos de custas = %d, esperado 1", len(txRepo.criadas))
	}
	if txRepo.criadas[0].Type != transaction.TypeDespesa || txRepo.criadas[0].Value != 350.5 {
		t.Fatalf("lançamento = %+v, esperado despesa de 350.50", txRepo.criadas[0])
	}
}

func TestRegisterJudicialOutcomeAcordoLevaParaNegociacao(t *testing.T) {
	cobranca := novaCobrancaFase2(t, "ch-1", time.Now().AddDate(0, 0, -100), 3000, charge.StatusOverdue)
	caso := &collection.Collection{
		ID: "coll-1", TenantID: "t-1", ContractID: "c-1", DebtorID: "lead-inquilino",
		Status: collection.StatusJudicial, TotalDebt: 3000,
		ChargeIDs: []string{"ch-1"},
		Judicial:  &collection.JudicialProcess{CaseNumber: "1234567-89.2026.8.26.0100", Court: "Vara", Status: collection.JudicialStatusEmTramite},
	}
	svc, _, _, _ := novoServicoFase2(t, caso, cobranca)

	atualizado, err := svc.RegisterJudicialOutcome(context.Background(), "t-1", "coll-1",
		collection.JudicialStatusAcordo, time.Now(), "acordo homologado", nil, "u1")
	if err != nil {
		t.Fatalf("erro = %v", err)
	}
	if atualizado.Status != collection.StatusNegotiated {
		t.Fatalf("caso = %s, esperado NEGOTIATED", atualizado.Status)
	}
}

func TestRegisterJudicialOutcomeRejeitaStatusDeAndamento(t *testing.T) {
	caso := &collection.Collection{ID: "coll-1", TenantID: "t-1", Status: collection.StatusJudicial,
		Judicial: &collection.JudicialProcess{CaseNumber: "1234567-89.2026.8.26.0100", Court: "Vara"}}
	svc, _, _, _ := novoServicoFase2(t, caso)

	if _, err := svc.RegisterJudicialOutcome(context.Background(), "t-1", "coll-1",
		collection.JudicialStatusEmTramite, time.Now(), "", nil, "u1"); !errors.Is(err, ErrInvalidJudicialOutcome) {
		t.Fatalf("erro = %v, esperado ErrInvalidJudicialOutcome", err)
	}
}

// txRepoMemoria guarda os lançamentos criados pelo serviço de transações.
type txRepoMemoria struct {
	criadas []*transaction.Transaction
}

func (r *txRepoMemoria) Create(_ context.Context, t *transaction.Transaction) error {
	r.criadas = append(r.criadas, t)
	return nil
}
func (r *txRepoMemoria) GetByID(context.Context, string, string) (*transaction.Transaction, error) {
	return nil, nil
}
func (r *txRepoMemoria) CreateBatch(_ context.Context, txs []*transaction.Transaction) error {
	r.criadas = append(r.criadas, txs...)
	return nil
}
func (r *txRepoMemoria) GetBySourceEventID(context.Context, string, string) (*transaction.Transaction, error) {
	return nil, nil
}
func (r *txRepoMemoria) List(context.Context, string, string, int64, int64) ([]*transaction.Transaction, int64, error) {
	return nil, 0, nil
}
func (r *txRepoMemoria) Search(context.Context, string, transaction.SearchFilter) ([]*transaction.Transaction, int64, error) {
	return nil, 0, nil
}
func (r *txRepoMemoria) Update(context.Context, *transaction.Transaction) error { return nil }
func (r *txRepoMemoria) Delete(context.Context, string, string) error           { return nil }

// Quebrar com `restore_discount` revoga o desconto concedido: a dívida volta ao
// valor cheio (prática de quebra de acordo), e não ao valor negociado.
func TestBreakAgreementComRestoreDiscountRevogaDesconto(t *testing.T) {
	caso, cobrancas := casoComCobrancas(t, "ch-1", "ch-2") // 2000 no total
	svc, _, _, chargeRepo := novoServicoFase2(t, caso, cobrancas...)

	if _, _, err := svc.CreateAgreement(context.Background(), "t-1", "coll-1", collection.NewAgreementInput{
		TotalAmount: 1500, Discount: 500, Installments: 3, IntervalDays: 30,
		FirstDueDate: time.Now().AddDate(0, 0, 5), Notes: "com desconto",
	}, "u1", false); err != nil {
		t.Fatalf("acordo falhou: %v", err)
	}
	antes := len(chargeRepo.criadas)

	atualizado, _, err := svc.BreakAgreement(context.Background(), "t-1", "coll-1", "não cumpriu", "u1", true)
	if err != nil {
		t.Fatalf("BreakAgreement() erro = %v", err)
	}
	if len(chargeRepo.criadas) != antes+1 {
		t.Fatalf("esperava a dívida restabelecida (criadas %d → %d)", antes, len(chargeRepo.criadas))
	}
	restabelecida := chargeRepo.criadas[len(chargeRepo.criadas)-1]
	if restabelecida.Amount.Float() != 2000 {
		t.Fatalf("dívida restabelecida = %.2f, esperado 2000.00 (valor cheio, desconto revogado)", restabelecida.Amount.Float())
	}
	if atualizado.TotalDebt != 2000 {
		t.Fatalf("dívida do caso = %.2f, esperado 2000.00", atualizado.TotalDebt)
	}
}

// Cancelar o acordo (erro de cadastro) NÃO é quebra: o desconto permanece.
func TestCancelAgreementMantemDesconto(t *testing.T) {
	caso, cobrancas := casoComCobrancas(t, "ch-1", "ch-2")
	svc, _, _, chargeRepo := novoServicoFase2(t, caso, cobrancas...)

	if _, _, err := svc.CreateAgreement(context.Background(), "t-1", "coll-1", collection.NewAgreementInput{
		TotalAmount: 1500, Discount: 500, Installments: 3, IntervalDays: 30,
		FirstDueDate: time.Now().AddDate(0, 0, 5), Notes: "com desconto",
	}, "u1", false); err != nil {
		t.Fatalf("acordo falhou: %v", err)
	}
	antes := len(chargeRepo.criadas)

	if _, _, err := svc.CancelAgreement(context.Background(), "t-1", "coll-1", "cadastro errado", "u1"); err != nil {
		t.Fatalf("CancelAgreement() erro = %v", err)
	}
	if len(chargeRepo.criadas) != antes+1 {
		t.Fatalf("esperava a dívida restabelecida (criadas %d → %d)", antes, len(chargeRepo.criadas))
	}
	restabelecida := chargeRepo.criadas[len(chargeRepo.criadas)-1]
	if restabelecida.Amount.Float() != 1500 {
		t.Fatalf("dívida restabelecida = %.2f, esperado 1500.00 (desconto mantido no cancelamento)", restabelecida.Amount.Float())
	}
}

// A ordem interna da quebra importa: se as parcelas fossem canceladas antes de o
// caso apontar para a dívida restabelecida, a reconciliação disparada pelo
// cancelamento encontraria todas as cobranças fechadas e encerraria o caso como
// RESOLVED no meio da quebra (foi o 409 "collection changed concurrently" que a
// validação no beta pegou). O teste garante o efeito final: caso ACTIVE com a
// dívida restabelecida e nenhuma parcela aberta.
func TestBreakAgreementNaoEncerraCasoPorEngano(t *testing.T) {
	caso, cobrancas := casoComCobrancas(t, "ch-1", "ch-2")
	svc, _, _, chargeRepo := novoServicoFase2(t, caso, cobrancas...)

	if _, _, err := svc.CreateAgreement(context.Background(), "t-1", "coll-1", collection.NewAgreementInput{
		TotalAmount: 2000, Installments: 2, IntervalDays: 30,
		FirstDueDate: time.Now().AddDate(0, 0, 5), Notes: "acordo",
	}, "u1", false); err != nil {
		t.Fatalf("acordo falhou: %v", err)
	}
	acordo, _ := svc.GetAgreement(context.Background(), "t-1", "coll-1")

	atualizado, _, err := svc.BreakAgreement(context.Background(), "t-1", "coll-1", "não pagou", "u1", false)
	if err != nil {
		t.Fatalf("BreakAgreement() erro = %v", err)
	}
	if atualizado.Status != collection.StatusActive {
		t.Fatalf("caso = %s, esperado ACTIVE (nunca RESOLVED no meio da quebra)", atualizado.Status)
	}
	if atualizado.TotalDebt != 2000 {
		t.Fatalf("dívida do caso = %.2f, esperado 2000.00", atualizado.TotalDebt)
	}
	for _, id := range acordo.InstallmentChargeIDs {
		if c := chargeRepo.porID[id]; c != nil && charge.IsOutstandingStatus(c.Status) {
			t.Fatalf("parcela %s continuou aberta (%s)", id, c.Status)
		}
	}
}
