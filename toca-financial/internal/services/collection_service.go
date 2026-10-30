package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/audit"
	valueobjects "github.com/oginoino/toca/pkg/domain/valueobjects"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/collection"
	"github.com/oginoino/toca/toca-financial/internal/insurer"
)

// ErrInvalidJudicial — dados do processo judicial inconsistentes (ex.:
// número CNJ malformado, status desconhecido). O handler trata como 400.
var ErrInvalidJudicial = errors.New("dados do processo judicial inválidos")

// ErrInvalidGuarantee — dados do acionamento de garantia inconsistentes
// (tipo ou status desconhecido, valor não positivo). Handler trata 400.
var ErrInvalidGuarantee = errors.New("dados do acionamento de garantia inválidos")

// ErrStatusRequiresDomainData — tentativa de assumir um status que pertence a um
// fluxo dedicado (garantia/judicial) sem os dados daquele fluxo. O handler
// traduz em 409.
var ErrStatusRequiresDomainData = errors.New("status exige dados do fluxo correspondente")

// ErrChargeAlreadyInCase — a cobrança informada já pertence a um caso aberto.
// O handler traduz em 409 com o id do caso.
var ErrChargeAlreadyInCase = errors.New("cobrança já está em um caso de cobrança aberto")

// ErrInvalidCollectionInput — a criação manual do caso recebeu cobranças que
// não existem, não são do tenant, não estão vencidas, já estão fechadas ou cuja
// soma não bate com a dívida declarada. O handler trata como 400.
//
// Antes desta validação, `POST /collections` aceitava `charge_ids` e
// `total_debt` livres: dava para abrir um caso com dívida inventada ou
// apontando para cobrança inexistente (ou de outro tenant), e nada reclamava —
// o caso ficava no KPI e o motor nunca o corrigia.
var ErrInvalidCollectionInput = errors.New("dados do caso de cobrança inválidos")

// toleranceCents é a folga aceita entre a soma das cobranças e a dívida
// declarada, em centavos, para absorver arredondamento de formulário.
const toleranceCents int64 = 1

type CollectionService struct {
	repo     collection.RepositoryInterface
	eventBus events.EventBus
	// insurer aciona o seguro fiança no acionamento de garantia (#447).
	// Opcional — quando nil, ActivateGuarantee usa insurer.Default()
	// (adaptador manual), então o fluxo nunca trava por integração.
	insurer insurer.Insurer
	// chargeRepo valida as cobranças da criação manual (F0.7 do plano de
	// inadimplência). Opcional para não quebrar construção em teste, mas o
	// router de produção SEMPRE injeta: sem ele a validação é pulada.
	chargeRepo charge.RepositoryInterface
	// ── Fase 2 ──
	// agreementRepo guarda o acordo formalizado (financeiro.collection_agreements).
	agreementRepo collection.AgreementRepository
	// chargeService materializa as parcelas do acordo e dá baixa nas cobranças
	// quando a garantia paga ou o judicial termina.
	chargeService *ChargeService
	// transactionService lança as custas do processo judicial como despesa (G12).
	transactionService *TransactionService
	// approvalThreshold é o desconto a partir do qual o acordo exige segunda
	// assinatura (`finance:approve_adjustment`).
	approvalThreshold float64
	// transferHoldReleaser libera a retenção de repasse por inadimplência quando o
	// caso deixa de estar aberto (a razão da retenção desapareceu). Injetado como
	// função para o serviço de cobrança não depender do serviço de repasse.
	transferHoldReleaser func(ctx context.Context, tenantID, contractID, reason string) (int, error)
}

// WithTransferHoldReleaser injeta quem libera a retenção de repasse do contrato.
func (s *CollectionService) WithTransferHoldReleaser(fn func(ctx context.Context, tenantID, contractID, reason string) (int, error)) *CollectionService {
	s.transferHoldReleaser = fn
	return s
}

// OpenCaseDebtForContract devolve a dívida em aberto do contrato na inadimplência
// e o caso que a sustenta — é o insumo da política de retenção de repasse.
//
// Soma os casos NÃO resolvidos do contrato. Contrato sem caso aberto devolve zero,
// e a retenção simplesmente não acontece.
func (s *CollectionService) OpenCaseDebtForContract(ctx context.Context, tenantID, contractID string) (float64, string) {
	if s.repo == nil || strings.TrimSpace(contractID) == "" {
		return 0, ""
	}
	casos, err := s.repo.FindByContract(ctx, contractID, tenantID)
	if err != nil {
		return 0, ""
	}
	total := 0.0
	var principal string
	for _, c := range casos {
		if c == nil || c.Status == collection.StatusResolved {
			continue
		}
		total += c.TotalDebt
		if principal == "" {
			principal = c.ID
		}
	}
	return total, principal
}

// releaseTransferHoldsForContract libera a retenção dos repasses do contrato
// quando o caso deixa de estar aberto. Best-effort: falhar aqui não pode desfazer
// a resolução do caso (o operador ainda pode liberar manualmente).
func (s *CollectionService) releaseTransferHoldsForContract(ctx context.Context, tenantID, contractID, reason string) {
	if s.transferHoldReleaser == nil || strings.TrimSpace(contractID) == "" {
		return
	}
	if _, err := s.transferHoldReleaser(ctx, tenantID, contractID, reason); err != nil {
		log.Printf("[Collections] failed to release transfer holds for contract %s: %v", contractID, err)
	}
}

func NewCollectionService(repo collection.RepositoryInterface) *CollectionService {
	return &CollectionService{repo: repo}
}

// WithChargeRepository injeta o repositório de cobranças usado para validar a
// criação manual do caso (existência, tenant, vencimento, status e soma).
func (s *CollectionService) WithChargeRepository(repo charge.RepositoryInterface) *CollectionService {
	s.chargeRepo = repo
	return s
}

// validateCreate confere as cobranças informadas na criação manual do caso.
//
// Regras (plano §7.1: nenhum estado financeiro é inventado por fora):
//   - a lista não pode ser vazia nem conter duplicatas;
//   - toda cobrança precisa existir PARA O TENANT (o repositório já filtra por
//     tenant, então uma cobrança de outro tenant simplesmente não volta);
//   - toda cobrança precisa estar vencida e ainda aberta (PENDING, OVERDUE ou
//     PARTIALLY_PAID) — caso não se abre sobre cobrança paga ou futura;
//   - a soma dos saldos remanescentes precisa bater com a dívida declarada.
func (s *CollectionService) validateCreate(ctx context.Context, tenantID string, chargeIDs []string, totalDebt float64) error {
	if s.chargeRepo == nil {
		return nil
	}
	if len(chargeIDs) == 0 {
		return fmt.Errorf("%w: informe ao menos uma cobrança", ErrInvalidCollectionInput)
	}

	vistos := make(map[string]struct{}, len(chargeIDs))
	unicos := make([]string, 0, len(chargeIDs))
	for _, id := range chargeIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			return fmt.Errorf("%w: cobrança sem identificador", ErrInvalidCollectionInput)
		}
		if _, repetido := vistos[id]; repetido {
			return fmt.Errorf("%w: cobrança %s repetida", ErrInvalidCollectionInput, id)
		}
		vistos[id] = struct{}{}
		unicos = append(unicos, id)
	}

	charges, err := s.chargeRepo.ListByIDs(ctx, tenantID, unicos)
	if err != nil {
		return err
	}
	if len(charges) != len(unicos) {
		return fmt.Errorf("%w: cobrança inexistente para este tenant (%d informadas, %d encontradas)",
			ErrInvalidCollectionInput, len(unicos), len(charges))
	}

	agora := time.Now()
	var somaCents int64
	for _, c := range charges {
		// Cobrança que já está num caso ABERTO não pode abrir outro: sem esta
		// guarda, a abertura manual (botão na tela) criaria um caso paralelo para
		// a mesma dívida e o motor passaria a atualizar os dois.
		if caso, err := s.repo.FindByChargeID(ctx, tenantID, c.ID); err == nil && caso != nil && caso.Status != collection.StatusResolved {
			// Id completo do caso na mensagem: é o que o operador usa para abrir o
			// caso existente (a versão curta serve para caber na tela, não aqui).
			return fmt.Errorf("%w: cobrança %s já está no caso %s (%s)",
				ErrChargeAlreadyInCase, c.ID, caso.ID, caso.Status)
		}
		if c == nil {
			return fmt.Errorf("%w: cobrança inexistente para este tenant", ErrInvalidCollectionInput)
		}
		if !charge.IsOutstandingStatus(c.Status) {
			return fmt.Errorf("%w: cobrança %s está %s e não pode abrir caso", ErrInvalidCollectionInput, c.ID, c.Status)
		}
		if !c.DueDate.Before(agora) {
			return fmt.Errorf("%w: cobrança %s ainda não venceu", ErrInvalidCollectionInput, c.ID)
		}
		saldo := c.TotalAmount.Cents()
		if c.PaidAmount != nil {
			saldo -= c.PaidAmount.Cents()
		}
		if saldo < 0 {
			saldo = 0
		}
		somaCents += saldo
	}

	if declarado := valueobjects.NewMoneyFromFloat(totalDebt).Cents(); absInt64(somaCents-declarado) > toleranceCents {
		return fmt.Errorf("%w: dívida declarada (%.2f) difere da soma das cobranças (%.2f)",
			ErrInvalidCollectionInput, float64(declarado)/100, float64(somaCents)/100)
	}
	return nil
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// WithEventBus wires the publisher used by UpdateStatus. Optional — without
// it the status transitions still happen in storage, just without broadcast.
func (s *CollectionService) WithEventBus(bus events.EventBus) *CollectionService {
	s.eventBus = bus
	return s
}

// WithInsurer injeta o adaptador de seguradora usado por
// ActivateGuarantee (#447). Encadeável; sem ele o serviço cai em
// insurer.Default() (adaptador manual).
func (s *CollectionService) WithInsurer(in insurer.Insurer) *CollectionService {
	s.insurer = in
	return s
}

func (s *CollectionService) Create(ctx context.Context, contractID, debtorID, debtorName, tenantID string, totalDebt float64, daysOverdue int, chargeIDs []string) (*collection.Collection, error) {
	if err := s.validateCreate(ctx, tenantID, chargeIDs, totalDebt); err != nil {
		return nil, err
	}
	c := collection.New(contractID, debtorID, debtorName, tenantID, totalDebt, daysOverdue, chargeIDs)
	if err := s.repo.Create(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// CreateIdempotent cria a collection com proteção ON CONFLICT via
// source_event_id. Usada pelo handler quando o header Idempotency-Key
// está presente (FIN-COM-12).
func (s *CollectionService) CreateIdempotent(ctx context.Context, contractID, debtorID, debtorName, tenantID string, totalDebt float64, daysOverdue int, chargeIDs []string, sourceEventID string) (*collection.Collection, error) {
	if err := s.validateCreate(ctx, tenantID, chargeIDs, totalDebt); err != nil {
		return nil, err
	}
	c := collection.New(contractID, debtorID, debtorName, tenantID, totalDebt, daysOverdue, chargeIDs)
	c.SourceEventID = sourceEventID
	if err := s.repo.CreateIdempotent(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *CollectionService) GetByID(ctx context.Context, tenantID, id string) (*collection.Collection, error) {
	c, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, fmt.Errorf("collection not found")
	}
	return c, nil
}

func updateCollectionIfCurrent(ctx context.Context, repo collection.RepositoryInterface, c *collection.Collection, expectedUpdatedAt time.Time) error {
	if versioned, ok := repo.(collection.VersionedRepository); ok {
		persisted, err := versioned.UpdateIfVersion(ctx, c, expectedUpdatedAt)
		if err != nil {
			return err
		}
		if !persisted {
			return collection.ErrConcurrentModification
		}
		return nil
	}
	return repo.Update(ctx, c)
}

func (s *CollectionService) Update(ctx context.Context, c *collection.Collection) error {
	if c == nil {
		return fmt.Errorf("collection is nil")
	}
	current, err := s.repo.GetByID(ctx, c.TenantID, c.ID)
	if err != nil {
		return err
	}
	if current == nil {
		return fmt.Errorf("collection not found")
	}
	if current.Status != c.Status {
		return collection.ErrStatusTransitionRequired
	}
	if !current.UpdatedAt.Equal(c.UpdatedAt) {
		return collection.ErrConcurrentModification
	}
	return updateCollectionIfCurrent(ctx, s.repo, c, current.UpdatedAt)
}

// UpdateDetails persists editable fields and an optional lifecycle transition
// in a single version-checked write. Generic handler updates therefore cannot
// bypass the state machine or overwrite a concurrent payment resolution.
func (s *CollectionService) UpdateDetails(ctx context.Context, c *collection.Collection, newStatus collection.Status, actor string) (*collection.Collection, error) {
	if c == nil {
		return nil, fmt.Errorf("collection is nil")
	}
	current, err := s.repo.GetByID(ctx, c.TenantID, c.ID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, fmt.Errorf("collection not found")
	}
	if !c.UpdatedAt.Equal(current.UpdatedAt) {
		return nil, collection.ErrConcurrentModification
	}
	previous := current.Status
	if newStatus == "" {
		newStatus = previous
	}
	if !current.CanTransitionTo(newStatus) {
		return nil, fmt.Errorf("%w: %s -> %s", collection.ErrInvalidStatusTransition, previous, newStatus)
	}
	c.Status = newStatus
	if err := updateCollectionIfCurrent(ctx, s.repo, c, current.UpdatedAt); err != nil {
		return nil, err
	}
	if previous != newStatus {
		s.recordStatusTransition(ctx, c, previous, newStatus, actor)
		// Caso encerrado por aqui (é o caminho do `PUT /collections/:id`): a razão
		// da retenção de repasse desapareceu. Sem isto o dinheiro do proprietário
		// ficaria preso até alguém liberar à mão — foi o que a validação no beta
		// pegou (o gancho existia só no UpdateStatus, que o PUT não usa).
		if newStatus == collection.StatusResolved {
			s.releaseTransferHoldsForContract(ctx, c.TenantID, c.ContractID, "caso de cobrança "+shortID(c.ID)+" resolvido")
		}
	}
	return c, nil
}

// UpdateStatus transitions the Collection between buckets and publishes a
// CollectionEscalatedEvent so cross-service auditors (toca-deal,
// toca-comm) can react. The transition itself is allowed only through
// this method — direct Update() calls remain available for non-status
// edits (totals, actions log) but should not be used to flip Status,
// otherwise the audit chain breaks.
func (s *CollectionService) UpdateStatus(ctx context.Context, tenantID, id string, newStatus collection.Status, escalatedBy string) (*collection.Collection, error) {
	c, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, fmt.Errorf("collection not found")
	}
	previous := c.Status
	if previous == newStatus {
		return c, nil // idempotent — no event
	}
	// Status que pertencem a um fluxo dedicado só podem ser assumidos quando os
	// dados daquele fluxo existem. Antes, um `PUT /collections/:id` com
	// {"status":"GUARANTEE_ACTIVATED"} deixava o caso em garantia SEM acionamento
	// registrado (a validação no beta criou esse estado e o fluxo de garantia
	// passou a responder "cobrança sem acionamento de garantia").
	if newStatus == collection.StatusGuaranteeActivated && c.Guarantee == nil {
		return nil, fmt.Errorf("%w: registre o acionamento antes de mover o caso para garantia acionada", ErrStatusRequiresDomainData)
	}
	if newStatus == collection.StatusJudicial && c.Judicial == nil {
		return nil, fmt.Errorf("%w: registre o processo judicial antes de mover o caso para judicial", ErrStatusRequiresDomainData)
	}
	if !c.CanTransitionTo(newStatus) {
		return nil, fmt.Errorf("%w: %s -> %s", collection.ErrInvalidStatusTransition, previous, newStatus)
	}
	expectedUpdatedAt := c.UpdatedAt
	c.Status = newStatus
	if err := updateCollectionIfCurrent(ctx, s.repo, c, expectedUpdatedAt); err != nil {
		return nil, err
	}
	s.recordStatusTransition(ctx, c, previous, newStatus, escalatedBy)

	// Caso encerrado: a razão da retenção de repasse desapareceu. Liberar aqui não
	// paga nada — o pagamento continua exigindo aprovação —, só devolve o repasse
	// ao fluxo normal para o dinheiro do proprietário não ficar preso por
	// esquecimento.
	if newStatus == collection.StatusResolved {
		s.releaseTransferHoldsForContract(ctx, tenantID, c.ContractID, "caso de cobrança "+shortID(c.ID)+" resolvido")
	}
	return c, nil
}

func (s *CollectionService) recordStatusTransition(ctx context.Context, c *collection.Collection, previous, newStatus collection.Status, escalatedBy string) {
	audit.Record(ctx, audit.Entry{
		Action:     "collection.escalated",
		EntityType: "Collection",
		EntityID:   c.ID,
		Before:     map[string]any{"status": string(previous)},
		After:      map[string]any{"status": string(newStatus)},
		Metadata: map[string]any{
			"escalated_by": escalatedBy,
			"contract_id":  c.ContractID,
			"total_debt":   c.TotalDebt,
		},
	})

	if s.eventBus != nil {
		evt := contracts.CollectionEscalatedEvent{
			EventIDVal: events.DeriveNotificationID(c.ID, "collection.escalated", string(newStatus)),
			Data: contracts.CollectionEscalatedEventPayload{
				CollectionID:   c.ID,
				ContractID:     c.ContractID,
				TenantID:       c.TenantID,
				DebtorID:       c.DebtorID,
				PreviousStatus: string(previous),
				NewStatus:      string(newStatus),
				TotalDebt:      c.TotalDebt,
				EscalatedBy:    escalatedBy,
				Timestamp:      time.Now().UTC().Format(time.RFC3339),
			},
		}
		if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
			log.Printf("collection.escalated: failed to publish for collection=%s: %v", c.ID, err)
		}
	}
}

func (s *CollectionService) AddAction(ctx context.Context, tenantID, id string, action collection.Action) error {
	c, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if c == nil {
		return fmt.Errorf("collection not found")
	}
	return s.repo.AddAction(ctx, tenantID, id, action)
}

func (s *CollectionService) List(ctx context.Context, tenantID, status string, limit, offset int64) ([]*collection.Collection, int64, error) {
	return s.repo.List(ctx, tenantID, status, limit, offset)
}

// SetJudicialProcess registra (ou atualiza) os metadados do processo
// judicial de uma cobrança (#448): número CNJ, vara, advogado, valor da
// causa, status e próxima audiência. Valida o CNJ e o status; grava o
// processo no metadata da Collection e — se ela ainda não estava
// JUDICIAL — escala via UpdateStatus (que publica o evento). Atualizar
// um processo já existente preserva o CreatedAt original.
func (s *CollectionService) SetJudicialProcess(ctx context.Context, tenantID, id string, jp collection.JudicialProcess, actor string) (*collection.Collection, error) {
	c, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, fmt.Errorf("collection not found")
	}
	previousCollectionStatus := c.Status
	if !c.CanTransitionTo(collection.StatusJudicial) {
		return nil, fmt.Errorf("%w: %s -> %s", collection.ErrInvalidStatusTransition, c.Status, collection.StatusJudicial)
	}
	expectedUpdatedAt := c.UpdatedAt

	if !collection.ValidateCNJ(jp.CaseNumber) {
		return nil, fmt.Errorf("%w: número CNJ %q malformado", ErrInvalidJudicial, jp.CaseNumber)
	}
	if jp.Status == "" {
		jp.Status = collection.JudicialStatusEmTramite
	}
	if !collection.ValidJudicialStatus(jp.Status) {
		return nil, fmt.Errorf("%w: status %q desconhecido", ErrInvalidJudicial, jp.Status)
	}

	now := time.Now()
	// Atualizar um processo já registrado preserva o CreatedAt original.
	if c.Judicial != nil && !c.Judicial.CreatedAt.IsZero() {
		jp.CreatedAt = c.Judicial.CreatedAt
	} else {
		jp.CreatedAt = now
	}
	jp.UpdatedAt = now
	c.Judicial = &jp
	c.Actions = append(c.Actions, collection.Action{
		Date:        now,
		Type:        "judicial_process",
		Description: fmt.Sprintf("Processo judicial %s registrado (%s)", jp.CaseNumber, jp.Status),
		UserID:      actor,
	})
	c.Status = collection.StatusJudicial
	if err := updateCollectionIfCurrent(ctx, s.repo, c, expectedUpdatedAt); err != nil {
		return nil, err
	}

	audit.Record(ctx, audit.Entry{
		Action:     "collection.judicial_registered",
		EntityType: "Collection",
		EntityID:   c.ID,
		After: map[string]any{
			"case_number": jp.CaseNumber,
			"court":       jp.Court,
			"status":      string(jp.Status),
			"claim_value": jp.ClaimValue,
		},
		Metadata: map[string]any{"contract_id": c.ContractID, "actor": actor},
	})

	if previousCollectionStatus != collection.StatusJudicial {
		s.recordStatusTransition(ctx, c, previousCollectionStatus, collection.StatusJudicial, actor)
	}
	return c, nil
}

// ActivateGuarantee aciona a garantia locatícia de uma cobrança (#447):
// registra o GuaranteeActivation, escala a cobrança para
// GUARANTEE_ACTIVATED via UpdateStatus e — quando a garantia é Seguro
// Fiança — chama o adaptador da seguradora para abrir o acionamento.
// Reacionar sobrescreve o registro anterior.
func (s *CollectionService) ActivateGuarantee(ctx context.Context, tenantID, id string, gtype collection.GuaranteeType, amount float64, reason, actor string) (*collection.Collection, error) {
	c, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, fmt.Errorf("collection not found")
	}
	previousCollectionStatus := c.Status
	if !c.CanTransitionTo(collection.StatusGuaranteeActivated) {
		return nil, fmt.Errorf("%w: %s -> %s", collection.ErrInvalidStatusTransition, c.Status, collection.StatusGuaranteeActivated)
	}
	expectedUpdatedAt := c.UpdatedAt
	if !collection.ValidGuaranteeType(gtype) {
		return nil, fmt.Errorf("%w: tipo de garantia %q desconhecido", ErrInvalidGuarantee, gtype)
	}
	if amount <= 0 {
		return nil, fmt.Errorf("%w: valor acionado deve ser positivo", ErrInvalidGuarantee)
	}

	now := time.Now()
	ga := collection.GuaranteeActivation{
		Type:        gtype,
		Amount:      amount,
		Reason:      reason,
		Status:      collection.GuaranteeStatusRequested,
		RequestedAt: now,
		UpdatedAt:   now,
	}

	// Seguro Fiança envolve uma seguradora — aciona pelo adaptador. Para
	// Fiador/Caução a garantia é interna, sem integração externa.
	if gtype == collection.GuaranteeTypeSeguroFianca {
		ins := s.insurer
		if ins == nil {
			ins = insurer.Default()
		}
		res, ierr := ins.Activate(ctx, insurer.Request{
			CollectionID: c.ID,
			ContractID:   c.ContractID,
			TenantID:     c.TenantID,
			Amount:       amount,
			Reason:       reason,
		})
		if ierr != nil {
			return nil, fmt.Errorf("acionamento na seguradora falhou: %w", ierr)
		}
		ga.InsurerName = res.InsurerName
		ga.InsurerProtocol = res.Protocol
		ga.InsurerResponse = res.Message
		if res.Accepted {
			ga.Status = collection.GuaranteeStatusAccepted
		}
	}

	c.Guarantee = &ga
	c.Actions = append(c.Actions, collection.Action{
		Date:        now,
		Type:        "guarantee_activation",
		Description: fmt.Sprintf("Garantia %s acionada — valor %.2f (%s)", ga.Type, ga.Amount, ga.Status),
		UserID:      actor,
	})
	c.Status = collection.StatusGuaranteeActivated
	if err := updateCollectionIfCurrent(ctx, s.repo, c, expectedUpdatedAt); err != nil {
		return nil, err
	}

	audit.Record(ctx, audit.Entry{
		Action:     "collection.guarantee_activated",
		EntityType: "Collection",
		EntityID:   c.ID,
		After: map[string]any{
			"guarantee_type": string(ga.Type),
			"amount":         ga.Amount,
			"status":         string(ga.Status),
			"insurer_name":   ga.InsurerName,
		},
		Metadata: map[string]any{"contract_id": c.ContractID, "actor": actor},
	})

	if previousCollectionStatus != collection.StatusGuaranteeActivated {
		s.recordStatusTransition(ctx, c, previousCollectionStatus, collection.StatusGuaranteeActivated, actor)
	}
	return c, nil
}

// UpdateGuaranteeStatus registra a resposta do garantidor/seguradora
// sobre um acionamento já aberto (#447) — é o ponto de entrada que um
// webhook de seguradora também usaria. Marca ResolvedAt quando o status
// passa a ser terminal.
func (s *CollectionService) UpdateGuaranteeStatus(ctx context.Context, tenantID, id string, newStatus collection.GuaranteeActivationStatus, insurerResponse, actor string) (*collection.Collection, error) {
	c, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, fmt.Errorf("collection not found")
	}
	if c.Guarantee == nil {
		return nil, fmt.Errorf("%w: cobrança sem acionamento de garantia", ErrInvalidGuarantee)
	}
	expectedUpdatedAt := c.UpdatedAt
	if !collection.ValidGuaranteeActivationStatus(newStatus) {
		return nil, fmt.Errorf("%w: status %q desconhecido", ErrInvalidGuarantee, newStatus)
	}

	previousStatus := c.Guarantee.Status
	if previousStatus == newStatus {
		return c, nil
	}
	if previousStatus.IsTerminal() {
		return nil, fmt.Errorf("%w: acionamento terminal %s não pode transicionar para %s", ErrInvalidGuarantee, previousStatus, newStatus)
	}

	now := time.Now()
	c.Guarantee.Status = newStatus
	if insurerResponse != "" {
		c.Guarantee.InsurerResponse = insurerResponse
	}
	c.Guarantee.UpdatedAt = now
	if newStatus.IsTerminal() && c.Guarantee.ResolvedAt == nil {
		c.Guarantee.ResolvedAt = &now
	}
	c.Actions = append(c.Actions, collection.Action{
		Date:        now,
		Type:        "guarantee_activation",
		Description: fmt.Sprintf("Acionamento de garantia atualizado para %s", newStatus),
		UserID:      actor,
	})
	if err := updateCollectionIfCurrent(ctx, s.repo, c, expectedUpdatedAt); err != nil {
		return nil, err
	}

	audit.Record(ctx, audit.Entry{
		Action:     "collection.guarantee_updated",
		EntityType: "Collection",
		EntityID:   c.ID,
		Before:     map[string]any{"status": string(previousStatus)},
		After:      map[string]any{"status": string(newStatus)},
		Metadata:   map[string]any{"contract_id": c.ContractID, "actor": actor},
	})
	return c, nil
}
