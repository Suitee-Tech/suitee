package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// writeHistogram emits a histogram in Prometheus text format using the
// caller-supplied write callback. Shared between the HTTP and business
// metric writers so the format is consistent and easy to test.
func writeHistogram(write func(string), name, help string, h *histogram, baseLabels map[string]string) {
	if h == nil {
		return
	}
	write(fmt.Sprintf("# HELP %s %s\n", name, help))
	write(fmt.Sprintf("# TYPE %s histogram\n", name))

	bounds := h.limits()
	var cum uint64
	for i := 0; i < len(bounds); i++ {
		cum += h.counts[i].Load()
		le := strconv.FormatFloat(bounds[i], 'f', -1, 64)
		write(fmt.Sprintf("%s_bucket%s %d\n", name, formatBusinessLabels(baseLabels, map[string]string{"le": le}), cum))
	}
	cum += h.counts[len(bounds)].Load()
	write(fmt.Sprintf("%s_bucket%s %d\n", name, formatBusinessLabels(baseLabels, map[string]string{"le": "+Inf"}), cum))

	sum := float64(h.sumNS.Load()) / float64(time.Second)
	total := h.total.Load()
	write(fmt.Sprintf("%s_sum%s %s\n", name, formatBusinessLabels(baseLabels, nil), strconv.FormatFloat(sum, 'f', -1, 64)))
	write(fmt.Sprintf("%s_count%s %d\n", name, formatBusinessLabels(baseLabels, nil), total))
}

// formatBusinessLabels mirrors formatEventLabels but takes a base map for
// arbitrary labels instead of the event-specific shape. Used by helpers
// that don't fit the {service,method,path,status} HTTP histogram schema.
func formatBusinessLabels(base, extra map[string]string) string {
	if len(base) == 0 && len(extra) == 0 {
		return ""
	}
	m := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		m[k] = v
	}
	for k, v := range extra {
		m[k] = v
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("{")
	for i, k := range keys {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(k)
		b.WriteString("=\"")
		b.WriteString(escapeLabelValue(m[k]))
		b.WriteString("\"")
	}
	b.WriteString("}")
	return b.String()
}

var (
	requestCounts       sync.Map
	requestDuration     sync.Map
	circuitBreakerState sync.Map
	dlqMessagesTotal    sync.Map
	outboxPublishErrors sync.Map      // event_name -> *atomic.Uint64
	outboxFailedTotal   sync.Map      // event_name -> *atomic.Uint64 (events that hit MaxOutboxAttempts)
	outboxPendingGauge  atomic.Int64  // current pending rows in events.outbox_events
	outboxFailedGauge   atomic.Int64  // current FAILED rows in events.outbox_events
	dlqPendingGauge     atomic.Int64  // current rows in events.dlq_events without replayed_at
	reconciliationRuns  atomic.Uint64 // total proactive reconciliation sweeps completed
	// rlsBackstopActive — 1 quando o pool de aplicação está sujeito a RLS e 0
	// quando ele bypassa (achado M1). Publicado no boot por pkg/database para
	// que "o RLS não protege nada" seja alertável, e não só um log.
	rlsBackstopActive    atomic.Int64
	reconciliationFixed  atomic.Uint64 // total projections re-applied across all runs
	reconciliationErrors atomic.Uint64 // total per-row errors across all runs
	danglingLeadRefs     atomic.Int64  // deals whose owner_id/customer_id no longer resolves to crm.leads

	// Audit P1 — signer drift detection counter, keyed by tenant only
	// (deal_id would explode cardinality). Incremented every time a
	// SyncDealSignersFromActiveContract call observes drift.
	signerProjectionDrift sync.Map // tenant_id -> *atomic.Uint64

	// Audit P1 — per-event handler latency. Histogram keyed by event name
	// (subscribers map 1:1 to events). Service label omitted because each
	// service exports its own /metrics endpoint scoped by job label.
	eventHandlerDuration sync.Map // event_name -> *histogram

	// Audit P1 — counter of auth bypass events for owner/admin profiles.
	// Helps detect when a root profile is doing unusual amounts of writes.
	authAdminBypass sync.Map // profile -> *atomic.Uint64

	// Audit P1 — histogram of seconds elapsed between webhook receipt and
	// processing. Lets operators spot processing lag (queue saturation,
	// retry storms).
	webhookSignatureEventAge = newHistogram()

	// Phase 3/I3 — signed PDF persistence pipeline visibility. Counters
	// answer "is the bucket draining the queue?" and "how often do we
	// fall back to Clicksign URLs?". Cardinality is bounded (reasons
	// are a fixed enum, no tenant labels), so safe to keep counter-shaped.
	signedPdfPersistSucceeded  atomic.Uint64 // total signed PDFs successfully persisted
	signedPdfPersistFailed     sync.Map      // reason -> *atomic.Uint64
	signedPdfFallbackClicksign atomic.Uint64 // times GetDownloadURL returned the external URL because the local copy was missing
	signedPdfBackfillRuns      atomic.Uint64 // BackfillUnstoredSignedPDFs invocations
	signedPdfBackfillRecovered atomic.Uint64 // total docs successfully re-persisted across backfill runs

	// Roleta de Leads P1 — assignment outcomes by tenant, method, and result.
	// Outcomes: ok | no_eligible | cas_exhausted | error. Cardinality is
	// bounded (tenants × {sequential, load_balance, weighted, priority} × 4).
	rouletteAssignmentsTotal sync.Map // "tenant_id\x1fmethod\x1foutcome" -> *atomic.Uint64

	// Roleta de Leads P1 — SLA action counters by tenant + action. Action ∈
	// {redirect, escalate, lost_capped, broker_paused}.
	rouletteSLAActionsTotal sync.Map // "tenant_id\x1faction" -> *atomic.Uint64
)

var buckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.2, 0.5, 1, 2, 5}

type labels struct {
	service string
	method  string
	path    string
	status  string
}

func (l labels) key() string {
	return l.service + "\x1f" + l.method + "\x1f" + l.path + "\x1f" + l.status
}

type histogram struct {
	counts []atomic.Uint64
	sumNS  atomic.Uint64
	total  atomic.Uint64
	// bounds são os limites superiores deste histograma. nil = usa os
	// buckets padrão de latência HTTP; jobs de longa duração (o tick de
	// inadimplência, por exemplo) passam limites próprios, senão toda
	// observação cai em +Inf.
	bounds []float64
}

func newHistogram() *histogram {
	return &histogram{
		counts: make([]atomic.Uint64, len(buckets)+1),
	}
}

func newHistogramWithBounds(bounds []float64) *histogram {
	if len(bounds) == 0 {
		return newHistogram()
	}
	own := make([]float64, len(bounds))
	copy(own, bounds)
	return &histogram{
		counts: make([]atomic.Uint64, len(own)+1),
		bounds: own,
	}
}

// limits devolve os limites superiores em uso — próprios ou os padrão.
func (h *histogram) limits() []float64 {
	if len(h.bounds) == 0 {
		return buckets
	}
	return h.bounds
}

func (h *histogram) observe(d time.Duration) {
	ns := uint64(d.Nanoseconds())
	h.sumNS.Add(ns)
	h.total.Add(1)

	sec := float64(d) / float64(time.Second)
	bounds := h.limits()
	i := sort.SearchFloat64s(bounds, sec)
	if i >= len(bounds) {
		i = len(bounds)
	}
	h.counts[i].Add(1)
}

func GinMiddleware(service string) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		d := time.Since(start)

		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		l := labels{
			service: service,
			method:  c.Request.Method,
			path:    path,
			status:  strconv.Itoa(c.Writer.Status()),
		}

		key := l.key()
		if v, ok := requestCounts.Load(key); ok {
			v.(*atomic.Uint64).Add(1)
		} else {
			var n atomic.Uint64
			n.Store(1)
			actual, loaded := requestCounts.LoadOrStore(key, &n)
			if loaded {
				actual.(*atomic.Uint64).Add(1)
			}
		}

		if v, ok := requestDuration.Load(key); ok {
			v.(*histogram).observe(d)
		} else {
			h := newHistogram()
			h.observe(d)
			actual, loaded := requestDuration.LoadOrStore(key, h)
			if loaded {
				actual.(*histogram).observe(d)
			}
		}
	}
}

func Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Type", "text/plain; version=0.0.4")
		c.Status(http.StatusOK)

		_, _ = c.Writer.WriteString("# HELP http_requests_total Total number of HTTP requests processed.\n")
		_, _ = c.Writer.WriteString("# TYPE http_requests_total counter\n")
		requestCounts.Range(func(k, v interface{}) bool {
			lbl := parseLabels(k.(string))
			n := v.(*atomic.Uint64).Load()
			_, _ = c.Writer.WriteString(fmt.Sprintf("http_requests_total%s %d\n", formatLabels(lbl, nil), n))
			return true
		})

		_, _ = c.Writer.WriteString("# HELP http_request_duration_seconds HTTP request duration in seconds.\n")
		_, _ = c.Writer.WriteString("# TYPE http_request_duration_seconds histogram\n")
		requestDuration.Range(func(k, v interface{}) bool {
			lbl := parseLabels(k.(string))
			h := v.(*histogram)

			var cum uint64
			for i := 0; i < len(buckets); i++ {
				cum += h.counts[i].Load()
				le := strconv.FormatFloat(buckets[i], 'f', -1, 64)
				_, _ = c.Writer.WriteString(fmt.Sprintf("http_request_duration_seconds_bucket%s %d\n", formatLabels(lbl, map[string]string{"le": le}), cum))
			}
			cum += h.counts[len(buckets)].Load()
			_, _ = c.Writer.WriteString(fmt.Sprintf("http_request_duration_seconds_bucket%s %d\n", formatLabels(lbl, map[string]string{"le": "+Inf"}), cum))

			sum := float64(h.sumNS.Load()) / float64(time.Second)
			total := h.total.Load()
			_, _ = c.Writer.WriteString(fmt.Sprintf("http_request_duration_seconds_sum%s %s\n", formatLabels(lbl, nil), strconv.FormatFloat(sum, 'f', -1, 64)))
			_, _ = c.Writer.WriteString(fmt.Sprintf("http_request_duration_seconds_count%s %d\n", formatLabels(lbl, nil), total))
			return true
		})

		_, _ = c.Writer.WriteString("# HELP circuit_breaker_state Current circuit breaker state: 0=closed,1=half_open,2=open.\n")
		_, _ = c.Writer.WriteString("# TYPE circuit_breaker_state gauge\n")
		circuitBreakerState.Range(func(k, v interface{}) bool {
			client := k.(string)
			state := v.(*atomic.Int64).Load()
			_, _ = c.Writer.WriteString(fmt.Sprintf("circuit_breaker_state{client=\"%s\"} %d\n", escapeLabelValue(client), state))
			return true
		})

		_, _ = c.Writer.WriteString("# HELP rabbitmq_dlq_messages_total Total DLQ messages persisted by the DLQ consumer.\n")
		_, _ = c.Writer.WriteString("# TYPE rabbitmq_dlq_messages_total counter\n")
		dlqMessagesTotal.Range(func(k, v interface{}) bool {
			event := k.(string)
			n := v.(*atomic.Uint64).Load()
			_, _ = c.Writer.WriteString(fmt.Sprintf("rabbitmq_dlq_messages_total{event=\"%s\"} %d\n", escapeLabelValue(event), n))
			return true
		})

		_, _ = c.Writer.WriteString("# HELP events_outbox_publish_errors_total Total publish failures observed by the outbox drainer (each retry increments).\n")
		_, _ = c.Writer.WriteString("# TYPE events_outbox_publish_errors_total counter\n")
		outboxPublishErrors.Range(func(k, v interface{}) bool {
			event := k.(string)
			n := v.(*atomic.Uint64).Load()
			_, _ = c.Writer.WriteString(fmt.Sprintf("events_outbox_publish_errors_total{event=\"%s\"} %d\n", escapeLabelValue(event), n))
			return true
		})

		_, _ = c.Writer.WriteString("# HELP events_outbox_failed_total Total outbox events that exhausted their retry budget (status=FAILED).\n")
		_, _ = c.Writer.WriteString("# TYPE events_outbox_failed_total counter\n")
		outboxFailedTotal.Range(func(k, v interface{}) bool {
			event := k.(string)
			n := v.(*atomic.Uint64).Load()
			_, _ = c.Writer.WriteString(fmt.Sprintf("events_outbox_failed_total{event=\"%s\"} %d\n", escapeLabelValue(event), n))
			return true
		})

		_, _ = c.Writer.WriteString("# HELP database_rls_backstop_active 1 when the application pool is subject to RLS policies, 0 when its role bypasses RLS (policies are inert).\n")
		_, _ = c.Writer.WriteString("# TYPE database_rls_backstop_active gauge\n")
		_, _ = c.Writer.WriteString(fmt.Sprintf("database_rls_backstop_active %d\n", rlsBackstopActive.Load()))

		_, _ = c.Writer.WriteString("# HELP events_outbox_pending Current number of events in events.outbox_events with status='PENDING' (refreshed each drainer cycle).\n")
		_, _ = c.Writer.WriteString("# TYPE events_outbox_pending gauge\n")
		_, _ = c.Writer.WriteString(fmt.Sprintf("events_outbox_pending %d\n", outboxPendingGauge.Load()))

		_, _ = c.Writer.WriteString("# HELP events_outbox_failed_gauge Current number of events in events.outbox_events with status='FAILED' (refreshed each drainer cycle).\n")
		_, _ = c.Writer.WriteString("# TYPE events_outbox_failed_gauge gauge\n")
		_, _ = c.Writer.WriteString(fmt.Sprintf("events_outbox_failed_gauge %d\n", outboxFailedGauge.Load()))

		_, _ = c.Writer.WriteString("# HELP events_dlq_pending Current number of unprocessed rows in events.dlq_events (replayed_at IS NULL).\n")
		_, _ = c.Writer.WriteString("# TYPE events_dlq_pending gauge\n")
		_, _ = c.Writer.WriteString(fmt.Sprintf("events_dlq_pending %d\n", dlqPendingGauge.Load()))

		_, _ = c.Writer.WriteString("# HELP reconciliation_runs_total Proactive reconciliation sweeps completed by the service.\n")
		_, _ = c.Writer.WriteString("# TYPE reconciliation_runs_total counter\n")
		_, _ = c.Writer.WriteString(fmt.Sprintf("reconciliation_runs_total %d\n", reconciliationRuns.Load()))

		_, _ = c.Writer.WriteString("# HELP reconciliation_projections_fixed_total Projections re-applied across all reconciliation sweeps (drift detected and healed).\n")
		_, _ = c.Writer.WriteString("# TYPE reconciliation_projections_fixed_total counter\n")
		_, _ = c.Writer.WriteString(fmt.Sprintf("reconciliation_projections_fixed_total %d\n", reconciliationFixed.Load()))

		_, _ = c.Writer.WriteString("# HELP reconciliation_errors_total Per-row errors observed during reconciliation sweeps.\n")
		_, _ = c.Writer.WriteString("# TYPE reconciliation_errors_total counter\n")
		_, _ = c.Writer.WriteString(fmt.Sprintf("reconciliation_errors_total %d\n", reconciliationErrors.Load()))

		_, _ = c.Writer.WriteString("# HELP dangling_lead_refs Current count of non-deleted deals whose owner_id or customer_id does not resolve to an active crm.leads row.\n")
		_, _ = c.Writer.WriteString("# TYPE dangling_lead_refs gauge\n")
		_, _ = c.Writer.WriteString(fmt.Sprintf("dangling_lead_refs %d\n", danglingLeadRefs.Load()))

		_, _ = c.Writer.WriteString("# HELP signer_projection_drift_total Times the deal signer projection was found stale against the source envelope and re-synced.\n")
		_, _ = c.Writer.WriteString("# TYPE signer_projection_drift_total counter\n")
		signerProjectionDrift.Range(func(k, v interface{}) bool {
			tenant := k.(string)
			n := v.(*atomic.Uint64).Load()
			_, _ = c.Writer.WriteString(fmt.Sprintf("signer_projection_drift_total{tenant_id=\"%s\"} %d\n", escapeLabelValue(tenant), n))
			return true
		})

		_, _ = c.Writer.WriteString("# HELP event_handler_duration_seconds Latency of event subscribers per event name.\n")
		_, _ = c.Writer.WriteString("# TYPE event_handler_duration_seconds histogram\n")
		eventHandlerDuration.Range(func(k, v interface{}) bool {
			event := k.(string)
			h := v.(*histogram)
			extra := map[string]string{"event": event}

			var cum uint64
			for i := 0; i < len(buckets); i++ {
				cum += h.counts[i].Load()
				le := strconv.FormatFloat(buckets[i], 'f', -1, 64)
				_, _ = c.Writer.WriteString(fmt.Sprintf("event_handler_duration_seconds_bucket%s %d\n", formatEventLabels(extra, map[string]string{"le": le}), cum))
			}
			cum += h.counts[len(buckets)].Load()
			_, _ = c.Writer.WriteString(fmt.Sprintf("event_handler_duration_seconds_bucket%s %d\n", formatEventLabels(extra, map[string]string{"le": "+Inf"}), cum))

			sum := float64(h.sumNS.Load()) / float64(time.Second)
			total := h.total.Load()
			_, _ = c.Writer.WriteString(fmt.Sprintf("event_handler_duration_seconds_sum%s %s\n", formatEventLabels(extra, nil), strconv.FormatFloat(sum, 'f', -1, 64)))
			_, _ = c.Writer.WriteString(fmt.Sprintf("event_handler_duration_seconds_count%s %d\n", formatEventLabels(extra, nil), total))
			return true
		})

		_, _ = c.Writer.WriteString("# HELP auth_admin_bypass_total Counter of permission bypasses granted to root profiles (owner/admin).\n")
		_, _ = c.Writer.WriteString("# TYPE auth_admin_bypass_total counter\n")
		authAdminBypass.Range(func(k, v interface{}) bool {
			profile := k.(string)
			n := v.(*atomic.Uint64).Load()
			_, _ = c.Writer.WriteString(fmt.Sprintf("auth_admin_bypass_total{profile=\"%s\"} %d\n", escapeLabelValue(profile), n))
			return true
		})

		// Phase 3/I3 — signed PDF pipeline health. Pair `succeeded` with
		// the per-reason `failed` counter and the `fallback_clicksign`
		// gauge to answer the dashboard question "is the Clicksign
		// integration healthy?".
		_, _ = c.Writer.WriteString("# HELP signed_pdf_persist_succeeded_total Signed PDFs successfully persisted to the toca-documents bucket.\n")
		_, _ = c.Writer.WriteString("# TYPE signed_pdf_persist_succeeded_total counter\n")
		_, _ = c.Writer.WriteString(fmt.Sprintf("signed_pdf_persist_succeeded_total %d\n", signedPdfPersistSucceeded.Load()))

		_, _ = c.Writer.WriteString("# HELP signed_pdf_persist_failed_total Signed PDF persist failures, by reason (download|validation|storage|read|request|other).\n")
		_, _ = c.Writer.WriteString("# TYPE signed_pdf_persist_failed_total counter\n")
		signedPdfPersistFailed.Range(func(k, v interface{}) bool {
			reason := k.(string)
			n := v.(*atomic.Uint64).Load()
			_, _ = c.Writer.WriteString(fmt.Sprintf("signed_pdf_persist_failed_total{reason=\"%s\"} %d\n", escapeLabelValue(reason), n))
			return true
		})

		_, _ = c.Writer.WriteString("# HELP signed_pdf_fallback_clicksign_total Times GetDownloadURL returned the external Clicksign URL because the local copy was missing.\n")
		_, _ = c.Writer.WriteString("# TYPE signed_pdf_fallback_clicksign_total counter\n")
		_, _ = c.Writer.WriteString(fmt.Sprintf("signed_pdf_fallback_clicksign_total %d\n", signedPdfFallbackClicksign.Load()))

		_, _ = c.Writer.WriteString("# HELP signed_pdf_backfill_runs_total Invocations of BackfillUnstoredSignedPDFs (admin endpoint or future cron).\n")
		_, _ = c.Writer.WriteString("# TYPE signed_pdf_backfill_runs_total counter\n")
		_, _ = c.Writer.WriteString(fmt.Sprintf("signed_pdf_backfill_runs_total %d\n", signedPdfBackfillRuns.Load()))

		_, _ = c.Writer.WriteString("# HELP signed_pdf_backfill_recovered_total Stuck signed PDFs successfully re-persisted across all backfill runs.\n")
		_, _ = c.Writer.WriteString("# TYPE signed_pdf_backfill_recovered_total counter\n")
		_, _ = c.Writer.WriteString(fmt.Sprintf("signed_pdf_backfill_recovered_total %d\n", signedPdfBackfillRecovered.Load()))

		_, _ = c.Writer.WriteString("# HELP webhook_signature_event_age_seconds Seconds between webhook receipt and processing.\n")
		_, _ = c.Writer.WriteString("# TYPE webhook_signature_event_age_seconds histogram\n")
		{
			h := webhookSignatureEventAge
			var cum uint64
			for i := 0; i < len(buckets); i++ {
				cum += h.counts[i].Load()
				le := strconv.FormatFloat(buckets[i], 'f', -1, 64)
				_, _ = c.Writer.WriteString(fmt.Sprintf("webhook_signature_event_age_seconds_bucket{le=\"%s\"} %d\n", le, cum))
			}
			cum += h.counts[len(buckets)].Load()
			_, _ = c.Writer.WriteString(fmt.Sprintf("webhook_signature_event_age_seconds_bucket{le=\"+Inf\"} %d\n", cum))
			sum := float64(h.sumNS.Load()) / float64(time.Second)
			_, _ = c.Writer.WriteString(fmt.Sprintf("webhook_signature_event_age_seconds_sum %s\n", strconv.FormatFloat(sum, 'f', -1, 64)))
			_, _ = c.Writer.WriteString(fmt.Sprintf("webhook_signature_event_age_seconds_count %d\n", h.total.Load()))
		}

		_, _ = c.Writer.WriteString("# HELP rate_limit_rejected_total Total rate-limit rejections at the gateway, by tenant/IP and endpoint group.\n")
		_, _ = c.Writer.WriteString("# TYPE rate_limit_rejected_total counter\n")
		rateLimitRejectedTotal.Range(func(k, v interface{}) bool {
			key := k.(string)
			var who, group string
			for i := 0; i < len(key); i++ {
				if key[i] == 0x1f {
					who = key[:i]
					group = key[i+1:]
					break
				}
			}
			n := v.(*atomic.Uint64).Load()
			_, _ = c.Writer.WriteString(fmt.Sprintf("rate_limit_rejected_total{key=\"%s\",endpoint_group=\"%s\"} %d\n",
				escapeLabelValue(who), escapeLabelValue(group), n))
			return true
		})

		_, _ = c.Writer.WriteString("# HELP roulette_assignments_total Outcomes of AssignByRoulette calls grouped by distribution method.\n")
		_, _ = c.Writer.WriteString("# TYPE roulette_assignments_total counter\n")
		rouletteAssignmentsTotal.Range(func(k, v interface{}) bool {
			key := k.(string)
			parts := strings.SplitN(key, "\x1f", 3)
			if len(parts) != 3 {
				return true
			}
			n := v.(*atomic.Uint64).Load()
			_, _ = c.Writer.WriteString(fmt.Sprintf(
				"roulette_assignments_total{tenant_id=\"%s\",method=\"%s\",outcome=\"%s\"} %d\n",
				escapeLabelValue(parts[0]), escapeLabelValue(parts[1]), escapeLabelValue(parts[2]), n,
			))
			return true
		})

		_, _ = c.Writer.WriteString("# HELP roulette_sla_actions_total SLA worker actions triggered per tenant (redirect, escalate, lost_capped, broker_paused).\n")
		_, _ = c.Writer.WriteString("# TYPE roulette_sla_actions_total counter\n")
		rouletteSLAActionsTotal.Range(func(k, v interface{}) bool {
			key := k.(string)
			parts := strings.SplitN(key, "\x1f", 2)
			if len(parts) != 2 {
				return true
			}
			n := v.(*atomic.Uint64).Load()
			_, _ = c.Writer.WriteString(fmt.Sprintf(
				"roulette_sla_actions_total{tenant_id=\"%s\",action=\"%s\"} %d\n",
				escapeLabelValue(parts[0]), escapeLabelValue(parts[1]), n,
			))
			return true
		})

		// Business metrics — MRR, conversion funnels, time-to-* — emitted
		// alongside infra metrics so the same /metrics endpoint feeds the
		// product KPI dashboards.
		writeBusinessMetrics(func(s string) { _, _ = c.Writer.WriteString(s) })
	}
}

// formatEventLabels renders a Prometheus label set for the event-handler
// histogram. Kept separate from formatLabels because the latter assumes
// the {service,method,path,status} schema of HTTP requests.
func formatEventLabels(base, extra map[string]string) string {
	m := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		m[k] = v
	}
	for k, v := range extra {
		m[k] = v
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("{")
	for i, k := range keys {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(k)
		b.WriteString("=\"")
		b.WriteString(escapeLabelValue(m[k]))
		b.WriteString("\"")
	}
	b.WriteString("}")
	return b.String()
}

func SetCircuitBreakerState(client string, state int64) {
	if client == "" {
		return
	}

	if v, ok := circuitBreakerState.Load(client); ok {
		v.(*atomic.Int64).Store(state)
		return
	}

	var n atomic.Int64
	n.Store(state)
	actual, loaded := circuitBreakerState.LoadOrStore(client, &n)
	if loaded {
		actual.(*atomic.Int64).Store(state)
	}
}

func IncDLQMessagesTotal(event string) {
	if event == "" {
		return
	}

	if v, ok := dlqMessagesTotal.Load(event); ok {
		v.(*atomic.Uint64).Add(1)
		return
	}

	var n atomic.Uint64
	n.Store(1)
	actual, loaded := dlqMessagesTotal.LoadOrStore(event, &n)
	if loaded {
		actual.(*atomic.Uint64).Add(1)
	}
}

// IncOutboxPublishError increments the per-event counter of publish attempts
// that failed during outbox drain. Called by pkg/events on every failure
// before the row is rescheduled with backoff.
func IncOutboxPublishError(event string) {
	incEventCounter(&outboxPublishErrors, event)
}

// IncOutboxFailed increments the per-event counter of outbox rows that
// exhausted their retry budget (transitioned to status='FAILED'). A non-zero
// rate here is an operator alarm — the underlying issue needs investigation
// before manual replay.
func IncOutboxFailed(event string) {
	incEventCounter(&outboxFailedTotal, event)
}

// SetRLSBackstopActive publica se o pool de aplicação está sujeito a RLS
// (1) ou se conecta com um role que o ignora (0). Achado M1: sem isso o único
// sinal de que as políticas são inertes era uma linha de log no boot.
func SetRLSBackstopActive(active bool) {
	if active {
		rlsBackstopActive.Store(1)
		return
	}
	rlsBackstopActive.Store(0)
}

// SetOutboxPending updates the gauge of currently pending outbox rows.
// Called from the drainer each cycle so /metrics reflects backlog without
// requiring Prometheus to query Postgres directly.
func SetOutboxPending(value int64) {
	outboxPendingGauge.Store(value)
}

// SetOutboxFailedGauge updates the gauge of FAILED outbox rows.
func SetOutboxFailedGauge(value int64) {
	outboxFailedGauge.Store(value)
}

// SetDLQPending updates the gauge of un-replayed rows in events.dlq_events.
func SetDLQPending(value int64) {
	dlqPendingGauge.Store(value)
}

// IncReconciliationRun is called once per reconciliation sweep completion.
func IncReconciliationRun() {
	reconciliationRuns.Add(1)
}

// AddReconciliationFixed records how many projections were healed in a
// single sweep. Pass 0 when a sweep finds nothing to fix.
func AddReconciliationFixed(delta uint64) {
	if delta == 0 {
		return
	}
	reconciliationFixed.Add(delta)
}

// AddReconciliationErrors records per-row errors observed in a sweep.
// Top-level errors that abort the sweep before iterating rows are logged
// instead.
func AddReconciliationErrors(delta uint64) {
	if delta == 0 {
		return
	}
	reconciliationErrors.Add(delta)
}

// SetDanglingLeadRefs sets the gauge to the current sweep count.
// Soft-FK detection in toca-deal: detected deals where owner_id or
// customer_id no longer maps to an active crm.leads row.
func SetDanglingLeadRefs(value int64) {
	danglingLeadRefs.Store(value)
}

// IncSignerProjectionDrift increments the per-tenant counter every time
// the signer projection of a Deal is found stale against the source
// SignatureEnvelope and re-synced. Empty tenant is dropped (caller bug).
func IncSignerProjectionDrift(tenantID string) {
	if tenantID == "" {
		return
	}
	incEventCounter(&signerProjectionDrift, tenantID)
}

// Phase 3/I3 — signed PDF pipeline counters. See
// `roadmap.md` and `docs/domain/documents-status-mapping.md` for the
// dashboard intent — the Grafana board this powers is the primary
// signal for "is the Clicksign integration healthy?".

// IncSignedPdfPersistSucceeded is called every time a signed PDF
// successfully lands in the bucket. Pair-counter with the failure
// counter — `succeeded / (succeeded + failed)` is the persistence
// success rate operators watch.
func IncSignedPdfPersistSucceeded() {
	signedPdfPersistSucceeded.Add(1)
}

// IncSignedPdfPersistFailed is called when PersistSignedContractPDF
// bails out. `reason` is the bounded enum: `download`, `validation`,
// `storage`, `read`, `request`, `other`. Empty reason gets bucketed as
// `other` so we never silently drop failures.
func IncSignedPdfPersistFailed(reason string) {
	if reason == "" {
		reason = "other"
	}
	incEventCounter(&signedPdfPersistFailed, reason)
}

// IncSignedPdfFallbackClicksign is called when GetDownloadURL returned
// the external Clicksign URL because the local Supabase copy didn't
// exist yet. A non-zero rate here means the async subscriber is
// lagging — pair with the success counter to spot persistent issues.
func IncSignedPdfFallbackClicksign() {
	signedPdfFallbackClicksign.Add(1)
}

// IncSignedPdfBackfillRun is called once per BackfillUnstoredSignedPDFs
// invocation. Combined with the recovered counter it answers "is the
// backfill actually fixing things?".
func IncSignedPdfBackfillRun() {
	signedPdfBackfillRuns.Add(1)
}

// AddSignedPdfBackfillRecovered records how many stuck signed PDFs
// were successfully re-persisted in one backfill run. Pass 0 when a
// sweep finds nothing to fix.
func AddSignedPdfBackfillRecovered(delta uint64) {
	if delta == 0 {
		return
	}
	signedPdfBackfillRecovered.Add(delta)
}

// ObserveEventHandlerDuration records latency of an event-subscriber call,
// keyed by the event name. Used by pkg/events subscriber wrappers; safe to
// call with d=0 (no-op since the histogram drops zero observations).
func ObserveEventHandlerDuration(event string, d time.Duration) {
	if event == "" || d <= 0 {
		return
	}
	var h *histogram
	if v, ok := eventHandlerDuration.Load(event); ok {
		h = v.(*histogram)
	} else {
		h = newHistogram()
		actual, loaded := eventHandlerDuration.LoadOrStore(event, h)
		if loaded {
			h = actual.(*histogram)
		}
	}
	h.observe(d)
}

// IncAuthAdminBypass increments the counter every time the permission
// middleware short-circuits because the requester carries a root profile
// (owner/admin). Spikes here can indicate a compromised root account or
// runaway automation. Profile is lowercased to keep cardinality bounded.
func IncAuthAdminBypass(profile string) {
	if profile == "" {
		return
	}
	incEventCounter(&authAdminBypass, strings.ToLower(profile))
}

// ObserveWebhookSignatureEventAge records the gap between the webhook's
// received_at timestamp and the moment toca-signature actually processes
// it. p99 spikes hint at consumer backlog.
func ObserveWebhookSignatureEventAge(d time.Duration) {
	if d <= 0 {
		return
	}
	webhookSignatureEventAge.observe(d)
}

// IncRouletteAssignment records the outcome of an AssignByRoulette call.
// Labels (lowercased, bounded cardinality):
//   - tenant_id: the tenant the roulette ran for
//   - method: sequential | load_balance | weighted | priority | none
//   - outcome: ok | no_eligible | cas_exhausted | error
//
// Empty inputs are coerced to "unknown" so an instrumentation bug never
// silently drops counts.
func IncRouletteAssignment(tenantID, method, outcome string) {
	if tenantID == "" {
		tenantID = "unknown"
	}
	if method == "" {
		method = "unknown"
	}
	if outcome == "" {
		outcome = "unknown"
	}
	key := tenantID + "\x1f" + strings.ToLower(method) + "\x1f" + strings.ToLower(outcome)
	incEventCounter(&rouletteAssignmentsTotal, key)
}

// IncRouletteSLAAction records an SLA-driven action triggered by the worker.
// Action ∈ {redirect, escalate, lost_capped, broker_paused}. Tenant is
// always present (the worker iterates per tenant).
func IncRouletteSLAAction(tenantID, action string) {
	if tenantID == "" {
		tenantID = "unknown"
	}
	if action == "" {
		return
	}
	key := tenantID + "\x1f" + strings.ToLower(action)
	incEventCounter(&rouletteSLAActionsTotal, key)
}

// IncRateLimitRejected increments the counter every time the gateway
// rate-limit middleware blocks a request. Labels: tenant_id (or "ip:..."
// when no tenant header was sent) and endpoint_group (auth/ai/general).
// Helps detect tenants close to abuse limits or noisy IPs.
func IncRateLimitRejected(tenantOrIP, endpointGroup string) {
	if tenantOrIP == "" {
		tenantOrIP = "unknown"
	}
	if endpointGroup == "" {
		endpointGroup = "general"
	}
	key := tenantOrIP + "\x1f" + endpointGroup
	incEventCounter(&rateLimitRejectedTotal, key)
}

var rateLimitRejectedTotal sync.Map // "tenantOrIP\x1fgroup" -> *atomic.Uint64

func incEventCounter(store *sync.Map, event string) {
	if event == "" {
		return
	}
	if v, ok := store.Load(event); ok {
		v.(*atomic.Uint64).Add(1)
		return
	}
	var n atomic.Uint64
	n.Store(1)
	actual, loaded := store.LoadOrStore(event, &n)
	if loaded {
		actual.(*atomic.Uint64).Add(1)
	}
}

func parseLabels(key string) labels {
	parts := strings.Split(key, "\x1f")
	if len(parts) != 4 {
		return labels{}
	}
	return labels{
		service: parts[0],
		method:  parts[1],
		path:    parts[2],
		status:  parts[3],
	}
}

func formatLabels(l labels, extra map[string]string) string {
	m := map[string]string{
		"service": l.service,
		"method":  l.method,
		"path":    l.path,
		"status":  l.status,
	}
	for k, v := range extra {
		m[k] = v
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("{")
	for i, k := range keys {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(k)
		b.WriteString("=\"")
		b.WriteString(escapeLabelValue(m[k]))
		b.WriteString("\"")
	}
	b.WriteString("}")
	return b.String()
}

func escapeLabelValue(v string) string {
	v = strings.ReplaceAll(v, "\\", "\\\\")
	v = strings.ReplaceAll(v, "\n", "\\n")
	v = strings.ReplaceAll(v, "\"", "\\\"")
	return v
}
