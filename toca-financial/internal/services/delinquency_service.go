package services

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/clients"
	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/pkg/metrics"
	"github.com/oginoino/toca/pkg/timeutil"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/collection"
)

// DelinquencyConfig holds configuration for the delinquency engine
type DelinquencyConfig struct {
	// LateFeePercent is the flat late fee percentage (Brazilian default: 2%)
	LateFeePercent float64
	// MonthlyInterestPercent is the monthly interest rate (Brazilian default: 1%)
	MonthlyInterestPercent float64
	// AutoCreateCollectionAfterDays creates a collection case after N days overdue
	AutoCreateCollectionAfterDays int
	// ProcessInterval is how often the engine runs
	ProcessInterval time.Duration
}

// DefaultDelinquencyConfig returns Brazilian standard defaults
func DefaultDelinquencyConfig() DelinquencyConfig {
	return DelinquencyConfig{
		LateFeePercent:                2.0,
		MonthlyInterestPercent:        1.0,
		AutoCreateCollectionAfterDays: 5,
		ProcessInterval:               1 * time.Hour,
	}
}

// DelinquencyService is the core delinquency management engine
type DelinquencyService struct {
	// agreementFulfiller marca o acordo como cumprido quando as parcelas fecham
	// (Fase 2). Injetado como função para não acoplar o motor ao serviço de
	// cobrança — o motor só precisa avisar "este caso mudou".
	agreementFulfiller func(ctx context.Context, tenantID, collectionID string)
	// transferHoldReleaser libera a retenção de repasse por inadimplência quando o
	// caso é resolvido pela quitação das cobranças (mesmo padrão do fulfiller).
	transferHoldReleaser func(ctx context.Context, tenantID, contractID string)
	// limiarCache guarda o limiar efetivo de abertura de caso por tenant dentro
	// de uma janela curta: o tick percorre muitas cobranças do mesmo tenant e não
	// faz sentido reler a configuração a cada uma.
	limiarMu       sync.Mutex
	limiarCache    map[string]limiarEntry
	chargeRepo     charge.RepositoryInterface
	collectionRepo collection.RepositoryInterface
	eventBus       events.EventBus
	config         DelinquencyConfig
	dealClient     contractRateLookup
	// tenantSettings é optional. Quando wired, rateForCharge insere
	// "tenant defaults" no meio da fallback chain: contrato → tenant →
	// global. Sem ele, mantém o comportamento legado (contrato → global).
	tenantSettings *TenantFinancialSettingsService
	db             *database.PostgresDB // opcional: habilita distributed lock no tick (#445)

	stopCh  chan struct{}
	mu      sync.Mutex
	running bool
}

// contractRateLookup is the narrow surface of the deal client we depend on.
// Defining it as a local interface lets tests stub it without dragging the
// full clients.DealClient into the test scope.
type contractRateLookup interface {
	GetContract(ctx context.Context, tenantID, contractID string) (*clients.ContractDTO, error)
}

// SetDealClient wires an optional deal client that lets the delinquency engine
// pick up per-contract multa/juros rates. Nil-safe: callers that don't want
// the lookup can simply skip this call and the global config keeps applying.
// WithAgreementFulfiller injeta o reconciliador do acordo (Fase 2): o motor
// avisa quando as parcelas fecham e o serviço de cobrança marca o acordo como
// cumprido. Encadeável e opcional.
func (s *DelinquencyService) WithAgreementFulfiller(fn func(ctx context.Context, tenantID, collectionID string)) *DelinquencyService {
	s.agreementFulfiller = fn
	return s
}

// WithTransferHoldReleaser injeta quem libera a retenção de repasse do contrato
// quando o caso deixa de estar aberto.
func (s *DelinquencyService) WithTransferHoldReleaser(fn func(ctx context.Context, tenantID, contractID string)) *DelinquencyService {
	s.transferHoldReleaser = fn
	return s
}

func (s *DelinquencyService) SetDealClient(dc contractRateLookup) {
	s.dealClient = dc
}

// SetTenantSettings wires o tenantFinancialSettingsService — quando o
// contrato não traz overrides, rateForCharge consulta o tenant antes de
// cair no default global. Sem isso, contratos sem override caem direto
// para o config global (comportamento legado mantido).
func (s *DelinquencyService) SetTenantSettings(t *TenantFinancialSettingsService) {
	s.tenantSettings = t
}

// WithDB wires the Postgres pool used by Start() para coordenar
// múltiplas instâncias via pg_try_advisory_lock (#445). Sem isso, em
// um deploy com 2+ pods, cada tick processaria os mesmos charges e
// duplicaria fees + collections. Quando db é nil o lock é skipado
// (modo legacy single-instance — útil para dev/test).
func (s *DelinquencyService) WithDB(db *database.PostgresDB) *DelinquencyService {
	s.db = db
	return s
}

// rateForCharge returns the late-fee and monthly-interest percentages that
// should be used for `c`. Fallback chain (highest precedence first):
//
//  1. Per-contract override (contract.LateFeePct / .MonthlyInterestPct)
//     resolved via the deal client.
//  2. Per-tenant default (tenantFinancialSettings) — opcional.
//  3. Global service config (DelinquencyConfig).
//
// Nil dependencies mean that layer is intentionally unavailable and the next
// configured level applies. A lookup error is different: silently applying a
// fallback would persist the wrong monetary fee, so callers must skip/abort.
func (s *DelinquencyService) rateForCharge(ctx context.Context, c *charge.Charge) (float64, float64, error) {
	lateFee := s.config.LateFeePercent
	interest := s.config.MonthlyInterestPercent

	// Tenant-level default first — sobrescreve global, mas perde para
	// per-contract abaixo.
	if s.tenantSettings != nil && c != nil && c.TenantID != "" {
		ts, err := s.tenantSettings.Get(ctx, c.TenantID)
		if err != nil {
			return 0, 0, fmt.Errorf("load tenant delinquency settings: %w", err)
		}
		if ts != nil {
			// Zero is an explicit and valid tenant policy (fee waiver), not a
			// signal to fall back to the global 2%/1% defaults.
			lateFee = ts.DefaultLateFeePct
			interest = ts.DefaultMonthlyInterestPct
		}
	}

	// Per-contract — vence se trouxer override.
	if s.dealClient == nil || c == nil || c.ContractID == "" {
		return lateFee, interest, nil
	}
	contract, err := s.dealClient.GetContract(ctx, c.TenantID, c.ContractID)
	if err != nil {
		return 0, 0, fmt.Errorf("load contract delinquency settings: %w", err)
	}
	if contract == nil {
		return lateFee, interest, nil
	}
	if contract.LateFeePct > 0 {
		lateFee = contract.LateFeePct
	}
	if contract.MonthlyInterestPct > 0 {
		interest = contract.MonthlyInterestPct
	}
	return lateFee, interest, nil
}

// DelinquencyDashboard contains all dashboard data for the frontend
type DelinquencyDashboard struct {
	Stats           *charge.DelinquencyStats    `json:"stats"`
	AgingReport     []charge.AgingBucket        `json:"aging_report"`
	CollectionStats *collection.CollectionStats `json:"collection_stats"`
	// Config é a configuração efetiva do motor para o tenant (limiar de abertura
	// de caso, intervalo do tick e taxas). Fica aqui, e não numa rota própria,
	// porque o dashboard já é a chamada que a tela faz.
	Config *DelinquencyConfigView `json:"config,omitempty"`
}

// ProcessResult contains the result of a delinquency processing run
type ProcessResult struct {
	ChargesMarkedOverdue int `json:"charges_marked_overdue"`
	// ChargesReverted conta cobranças cuja marcação de atraso foi desfeita por
	// não estarem vencidas na regra atual (vence hoje não é atraso).
	ChargesReverted    int       `json:"charges_reverted"`
	LateFessCalculated int       `json:"late_fees_calculated"`
	CollectionsCreated int       `json:"collections_created"`
	CollectionsUpdated int       `json:"collections_updated"`
	ProcessedAt        time.Time `json:"processed_at"`
}

// ChargeOverdueEvent was promoted to pkg/events/contracts.ChargeOverdueEvent
// as part of FASE 4 (integração total). The alias preserves any existing
// callers that import this package symbol while the canonical definition
// lives in the shared contracts package — same wire format.
type ChargeOverdueEvent = contracts.ChargeOverdueEvent

func NewDelinquencyService(
	chargeRepo charge.RepositoryInterface,
	collectionRepo collection.RepositoryInterface,
	eventBus events.EventBus,
	config DelinquencyConfig,
) *DelinquencyService {
	return &DelinquencyService{
		chargeRepo:     chargeRepo,
		collectionRepo: collectionRepo,
		eventBus:       eventBus,
		config:         config,
		stopCh:         make(chan struct{}),
	}
}

// delinquencyLockKey é a chave do pg_try_advisory_lock que coordena
// múltiplas instâncias do toca-financial executando o tick da
// régua de inadimplência simultaneamente (#445). FNV-1a hash do
// string é estável entre processos e ambientes.
const delinquencyLockKey = "toca-financial.delinquency.tick"

// runWithLock executa fn protegido por advisory lock quando s.db está
// wired. Sem db → run direto (modo legacy). Quando outro pod já tem
// o lock, retorna no-op sem erro (não há nada a fazer).
//
// O lock é session-scoped (preso na conn enquanto Releaser não fechar),
// então mesmo que fn demore minutos o outro pod continua bloqueado.
// Ao terminar, libera para o próximo tick (que pode ser deste pod ou
// outro — quem chegar primeiro processa).
func (s *DelinquencyService) runWithLock(ctx context.Context, label string, fn func(context.Context) (*ProcessResult, error)) (*ProcessResult, error, bool) {
	if s.db == nil || !s.db.CanHoldSessionLockDuringWork() {
		// Single-instance / one-connection mode: a session advisory lock would
		// reserve the only pool slot and ProcessOverdueCharges would wait on
		// itself forever. Run without the distributed lock; deployments with
		// multiple replicas must configure DATABASE_MAX_POOL_SIZE >= 2.
		if s.db != nil {
			log.Printf("[Delinquency Engine] %s: advisory lock bypassed — database pool has fewer than 2 connections", label)
		}
		res, err := fn(ctx)
		return res, err, true
	}

	lockRes, releaser, err := s.db.TryAdvisoryLock(ctx, delinquencyLockKey)
	if err != nil {
		// Lock indisponível por erro de infra — log e degrada para
		// "pula este tick" (a próxima iteração tenta de novo). Melhor
		// under-process do que processar concorrentemente sem garantia.
		log.Printf("[Delinquency Engine] %s: advisory lock error: %v", label, err)
		return nil, err, false
	}
	defer func() {
		if relErr := releaser.Release(ctx); relErr != nil {
			log.Printf("[Delinquency Engine] %s: advisory unlock error: %v", label, relErr)
		}
	}()

	if !lockRes.Acquired {
		log.Printf("[Delinquency Engine] %s: skipped — another instance holds the lock (key=%s)",
			label, lockRes.Key)
		return nil, nil, false
	}

	res, err := fn(ctx)
	return res, err, true
}

// Start begins the background delinquency processing loop
func (s *DelinquencyService) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	go func() {
		log.Println("[Delinquency Engine] Started")
		// Run immediately on start
		result, err, ran := s.runTick("initial", s.ProcessOverdueCharges)
		switch {
		case err != nil:
			log.Printf("[Delinquency Engine] Initial processing error: %v", err)
		case !ran:
			// Outra instância pegou o lock primeiro — esperado em multi-pod.
		default:
			// `reverted` entra no log de propósito: uma execução que só DESFEZ
			// marcações indevidas aparecia como "0 overdue, 0 fees" e parecia que
			// o motor não havia feito nada — foi o que confundiu a validação em
			// produção quando a regra de vencimento mudou.
			log.Printf("[Delinquency Engine] Initial run: %d overdue, %d fees, %d collections created, %d reverted",
				result.ChargesMarkedOverdue, result.LateFessCalculated, result.CollectionsCreated, result.ChargesReverted)
		}

		ticker := time.NewTicker(s.config.ProcessInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				result, err, ran := s.runTick("tick", s.ProcessOverdueCharges)
				switch {
				case err != nil:
					log.Printf("[Delinquency Engine] Processing error: %v", err)
				case !ran:
					// Skipped (lock contention). Próximo tick tenta de novo.
				default:
					log.Printf("[Delinquency Engine] Processed: %d overdue, %d fees, %d collections, %d reverted",
						result.ChargesMarkedOverdue, result.LateFessCalculated, result.CollectionsCreated, result.ChargesReverted)
				}
			case <-s.stopCh:
				log.Println("[Delinquency Engine] Stopped")
				return
			}
		}
	}()
}

// runTick executa uma passagem do motor medindo duração e resultado.
//
// O tick NÃO gera linha de auditoria própria de propósito: ele roda de hora em
// hora e um registro por execução encheria audit.events com 24 linhas vazias por
// dia. O que importa é rastreado onde acontece — `charge.overdue_processed` por
// cobrança, `collection.auto_created`/`collection.charge_attached` por caso.
// A vitalidade do tick é observada pelas métricas (financial_delinquency_tick_*).
//
// A métrica conta UMA vez por execução que realmente rodou (quem pegou o
// advisory lock) — tick pulado por contenção não conta, senão o alerta de
// "motor parado" nunca dispararia num deploy multi-pod. O tick inicial do boot
// conta igual aos demais.
func (s *DelinquencyService) runTick(label string, fn func(context.Context) (*ProcessResult, error)) (*ProcessResult, error, bool) {
	inicio := time.Now()
	result, err, ran := s.runWithLock(context.Background(), label, fn)
	if !ran {
		return result, err, false
	}
	resultado := "success"
	if err != nil {
		resultado = "error"
	}
	metrics.ObserveDelinquencyTick(resultado, time.Since(inicio).Seconds())
	return result, err, true
}

// Stop stops the background processing loop
func (s *DelinquencyService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		close(s.stopCh)
		s.running = false
	}
}

// limiarEntry é o limiar efetivo de abertura de caso, com a origem do valor.
type limiarEntry struct {
	dias   int
	origem string
	em     time.Time
}

// limiarTTL é a validade do cache do limiar.
const limiarTTL = 60 * time.Second

// AutoCaseDaysForTenant resolve QUANTOS dias de atraso abrem o caso de cobrança
// para o tenant, e de onde veio o número:
//
//	origem "tenant" → política da própria imobiliária
//	origem "global" → padrão da plataforma
//
// Antes esse limiar era constante de código (5): quem cobra com 3 dias ou só quer
// o caso depois de 10 não tinha como ajustar, e a tela repetia o número (espelhado
// no frontend, sujeito a mentir se o padrão mudasse).
func (s *DelinquencyService) AutoCaseDaysForTenant(ctx context.Context, tenantID string) (int, string) {
	s.limiarMu.Lock()
	if s.limiarCache == nil {
		s.limiarCache = make(map[string]limiarEntry)
	}
	if e, ok := s.limiarCache[tenantID]; ok && time.Since(e.em) < limiarTTL {
		s.limiarMu.Unlock()
		return e.dias, e.origem
	}
	s.limiarMu.Unlock()

	dias, origem := s.config.AutoCreateCollectionAfterDays, "global"
	if s.tenantSettings != nil && strings.TrimSpace(tenantID) != "" {
		if ts, err := s.tenantSettings.Get(ctx, tenantID); err == nil && ts != nil && ts.DelinquencyAutoCaseAfterDays > 0 {
			dias, origem = ts.DelinquencyAutoCaseAfterDays, "tenant"
		}
	}

	s.limiarMu.Lock()
	s.limiarCache[tenantID] = limiarEntry{dias: dias, origem: origem, em: time.Now()}
	s.limiarMu.Unlock()
	return dias, origem
}

// ProcessOverdueCharges is the main processing function that:
// 1. Finds all pending charges past due
// 2. Marks them as OVERDUE
// 3. Calculates late fees and interest
// 4. Creates or updates collection cases
func (s *DelinquencyService) ProcessOverdueCharges(ctx context.Context) (*ProcessResult, error) {
	result := &ProcessResult{ProcessedAt: time.Now()}

	// 1. Get all tenants with pending past-due charges
	tenantIDs, err := s.chargeRepo.ListDistinctTenantIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list distinct tenant IDs: %w", err)
	}

	if len(tenantIDs) == 0 {
		return result, nil
	}

	// Collect charges from all tenants. O contexto ganha o tenant em cada
	// iteração: a listagem sai pelo pool de aplicação e `financial.charges` é
	// deny-by-default para ele, então sem as claims o motor lia zero cobranças
	// em TODO tenant (o tick logava "0 overdue" com a base cheia). A varredura
	// cross-tenant acima usa o pool de serviço; daqui para baixo o trabalho é
	// sempre de um tenant.
	//
	// A descoberta é por ESTADO (`ListOverdue`: vencida e ainda não fechada,
	// incluindo as que já estão OVERDUE), não por transição. Com a descoberta
	// por transição, uma cobrança marcada OVERDUE com menos de 24h de atraso
	// saía da listagem e nunca mais recebia multa/juros — o atraso congelava em
	// zero até alguém rodar o processamento manual.
	var pendingCharges []*charge.Charge
	for _, tid := range tenantIDs {
		tenantCtx := pkgctx.WithTenantID(ctx, tid)
		charges, err := s.chargeRepo.ListOverdue(tenantCtx, tid)
		if err != nil {
			log.Printf("[Delinquency Engine] Failed to list charges for tenant %s: %v", tid, err)
			continue
		}
		// Retrato do vencido no início do tick (só gauge, por tenant): é a
		// métrica que alimenta o alerta de saldo vencido. Antes do diagnóstico
		// não existia NENHUMA métrica de inadimplência.
		var saldoVencido float64
		for _, c := range charges {
			if c != nil && charge.IsOutstandingStatus(c.Status) {
				saldoVencido += remainingChargeDebt(c)
			}
		}
		metrics.SetOverdueSnapshot(tid, saldoVencido, len(charges))
		s.publishCollectionSnapshot(tenantCtx, tid)
		pendingCharges = append(pendingCharges, charges...)
	}

	if len(pendingCharges) == 0 {
		return result, nil
	}

	// 2. Process each charge
	for _, c := range pendingCharges {
		if c == nil {
			continue
		}
		// Mesmo motivo acima: leituras/escritas da cobrança saem pelo pool de
		// aplicação e precisam das claims do tenant dela.
		chargeCtx := pkgctx.WithTenantID(ctx, c.TenantID)
		// Resolve every authoritative monetary input before mutating the
		// aggregate, even in memory. A transient lookup failure must leave the
		// charge untouched for a clean retry on the next tick.
		lateFee, interest, err := s.rateForCharge(chargeCtx, c)
		if err != nil {
			log.Printf("[Delinquency Engine] Skipping charge %s: %v", c.ID, err)
			continue
		}
		expectedVersion := c.UpdatedAt
		markedOverdue := c.Status == charge.StatusPending
		// Mark as overdue
		if err := c.MarkOverdue(); err != nil {
			// A stale/malformed repository result must not reopen a terminal
			// charge or emit a second overdue event.
			log.Printf("[Delinquency Engine] Skipping charge %s in status %s: %v", c.ID, c.Status, err)
			continue
		}

		if s.revertIfNotYetOverdue(chargeCtx, c, expectedVersion, result) {
			continue
		}

		c.CalculateLateFees(lateFee, interest)

		// Update charge with new status and fees
		persisted, err := updateChargeIfCurrent(chargeCtx, s.chargeRepo, c, expectedVersion)
		if err != nil {
			log.Printf("[Delinquency Engine] Failed to update charge %s: %v", c.ID, err)
			continue
		}
		if !persisted {
			log.Printf("[Delinquency Engine] Skipping concurrently modified charge %s", c.ID)
			continue
		}
		if markedOverdue {
			result.ChargesMarkedOverdue++
		}
		result.LateFessCalculated++

		// Auditoria + evento de atraso (compartilhado com o processamento manual).
		s.recordChargeOverdue(chargeCtx, c, markedOverdue)

		// 3. Auto-create or update collection case if past threshold
		// (política do tenant quando houver; padrão da plataforma caso contrário)
		limiar, _ := s.AutoCaseDaysForTenant(chargeCtx, c.TenantID)
		if c.DaysOverdue() >= limiar {
			if err := s.ensureCollectionCase(chargeCtx, c, result); err != nil {
				log.Printf("[Delinquency Engine] Failed to ensure collection for charge %s: %v", c.ID, err)
			}
		}
	}

	return result, nil
}

// revertIfNotYetOverdue desfaz a marcação de atraso quando a regra de vencimento
// do produto diz que a cobrança AINDA NÃO venceu (vence no dia, atrasa no dia
// seguinte), e devolve true quando reverteu.
//
// Por que existe: as cobranças marcadas como OVERDUE pela regra antiga (que
// comparava instantes) continuariam aparecendo como vencidas mesmo depois da
// correção — foi o caso real em produção, uma conta que vencia no dia e já estava
// com multa aplicada. Aqui a própria regra desfaz o que ela não sustenta mais.
func (s *DelinquencyService) revertIfNotYetOverdue(ctx context.Context, c *charge.Charge, expectedVersion time.Time, result *ProcessResult) bool {
	if c == nil || c.Status != charge.StatusOverdue {
		return false
	}
	loc := timeutil.LocationForTenant(ctx, c.TenantID)
	if c.OverdueDays(time.Now(), loc) != 0 {
		return false
	}
	if err := c.RevertOverdue(); err != nil {
		log.Printf("[Delinquency Engine] Failed to revert overdue charge %s: %v", c.ID, err)
		return true
	}
	persisted, err := updateChargeIfCurrent(ctx, s.chargeRepo, c, expectedVersion)
	if err != nil || !persisted {
		log.Printf("[Delinquency Engine] Skipping revert of charge %s (persisted=%v err=%v)", c.ID, persisted, err)
		return true
	}
	result.ChargesReverted++
	s.recordChargeReverted(ctx, c)
	return true
}

// recordChargeReverted registra na trilha que a marcação de atraso foi desfeita
// (a cobrança não estava vencida na regra de dia civil). É uma mudança de valor
// cobrado do devedor: precisa de rastro.
func (s *DelinquencyService) recordChargeReverted(ctx context.Context, c *charge.Charge) {
	if c == nil {
		return
	}
	audit.Record(ctx, audit.Entry{
		Action:     "charge.overdue_reverted",
		EntityType: "Charge",
		EntityID:   c.ID,
		After: map[string]any{
			"status":       string(c.Status),
			"late_fee":     c.LateFee.Float(),
			"interest":     c.Interest.Float(),
			"total_amount": c.TotalAmount.Float(),
			"reason":       "cobrança vence no dia; atraso só no dia seguinte",
		},
		Metadata: map[string]any{"contract_id": c.ContractID, "tenant_id": c.TenantID},
	})
}

// recordChargeOverdue registra o que o motor fez com a cobrança: auditoria em
// todo processamento (o motor era silencioso — não havia rastro de quem marcou
// atraso nem de quantas vezes o encargo foi recalculado) e evento apenas na
// TRANSIÇÃO para vencido.
//
// Evento só na transição por dois motivos: (a) o contrato `charge.overdue` diz
// "fires the moment a Charge transitions from PENDING to OVERDUE"; (b) o motor
// roda de hora em hora e republicar a cada tick geraria aviso diário repetido
// ao inquilino. O `mandatory=true` do publisher faria cada repetição falhar por
// não ter rota quando não há consumidor — ruído no log e perda silenciosa.
//
// Cobrança de locação sai como `rental.charge_overdue` (payload rico: lead do
// inquilino, lead do proprietário, imóvel), que é o contrato que o toca-comm já
// consome. Os demais tipos saem como `charge.overdue`.
func (s *DelinquencyService) recordChargeOverdue(ctx context.Context, c *charge.Charge, transicao bool) {
	if c == nil {
		return
	}

	audit.Record(ctx, audit.Entry{
		Action:     "charge.overdue_processed",
		EntityType: "Charge",
		EntityID:   c.ID,
		After: map[string]any{
			"status":         string(c.Status),
			"days_overdue":   c.DaysOverdue(),
			"late_fee":       c.LateFee.Float(),
			"interest":       c.Interest.Float(),
			"total_amount":   c.TotalAmount.Float(),
			"marked_overdue": transicao,
		},
		Metadata: map[string]any{"contract_id": c.ContractID, "tenant_id": c.TenantID},
	})

	if !transicao || s.eventBus == nil {
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if c.Type == charge.TypeRent {
		event := contracts.RentalChargeOverdueEvent{
			EventIDVal: events.DeriveNotificationID(c.ID, "rental.charge_overdue"),
			Data: contracts.RentalChargeOverdueEventPayload{
				ChargeID:       c.ID,
				TenantID:       c.TenantID,
				TenantLeadID:   c.TenantPartyID,
				OwnerLeadID:    c.OwnerID,
				Amount:         c.TotalAmount.Float(),
				DueDate:        c.DueDate.Format(time.RFC3339),
				DaysOverdue:    c.DaysOverdue(),
				PropertyID:     c.PropertyID,
				ContractID:     c.ContractID,
				ReferenceMonth: c.ReferenceMonth,
				Timestamp:      now,
			},
		}
		if err := s.eventBus.Publish(ctx, event); err != nil {
			log.Printf("[Delinquency Engine] Failed to publish rental overdue event for charge %s: %v", c.ID, err)
		}
		return
	}

	event := contracts.ChargeOverdueEvent{
		EventIDVal: events.DeriveNotificationID(c.ID, "charge.overdue"),
		Data: contracts.ChargeOverdueEventPayload{
			ChargeID:    c.ID,
			ContractID:  c.ContractID,
			TenantID:    c.TenantID,
			DebtorID:    c.TenantPartyID,
			Amount:      c.Amount.Float(),
			TotalAmount: c.TotalAmount.Float(),
			DaysOverdue: c.DaysOverdue(),
			DueDate:     c.DueDate,
			Timestamp:   now,
		},
	}
	if err := s.eventBus.Publish(ctx, event); err != nil {
		log.Printf("[Delinquency Engine] Failed to publish overdue event for charge %s: %v", c.ID, err)
	}
}

// ensureCollectionCase creates a new collection case or adds the charge to an existing one
func (s *DelinquencyService) ensureCollectionCase(ctx context.Context, c *charge.Charge, result *ProcessResult) error {
	// Check if charge is already in a collection
	existing, err := s.collectionRepo.FindByChargeID(ctx, c.TenantID, c.ID)
	if err != nil {
		return err
	}

	if existing != nil {
		if existing.Status == collection.StatusResolved {
			// RESOLVED é terminal: o tick não revive o caso. Mas inadimplência
			// RECORRENTE precisa de caso novo — antes o `return nil` deixava a
			// dívida nova invisível para sempre (G5 do plano): o inquilino
			// quitava, atrasava de novo e nenhum caso aparecia.
			//
			// A distinção é a participação da cobrança no caso encerrado: se ela
			// estava lá, é estado residual (nada a fazer); se é dívida nova, abre
			// caso novo.
			for _, id := range existing.ChargeIDs {
				if id == c.ID {
					return nil
				}
			}
			return s.createCollectionCase(ctx, c, result)
		}
		expectedUpdatedAt := existing.UpdatedAt
		// Update existing collection
		existing.AddCharge(c.ID)

		// Recalculate: fetch all charges in this collection
		charges, err := s.chargeRepo.ListByIDs(ctx, c.TenantID, existing.ChargeIDs)
		if err == nil {
			totalDebt := 0.0
			maxDays := 0
			for _, ch := range charges {
				if ch != nil && charge.IsOutstandingStatus(ch.Status) {
					totalDebt += remainingChargeDebt(ch)
					days := ch.DaysOverdue()
					if days > maxDays {
						maxDays = days
					}
				}
			}
			existing.RecalculateDebt(totalDebt, maxDays)
		}

		if err := updateCollectionIfCurrent(ctx, s.collectionRepo, existing, expectedUpdatedAt); err != nil {
			return err
		}
		s.auditCollection(ctx, "collection.charge_attached", existing, c, "charges")
		result.CollectionsUpdated++
		return nil
	}

	// Check if debtor already has an active collection for this contract
	activeByDebtor, err := s.collectionRepo.FindActiveByDebtor(ctx, c.TenantPartyID, c.TenantID)
	if err != nil {
		return err
	}

	if activeByDebtor != nil && activeByDebtor.ContractID == c.ContractID {
		expectedUpdatedAt := activeByDebtor.UpdatedAt
		// Add charge to existing debtor collection
		activeByDebtor.AddCharge(c.ID)
		activeByDebtor.TotalDebt += remainingChargeDebt(c)
		if c.DaysOverdue() > activeByDebtor.DaysOverdue {
			activeByDebtor.DaysOverdue = c.DaysOverdue()
		}
		if err := updateCollectionIfCurrent(ctx, s.collectionRepo, activeByDebtor, expectedUpdatedAt); err != nil {
			return err
		}
		s.auditCollection(ctx, "collection.charge_attached", activeByDebtor, c, "debtor")
		result.CollectionsUpdated++
		return nil
	}

	if err := s.createCollectionCase(ctx, c, result); err != nil {
		return err
	}
	return nil
}

// auditCollection registra na trilha o que o MOTOR fez com o caso.
//
// O motor era completamente silencioso: marcar atraso, recalcular encargos e
// agrupar dívida não deixavam rastro em audit.events (só o serviço de cobrança,
// nas ações do operador, auditava). Uma divergência de dívida não tinha como ser
// reconstruída depois.
func (s *DelinquencyService) auditCollection(ctx context.Context, action string, coll *collection.Collection, c *charge.Charge, origem string) {
	if coll == nil || c == nil {
		return
	}
	after := map[string]any{
		"collection_id": coll.ID,
		"charge_id":     c.ID,
		"total_debt":    coll.TotalDebt,
		"days_overdue":  coll.DaysOverdue,
		"charge_count":  len(coll.ChargeIDs),
		"status":        string(coll.Status),
	}
	if origem != "" {
		after["matched_by"] = origem
	}
	audit.Record(ctx, audit.Entry{
		Action:     action,
		EntityType: "Collection",
		EntityID:   coll.ID,
		After:      after,
		Metadata:   map[string]any{"contract_id": c.ContractID, "tenant_id": c.TenantID, "engine": "delinquency"},
	})
}

// createCollectionCase abre um caso novo para a cobrança informada.
//
// A criação usa chave de idempotência derivada de (contrato, cobrança): o motor
// pode reprocessar a mesma cobrança (retry, tick manual concorrente) e antes
// isso criava um caso duplicado — `collection.New` não preenchia
// `SourceEventID` e o `Create` é um INSERT simples, sem chave única por
// cobrança. Com a chave, o ON CONFLICT do repositório absorve a repetição.
func (s *DelinquencyService) createCollectionCase(ctx context.Context, c *charge.Charge, result *ProcessResult) error {
	coll := collection.New(
		c.ContractID,
		c.TenantPartyID,
		c.TenantPartyID, // debtor_name - using ID as fallback, real name would come from CRM
		c.TenantID,
		remainingChargeDebt(c),
		c.DaysOverdue(),
		[]string{c.ID},
	)
	coll.SourceEventID = events.DeriveNotificationID(c.ContractID, "auto-collection", c.ID)

	if err := s.collectionRepo.CreateIdempotent(ctx, coll); err != nil {
		return err
	}
	s.auditCollection(ctx, "collection.auto_created", coll, c, "")
	result.CollectionsCreated++

	// Publish collection created event
	if s.eventBus != nil {
		event := contracts.CollectionCreatedEvent{
			EventIDVal: events.DeriveNotificationID(coll.ID, "collection.created"),
			Data: contracts.CollectionCreatedEventPayload{
				CollectionID: coll.ID,
				ContractID:   c.ContractID,
				TenantID:     c.TenantID,
				DebtorID:     c.TenantPartyID,
				TotalDebt:    remainingChargeDebt(c),
				DaysOverdue:  c.DaysOverdue(),
				ChargeCount:  len(coll.ChargeIDs),
				Timestamp:    time.Now().UTC().Format(time.RFC3339),
			},
		}
		if err := s.eventBus.Publish(ctx, event); err != nil {
			log.Printf("[Delinquency Engine] Failed to publish collection event: %v", err)
		}
	}

	return nil
}

func remainingChargeDebt(c *charge.Charge) float64 {
	if c == nil || !charge.IsOutstandingStatus(c.Status) {
		return 0
	}
	remaining := c.TotalAmount.Float()
	if c.PaidAmount != nil {
		remaining -= c.PaidAmount.Float()
	}
	if remaining < 0 {
		return 0
	}
	return remaining
}

// ── Dashboard & Analytics ──

// DelinquencyConfigView é a configuração efetiva do motor para o tenant.
//
// `Source` diz de onde veio o limiar: "tenant" (política da imobiliária) ou
// "global" (padrão da plataforma) — a tela usa isso para explicar o número ao
// operador em vez de afirmar um valor fixo.
type DelinquencyConfigView struct {
	AutoCreateCollectionAfterDays int     `json:"auto_create_collection_after_days"`
	ProcessIntervalMinutes        int     `json:"process_interval_minutes"`
	LateFeePercent                float64 `json:"late_fee_percent"`
	MonthlyInterestPercent        float64 `json:"monthly_interest_percent"`
	Source                        string  `json:"source"`
}

// GetDashboard returns the complete delinquency dashboard for a tenant
func (s *DelinquencyService) GetDashboard(ctx context.Context, tenantID string) (*DelinquencyDashboard, error) {
	dashboard := &DelinquencyDashboard{}

	// Get charge delinquency stats
	stats, err := s.chargeRepo.GetDelinquencyStats(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get delinquency stats: %w", err)
	}
	dashboard.Stats = stats

	// Get aging report
	aging, err := s.chargeRepo.AgingReport(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get aging report: %w", err)
	}
	dashboard.AgingReport = aging

	// Get collection stats
	collStats, err := s.collectionRepo.GetStats(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get collection stats: %w", err)
	}
	dashboard.CollectionStats = collStats

	// Configuração efetiva do motor: a tela mostra o limiar real em vez de
	// repetir o número que estava escrito no frontend.
	limiarDias, limiarOrigem := s.AutoCaseDaysForTenant(ctx, tenantID)
	dashboard.Config = &DelinquencyConfigView{
		AutoCreateCollectionAfterDays: limiarDias,
		ProcessIntervalMinutes:        int(s.config.ProcessInterval.Minutes()),
		LateFeePercent:                s.config.LateFeePercent,
		MonthlyInterestPercent:        s.config.MonthlyInterestPercent,
		Source:                        limiarOrigem,
	}

	// Fill in active collections count
	//
	// O dashboard é o endpoint que a TELA consome, e contava só `collStats.Active`:
	// caso em negociação, garantia ou judicial sumia do indicador — o caso existia
	// na tabela, aparecia no aging e não aparecia em "casos ativos". A conta agora é
	// a mesma de GetDelinquencyStats: caso ativo é tudo que não está resolvido.
	if stats != nil && collStats != nil {
		stats.ActiveCollections = collStats.Active + collStats.Negotiated +
			collStats.GuaranteeActivated + collStats.Judicial
		stats.RecoveryRate = s.recoveryRate(ctx, tenantID, collStats.TotalDebt)
	}

	return dashboard, nil
}

// GetAgingReport returns the aging report for a tenant
func (s *DelinquencyService) GetAgingReport(ctx context.Context, tenantID string) ([]charge.AgingBucket, error) {
	return s.chargeRepo.AgingReport(ctx, tenantID)
}

// GetDelinquencyStats returns KPI stats for a tenant
func (s *DelinquencyService) GetDelinquencyStats(ctx context.Context, tenantID string) (*charge.DelinquencyStats, error) {
	stats, err := s.chargeRepo.GetDelinquencyStats(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	collStats, err := s.collectionRepo.GetStats(ctx, tenantID)
	if err == nil && collStats != nil {
		// "Casos ativos" = tudo que não está resolvido. Antes contava apenas
		// ACTIVE, então caso em negociação/garantia/judicial desaparecia do KPI
		// (G22 do plano).
		stats.ActiveCollections = collStats.Active + collStats.Negotiated +
			collStats.GuaranteeActivated + collStats.Judicial
		stats.RecoveryRate = s.recoveryRate(ctx, tenantID, collStats.TotalDebt)
	}

	return stats, nil
}

// recoveryRate devolve a fração recuperada dos casos encerrados.
//
// Numerador: soma do que foi efetivamente pago nas cobranças dos casos
// RESOLVED. Denominador: esse valor + a dívida ainda aberta. O campo existia na
// API e nunca era calculado (voltava sempre 0), então o indicador de recuperação
// simplesmente não existia na prática.
//
// Limitado aos 500 casos resolvidos mais recentes: é um indicador de gestão,
// não um extrato contábil, e uma imobiliária grande não pode paginar milhares de
// jsonb a cada abertura de tela.
func (s *DelinquencyService) recoveryRate(ctx context.Context, tenantID string, openDebt float64) float64 {
	resolvidos, _, err := s.collectionRepo.List(ctx, tenantID, string(collection.StatusResolved), 500, 0)
	if err != nil || len(resolvidos) == 0 {
		return 0
	}

	ids := make([]string, 0, len(resolvidos)*2)
	for _, c := range resolvidos {
		if c == nil {
			continue
		}
		ids = append(ids, c.ChargeIDs...)
	}
	if len(ids) == 0 {
		return 0
	}

	charges, err := s.chargeRepo.ListByIDs(ctx, tenantID, ids)
	if err != nil {
		return 0
	}
	var recuperado float64
	for _, c := range charges {
		if c != nil && c.PaidAmount != nil {
			recuperado += c.PaidAmount.Float()
		}
	}
	total := recuperado + openDebt
	if total <= 0 {
		return 0
	}
	return recuperado / total
}

// publishCollectionSnapshot publica os gauges de retrato dos casos do tenant
// (quantos em cada status e quanto de dívida os casos abertos representam).
//
// É best-effort: falha de leitura não pode derrubar o motor — o tick continua,
// só a métrica daquele tenant fica com o valor anterior.
func (s *DelinquencyService) publishCollectionSnapshot(ctx context.Context, tenantID string) {
	stats, err := s.collectionRepo.GetStats(ctx, tenantID)
	if err != nil {
		log.Printf("[Delinquency Engine] Collection stats unavailable for tenant %s: %v", tenantID, err)
		return
	}
	if stats == nil {
		return
	}
	metrics.SetCollectionStatusCounts(tenantID, map[string]float64{
		"active":              float64(stats.Active),
		"negotiated":          float64(stats.Negotiated),
		"guarantee_activated": float64(stats.GuaranteeActivated),
		"judicial":            float64(stats.Judicial),
		"resolved":            float64(stats.Resolved),
	})
	// A dívida "aberta" exclui os casos resolvidos: somar RESOLVED (que já está
	// com total_debt zerado no caso, mas pode carregar valor histórico em base
	// antiga) inflava o número que a diretoria olha.
	metrics.SetOpenCollectionDebtBRL(tenantID, stats.TotalDebt)
}

// ProcessManualTrigger allows a manual trigger of the delinquency engine for a specific tenant.
//
// Passa pelo MESMO lock distribuído do tick: sem isso, dois operadores clicando
// "Processar Inadimplência" ao mesmo tempo — ou um clique durante o tick —
// processavam as mesmas cobranças em paralelo e podiam abrir/atualizar o caso
// duas vezes (a criação não tem chave única por cobrança).
func (s *DelinquencyService) ProcessManualTrigger(ctx context.Context, tenantID string) (*ProcessResult, error) {
	result, err, ran := s.runWithLock(ctx, "manual trigger ("+tenantID+")", func(lockCtx context.Context) (*ProcessResult, error) {
		return s.processTenantOverdueCharges(lockCtx, tenantID)
	})
	if err != nil {
		metrics.IncDelinquencyManualRun("error")
		audit.Record(ctx, audit.Entry{
			Action:     "delinquency.manual_run",
			EntityType: "Tenant",
			EntityID:   tenantID,
			After:      map[string]any{"result": "error", "error": err.Error()},
			Metadata:   map[string]any{"engine": "delinquency"},
		})
		return nil, err
	}
	if !ran {
		// Outra instância está processando agora: devolve o resultado vazio em
		// vez de erro — o operador não tem culpa e o trabalho está em curso.
		return result, nil
	}
	metrics.IncDelinquencyManualRun("success")
	audit.Record(ctx, audit.Entry{
		Action:     "delinquency.manual_run",
		EntityType: "Tenant",
		EntityID:   tenantID,
		After: map[string]any{
			"result":               "success",
			"charges_marked":       result.ChargesMarkedOverdue,
			"late_fees_calculated": result.LateFessCalculated,
			"collections_created":  result.CollectionsCreated,
			"collections_updated":  result.CollectionsUpdated,
		},
		Metadata: map[string]any{"engine": "delinquency", "trigger": "manual"},
	})
	return result, nil
}

// processTenantOverdueCharges é o corpo do processamento manual (um tenant),
// separado para rodar dentro do lock.
func (s *DelinquencyService) processTenantOverdueCharges(ctx context.Context, tenantID string) (*ProcessResult, error) {
	result := &ProcessResult{ProcessedAt: time.Now()}

	// Get overdue charges for this tenant
	overdueCharges, err := s.chargeRepo.ListOverdue(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	for _, c := range overdueCharges {
		if c == nil || !charge.IsOutstandingStatus(c.Status) {
			continue
		}
		lateFee, interest, err := s.rateForCharge(ctx, c)
		if err != nil {
			return nil, fmt.Errorf("resolve delinquency rates for charge %s: %w", c.ID, err)
		}
		expectedVersion := c.UpdatedAt
		if s.revertIfNotYetOverdue(ctx, c, expectedVersion, result) {
			continue
		}

		markedOverdue := false
		if c.Status == charge.StatusPending {
			if err := c.MarkOverdue(); err != nil {
				continue
			}
			markedOverdue = true
		}

		c.CalculateLateFees(lateFee, interest)

		persisted, err := updateChargeIfCurrent(ctx, s.chargeRepo, c, expectedVersion)
		if err != nil {
			log.Printf("[Delinquency Engine] Failed to update charge %s: %v", c.ID, err)
			continue
		}
		if !persisted {
			log.Printf("[Delinquency Engine] Skipping concurrently modified charge %s", c.ID)
			continue
		}
		// O manual também audita e publica: antes ele era um caminho silencioso
		// (sem audit e sem evento), então cobrança marcada como vencida "por
		// botão" não deixava rastro nem avisava ninguém.
		s.recordChargeOverdue(ctx, c, markedOverdue)
		if markedOverdue {
			result.ChargesMarkedOverdue++
		}
		result.LateFessCalculated++

		limiar, _ := s.AutoCaseDaysForTenant(ctx, c.TenantID)
		if c.DaysOverdue() >= limiar {
			if err := s.ensureCollectionCase(ctx, c, result); err != nil {
				log.Printf("[Delinquency Engine] Failed to ensure collection for charge %s: %v", c.ID, err)
			}
		}
	}

	return result, nil
}

// ResolveChargePayment handles when a charge gets paid, potentially resolving collection cases
func (s *DelinquencyService) ResolveChargePayment(ctx context.Context, tenantID, chargeID string) error {
	// Find collection containing this charge
	coll, err := s.collectionRepo.FindByChargeID(ctx, tenantID, chargeID)
	if err != nil || coll == nil {
		return err // No collection for this charge
	}
	if coll.Status == collection.StatusResolved && coll.TotalDebt == 0 {
		return nil
	}
	expectedUpdatedAt := coll.UpdatedAt

	// Check if all charges in collection are now paid
	charges, err := s.chargeRepo.ListByIDs(ctx, tenantID, coll.ChargeIDs)
	if err != nil {
		return err
	}

	allPaid := true
	totalDebt := 0.0
	maxDays := 0
	for _, c := range charges {
		if c != nil && charge.IsOutstandingStatus(c.Status) {
			allPaid = false
			remaining := c.TotalAmount.Float()
			if c.PaidAmount != nil {
				remaining -= c.PaidAmount.Float()
				if remaining < 0 {
					remaining = 0
				}
			}
			totalDebt += remaining
			days := c.DaysOverdue()
			if days > maxDays {
				maxDays = days
			}
		} else if c != nil && !charge.IsClosedStatus(c.Status) {
			// Unknown lifecycle values must not auto-resolve a collection.
			allPaid = false
		}
	}

	if allPaid {
		coll.Status = collection.StatusResolved
		coll.TotalDebt = 0
		action := collection.Action{
			Date:        time.Now(),
			Type:        "auto_resolved",
			Description: "Todas as cobranças foram pagas. Caso resolvido automaticamente.",
			UserID:      "system",
		}
		coll.Actions = append(coll.Actions, action)
	} else {
		coll.RecalculateDebt(totalDebt, maxDays)
	}

	if err := updateCollectionIfCurrent(ctx, s.collectionRepo, coll, expectedUpdatedAt); err != nil {
		return err
	}

	// Caso resolvido (todas as cobranças fechadas): libera a retenção dos repasses
	// do contrato, se houver — o dinheiro do proprietário não pode continuar preso
	// depois que a dívida que motivou a retenção acabou.
	if coll.Status == collection.StatusResolved && s.transferHoldReleaser != nil {
		s.transferHoldReleaser(ctx, tenantID, coll.ContractID)
	}

	// Acordo cumprido: quando todas as parcelas fecham, o acordo vira FULFILLED.
	// Fica aqui porque este é o caminho por onde TODO pagamento passa — e só o
	// cumprimento é automático (quebrar acordo tem efeito financeiro e é decisão
	// do operador).
	if s.agreementFulfiller != nil {
		s.agreementFulfiller(ctx, tenantID, coll.ID)
	}
	return nil
}
