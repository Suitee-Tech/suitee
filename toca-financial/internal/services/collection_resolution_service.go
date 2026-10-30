package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/collection"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
)

// ErrInvalidSettlement — valor de baixa inválido (handler trata 400).
var ErrInvalidSettlement = errors.New("dados da baixa inválidos")

// ErrInvalidJudicialOutcome — desfecho judicial inválido (handler trata 400).
var ErrInvalidJudicialOutcome = errors.New("desfecho judicial inválido")

// SettlementResult resume a baixa por garantia: o que entrou, o que foi
// aplicado nas cobranças, o que sobrou como crédito e o que foi criado para o
// acerto da caução. É esse trio que o operador precisa conferir.
type SettlementResult struct {
	AppliedAmount        float64  `json:"applied_amount"`
	CreditAmount         float64  `json:"credit_amount"`
	SettledChargeIDs     []string `json:"settled_charge_ids"`
	ResolvedCollection   bool     `json:"resolved_collection"`
	DepositDeductionID   string   `json:"security_deposit_deduction_id,omitempty"`
	RemainingCaseBalance float64  `json:"remaining_case_balance"`
}

// SettleGuarantee dá baixa nas cobranças do caso com o valor recebido do
// garantidor (seguradora, fiador ou caução) — G1 do plano.
//
// Antes, registrar `PAID` na garantia era só gravar um status: a dívida
// continuava vencida, o aging continuava contando e o dinheiro recuperado não
// entrava em lugar nenhum. Agora o valor recebido:
//
//  1. abate as cobranças do caso da MAIS ANTIGA para a mais nova (é a ordem que
//     o devedor e o contador esperam, e a que reduz primeiro o atraso mais caro);
//  2. se sobrar, vira crédito do caso (`credit_balance`) em vez de desaparecer;
//  3. se quitar tudo, o caso é resolvido pelo próprio caminho de pagamento.
//
// Para garantia do tipo CAUCAO o valor sai do depósito de caução do contrato: o
// sistema registra a dedução ("Dedução caução", o mesmo marcador que o acerto da
// rescisão já soma) para o saldo a devolver ao inquilino refletir o que foi usado.
//
// Idempotência: o ID do pagamento é determinístico por (caso, cobrança,
// referência). Reenviar a mesma baixa não paga duas vezes.
func (s *CollectionService) SettleGuarantee(
	ctx context.Context,
	tenantID, collectionID string,
	amount float64,
	reference, insurerResponse, actor string,
) (*collection.Collection, *SettlementResult, error) {
	if amount <= 0 {
		return nil, nil, fmt.Errorf("%w: valor recebido deve ser maior que zero", ErrInvalidSettlement)
	}
	if s.chargeService == nil {
		return nil, nil, fmt.Errorf("charge service is required for settlement")
	}

	caso, err := s.GetByID(ctx, tenantID, collectionID)
	if err != nil {
		return nil, nil, err
	}

	garantia := caso.Guarantee
	if garantia == nil {
		return nil, nil, fmt.Errorf("%w: caso não tem garantia acionada", ErrInvalidSettlement)
	}

	// 1. Status da garantia registrado primeiro: se a baixa falhar no meio, o
	// operador vê a garantia quitada com baixa incompleta (e reenvia) em vez de
	// ver baixa feita sem registro do recebimento.
	if _, err := s.UpdateGuaranteeStatus(ctx, tenantID, collectionID, collection.GuaranteeStatusPaid, insurerResponse, actor); err != nil {
		return nil, nil, err
	}
	caso, err = s.GetByID(ctx, tenantID, collectionID)
	if err != nil {
		return nil, nil, err
	}

	// 2. Cobranças do caso, da mais antiga para a mais nova.
	charges, err := s.chargeRepo.ListByIDs(ctx, tenantID, caso.ChargeIDs)
	if err != nil {
		return nil, nil, err
	}
	abertas := make([]*charge.Charge, 0, len(charges))
	for _, c := range charges {
		if c != nil && charge.IsOutstandingStatus(c.Status) {
			abertas = append(abertas, c)
		}
	}
	sort.SliceStable(abertas, func(i, j int) bool { return abertas[i].DueDate.Before(abertas[j].DueDate) })

	metodo := metodoDaGarantia(garantia.Type)
	restante := amount
	aplicado := 0.0
	baixadas := make([]string, 0, len(abertas))
	for _, c := range abertas {
		if restante <= 0 {
			break
		}
		saldo := c.TotalAmount.Float()
		if c.PaidAmount != nil {
			saldo -= c.PaidAmount.Float()
		}
		if saldo <= 0 {
			continue
		}
		valor := saldo
		if valor > restante {
			valor = restante
		}
		paymentID := events.DeriveNotificationID(collectionID, "guarantee-settlement", c.ID, reference)
		if _, err := s.chargeService.MarkPaidIdempotent(ctx, tenantID, c.ID, metodo, valor, paymentID, time.Now()); err != nil {
			return nil, nil, fmt.Errorf("baixar cobrança %s: %w", c.ID, err)
		}
		baixadas = append(baixadas, c.ID)
		aplicado += valor
		restante -= valor
	}

	resultado := &SettlementResult{
		AppliedAmount:    round2(aplicado),
		SettledChargeIDs: baixadas,
	}

	// 3. Excedente: crédito do caso (dinheiro entrou e tem destino declarado).
	//
	// O valor é guardado em variável local porque o caso é RELIDO mais abaixo
	// (para pegar a versão que a reconciliação dos pagamentos acabou de gravar), e
	// a releitura descarta o que ainda não foi persistido. Era o que acontecia: a
	// baixa informava "crédito de R$ 200" e o caso era gravado sem ele — dinheiro
	// recebido a maior que desaparecia do registro. A validação no beta pegou.
	credito := 0.0
	if restante > 0 {
		credito = round2(restante)
		resultado.CreditAmount = credito
	}

	// 4. Caução: registra a dedução no contrato, que é o que o acerto da
	// rescisão lê para calcular o que devolver ao inquilino.
	if garantia.Type == collection.GuaranteeTypeCaucao {
		deducao, err := s.registrarDeducaoCaucao(ctx, caso, aplicado, reference)
		if err != nil {
			log.Printf("[Collections] failed to register security deposit deduction for collection %s: %v", collectionID, err)
		} else if deducao != nil {
			resultado.DepositDeductionID = deducao.ID
		}
	}

	// 5. Saldo remanescente e estado final (leitura fresca, com o crédito
	// reaplicado — ver comentário do passo 3).
	caso, err = s.GetByID(ctx, tenantID, collectionID)
	if err != nil {
		return nil, nil, err
	}
	if credito > 0 {
		caso.CreditBalance = round2(caso.CreditBalance + credito)
	}
	if saldo, err := s.recalcularDividaDoCaso(ctx, caso); err == nil {
		caso.TotalDebt = saldo
		resultado.RemainingCaseBalance = round2(saldo)
	}
	{
		caso.Actions = append(caso.Actions, collection.Action{
			Date: time.Now(),
			Type: "guarantee_settled",
			Description: fmt.Sprintf("Indenização de %s recebida: R$ %.2f (aplicado R$ %.2f, crédito R$ %.2f)",
				garantia.Type, amount, resultado.AppliedAmount, resultado.CreditAmount),
			UserID: actor,
		})
	}
	// Quitou tudo: o caso é resolvido aqui também, e não só pelo caminho de
	// pagamento da cobrança. A baixa por garantia é o evento que encerra a
	// dívida; depender de um hook externo deixaria o caso aberto com saldo zero
	// quando o resolver não estivesse ligado (foi o que o teste flagrou).
	alvo := caso.Status
	if resultado.RemainingCaseBalance <= 0 && caso.Status != collection.StatusResolved {
		alvo = collection.StatusResolved
	}
	caso, err = s.UpdateDetails(ctx, caso, alvo, actor)
	if err != nil {
		return nil, nil, err
	}
	resultado.ResolvedCollection = caso.Status == collection.StatusResolved
	resultado.RemainingCaseBalance = round2(caso.TotalDebt)

	s.publishSettlementEvent(ctx, contracts.CollectionSettledEvent{
		EventIDVal: events.DeriveNotificationID(collectionID, "collection.guarantee_settled", reference),
		Data: contracts.CollectionSettlementEventPayload{
			CollectionID:       collectionID,
			ContractID:         caso.ContractID,
			TenantID:           tenantID,
			GuaranteeType:      string(garantia.Type),
			SettledAmount:      round2(amount),
			AppliedAmount:      resultado.AppliedAmount,
			CreditAmount:       resultado.CreditAmount,
			SettledChargeIDs:   baixadas,
			Reference:          reference,
			ResolvedCollection: resultado.ResolvedCollection,
			Timestamp:          time.Now().UTC().Format(time.RFC3339),
		},
	})

	return caso, resultado, nil
}

// registrarDeducaoCaucao cria a cobrança-memo "Dedução caução" já quitada.
//
// Ela não é dívida nova: é o registro de que parte do depósito foi consumida.
// `ComputeSecurityDepositBalance` soma exatamente essas cobranças (Type OTHER +
// descrição "Dedução caução") para calcular `deductions_charged` e o
// `return_due` da rescisão — sem ela, o inquilino receberia de volta um valor
// que já foi usado para cobrir o aluguel.
func (s *CollectionService) registrarDeducaoCaucao(ctx context.Context, caso *collection.Collection, valor float64, reference string) (*charge.Charge, error) {
	if valor <= 0 || caso == nil {
		return nil, nil
	}
	sourceID := events.DeriveNotificationID(caso.ID, "security-deposit-deduction", reference)
	deducao, err := s.chargeService.CreateWithSourceEventID(
		ctx, sourceID,
		caso.ContractID, "", "", caso.DebtorID, "", "",
		caso.TenantID, charge.TypeOther,
		fmt.Sprintf("Dedução caução — caso %s", shortID(caso.ID)),
		time.Now().Format("2006-01"), round2(valor), time.Now(),
	)
	if err != nil {
		return nil, err
	}
	// Quitada na hora: o valor saiu do depósito, não é cobrança a receber.
	paymentID := events.DeriveNotificationID(deducao.ID, "security-deposit-deduction-settled")
	if _, err := s.chargeService.MarkPaidIdempotent(ctx, caso.TenantID, deducao.ID, "CAUCAO", round2(valor), paymentID, time.Now()); err != nil {
		return nil, err
	}
	return deducao, nil
}

// RegisterJudicialOutcome registra o desfecho do processo e aplica o efeito
// correspondente no caso (G3 e G12 do plano).
//
//	ACORDO                → NEGOTIATED (o acordo é formalizado em /agreement)
//	SENTENCA_PROCEDENTE   → segue JUDICIAL, aguardando o recebimento
//	SENTENCA_IMPROCEDENTE → ACTIVE (a cobrança e a régua retomam)
//	ARQUIVADO             → ACTIVE (a cobrança e a régua retomam)
//
// As custas informadas viram despesa lançada no financeiro: o processo custava
// dinheiro e esse custo não existia em lugar nenhum.
func (s *CollectionService) RegisterJudicialOutcome(
	ctx context.Context,
	tenantID, collectionID string,
	status collection.JudicialStatus,
	decidedAt time.Time,
	notes string,
	costs []collection.JudicialCost,
	actor string,
) (*collection.Collection, error) {
	if !collection.ValidJudicialStatus(status) || !status.IsJudicialOutcome() {
		return nil, fmt.Errorf("%w: %q não é um desfecho (use ACORDO, SENTENCA_PROCEDENTE, SENTENCA_IMPROCEDENTE ou ARQUIVADO)",
			ErrInvalidJudicialOutcome, status)
	}
	for _, c := range costs {
		if strings.TrimSpace(c.Description) == "" || c.Amount <= 0 {
			return nil, fmt.Errorf("%w: cada custa precisa de descrição e valor maior que zero", ErrInvalidJudicialOutcome)
		}
	}

	caso, err := s.GetByID(ctx, tenantID, collectionID)
	if err != nil {
		return nil, err
	}
	if caso.Judicial == nil {
		return nil, fmt.Errorf("%w: caso não tem processo judicial registrado", ErrInvalidJudicialOutcome)
	}

	now := time.Now()
	if decidedAt.IsZero() {
		decidedAt = now
	}
	decisao := decidedAt
	caso.Judicial.Status = status
	caso.Judicial.DecidedAt = &decisao
	if strings.TrimSpace(notes) != "" {
		if caso.Judicial.Notes == "" {
			caso.Judicial.Notes = strings.TrimSpace(notes)
		} else {
			caso.Judicial.Notes = caso.Judicial.Notes + "\n" + strings.TrimSpace(notes)
		}
	}
	caso.Judicial.Costs = append(caso.Judicial.Costs, costs...)

	alvo := status.OutcomeTargetStatus(caso.Status)
	caso.Actions = append(caso.Actions, collection.Action{
		Date:        now,
		Type:        "judicial_outcome",
		Description: descricaoDesfecho(status, alvo, costs),
		UserID:      actor,
	})

	atualizado, err := s.UpdateDetails(ctx, caso, alvo, actor)
	if err != nil {
		return nil, err
	}

	// Custas: despesa do contrato, uma linha por item (descrição própria).
	var custasLancadas float64
	for _, c := range costs {
		if s.transactionService == nil {
			log.Printf("[Collections] transaction service not wired; judicial cost not posted: %s %.2f", c.Description, c.Amount)
			continue
		}
		t := transaction.New(caso.ContractID, transaction.TypeDespesa, "Custas judiciais — "+c.Description, c.Amount, decidedAt, tenantID)
		t.Category = "custas_judiciais"
		t.Status = transaction.StatusPaid
		t.PaymentDate = &decisao
		if err := s.transactionService.Create(ctx, t); err != nil {
			log.Printf("[Collections] failed to post judicial cost %q of collection %s: %v", c.Description, collectionID, err)
			continue
		}
		custasLancadas += c.Amount
	}

	s.publishSettlementEvent(ctx, contracts.CollectionJudicialClosedEvent{
		EventIDVal: events.DeriveNotificationID(collectionID, "collection.judicial_closed", string(status)),
		Data: contracts.CollectionSettlementEventPayload{
			CollectionID:   collectionID,
			ContractID:     caso.ContractID,
			TenantID:       tenantID,
			JudicialStatus: string(status),
			SettledAmount:  round2(custasLancadas),
			Timestamp:      now.UTC().Format(time.RFC3339),
		},
	})

	return atualizado, nil
}

// ReopenAfterGuaranteeRejection devolve o caso à cobrança quando o garantidor
// recusa ou cancela o acionamento (a régua volta a avisar o devedor).
func (s *CollectionService) ReopenAfterGuaranteeRejection(ctx context.Context, tenantID, collectionID, actor string) (*collection.Collection, error) {
	caso, err := s.GetByID(ctx, tenantID, collectionID)
	if err != nil {
		return nil, err
	}
	if caso.Status != collection.StatusGuaranteeActivated {
		return caso, nil
	}
	caso.Actions = append(caso.Actions, collection.Action{
		Date:        time.Now(),
		Type:        "guarantee_rejected",
		Description: "Garantia recusada/cancelada: caso volta para cobrança ativa e a régua retoma",
		UserID:      actor,
	})
	return s.UpdateDetails(ctx, caso, collection.StatusActive, actor)
}

func (s *CollectionService) publishSettlementEvent(ctx context.Context, event events.Event) {
	if s.eventBus == nil {
		return
	}
	if err := s.eventBus.PublishReliable(ctx, event); err != nil {
		log.Printf("[Collections] failed to publish %s: %v", event.Name(), err)
	}
	audit.Record(ctx, audit.Entry{
		Action:     event.Name(),
		EntityType: "Collection",
		EntityID:   collectionIDFromEvent(event),
		After:      map[string]any{"payload": event.Payload()},
		Metadata:   map[string]any{"path": "publishSettlementEvent"},
	})
}

func collectionIDFromEvent(event events.Event) string {
	if p, ok := event.Payload().(contracts.CollectionSettlementEventPayload); ok {
		return p.CollectionID
	}
	return ""
}

// round2 arredonda para centavos. O cálculo de dinheiro do serviço trabalha em
// float (o domínio faz o mesmo) e sem o arredondamento a soma das parcelas
// acumula fração de centavo que aparece no extrato.
func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}

func metodoDaGarantia(t collection.GuaranteeType) string {
	switch t {
	case collection.GuaranteeTypeSeguroFianca:
		return "SEGURO_FIANCA"
	case collection.GuaranteeTypeFiador:
		return "FIADOR"
	case collection.GuaranteeTypeCaucao:
		return "CAUCAO"
	default:
		return "GARANTIA"
	}
}

func descricaoDesfecho(status collection.JudicialStatus, alvo collection.Status, costs []collection.JudicialCost) string {
	efeito := map[collection.JudicialStatus]string{
		collection.JudicialStatusAcordo:               "acordo homologado — caso passa a negociação",
		collection.JudicialStatusSentencaProcedente:   "sentença procedente — aguardando recebimento",
		collection.JudicialStatusSentencaImprocedente: "sentença improcedente — cobrança retomada",
		collection.JudicialStatusArquivado:            "processo arquivado — cobrança retomada",
	}[status]
	texto := fmt.Sprintf("Desfecho judicial: %s (%s)", status, efeito)
	if len(costs) > 0 {
		total := 0.0
		for _, c := range costs {
			total += c.Amount
		}
		texto += fmt.Sprintf("; custas lançadas: R$ %.2f", total)
	}
	if alvo != "" {
		texto += fmt.Sprintf(" [caso: %s]", alvo)
	}
	return texto
}
