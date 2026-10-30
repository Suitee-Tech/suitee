package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

	"github.com/oginoino/toca/pkg/config"
	appCtx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/metrics"
)

// PostgresDB wrapper over sqlx DB and pgx pool
type PostgresDB struct {
	*sqlx.DB
	pool                   *pgxpool.Pool
	ServicePool            *pgxpool.Pool
	serviceDB              *sqlx.DB
	serviceRoleBypassesRLS bool
	userRoleBypassesRLS    bool
}

var ErrServiceRoleRequired = errors.New("database service role with BYPASSRLS is required")

// ErrRLSBackstopRequired é devolvido no boot quando REQUIRE_RLS_BACKSTOP=true e
// o pool de aplicação conecta com um role que ignora RLS (achado M1 da revisão
// de permissões de 2026-09-19).
var ErrRLSBackstopRequired = errors.New("application pool role must not bypass RLS (REQUIRE_RLS_BACKSTOP=true)")

// ConnectPostgres establishes a connection to the PostgreSQL database using pgxpool and sqlx
func ConnectPostgres(ctx context.Context, cfg config.DatabaseConfig) (*PostgresDB, error) {
	if cfg.URI == "" {
		return nil, fmt.Errorf("database URI is required for Postgres connection")
	}

	// For pgxpool (recommended for most direct pgx usage and high-performance querying)
	poolConfig, err := pgxpool.ParseConfig(cfg.URI)
	if err != nil {
		return nil, fmt.Errorf("failed to parse postgres uri: %w", err)
	}

	poolConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
	poolConfig.ConnConfig.StatementCacheCapacity = 0
	poolConfig.ConnConfig.DescriptionCacheCapacity = 0

	// Apply connection pool settings
	if cfg.MaxPoolSize > 0 {
		poolConfig.MaxConns = int32(cfg.MaxPoolSize)
	} else {
		poolConfig.MaxConns = 25
	}

	// MIN_POOL_SIZE=0 é escolha legítima e o default operacional do beta: sem
	// conexões ociosas, cada serviço abre conexão sob demanda e as fecha quando
	// ocioso. Antes, qualquer valor <= 0 viráva 2 — com dois pools por serviço
	// (aplicação + serviço) isso mantinha ~2 conexões permanentes por serviço e
	// esgotava os slots do Postgres (incidente de 2026-09-30: "remaining
	// connection slots are reserved for roles with the SUPERUSER attribute",
	// com os serviços rodando e nenhuma conexão nova entrando).
	if cfg.MinPoolSize > 0 {
		poolConfig.MinConns = int32(cfg.MinPoolSize)
	} else {
		poolConfig.MinConns = 0
	}
	// pgxpool requires MinConns <= MaxConns. Local/Supabase-pooler
	// environments commonly cap each service at one connection; keeping the
	// old default MinConns=2 in that case produced an internally contradictory
	// pool configuration and misleading startup diagnostics.
	poolConfig.MaxConns, poolConfig.MinConns = normalizePoolLimits(
		poolConfig.MaxConns,
		poolConfig.MinConns,
	)

	poolConfig.MaxConnLifetime = time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute

	// Set request.jwt.claims per checkout so Supabase RLS policies that read
	// auth.jwt() see the correct tenant. This runs for every connection pulled
	// from the pool, including connections later used by sqlx (see
	// stdlib.OpenDBFromPool below), so repository queries are also gated.
	//
	// staticcheck flags BeforeAcquire as deprecated (suggests PrepareConn), but
	// PrepareConn only runs ONCE when a connection is prepared, while
	// BeforeAcquire runs on every Acquire — required here because each acquire
	// is for a different tenant context. Keep BeforeAcquire until pgx exposes
	// a per-acquire equivalent.
	//nolint:staticcheck // SA1019: BeforeAcquire is the right semantic here
	poolConfig.BeforeAcquire = applyJWTClaimsHook

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres via pgxpool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping postgres via pgxpool: %w", err)
	}

	// ServicePool: intended for internal cross-tenant operations (outbox
	// dispatcher, DLQ replay, invitation lookups) that must bypass RLS. Point
	// DATABASE_URI_SERVICE at a Supabase direct connection using the
	// service_role user (or any role with BYPASSRLS) to enable real bypass. If
	// it equals DATABASE_URI, this pool is effectively a reference to the
	// tenant-scoped pool — safe but not actually bypassing RLS.
	// Cross-tenant callers use QueryServiceRows/QueryServiceRow/ExecService;
	// production config rejects an absent or identical service URI so these
	// operations cannot silently fall back to the tenant-scoped credential.
	var servicePool *pgxpool.Pool
	if cfg.ServiceURI != "" && cfg.ServiceURI != cfg.URI {
		servicePoolConfig, err := pgxpool.ParseConfig(cfg.ServiceURI)
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("failed to parse postgres service uri: %w", err)
		}

		servicePoolConfig.ConnConfig.DefaultQueryExecMode = poolConfig.ConnConfig.DefaultQueryExecMode
		servicePoolConfig.ConnConfig.StatementCacheCapacity = poolConfig.ConnConfig.StatementCacheCapacity
		servicePoolConfig.ConnConfig.DescriptionCacheCapacity = poolConfig.ConnConfig.DescriptionCacheCapacity

		servicePoolConfig.MaxConns = poolConfig.MaxConns
		servicePoolConfig.MinConns = poolConfig.MinConns
		servicePoolConfig.MaxConnLifetime = poolConfig.MaxConnLifetime
		servicePoolConfig.MaxConnIdleTime = poolConfig.MaxConnIdleTime
		// The service-role pool intentionally does not apply request.jwt.claims:
		// its cross-tenant workers are authorized by application-level guards and
		// must retain BYPASSRLS. It still needs bounded statements/lock waits,
		// otherwise one blocked event can hold a privileged transaction and its
		// RabbitMQ delivery forever.
		//nolint:staticcheck // SA1019: per-acquire request deadline is required
		servicePoolConfig.BeforeAcquire = applyServiceTimeoutsHook

		servicePool, err = pgxpool.NewWithConfig(ctx, servicePoolConfig)
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("failed to connect to postgres service via pgxpool: %w", err)
		}
		if err := servicePool.Ping(ctx); err != nil {
			servicePool.Close()
			pool.Close()
			return nil, fmt.Errorf("failed to ping postgres service via pgxpool: %w", err)
		}
	} else {
		servicePool = pool
	}

	// Wrap the existing pgxpool as a database/sql DB so sqlx-driven queries
	// acquire connections from the same pool that runs BeforeAcquire. Before,
	// sqlx was built from a separate driver connection string and kept an
	// independent pool with no claims hook — repository queries ran without
	// request.jwt.claims and RLS silently filtered every row.
	stdDB := stdlib.OpenDBFromPool(pool)
	stdDB.SetMaxOpenConns(int(poolConfig.MaxConns))
	stdDB.SetMaxIdleConns(0)
	stdDB.SetConnMaxLifetime(poolConfig.MaxConnLifetime)
	stdDB.SetConnMaxIdleTime(poolConfig.MaxConnIdleTime)
	sqlxDB := sqlx.NewDb(stdDB, "pgx")

	// Repositories are written against sqlx and select the transaction carried
	// by context. Wrap the service pool as sqlx as well so an internal worker can
	// execute a whole cross-tenant financial graph in one service-role SQL
	// transaction instead of mixing non-atomic pool calls.
	serviceSQLXDB := sqlxDB
	if servicePool != pool {
		serviceStdDB := stdlib.OpenDBFromPool(servicePool)
		serviceStdDB.SetMaxOpenConns(int(poolConfig.MaxConns))
		serviceStdDB.SetMaxIdleConns(0)
		serviceStdDB.SetConnMaxLifetime(poolConfig.MaxConnLifetime)
		serviceStdDB.SetConnMaxIdleTime(poolConfig.MaxConnIdleTime)
		serviceSQLXDB = sqlx.NewDb(serviceStdDB, "pgx")
	}

	// P2/M1: as duas metades do retrato de RLS. O pool de serviço PRECISA
	// bypassar (senão o cross-tenant legítimo morre); o pool de APLICAÇÃO
	// (tenant-scoped, dono do request.jwt.claims) NÃO deveria — enquanto ele
	// bypassar, o isolamento depende 100% do WHERE escrito à mão em cada query,
	// sem backstop de banco.
	var serviceRoleBypassesRLS bool
	capabilityErr := servicePool.QueryRow(ctx, `
		SELECT COALESCE((
			SELECT rolbypassrls OR rolsuper
			FROM pg_roles
			WHERE rolname = current_user
		), false)
	`).Scan(&serviceRoleBypassesRLS)

	var userRoleBypassesRLS bool
	if capabilityErr == nil {
		capabilityErr = pool.QueryRow(ctx, `
			SELECT COALESCE((
				SELECT rolbypassrls OR rolsuper
				FROM pg_roles
				WHERE rolname = current_user
			), false)
		`).Scan(&userRoleBypassesRLS)
	}
	if capabilityErr != nil {
		if serviceSQLXDB != sqlxDB {
			_ = serviceSQLXDB.Close()
		}
		_ = sqlxDB.Close()
		if servicePool != pool {
			servicePool.Close()
		}
		pool.Close()
		return nil, fmt.Errorf("verify database service-role RLS capability: %w", capabilityErr)
	}
	if cfg.ServiceURI != "" && cfg.ServiceURI != cfg.URI && !serviceRoleBypassesRLS {
		if serviceSQLXDB != sqlxDB {
			_ = serviceSQLXDB.Close()
		}
		_ = sqlxDB.Close()
		servicePool.Close()
		pool.Close()
		return nil, ErrServiceRoleRequired
	}

	backstop := RLSBackstop{
		UserRoleBypassesRLS:    userRoleBypassesRLS,
		ServiceRoleBypassesRLS: serviceRoleBypassesRLS,
		DedicatedServiceRole:   cfg.ServiceURI != "" && cfg.ServiceURI != cfg.URI,
	}
	// Métrica além do log: o estado "as políticas de RLS são inertes" precisa
	// ser alertável (database_rls_backstop_active).
	metrics.SetRLSBackstopActive(backstop.Active())
	decision := EvaluateRLSBackstop(backstop)
	switch decision.Level {
	case "error":
		logger.Error(decision.Event, "detail", decision.Message)
	default:
		logger.Info(decision.Event, "detail", decision.Message)
	}
	if decision.Fatal {
		if serviceSQLXDB != sqlxDB {
			_ = serviceSQLXDB.Close()
		}
		_ = sqlxDB.Close()
		if servicePool != pool {
			servicePool.Close()
		}
		pool.Close()
		if decision.Err != nil {
			return nil, decision.Err
		}
		return nil, ErrRLSBackstopRequired
	}

	logger.Info("Successfully connected to Postgres Database", map[string]interface{}{
		"max_conns": poolConfig.MaxConns,
		"min_conns": poolConfig.MinConns,
	})

	return &PostgresDB{
		DB:                     sqlxDB,
		pool:                   pool,
		ServicePool:            servicePool,
		serviceDB:              serviceSQLXDB,
		serviceRoleBypassesRLS: serviceRoleBypassesRLS,
		userRoleBypassesRLS:    userRoleBypassesRLS,
	}, nil
}

func normalizePoolLimits(maxConns, minConns int32) (int32, int32) {
	if minConns > maxConns {
		minConns = maxConns
	}
	return maxConns, minConns
}

// applyJWTClaimsHook is used as pgxpool's BeforeAcquire callback. It seeds
// request.jwt.claims on every connection checkout so Supabase RLS policies
// that read auth.jwt() receive the current tenant. Clears the setting when
// the context carries no tenant so a leftover value from a previous checkout
// cannot leak across requests.
func applyJWTClaimsHook(ctx context.Context, conn *pgx.Conn) bool {
	tenantID, _ := appCtx.GetTenantID(ctx)
	accountID, _ := appCtx.GetAccountID(ctx)
	statementTimeout := "0"
	if strings.TrimSpace(tenantID) != "" {
		// Backstop cap for tenant-scoped requests that carry no explicit
		// deadline. Overridden below when the request context has one.
		statementTimeout = "15000"
	}
	// Track the request deadline (set by the RequestTimeout middleware) so the
	// DB aborts the query just BEFORE the middleware fires. Without this a fixed
	// statement_timeout races the middleware: when the DB wins, the handler sees
	// a query error and returns 500; when the middleware wins, the client gets
	// 504 — same slowness, two codes. Cancelling ~1.5s early makes the DB win
	// deterministically (callers map 57014 -> 504 via database.IsTimeout), and
	// it also stops the old fixed 15s from wrongly cutting the 60s batch routes.
	if dl, ok := ctx.Deadline(); ok {
		budget := time.Until(dl) - 1500*time.Millisecond
		if budget < time.Second {
			budget = time.Second
		}
		statementTimeout = strconv.FormatInt(budget.Milliseconds(), 10)
	}
	claims := map[string]interface{}{}
	if strings.TrimSpace(accountID) != "" {
		claims["sub"] = accountID
	}
	if strings.TrimSpace(tenantID) != "" {
		claims["app_metadata"] = map[string]interface{}{
			"tenant_id": tenantID,
		}
	}
	if len(claims) > 0 {
		claimsJSON, err := json.Marshal(claims)
		if err == nil {
			_, err = conn.Exec(ctx, "SELECT set_config('request.jwt.claims', $1, false)", string(claimsJSON))
			if err != nil {
				logger.Error("Failed to set request.jwt.claims", map[string]interface{}{"error": err.Error()})
				return false
			}
		}
	} else {
		_, _ = conn.Exec(ctx, "SELECT set_config('request.jwt.claims', '{}', false)")
	}
	_, err := conn.Exec(ctx, "SELECT set_config('statement_timeout', $1, false)", statementTimeout)
	if err != nil {
		logger.Error("Failed to set statement_timeout", map[string]interface{}{"error": err.Error()})
		return false
	}
	return true
}

const (
	serviceStatementTimeout = 60 * time.Second
	serviceLockTimeout      = 10 * time.Second
	databaseDeadlineMargin  = 1500 * time.Millisecond
	serviceTimeoutConfigSQL = `
		SELECT
			set_config('statement_timeout', $1, false),
			set_config('lock_timeout', $2, false)
	`
)

// applyServiceTimeoutsHook bounds privileged worker queries without seeding
// tenant/account claims. Service-role connections deliberately bypass RLS; a
// tenant claim here would be misleading and could leak between cross-tenant
// jobs even though it does not constrain a BYPASSRLS role.
func applyServiceTimeoutsHook(ctx context.Context, conn *pgx.Conn) bool {
	statementTimeout, lockTimeout := serviceTimeoutValues(ctx, time.Now())
	_, err := conn.Exec(ctx, serviceTimeoutConfigSQL, statementTimeout, lockTimeout)
	if err != nil {
		logger.Error("Failed to set service-role database timeouts", map[string]interface{}{"error": err.Error()})
		return false
	}
	return true
}

func serviceTimeoutValues(ctx context.Context, now time.Time) (statementTimeout, lockTimeout string) {
	statementBudget := serviceStatementTimeout
	if deadline, ok := ctx.Deadline(); ok {
		statementBudget = deadline.Sub(now) - databaseDeadlineMargin
		if statementBudget < time.Second {
			statementBudget = time.Second
		}
	}

	lockBudget := serviceLockTimeout
	if lockBudget > statementBudget {
		lockBudget = statementBudget
	}
	return strconv.FormatInt(statementBudget.Milliseconds(), 10), strconv.FormatInt(lockBudget.Milliseconds(), 10)
}

// Close closes both sqlx and pgxpool connections
func (p *PostgresDB) Close() error {
	var err error
	if p.serviceDB != nil && p.serviceDB != p.DB {
		if serviceErr := p.serviceDB.Close(); serviceErr != nil {
			err = serviceErr
		}
	}
	if p.DB != nil {
		if dbErr := p.DB.Close(); dbErr != nil && err == nil {
			err = dbErr
		}
	}
	if p.pool != nil {
		p.pool.Close()
	}
	if p.ServicePool != nil && p.ServicePool != p.pool {
		p.ServicePool.Close()
	}
	return err
}

// GetPool returns the underlying pgxpool.Pool
func (p *PostgresDB) GetPool() *pgxpool.Pool {
	return p.pool
}

// CanHoldSessionLockDuringWork reports whether one connection can remain
// reserved by a session-scoped advisory lock while the protected job uses a
// second connection for its repository queries.
//
// A pool with a single slot cannot do both: acquiring the lock and then
// querying through the same pool self-deadlocks. Callers must fall back to
// single-instance execution (without the distributed lock) or provision at
// least two connections.
func (p *PostgresDB) CanHoldSessionLockDuringWork() bool {
	if p == nil || p.pool == nil || p.DB == nil {
		return false
	}
	// Check both layers: pgxpool and database/sql have independent capacity
	// accounting even though sqlx wraps the pgx pool.
	return p.pool.Config().MaxConns >= 2 && p.DB.Stats().MaxOpenConnections >= 2
}

// QueryServiceRows runs a query on the service-role pool, bypassing RLS. Use
// for cross-tenant workers (SLA redirects, outbox drain, DLQ replay) where
// the current request context has no tenant but we still need to see every
// row. Caller owns closing pgx.Rows.
func (p *PostgresDB) QueryServiceRows(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	pool := p.ServicePool
	if pool == nil {
		pool = p.pool
	}
	return pool.Query(ctx, query, args...)
}

// QueryServiceRow runs a single-row query on the service-role pool.
func (p *PostgresDB) QueryServiceRow(ctx context.Context, query string, args ...interface{}) pgx.Row {
	pool := p.ServicePool
	if pool == nil {
		pool = p.pool
	}
	return pool.QueryRow(ctx, query, args...)
}

// ExecService executes a statement on the service-role pool (pgx). Use
// ExecServiceRows when the RowsAffected matters.
func (p *PostgresDB) ExecService(ctx context.Context, query string, args ...interface{}) error {
	pool := p.ServicePool
	if pool == nil {
		pool = p.pool
	}
	_, err := pool.Exec(ctx, query, args...)
	return err
}

// ExecServiceRows roda um comando no pool service-role e devolve o CommandTag.
//
// Existe porque `ExecService` devolve só erro: sem o tag não dá para distinguir
// "afetou 0 linhas" de "deu certo". Há tabelas em que o escritor privilegiado é
// o ÚNICO autorizado — `financial.journal_entries` (ledger append-only) ficou só
// com policy de SELECT depois da migration 20260822123000 — e o RowsAffected é a
// única forma de saber se a linha alvo existia.
func (p *PostgresDB) ExecServiceRows(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	pool := p.ServicePool
	if pool == nil {
		pool = p.pool
	}
	return pool.Exec(ctx, query, args...)
}

// RunInTx executes a function within a SQL transaction using sqlx
func (p *PostgresDB) RunInTx(ctx context.Context, fn func(tx *sqlx.Tx) error) error {
	// Compose transactional services without opening an independent nested
	// transaction. PostgreSQL/database/sql has no implicit nested transaction:
	// starting a new one here would make the inner writes commit independently
	// from the transaction already carried by ctx (and can deadlock a one-slot
	// pool). The outer owner remains responsible for commit/rollback.
	if tx := TxFromCtx(ctx); tx != nil {
		return fn(tx)
	}
	return runInSQLXTx(ctx, p.DB, fn)
}

func runInSQLXTx(ctx context.Context, db *sqlx.DB, fn func(tx *sqlx.Tx) error) error {
	if db == nil {
		return fmt.Errorf("database connection is nil")
	}
	if fn == nil {
		return fmt.Errorf("transaction callback is nil")
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	hooks := &afterCommitCallbacks{}
	afterCommitByTx.Store(tx, hooks)
	defer afterCommitByTx.Delete(tx)

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p) // Re-throw panic after rollback
		}
	}()

	if err := fn(tx); err != nil {
		if rErr := tx.Rollback(); rErr != nil {
			return fmt.Errorf("transaction fn error: %v, rollback error: %v", err, rErr)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	hooks.run()

	return nil
}

// RunInServiceContext executes an internal cross-tenant workflow on the
// verified BYPASSRLS pool and carries its transaction to every repository via
// context. A tenant transaction can never be silently reused as a service
// transaction.
func (p *PostgresDB) RunInServiceContext(ctx context.Context, fn func(context.Context) error) error {
	if fn == nil {
		return fmt.Errorf("service transaction callback is nil")
	}
	if tx := TxFromCtx(ctx); tx != nil {
		if !ServiceTxFromCtx(ctx) {
			return ErrServiceRoleRequired
		}
		return fn(ctx)
	}
	if p == nil || p.serviceDB == nil || !p.serviceRoleBypassesRLS {
		return ErrServiceRoleRequired
	}
	return runInSQLXTx(ctx, p.serviceDB, func(tx *sqlx.Tx) error {
		return fn(withServiceTx(ctx, tx))
	})
}

// ── Transaction-aware helpers ──────────────────────────────────────
// Repositories that call r.db.Exec / r.db.QueryRow / r.db.Query
// (instead of ExecContext/QueryRowContext/QueryContext) automatically
// participate in a transaction when one is stored in the context via
// WithTx. Otherwise they fall back to the default *sqlx.DB connection.

type ctxTxKey struct{}
type ctxServiceTxKey struct{}

type afterCommitCallbacks struct {
	mu        sync.Mutex
	callbacks []func()
}

func (h *afterCommitCallbacks) add(callback func()) {
	if callback == nil {
		return
	}
	h.mu.Lock()
	h.callbacks = append(h.callbacks, callback)
	h.mu.Unlock()
}

func (h *afterCommitCallbacks) run() {
	h.mu.Lock()
	callbacks := append([]func(){}, h.callbacks...)
	h.callbacks = nil
	h.mu.Unlock()
	for _, callback := range callbacks {
		// A post-commit observer can no longer roll back the business write.
		// Keep it from turning a successful transaction into a caller-visible
		// panic; observers are responsible for their own failure reporting.
		func() {
			defer func() { _ = recover() }()
			callback()
		}()
	}
}

var afterCommitByTx sync.Map // map[*sqlx.Tx]*afterCommitCallbacks

// WithTx stores a *sqlx.Tx in the context so that downstream
// repository calls participate in the same transaction.
func WithTx(ctx context.Context, tx *sqlx.Tx) context.Context {
	return context.WithValue(ctx, ctxTxKey{}, tx)
}

func withServiceTx(ctx context.Context, tx *sqlx.Tx) context.Context {
	return context.WithValue(WithTx(ctx, tx), ctxServiceTxKey{}, true)
}

// TxFromCtx extracts a *sqlx.Tx from the context, or nil.
func TxFromCtx(ctx context.Context) *sqlx.Tx {
	tx, _ := ctx.Value(ctxTxKey{}).(*sqlx.Tx)
	return tx
}

// ServiceTxFromCtx reports whether the carried transaction was opened against
// the verified service-role pool.
func ServiceTxFromCtx(ctx context.Context) bool {
	serviceTx, _ := ctx.Value(ctxServiceTxKey{}).(bool)
	return serviceTx
}

// DeferUntilCommit queues an observer while ctx belongs to a transaction
// owned by RunInTx. It returns true when queued. Callers should execute the
// observer immediately when it returns false (no managed transaction).
// Nested RunInTx calls share the outer queue and callbacks are discarded on
// rollback or commit failure.
func DeferUntilCommit(ctx context.Context, callback func()) bool {
	return DeferTxUntilCommit(TxFromCtx(ctx), callback)
}

// DeferTxUntilCommit é a variante de DeferUntilCommit para callers que já têm
// o *sqlx.Tx em mãos (ex.: EnqueueOutboxInTx). O callback roda DEPOIS do
// commit e é descartado quando a transação sofre rollback — é o que permite
// escrever em tabelas deny-by-default sob RLS (events.outbox_events) pelo
// pool de serviço mantendo a semântica atômica com a escrita de negócio.
func DeferTxUntilCommit(tx *sqlx.Tx, callback func()) bool {
	if tx == nil || callback == nil {
		return false
	}
	value, ok := afterCommitByTx.Load(tx)
	if !ok {
		return false
	}
	hooks, ok := value.(*afterCommitCallbacks)
	if !ok || hooks == nil {
		return false
	}
	hooks.add(callback)
	return true
}

// Exec runs a statement using the transaction in ctx (if any), falling
// back to r.db.ExecContext.
func (p *PostgresDB) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return p.executor(ctx).ExecContext(ctx, query, args...)
}

// QueryRow returns a single row using the transaction in ctx (if any).
func (p *PostgresDB) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return p.executor(ctx).QueryRowContext(ctx, query, args...)
}

// ── Variantes ...Context (tx-aware) ────────────────────────────────
//
// ATENÇÃO: `PostgresDB` embute `*sqlx.DB`, então `QueryContext`,
// `QueryRowContext` e `ExecContext` eram os métodos PROMOVIDOS do pool de
// APLICAÇÃO: participavam da transação do ctx apenas por acidente — nunca.
// Medido no beta em 2026-09-30: dentro de `RunInServiceContext` a sessão
// continuava `toca_app` (40 linhas visíveis no pool de serviço contra 0 dentro
// da transação), e o portal do cliente lia "nada" com a transação de serviço
// parecendo bem-sucedida. Os repositórios escrevem `r.db.QueryRowContext(ctx,
// ...)`, então sem estas três declarações a transação de serviço era decorativa.

// QueryContext runs a multi-row query using the transaction in ctx (if any).
func (p *PostgresDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return p.executor(ctx).QueryContext(ctx, query, args...)
}

// QueryRowContext returns a single row using the transaction in ctx (if any).
func (p *PostgresDB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return p.executor(ctx).QueryRowContext(ctx, query, args...)
}

// ExecContext runs a statement using the transaction in ctx (if any).
func (p *PostgresDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return p.executor(ctx).ExecContext(ctx, query, args...)
}

// Query runs a multi-row query using the transaction in ctx (if any).
func (p *PostgresDB) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return p.executor(ctx).QueryContext(ctx, query, args...)
}

// ctxExecutor é o que *sqlx.Tx e *sqlx.DB têm em comum para os três acessos
// usados pelos repositórios.
type ctxExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// executor escolhe o pool: a transação carregada no contexto
// (RunInServiceContext, ou RunInTx aninhado) ou o pool de aplicação — que roda
// com as claims do tenant do contexto (`applyJWTClaimsHook`) e portanto sob RLS.
// Num lugar só, para nenhum helper esquecer a transação e escrever fora dela.
func (p *PostgresDB) executor(ctx context.Context) ctxExecutor {
	if tx := TxFromCtx(ctx); tx != nil {
		return tx
	}
	return p.DB
}
