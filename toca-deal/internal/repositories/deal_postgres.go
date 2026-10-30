package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	access "github.com/oginoino/toca/pkg/partnershipaccess"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

type dealPostgresRepository struct {
	db *database.PostgresDB
}

func NewDealPostgresRepository(db *database.PostgresDB) deal.Repository {
	return &dealPostgresRepository{db: db}
}

func (r *dealPostgresRepository) Create(ctx context.Context, d *deal.Deal) error {
	if d.ID == "" {
		d.ID = uuid.NewString()
	}
	d.CreatedAt = time.Now()
	return r.db.RunInTx(ctx, func(tx *sqlx.Tx) error {
		if d.CoBrokering {
			if err := validatePartnershipForCreation(ctx, tx, d); err != nil {
				return err
			}
		}
		metadata, err := marshalStoredDeal(d)
		if err != nil {
			return err
		}
		// ON CONFLICT DO NOTHING fecha a corrida de idempotência no banco: dois
		// envios simultâneos com a mesma chave (uq_deal_deals_idempotency_key)
		// ou o mesmo UUID inserem uma única linha; o perdedor relê o vencedor
		// pela chave e o devolve como replay idempotente.
		err = tx.QueryRowContext(ctx, `INSERT INTO deal.deals (id, tenant_id, metadata) VALUES ($1, $2, $3)
			ON CONFLICT DO NOTHING
			RETURNING created_at`, d.ID, d.TenantID, string(metadata)).Scan(&d.CreatedAt)
		if errors.Is(err, sql.ErrNoRows) {
			return deal.ErrIdempotencyConflict
		}
		return err
	})
}

// GetByIdempotencyKey devolve o negócio ativo criado com a chave. A consulta
// espelha o predicado do índice único: cancelados/removidos liberam a chave.
func (r *dealPostgresRepository) GetByIdempotencyKey(ctx context.Context, tenantID, idempotencyKey string) (*deal.Deal, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(idempotencyKey) == "" {
		return nil, nil
	}
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata FROM deal.deals
		WHERE tenant_id = $1 AND deleted_at IS NULL
		  AND (metadata->>'idempotency_key') = $2
		  AND COALESCE(metadata->>'status','') <> 'CANCELLED'
		ORDER BY created_at DESC LIMIT 1`, tenantID, idempotencyKey).Scan(&metadata)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("deal lookup by idempotency key: %w", err)
	}
	var d deal.Deal
	_ = unmarshalStoredDeal(metadata, &d)
	return &d, nil
}

// FindRecentDuplicate devolve o negócio ativo mais recente com o mesmo imóvel,
// cliente, tipo e valor criado a partir de `since` — a guarda de janela para
// clientes que não enviam chave de idempotência.
func (r *dealPostgresRepository) FindRecentDuplicate(ctx context.Context, tenantID, propertyID, customerID string, dealType deal.Type, value float64, since time.Time) (*deal.Deal, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(propertyID) == "" || strings.TrimSpace(customerID) == "" {
		return nil, nil
	}
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata FROM deal.deals
		WHERE tenant_id = $1 AND deleted_at IS NULL AND created_at >= $2
		  AND (metadata->>'property_id') = $3
		  AND (metadata->>'customer_id') = $4
		  AND (metadata->>'type') = $5
		  AND ABS(COALESCE((metadata->>'value')::numeric,0) - $6::numeric) < 0.005
		ORDER BY created_at DESC LIMIT 1`,
		tenantID, since, propertyID, customerID, string(dealType), value).Scan(&metadata)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("deal duplicate window lookup: %w", err)
	}
	var d deal.Deal
	_ = unmarshalStoredDeal(metadata, &d)
	return &d, nil
}

func (r *dealPostgresRepository) GetByID(ctx context.Context, tenantID, id string) (*deal.Deal, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, "SELECT metadata FROM deal.deals WHERE id = $1 AND tenant_id = $2", id, tenantID).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("deal not found: %w", err)
	}
	var d deal.Deal
	_ = unmarshalStoredDeal(metadata, &d)
	return &d, nil
}

// GetByIDInTx reads the deal via the caller's transaction. Required when the
// caller is already inside RunInTx and is about to UpdateInTx the same row —
// using the default pool here would acquire a second connection, and under
// load (many concurrent webhook subscribers each needing 2 conns) the pool
// can self-deadlock with every transaction holding one and waiting for
// another that will never come.
func (r *dealPostgresRepository) GetByIDInTx(ctx context.Context, tx *sqlx.Tx, tenantID, id string) (*deal.Deal, error) {
	if tx == nil {
		return nil, fmt.Errorf("transaction is nil; use GetByID for non-tx callers")
	}
	var metadata []byte
	err := tx.QueryRowxContext(ctx, "SELECT metadata FROM deal.deals WHERE id = $1 AND tenant_id = $2", id, tenantID).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("deal not found: %w", err)
	}
	var d deal.Deal
	_ = unmarshalStoredDeal(metadata, &d)
	return &d, nil
}

func (r *dealPostgresRepository) Update(ctx context.Context, d *deal.Deal) error {
	d.UpdatedAt = time.Now()
	metadata, _ := marshalStoredDeal(d)
	// B12: `AND tenant_id` como defesa em profundidade — um agregado montado a
	// partir de corpo de requisição (sem leitura prévia) não pode sobrescrever a
	// linha de outro tenant.
	res, err := r.db.ExecContext(ctx, "UPDATE deal.deals SET metadata = $1 WHERE id = $2 AND tenant_id = $3", string(metadata), d.ID, d.TenantID)
	if err != nil {
		return err
	}
	// Falha ALTA (e não no-op silencioso): 0 linhas significa id inexistente ou
	// tenant divergente entre o agregado e a linha. Antes, sem o predicado de
	// tenant, esse caso gravava na linha de outro tenant.
	if n, rowsErr := res.RowsAffected(); rowsErr == nil && n == 0 {
		return fmt.Errorf("deal not updated: id=%s tenant=%s not found", d.ID, d.TenantID)
	}
	return nil
}

// UpdateInTx mirrors Update but executes against the caller-supplied
// transaction. Used by ContractService.SignContract so the contract
// Update + deal Update (party/signer mirror) + outbox event all commit
// or roll back together. Audit P0.1 follow-up.
func (r *dealPostgresRepository) UpdateInTx(ctx context.Context, tx *sqlx.Tx, d *deal.Deal) error {
	if tx == nil {
		return fmt.Errorf("transaction is nil; use Update for non-tx callers")
	}
	d.UpdatedAt = time.Now()
	metadata, _ := marshalStoredDeal(d)
	res, err := tx.ExecContext(ctx, "UPDATE deal.deals SET metadata = $1 WHERE id = $2 AND tenant_id = $3", string(metadata), d.ID, d.TenantID)
	if err != nil {
		return err
	}
	if n, rowsErr := res.RowsAffected(); rowsErr == nil && n == 0 {
		return fmt.Errorf("deal not updated in tx: id=%s tenant=%s not found", d.ID, d.TenantID)
	}
	return nil
}

func (r *dealPostgresRepository) Delete(ctx context.Context, tenantID, id string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE deal.deals SET deleted_at = NOW() WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL", id, tenantID)
	return err
}

func (r *dealPostgresRepository) List(ctx context.Context, tenantID string, limit, offset int64) ([]*deal.Deal, int64, error) {
	return r.list(ctx, tenantID, "", nil, limit, offset)
}

func (r *dealPostgresRepository) GetByIDScoped(ctx context.Context, tenantID, id, accountID string) (*deal.Deal, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata
		FROM deal.deals
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL AND `+brokerVisibility("metadata", "$3")+`
	`, id, tenantID, accountID).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("deal not found: %w", err)
	}
	var d deal.Deal
	_ = unmarshalStoredDeal(metadata, &d)
	return &d, nil
}

func (r *dealPostgresRepository) DeleteScoped(ctx context.Context, tenantID, id, accountID string) error {
	// Excluir é do responsável: a participação dá visibilidade e trabalho no
	// negócio, não o poder de removê-lo do colega que o cadastrou.
	_, err := r.db.ExecContext(ctx, `
		UPDATE deal.deals
		SET deleted_at = NOW()
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL AND (metadata->>'broker_id') = $3
	`, id, tenantID, accountID)
	return err
}

func (r *dealPostgresRepository) ListScoped(ctx context.Context, tenantID, accountID string, limit, offset int64) ([]*deal.Deal, int64, error) {
	return r.list(ctx, tenantID, accountID, nil, limit, offset)
}

func (r *dealPostgresRepository) ListByCustomerIDs(ctx context.Context, tenantID string, customerIDs []string, limit, offset int64) ([]*deal.Deal, int64, error) {
	return r.list(ctx, tenantID, "", customerIDs, limit, offset)
}

func (r *dealPostgresRepository) ListByCustomerIDsScoped(ctx context.Context, tenantID, accountID string, customerIDs []string, limit, offset int64) ([]*deal.Deal, int64, error) {
	return r.list(ctx, tenantID, accountID, customerIDs, limit, offset)
}

func (r *dealPostgresRepository) Search(ctx context.Context, tenantID string, filter deal.DealFilter) ([]*deal.Deal, int64, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 10
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	args := []any{tenantID}
	argN := 2
	where := []string{"tenant_id = $1", "deleted_at IS NULL"}

	if filter.BrokerScopeID != "" {
		where = append(where, brokerVisibility("metadata", fmt.Sprintf("$%d", argN)))
		args = append(args, filter.BrokerScopeID)
		argN++
	}
	if filter.BrokerID != "" {
		// Filtro "negócios do corretor": mesma noção de carteira da listagem,
		// senão a tela filtraria por um corretor e esconderia o que ele divide.
		where = append(where, brokerVisibility("metadata", fmt.Sprintf("$%d", argN)))
		args = append(args, filter.BrokerID)
		argN++
	}
	if filter.Status != "" {
		where = append(where, fmt.Sprintf("(metadata->>'status') = $%d", argN))
		args = append(args, string(filter.Status))
		argN++
	}
	if filter.Type != "" {
		where = append(where, fmt.Sprintf("(metadata->>'type') = $%d", argN))
		args = append(args, string(filter.Type))
		argN++
	}

	if filter.MinValue != nil {
		where = append(where, fmt.Sprintf("COALESCE((metadata->>'value')::numeric, 0) >= $%d", argN))
		args = append(args, *filter.MinValue)
		argN++
	}
	if filter.MaxValue != nil {
		where = append(where, fmt.Sprintf("COALESCE((metadata->>'value')::numeric, 0) <= $%d", argN))
		args = append(args, *filter.MaxValue)
		argN++
	}

	addressExpr := "COALESCE(metadata#>>'{property,address,full}', metadata#>>'{property,address_full}', metadata->>'property_address', '')"

	if filter.PropertyID != "" {
		where = append(where, fmt.Sprintf("(metadata->>'property_id') = $%d", argN))
		args = append(args, filter.PropertyID)
		argN++
	}
	if v := strings.TrimSpace(filter.PropertyAddress); v != "" {
		where = append(where, fmt.Sprintf("%s ILIKE $%d", addressExpr, argN))
		args = append(args, "%"+v+"%")
		argN++
	}
	if v := strings.TrimSpace(filter.PartyName); v != "" {
		where = append(where, fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements(metadata->'parties') p WHERE p->>'name' ILIKE $%d)", argN))
		args = append(args, "%"+v+"%")
		argN++
	}
	if v := strings.TrimSpace(filter.PartyEmail); v != "" {
		where = append(where, fmt.Sprintf("(EXISTS (SELECT 1 FROM jsonb_array_elements(metadata->'parties') p WHERE p->>'email' ILIKE $%d) OR EXISTS (SELECT 1 FROM jsonb_array_elements(metadata->'signers') s WHERE s->>'email' ILIKE $%d))", argN, argN))
		args = append(args, "%"+v+"%")
		argN++
	}
	if v := strings.TrimSpace(filter.LeadID); v != "" {
		where = append(where, fmt.Sprintf("((metadata->>'lead_id') = $%d OR (metadata->>'customer_id') = $%d OR (metadata->>'owner_id') = $%d OR (metadata->'parties') @> jsonb_build_array(jsonb_build_object('lead_id', $%d::text)))", argN, argN, argN, argN))
		args = append(args, v)
		argN++
	}

	if filter.CreatedFrom != nil {
		where = append(where, fmt.Sprintf("created_at >= $%d", argN))
		args = append(args, *filter.CreatedFrom)
		argN++
	}
	if filter.CreatedTo != nil {
		where = append(where, fmt.Sprintf("created_at <= $%d", argN))
		args = append(args, *filter.CreatedTo)
		argN++
	}

	if len(filter.CustomerIDs) > 0 {
		where = append(where, fmt.Sprintf("(metadata->>'customer_id') = ANY($%d)", argN))
		args = append(args, filter.CustomerIDs)
		argN++
	}

	if s := strings.TrimSpace(filter.Search); s != "" {
		where = append(where, fmt.Sprintf("(%s ILIKE $%d OR EXISTS (SELECT 1 FROM jsonb_array_elements(metadata->'parties') p WHERE p->>'name' ILIKE $%d))", addressExpr, argN, argN))
		args = append(args, "%"+s+"%")
		argN++
	}

	whereSQL := "WHERE " + strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM deal.deals "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf("SELECT metadata FROM deal.deals %s ORDER BY created_at DESC LIMIT $%d::bigint OFFSET $%d::bigint", whereSQL, argN, argN+1)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*deal.Deal, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var d deal.Deal
		if err := unmarshalStoredDeal(metadata, &d); err != nil {
			return nil, 0, err
		}
		out = append(out, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *dealPostgresRepository) FindBySignerEmail(ctx context.Context, email string) ([]*deal.Deal, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT metadata
		FROM deal.deals
		WHERE deleted_at IS NULL
		  AND (metadata->'signers') @> jsonb_build_array(jsonb_build_object('email', $1::text))
	`, email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*deal.Deal, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var d deal.Deal
		if err := unmarshalStoredDeal(metadata, &d); err != nil {
			return nil, err
		}
		out = append(out, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// portalIdentityMatchClauses monta o predicado de identidade do cliente sem
// conhecer o tenant: a conta do portal é local ao cliente, e o negócio pode
// estar em qualquer imobiliária. A parte pode existir antes do signatário, por
// isso as duas coleções são consultadas. O e-mail é case-insensitive.
func portalIdentityMatchClauses(document, email string) ([]string, []any) {
	if document == "" && email == "" {
		return nil, nil
	}

	args := make([]any, 0, 2)
	documentArg := 0
	emailArg := 0
	if document != "" {
		args = append(args, document)
		documentArg = len(args)
	}
	if email != "" {
		args = append(args, email)
		emailArg = len(args)
	}

	match := func(collection string) string {
		parts := make([]string, 0, 2)
		if document != "" {
			parts = append(parts, fmt.Sprintf(
				"COALESCE(value->>'account_id', '') = $%d", documentArg,
			))
		}
		if email != "" {
			parts = append(parts, fmt.Sprintf(
				"lower(COALESCE(value->>'email', '')) = lower($%d)", emailArg,
			))
		}
		return fmt.Sprintf(
			"EXISTS (SELECT 1 FROM jsonb_array_elements(COALESCE(metadata->'%s', '[]'::jsonb)) AS item(value) WHERE %s)",
			collection,
			strings.Join(parts, " OR "),
		)
	}

	return []string{match("parties"), match("signers")}, args
}

func (r *dealPostgresRepository) FindBySignerIdentity(ctx context.Context, document, email string) ([]*deal.Deal, error) {
	clauses, args := portalIdentityMatchClauses(document, email)
	if len(clauses) == 0 {
		return []*deal.Deal{}, nil
	}

	query := "SELECT metadata FROM deal.deals WHERE deleted_at IS NULL AND (" + strings.Join(clauses, " OR ") + ") ORDER BY created_at DESC, id"

	// POOL DE SERVIÇO (BYPASSRLS) explícito: o portal do cliente é cross-tenant
	// por definição (o tenant do token é a conta local do cliente, o negócio é
	// da imobiliária) e a RLS de `deal.deals` filtra por tenant — sob o pool de
	// aplicação a lista voltava vazia e o cliente perdia o acompanhamento do
	// negócio. A autorização é a própria identidade da parte/signatário aqui
	// (account ou e-mail do token), então nada é exposto além do negócio do
	// próprio cliente. É o mesmo arranjo do toca-crm para as leituras do portal.
	rows, err := r.db.QueryServiceRows(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*deal.Deal, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var d deal.Deal
		if err := unmarshalStoredDeal(metadata, &d); err != nil {
			return nil, err
		}
		out = append(out, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		logger.Warn("deal portal identity lookup returned no deals",
			"has_account", document != "", "has_email", email != "",
			"service_tx", database.ServiceTxFromCtx(ctx),
			"row_security_bypass", r.db.ServicePool != nil)
	}
	return out, nil
}

// ListForPortalAccountRepair devolve, pelo POOL DE SERVIÇO, os negócios ativos
// que ainda têm parte/signatário com e-mail e sem conta de portal. É a lista do
// cmd/repair-portal-accounts: o reparo roda sem claims de tenant, então a RLS do
// pool de aplicação devolveria zero linhas em silêncio.
func (r *dealPostgresRepository) ListForPortalAccountRepair(ctx context.Context, tenantID string, limit int) ([]*deal.Deal, error) {
	if limit <= 0 {
		limit = 500
	}
	query := `
		SELECT metadata
		  FROM deal.deals
		 WHERE deleted_at IS NULL
		   AND (
		     EXISTS (
		       SELECT 1 FROM jsonb_array_elements(COALESCE(metadata->'parties', '[]'::jsonb)) AS p
		        WHERE COALESCE(p->>'email', '') <> ''
		          AND COALESCE(p->>'account_id', '') = ''
		     )
		     OR EXISTS (
		       SELECT 1 FROM jsonb_array_elements(COALESCE(metadata->'signers', '[]'::jsonb)) AS s
		        WHERE COALESCE(s->>'email', '') <> ''
		          AND COALESCE(s->>'account_id', '') = ''
		     )
		   )
	`
	args := []any{}
	if strings.TrimSpace(tenantID) != "" {
		query += " AND tenant_id = $1"
		args = append(args, tenantID)
	}
	query += fmt.Sprintf(" ORDER BY created_at ASC, id ASC LIMIT %d", limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*deal.Deal, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var d deal.Deal
		if err := unmarshalStoredDeal(metadata, &d); err != nil {
			return nil, err
		}
		out = append(out, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *dealPostgresRepository) GetStats(ctx context.Context, tenantID string) (int64, float64, error) {
	return r.getStats(ctx, tenantID, "")
}

func (r *dealPostgresRepository) GetStatsScoped(ctx context.Context, tenantID, accountID string) (int64, float64, error) {
	return r.getStats(ctx, tenantID, accountID)
}

func (r *dealPostgresRepository) FindByPartyAccountID(ctx context.Context, accountID string) ([]*deal.Deal, error) {
	if accountID == "" {
		return []*deal.Deal{}, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT metadata
		FROM deal.deals
		WHERE deleted_at IS NULL
		  AND (
		    (metadata->'parties') @> jsonb_build_array(jsonb_build_object('account_id', $1::text))
		    OR (metadata->'signers') @> jsonb_build_array(jsonb_build_object('account_id', $1::text))
		  )
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*deal.Deal, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var d deal.Deal
		if err := unmarshalStoredDeal(metadata, &d); err != nil {
			return nil, err
		}
		out = append(out, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ListProposalsAwaitingResponse devolve, cross-tenant pelo POOL DE SERVIÇO, os
// negócios com pelo menos uma proposta aguardando resposta (SENT ou
// COUNTER_OFFER) enviada dentro da janela [notOlderThan, olderThan]. O filtro é
// deliberadamente largo: o motor §F6 decide em Go qual estágio da cadência está
// devido, porque o estágio depende de `follow_ups`/`follow_up_last_at` gravados
// no próprio JSONB da proposta.
//
// `olderThan` normalmente é now - stage1Days: nada mais novo que isso pode ter
// lembrete devido, o que evita varrer a base inteira a cada tick. `notOlderThan`
// é o teto de idade (now - maxAgeDays): proposta parada há meses não recebe
// lembrete nenhum — evita rajada de e-mails antigos no primeiro deploy.
//
// RLS: este é um worker sem tenant no contexto (roda no boot, com
// context.Background). Como `deal.deals` é deny-by-default para o pool de
// aplicação, a leitura PRECISA sair pelo pool de serviço — do contrário a
// varredura volta vazia e a régua nunca dispara (foi exatamente o que o smoke
// no beta pegou).
func (r *dealPostgresRepository) ListProposalsAwaitingResponse(ctx context.Context, olderThan, notOlderThan time.Time, limit int) ([]*deal.Deal, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.db.QueryServiceRows(ctx, `
		SELECT d.metadata
		FROM deal.deals d
		WHERE d.deleted_at IS NULL
		  AND COALESCE(d.metadata->>'status', '') NOT IN ('CONVERTED', 'CANCELLED')
		  AND EXISTS (
		    SELECT 1
		    FROM jsonb_array_elements(COALESCE(d.metadata->'proposals', '[]'::jsonb)) AS p
		    WHERE p->>'status' IN ('SENT', 'COUNTER_OFFER')
		      AND COALESCE(NULLIF(p->>'created_at', '')::timestamptz, d.created_at) <= $1
		      AND COALESCE(NULLIF(p->>'created_at', '')::timestamptz, d.created_at) > $2
		  )
		ORDER BY d.created_at ASC
		LIMIT $3
	`, olderThan, notOlderThan, limit)
	if err != nil {
		return nil, fmt.Errorf("list proposals awaiting response: %w", err)
	}
	defer rows.Close()

	out := make([]*deal.Deal, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var d deal.Deal
		if err := unmarshalStoredDeal(metadata, &d); err != nil {
			return nil, err
		}
		out = append(out, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// MarkProposalFollowUp grava o estágio disparado direto no elemento da proposta
// (`follow_ups` + `follow_up_last_at`), pelo pool de serviço.
//
// Dois motivos para não reusar Update:
//   - RLS: o worker não tem tenant no contexto, então um UPDATE pelo pool de
//     aplicação não afetaria linha nenhuma;
//   - concorrência: reescrever o documento inteiro a partir do agregado lido no
//     início do tick sobrescreveria mudanças feitas nesse meio-tempo (aceite da
//     proposta, novo signatário, etc.). O jsonb_set toca só a proposta alvo.
func (r *dealPostgresRepository) MarkProposalFollowUp(ctx context.Context, tenantID, dealID, proposalID string, followUps int, at time.Time) error {
	if tenantID == "" || dealID == "" || proposalID == "" {
		return fmt.Errorf("mark proposal follow-up: missing ids")
	}
	tag, err := r.db.ExecServiceRows(ctx, `
		WITH target AS (
		  SELECT (ord - 1)::int AS idx
		  FROM deal.deals d
		  CROSS JOIN LATERAL jsonb_array_elements(COALESCE(d.metadata->'proposals', '[]'::jsonb))
		    WITH ORDINALITY AS t(p, ord)
		  WHERE d.id = $1 AND d.tenant_id = $2 AND p->>'id' = $3
		  LIMIT 1
		)
		UPDATE deal.deals d
		SET metadata = jsonb_set(
		      d.metadata,
		      ARRAY['proposals', target.idx::text],
		      (d.metadata->'proposals'->target.idx)
		        || jsonb_build_object('follow_ups', $4::int, 'follow_up_last_at', $5::text),
		      false)
		FROM target
		WHERE d.id = $1 AND d.tenant_id = $2
	`, dealID, tenantID, proposalID, followUps, at.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("mark proposal follow-up: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("mark proposal follow-up: deal=%s proposal=%s not found", dealID, proposalID)
	}
	return nil
}

func (r *dealPostgresRepository) FindByPartyEmail(ctx context.Context, email string) ([]*deal.Deal, error) {
	if email == "" {
		return []*deal.Deal{}, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT metadata
		FROM deal.deals
		WHERE deleted_at IS NULL
		  AND (
		    (metadata->'parties') @> jsonb_build_array(jsonb_build_object('email', $1::text))
		    OR (metadata->'signers') @> jsonb_build_array(jsonb_build_object('email', $1::text))
		  )
	`, email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*deal.Deal, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var d deal.Deal
		if err := unmarshalStoredDeal(metadata, &d); err != nil {
			return nil, err
		}
		out = append(out, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *dealPostgresRepository) FindByPartyLeadID(ctx context.Context, leadID string) ([]*deal.Deal, error) {
	if leadID == "" {
		return []*deal.Deal{}, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT metadata
		FROM deal.deals
		WHERE deleted_at IS NULL
		  AND (metadata->'parties') @> jsonb_build_array(jsonb_build_object('lead_id', $1::text))
	`, leadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*deal.Deal, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var d deal.Deal
		if err := unmarshalStoredDeal(metadata, &d); err != nil {
			return nil, err
		}
		out = append(out, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *dealPostgresRepository) list(ctx context.Context, tenantID string, brokerID string, customerIDs []string, limit, offset int64) ([]*deal.Deal, int64, error) {
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}

	args := []any{tenantID}
	argN := 2
	where := "WHERE tenant_id = $1 AND deleted_at IS NULL"

	if brokerID != "" {
		where += " AND " + brokerVisibility("metadata", fmt.Sprintf("$%d", argN))
		args = append(args, brokerID)
		argN++
	}
	if len(customerIDs) > 0 {
		where += fmt.Sprintf(" AND (metadata->>'customer_id') = ANY($%d)", argN)
		args = append(args, customerIDs)
		argN++
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM deal.deals "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf("SELECT metadata FROM deal.deals %s ORDER BY created_at DESC LIMIT $%d::bigint OFFSET $%d::bigint", where, argN, argN+1)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*deal.Deal, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var d deal.Deal
		if err := unmarshalStoredDeal(metadata, &d); err != nil {
			return nil, 0, err
		}
		out = append(out, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *dealPostgresRepository) getStats(ctx context.Context, tenantID string, brokerID string) (int64, float64, error) {
	args := []any{tenantID}
	argN := 2
	where := "WHERE tenant_id = $1 AND deleted_at IS NULL"
	if brokerID != "" {
		where += " AND " + brokerVisibility("metadata", fmt.Sprintf("$%d", argN))
		args = append(args, brokerID)
	}

	var proposals int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM deal.deals "+where+" AND (metadata->>'status') = 'PROPOSAL'", args...).Scan(&proposals); err != nil {
		return 0, 0, err
	}

	var sales float64
	_ = r.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM((metadata->>'value')::numeric), 0)
		FROM deal.deals
		`+where+`
		  AND (metadata->>'status') = 'CONVERTED'
		  AND date_trunc('month', (metadata->>'updated_at')::timestamptz) = date_trunc('month', NOW())
	`, args...).Scan(&sales)

	return proposals, sales, nil
}

// Keep the agreement and shared resources locked until the new deal commits.
// A concurrent revocation either wins before this check, or waits until the
// already-authorized deal (and its accepted financial snapshot) is persisted.
func validatePartnershipForCreation(ctx context.Context, tx *sqlx.Tx, d *deal.Deal) error {
	denied := errors.New("o compartilhamento foi alterado; revise os dados antes de criar o negócio")
	if d.PartnershipSnapshot == nil {
		return denied
	}
	var metadata []byte
	if err := tx.QueryRowContext(ctx, `SELECT metadata FROM core.partnerships WHERE id=$1 AND status='active' AND deleted_at IS NULL FOR SHARE`, d.PartnershipSnapshot.ID).Scan(&metadata); err != nil {
		return fmt.Errorf("partnership unavailable: %w", err)
	}
	var p CrossPartnership
	if err := json.Unmarshal(metadata, &p); err != nil {
		return err
	}
	if !((p.RequesterID == d.TenantID && p.TargetID == d.SourceCompanyID) || (p.TargetID == d.TenantID && p.RequesterID == d.SourceCompanyID)) || p.FinancialVersion != d.PartnershipSnapshot.Version {
		return denied
	}
	grant := p.GrantFrom(d.SourceCompanyID)
	if !grant.Sharing.AllowCoBrokering {
		return denied
	}
	foundShared := false
	for _, item := range []struct{ kind, id string }{{"property", d.PropertyID}, {"lead", d.LeadID}} {
		if item.id == "" {
			continue
		}
		query := `SELECT tenant_id::text,metadata FROM core.properties WHERE id=$1 AND deleted_at IS NULL FOR SHARE`
		if item.kind == "lead" {
			query = `SELECT tenant_id::text,metadata FROM crm.leads WHERE id=$1 AND deleted_at IS NULL FOR SHARE`
		}
		var owner string
		if err := tx.QueryRowContext(ctx, query, item.id).Scan(&owner, &metadata); err != nil {
			return err
		}
		if owner == d.TenantID {
			continue
		}
		if owner != d.SourceCompanyID {
			return denied
		}
		foundShared = true
		if item.kind == "property" {
			var property access.Property
			if err := json.Unmarshal(metadata, &property); err != nil {
				return err
			}
			if !grant.IncludesProperty(property) {
				return denied
			}
		} else {
			var lead struct {
				Shared bool `json:"partnership_shared"`
			}
			if err := json.Unmarshal(metadata, &lead); err != nil {
				return err
			}
			if !lead.Shared || !grant.Allows("leads") {
				return denied
			}
		}
	}
	if !foundShared {
		return denied
	}
	d.PartnershipSnapshot = p.Snapshot()
	return nil
}

// Financial agreement terms are storage-only; ordinary deal API responses must
// not expose this internal snapshot to users who can read deals.
type storedDeal struct {
	*deal.Deal
	PartnershipSnapshot *access.Snapshot `json:"partnership_snapshot,omitempty"`
}

func marshalStoredDeal(d *deal.Deal) ([]byte, error) {
	return json.Marshal(storedDeal{Deal: d, PartnershipSnapshot: d.PartnershipSnapshot})
}
func unmarshalStoredDeal(data []byte, d *deal.Deal) error {
	stored := storedDeal{Deal: d}
	if err := json.Unmarshal(data, &stored); err != nil {
		return err
	}
	d.PartnershipSnapshot = stored.PartnershipSnapshot
	return nil
}
