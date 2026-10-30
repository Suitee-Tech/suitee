// Package audit writes append-only forensic records of mutations across
// services. Designed for compliance (LGPD Art. 37/38) and incident
// forensics — every write to audit.events is preserved for 5 years by
// the table's monthly partitioning + retention policy.
//
// Typical usage from a service method:
//
//	audit.Record(ctx, audit.Entry{
//	    Action:     "update",
//	    EntityType: "Deal",
//	    EntityID:   d.ID,
//	    Before:     audit.Snapshot(oldDeal, "status", "value"),
//	    After:      audit.Snapshot(newDeal, "status", "value"),
//	})
//
// The helper pulls tenant_id, actor_id, request_id and trace_id from the
// context automatically. PII fields (email/phone/cpf) MUST be filtered
// out of Before/After by the caller — usually via pkg/pii.MaskForLog
// before passing to Snapshot — because audit.events is retained for 5y
// and a leak there persists past LGPD anonymization requests.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/logger"
	"go.opentelemetry.io/otel/trace"
)

// Entry is the audit record payload. tenant_id, actor_id, request_id and
// trace_id are filled by Record from the context — callers only supply
// the domain-specific fields.
type Entry struct {
	Action     string         // "create" | "update" | "delete" | "soft_delete" | domain-specific (e.g. "contract.sign")
	EntityType string         // canonical domain noun, capitalised
	EntityID   string         // primary key as string
	Before     map[string]any // pre-mutation snapshot, optional
	After      map[string]any // post-mutation snapshot, optional
	Metadata   map[string]any // extra context (e.g. "source": "webhook")
}

var (
	defaultRecorder *Recorder
	defaultMu       sync.RWMutex
	// testSink, quando definido, recebe cada Entry ANTES de qualquer acesso a
	// banco. Existe para os testes de regressão do P2 (M4/M5) provarem que a
	// mutação sensível é auditada sem precisar de Postgres — e sem depender de
	// parsing de log. Nunca é usado em produção.
	testSink func(Entry)
)

// SetTestSink instala (ou limpa) o coletor usado pelos testes. Devolve uma função
// de limpeza para o `t.Cleanup`.
func SetTestSink(fn func(Entry)) func() {
	defaultMu.Lock()
	prev := testSink
	testSink = fn
	defaultMu.Unlock()
	return func() {
		defaultMu.Lock()
		testSink = prev
		defaultMu.Unlock()
	}
}

// Recorder writes audit entries against a Postgres pool. Single instance
// per process — services set the global via SetDefault at boot.
type Recorder struct {
	db *database.PostgresDB
}

const postCommitRecordTimeout = 3 * time.Second

// NewRecorder returns a Recorder bound to the given pool. Pass nil to
// disable persistence (the package falls back to structured logging
// only, which keeps tests + dev environments hassle-free).
func NewRecorder(db *database.PostgresDB) *Recorder {
	return &Recorder{db: db}
}

// SetDefault wires the global Recorder used by package-level Record().
// Idempotent and safe to call from each service's main.go.
func SetDefault(r *Recorder) {
	defaultMu.Lock()
	defaultRecorder = r
	defaultMu.Unlock()
}

// Record persists one audit entry using the global Recorder. Errors are
// logged but never returned — auditing must not block the business path.
// When no Recorder is wired (tests, dev), only the structured log is
// produced.
func Record(ctx context.Context, e Entry) {
	// audit.events is written through the service pool, outside the tenant SQL
	// transaction. Queue the insert until the managed transaction commits so a
	// later financial failure cannot leave a forensic event for data that was
	// rolled back. The detached context preserves tenant/actor/trace values even
	// if the HTTP request is cancelled immediately after commit.
	if database.DeferUntilCommit(ctx, func() {
		// The originating request may already be cancelled after commit, but an
		// unavailable audit database must not hold a successfully committed
		// financial response forever. Preserve request values while bounding this
		// best-effort observer.
		auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), postCommitRecordTimeout)
		defer cancel()
		recordNow(auditCtx, e)
	}) {
		return
	}
	recordNow(ctx, e)
}

func recordNow(ctx context.Context, e Entry) {
	defaultMu.RLock()
	r := defaultRecorder
	sink := testSink
	defaultMu.RUnlock()

	if sink != nil {
		sink(e)
	}

	tenantID, _ := pkgctx.GetTenantID(ctx)
	actorID, _ := pkgctx.GetAccountID(ctx)
	requestID := pkgctx.GetRequestID(ctx)
	var traceID string
	if sc := trace.SpanFromContext(ctx).SpanContext(); sc.IsValid() {
		traceID = sc.TraceID().String()
	}

	if e.Metadata == nil {
		e.Metadata = map[string]any{}
	}
	if requestID != "" {
		e.Metadata["request_id"] = requestID
	}
	if traceID != "" {
		e.Metadata["trace_id"] = traceID
	}

	// Always log — operators can grep the structured log even when the
	// DB is unavailable. Tagged with event=audit.recorded so the audit
	// trail is separable from generic info logs.
	logger.InfoContext(ctx, "audit.recorded",
		"event", "audit.recorded",
		"audit_action", e.Action,
		"entity_type", e.EntityType,
		"entity_id", e.EntityID,
	)

	if r == nil || r.db == nil || r.db.ServicePool == nil {
		return
	}
	beforeJSON, _ := json.Marshal(e.Before)
	afterJSON, _ := json.Marshal(e.After)
	metaJSON, _ := json.Marshal(e.Metadata)

	actorType := "user"
	if actorID == "" {
		actorType = "service"
	}

	var tid any = nil
	if tenantID != "" {
		tid = tenantID
	}
	var aid any = nil
	if actorID != "" {
		aid = actorID
	}

	const query = `
		INSERT INTO audit.events
		    (tenant_id, actor_id, actor_type, action, entity_type, entity_id, before, after, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8::jsonb, $9::jsonb)
	`
	if _, err := r.db.ServicePool.Exec(ctx, query,
		tid, aid, actorType, e.Action, e.EntityType, e.EntityID,
		string(beforeJSON), string(afterJSON), string(metaJSON),
	); err != nil {
		// Audit failure is logged but never returned. A flaky audit
		// pipeline shouldn't break user-facing writes; the structured
		// log emitted above is the recovery hook (operators can replay
		// from log into audit.events if needed).
		logger.ErrorContext(ctx, "audit insert failed",
			"event", "audit.write_failed",
			"audit_action", e.Action,
			"entity_type", e.EntityType,
			"entity_id", e.EntityID,
			"error", err.Error(),
		)
	}
}

// LogEntry is a stored row from audit.events. Returned by ListByEntity so
// callers can render an audit trail without rebuilding the columns by
// hand. Timestamps are RFC3339 strings to keep the JSON payload
// predictable across the wire.
type LogEntry struct {
	ID         string         `json:"id"`
	TenantID   string         `json:"tenant_id"`
	ActorID    string         `json:"actor_id,omitempty"`
	ActorType  string         `json:"actor_type"`
	Action     string         `json:"action"`
	EntityType string         `json:"entity_type"`
	EntityID   string         `json:"entity_id"`
	Before     map[string]any `json:"before,omitempty"`
	After      map[string]any `json:"after,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	CreatedAt  string         `json:"created_at"`
}

// ListByEntity returns the audit trail for a single entity, newest first,
// scoped to the active tenant. ServicePool bypasses RLS, so tenant
// isolation is enforced explicitly here — callers MUST pass a non-empty
// tenantID (the handler resolves it from the request context). limit
// caps the page size; pass <=0 for the default of 100.
func ListByEntity(ctx context.Context, tenantID, entityType, entityID string, limit int) ([]LogEntry, error) {
	defaultMu.RLock()
	r := defaultRecorder
	defaultMu.RUnlock()
	if r == nil || r.db == nil || r.db.ServicePool == nil {
		return []LogEntry{}, nil
	}
	if tenantID == "" || entityType == "" || entityID == "" {
		return []LogEntry{}, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	const query = `
		SELECT id, tenant_id, actor_id, actor_type, action, entity_type, entity_id,
		       before, after, metadata, created_at
		  FROM audit.events
		 WHERE tenant_id = $1::uuid AND entity_type = $2 AND entity_id = $3
		 ORDER BY created_at DESC
		 LIMIT $4
	`
	rows, err := r.db.ServicePool.Query(ctx, query, tenantID, entityType, entityID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]LogEntry, 0, limit)
	for rows.Next() {
		var (
			rec                              LogEntry
			actorID                          *string
			beforeRaw, afterRaw, metadataRaw []byte
			createdAt                        interface{}
		)
		if err := rows.Scan(
			&rec.ID, &rec.TenantID, &actorID, &rec.ActorType,
			&rec.Action, &rec.EntityType, &rec.EntityID,
			&beforeRaw, &afterRaw, &metadataRaw, &createdAt,
		); err != nil {
			return nil, err
		}
		if actorID != nil {
			rec.ActorID = *actorID
		}
		_ = json.Unmarshal(beforeRaw, &rec.Before)
		_ = json.Unmarshal(afterRaw, &rec.After)
		_ = json.Unmarshal(metadataRaw, &rec.Metadata)
		if ts, ok := createdAt.(interface{ Format(string) string }); ok {
			rec.CreatedAt = ts.Format("2006-01-02T15:04:05Z07:00")
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// ListFilters parameterizes List. All filters are optional — empty
// strings are treated as "no constraint" — and combine with AND.
// Cursor pagination via CreatedBefore: pass the CreatedAt of the last
// item from the previous page (RFC3339) to fetch the next page without
// risk of skipping or repeating rows when new events arrive between
// requests.
type ListFilters struct {
	// TenantID — empty means cross-tenant (auditor profile). The caller
	// is responsible for gating this via permission check; the package
	// trusts the caller.
	TenantID string
	// Action filter — exact match (e.g. "auth.banking_updated"). Use
	// ActionPrefix for partial matches.
	Action       string
	ActionPrefix string // e.g. "auth." to fetch all auth events
	EntityType   string
	EntityID     string
	ActorID      string
	// From/To — RFC3339 timestamps for the created_at range. Empty =
	// no bound. Inclusive on both ends.
	From string
	To   string
	// CreatedBefore — cursor para a próxima página. Quando setado,
	// retorna apenas eventos com created_at < CreatedBefore (paginação
	// cursor-based em vez de offset, que poderia pular/repetir eventos
	// com inserções concorrentes).
	CreatedBefore string
	// Limit — 1..500. Default 100.
	Limit int
}

// buildListQuery monta o SQL de List a partir dos filtros. Exposto
// internamente para testes — não exporta porque a forma do SQL é
// detalhe de implementação que pode mudar.
//
// Retorna (query, args). Caller passa direto pro QueryContext.
func buildListQuery(f ListFilters) (string, []any) {
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	args := make([]any, 0, 8)
	conds := make([]string, 0, 8)
	addCond := func(sqlTpl string, val any) {
		args = append(args, val)
		conds = append(conds, fmt.Sprintf(sqlTpl, len(args)))
	}

	if f.TenantID != "" {
		addCond("tenant_id = $%d::uuid", f.TenantID)
	}
	if f.Action != "" {
		addCond("action = $%d", f.Action)
	}
	if f.ActionPrefix != "" {
		addCond("action LIKE $%d", f.ActionPrefix+"%")
	}
	if f.EntityType != "" {
		addCond("entity_type = $%d", f.EntityType)
	}
	if f.EntityID != "" {
		addCond("entity_id = $%d", f.EntityID)
	}
	if f.ActorID != "" {
		addCond("actor_id = $%d::uuid", f.ActorID)
	}
	if f.From != "" {
		addCond("created_at >= $%d::timestamptz", f.From)
	}
	if f.To != "" {
		addCond("created_at <= $%d::timestamptz", f.To)
	}
	if f.CreatedBefore != "" {
		addCond("created_at < $%d::timestamptz", f.CreatedBefore)
	}

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + joinConds(conds)
	}
	args = append(args, limit)
	limitPlaceholder := len(args)

	return fmt.Sprintf(`
		SELECT id, tenant_id, actor_id, actor_type, action, entity_type, entity_id,
		       before, after, metadata, created_at
		  FROM audit.events
		  %s
		 ORDER BY created_at DESC
		 LIMIT $%d
	`, where, limitPlaceholder), args
}

// List returns audit entries matching ListFilters, newest first.
// Bypasses RLS via ServicePool — caller MUST gate this with a
// permission check (system:view_audit_trail) before invoking.
//
// Performance: queries use the (tenant_id, created_at DESC) index when
// TenantID is set; cross-tenant queries fall back to a sequential scan
// of the active partition, which is fine for the audit volume but
// requires a reasonable date range or low Limit to avoid hot pages.
func List(ctx context.Context, f ListFilters) ([]LogEntry, error) {
	defaultMu.RLock()
	r := defaultRecorder
	defaultMu.RUnlock()
	if r == nil || r.db == nil || r.db.ServicePool == nil {
		return []LogEntry{}, nil
	}

	query, args := buildListQuery(f)

	rows, err := r.db.ServicePool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// O último argumento é sempre o LIMIT (buildListQuery o appendou
	// como último elemento). Usamos como capacity hint para o slice.
	limitHint := 100
	if n, ok := args[len(args)-1].(int); ok {
		limitHint = n
	}
	out := make([]LogEntry, 0, limitHint)
	for rows.Next() {
		var (
			rec                              LogEntry
			tenantID, actorID                *string
			beforeRaw, afterRaw, metadataRaw []byte
			createdAt                        interface{}
		)
		if err := rows.Scan(
			&rec.ID, &tenantID, &actorID, &rec.ActorType,
			&rec.Action, &rec.EntityType, &rec.EntityID,
			&beforeRaw, &afterRaw, &metadataRaw, &createdAt,
		); err != nil {
			return nil, err
		}
		if tenantID != nil {
			rec.TenantID = *tenantID
		}
		if actorID != nil {
			rec.ActorID = *actorID
		}
		_ = json.Unmarshal(beforeRaw, &rec.Before)
		_ = json.Unmarshal(afterRaw, &rec.After)
		_ = json.Unmarshal(metadataRaw, &rec.Metadata)
		if ts, ok := createdAt.(interface{ Format(string) string }); ok {
			rec.CreatedAt = ts.Format("2006-01-02T15:04:05Z07:00")
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// joinConds — concat de cond strings com " AND " sem importar strings
// só pra isso. Evita allocations excessivas no caminho quente.
func joinConds(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for i := 1; i < len(parts); i++ {
		out += " AND " + parts[i]
	}
	return out
}

// Snapshot picks the named fields from a struct (via JSON marshalling)
// and returns them as a map suitable for Before/After. This keeps audit
// blobs small and predictable instead of dumping the whole aggregate.
//
// Why JSON: avoids reflection over arbitrary struct tags + keeps the
// shape stable across Go versions. Performance is fine for audit
// purposes (we're talking dozens of writes per request, not thousands).
func Snapshot(v any, fields ...string) map[string]any {
	if v == nil {
		return nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return map[string]any{"_snapshot_error": err.Error()}
	}
	var full map[string]any
	if err := json.Unmarshal(raw, &full); err != nil {
		return map[string]any{"_snapshot_error": err.Error()}
	}
	if len(fields) == 0 {
		return full
	}
	picked := make(map[string]any, len(fields))
	for _, f := range fields {
		if v, ok := full[f]; ok {
			picked[f] = v
		}
	}
	return picked
}

// Diff returns only fields whose value changed between two snapshots.
// Convenient for update audit entries when callers only want the deltas.
func Diff(before, after map[string]any) map[string]any {
	if len(before) == 0 {
		return after
	}
	if len(after) == 0 {
		return before
	}
	diff := map[string]any{}
	for k, v := range after {
		if !equalAny(before[k], v) {
			diff[k] = map[string]any{
				"before": before[k],
				"after":  v,
			}
		}
	}
	return diff
}

func equalAny(a, b any) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

// String is a debug helper that pretty-prints an entry (useful in tests).
func (e Entry) String() string {
	b, _ := json.MarshalIndent(e, "", "  ")
	return fmt.Sprintf("audit.Entry%s", string(b))
}
