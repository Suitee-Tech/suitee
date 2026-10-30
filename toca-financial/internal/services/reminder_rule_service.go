package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/oginoino/toca/pkg/audit"
	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/collection"
	"github.com/oginoino/toca/toca-financial/internal/domain/reminder_rule"
)

type ReminderRuleService struct {
	repo       reminder_rule.Repository
	chargeRepo charge.RepositoryInterface
	chargeSvc  reminderRuleNotifier
	// collectionRepo is optional. When wired, the engine pauses reminders
	// for charges whose Collection is in a non-active dunning status —
	// NEGOTIATED (cliente aceitou parcelamento), JUDICIAL (cobrança via
	// jurídico) or GUARANTEE_ACTIVATED (seguro/garantia assumiu). Issue
	// #446 — sem este guard, a régua continua disparando depois de o
	// cliente fechar acordo, o que gera atrito no relacionamento.
	collectionRepo collection.RepositoryInterface
	// Scheduler state. The engine ticks once per day by default; sleeping
	// 24h is fine because reminders are calendar-aligned, not minute-aligned.
	interval time.Duration
	stopCh   chan struct{}
	mu       sync.Mutex
	running  bool
	// db habilita o lock distribuído do tick (mesmo mecanismo do motor de
	// inadimplência). Sem ele, duas réplicas do toca-financial rodavam a régua
	// ao mesmo tempo: o dedup diário é read-modify-write em `RemindersSent`,
	// sem CAS, então a mesma cobrança podia receber dois avisos no mesmo dia.
	db *database.PostgresDB
}

// WithDB wires the pool used to serialize the daily tick across replicas.
// Chainable; without it the engine keeps the legacy single-instance behavior.
func (s *ReminderRuleService) WithDB(db *database.PostgresDB) *ReminderRuleService {
	s.db = db
	return s
}

// reminderRuleNotifier is the narrow surface of ChargeService we need at
// evaluation time. Defining it locally avoids a hard dependency on the
// concrete service type in tests. The engine uses NotifyWithOptions so it
// can pass the rule's template + SentBy attribution.
type reminderRuleNotifier interface {
	NotifyWithOptions(ctx context.Context, tenantID, chargeID string, opts NotifyOptions) (*NotifyResult, error)
}

// reminderLockKey identifica o lock da régua de comunicação. Chave distinta da
// do motor de inadimplência: são trabalhos independentes e um não pode bloquear
// o outro.
const reminderLockKey = "toca-financial.reminder.tick"

func NewReminderRuleService(repo reminder_rule.Repository, chargeRepo charge.RepositoryInterface) *ReminderRuleService {
	return &ReminderRuleService{
		repo:       repo,
		chargeRepo: chargeRepo,
		interval:   24 * time.Hour,
		stopCh:     make(chan struct{}),
	}
}

// SetInterval overrides the tick cadence. Mainly used by tests and the
// optional REMINDER_ENGINE_INTERVAL env-var read in main.go.
func (s *ReminderRuleService) SetInterval(d time.Duration) {
	if d > 0 {
		s.interval = d
	}
}

// Start kicks off the daily evaluation loop. Idempotent: calling twice is
// a no-op. The first tick runs immediately so a freshly-deployed service
// doesn't wait 24h for the first dispatch.
func (s *ReminderRuleService) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	go func() {
		log.Println("[Reminder Engine] started")
		s.tick()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.tick()
			case <-s.stopCh:
				log.Println("[Reminder Engine] stopped")
				return
			}
		}
	}()
}

func (s *ReminderRuleService) tick() {
	ctx := context.Background()

	// Lock distribuído: só uma réplica dispara a régua do dia. O mesmo desenho
	// do motor de inadimplência (chave própria, liberada no fim do tick).
	if s.db != nil && s.db.CanHoldSessionLockDuringWork() {
		lockRes, releaser, err := s.db.TryAdvisoryLock(ctx, reminderLockKey)
		if err != nil {
			log.Printf("[Reminder Engine] advisory lock error: %v", err)
			return
		}
		defer func() {
			if relErr := releaser.Release(ctx); relErr != nil {
				log.Printf("[Reminder Engine] advisory unlock error: %v", relErr)
			}
		}()
		if !lockRes.Acquired {
			// Outra réplica está rodando a régua agora: sai em silêncio, o dia
			// já está sendo coberto.
			return
		}
	} else if s.db != nil {
		log.Printf("[Reminder Engine] advisory lock bypassed — database pool has fewer than 2 connections")
	}

	results := s.EvaluateAndDispatchAll(ctx, time.Now())
	totalSent, totalSkipped, totalPaused, totalErrors := 0, 0, 0, 0
	for _, r := range results {
		totalSent += r.NotifiesSent
		totalSkipped += r.NotifySkipped
		totalPaused += r.NotifyPausedByCollection
		totalErrors += len(r.Errors)
	}
	// paused_collection is tracked separately from skipped (intraday
	// dedup) so ops can spot trends — a spike in paused means many
	// novas negociações fechadas; a spike in skipped suggests duplicate
	// runs of the daily tick.
	log.Printf("[Reminder Engine] tick complete: tenants=%d sent=%d skipped=%d paused_collection=%d errors=%d", len(results), totalSent, totalSkipped, totalPaused, totalErrors)
}

// Stop signals the ticker to exit. Safe to call multiple times.
func (s *ReminderRuleService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.running = false
	close(s.stopCh)
	s.stopCh = make(chan struct{})
}

// SetChargeService wires the ChargeService used to dispatch reminders.
// Kept as a setter to avoid an init-time cycle (ChargeService → ChargeRepo
// → ReminderRuleService → ChargeService).
func (s *ReminderRuleService) SetChargeService(c reminderRuleNotifier) { s.chargeSvc = c }

// SetCollectionRepo wires the optional Collection repository so the engine
// can pause reminders for charges that already moved to NEGOTIATED /
// JUDICIAL / GUARANTEE_ACTIVATED. Optional: when nil, the engine evaluates
// every open charge as before (no regression for tenants without the
// delinquency engine wired). Kept as a setter to mirror SetChargeService
// and keep the constructor stable across services.
func (s *ReminderRuleService) SetCollectionRepo(r collection.RepositoryInterface) {
	s.collectionRepo = r
}

// --------------------------- CRUD ----------------------------------------

type CreateRuleInput struct {
	Name        string
	Description string
	TriggerType reminder_rule.TriggerType
	OffsetDays  int
	Channels    []reminder_rule.Channel
	Template    string
	Enabled     *bool
}

func (s *ReminderRuleService) Create(ctx context.Context, tenantID string, in CreateRuleInput) (*reminder_rule.ReminderRule, error) {
	r, err := reminder_rule.New(tenantID, in.Name, in.TriggerType, in.OffsetDays, in.Channels, in.Template)
	if err != nil {
		return nil, err
	}
	r.Description = strings.TrimSpace(in.Description)
	if in.Enabled != nil {
		r.Enabled = *in.Enabled
	}
	if err := s.repo.Create(ctx, r); err != nil {
		return nil, err
	}
	return r, nil
}

type UpdateRuleInput struct {
	Name        *string
	Description *string
	TriggerType *reminder_rule.TriggerType
	OffsetDays  *int
	Channels    []reminder_rule.Channel // optional; len==0 means leave untouched
	Template    *string
	Enabled     *bool
}

func (s *ReminderRuleService) Update(ctx context.Context, tenantID, id string, in UpdateRuleInput) (*reminder_rule.ReminderRule, error) {
	r, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, errors.New("reminder rule not found")
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
		r.Name = strings.TrimSpace(*in.Name)
	}
	if in.Description != nil {
		r.Description = strings.TrimSpace(*in.Description)
	}
	if in.TriggerType != nil {
		if err := reminder_rule.ValidateTrigger(*in.TriggerType); err != nil {
			return nil, err
		}
		r.TriggerType = *in.TriggerType
	}
	if in.OffsetDays != nil {
		if *in.OffsetDays < 0 {
			return nil, errors.New("offset_days cannot be negative")
		}
		r.OffsetDays = *in.OffsetDays
	}
	if len(in.Channels) > 0 {
		seen := make(map[reminder_rule.Channel]bool)
		out := make([]reminder_rule.Channel, 0, len(in.Channels))
		for _, c := range in.Channels {
			if err := reminder_rule.ValidateChannel(c); err != nil {
				return nil, err
			}
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
		r.Channels = out
	}
	if in.Template != nil && strings.TrimSpace(*in.Template) != "" {
		r.Template = *in.Template
	}
	if in.Enabled != nil {
		r.Enabled = *in.Enabled
	}
	// Cross-field validation after mutations.
	if r.TriggerType == reminder_rule.TriggerOnDue && r.OffsetDays != 0 {
		return nil, errors.New("offset_days must be 0 when trigger_type is ON_DUE")
	}
	if (r.TriggerType == reminder_rule.TriggerBeforeDue || r.TriggerType == reminder_rule.TriggerAfterDue) && r.OffsetDays == 0 {
		return nil, errors.New("offset_days must be >= 1 when trigger_type is BEFORE_DUE or AFTER_DUE")
	}
	if err := s.repo.Update(ctx, r); err != nil {
		return nil, err
	}
	return r, nil
}

func (s *ReminderRuleService) ListByTenant(ctx context.Context, tenantID string) ([]*reminder_rule.ReminderRule, error) {
	return s.repo.ListByTenant(ctx, tenantID)
}

func (s *ReminderRuleService) GetByID(ctx context.Context, tenantID, id string) (*reminder_rule.ReminderRule, error) {
	return s.repo.GetByID(ctx, tenantID, id)
}

func (s *ReminderRuleService) Delete(ctx context.Context, tenantID, id string) error {
	return s.repo.Delete(ctx, tenantID, id)
}

// --------------------------- Evaluation ----------------------------------

// EvaluationResult is what the engine returns after one tenant's pass.
//
// NotifyPausedByCollection counts charges that matched a rule but were
// suppressed because the parent Collection is in a non-active dunning
// status (NEGOTIATED / JUDICIAL / GUARANTEE_ACTIVATED). Surfaced as a
// distinct counter from NotifySkipped (which counts intraday dedup) so
// ops can tell apart "already sent today" from "in active negotiation"
// in the daily summary log.
type EvaluationResult struct {
	TenantID                 string   `json:"tenant_id"`
	RulesEvaluated           int      `json:"rules_evaluated"`
	ChargesMatched           int      `json:"charges_matched"`
	NotifiesSent             int      `json:"notifies_sent"`
	NotifySkipped            int      `json:"notify_skipped"`
	NotifyPausedByCollection int      `json:"notify_paused_by_collection"`
	Errors                   []string `json:"errors,omitempty"`
}

// EvaluateAndDispatch runs the rules for `tenantID` against `referenceDay`
// (typically the wall clock day at the moment the scheduler ticks). For
// each enabled rule it lists charges matching the rule's trigger date and
// fires a Notify via the wired ChargeService.
//
// We rely on the ChargeService's reminder audit (RemindersSent) to avoid
// emitting duplicates: callers can inspect the lastReminderAt on each
// charge and skip if it's the same day as today's evaluation. For v1 we
// keep this loose — the comm layer will eventually dedupe by content
// hash — but we DO de-dup at evaluation time to keep the daily cron idempotent
// within the same day.
func (s *ReminderRuleService) EvaluateAndDispatch(ctx context.Context, tenantID string, referenceDay time.Time) (*EvaluationResult, error) {
	if s.chargeSvc == nil {
		return nil, errors.New("charge service not wired into reminder engine")
	}
	rules, err := s.repo.ListEnabled(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list enabled rules: %w", err)
	}
	result := &EvaluationResult{TenantID: tenantID, RulesEvaluated: len(rules)}

	if len(rules) == 0 {
		return result, nil
	}

	// Pull all non-paid, non-cancelled charges for the tenant. For very
	// large tenants this should switch to a per-rule date-range query, but
	// here we paginate via repo.Search if available; falling back to List
	// (1000 cap) covers typical sizes.
	openCharges, err := s.listOpenCharges(ctx, tenantID)
	if err != nil {
		return result, fmt.Errorf("list open charges: %w", err)
	}

	refDay := toDay(referenceDay)
	// Cache one lookup per charge — the same charge can match multiple
	// rules in the same tick (ex.: "3 dias antes" e "no dia" coincidem em
	// alguns calendários) and we don't want to hammer the collection repo.
	// The cache stores both Status e CollectionID — Status guia o
	// short-circuit do pause; CollectionID alimenta o audit per-pause
	// (issue #446 follow-up: audit das pausas exige rastreabilidade da
	// collection que causou o bloqueio).
	type collectionCacheEntry struct {
		status       collection.Status
		collectionID string
	}
	collectionStatusCache := make(map[string]collectionCacheEntry, len(openCharges))
	// pausedTrace acumula info dos pares (charge,collection) suprimidos
	// para emissão de UM único audit.Record agregado por tick (em vez de
	// N entries por charge × rule). Mantém o audit acionável sem
	// poluir audit.events com ruído.
	type pauseSample struct {
		chargeID     string
		collectionID string
		status       collection.Status
	}
	var pausedTrace []pauseSample
	pausedChargeSeen := make(map[string]bool) // dedup quando duas rules casam

	for _, rule := range rules {
		for _, c := range openCharges {
			if !rule.MatchesDueDate(refDay, c.DueDate) {
				continue
			}
			result.ChargesMatched++
			// Idempotency: skip if a reminder already went out today for this
			// charge regardless of which rule triggered it. Keeps the daily
			// tick safe to re-run.
			if alreadyRemindedOn(c, refDay) {
				result.NotifySkipped++
				continue
			}
			// Issue #446 — pause régua automática when the parent
			// Collection already moved past ACTIVE. The delinquency
			// service flips status to NEGOTIATED after the operador
			// fecha um acordo de parcelamento with the cliente; from
			// that point on continuing to send "estamos no aguardo do
			// pagamento" generates atrito and contradicts the deal.
			// JUDICIAL and GUARANTEE_ACTIVATED are also non-active —
			// the lawyer / seguradora is now the cobrança channel, so
			// disparar a régua automática estaria fora do contrato.
			if s.collectionRepo != nil {
				entry, ok := collectionStatusCache[c.ID]
				if !ok {
					col, err := s.collectionRepo.FindByChargeID(ctx, tenantID, c.ID)
					if err != nil {
						// Fail-open: a transient repo error shouldn't
						// silence the cobrança régua entirely — log and
						// proceed with the historic behaviour. Recorded
						// as an error so it surfaces in the per-tenant
						// summary.
						result.Errors = append(result.Errors, fmt.Sprintf("collection lookup for charge %s: %s", c.ID, err.Error()))
					} else if col != nil {
						entry.status = col.Status
						entry.collectionID = col.ID
					}
					collectionStatusCache[c.ID] = entry
				}
				if isDunningPausedByCollection(entry.status) {
					result.NotifyPausedByCollection++
					if !pausedChargeSeen[c.ID] {
						pausedChargeSeen[c.ID] = true
						pausedTrace = append(pausedTrace, pauseSample{
							chargeID:     c.ID,
							collectionID: entry.collectionID,
							status:       entry.status,
						})
					}
					continue
				}
			}
			channels := make([]string, 0, len(rule.Channels))
			for _, ch := range rule.Channels {
				channels = append(channels, string(ch))
			}
			opts := NotifyOptions{
				Template:    rule.Template,
				SentBy:      "rule:" + rule.ID,
				SentByLabel: "Régua: " + rule.Name,
				Channels:    channels,
			}
			if _, err := s.chargeSvc.NotifyWithOptions(ctx, tenantID, c.ID, opts); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("charge %s: %s", c.ID, err.Error()))
				continue
			}
			result.NotifiesSent++
		}
	}

	// Issue #446 — audit aggregate por tick. Em vez de N audit entries
	// (uma por charge × rule pausada), emite UMA entry sumarizando o
	// que foi suprimido. Mantém audit.events acionável sem ruído: ops
	// vê "tenant X teve 12 pausas hoje em 3 collections" e drilla
	// quando precisa investigar. Só registra quando houve ao menos
	// uma pausa — silêncio nos dias normais.
	if len(pausedTrace) > 0 {
		// Dedup collection IDs e separa por status para a UI de
		// admin/audit ver o porquê da pausa sem decodificar IDs.
		collectionIDs := make([]string, 0)
		statusCount := make(map[string]int)
		seenCollection := make(map[string]bool)
		chargeIDs := make([]string, 0, len(pausedTrace))
		for _, p := range pausedTrace {
			chargeIDs = append(chargeIDs, p.chargeID)
			if p.collectionID != "" && !seenCollection[p.collectionID] {
				seenCollection[p.collectionID] = true
				collectionIDs = append(collectionIDs, p.collectionID)
			}
			statusCount[string(p.status)]++
		}
		audit.Record(ctx, audit.Entry{
			Action:     "reminder.tick.paused_by_collection",
			EntityType: "Tenant",
			EntityID:   tenantID,
			Metadata: map[string]any{
				"tenant_id":            tenantID,
				"reference_day":        refDay.Format("2006-01-02"),
				"paused_charge_count":  len(pausedTrace),
				"paused_collections":   collectionIDs,
				"paused_charge_ids":    chargeIDs,
				"by_collection_status": statusCount,
			},
		})
	}
	return result, nil
}

// isDunningPausedByCollection returns true when the Collection status means
// the cobrança automática régua should NOT fire. Kept as a free function so
// the same predicate can be reused by tests and by future engines (ex.: a
// WhatsApp dispatcher) without duplicating the enum list. RESOLVED is also
// included for defensive symmetry — a charge that's still PENDING but whose
// Collection is RESOLVED is almost certainly a stale state and a reminder
// would confuse the user; let the operador fix the data first.
func isDunningPausedByCollection(status collection.Status) bool {
	switch status {
	case collection.StatusNegotiated,
		collection.StatusJudicial,
		collection.StatusGuaranteeActivated,
		collection.StatusResolved:
		return true
	default:
		return false
	}
}

// EvaluateAndDispatchAll iterates over every tenant with enabled rules.
// Wired by the daily scheduler in main.go.
func (s *ReminderRuleService) EvaluateAndDispatchAll(ctx context.Context, referenceDay time.Time) []*EvaluationResult {
	tenants, err := s.repo.ListDistinctTenantIDs(ctx)
	if err != nil {
		log.Printf("[Reminder Engine] failed to list tenants: %v", err)
		return nil
	}
	out := make([]*EvaluationResult, 0, len(tenants))
	for _, tid := range tenants {
		// O contexto ganha o tenant em cada iteração. A régua lê regras,
		// cobranças e cobranças-ativas e grava o lembrete pelo pool de
		// aplicação, que roda SOB RLS: sem as claims o motor voltava
		// "tenants=0"/zero cobranças e nunca disparava nada (o tick do beta
		// logava "tenants=0" com regra ativa no banco).
		tenantCtx := pkgctx.WithTenantID(ctx, tid)
		res, err := s.EvaluateAndDispatch(tenantCtx, tid, referenceDay)
		if err != nil {
			log.Printf("[Reminder Engine] tenant %s failed: %v", tid, err)
			continue
		}
		out = append(out, res)
	}
	return out
}

// --------------------------- Helpers -------------------------------------

func (s *ReminderRuleService) listOpenCharges(ctx context.Context, tenantID string) ([]*charge.Charge, error) {
	// "" status returns all; we then retain only outstanding lifecycle
	// states. In particular, REVERSED is terminal and must never receive a
	// new payment reminder. For larger
	// tenants the repo could expose a "pending+overdue" status filter.
	items, _, err := s.chargeRepo.List(ctx, tenantID, "", "", "", 1000, 0)
	if err != nil {
		return nil, err
	}
	open := make([]*charge.Charge, 0, len(items))
	for _, c := range items {
		if c == nil || !charge.IsOutstandingStatus(c.Status) {
			continue
		}
		open = append(open, c)
	}
	return open, nil
}

func alreadyRemindedOn(c *charge.Charge, day time.Time) bool {
	target := toDay(day)
	for _, r := range c.RemindersSent {
		if toDay(r.SentAt).Equal(target) {
			return true
		}
	}
	return false
}

func toDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
