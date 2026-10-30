package database

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

type FilterOp int

const (
	OpEq FilterOp = iota
	OpIn
	OpGte
	OpLte
	OpILike
	OpContains
	OpExists
)

type Filter struct {
	Field   string
	Op      FilterOp
	Value   any
	IsJSONB bool
}

type ListOptions struct {
	TenantID  string
	Filters   []Filter
	Limit     int64
	Offset    int64
	OrderBy   string
	OrderDesc bool
}

type Row struct {
	ID        string
	Metadata  []byte
	CreatedAt time.Time
	UpdatedAt time.Time
}

type JSONBQuery struct {
	db         *PostgresDB
	schema     string
	table      string
	softDelete bool
}

var identRx = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var jsonKeyRx = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

func NewJSONBQuery(db *PostgresDB, schema, table string, softDelete bool) (*JSONBQuery, error) {
	if db == nil {
		return nil, fmt.Errorf("db is required")
	}
	if !identRx.MatchString(schema) {
		return nil, fmt.Errorf("invalid schema")
	}
	if !identRx.MatchString(table) {
		return nil, fmt.Errorf("invalid table")
	}
	return &JSONBQuery{db: db, schema: schema, table: table, softDelete: softDelete}, nil
}

func (q *JSONBQuery) List(ctx context.Context, opts ListOptions) ([]Row, int64, error) {
	if opts.TenantID == "" {
		return nil, 0, fmt.Errorf("tenant_id required")
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 10
	}
	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}

	whereSQL, args, err := q.buildWhere(opts.TenantID, opts.Filters)
	if err != nil {
		return nil, 0, err
	}

	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(1) FROM %s.%s %s", q.schema, q.table, whereSQL)
	if err := q.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	orderBy := "created_at"
	if opts.OrderBy != "" {
		if !identRx.MatchString(opts.OrderBy) {
			return nil, 0, fmt.Errorf("invalid order_by")
		}
		orderBy = opts.OrderBy
	}
	dir := "ASC"
	if opts.OrderDesc {
		dir = "DESC"
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf(
		"SELECT id, metadata, created_at, updated_at FROM %s.%s %s ORDER BY %s %s LIMIT $%d OFFSET $%d",
		q.schema,
		q.table,
		whereSQL,
		orderBy,
		dir,
		len(args)-1,
		len(args),
	)

	rows, err := q.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]Row, 0)
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.ID, &r.Metadata, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return out, total, nil
}

func (q *JSONBQuery) Count(ctx context.Context, tenantID string, filters []Filter) (int64, error) {
	if tenantID == "" {
		return 0, fmt.Errorf("tenant_id required")
	}
	whereSQL, args, err := q.buildWhere(tenantID, filters)
	if err != nil {
		return 0, err
	}
	var total int64
	query := fmt.Sprintf("SELECT COUNT(1) FROM %s.%s %s", q.schema, q.table, whereSQL)
	if err := q.db.QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func (q *JSONBQuery) GetByIDs(ctx context.Context, tenantID string, ids []string) ([]Row, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("tenant_id required")
	}
	if len(ids) == 0 {
		return []Row{}, nil
	}
	args := []any{tenantID, ids}
	where := "WHERE tenant_id = $1 AND id = ANY($2)"
	if q.softDelete {
		where += " AND deleted_at IS NULL"
	}

	query := fmt.Sprintf("SELECT id, metadata, created_at, updated_at FROM %s.%s %s", q.schema, q.table, where)
	rows, err := q.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Row, 0, len(ids))
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.ID, &r.Metadata, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (q *JSONBQuery) buildWhere(tenantID string, filters []Filter) (string, []any, error) {
	whereParts := []string{"WHERE tenant_id = $1"}
	args := []any{tenantID}
	argN := 2

	if q.softDelete {
		whereParts = append(whereParts, "deleted_at IS NULL")
	}

	for _, f := range filters {
		if f.Field == "" {
			return "", nil, fmt.Errorf("filter field required")
		}
		expr, err := q.filterExpr(f)
		if err != nil {
			return "", nil, err
		}

		switch f.Op {
		case OpEq:
			whereParts = append(whereParts, fmt.Sprintf("%s = $%d", expr, argN))
			args = append(args, f.Value)
			argN++
		case OpIn:
			whereParts = append(whereParts, fmt.Sprintf("%s = ANY($%d)", expr, argN))
			args = append(args, f.Value)
			argN++
		case OpGte:
			whereParts = append(whereParts, fmt.Sprintf("%s >= $%d", expr, argN))
			args = append(args, f.Value)
			argN++
		case OpLte:
			whereParts = append(whereParts, fmt.Sprintf("%s <= $%d", expr, argN))
			args = append(args, f.Value)
			argN++
		case OpILike:
			whereParts = append(whereParts, fmt.Sprintf("%s ILIKE $%d", expr, argN))
			args = append(args, f.Value)
			argN++
		case OpContains:
			if !f.IsJSONB {
				return "", nil, fmt.Errorf("contains op requires jsonb")
			}
			if !jsonKeyRx.MatchString(f.Field) {
				return "", nil, fmt.Errorf("invalid json key")
			}
			whereParts = append(whereParts, fmt.Sprintf("metadata->'%s' @> $%d::jsonb", f.Field, argN))
			args = append(args, f.Value)
			argN++
		case OpExists:
			if !f.IsJSONB {
				return "", nil, fmt.Errorf("exists op requires jsonb")
			}
			if !jsonKeyRx.MatchString(f.Field) {
				return "", nil, fmt.Errorf("invalid json key")
			}
			whereParts = append(whereParts, fmt.Sprintf("metadata ? '%s'", f.Field))
		default:
			return "", nil, fmt.Errorf("unsupported filter op")
		}
	}

	return strings.Join(whereParts, " AND "), args, nil
}

func (q *JSONBQuery) filterExpr(f Filter) (string, error) {
	if f.IsJSONB {
		if !jsonKeyRx.MatchString(f.Field) {
			return "", fmt.Errorf("invalid json key")
		}
		return fmt.Sprintf("metadata->>'%s'", f.Field), nil
	}
	if !identRx.MatchString(f.Field) {
		return "", fmt.Errorf("invalid column")
	}
	return f.Field, nil
}
