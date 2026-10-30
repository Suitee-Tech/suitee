package metrics

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Business metrics — product KPIs surfaced alongside infra metrics so the
// same Grafana stack answers both "is the system healthy?" and "is the
// product working?". Kept lock-free where possible; falls back to
// sync.Map for cardinality-keyed counters.
//
// Cardinality discipline: only attributes that are bounded by the product
// (deal stages, lead statuses, currency) become labels. tenant_id is a
// label only on gauges that operators want to slice (active deals,
// MRR) — never on counters/histograms where the long tail would blow up
// Prometheus storage.

var (
	// activeDealsByStage{stage, tenant_id} — current snapshot of deals in
	// each pipeline stage. Set by toca-deal periodically (or on each
	// transition). Used by leadership dashboards to track funnel volume.
	activeDealsByStage sync.Map // "tenant_id\x1fstage" -> *atomic.Int64

	// dealConversionsTotal{from_stage, to_stage} — every Deal status
	// transition. Builders compute lead_conversion_ratio off this.
	dealConversionsTotal sync.Map // "from\x1fto" -> *atomic.Uint64

	// leadStatusChangedTotal{from_status, to_status} — every Lead status
	// transition for funnel analytics.
	leadStatusChangedTotal sync.Map // "from\x1fto" -> *atomic.Uint64

	// rentalsCreatedTotal — counter of rentals materialised in
	// toca-financial. The denominator for MRR calculations.
	rentalsCreatedTotal atomic.Uint64

	// mrrCentsByTenant{tenant_id} — gauge holding the current monthly
	// recurring revenue (in BRL cents to avoid float ops in Go). Updated
	// by toca-financial when rentals are created/cancelled.
	mrrCentsByTenant sync.Map // tenant_id -> *atomic.Int64

	// timeToContractSeconds — histogram of how long it took for each
	// deal that reached the contract stage. p50/p90 surface team speed.
	timeToContractSeconds = newHistogram()

	// timeToConversionSeconds — histogram of the full Lead → Deal CONVERTED
	// trajectory. Slower-moving but more business-aligned than time-to-contract.
	timeToConversionSeconds = newHistogram()

	// invoicePaidTotal{type} — counter of invoices paid, sliced by type
	// (charge, commission, etc.). Useful for cash-flow tracking.
	invoicePaidTotal sync.Map // type -> *atomic.Uint64

	// commEmailSentTotal{category} — transactional emails actually submitted
	// to Resend, by beta category (lead_assigned, deal_updates, proposals,
	// appointments, recommendations, chat_reminders). Bounded cardinality.
	commEmailSentTotal sync.Map // category -> *atomic.Uint64

	// commEmailSkippedTotal{reason} — suppressions/skips by reason
	// (flag_off, optout, online, cooldown, no_email, dedup, send_failed).
	commEmailSkippedTotal sync.Map // reason -> *atomic.Uint64

	// commChatReminderCandidatesTotal — unread-chat candidates processed by
	// the reminder engine across ticks.
	commChatReminderCandidatesTotal atomic.Uint64

	// commChatReminderSentTotal — chat reminder emails actually dispatched.
	commChatReminderSentTotal atomic.Uint64

	// proposalFollowUpEnqueuedTotal — proposal follow-up reminders enqueued by
	// the toca-deal engine (§F6), by stage of the cadence.
	proposalFollowUpEnqueuedTotal atomic.Uint64

	// proposalFollowUpScannedTotal — deals scanned by the follow-up sweep.
	proposalFollowUpScannedTotal atomic.Uint64

	// --- Inadimplência (Fase 1.3 do plano de inadimplência) ----------------
	//
	// O domínio de cobrança não tinha nenhuma série Prometheus: "o motor
	// parou" e "a carteira vencida dobrou" só apareciam quando alguém abria
	// a tela. As séries abaixo são o sinal mínimo para alertar — e seguem a
	// mesma disciplina de cardinalidade do topo do arquivo: tenant_id só em
	// gauge, result é enum fechado e nenhum id de cobrança/caso/contrato
	// vira label.

	// overdueSnapshotsByTenant{tenant_id} — valor vencido e quantidade de
	// cobranças do mesmo tenant, gravados juntos para que o painel nunca
	// misture valor de uma varredura com contagem de outra.
	overdueSnapshotsByTenant sync.Map // tenant_id -> *overdueSnapshot

	// collectionStatusCounts{tenant_id,status} — casos por status. É
	// snapshot, não acumulador: SetCollectionStatusCounts substitui o
	// conjunto do tenant (status que saiu do mapa deixa de ser exposto).
	collectionStatusCounts sync.Map // "tenant_id\x1fstatus" -> *atomicFloat64

	// openCollectionDebtBRLByTenant{tenant_id} — dívida total dos casos
	// ABERTOS. Caso RESOLVED fica fora: incluí-lo infla o número (G22 do
	// diagnóstico).
	openCollectionDebtBRLByTenant sync.Map // tenant_id -> *atomicFloat64

	// delinquencyTickTotal{result} — ticks do motor horário por resultado.
	// Sem tenant_id de propósito: o counter é global e o alerta é sobre o
	// motor, não sobre um tenant.
	delinquencyTickTotal sync.Map // result -> *atomic.Uint64

	// delinquencyTickDurationSeconds — duração da varredura inteira do tick.
	// Buckets próprios (limites até 10 min): com os buckets padrão de
	// latência HTTP (topo em 5s) quase toda execução cairia em +Inf e o p99
	// seria inútil.
	delinquencyTickDurationSeconds = newHistogramWithBounds([]float64{0.5, 1, 2, 5, 10, 30, 60, 120, 300, 600})

	// delinquencyManualRunsTotal{result} — disparos manuais do motor
	// (ProcessManualTrigger), que não passam pelo advisory lock (G28) e por
	// isso precisam de sinal próprio.
	delinquencyManualRunsTotal sync.Map // result -> *atomic.Uint64
)

// overdueSnapshot guarda o par (valor vencido, cobranças vencidas) de um
// tenant. O valor é float — Prometheus lê float — e atomic não tem Float64,
// então os bits são guardados para manter a leitura sem lock.
type overdueSnapshot struct {
	amount  atomicFloat64
	charges atomic.Int64
}

// atomicFloat64 é um float64 atômico via math.Float64bits.
type atomicFloat64 struct {
	bits atomic.Uint64
}

func (f *atomicFloat64) store(v float64) { f.bits.Store(math.Float64bits(v)) }

func (f *atomicFloat64) load() float64 { return math.Float64frombits(f.bits.Load()) }

// SetActiveDealsByStage updates the gauge of active deals for one
// (tenant_id, stage) combination. Callers should pass tenant_id as UUID
// string and stage as the lowercased status enum.
func SetActiveDealsByStage(tenantID, stage string, value int64) {
	if tenantID == "" || stage == "" {
		return
	}
	key := tenantID + "\x1f" + stage
	if v, ok := activeDealsByStage.Load(key); ok {
		v.(*atomic.Int64).Store(value)
		return
	}
	var n atomic.Int64
	n.Store(value)
	actual, loaded := activeDealsByStage.LoadOrStore(key, &n)
	if loaded {
		actual.(*atomic.Int64).Store(value)
	}
}

// IncDealConversion records one Deal status transition. from/to are the
// stage labels (lowercased). Pass empty strings to skip — guards against
// noisy increments from intermediate transient states.
func IncDealConversion(from, to string) {
	if from == "" || to == "" || from == to {
		return
	}
	incEventCounter(&dealConversionsTotal, from+"\x1f"+to)
}

// IncLeadStatusChanged records one Lead status transition.
func IncLeadStatusChanged(from, to string) {
	if from == "" || to == "" || from == to {
		return
	}
	incEventCounter(&leadStatusChangedTotal, from+"\x1f"+to)
}

// IncRentalCreated is called once per Rental insert in toca-financial.
func IncRentalCreated() {
	rentalsCreatedTotal.Add(1)
}

// SetMRRCents updates the MRR gauge for one tenant. Pass the integer
// amount in BRL cents so the metric value stays exact (Prometheus parses
// floats anyway, but cents-as-int avoids accumulated rounding when
// summing).
func SetMRRCents(tenantID string, cents int64) {
	if tenantID == "" {
		return
	}
	if v, ok := mrrCentsByTenant.Load(tenantID); ok {
		v.(*atomic.Int64).Store(cents)
		return
	}
	var n atomic.Int64
	n.Store(cents)
	actual, loaded := mrrCentsByTenant.LoadOrStore(tenantID, &n)
	if loaded {
		actual.(*atomic.Int64).Store(cents)
	}
}

// ObserveTimeToContract records how long it took for the Deal to reach
// the contract stage. Called by toca-deal when a Deal transitions into
// the CONTRACT status — the duration is now() - deal.created_at.
func ObserveTimeToContract(d time.Duration) {
	if d <= 0 {
		return
	}
	timeToContractSeconds.observe(d)
}

// ObserveTimeToConversion records the full lead-to-converted lifecycle.
func ObserveTimeToConversion(d time.Duration) {
	if d <= 0 {
		return
	}
	timeToConversionSeconds.observe(d)
}

// IncInvoicePaid counts paid invoices by type ("charge"|"commission"|...).
func IncInvoicePaid(invoiceType string) {
	if invoiceType == "" {
		invoiceType = "unknown"
	}
	incEventCounter(&invoicePaidTotal, invoiceType)
}

// IncEmailSent counts a transactional email actually submitted to Resend.
func IncEmailSent(category string) {
	if category == "" {
		category = "unknown"
	}
	incEventCounter(&commEmailSentTotal, category)
}

// IncEmailSkipped counts a suppressed/skipped transactional email by reason.
func IncEmailSkipped(reason string) {
	if reason == "" {
		reason = "unknown"
	}
	incEventCounter(&commEmailSkippedTotal, reason)
}

// AddChatReminderCandidates accumulates unread-chat candidates processed.
func AddChatReminderCandidates(n uint64) {
	commChatReminderCandidatesTotal.Add(n)
}

// IncChatReminderSent counts a dispatched chat reminder email.
func IncChatReminderSent() {
	commChatReminderSentTotal.Add(1)
}

// AddProposalFollowUpEnqueued accumulates proposal follow-up reminders
// enqueued by the toca-deal engine (§F6). Each reminder is one group
// (proposal + stage), so the counter measures groups dispatched — not ticks.
func AddProposalFollowUpEnqueued(n uint64) {
	proposalFollowUpEnqueuedTotal.Add(n)
}

// AddProposalFollowUpScanned accumulates deals scanned by the follow-up sweep.
func AddProposalFollowUpScanned(n uint64) {
	proposalFollowUpScannedTotal.Add(n)
}

// SetOverdueSnapshot publica o retrato da carteira vencida de um tenant:
// valor total vencido (BRL) e quantidade de cobranças. Quem chama é o motor
// de inadimplência ao fim da varredura (tick horário ou disparo manual); o
// gauge é um retrato, então cada tenant guarda o valor da última passagem.
func SetOverdueSnapshot(tenantID string, amountBRL float64, charges int) {
	if tenantID == "" {
		return
	}
	// Valor/contagem negativos são bug de agregação: 0 mente menos que um
	// negativo no painel.
	if amountBRL < 0 {
		amountBRL = 0
	}
	if charges < 0 {
		charges = 0
	}

	var snap *overdueSnapshot
	if cur, ok := overdueSnapshotsByTenant.Load(tenantID); ok {
		snap = cur.(*overdueSnapshot)
	} else {
		actual, _ := overdueSnapshotsByTenant.LoadOrStore(tenantID, &overdueSnapshot{})
		snap = actual.(*overdueSnapshot)
	}
	snap.amount.store(amountBRL)
	snap.charges.Store(int64(charges))
}

// SetCollectionStatusCounts substitui o retrato de casos por status do
// tenant (gauge, não acumulador): status ausente do mapa sai da exposição,
// senão um caso que deixou NEGOTIATED ficaria contado para sempre. O status
// é normalizado para minúsculas (convenção do arquivo: ACTIVE -> active)
// para que duas chamadas não criem séries duplicadas.
func SetCollectionStatusCounts(tenantID string, counts map[string]float64) {
	if tenantID == "" {
		return
	}

	keep := make(map[string]struct{}, len(counts))
	for status, n := range counts {
		status = strings.ToLower(strings.TrimSpace(status))
		if status == "" {
			continue
		}
		if n < 0 {
			n = 0
		}
		storeFloatGauge(&collectionStatusCounts, tenantID+"\x1f"+status, n)
		keep[status] = struct{}{}
	}

	// Remove os status que não vieram neste retrato. O tenant é UUID, então
	// o separador \x1f não aparece nele e o prefixo não casa com outro tenant.
	prefix := tenantID + "\x1f"
	collectionStatusCounts.Range(func(k, _ interface{}) bool {
		key := k.(string)
		if strings.HasPrefix(key, prefix) {
			if _, ok := keep[key[len(prefix):]]; !ok {
				collectionStatusCounts.Delete(key)
			}
		}
		return true
	})
}

// SetOpenCollectionDebtBRL publica a dívida total dos casos ABERTOS do
// tenant. Casos RESOLVED ficam de fora por contrato: somá-los infla o KPI
// (G22 do diagnóstico de inadimplência).
func SetOpenCollectionDebtBRL(tenantID string, amountBRL float64) {
	if tenantID == "" {
		return
	}
	if amountBRL < 0 {
		amountBRL = 0
	}
	storeFloatGauge(&openCollectionDebtBRLByTenant, tenantID, amountBRL)
}

// ObserveDelinquencyTick registra uma execução do tick horário do motor de
// inadimplência: conta por resultado e mede a duração da varredura inteira.
// `result` é enum fechado (success|error) — só "success" conta como sucesso,
// porque bug de instrumentação não pode mascarar motor quebrado deixando o
// alerta de erro mudo.
func ObserveDelinquencyTick(result string, seconds float64) {
	incEventCounter(&delinquencyTickTotal, normalizeTickResult(result))
	// Duração absurda (>24h) é bug de chamada e estouraria a conversão para
	// time.Duration; o counter acima já registrou que o tick aconteceu.
	if seconds > 0 && seconds <= 86400 {
		delinquencyTickDurationSeconds.observe(time.Duration(seconds * float64(time.Second)))
	}
}

// IncDelinquencyManualRun conta um disparo manual do motor (o
// ProcessManualTrigger, que não passa pelo advisory lock — G28). Mesmo enum
// fechado do tick: success|error.
func IncDelinquencyManualRun(result string) {
	incEventCounter(&delinquencyManualRunsTotal, normalizeTickResult(result))
}

// normalizeTickResult mantém o rótulo com dois valores possíveis. Tudo que
// não for "success" vira "error" de propósito (fail-safe): um valor novo ou
// inválido não pode esconder uma falha do alerta.
func normalizeTickResult(result string) string {
	if strings.EqualFold(strings.TrimSpace(result), "success") {
		return "success"
	}
	return "error"
}

// storeFloatGauge grava um gauge float no sync.Map, criando a entrada na
// primeira chamada; o LoadOrStore resolve a corrida entre goroutines.
func storeFloatGauge(store *sync.Map, key string, v float64) {
	if cur, ok := store.Load(key); ok {
		cur.(*atomicFloat64).store(v)
		return
	}
	var f atomicFloat64
	f.store(v)
	actual, _ := store.LoadOrStore(key, &f)
	actual.(*atomicFloat64).store(v)
}

// splitKey separa "a\x1fb" — o mesmo separador usado nas chaves do arquivo.
func splitKey(key string) (string, string) {
	for i := 0; i < len(key); i++ {
		if key[i] == 0x1f {
			return key[:i], key[i+1:]
		}
	}
	return key, ""
}

// formatBRLValue mantém o valor do gauge legível no texto Prometheus sem
// notação científica (valor monetário é sempre decimal simples).
func formatBRLValue(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// writeBusinessMetrics emits the business series in Prometheus text format.
// Called by the main Handler so all business KPIs ship on the same
// /metrics endpoint as the infra ones.
func writeBusinessMetrics(write func(string)) {
	write("# HELP active_deals_by_stage Current number of non-deleted deals per (tenant, pipeline stage).\n")
	write("# TYPE active_deals_by_stage gauge\n")
	activeDealsByStage.Range(func(k, v interface{}) bool {
		key := k.(string)
		var tenant, stage string
		_, _ = fmt.Sscanf(key, "%[^\x1f]\x1f%s", &tenant, &stage)
		// Sscanf with separator is unreliable; do it by hand.
		for i := 0; i < len(key); i++ {
			if key[i] == 0x1f {
				tenant = key[:i]
				stage = key[i+1:]
				break
			}
		}
		val := v.(*atomic.Int64).Load()
		write(fmt.Sprintf("active_deals_by_stage{tenant_id=\"%s\",stage=\"%s\"} %d\n",
			escapeLabelValue(tenant), escapeLabelValue(stage), val))
		return true
	})

	write("# HELP deal_conversions_total Counter of deal status transitions.\n")
	write("# TYPE deal_conversions_total counter\n")
	dealConversionsTotal.Range(func(k, v interface{}) bool {
		key := k.(string)
		var from, to string
		for i := 0; i < len(key); i++ {
			if key[i] == 0x1f {
				from = key[:i]
				to = key[i+1:]
				break
			}
		}
		n := v.(*atomic.Uint64).Load()
		write(fmt.Sprintf("deal_conversions_total{from_stage=\"%s\",to_stage=\"%s\"} %d\n",
			escapeLabelValue(from), escapeLabelValue(to), n))
		return true
	})

	write("# HELP lead_status_changed_total Counter of lead status transitions.\n")
	write("# TYPE lead_status_changed_total counter\n")
	leadStatusChangedTotal.Range(func(k, v interface{}) bool {
		key := k.(string)
		var from, to string
		for i := 0; i < len(key); i++ {
			if key[i] == 0x1f {
				from = key[:i]
				to = key[i+1:]
				break
			}
		}
		n := v.(*atomic.Uint64).Load()
		write(fmt.Sprintf("lead_status_changed_total{from_status=\"%s\",to_status=\"%s\"} %d\n",
			escapeLabelValue(from), escapeLabelValue(to), n))
		return true
	})

	write("# HELP rentals_created_total Total Rentals materialised by toca-financial since process start.\n")
	write("# TYPE rentals_created_total counter\n")
	write(fmt.Sprintf("rentals_created_total %d\n", rentalsCreatedTotal.Load()))

	write("# HELP mrr_cents Current Monthly Recurring Revenue in BRL cents, per tenant.\n")
	write("# TYPE mrr_cents gauge\n")
	mrrCentsByTenant.Range(func(k, v interface{}) bool {
		tenant := k.(string)
		val := v.(*atomic.Int64).Load()
		write(fmt.Sprintf("mrr_cents{tenant_id=\"%s\"} %d\n", escapeLabelValue(tenant), val))
		return true
	})

	writeHistogram(write, "time_to_contract_seconds", "Seconds elapsed between Deal creation and CONTRACT stage.", timeToContractSeconds, nil)
	writeHistogram(write, "time_to_conversion_seconds", "Seconds elapsed between Deal creation and CONVERTED stage.", timeToConversionSeconds, nil)

	write("# HELP invoice_paid_total Counter of paid invoices by type.\n")
	write("# TYPE invoice_paid_total counter\n")
	invoicePaidTotal.Range(func(k, v interface{}) bool {
		invType := k.(string)
		n := v.(*atomic.Uint64).Load()
		write(fmt.Sprintf("invoice_paid_total{type=\"%s\"} %d\n", escapeLabelValue(invType), n))
		return true
	})

	write("# HELP comm_email_sent_total Transactional emails submitted to Resend by category.\n")
	write("# TYPE comm_email_sent_total counter\n")
	commEmailSentTotal.Range(func(k, v interface{}) bool {
		write(fmt.Sprintf("comm_email_sent_total{category=\"%s\"} %d\n", escapeLabelValue(k.(string)), v.(*atomic.Uint64).Load()))
		return true
	})

	write("# HELP comm_email_skipped_total Suppressed or skipped transactional emails by reason.\n")
	write("# TYPE comm_email_skipped_total counter\n")
	commEmailSkippedTotal.Range(func(k, v interface{}) bool {
		write(fmt.Sprintf("comm_email_skipped_total{reason=\"%s\"} %d\n", escapeLabelValue(k.(string)), v.(*atomic.Uint64).Load()))
		return true
	})

	write("# HELP comm_chat_reminder_candidates_total Unread-chat candidates processed by the reminder engine.\n")
	write("# TYPE comm_chat_reminder_candidates_total counter\n")
	write(fmt.Sprintf("comm_chat_reminder_candidates_total %d\n", commChatReminderCandidatesTotal.Load()))

	write("# HELP comm_chat_reminder_sent_total Chat reminder emails dispatched.\n")
	write("# TYPE comm_chat_reminder_sent_total counter\n")
	write(fmt.Sprintf("comm_chat_reminder_sent_total %d\n", commChatReminderSentTotal.Load()))

	write("# HELP proposal_followup_scanned_total Deals scanned by the proposal follow-up sweep.\n")
	write("# TYPE proposal_followup_scanned_total counter\n")
	write(fmt.Sprintf("proposal_followup_scanned_total %d\n", proposalFollowUpScannedTotal.Load()))

	write("# HELP proposal_followup_enqueued_total Proposal follow-up reminders enqueued by group (proposal + stage).\n")
	write("# TYPE proposal_followup_enqueued_total counter\n")
	write(fmt.Sprintf("proposal_followup_enqueued_total %d\n", proposalFollowUpEnqueuedTotal.Load()))

	// --- Inadimplência (Fase 1.3) — carteira vencida, casos e motor -------
	write("# HELP financial_overdue_amount_brl Total overdue amount in BRL per tenant (unpaid balance of overdue charges).\n")
	write("# TYPE financial_overdue_amount_brl gauge\n")
	overdueSnapshotsByTenant.Range(func(k, v interface{}) bool {
		snap := v.(*overdueSnapshot)
		write(fmt.Sprintf("financial_overdue_amount_brl{tenant_id=\"%s\"} %s\n",
			escapeLabelValue(k.(string)), formatBRLValue(snap.amount.load())))
		return true
	})

	write("# HELP financial_overdue_charges Number of overdue charges per tenant.\n")
	write("# TYPE financial_overdue_charges gauge\n")
	overdueSnapshotsByTenant.Range(func(k, v interface{}) bool {
		snap := v.(*overdueSnapshot)
		write(fmt.Sprintf("financial_overdue_charges{tenant_id=\"%s\"} %d\n",
			escapeLabelValue(k.(string)), snap.charges.Load()))
		return true
	})

	write("# HELP financial_collections_by_status Collection cases per tenant and status (active|negotiated|guarantee_activated|judicial|resolved).\n")
	write("# TYPE financial_collections_by_status gauge\n")
	collectionStatusCounts.Range(func(k, v interface{}) bool {
		tenant, status := splitKey(k.(string))
		write(fmt.Sprintf("financial_collections_by_status{tenant_id=\"%s\",status=\"%s\"} %s\n",
			escapeLabelValue(tenant), escapeLabelValue(status), formatBRLValue(v.(*atomicFloat64).load())))
		return true
	})

	write("# HELP financial_collection_case_total_debt_brl Total debt in BRL held by open collection cases per tenant (RESOLVED cases excluded).\n")
	write("# TYPE financial_collection_case_total_debt_brl gauge\n")
	openCollectionDebtBRLByTenant.Range(func(k, v interface{}) bool {
		write(fmt.Sprintf("financial_collection_case_total_debt_brl{tenant_id=\"%s\"} %s\n",
			escapeLabelValue(k.(string)), formatBRLValue(v.(*atomicFloat64).load())))
		return true
	})

	writeHistogram(write, "financial_delinquency_tick_duration_seconds", "Duration in seconds of the hourly delinquency engine sweep (whole tick).", delinquencyTickDurationSeconds, nil)

	// Counters de rótulo fechado: os dois valores são sempre emitidos, mesmo
	// zerados, para que `increase(...{result="success"}[3h])` encontre a série
	// desde o boot do processo — sem ela o alerta de motor parado ficaria mudo.
	writeEnumCounter(write, "financial_delinquency_tick_total", "Delinquency engine ticks by result (success|error).", "result", []string{"success", "error"}, &delinquencyTickTotal)
	writeEnumCounter(write, "financial_delinquency_manual_runs_total", "Manual delinquency engine runs by result (success|error).", "result", []string{"success", "error"}, &delinquencyManualRunsTotal)
}

// writeEnumCounter emite um counter de rótulo fechado, incluindo os valores
// ainda não observados como 0 — é o que permite um alerta do tipo
// "nenhum sucesso na janela" existir antes da primeira falha.
func writeEnumCounter(write func(string), name, help, label string, values []string, store *sync.Map) {
	write(fmt.Sprintf("# HELP %s %s\n", name, help))
	write(fmt.Sprintf("# TYPE %s counter\n", name))
	for _, v := range values {
		var n uint64
		if cur, ok := store.Load(v); ok {
			n = cur.(*atomic.Uint64).Load()
		}
		write(fmt.Sprintf("%s{%s=\"%s\"} %d\n", name, label, escapeLabelValue(v), n))
	}
}

// --- Inadimplência: o que o domínio expõe e quem chama -------------------
//
// Endpoint: /metrics do toca-financial (mesmo Handler das métricas de infra).
//
//	financial_overdue_amount_brl{tenant_id}              gauge     SetOverdueSnapshot
//	financial_overdue_charges{tenant_id}                 gauge     SetOverdueSnapshot
//	financial_collections_by_status{tenant_id,status}    gauge     SetCollectionStatusCounts
//	financial_collection_case_total_debt_brl{tenant_id}  gauge     SetOpenCollectionDebtBRL
//	financial_delinquency_tick_total{result}             counter   ObserveDelinquencyTick
//	financial_delinquency_tick_duration_seconds          histogram ObserveDelinquencyTick
//	financial_delinquency_manual_runs_total{result}      counter   IncDelinquencyManualRun
//	events_outbox_failed_total{event}                    counter   IncOutboxFailed (já existia em metrics.go)
//
// Quem deve chamar:
//   - DelinquencyService.ProcessOverdueCharges (tick horário): uma vez por
//     execução que pegou o advisory lock — ObserveDelinquencyTick("success"|
//     "error", segundos) no fim, e SetOverdueSnapshot por tenant varrido;
//   - DelinquencyService.ProcessManualTrigger: IncDelinquencyManualRun com o
//     mesmo enum, mais os gauges do tenant processado;
//   - CollectionService (dashboard/stats): SetCollectionStatusCounts com o
//     retrato completo do tenant e SetOpenCollectionDebtBRL só com casos
//     abertos (RESOLVED fora);
//   - pkg/events (drainer do outbox): IncOutboxFailed quando um evento esgota
//     as tentativas.
//
// Semântica: os gauges são retratos por tenant — quem para de chamar deixa o
// último valor exposto (o alerta de crescimento usa 24h de janela, então um
// tenant morto não gera série nova, só mantém a antiga). Cada processo expõe
// o que viu: em multi-réplica, agregue com `max by (tenant_id)` — `sum`
// contaria a mesma dívida uma vez por réplica.
