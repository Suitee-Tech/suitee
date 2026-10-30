package services

import (
	"context"
	"errors"
	"log"
	"strconv"
	"time"

	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/pkg/metrics"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// proposalFollowUpLockKey é a chave do pg_try_advisory_lock que coordena o tick
// entre réplicas — mesmo padrão do lembrete de chat (toca-comm) e da régua
// financeira (toca-financial, #445).
const proposalFollowUpLockKey = "toca-deal.proposal-followup.tick"

// ProposalFollowUpRepository é a superfície mínima que o motor precisa: a
// varredura cross-tenant (pool de serviço) e o registro do estágio disparado.
// Interface estreita de propósito, para os fakes de teste não precisarem do
// repositório inteiro.
//
// O mark é uma escrita pontual por (deal, proposta) — e não um Update do
// agregado — para o worker sem tenant no contexto escrever pelo pool de serviço
// sem reescrever o documento inteiro (ver deal_postgres.go).
type ProposalFollowUpRepository interface {
	ListProposalsAwaitingResponse(ctx context.Context, olderThan, notOlderThan time.Time, limit int) ([]*deal.Deal, error)
	MarkProposalFollowUp(ctx context.Context, tenantID, dealID, proposalID string, followUps int, at time.Time) error
}

// ProposalFollowUpTickResult resume uma varredura (para log e testes).
type ProposalFollowUpTickResult struct {
	Scanned  int `json:"scanned"`
	Enqueued int `json:"enqueued"`
	Skipped  int `json:"skipped"`
	Errors   int `json:"errors"`
}

// ProposalFollowUpEngine dispara o lembrete temporal de proposta sem resposta
// (plano beta §F6).
//
// Semântica de idempotência — a mesma correção do lembrete de chat: o evento é
// ancorado no GRUPO (proposta + estágio), não na janela de tempo. Duas
// varreduras seguidas produzem o MESMO event_id enquanto o estágio não avança,
// e o dedup do toca-comm (`comm.sent_externals`) colapsa a segunda. Um novo
// estágio é um grupo novo; proposta respondida mata a régua.
type ProposalFollowUpEngine struct {
	repo  ProposalFollowUpRepository
	bus   events.EventBus
	db    *database.PostgresDB
	sched deal.ProposalFollowUpSchedule
	// scanLimit limita o lote por tick (motor pode ser chamado de novo no
	// próximo tick; a ordenação por created_at garante que ninguém fica
	// preso para sempre).
	scanLimit int
	now       func() time.Time
}

// NewProposalFollowUpEngine monta o motor. Devolve erro quando o repositório
// não implementa a varredura cross-tenant — melhor falhar alto no boot do que
// subir um motor que nunca encontra nada.
func NewProposalFollowUpEngine(repo ProposalFollowUpRepository, bus events.EventBus, db *database.PostgresDB, sched deal.ProposalFollowUpSchedule) (*ProposalFollowUpEngine, error) {
	if repo == nil {
		return nil, errors.New("proposal follow-up engine requires a repository")
	}
	if bus == nil {
		return nil, errors.New("proposal follow-up engine requires an event bus")
	}
	return &ProposalFollowUpEngine{
		repo:      repo,
		bus:       bus,
		db:        db,
		sched:     sched.Norm(),
		scanLimit: 200,
		now:       time.Now,
	}, nil
}

// Enabled informa se a régua tem pelo menos um estágio configurado.
func (e *ProposalFollowUpEngine) Enabled() bool {
	return e != nil && e.sched.MaxStages > 0
}

// Tick executa uma varredura. Erros por negócio são contabilizados e não
// abortam o lote; o lock garante que só uma réplica processa por vez.
func (e *ProposalFollowUpEngine) Tick(ctx context.Context) (ProposalFollowUpTickResult, error) {
	var result ProposalFollowUpTickResult
	if !e.Enabled() {
		return result, nil
	}
	e.runWithLock(ctx, func(ctx context.Context) {
		result = e.tickLocked(ctx)
	})
	return result, nil
}

func (e *ProposalFollowUpEngine) tickLocked(ctx context.Context) ProposalFollowUpTickResult {
	result := ProposalFollowUpTickResult{}
	now := e.now()

	// Nada mais novo que now-stage1Days pode ter lembrete devido: o filtro
	// corta a varredura sem perder candidato.
	olderThan := now.AddDate(0, 0, -e.sched.Stage1Days)
	notOlderThan := now.AddDate(0, 0, -e.sched.MaxAgeDays)
	deals, err := e.repo.ListProposalsAwaitingResponse(ctx, olderThan, notOlderThan, e.scanLimit)
	if err != nil {
		result.Errors++
		log.Printf("[ProposalFollowUp] candidate scan failed: %v", err)
		return result
	}
	result.Scanned = len(deals)
	metrics.AddProposalFollowUpScanned(uint64(len(deals)))

	for _, d := range deals {
		if d == nil || d.TenantID == "" {
			continue
		}
		enqueuedForDeal := 0
		for i := range d.Proposals {
			p := &d.Proposals[i]
			stage := p.NextFollowUpStage(now, e.sched)
			if stage == 0 {
				result.Skipped++
				continue
			}
			awaiting := p.AwaitingFor()
			if awaiting == "" {
				result.Skipped++
				continue
			}
			if err := e.publish(ctx, d, p, stage, awaiting, now); err != nil {
				result.Errors++
				log.Printf("[ProposalFollowUp] publish failed deal=%s proposal=%s stage=%d: %v", d.ID, p.ID, stage, err)
				continue
			}
			// Marca DEPOIS de publicar: se o processo cair no meio, o próximo
			// tick republica o MESMO event_id (chave estável) e o dedup do
			// consumidor garante um único e-mail por grupo.
			nextFollowUps := p.FollowUps + 1
			if err := e.repo.MarkProposalFollowUp(ctx, d.TenantID, d.ID, p.ID, nextFollowUps, now); err != nil {
				// O evento já está no outbox; sem o mark o próximo tick
				// republica o mesmo event_id, que é inofensivo (dedup).
				result.Errors++
				log.Printf("[ProposalFollowUp] mark failed deal=%s proposal=%s: %v", d.ID, p.ID, err)
				continue
			}
			p.MarkFollowUpSent(now)
			enqueuedForDeal++
		}
		if enqueuedForDeal == 0 {
			continue
		}
		result.Enqueued += enqueuedForDeal
		metrics.AddProposalFollowUpEnqueued(uint64(enqueuedForDeal))
	}
	return result
}

func (e *ProposalFollowUpEngine) publish(ctx context.Context, d *deal.Deal, p *deal.Proposal, stage int, awaiting deal.ProposalAwaitingFor, now time.Time) error {
	var counter *float64
	if p.CounterValue != nil && *p.CounterValue > 0 {
		v := *p.CounterValue
		counter = &v
	}
	evt := contracts.ProposalFollowUpDueEvent{
		// Chave estável do GRUPO: proposta + estágio. É o que impede o
		// lembrete de repetir a cada varredura.
		EventIDVal: events.DeriveNotificationID(p.ID, "proposal_followup", strconv.Itoa(stage)),
		Data: contracts.ProposalFollowUpDueEventPayload{
			DealID:             d.ID,
			TenantID:           d.TenantID,
			ProposalID:         p.ID,
			Stage:              stage,
			MaxStages:          e.sched.MaxStages,
			DaysPending:        p.DaysPending(now),
			ProposalStatus:     string(p.Status),
			AwaitingFor:        string(awaiting),
			Value:              p.Value,
			CounterValue:       counter,
			BrokerID:           d.BrokerID,
			CustomerAccountIDs: SignerAccountIDs(d),
			CustomerEmails:     CustomerEmailsWithoutAccount(d),
			ProposalCreatedAt:  p.CreatedAt.UTC().Format(time.RFC3339),
		},
	}
	// O tenant vai no contexto: o INSERT no outbox usa o pool de serviço e
	// resolve o tenant_id por lá (EnqueueOutbox -> tenantIDForOutbox).
	tenantCtx := pkgctx.WithTenantID(ctx, d.TenantID)
	return e.bus.PublishReliable(tenantCtx, evt)
}

// RunLoop agenda a varredura em background. Retorna imediatamente; o loop roda
// até o ctx ser cancelado (shutdown gracioso). A primeira varredura tem um
// atraso curto para não competir com o boot dos outros workers.
func (e *ProposalFollowUpEngine) RunLoop(ctx context.Context, interval time.Duration) {
	if !e.Enabled() {
		log.Println("[ProposalFollowUp] engine disabled (max stages = 0)")
		return
	}
	if interval <= 0 {
		interval = time.Hour
	}
	go func() {
		first := time.NewTimer(3 * time.Minute)
		defer first.Stop()
		select {
		case <-ctx.Done():
			return
		case <-first.C:
		}

		e.logTick(ctx)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				e.logTick(ctx)
			}
		}
	}()
}

func (e *ProposalFollowUpEngine) logTick(ctx context.Context) {
	result, err := e.Tick(ctx)
	if err != nil {
		log.Printf("[ProposalFollowUp] tick failed: %v", err)
		return
	}
	// Loga TODO tick, inclusive scanned=0: um sweep que "não acha nada" é
	// indistinguível de um sweep quebrado (foi assim que a varredura rodando
	// pelo pool de aplicação, filtrada por RLS, passou despercebida).
	log.Printf("[ProposalFollowUp] tick done: scanned=%d enqueued=%d skipped=%d errors=%d",
		result.Scanned, result.Enqueued, result.Skipped, result.Errors)
}

// runWithLock espelha o padrão do toca-financial/comm: protege o tick com
// pg_try_advisory_lock quando o db está wired; sem lock (outra réplica
// processando) o tick é pulado silenciosamente.
func (e *ProposalFollowUpEngine) runWithLock(ctx context.Context, fn func(context.Context)) {
	if e.db == nil || !e.db.CanHoldSessionLockDuringWork() {
		fn(ctx)
		return
	}
	lockRes, releaser, err := e.db.TryAdvisoryLock(ctx, proposalFollowUpLockKey)
	if err != nil {
		log.Printf("[ProposalFollowUp] advisory lock error; skipping tick: %v", err)
		return
	}
	defer func() {
		if relErr := releaser.Release(ctx); relErr != nil {
			log.Printf("[ProposalFollowUp] advisory unlock error: %v", relErr)
		}
	}()
	if !lockRes.Acquired {
		log.Printf("[ProposalFollowUp] tick skipped — another instance holds the lock (key=%s)", lockRes.Key)
		return
	}
	fn(ctx)
}
