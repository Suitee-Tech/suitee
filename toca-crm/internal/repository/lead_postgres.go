package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

type leadPostgresRepository struct {
	db *database.PostgresDB
	// updateSeqMissing vira true se PostgreSQL retornar "column updated_seq
	// does not exist" — sinal que a migration 20260517120000_lead_updated_seq
	// ainda não rodou no ambiente. Quando true, o repo desabilita o
	// bumping do seq e o ETag cai pro formato legado (só timestamp).
	// Reabilita sozinho assim que o serviço reinicia em uma stack onde
	// a migration já rodou. Sentinela atômica para ser thread-safe.
	updateSeqMissing atomic.Bool
}

// isMissingUpdatedSeq inspeciona o erro retornado pelo PostgreSQL e detecta
// o caso específico de coluna updated_seq ausente. Usado pelo Patch/Update
// para cair em fallback sem quebrar o handler.
func isMissingUpdatedSeq(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// pgx retorna mensagens estilo: ERROR: column "updated_seq" does not exist (SQLSTATE 42703)
	return strings.Contains(msg, "updated_seq") && strings.Contains(strings.ToLower(msg), "does not exist")
}

// isNoRows cobre o "nenhum registro" dos dois pools: database/sql (pool de
// aplicação, via sqlx) e pgx (pool de serviço, usado nos fluxos cross-tenant
// do portal do cliente).
func isNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows)
}

func NewLeadPostgresRepository(db *database.PostgresDB) *leadPostgresRepository {
	return &leadPostgresRepository{db: db}
}

func (r *leadPostgresRepository) Create(ctx context.Context, l *lead.Lead) error {
	if l.ID == "" {
		l.ID = uuid.NewString()
	}
	metadata, _ := json.Marshal(l)
	query := `INSERT INTO crm.leads (id, tenant_id, metadata) VALUES ($1, $2, $3) RETURNING created_at`
	return r.db.QueryRowContext(ctx, query, l.ID, l.TenantID, string(metadata)).Scan(&l.CreatedAt)
}

// GetByID resolve o lead dentro do tenant informado.
//
// SECURITY (achado C9): a query filtra por `tenant_id` além do `id`. Sem isso,
// qualquer usuário com `leads:read`/`leads:write` lia (e sobrescrevia) leads de
// outro tenant conhecendo o UUID. Chamadores legítimos passam o tenant do
// contexto da requisição; workers/eventos passam o tenant do payload.
func (r *leadPostgresRepository) GetByID(ctx context.Context, tenantID, id string) (*lead.Lead, error) {
	if strings.TrimSpace(tenantID) == "" {
		// Falha fechado: sem tenant não há como provar que a linha pertence ao
		// chamador. Nunca degradar para uma busca global.
		return nil, fmt.Errorf("lead not found: tenant is required")
	}
	var metadata []byte
	var updatedAt time.Time
	var updatedSeq int64
	var err error
	if !r.updateSeqMissing.Load() {
		err = r.db.QueryRowContext(ctx, "SELECT metadata, updated_at, updated_seq FROM crm.leads WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL", id, tenantID).Scan(&metadata, &updatedAt, &updatedSeq)
		if isMissingUpdatedSeq(err) {
			r.updateSeqMissing.Store(true)
		}
	}
	if r.updateSeqMissing.Load() {
		err = r.db.QueryRowContext(ctx, "SELECT metadata, updated_at FROM crm.leads WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL", id, tenantID).Scan(&metadata, &updatedAt)
	}
	if err != nil {
		return nil, fmt.Errorf("lead not found: %w", err)
	}
	var l lead.Lead
	if err := json.Unmarshal(metadata, &l); err != nil {
		return nil, err
	}
	// SQL version columns are authoritative; metadata can predate the bump.
	l.UpdatedAt, l.UpdatedSeq = updatedAt, updatedSeq
	return &l, nil
}

func (r *leadPostgresRepository) GetByEmail(ctx context.Context, tenantID, email string) (*lead.Lead, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata
		FROM crm.leads
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND (metadata->>'Email') = $2
		ORDER BY created_at DESC
		LIMIT 1
	`, tenantID, email).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("lead not found: %w", err)
	}
	var l lead.Lead
	_ = json.Unmarshal(metadata, &l)
	return &l, nil
}

func (r *leadPostgresRepository) GetByPhone(ctx context.Context, tenantID, phone string) (*lead.Lead, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata
		FROM crm.leads
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND (metadata->>'Phone') = $2
		ORDER BY created_at DESC
		LIMIT 1
	`, tenantID, phone).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("lead not found: %w", err)
	}
	var l lead.Lead
	_ = json.Unmarshal(metadata, &l)
	return &l, nil
}

func (r *leadPostgresRepository) GetByDisplayID(ctx context.Context, tenantID, displayID string) (*lead.Lead, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata
		FROM crm.leads
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND (metadata->>'DisplayID') = $2
		ORDER BY created_at DESC
		LIMIT 1
	`, tenantID, displayID).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("lead not found: %w", err)
	}
	var l lead.Lead
	_ = json.Unmarshal(metadata, &l)
	return &l, nil
}

func (r *leadPostgresRepository) ListByAccountID(ctx context.Context, tenantID, accountID string, limit int64) ([]*lead.Lead, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT metadata
		FROM crm.leads
		WHERE tenant_id = $1 AND (metadata->>'AccountID') = $2 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $3
	`, tenantID, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	leads := make([]*lead.Lead, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var l lead.Lead
		if err := json.Unmarshal(metadata, &l); err != nil {
			return nil, err
		}
		leads = append(leads, &l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return leads, nil
}

func (r *leadPostgresRepository) FindByAccountID(ctx context.Context, accountID string, limit int64) ([]*lead.Lead, error) {
	if limit <= 0 {
		limit = 20
	}
	// POOL DE SERVIÇO (BYPASSRLS): leitura cross-tenant deliberada do portal
	// do cliente. O tenant do token de um cliente é a conta local dele — não a
	// empresa dona do lead — então sob RLS o pool de aplicação simplesmente não
	// enxerga a linha e o portal ficava vazio. A autorização é a posse do
	// account_id, provada pelo JWT (o handler deriva do token, nunca do corpo).
	rows, err := r.db.QueryServiceRows(ctx, `
		SELECT metadata
		FROM crm.leads
		WHERE (metadata->>'AccountID') = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2
	`, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	leads := make([]*lead.Lead, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var l lead.Lead
		if err := json.Unmarshal(metadata, &l); err != nil {
			return nil, err
		}
		leads = append(leads, &l)
	}
	return leads, rows.Err()
}

// FindByEmailAnyTenant resolve leads pelo e-mail sem filtro de tenant.
//
// SECURITY (achado A19): caminho cross-tenant deliberado e explícito. O único
// chamador é o overview do portal do cliente, que prova a posse do e-mail pelo
// token. Para qualquer rota de operador use List/GetByEmail (tenant-scoped).
//
// Roda no POOL DE SERVIÇO (BYPASSRLS) porque o tenant do token do cliente é a
// conta local dele: sob RLS o pool de aplicação não enxerga o lead da
// imobiliária e o portal ficava sem nenhum dado.
func (r *leadPostgresRepository) FindByEmailAnyTenant(ctx context.Context, email string, limit int64) ([]*lead.Lead, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryServiceRows(ctx, `
		SELECT metadata
		FROM crm.leads
		WHERE lower(metadata->>'Email') = lower($1) AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2
	`, email, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	leads := make([]*lead.Lead, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var l lead.Lead
		if err := json.Unmarshal(metadata, &l); err != nil {
			return nil, err
		}
		leads = append(leads, &l)
	}
	return leads, rows.Err()
}

func (r *leadPostgresRepository) Update(ctx context.Context, l *lead.Lead) error {
	l.UpdatedAt = time.Now()
	metadata, _ := json.Marshal(l)

	// Path normal (com updated_seq) — só tenta se ainda não detectamos
	// que a migration está ausente nesse ambiente.
	if !r.updateSeqMissing.Load() {
		var newSeq int64
		err := r.db.QueryRowContext(ctx,
			"UPDATE crm.leads SET metadata = ($1::jsonb - 'visit_operation' - 'visit_request') || jsonb_build_object('visit_operation', metadata->'visit_operation', 'visit_request', metadata->'visit_request'), updated_at = NOW(), updated_seq = updated_seq + 1 WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL RETURNING updated_seq",
			string(metadata), l.ID, l.TenantID,
		).Scan(&newSeq)
		if err == nil {
			l.UpdatedSeq = newSeq
			return nil
		}
		if err == sql.ErrNoRows {
			return nil
		}
		if !isMissingUpdatedSeq(err) {
			return err
		}
		// Migration ausente — marca a flag e cai no fallback abaixo.
		r.updateSeqMissing.Store(true)
		logger.Warn("crm.leads.updated_seq column missing, falling back",
			"migration", "20260517120000_lead_updated_seq.sql",
			"impact", "ETag concorrência cai para resolução por timestamp",
		)
	}

	// Fallback: schema pré-Wave-1.6 sem updated_seq.
	_, err := r.db.ExecContext(ctx,
		"UPDATE crm.leads SET metadata = ($1::jsonb - 'visit_operation' - 'visit_request') || jsonb_build_object('visit_operation', metadata->'visit_operation', 'visit_request', metadata->'visit_request'), updated_at = NOW() WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL",
		string(metadata), l.ID, l.TenantID,
	)
	return err
}

// GetByIDForTenantService resolve o lead de um tenant EXPLÍCITO pelo pool de
// SERVIÇO (BYPASSRLS) — rotas internas, onde não há claims de JWT para a
// política de RLS e o pool de aplicação devolveria "não encontrado" para uma
// linha existente. O escopo é garantido pelo `tenant_id` no WHERE.
func (r *leadPostgresRepository) GetByIDForTenantService(ctx context.Context, tenantID, id string) (*lead.Lead, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, fmt.Errorf("lead not found: tenant is required")
	}
	var metadata []byte
	var updatedAt time.Time
	var updatedSeq int64
	var err error
	if !r.updateSeqMissing.Load() {
		err = r.db.QueryServiceRow(ctx, "SELECT metadata, updated_at, updated_seq FROM crm.leads WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL", id, tenantID).Scan(&metadata, &updatedAt, &updatedSeq)
		if isMissingUpdatedSeq(err) {
			r.updateSeqMissing.Store(true)
		}
	}
	if r.updateSeqMissing.Load() {
		err = r.db.QueryServiceRow(ctx, "SELECT metadata, updated_at FROM crm.leads WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL", id, tenantID).Scan(&metadata, &updatedAt)
	}
	if err != nil {
		return nil, fmt.Errorf("lead not found: %w", err)
	}
	var l lead.Lead
	if err := json.Unmarshal(metadata, &l); err != nil {
		return nil, err
	}
	l.UpdatedAt, l.UpdatedSeq = updatedAt, updatedSeq
	return &l, nil
}

// GetByIDAnyTenant resolve o lead sem filtro de tenant.
//
// SECURITY: caminho cross-tenant deliberado e explícito (achado C9). Os usos
// são o portal do cliente (que prova a identidade do titular logo em seguida,
// por account id ou e-mail) e o worker de eventos que sincroniza visitas. Para
// tudo o mais use GetByID com o tenant do chamador.
//
// POOL DE SERVIÇO (BYPASSRLS): o tenant do token do cliente é a conta local
// dele, não a empresa dona do lead — sob RLS o pool de aplicação filtraria a
// linha e o portal não enxergaria nem o próprio cadastro.
func (r *leadPostgresRepository) GetByIDAnyTenant(ctx context.Context, id string) (*lead.Lead, error) {
	var metadata []byte
	var updatedAt time.Time
	var updatedSeq int64
	var err error
	if !r.updateSeqMissing.Load() {
		err = r.db.QueryServiceRow(ctx, "SELECT metadata, updated_at, updated_seq FROM crm.leads WHERE id = $1 AND deleted_at IS NULL", id).Scan(&metadata, &updatedAt, &updatedSeq)
		if isMissingUpdatedSeq(err) {
			r.updateSeqMissing.Store(true)
		}
	}
	if r.updateSeqMissing.Load() {
		err = r.db.QueryServiceRow(ctx, "SELECT metadata, updated_at FROM crm.leads WHERE id = $1 AND deleted_at IS NULL", id).Scan(&metadata, &updatedAt)
	}
	if err != nil {
		return nil, fmt.Errorf("lead not found: %w", err)
	}
	var l lead.Lead
	if err := json.Unmarshal(metadata, &l); err != nil {
		return nil, err
	}
	l.UpdatedAt, l.UpdatedSeq = updatedAt, updatedSeq
	return &l, nil
}

func (r *leadPostgresRepository) Patch(ctx context.Context, leadID, tenantID string, expected *lead.LeadExpectedVersion, updates map[string]interface{}) (*lead.Lead, bool, error) {
	return r.patch(ctx, leadID, tenantID, expected, updates, false)
}

// PatchService é o Patch para os fluxos CROSS-TENANT do portal do cliente
// (solicitar/reagendar/cancelar visita) e para o worker de recuperação de
// visitas: o tenant do token do cliente é a conta local dele, então o UPDATE
// pelo pool de aplicação não encontra a linha sob RLS. A autorização é feita
// ANTES, no serviço (posse do lead por account_id/e-mail), e o WHERE do patch
// continua carregando o tenant do lead — nada aqui dispensa o predicado.
func (r *leadPostgresRepository) PatchService(ctx context.Context, leadID, tenantID string, expected *lead.LeadExpectedVersion, updates map[string]interface{}) (*lead.Lead, bool, error) {
	return r.patch(ctx, leadID, tenantID, expected, updates, true)
}

func (r *leadPostgresRepository) patch(ctx context.Context, leadID, tenantID string, expected *lead.LeadExpectedVersion, updates map[string]interface{}, service bool) (*lead.Lead, bool, error) {
	if leadID == "" || tenantID == "" {
		return nil, false, fmt.Errorf("lead_id and tenant_id are required")
	}
	if len(updates) == 0 {
		// SECURITY (achado C9): propaga o tenant do chamador para a leitura —
		// era por aqui que um PUT com corpo vazio devolvia o lead de outro
		// tenant (200 + DTO completo).
		var l *lead.Lead
		var err error
		if service {
			l, err = r.GetByIDAnyTenant(ctx, leadID)
			if err == nil && l != nil && l.TenantID != tenantID {
				return nil, false, nil
			}
		} else {
			l, err = r.GetByID(ctx, tenantID, leadID)
		}
		if err != nil {
			return nil, false, err
		}
		return l, true, nil
	}

	now := time.Now().UTC()

	// Path normal (com updated_seq) — só roda se a coluna existir nesse env.
	if !r.updateSeqMissing.Load() {
		l, ok, err := r.patchWithSeq(ctx, leadID, tenantID, expected, updates, now, service)
		if err == nil {
			return l, ok, nil
		}
		if !isMissingUpdatedSeq(err) {
			return nil, false, err
		}
		// Coluna não existe — marca a flag, loga uma vez, cai no fallback.
		r.updateSeqMissing.Store(true)
		logger.Warn("crm.leads.updated_seq column missing, falling back",
			"migration", "20260517120000_lead_updated_seq.sql",
			"impact", "Patch passa a usar apenas updated_at para optimistic locking",
		)
	}

	return r.patchWithoutSeq(ctx, leadID, tenantID, expected, updates, now, service)
}

// patchWithSeq executes the post-Wave-1.6 UPDATE that bumps updated_seq.
// Returns isMissingUpdatedSeq(err)==true caller for fallback.
func (r *leadPostgresRepository) patchWithSeq(ctx context.Context, leadID, tenantID string, expected *lead.LeadExpectedVersion, updates map[string]interface{}, now time.Time, service bool) (*lead.Lead, bool, error) {
	expr, args, argN := buildPatchExpr(updates, now)
	idArg := argN
	args = append(args, leadID)
	argN++
	tenantArg := argN
	args = append(args, tenantID)
	argN++

	where := fmt.Sprintf("WHERE id = $%d AND tenant_id = $%d AND deleted_at IS NULL", idArg, tenantArg)
	if expected != nil {
		if expected.UpdatedSeq != nil {
			args = append(args, *expected.UpdatedSeq)
			where += fmt.Sprintf(" AND updated_seq = $%d", argN)
			argN++
		}
		if expected.UpdatedAt != nil {
			args = append(args, *expected.UpdatedAt)
			where += fmt.Sprintf(" AND updated_at = $%d", argN)
		}
	}

	var metadata []byte
	var newSeq int64
	var persistedAt time.Time
	query := "UPDATE crm.leads SET metadata = " + expr + ", updated_at = NOW(), updated_seq = updated_seq + 1 " + where + " RETURNING metadata, updated_seq, updated_at"
	var err error
	if service {
		err = r.db.QueryServiceRow(ctx, query, args...).Scan(&metadata, &newSeq, &persistedAt)
	} else {
		err = r.db.QueryRowContext(ctx, query, args...).Scan(&metadata, &newSeq, &persistedAt)
	}
	if isNoRows(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	var l lead.Lead
	if err := json.Unmarshal(metadata, &l); err != nil {
		return nil, false, err
	}
	l.UpdatedAt = persistedAt
	l.UpdatedSeq = newSeq
	return &l, true, nil
}

// patchWithoutSeq executes the pre-Wave-1.6 UPDATE without referencing
// updated_seq. Used when the migration hasn't run yet on this environment.
func (r *leadPostgresRepository) patchWithoutSeq(ctx context.Context, leadID, tenantID string, expected *lead.LeadExpectedVersion, updates map[string]interface{}, now time.Time, service bool) (*lead.Lead, bool, error) {
	expr, args, argN := buildPatchExpr(updates, now)
	idArg := argN
	args = append(args, leadID)
	argN++
	tenantArg := argN
	args = append(args, tenantID)
	argN++

	where := fmt.Sprintf("WHERE id = $%d AND tenant_id = $%d AND deleted_at IS NULL", idArg, tenantArg)
	if expected != nil && expected.UpdatedAt != nil {
		args = append(args, *expected.UpdatedAt)
		where += fmt.Sprintf(" AND updated_at = $%d", argN)
	}

	var metadata []byte
	var persistedAt time.Time
	query := "UPDATE crm.leads SET metadata = " + expr + ", updated_at = NOW() " + where + " RETURNING metadata, updated_at"
	var err error
	if service {
		err = r.db.QueryServiceRow(ctx, query, args...).Scan(&metadata, &persistedAt)
	} else {
		err = r.db.QueryRowContext(ctx, query, args...).Scan(&metadata, &persistedAt)
	}
	if isNoRows(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var l lead.Lead
	if err := json.Unmarshal(metadata, &l); err != nil {
		return nil, false, err
	}
	l.UpdatedAt = persistedAt
	return &l, true, nil
}

// buildPatchExpr monta a expressão jsonb_set encadeada + os args base
// (todos os valores do updates + UpdatedAt). Retorna o próximo argN livre
// para o caller anexar where args (id, tenant, expected).
func buildPatchExpr(updates map[string]interface{}, now time.Time) (string, []any, int) {
	args := make([]any, 0, len(updates)+3)
	argN := 1
	expr := "metadata"

	keys := make([]string, 0, len(updates))
	for k := range updates {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := updates[k]
		b, _ := json.Marshal(v)
		expr = fmt.Sprintf("jsonb_set(%s, '{%s}', $%d::jsonb, true)", expr, k, argN)
		args = append(args, string(b))
		argN++
	}

	expr = fmt.Sprintf("jsonb_set(%s, '{UpdatedAt}', to_jsonb($%d::timestamptz), true)", expr, argN)
	args = append(args, now)
	argN++

	return expr, args, argN
}

func (r *leadPostgresRepository) AssignBrokerIfUnassigned(ctx context.Context, leadID, tenantID, brokerID, details string) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE crm.leads
		SET metadata = jsonb_set(
			jsonb_set(metadata, '{AssignedBrokerID}', to_jsonb($1::text), true),
			'{UpdatedAt}', to_jsonb(NOW()::timestamptz), true
		),
		    updated_at = NOW()
		WHERE id = $2
		  AND tenant_id = $3
		  AND COALESCE(metadata->>'AssignedBrokerID','') = ''
		  AND deleted_at IS NULL
	`, brokerID, leadID, tenantID)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// Delete faz o soft delete do lead dentro do tenant informado.
//
// SECURITY (achado C9): filtra por `tenant_id`; antes bastava o UUID para
// remover o lead de outro tenant. Retorna sql.ErrNoRows quando nada foi
// afetado, para o chamador distinguir "removi" de "não existe/não é seu" em vez
// de responder sucesso silencioso.
func (r *leadPostgresRepository) Delete(ctx context.Context, tenantID, id string) error {
	if strings.TrimSpace(tenantID) == "" {
		return fmt.Errorf("tenant is required to delete a lead")
	}
	res, err := r.db.ExecContext(ctx, "UPDATE crm.leads SET deleted_at = NOW() WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL", id, tenantID)
	if err != nil {
		return err
	}
	if n, rowsErr := res.RowsAffected(); rowsErr == nil && n == 0 {
		return fmt.Errorf("lead not found: %w", sql.ErrNoRows)
	}
	return nil
}

func (r *leadPostgresRepository) List(ctx context.Context, tenantID string, filter lead.LeadFilter) ([]*lead.Lead, int64, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 10
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	args := make([]interface{}, 0, 18)
	args = append(args, tenantID)
	argN := 2

	var where strings.Builder
	where.WriteString(" WHERE tenant_id = $1 AND deleted_at IS NULL")

	if v := strings.TrimSpace(filter.DisplayID); v != "" {
		where.WriteString(fmt.Sprintf(" AND (metadata->>'DisplayID') = $%d", argN))
		args = append(args, v)
		argN++
	}
	if v := strings.TrimSpace(filter.Email); v != "" {
		where.WriteString(fmt.Sprintf(" AND (metadata->>'Email') = $%d", argN))
		args = append(args, v)
		argN++
	}
	if v := strings.TrimSpace(filter.Phone); v != "" {
		where.WriteString(fmt.Sprintf(" AND (metadata->>'Phone') = $%d", argN))
		args = append(args, v)
		argN++
	}
	if v := strings.TrimSpace(filter.AccountID); v != "" {
		where.WriteString(fmt.Sprintf(" AND (metadata->>'AccountID') = $%d", argN))
		args = append(args, v)
		argN++
	}
	if v := strings.TrimSpace(filter.ContractID); v != "" {
		where.WriteString(fmt.Sprintf(" AND (metadata->>'ContractID') = $%d", argN))
		args = append(args, v)
		argN++
	}

	if v := strings.TrimSpace(filter.Source); v != "" {
		where.WriteString(fmt.Sprintf(" AND (metadata->>'Source') = $%d", argN))
		args = append(args, v)
		argN++
	}
	if len(filter.SourceIn) > 0 {
		where.WriteString(fmt.Sprintf(" AND (metadata->>'Source') = ANY($%d)", argN))
		args = append(args, filter.SourceIn)
		argN++
	}

	if v := strings.TrimSpace(filter.Status); v != "" {
		where.WriteString(fmt.Sprintf(" AND (metadata->>'Status') = $%d", argN))
		args = append(args, v)
		argN++
	} else if len(filter.StatusIn) > 0 {
		where.WriteString(fmt.Sprintf(" AND (metadata->>'Status') = ANY($%d)", argN))
		args = append(args, filter.StatusIn)
		argN++
	}

	if filter.Qualified != nil && *filter.Qualified {
		where.WriteString(" AND (metadata->>'Status') = ANY(ARRAY['QUALIFIED','VISIT_SCHEDULED','PROPOSAL','NEGOTIATION'])")
	}

	if v := strings.TrimSpace(filter.Role); v != "" {
		where.WriteString(fmt.Sprintf(" AND (metadata->>'Role') = $%d", argN))
		args = append(args, v)
		argN++
	}
	if filter.IsClient != nil {
		where.WriteString(fmt.Sprintf(" AND (metadata->>'IsClient') = $%d", argN))
		args = append(args, fmt.Sprintf("%t", *filter.IsClient))
		argN++
	}

	if v := strings.TrimSpace(filter.AssignedBrokerID); v != "" {
		where.WriteString(fmt.Sprintf(" AND (metadata->>'AssignedBrokerID') = $%d", argN))
		args = append(args, v)
		argN++
	}
	if filter.Unassigned != nil {
		if *filter.Unassigned {
			where.WriteString(" AND COALESCE(metadata->>'AssignedBrokerID','') = ''")
		} else {
			where.WriteString(" AND COALESCE(metadata->>'AssignedBrokerID','') <> ''")
		}
	}
	if v := strings.TrimSpace(filter.BrokerScopeID); v != "" {
		where.WriteString(fmt.Sprintf(" AND ((metadata->>'AssignedBrokerID') = $%d OR COALESCE(metadata->>'AssignedBrokerID','') = '')", argN))
		args = append(args, v)
		argN++
	}

	if filter.AssignedFrom != nil {
		where.WriteString(fmt.Sprintf(" AND NULLIF(metadata->>'AssignedAt','')::timestamptz >= $%d", argN))
		args = append(args, *filter.AssignedFrom)
		argN++
	}
	if filter.AssignedTo != nil {
		where.WriteString(fmt.Sprintf(" AND NULLIF(metadata->>'AssignedAt','')::timestamptz <= $%d", argN))
		args = append(args, *filter.AssignedTo)
		argN++
	}
	if filter.ExpiresBefore != nil {
		where.WriteString(fmt.Sprintf(" AND NULLIF(metadata->>'ExpiresAt','')::timestamptz <= $%d", argN))
		args = append(args, *filter.ExpiresBefore)
		argN++
	}
	if filter.ExpiresAfter != nil {
		where.WriteString(fmt.Sprintf(" AND NULLIF(metadata->>'ExpiresAt','')::timestamptz >= $%d", argN))
		args = append(args, *filter.ExpiresAfter)
		argN++
	}

	if filter.EscalationLevelMin != nil {
		where.WriteString(fmt.Sprintf(" AND COALESCE(NULLIF(metadata->>'EscalationLevel','')::int, 0) >= $%d", argN))
		args = append(args, *filter.EscalationLevelMin)
		argN++
	}
	if filter.RouletteRetriesMin != nil {
		where.WriteString(fmt.Sprintf(" AND COALESCE(NULLIF(metadata->>'RouletteRetries','')::int, 0) >= $%d", argN))
		args = append(args, *filter.RouletteRetriesMin)
		argN++
	}
	if v := strings.TrimSpace(filter.TierID); v != "" {
		where.WriteString(fmt.Sprintf(" AND (metadata->'SLARetries') ? $%d", argN))
		args = append(args, v)
		argN++
	}

	if filter.CreatedFrom != nil {
		where.WriteString(fmt.Sprintf(" AND created_at >= $%d", argN))
		args = append(args, *filter.CreatedFrom)
		argN++
	}
	if filter.CreatedTo != nil {
		where.WriteString(fmt.Sprintf(" AND created_at <= $%d", argN))
		args = append(args, *filter.CreatedTo)
		argN++
	}
	if filter.UpdatedFrom != nil {
		where.WriteString(fmt.Sprintf(" AND updated_at >= $%d", argN))
		args = append(args, *filter.UpdatedFrom)
		argN++
	}
	if filter.UpdatedTo != nil {
		where.WriteString(fmt.Sprintf(" AND updated_at <= $%d", argN))
		args = append(args, *filter.UpdatedTo)
		argN++
	}

	if filter.LastInteractionFrom != nil {
		where.WriteString(fmt.Sprintf(" AND NULLIF(metadata->>'LastInteraction','')::timestamptz >= $%d", argN))
		args = append(args, *filter.LastInteractionFrom)
		argN++
	}
	if filter.LastInteractionTo != nil {
		where.WriteString(fmt.Sprintf(" AND NULLIF(metadata->>'LastInteraction','')::timestamptz <= $%d", argN))
		args = append(args, *filter.LastInteractionTo)
		argN++
	}
	if filter.StaleDays != nil && *filter.StaleDays > 0 {
		where.WriteString(fmt.Sprintf(" AND NULLIF(metadata->>'LastInteraction','')::timestamptz < NOW() - ($%d::int * INTERVAL '1 day')", argN))
		args = append(args, *filter.StaleDays)
		argN++
	}

	if v := strings.TrimSpace(filter.PropertyID); v != "" {
		where.WriteString(fmt.Sprintf(" AND (metadata->'PropertyIDs') @> $%d::jsonb", argN))
		args = append(args, fmt.Sprintf(`["%s"]`, v))
		argN++
	}
	if v := strings.TrimSpace(filter.CaptacaoPropertyID); v != "" {
		where.WriteString(fmt.Sprintf(" AND (metadata->>'CaptacaoPropertyID') = $%d", argN))
		args = append(args, v)
		argN++
	}
	if v := strings.TrimSpace(filter.OwnerLeadID); v != "" {
		where.WriteString(fmt.Sprintf(" AND (metadata->>'OwnerLeadID') = $%d", argN))
		args = append(args, v)
		argN++
	}
	if filter.IsCaptacao != nil {
		if *filter.IsCaptacao {
			where.WriteString(" AND COALESCE(metadata->>'CaptacaoPropertyID','') <> ''")
		} else {
			where.WriteString(" AND COALESCE(metadata->>'CaptacaoPropertyID','') = ''")
		}
	}

	interestTypeExpr := "COALESCE(metadata#>>'{PropertyInterest,type}', metadata#>>'{PropertyInterest,Type}')"
	interestLocExpr := "COALESCE(metadata#>>'{PropertyInterest,preferred_location}', metadata#>>'{PropertyInterest,PreferredLocation}')"
	interestMinPriceExpr := "COALESCE(NULLIF((metadata#>>'{PropertyInterest,price_range,min}')::numeric,0), NULLIF((metadata#>>'{PropertyInterest,PriceRange,Min}')::numeric,0), 0)"
	interestMaxPriceExpr := "COALESCE(NULLIF((metadata#>>'{PropertyInterest,price_range,max}')::numeric,0), NULLIF((metadata#>>'{PropertyInterest,PriceRange,Max}')::numeric,0), 0)"
	interestBedroomsExpr := "COALESCE(NULLIF((metadata#>>'{PropertyInterest,features,bedrooms}')::int,0), NULLIF((metadata#>>'{PropertyInterest,Features,Bedrooms}')::int,0), 0)"
	interestBathroomsExpr := "COALESCE(NULLIF((metadata#>>'{PropertyInterest,features,bathrooms}')::int,0), NULLIF((metadata#>>'{PropertyInterest,Features,Bathrooms}')::int,0), 0)"
	interestParkingExpr := "COALESCE(NULLIF((metadata#>>'{PropertyInterest,features,parking}')::int,0), NULLIF((metadata#>>'{PropertyInterest,Features,Parking}')::int,0), 0)"
	interestAreaMinExpr := "COALESCE(NULLIF((metadata#>>'{PropertyInterest,features,area_min}')::numeric,0), NULLIF((metadata#>>'{PropertyInterest,Features,AreaMin}')::numeric,0), 0)"
	interestAreaMaxExpr := "COALESCE(NULLIF((metadata#>>'{PropertyInterest,features,area_max}')::numeric,0), NULLIF((metadata#>>'{PropertyInterest,Features,AreaMax}')::numeric,0), 0)"

	if v := strings.TrimSpace(filter.InterestType); v != "" {
		where.WriteString(fmt.Sprintf(" AND %s = $%d", interestTypeExpr, argN))
		args = append(args, v)
		argN++
	}
	if v := strings.TrimSpace(filter.InterestLocation); v != "" {
		where.WriteString(fmt.Sprintf(" AND %s ILIKE $%d", interestLocExpr, argN))
		args = append(args, "%"+v+"%")
		argN++
	}
	if filter.InterestMinPrice != nil {
		where.WriteString(fmt.Sprintf(" AND %s >= $%d", interestMinPriceExpr, argN))
		args = append(args, *filter.InterestMinPrice)
		argN++
	}
	if filter.InterestMaxPrice != nil {
		where.WriteString(fmt.Sprintf(" AND %s <= $%d", interestMaxPriceExpr, argN))
		args = append(args, *filter.InterestMaxPrice)
		argN++
	}
	if filter.InterestBedroomsMin != nil {
		where.WriteString(fmt.Sprintf(" AND %s >= $%d", interestBedroomsExpr, argN))
		args = append(args, *filter.InterestBedroomsMin)
		argN++
	}
	if filter.InterestBathroomsMin != nil {
		where.WriteString(fmt.Sprintf(" AND %s >= $%d", interestBathroomsExpr, argN))
		args = append(args, *filter.InterestBathroomsMin)
		argN++
	}
	if filter.InterestParkingMin != nil {
		where.WriteString(fmt.Sprintf(" AND %s >= $%d", interestParkingExpr, argN))
		args = append(args, *filter.InterestParkingMin)
		argN++
	}
	if filter.InterestAreaMin != nil {
		where.WriteString(fmt.Sprintf(" AND %s >= $%d", interestAreaMinExpr, argN))
		args = append(args, *filter.InterestAreaMin)
		argN++
	}
	if filter.InterestAreaMax != nil {
		where.WriteString(fmt.Sprintf(" AND %s <= $%d", interestAreaMaxExpr, argN))
		args = append(args, *filter.InterestAreaMax)
		argN++
	}

	if v := strings.TrimSpace(filter.NoteSearch); v != "" {
		where.WriteString(fmt.Sprintf(" AND EXISTS (SELECT 1 FROM jsonb_array_elements(metadata->'History') h WHERE h->>'Details' ILIKE $%d)", argN))
		args = append(args, "%"+v+"%")
		argN++
	}

	if v := strings.TrimSpace(filter.Tag); v != "" {
		tag := strings.ToUpper(v)
		clauses := []string{
			fmt.Sprintf("UPPER(COALESCE(metadata->>'Source','')) = $%d", argN),
			fmt.Sprintf("UPPER(COALESCE(metadata->>'Role','')) = $%d", argN),
			fmt.Sprintf("UPPER(COALESCE(%s,'')) = $%d", interestTypeExpr, argN),
			fmt.Sprintf("COALESCE(%s,'') ILIKE $%d", interestLocExpr, argN+1),
		}
		if tag == "CAPTACAO" || tag == "CAPTAÇÃO" {
			clauses = append(clauses, "LOWER(COALESCE(metadata->>'Role','')) = 'owner'", "COALESCE(metadata->>'CaptacaoPropertyID','') <> ''")
		}
		if tag == "LOCACAO" || tag == "LOCAÇÃO" {
			clauses = append(clauses, "LOWER(COALESCE(metadata->>'Role','')) = 'tenant'")
		}
		if tag == "COMPRA_VENDA" || tag == "COMPRA E VENDA" {
			clauses = append(clauses, "LOWER(COALESCE(metadata->>'Role','')) = 'buyer'")
		}
		where.WriteString(" AND (" + strings.Join(clauses, " OR ") + ")")
		args = append(args, tag, "%"+tag+"%")
		argN += 2
	}

	if s := strings.TrimSpace(filter.Search); s != "" {
		where.WriteString(fmt.Sprintf(" AND ((metadata->>'Name') ILIKE $%d OR (metadata->>'Email') ILIKE $%d OR (metadata->>'Phone') ILIKE $%d OR (metadata->>'DisplayID') ILIKE $%d OR COALESCE(%s,'') ILIKE $%d)", argN, argN, argN, argN, interestLocExpr, argN))
		args = append(args, "%"+s+"%")
		argN++
	}

	var total int64
	countQuery := "SELECT COUNT(*) FROM crm.leads" + where.String()
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	where.WriteString(fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argN, argN+1))
	args = append(args, limit, offset)

	query := "SELECT metadata FROM crm.leads" + where.String()
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	result := make([]*lead.Lead, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var l lead.Lead
		if err := json.Unmarshal(metadata, &l); err != nil {
			return nil, 0, err
		}
		result = append(result, &l)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return result, total, nil
}

func (r *leadPostgresRepository) GetStats(ctx context.Context, tenantID string, brokerID string) (int64, map[string]int64, float64, error) {
	args := []any{tenantID}
	argN := 2
	where := "WHERE tenant_id = $1 AND deleted_at IS NULL"
	if brokerID != "" {
		where += fmt.Sprintf(" AND (metadata->>'AssignedBrokerID') = $%d", argN)
		args = append(args, brokerID)
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM crm.leads "+where, args...).Scan(&total); err != nil {
		return 0, nil, 0, err
	}

	rows, err := r.db.QueryContext(ctx, "SELECT (metadata->>'Status') AS status, COUNT(1) FROM crm.leads "+where+" GROUP BY (metadata->>'Status')", args...)
	if err != nil {
		return 0, nil, 0, err
	}
	defer rows.Close()

	statusCounts := map[string]int64{}
	var converted int64
	for rows.Next() {
		var status string
		var cnt int64
		if err := rows.Scan(&status, &cnt); err != nil {
			return 0, nil, 0, err
		}
		statusCounts[status] = cnt
		if status == string(lead.LeadStatusConverted) {
			converted = cnt
		}
	}
	if err := rows.Err(); err != nil {
		return 0, nil, 0, err
	}

	conversion := 0.0
	if total > 0 {
		conversion = float64(converted) / float64(total)
	}
	return total, statusCounts, conversion, nil
}

func (r *leadPostgresRepository) GetActiveLeadCountsByBroker(ctx context.Context, tenantID string, brokerIDs []string) (map[string]int, error) {
	if len(brokerIDs) == 0 {
		return map[string]int{}, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT (metadata->>'AssignedBrokerID') AS broker_id, COUNT(1)
		FROM crm.leads
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND (metadata->>'AssignedBrokerID') = ANY($2)
		  AND COALESCE(metadata->>'Status','') NOT IN ('CONVERTED','LOST')
		GROUP BY (metadata->>'AssignedBrokerID')
	`, tenantID, brokerIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var id string
		var cnt int
		if err := rows.Scan(&id, &cnt); err != nil {
			return nil, err
		}
		if id != "" {
			out[id] = cnt
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *leadPostgresRepository) ListForSLARedirect(ctx context.Context, tenantID string, statuses []string, maxRetries int, cutoff time.Time, limit int64) ([]*lead.Lead, error) {
	if limit <= 0 {
		limit = 50
	}
	if len(statuses) == 0 {
		statuses = []string{string(lead.LeadStatusPendingAcceptance)}
	}

	rows, err := r.db.QueryServiceRows(ctx, `
		SELECT metadata
		FROM crm.leads
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND (metadata->>'Status') = ANY($2)
		  AND COALESCE((metadata->>'RouletteRetries')::int, 0) < $3
		  AND (metadata->>'AssignedAt')::timestamptz <= $4
		ORDER BY (metadata->>'AssignedAt')::timestamptz ASC
		LIMIT $5
	`, tenantID, statuses, maxRetries, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*lead.Lead, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var l lead.Lead
		if err := json.Unmarshal(metadata, &l); err != nil {
			return nil, err
		}
		out = append(out, &l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ListFirstContactSLABreaches — Issue #427. Filtra leads que cruzaram
// 1h sem first_contact e ainda não foram marcados como SLA-violated.
// Status terminal (CONVERTED, LOST) excluído. Ordenado por AssignedAt
// ascendente para que os mais antigos sejam processados primeiro
// (mais críticos pro time de atendimento).
//
// ATENÇÃO: lead.Lead só tem tags `bson`, então json.Marshal usa o NOME
// do campo Go como chave JSONB (PascalCase) — não a forma snake_case da
// tag bson. Os filtros abaixo precisam usar 'FirstContactSLAViolatedAt'
// e 'AssignedAt'. Usar a chave errada torna o filtro de idempotência um
// no-op e o worker re-notifica o mesmo lead a cada tick (#427 flood).
func (r *leadPostgresRepository) ListFirstContactSLABreaches(ctx context.Context, tenantID string, cutoff time.Time, limit int64) ([]*lead.Lead, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := r.db.QueryServiceRows(ctx, `
		SELECT metadata
		FROM crm.leads
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND (metadata->>'AssignedAt') IS NOT NULL
		  AND (metadata->>'AssignedAt') != ''
		  AND (metadata->>'AssignedAt') != '0001-01-01T00:00:00Z'
		  AND (metadata->>'FirstContactAt') IS NULL
		  AND (metadata->>'FirstContactSLAViolatedAt') IS NULL
		  AND (metadata->>'Status') NOT IN ('CONVERTED', 'LOST')
		  AND (metadata->>'AssignedAt')::timestamptz <= $2
		ORDER BY (metadata->>'AssignedAt')::timestamptz ASC
		LIMIT $3
	`, tenantID, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*lead.Lead, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var l lead.Lead
		if err := json.Unmarshal(metadata, &l); err != nil {
			return nil, err
		}
		out = append(out, &l)
	}
	return out, rows.Err()
}

// ListSharedForPartner returns every non-deleted lead owned by ownerTenantID
// that opted into the partner shared-content feature.
//
// SECURITY: filters by ownerTenantID (the COUNTERPART tenant), NOT the JWT's
// tenant — it deliberately crosses tenant isolation. Only LeadService.
// SharedLeadsForCompany may call it, and only after verifying an active
// partnership grants the `leads` view scope.
//
// The opt-in flag is persisted under the lowercase `partnership_shared` JSONB
// key (the only json-tagged field on lead.Lead); every other key is PascalCase.
func (r *leadPostgresRepository) ListSharedForPartner(ctx context.Context, ownerTenantID string) ([]*lead.Lead, error) {
	if ownerTenantID == "" {
		return nil, fmt.Errorf("owner tenant_id required")
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT metadata
		FROM crm.leads
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND (metadata->>'partnership_shared') = 'true'
		ORDER BY created_at DESC
	`, ownerTenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*lead.Lead, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var l lead.Lead
		if err := json.Unmarshal(metadata, &l); err != nil {
			return nil, err
		}
		if l.TenantID == "" {
			l.TenantID = ownerTenantID
		}
		out = append(out, &l)
	}
	return out, rows.Err()
}

func (r *leadPostgresRepository) ReassignBrokerIfCurrent(ctx context.Context, leadID, tenantID, expectedBrokerID, newBrokerID, tierID, details string) (bool, error) {
	pool := r.db.ServicePool
	if pool == nil {
		pool = r.db.GetPool()
	}

	res, err := pool.Exec(ctx, `
		UPDATE crm.leads
		SET metadata = jsonb_set(metadata, '{AssignedBrokerID}', to_jsonb($1::text), true),
		    updated_at = NOW()
		WHERE id = $2
		  AND tenant_id = $3
		  AND (metadata->>'AssignedBrokerID') = $4
		  AND deleted_at IS NULL
	`, newBrokerID, leadID, tenantID, expectedBrokerID)
	if err != nil {
		return false, err
	}
	return res.RowsAffected() > 0, nil
}

func (r *leadPostgresRepository) CountUnassigned(ctx context.Context, tenantID string) (int64, error) {
	var total int64
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM crm.leads
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND COALESCE(metadata->>'AssignedBrokerID','') = ''
		  AND COALESCE(metadata->>'IsClient','false') = 'false'
	`, tenantID).Scan(&total)
	return total, err
}

func (r *leadPostgresRepository) ListUnassigned(ctx context.Context, tenantID string, limit int64) ([]*lead.Lead, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT metadata
		FROM crm.leads
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND COALESCE(metadata->>'AssignedBrokerID','') = ''
		  AND COALESCE(metadata->>'IsClient','false') = 'false'
		ORDER BY created_at DESC
		LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*lead.Lead, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var l lead.Lead
		if err := json.Unmarshal(metadata, &l); err != nil {
			return nil, err
		}
		out = append(out, &l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Worker-only cross-tenant scan; each operation is then scoped to its tenant.
func (r *leadPostgresRepository) ListPendingVisitOperations(ctx context.Context, limit int) ([]*lead.Lead, error) {
	rows, err := r.db.QueryServiceRows(ctx, `SELECT id, tenant_id FROM crm.leads WHERE deleted_at IS NULL
 AND metadata->'visit_operation' IS NOT NULL AND metadata->'visit_operation' <> 'null'::jsonb
 AND COALESCE((metadata->'visit_operation'->>'lease_until')::timestamptz,'epoch'::timestamptz)<=NOW()
 ORDER BY updated_at, id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	type scopedID struct{ id, tenant string }
	var ids []scopedID
	for rows.Next() {
		var id scopedID
		if err := rows.Scan(&id.id, &id.tenant); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	result := make([]*lead.Lead, 0, len(ids))
	for _, id := range ids {
		// O tenant de cada linha é explícito aqui: este caminho é o agregado
		// cross-tenant de parcerias (ListSharedForPartner), que já é protegido
		// por permissão no serviço de parcerias.
		l, err := r.GetByID(ctx, id.tenant, id.id)
		if err != nil {
			return nil, err
		}
		result = append(result, l)
	}
	return result, nil
}
