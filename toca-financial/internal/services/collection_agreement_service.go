package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/collection"
)

// ErrApprovalRequired — o desconto do acordo passa do limiar de alçada e quem
// está operando não tem a permissão de aprovar. O handler traduz em 403.
var ErrApprovalRequired = errors.New("desconto acima do limiar exige aprovação")

// Limiar de alçada de desconto (mesma variável do gate M11 das cobranças).
const (
	defaultAdjustmentApprovalThresholdBRL = 5000.0
	adjustmentThresholdEnv                = "FINANCIAL_ADJUSTMENT_APPROVAL_THRESHOLD_BRL"
)

// AdjustmentApprovalThresholdBRL resolve o limiar a partir do ambiente.
// Valor inválido ou negativo cai no default — nunca em "sem limiar": erro de
// configuração não pode afrouxar o controle.
func AdjustmentApprovalThresholdBRL() float64 {
	raw := strings.TrimSpace(os.Getenv(adjustmentThresholdEnv))
	if raw == "" {
		return defaultAdjustmentApprovalThresholdBRL
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value < 0 {
		return defaultAdjustmentApprovalThresholdBRL
	}
	return value
}

// ErrAgreementConflict — já existe acordo vigente para o caso (409).
var ErrAgreementConflict = errors.New("caso já possui acordo vigente")

// WithAgreementRepository injeta o repositório do acordo (Fase 2).
func (s *CollectionService) WithAgreementRepository(repo collection.AgreementRepository) *CollectionService {
	s.agreementRepo = repo
	return s
}

// WithChargeService injeta o serviço de cobrança usado para materializar as
// parcelas do acordo e dar baixa nas cobranças (Fase 2).
func (s *CollectionService) WithChargeService(cs *ChargeService) *CollectionService {
	s.chargeService = cs
	return s
}

// WithTransactionService injeta o serviço de lançamentos usado para registrar
// as custas do processo judicial como despesa (Fase 2, G12).
func (s *CollectionService) WithTransactionService(ts *TransactionService) *CollectionService {
	s.transactionService = ts
	return s
}

// WithApprovalThreshold define o valor de desconto a partir do qual o acordo
// exige segunda assinatura (`finance:approve_adjustment`). Zero desabilita a
// exigência.
func (s *CollectionService) WithApprovalThreshold(v float64) *CollectionService {
	s.approvalThreshold = v
	return s
}

// CreateAgreement formaliza o acordo do caso (G2 do plano).
//
// Efeitos, na ordem em que acontecem (a ordem importa — ver comentário):
//  1. valida entrada e estado do caso;
//  2. cria o acordo (ACTIVE);
//  3. cria as cobranças das parcelas (entrada + N parcelas), cada uma com chave
//     de idempotência derivada de (acordo, parcela);
//  4. troca as cobranças do caso pelas parcelas e o coloca em NEGOTIATED;
//  5. cancela as cobranças originais, com motivo.
//
// Por que criar as parcelas ANTES de cancelar as originais: o pagamento de
// qualquer cobrança do caso dispara a reconciliação, que resolve o caso quando
// todas as cobranças estão fechadas. Cancelar primeiro deixaria o caso com
// apenas cobranças canceladas por alguns instantes — e o caso seria encerrado
// como RESOLVED no meio do acordo.
func (s *CollectionService) CreateAgreement(
	ctx context.Context,
	tenantID, collectionID string,
	in collection.NewAgreementInput,
	actor string,
	canApprove bool,
) (*collection.Collection, *collection.Agreement, error) {
	if s.agreementRepo == nil || s.chargeService == nil {
		return nil, nil, fmt.Errorf("agreement repository and charge service are required")
	}

	caso, err := s.GetByID(ctx, tenantID, collectionID)
	if err != nil {
		return nil, nil, err
	}
	if caso.Status == collection.StatusResolved {
		return nil, nil, fmt.Errorf("%w: caso resolvido não aceita acordo", collection.ErrInvalidStatusTransition)
	}
	// A transição é validada ANTES de qualquer escrita. Sem isso, uma transição
	// inválida (ex.: GUARANTEE_ACTIVATED → NEGOTIATED) só era descoberta na
	// atualização do caso, depois de o acordo e as parcelas já estarem gravados —
	// e o caso ficava com um acordo vigente que ninguém pediu. Foi o que a
	// validação no beta encontrou.
	if !caso.CanTransitionTo(collection.StatusNegotiated) {
		return nil, nil, fmt.Errorf("%w: caso em %s não pode ser negociado (cancele a garantia ou encerre o processo antes)",
			collection.ErrInvalidStatusTransition, caso.Status)
	}

	existing, err := s.agreementRepo.GetActiveByCollection(ctx, tenantID, collectionID)
	if err != nil {
		return nil, nil, err
	}
	if existing != nil {
		return nil, nil, fmt.Errorf("%w (acordo %s em %s)", ErrAgreementConflict, existing.ID, existing.Status)
	}

	now := time.Now()
	in.OriginalTotal = caso.TotalDebt
	in.CreatedBy = actor
	if err := in.Validate(now); err != nil {
		return nil, nil, err
	}

	// Alçada: desconto grande exige segunda assinatura. Sem a permissão, o
	// operador recebe 403 em vez de o sistema aceitar e registrar depois que
	// ninguém aprovou.
	approvalRequired := s.approvalThreshold > 0 && in.Discount > s.approvalThreshold
	if approvalRequired && !canApprove {
		return nil, nil, fmt.Errorf("%w: desconto de %.2f acima do limiar de %.2f",
			ErrApprovalRequired, in.Discount, s.approvalThreshold)
	}

	acordo := collection.NewAgreement(collectionID, tenantID, caso.ContractID, in, now)
	acordo.OriginalChargeIDs = append([]string(nil), caso.ChargeIDs...)
	acordo.ApprovalRequired = approvalRequired
	if approvalRequired && canApprove {
		acordo.ApprovedBy = actor
	}

	// Vínculo econômico das parcelas: elas SUBSTITUEM as cobranças originais, então
	// precisam carregar o mesmo dono/imóvel/tipo. Criar parcela "solta" (sem
	// owner_id/property_id) faria o pagamento não gerar repasse ao proprietário e
	// perder a alocação da locação — o dinheiro entraria e não seria distribuído.
	originais, err := s.chargeRepo.ListByIDs(ctx, tenantID, caso.ChargeIDs)
	if err != nil {
		return nil, nil, err
	}
	modelo := &charge.Charge{TenantPartyID: caso.DebtorID, TenantID: tenantID, ContractID: caso.ContractID, Type: charge.TypeOther}
	for _, c := range originais {
		if c == nil {
			continue
		}
		if modelo.OwnerID == "" {
			modelo = c
		}
	}

	// Parcelas: uma cobrança por vencimento do plano (a entrada entra como
	// cobrança com vencimento na data da primeira parcela — é dívida do devedor
	// até ser paga, mesmo quando recebida na assinatura).
	parcelas := acordo.Parcelas()
	criadas := make([]string, 0, len(parcelas))
	for _, p := range parcelas {
		descricao := fmt.Sprintf("Acordo %s — parcela %d/%d", shortID(acordo.ID), p.Numero, acordo.Installments)
		if p.EhEntrada {
			descricao = fmt.Sprintf("Acordo %s — entrada", shortID(acordo.ID))
		}
		sourceID := events.DeriveNotificationID(acordo.ID, "agreement-installment", fmt.Sprintf("%d", p.Numero))
		nova, err := s.chargeService.CreateWithSourceEventID(
			ctx, sourceID,
			caso.ContractID, modelo.DealID, modelo.LeadID, modelo.TenantPartyID, modelo.OwnerID, modelo.PropertyID,
			tenantID, modelo.Type, descricao,
			p.Vencimento.Format("2006-01"), p.Valor, p.Vencimento,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("criar parcela %d do acordo: %w", p.Numero, err)
		}
		criadas = append(criadas, nova.ID)
	}
	acordo.InstallmentChargeIDs = criadas

	if err := s.agreementRepo.Create(ctx, acordo); err != nil {
		return nil, nil, err
	}

	// O caso passa a ser as parcelas: a dívida original deixa de existir como
	// dívida (as cobranças originais serão canceladas em seguida).
	valorAcordado := acordo.TotalAmount
	caso.ChargeIDs = append([]string(nil), criadas...)
	caso.TotalDebt = valorAcordado
	caso.NegotiatedAmount = &valorAcordado
	negociadoEm := now
	caso.NegotiatedDate = &negociadoEm
	caso.Actions = append(caso.Actions, collection.Action{
		Date:        now,
		Type:        "agreement_created",
		Description: fmt.Sprintf("Acordo de R$ %.2f em %dx%s (desconto de R$ %.2f)", valorAcordado, acordo.Installments, entradaDescricao(acordo), acordo.Discount),
		UserID:      actor,
	})
	atualizado, err := s.UpdateDetails(ctx, caso, collection.StatusNegotiated, actor)
	if err != nil {
		// Compensação: sem transação envolvendo serviço de cobrança e caso (os
		// repositórios são tx-aware, mas o serviço não tem handle de pool), o que
		// resta é desfazer o que já foi gravado — cancelar as parcelas recém-criadas
		// e encerrar o acordo — para não deixar dívida nova órfã de caso.
		for _, id := range criadas {
			if _, cErr := s.chargeService.Cancel(ctx, tenantID, id, "acordo não aplicado: "+err.Error()); cErr != nil {
				log.Printf("[Collections] compensation failed for installment %s: %v", id, cErr)
			}
		}
		acordo.Cancel("acordo não aplicado: "+err.Error(), now)
		if uErr := s.agreementRepo.Update(ctx, acordo); uErr != nil {
			log.Printf("[Collections] compensation failed for agreement %s: %v", acordo.ID, uErr)
		}
		return nil, nil, err
	}

	// Só agora as originais saem: elas não pertencem mais ao caso, então o
	// cancelamento não dispara reconciliação do caso (FindByChargeID deixa de
	// encontrá-las) e não há janela em que o caso fique sem dívida.
	for _, id := range acordo.OriginalChargeIDs {
		if _, err := s.chargeService.Cancel(ctx, tenantID, id, "renegociada no acordo "+shortID(acordo.ID)); err != nil {
			// Cancelamento falho não desfaz o acordo: a cobrança antiga fica no
			// contrato (fora do caso) e a auditoria registra a pendência.
			log.Printf("[Collections] failed to cancel original charge %s of agreement %s: %v", id, acordo.ID, err)
		}
	}

	s.publishAgreementEvent(ctx, "collection.agreement_created", contracts.CollectionAgreementEventPayload{
		AgreementID:    acordo.ID,
		CollectionID:   collectionID,
		ContractID:     caso.ContractID,
		TenantID:       tenantID,
		TotalAmount:    acordo.TotalAmount,
		Discount:       acordo.Discount,
		Installments:   acordo.Installments,
		OriginalTotal:  acordo.OriginalTotal,
		InstallmentIDs: criadas,
		Status:         string(acordo.Status),
		Timestamp:      now.UTC().Format(time.RFC3339),
	})

	atualizado.Agreement = acordo
	return atualizado, acordo, nil
}

// GetAgreement devolve o acordo vigente do caso (nil quando não existe).
func (s *CollectionService) GetAgreement(ctx context.Context, tenantID, collectionID string) (*collection.Agreement, error) {
	if s.agreementRepo == nil {
		return nil, fmt.Errorf("agreement repository is required")
	}
	if _, err := s.GetByID(ctx, tenantID, collectionID); err != nil {
		return nil, err
	}
	return s.agreementRepo.GetActiveByCollection(ctx, tenantID, collectionID)
}

// BreakAgreement quebra o acordo: o caso volta a ACTIVE e as parcelas não pagas
// seguem como dívida vencida (são elas a dívida agora).
//
// `restoreDiscount` recria o desconto concedido como cobrança nova
// ("restabelecimento do desconto"), que é a prática comum quando o acordo não é
// cumprido. Fica DESLIGADO por padrão: criar dívida é decisão do operador, não
// efeito colateral de um clique.
func (s *CollectionService) BreakAgreement(ctx context.Context, tenantID, collectionID, reason, actor string, restoreDiscount bool) (*collection.Collection, *collection.Agreement, error) {
	return s.endAgreement(ctx, tenantID, collectionID, reason, actor, restoreDiscount, true)
}

// CancelAgreement encerra o acordo sem tratar como quebra (erro de cadastro).
func (s *CollectionService) CancelAgreement(ctx context.Context, tenantID, collectionID, reason, actor string) (*collection.Collection, *collection.Agreement, error) {
	return s.endAgreement(ctx, tenantID, collectionID, reason, actor, false, false)
}

func (s *CollectionService) endAgreement(ctx context.Context, tenantID, collectionID, reason, actor string, restoreDiscount, quebra bool) (*collection.Collection, *collection.Agreement, error) {
	if s.agreementRepo == nil {
		return nil, nil, fmt.Errorf("agreement repository is required")
	}
	acordo, err := s.agreementRepo.GetActiveByCollection(ctx, tenantID, collectionID)
	if err != nil {
		return nil, nil, err
	}
	if acordo == nil {
		return nil, nil, collection.ErrAgreementNotFound
	}

	now := time.Now()
	if quebra {
		err = acordo.Break(reason, now)
	} else {
		err = acordo.Cancel(reason, now)
	}
	if err != nil {
		return nil, nil, err
	}
	if err := s.agreementRepo.Update(ctx, acordo); err != nil {
		return nil, nil, err
	}

	// Encerrar o acordo desfaz o plano de parcelas:
	//
	//   - as parcelas EM ABERTO saem (não são mais a dívida: a dívida volta a ser
	//     o débito original, restabelecido abaixo). Sem isso, cancelar um acordo
	//     deixava as parcelas vivas cobrando em paralelo — a validação no beta
	//     pegou 3 parcelas de R$ 600 sobrevivendo a um acordo cancelado, ao lado
	//     de um acordo novo de R$ 1.800);
	//   - as parcelas PAGAS viram histórico;
	//   - a dívida restabelecida vira UMA cobrança nova:
	//     original − desconto − pago. Com `restore_discount`, o desconto é
	//     revogado (prática de quebra) e a dívida volta ao valor cheio.
	//
	// ORDEM IMPORTA, e foi o segundo achado da validação: a cobrança restabelecida
	// precisa entrar no caso ANTES de as parcelas serem canceladas. Cancelar
	// dispara a reconciliação do caso (todo pagamento/cancelamento de cobrança
	// passa por ela); com as parcelas ainda no caso, cancelar todas fazia o caso
	// ser encerrado como RESOLVED no meio da quebra. E a atualização seguinte
	// batia no CAS com a versão já mudada pela reconciliação (409 "collection
	// changed concurrently").
	var (
		pagas         []string
		emAberto      []string
		pagoTotal     float64
		restabelecida float64
	)
	if s.chargeService != nil && s.chargeRepo != nil {
		parcelas, err := s.chargeRepo.ListByIDs(ctx, tenantID, acordo.InstallmentChargeIDs)
		if err != nil {
			return nil, nil, err
		}
		for _, p := range parcelas {
			if p == nil {
				continue
			}
			if p.PaidAmount != nil {
				pagoTotal += p.PaidAmount.Float()
			}
			if charge.IsOutstandingStatus(p.Status) {
				emAberto = append(emAberto, p.ID)
				continue
			}
			pagas = append(pagas, p.ID)
		}
	}

	desconto := acordo.Discount
	if quebra && restoreDiscount {
		desconto = 0
	}
	restabelecida = round2(acordo.OriginalTotal - desconto - pagoTotal)
	if restabelecida < 0 {
		restabelecida = 0
	}

	// Leitura fresca do caso: a lista de parcelas acima pode ter mudado a versão
	// do caso se alguma cobrança foi paga no meio.
	caso, err := s.GetByID(ctx, tenantID, collectionID)
	if err != nil {
		return nil, nil, err
	}

	var restabelecidaID string
	if restabelecida > 0 && s.chargeService != nil {
		sourceID := events.DeriveNotificationID(acordo.ID, "agreement-debt-restored", fmt.Sprintf("%t", quebra))
		descricao := fmt.Sprintf("Dívida restabelecida — acordo %s %s", shortID(acordo.ID), map[bool]string{true: "quebrado", false: "cancelado"}[quebra])
		if quebra && restoreDiscount && acordo.Discount > 0 {
			descricao += " (desconto revogado)"
		}
		nova, err := s.chargeService.CreateWithSourceEventID(
			ctx, sourceID,
			caso.ContractID, "", "", caso.DebtorID, donoDoCaso(ctx, s, caso), imovelDoCaso(ctx, s, caso),
			tenantID, charge.TypeOther, descricao,
			now.Format("2006-01"), restabelecida, now.AddDate(0, 0, 5),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("restabelecer a dívida do acordo: %w", err)
		}
		restabelecidaID = nova.ID
	}

	caso.ChargeIDs = append(append([]string(nil), pagas...), restabelecidaID)
	for _, id := range caso.ChargeIDs {
		if id == "" {
			caso.ChargeIDs = caso.ChargeIDs[:len(caso.ChargeIDs)-1]
			break
		}
	}
	caso.TotalDebt = restabelecida

	acao := "agreement_cancelled"
	if quebra {
		acao = "agreement_broken"
	}
	caso.Actions = append(caso.Actions, collection.Action{
		Date:        now,
		Type:        acao,
		Description: fmt.Sprintf("Acordo %s: %s (dívida restabelecida em R$ %.2f)", shortID(acordo.ID), reason, restabelecida),
		UserID:      actor,
	})
	atualizado, err := s.UpdateDetails(ctx, caso, collection.StatusActive, actor)
	if err != nil {
		return nil, nil, err
	}

	// Só agora as parcelas em aberto saem: elas já não pertencem ao caso, então o
	// cancelamento não dispara reconciliação nem encerra o caso por engano.
	for _, id := range emAberto {
		if _, err := s.chargeService.Cancel(ctx, tenantID, id, "acordo encerrado: "+reason); err != nil {
			log.Printf("[Collections] failed to cancel open installment %s: %v", id, err)
		}
	}

	evento := "collection.agreement_cancelled"
	if quebra {
		evento = "collection.agreement_broken"
	}
	s.publishAgreementEvent(ctx, evento, contracts.CollectionAgreementEventPayload{
		AgreementID:   acordo.ID,
		CollectionID:  collectionID,
		ContractID:    caso.ContractID,
		TenantID:      tenantID,
		TotalAmount:   acordo.TotalAmount,
		Discount:      acordo.Discount,
		Installments:  acordo.Installments,
		OriginalTotal: acordo.OriginalTotal,
		Status:        string(acordo.Status),
		Reason:        reason,
		Timestamp:     now.UTC().Format(time.RFC3339),
	})

	atualizado.Agreement = acordo
	return atualizado, acordo, nil
}

// recalcularDividaDoCaso soma o saldo das cobranças abertas do caso.
func (s *CollectionService) recalcularDividaDoCaso(ctx context.Context, caso *collection.Collection) (float64, error) {
	if s.chargeRepo == nil || caso == nil || len(caso.ChargeIDs) == 0 {
		return caso.TotalDebt, nil
	}
	charges, err := s.chargeRepo.ListByIDs(ctx, caso.TenantID, caso.ChargeIDs)
	if err != nil {
		return 0, err
	}
	total := 0.0
	for _, c := range charges {
		if c == nil || !charge.IsOutstandingStatus(c.Status) {
			continue
		}
		saldo := c.TotalAmount.Float()
		if c.PaidAmount != nil {
			saldo -= c.PaidAmount.Float()
		}
		if saldo > 0 {
			total += saldo
		}
	}
	return total, nil
}

// ReconcileAgreement marca o acordo como cumprido quando todas as parcelas
// estão fechadas. Chamado no caminho de pagamento (ResolveChargePayment).
//
// Só o cumprimento é automático: quebra de acordo tem efeito financeiro (volta a
// cobrar, pode restabelecer desconto) e é decisão do operador, não do motor.
func (s *CollectionService) ReconcileAgreement(ctx context.Context, tenantID, collectionID string) {
	if s.agreementRepo == nil || s.chargeRepo == nil {
		return
	}
	acordo, err := s.agreementRepo.GetActiveByCollection(ctx, tenantID, collectionID)
	if err != nil || acordo == nil {
		return
	}
	if len(acordo.InstallmentChargeIDs) == 0 {
		return
	}
	charges, err := s.chargeRepo.ListByIDs(ctx, tenantID, acordo.InstallmentChargeIDs)
	if err != nil {
		return
	}
	for _, c := range charges {
		if c != nil && charge.IsOutstandingStatus(c.Status) {
			return // ainda há parcela aberta
		}
	}
	acordo.Fulfill(time.Now())
	if err := s.agreementRepo.Update(ctx, acordo); err != nil {
		log.Printf("[Collections] failed to fulfill agreement %s: %v", acordo.ID, err)
		return
	}
	audit.Record(ctx, audit.Entry{
		Action:     "collection.agreement_fulfilled",
		EntityType: "Collection",
		EntityID:   collectionID,
		After:      map[string]any{"agreement_id": acordo.ID},
		Metadata:   map[string]any{"path": "ReconcileAgreement", "tenant_id": tenantID},
	})
}

func (s *CollectionService) publishAgreementEvent(ctx context.Context, name string, payload contracts.CollectionAgreementEventPayload) {
	if s.eventBus == nil {
		return
	}
	event := contracts.CollectionAgreementEvent{
		EventIDVal: events.DeriveNotificationID(payload.AgreementID, name, payload.Status),
		Data:       payload,
	}
	event.EventName = name
	if err := s.eventBus.PublishReliable(ctx, event); err != nil {
		log.Printf("[Collections] failed to publish %s for agreement %s: %v", name, payload.AgreementID, err)
	}
}

func entradaDescricao(a *collection.Agreement) string {
	if a == nil || a.DownPayment <= 0 {
		return ""
	}
	return fmt.Sprintf(" com entrada de R$ %.2f", a.DownPayment)
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

// donoDoCaso e imovelDoCaso leem o vínculo econômico das cobranças do caso.
//
// Usado no restabelecimento de desconto (acordo quebrado): a cobrança nova
// precisa nascer com o mesmo dono/imóvel, senão o pagamento dela não gera
// repasse ao proprietário.
func donoDoCaso(ctx context.Context, s *CollectionService, caso *collection.Collection) string {
	c := primeiraCobrancaDoCaso(ctx, s, caso)
	if c == nil {
		return ""
	}
	return c.OwnerID
}

func imovelDoCaso(ctx context.Context, s *CollectionService, caso *collection.Collection) string {
	c := primeiraCobrancaDoCaso(ctx, s, caso)
	if c == nil {
		return ""
	}
	return c.PropertyID
}

func primeiraCobrancaDoCaso(ctx context.Context, s *CollectionService, caso *collection.Collection) *charge.Charge {
	if s.chargeRepo == nil || caso == nil || len(caso.ChargeIDs) == 0 {
		return nil
	}
	charges, err := s.chargeRepo.ListByIDs(ctx, caso.TenantID, caso.ChargeIDs)
	if err != nil {
		return nil
	}
	for _, c := range charges {
		if c != nil {
			return c
		}
	}
	return nil
}
