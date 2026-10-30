package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/audit"
	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/metrics"
	"github.com/oginoino/toca/toca-crm/internal/domain/broker"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

type LeadRouletteService struct {
	leadRepo     lead.Repository
	rouletteRepo lead.RouletteRepository
	historyRepo  lead.RouletteHistoryRepository
	brokerRepo   broker.Repository
	eventBus     events.EventBus
	auditRepo    lead.RouletteAuditRepository
}

func (s *LeadRouletteService) publishAsync(ctx context.Context, evt events.Event) {
	if s.eventBus == nil || evt == nil {
		return
	}
	go func() {
		// Contexto de FUNDO, nunca o da request: o handler HTTP retorna e
		// cancela o ctx — o filho derivado morria com "context canceled" e o
		// insert no outbox falhava (evento lead.assigned nunca publicado;
		// corrida intermitente com o handler). Mesmo padrão do
		// LeadService.publishAsync.
		ctx2, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.eventBus.PublishReliable(ctx2, evt); err != nil {
			logger.Warn("roulette.publish failed",
				"event", evt.Name(),
				"event_id", evt.EventID(),
				"error", err.Error(),
			)
		}
	}()
}

func NewLeadRouletteService(
	lRepo lead.Repository,
	rRepo lead.RouletteRepository,
	hRepo lead.RouletteHistoryRepository,
	bRepo broker.Repository,
	eBus events.EventBus,
	aRepo lead.RouletteAuditRepository,
) *LeadRouletteService {
	return &LeadRouletteService{
		leadRepo:     lRepo,
		rouletteRepo: rRepo,
		historyRepo:  hRepo,
		brokerRepo:   bRepo,
		eventBus:     eBus,
		auditRepo:    aRepo,
	}
}

func (s *LeadRouletteService) GetRoulette(ctx context.Context, tenantID string) (*lead.RouletteConfig, error) {
	return s.rouletteRepo.GetConfig(ctx, tenantID)
}

// ConfigWarnings inspeciona a config junto com os brokers reais e devolve
// avisos para configurações tecnicamente válidas mas sem efeito útil. Custa
// uma chamada GetByIDs no broker repo; se ela falha, retornamos os warnings
// "rasos" (sem inspeção de Weight/IsPriority) para não bloquear o GET.
// Wave P1.4 — handler GET /leads/roulette anexa o resultado ao DTO.
func (s *LeadRouletteService) ConfigWarnings(ctx context.Context, tenantID string, cfg *lead.RouletteConfig) []string {
	if cfg == nil {
		return nil
	}
	if s.brokerRepo == nil {
		return lead.ConfigWarnings(cfg, nil)
	}
	ids := uniqueBrokerIDs(cfg)
	if len(ids) == 0 {
		return lead.ConfigWarnings(cfg, nil)
	}
	lookup, err := s.brokerRepo.GetByIDs(ctx, tenantID, ids)
	if err != nil {
		// Falha de broker repo não deve quebrar GET — devolve warnings rasos.
		return lead.ConfigWarnings(cfg, nil)
	}
	return lead.ConfigWarnings(cfg, lookup)
}

func uniqueBrokerIDs(cfg *lead.RouletteConfig) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(cfg.ActiveBrokers))
	for _, id := range cfg.ActiveBrokers {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, pool := range cfg.StageBrokerPools {
		for _, id := range pool {
			if id != "" && !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out
}

type RouletteActor struct {
	ID    string
	Email string
}

func normalizeStageKey(k string) string {
	return strings.ToUpper(strings.TrimSpace(k))
}

func normalizeStageBrokerPools(in map[string][]string) map[string][]string {
	if in == nil {
		return nil
	}
	out := make(map[string][]string, len(in))
	for k, v := range in {
		nk := normalizeStageKey(k)
		out[nk] = v
	}
	return out
}

func normalizeStageLastIndex(in map[string]int) map[string]int {
	if in == nil {
		return nil
	}
	out := make(map[string]int, len(in))
	for k, v := range in {
		nk := normalizeStageKey(k)
		// Se colidir após normalização, preserva o índice "mais avançado"
		if cur, ok := out[nk]; ok && cur > v {
			continue
		}
		out[nk] = v
	}
	return out
}

func (s *LeadRouletteService) UpdateConfig(ctx context.Context, tenantID string, req lead.UpdateRouletteConfigDTO, actor RouletteActor) (*lead.RouletteConfig, error) {
	cfg, err := s.rouletteRepo.GetConfig(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	var before lead.RouletteConfig
	{
		b, _ := json.Marshal(cfg)
		_ = json.Unmarshal(b, &before)
	}

	assignUnassigned := req.AssignUnassignedLeads != nil && *req.AssignUnassignedLeads

	if req.Enabled != nil {
		cfg.Enabled = *req.Enabled
	}
	if req.ActiveBrokers != nil {
		cfg.ActiveBrokers = req.ActiveBrokers
	}
	if req.StageBrokerPools != nil {
		cfg.StageBrokerPools = normalizeStageBrokerPools(req.StageBrokerPools)
	}
	if req.DistMethod != nil {
		cfg.DistMethod = *req.DistMethod
	}
	if req.RespectSpecialty != nil {
		cfg.RespectSpecialty = *req.RespectSpecialty
	}
	if req.RespectRegion != nil {
		cfg.RespectRegion = *req.RespectRegion
	}
	if req.RespectSchedule != nil {
		cfg.RespectSchedule = *req.RespectSchedule
	}
	if req.AutoCapLimit != nil {
		cfg.AutoCapLimit = *req.AutoCapLimit
	}
	if req.SLATiers != nil {
		cfg.SLATiers = req.SLATiers
	}
	if req.Escalation != nil {
		cfg.Escalation = req.Escalation
	}
	if req.MaxEscalations != nil {
		cfg.MaxEscalations = *req.MaxEscalations
	}
	if req.NotifChannels != nil {
		cfg.NotifChannels = req.NotifChannels
	}
	if req.QualFields != nil {
		cfg.QualFields = req.QualFields
	}
	if req.QualRouting != nil {
		cfg.QualRouting = req.QualRouting
	}
	if req.GlobalBusinessHours != nil {
		cfg.GlobalBusinessHours = req.GlobalBusinessHours
	}
	if req.SimulationSeed != nil {
		cfg.SimulationSeed = *req.SimulationSeed
	}

	// Garante consistência de chaves no storage (evita "NEW" vs "new" quebrar pool/ponteiro).
	cfg.StageBrokerPools = normalizeStageBrokerPools(cfg.StageBrokerPools)
	cfg.StageLastIndex = normalizeStageLastIndex(cfg.StageLastIndex)

	// Garante entrada inicial (-1 = ainda não usada) para cada stage com pool configurada.
	// Isso deixa o jsonb pré-populado, evitando ambiguidade no CAS do ponteiro e permitindo
	// que a primeira atribuição em cada stage encontre a chave aninhada já existente.
	if cfg.StageLastIndex == nil {
		cfg.StageLastIndex = map[string]int{}
	}
	for stageKey := range cfg.StageBrokerPools {
		if _, ok := cfg.StageLastIndex[stageKey]; !ok {
			cfg.StageLastIndex[stageKey] = -1
		}
	}

	if actor.Email != "" {
		cfg.UpdatedBy = actor.Email
	} else if actor.ID != "" {
		cfg.UpdatedBy = actor.ID
	}

	if err := s.rouletteRepo.UpdateConfig(ctx, cfg); err != nil {
		return nil, err
	}

	if s.auditRepo != nil && actor.ID != "" {
		diff := map[string]interface{}{}
		if req.Enabled != nil && before.Enabled != cfg.Enabled {
			diff["enabled"] = map[string]interface{}{"before": before.Enabled, "after": cfg.Enabled}
		}
		if req.ActiveBrokers != nil && !reflect.DeepEqual(before.ActiveBrokers, cfg.ActiveBrokers) {
			diff["active_brokers"] = map[string]interface{}{"before": before.ActiveBrokers, "after": cfg.ActiveBrokers}
		}
		if req.StageBrokerPools != nil && !reflect.DeepEqual(before.StageBrokerPools, cfg.StageBrokerPools) {
			diff["stage_broker_pools"] = map[string]interface{}{"before": before.StageBrokerPools, "after": cfg.StageBrokerPools}
		}
		if req.DistMethod != nil && before.DistMethod != cfg.DistMethod {
			diff["dist_method"] = map[string]interface{}{"before": before.DistMethod, "after": cfg.DistMethod}
		}
		if req.RespectSpecialty != nil && before.RespectSpecialty != cfg.RespectSpecialty {
			diff["respect_specialty"] = map[string]interface{}{"before": before.RespectSpecialty, "after": cfg.RespectSpecialty}
		}
		if req.RespectRegion != nil && before.RespectRegion != cfg.RespectRegion {
			diff["respect_region"] = map[string]interface{}{"before": before.RespectRegion, "after": cfg.RespectRegion}
		}
		if req.RespectSchedule != nil && before.RespectSchedule != cfg.RespectSchedule {
			diff["respect_schedule"] = map[string]interface{}{"before": before.RespectSchedule, "after": cfg.RespectSchedule}
		}
		if req.AutoCapLimit != nil && before.AutoCapLimit != cfg.AutoCapLimit {
			diff["auto_cap_limit"] = map[string]interface{}{"before": before.AutoCapLimit, "after": cfg.AutoCapLimit}
		}
		if req.SLATiers != nil && !reflect.DeepEqual(before.SLATiers, cfg.SLATiers) {
			diff["sla_tiers"] = map[string]interface{}{"before": before.SLATiers, "after": cfg.SLATiers}
		}
		if req.Escalation != nil && !reflect.DeepEqual(before.Escalation, cfg.Escalation) {
			diff["escalation"] = map[string]interface{}{"before": before.Escalation, "after": cfg.Escalation}
		}
		if req.MaxEscalations != nil && before.MaxEscalations != cfg.MaxEscalations {
			diff["max_escalations"] = map[string]interface{}{"before": before.MaxEscalations, "after": cfg.MaxEscalations}
		}
		if req.NotifChannels != nil && !reflect.DeepEqual(before.NotifChannels, cfg.NotifChannels) {
			diff["notif_channels"] = map[string]interface{}{"before": before.NotifChannels, "after": cfg.NotifChannels}
		}
		if req.QualFields != nil && !reflect.DeepEqual(before.QualFields, cfg.QualFields) {
			diff["qual_fields"] = map[string]interface{}{"before": before.QualFields, "after": cfg.QualFields}
		}
		if req.QualRouting != nil && !reflect.DeepEqual(before.QualRouting, cfg.QualRouting) {
			diff["qual_routing"] = map[string]interface{}{"before": before.QualRouting, "after": cfg.QualRouting}
		}
		if req.GlobalBusinessHours != nil && !reflect.DeepEqual(before.GlobalBusinessHours, cfg.GlobalBusinessHours) {
			diff["global_business_hours"] = map[string]interface{}{"before": before.GlobalBusinessHours, "after": cfg.GlobalBusinessHours}
		}
		if req.SimulationSeed != nil && before.SimulationSeed != cfg.SimulationSeed {
			diff["simulation_seed"] = map[string]interface{}{"before": before.SimulationSeed, "after": cfg.SimulationSeed}
		}
		if len(diff) > 0 {
			_ = s.auditRepo.Create(ctx, &lead.RouletteAuditEntry{
				ID:         uuid.New().String(),
				TenantID:   tenantID,
				ActorID:    actor.ID,
				ActorEmail: actor.Email,
				Action:     "config.update",
				TargetID:   tenantID,
				Diff:       diff,
				CreatedAt:  time.Now(),
			})
			// Wave P1.6 — notifica outros admins/abas via outbox event para
			// que o FE invalide a query de config e mostre toast de "alterado
			// por outra pessoa". Só publica se realmente houve mudança (diff
			// não-vazio) para evitar tráfego de no-op.
			changedFields := make([]string, 0, len(diff))
			for k := range diff {
				changedFields = append(changedFields, k)
			}
			sort.Strings(changedFields)
			s.publishAsync(ctx, RouletteConfigUpdatedEvent{
				TenantID:      tenantID,
				ActorID:       actor.ID,
				ActorEmail:    actor.Email,
				ChangedFields: changedFields,
				UpdatedAt:     cfg.UpdatedAt,
			})

			// Global audit (#431/#467 — J03/J10 BDD audit trail).
			// O auditRepo acima é domain-specific (lead.RouletteAuditEntry,
			// powered o /roulette/audit). Esta entry vai para o audit.events
			// global, alimentando o filtro entity_type=roulette_config no
			// /audit-trail (cenário principal J10 etapa 5).
			audit.Record(ctx, audit.Entry{
				Action:     "roulette_config.updated",
				EntityType: "roulette_config",
				EntityID:   tenantID,
				Before:     audit.Snapshot(before),
				After:      audit.Snapshot(cfg),
				Metadata: map[string]any{
					"actor_email":    actor.Email,
					"changed_fields": changedFields,
					"diff":           diff,
				},
			})
		}
	}

	if cfg.Enabled && assignUnassigned {
		go func() {
			bg := pkgctx.WithTenantID(context.Background(), tenantID)
			_, _ = s.AssignUnassignedLeads(bg, tenantID, 1000)
		}()
	}

	return cfg, nil
}

func (s *LeadRouletteService) HasUnassignedLeads(ctx context.Context, tenantID string) (bool, error) {
	count, err := s.leadRepo.CountUnassigned(ctx, tenantID)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *LeadRouletteService) CountUnassignedLeads(ctx context.Context, tenantID string) (int64, error) {
	return s.leadRepo.CountUnassigned(ctx, tenantID)
}

func (s *LeadRouletteService) AssignUnassignedLeads(ctx context.Context, tenantID string, max int) (int, error) {
	if max <= 0 {
		max = 1000
	}

	assigned := 0
	batchSize := int64(200)

	for assigned < max {
		batch, err := s.leadRepo.ListUnassigned(ctx, tenantID, batchSize)
		if err != nil {
			return assigned, err
		}
		if len(batch) == 0 {
			break
		}

		assignedInBatch := 0
		for _, l := range batch {
			if assigned >= max {
				break
			}
			ok, err := s.AssignByRoulette(ctx, l, "")
			if err != nil {
				return assigned, err
			}
			if ok {
				assigned++
				assignedInBatch++
			}
		}

		if assignedInBatch == 0 {
			break
		}
	}

	return assigned, nil
}

func poolForStage(cfg *lead.RouletteConfig, stage string) []string {
	if cfg == nil {
		return nil
	}
	stage = strings.ToUpper(strings.TrimSpace(stage))
	if stage != "" && cfg.StageBrokerPools != nil {
		if p, ok := cfg.StageBrokerPools[stage]; ok && len(p) > 0 {
			return p
		}
	}
	return cfg.ActiveBrokers
}

func currentIndex(cfg *lead.RouletteConfig, stage string) int {
	if cfg == nil {
		return -1
	}
	stage = strings.ToUpper(strings.TrimSpace(stage))
	if stage != "" && cfg.StageLastIndex != nil {
		if v, ok := cfg.StageLastIndex[stage]; ok {
			return v
		}
	}
	return cfg.LastIndex
}

// pickBroker applies the 4 algorithms and 4 filters
func (s *LeadRouletteService) pickBroker(ctx context.Context, tenantID string, cfg *lead.RouletteConfig, l *lead.Lead, excludeBrokerID string) (string, int, lead.DistMethod, error) {
	return s.pickBrokerWithRng(ctx, tenantID, cfg, l, excludeBrokerID, nil)
}

func (s *LeadRouletteService) pickBrokerWithRng(ctx context.Context, tenantID string, cfg *lead.RouletteConfig, l *lead.Lead, excludeBrokerID string, rng *rand.Rand) (string, int, lead.DistMethod, error) {
	if cfg == nil || !cfg.Enabled {
		return "", -1, "", nil
	}

	stage := strings.ToUpper(string(l.Status))
	pool := poolForStage(cfg, stage)
	if len(pool) == 0 {
		return "", -1, cfg.DistMethod, nil
	}

	brokerMap, err := s.brokerRepo.GetByIDs(ctx, tenantID, pool)
	if err != nil {
		return "", -1, cfg.DistMethod, err
	}
	loads, err := s.leadRepo.GetActiveLeadCountsByBroker(ctx, tenantID, pool)
	if err != nil {
		return "", -1, cfg.DistMethod, err
	}

	respectSpecialty := cfg.RespectSpecialty
	respectRegion := cfg.RespectRegion
	respectSchedule := cfg.RespectSchedule

	leadTags := l.DeriveTags()
	if cfg.QualRouting != nil && len(cfg.QualRouting.SpecialtyFieldIDs) > 0 {
		leadTags = append(leadTags, extractQualificationValues(l.Qualification, cfg.QualRouting.SpecialtyFieldIDs)...)
	}

	leadLocation := ""
	if cfg.QualRouting != nil && cfg.QualRouting.RegionFieldID != nil && *cfg.QualRouting.RegionFieldID != "" {
		leadLocation = extractQualificationLocation(l.Qualification, *cfg.QualRouting.RegionFieldID)
	}
	if leadLocation == "" && l.PropertyInterest != nil {
		leadLocation = l.PropertyInterest.PreferredLocation
	}

	eligible := func(brokerID string) bool {
		if brokerID == "" {
			return false
		}
		if excludeBrokerID != "" && brokerID == excludeBrokerID {
			return false
		}
		b, ok := brokerMap[brokerID]
		if !ok || b == nil {
			return false
		}

		// Wave P1.7 — IsCurrentlyPaused honra PausedUntil para auto-unpause
		// sem job dedicado: brokers cuja pausa expirou voltam a entrar na
		// elegibilidade na próxima atribuição.
		if !b.Active || b.IsCurrentlyPaused(time.Now()) {
			return false
		}

		if cfg.AutoCapLimit && b.MaxActiveLeads > 0 {
			if loads[brokerID] >= b.MaxActiveLeads {
				return false
			}
		}

		if respectSpecialty {
			if len(b.Specialties) > 0 && !hasIntersection(b.Specialties, leadTags) {
				return false
			}
		}

		if respectRegion {
			if len(b.Regions) > 0 && leadLocation != "" && !regionMatches(b.Regions, leadLocation) {
				return false
			}
		}

		if respectSchedule {
			bh := b.BusinessHours
			if bh == nil {
				bh = cfg.GlobalBusinessHours
			}
			if bh != nil && !bh.IsOpenAt(time.Now()) {
				return false
			}
		}

		return true
	}

	countEligible := func() int {
		n := 0
		for _, id := range pool {
			if eligible(id) {
				n++
			}
		}
		return n
	}

	if countEligible() == 0 {
		if respectSpecialty {
			respectSpecialty = false
		}
		if countEligible() == 0 && respectRegion {
			respectRegion = false
		}
		if countEligible() == 0 && respectSchedule {
			respectSchedule = false
		}
	}

	start := currentIndex(cfg, stage)

	if cfg.DistMethod == lead.DistMethodPriority {
		// Busca primeiro os especialistas top (IsPriority)
		for i := 1; i <= len(pool); i++ {
			idx := (start + i) % len(pool)
			id := pool[idx]
			if eligible(id) && brokerMap[id].IsPriority {
				return id, idx, lead.DistMethodPriority, nil
			}
		}
		// Fallback para sequential
	}

	if cfg.DistMethod == lead.DistMethodWeighted {
		// Seleção ponderada
		var eligibleBrokers []string
		var totalWeight float64
		for _, id := range pool {
			if eligible(id) {
				eligibleBrokers = append(eligibleBrokers, id)
				w := brokerMap[id].Weight
				if w <= 0 {
					w = 1.0 // Peso default
				}
				totalWeight += w
			}
		}
		if len(eligibleBrokers) > 0 {
			rnd := rand.Float64()
			if rng != nil {
				rnd = rng.Float64()
			}
			r := rnd * totalWeight
			currentWeight := 0.0
			for _, id := range eligibleBrokers {
				w := brokerMap[id].Weight
				if w <= 0 {
					w = 1.0
				}
				currentWeight += w
				if r <= currentWeight {
					// Find index in original pool to advance pointer
					idx := -1
					for i, pid := range pool {
						if pid == id {
							idx = i
							break
						}
					}
					return id, idx, lead.DistMethodWeighted, nil
				}
			}
		}
		// Fallback se ninguém for elegível
	}

	if cfg.DistMethod == lead.DistMethodLoadBalance {
		// 1) encontra a menor carga entre elegíveis
		minLoad := int(^uint(0) >> 1)
		for _, id := range pool {
			if !eligible(id) {
				continue
			}
			if loads[id] < minLoad {
				minLoad = loads[id]
			}
		}
		if minLoad == int(^uint(0)>>1) {
			// ninguém elegível
			return "", -1, lead.DistMethodLoadBalance, nil
		}

		// 2) tie-break: percorre em ordem a partir do ponteiro e pega o primeiro com minLoad
		bestID := ""
		bestIdx := -1
		for i := 1; i <= len(pool); i++ {
			idx := (start + i) % len(pool)
			id := pool[idx]
			if !eligible(id) {
				continue
			}
			if loads[id] == minLoad {
				bestID = id
				bestIdx = idx
				break
			}
		}
		if bestID != "" {
			return bestID, bestIdx, lead.DistMethodLoadBalance, nil
		}
	}

	// Sequential e Fallback Geral
	for i := 1; i <= len(pool); i++ {
		idx := (start + i) % len(pool)
		if eligible(pool[idx]) {
			return pool[idx], idx, lead.DistMethodSequential, nil
		}
	}

	return "", -1, lead.DistMethodSequential, nil
}

func matchSLATier(l *lead.Lead, tiers []lead.SLATier) *lead.SLATier {
	if l == nil || len(tiers) == 0 {
		return nil
	}

	tags := l.DeriveTags()
	bestIdx := -1
	bestScore := 0
	fallbackIdx := -1

	for i := range tiers {
		t := tiers[i]
		if len(t.AppliesTo) == 0 && fallbackIdx == -1 {
			fallbackIdx = i
		}
		score := countIntersect(t.AppliesTo, tags)
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	if bestIdx >= 0 && bestScore > 0 {
		out := tiers[bestIdx]
		return &out
	}
	if fallbackIdx >= 0 {
		out := tiers[fallbackIdx]
		return &out
	}
	return nil
}

// AssignByRoulette attempts to route the lead and returns true if assigned
func (s *LeadRouletteService) AssignByRoulette(ctx context.Context, l *lead.Lead, excludeBrokerID string) (bool, error) {
	if l == nil || l.ID == "" || l.TenantID == "" {
		return false, nil
	}
	if strings.TrimSpace(l.AssignedBrokerID) != "" {
		return false, nil
	}

	cfg, err := s.rouletteRepo.GetConfig(ctx, l.TenantID)
	if err != nil {
		return false, err
	}
	if cfg == nil || !cfg.Enabled {
		return false, nil
	}

	stage := string(l.Status)
	if stage == "" {
		stage = string(lead.LeadStatusNew)
	}
	stage = normalizeStageKey(stage)

	var brokerID string
	var method lead.DistMethod
	maxAttempts := 10

	for attempts := 0; attempts < maxAttempts; attempts++ {
		// Sempre usa config "fresh" para evitar ficar preso em ponteiro antigo quando há contensão.
		if attempts > 0 {
			cfg, err = s.rouletteRepo.GetConfig(ctx, l.TenantID)
			if err != nil {
				return false, err
			}
			if cfg == nil || !cfg.Enabled {
				return false, nil
			}
		}

		chosenID, idx, m, err := s.pickBroker(ctx, l.TenantID, cfg, l, excludeBrokerID)
		if err != nil {
			return false, err
		}
		if chosenID == "" || idx < 0 {
			pool := poolForStage(cfg, stage)
			expected := currentIndex(cfg, stage)
			if len(pool) > 0 {
				next := (expected + 1) % len(pool)
				if _, bumpErr := s.rouletteRepo.SetIndexCAS(ctx, l.TenantID, stage, expected, next); bumpErr != nil {
					logger.Warn("failed to bump roulette pointer after no_eligible",
						"tenant_id", l.TenantID,
						"stage", stage,
						"expected", expected,
						"next", next,
						"error", bumpErr.Error())
				}
			}
			if s.auditRepo != nil {
				_ = s.auditRepo.Create(ctx, &lead.RouletteAuditEntry{
					ID:        uuid.New().String(),
					TenantID:  l.TenantID,
					ActorID:   "system",
					Action:    "lead.unassigned",
					TargetID:  l.ID,
					Diff:      map[string]interface{}{"reason": "no_eligible_brokers", "stage": stage, "tags": l.DeriveTags()},
					CreatedAt: time.Now(),
				})
			}
			metrics.IncRouletteAssignment(l.TenantID, string(cfg.DistMethod), "no_eligible")
			return false, nil
		}

		expected := currentIndex(cfg, stage)
		ok, err := s.rouletteRepo.SetIndexCAS(ctx, l.TenantID, stage, expected, idx)
		if err != nil {
			return false, err
		}
		if ok {
			brokerID = chosenID
			method = m
			break
		}
	}

	if brokerID == "" {
		if s.auditRepo != nil {
			_ = s.auditRepo.Create(ctx, &lead.RouletteAuditEntry{
				ID:        uuid.New().String(),
				TenantID:  l.TenantID,
				ActorID:   "system",
				Action:    "lead.unassigned",
				TargetID:  l.ID,
				Diff:      map[string]interface{}{"reason": "cas_exhausted", "stage": stage, "tags": l.DeriveTags()},
				CreatedAt: time.Now(),
			})
		}
		metrics.IncRouletteAssignment(l.TenantID, string(cfg.DistMethod), "cas_exhausted")
		return false, nil
	}

	// Apply assignment via repo (idempotent)
	ok, err := s.leadRepo.AssignBrokerIfUnassigned(ctx, l.ID, l.TenantID, brokerID, string(method))
	if err != nil {
		metrics.IncRouletteAssignment(l.TenantID, string(method), "error")
		return false, err
	}
	if !ok {
		// Lead já estava atribuído por um caminho concorrente — não emite
		// métrica de erro porque é resultado normal de idempotência.
		return false, nil
	}
	metrics.IncRouletteAssignment(l.TenantID, string(method), "ok")

	// Important: update the struct in memory because subsequent operations might overwrite it in DB
	l.AssignedBrokerID = brokerID

	// Save History
	if s.historyRepo != nil {
		_ = s.historyRepo.Create(ctx, &lead.RouletteHistory{
			ID:         uuid.New().String(),
			TenantID:   l.TenantID,
			LeadID:     l.ID,
			ToBrokerID: brokerID,
			Stage:      stage,
			Method:     string(method),
			CreatedAt:  time.Now(),
		})
	}

	// Calculate SLA Expiration
	sla := matchSLATier(l, cfg.SLATiers)
	if sla != nil && sla.TimeLimitMin > 0 {
		now := time.Now()
		l.ExpiresAt = now.Add(time.Duration(sla.TimeLimitMin) * time.Minute)
		l.Status = lead.LeadStatusPendingAcceptance
		l.AssignedAt = now
		_ = s.leadRepo.Update(ctx, l)
	}

	leadTags := l.DeriveTags()
	if s.auditRepo != nil {
		_ = s.auditRepo.Create(ctx, &lead.RouletteAuditEntry{
			ID:        uuid.New().String(),
			TenantID:  l.TenantID,
			ActorID:   "system",
			Action:    "lead.assigned",
			TargetID:  l.ID,
			Diff:      map[string]interface{}{"broker_id": brokerID, "stage": stage, "method": string(method), "tags": leadTags},
			CreatedAt: time.Now(),
		})
	}

	// Global audit (#467). O auditRepo acima alimenta /roulette/audit
	// (domínio); este Record() alimenta o /audit-trail global do J10.
	// Metadata inclui regra (method), pool (stage) e tags — exatamente
	// o que o auditor Norberto procura no cenário principal do BDD J10
	// (etapa 4: "regra aplicada, pool, especialidade").
	audit.Record(ctx, audit.Entry{
		Action:     "lead.assigned",
		EntityType: "lead",
		EntityID:   l.ID,
		Before:     map[string]any{"assigned_broker_id": ""},
		After:      map[string]any{"assigned_broker_id": brokerID},
		Metadata: map[string]any{
			"rule":   string(method),
			"stage":  stage,
			"tags":   leadTags,
			"actor":  "system",
			"source": "roulette",
		},
	})

	// Publish Event para toca-comm
	if s.eventBus != nil {
		channels := []string{}
		for _, ch := range cfg.NotifChannels {
			if ch.Enabled {
				channels = append(channels, ch.Name)
			}
		}

		s.publishAsync(ctx, LeadAssignedEvent{
			LeadID:   l.ID,
			LeadName: l.Name,
			TenantID: l.TenantID,
			BrokerID: brokerID,
			Channels: channels,
			Stage:    stage,
			Method:   string(method),
			Tags:     l.DeriveTags(),
		})
	}

	return true, nil
}

type LeadAssignedEvent struct {
	LeadID string `json:"lead_id"`
	// LeadName permite ao consumidor (toca-comm) montar notificação/e-mail
	// com o nome do lead, sem round-trip ao CRM. Aditivo — consumidores
	// antigos ignoram. Beta de e-mail transacional (notificacoes-email-beta).
	LeadName string   `json:"lead_name,omitempty"`
	TenantID string   `json:"tenant_id"`
	BrokerID string   `json:"broker_id"`
	Channels []string `json:"channels"`
	Stage    string   `json:"stage,omitempty"`
	Method   string   `json:"method,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

func (e LeadAssignedEvent) EventID() string {
	return fmt.Sprintf("%s-%s-%d", e.Name(), e.LeadID, time.Now().UnixNano())
}
func (e LeadAssignedEvent) Name() string         { return "lead.assigned" }
func (e LeadAssignedEvent) Payload() interface{} { return e }

// RouletteConfigUpdatedEvent é emitido quando a config muda — diff
// contém apenas as keys alteradas. Consumidores (WebSocket fan-out,
// auditoria externa) usam ChangedFields para decidir se vale a pena
// invalidar caches no front. ActorID/ActorEmail são oportunísticos: o
// audit já mantém a verdade fonte, o evento serve para notificação.
type RouletteConfigUpdatedEvent struct {
	TenantID      string    `json:"tenant_id"`
	ActorID       string    `json:"actor_id,omitempty"`
	ActorEmail    string    `json:"actor_email,omitempty"`
	ChangedFields []string  `json:"changed_fields"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (e RouletteConfigUpdatedEvent) EventID() string {
	return fmt.Sprintf("%s-%s-%d", e.Name(), e.TenantID, time.Now().UnixNano())
}
func (e RouletteConfigUpdatedEvent) Name() string         { return "roulette.config_updated" }
func (e RouletteConfigUpdatedEvent) Payload() interface{} { return e }

type RouletteSimulationItem struct {
	Number   int    `json:"n"`
	BrokerID string `json:"broker_id"`
}

type RouletteSimulationResult struct {
	TenantID    string                   `json:"tenant_id"`
	Stage       string                   `json:"stage"`
	Count       int                      `json:"count"`
	Persist     bool                     `json:"persist"`
	Seed        int64                    `json:"seed,omitempty"`
	PoolSize    int                      `json:"pool_size"`
	Assignments []RouletteSimulationItem `json:"assignments"`
}

func (s *LeadRouletteService) SimulateRoulette(ctx context.Context, tenantID string, stage string, count int, persist bool, seed int64) (*RouletteSimulationResult, error) {
	if count <= 0 {
		count = 1
	}

	stageKey := strings.ToUpper(strings.TrimSpace(stage))

	cfg, err := s.rouletteRepo.GetConfig(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	if !cfg.Enabled {
		return &RouletteSimulationResult{
			TenantID:    tenantID,
			Stage:       stage,
			Count:       count,
			Persist:     persist,
			PoolSize:    0,
			Assignments: []RouletteSimulationItem{},
		}, nil
	}

	pool := poolForStage(cfg, stage)
	res := &RouletteSimulationResult{
		TenantID:    tenantID,
		Stage:       stage,
		Count:       count,
		Persist:     persist,
		Seed:        seed,
		PoolSize:    len(pool),
		Assignments: make([]RouletteSimulationItem, 0, count),
	}

	if len(pool) == 0 {
		return res, nil
	}

	// Mock lead for simulation
	l := &lead.Lead{
		ID:       uuid.New().String(),
		TenantID: tenantID,
		Status:   lead.LeadStatus(stageKey),
	}

	var rng *rand.Rand
	if seed <= 0 {
		seed = cfg.SimulationSeed
		res.Seed = seed
	}
	if seed > 0 {
		rng = rand.New(rand.NewSource(seed))
	}

	for i := 0; i < count; i++ {
		// Para persist=true, a simulação precisa refletir o ponteiro real em banco.
		// Então: escolhe candidato baseado na config atual, tenta CAS com o "expected" anterior;
		// se falhar, refaz com config atualizada e tenta novamente (com limite).
		attempts := 1
		if persist {
			attempts = 5
		}

		var chosenID string
		var idx int
		for a := 0; a < attempts; a++ {
			expectedBefore := currentIndex(cfg, stageKey)

			var err error
			chosenID, idx, _, err = s.pickBrokerWithRng(ctx, tenantID, cfg, l, "", rng)
			if err != nil {
				return res, err
			}
			if chosenID == "" {
				// Sem elegíveis
				return res, nil
			}

			if !persist {
				break
			}

			ok, err := s.rouletteRepo.SetIndexCAS(ctx, tenantID, stageKey, expectedBefore, idx)
			if err != nil {
				return res, err
			}
			if ok {
				break
			}

			// CAS falhou: refetch config e tenta novamente este mesmo item
			cfg, err = s.rouletteRepo.GetConfig(ctx, tenantID)
			if err != nil {
				return res, err
			}
			chosenID = ""
		}

		if chosenID == "" {
			// não conseguiu persistir avanço do ponteiro
			break
		}

		res.Assignments = append(res.Assignments, RouletteSimulationItem{Number: i + 1, BrokerID: chosenID})

		// Advance index localmente (dry-run) ou para manter consistência no retorno (persist com CAS ok)
		if cfg.StageLastIndex == nil {
			cfg.StageLastIndex = make(map[string]int)
		}
		if stageKey != "" {
			cfg.StageLastIndex[stageKey] = idx
		} else {
			cfg.LastIndex = idx
		}
	}

	return res, nil
}

func (s *LeadRouletteService) HandleLeadCreatedAssign(ctx context.Context, event events.Event) error {
	// Parse event payload
	// The payload is typically a map or the LeadCreatedEvent struct
	payload := event.Payload()
	var leadID, tenantID string

	switch v := payload.(type) {
	case map[string]interface{}:
		if id, ok := v["lead_id"].(string); ok {
			leadID = id
		}
		if tid, ok := v["tenant_id"].(string); ok {
			tenantID = tid
		}
	case LeadCreatedEvent:
		leadID = v.LeadID
		tenantID = v.TenantID
	}

	if leadID == "" || tenantID == "" {
		return nil
	}

	tenantCtx := pkgctx.WithTenantID(ctx, tenantID)

	l, err := s.leadRepo.GetByID(tenantCtx, tenantID, leadID)
	if err != nil || l == nil {
		return err
	}

	if l.AssignedBrokerID != "" {
		return nil // already assigned
	}

	_, err = s.AssignByRoulette(tenantCtx, l, "")
	return err
}
