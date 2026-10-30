package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-calendar/internal/models/appointment"
)

type appointmentPostgresRepository struct {
	db *database.PostgresDB
}

func NewAppointmentPostgresRepository(db *database.PostgresDB) appointment.Repository {
	return &appointmentPostgresRepository{db: db}
}

func (r *appointmentPostgresRepository) Create(ctx context.Context, appt *appointment.Appointment) error {
	if appt.ID == "" {
		appt.ID = uuid.NewString()
	}
	appt.CreatedAt = time.Now()
	appt.UpdatedAt = appt.CreatedAt
	metadata, _ := json.Marshal(appt)
	query := `INSERT INTO calendar.appointments (id, tenant_id, metadata) VALUES ($1, $2, $3) RETURNING created_at`
	return r.reserveSlot(ctx, appt, "", func(tx *sqlx.Tx) error {
		return tx.QueryRowContext(ctx, query, appt.ID, appt.TenantID, string(metadata)).Scan(&appt.CreatedAt)
	})
}

// GetByIDForLink lê um compromisso a partir do link assinado do e-mail, onde
// não há sessão nem claims de tenant no contexto. Usa o pool de serviço porque a
// leitura no pool do app depende das claims do JWT e devolveria "nada" para uma
// linha que existe. A autorização continua sendo do serviço: o tenant vem da
// assinatura e é comparado com o do compromisso antes de qualquer resposta.
func (r *appointmentPostgresRepository) GetByIDForLink(ctx context.Context, id string) (*appointment.Appointment, error) {
	var metadata []byte
	err := r.db.QueryServiceRow(ctx, "SELECT metadata FROM calendar.appointments WHERE id = $1 AND deleted_at IS NULL", id).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("appointment not found: %w", err)
	}
	var a appointment.Appointment
	_ = json.Unmarshal(metadata, &a)
	return &a, nil
}

func (r *appointmentPostgresRepository) GetByID(ctx context.Context, id string) (*appointment.Appointment, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, "SELECT metadata FROM calendar.appointments WHERE id = $1 AND deleted_at IS NULL", id).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("appointment not found: %w", err)
	}
	var a appointment.Appointment
	_ = json.Unmarshal(metadata, &a)
	return &a, nil
}

func (r *appointmentPostgresRepository) Update(ctx context.Context, appt *appointment.Appointment) error {
	expected := appt.UpdatedAt
	appt.UpdatedAt = time.Now()
	metadata, _ := json.Marshal(appt)
	err := r.reserveSlot(ctx, appt, appt.ID, func(tx *sqlx.Tx) error {
		result, err := tx.ExecContext(ctx, "UPDATE calendar.appointments SET metadata = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL AND COALESCE(metadata->>'updated_at', '0001-01-01T00:00:00Z') = $4", string(metadata), appt.ID, appt.TenantID, expected.Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err == nil && rows == 0 {
			return appointment.ErrConcurrentModification
		}
		return err
	})
	if err != nil {
		appt.UpdatedAt = expected
	}
	return err
}

// Lock and conflict check must use the same transaction as the write.
// The transaction-scoped lock coordinates every calendar instance for the
// agency/agent/day, and is released on commit/rollback without a lease.
func (r *appointmentPostgresRepository) reserveSlot(ctx context.Context, appt *appointment.Appointment, excludeID string, write func(*sqlx.Tx) error) error {
	return r.db.RunInTx(ctx, func(tx *sqlx.Tx) error {
		// Inspection snapshots mirror a schedule already accepted by operations.
		// ScheduledBy is the organizer, not the external professional reserving a slot.
		if appt.InspectionID == "" && appt.AgentID != "" && appt.Status != appointment.StatusCancelado {
			key := appt.TenantID + "/" + appt.AgentID + "/" + appt.Date
			if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", key); err != nil {
				return err
			}
			var occupied bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS (
				SELECT 1 FROM calendar.appointments WHERE tenant_id = $1 AND deleted_at IS NULL
				AND metadata->>'agent_id' = $2 AND metadata->>'date' = $3
				AND COALESCE(metadata->>'status', '') <> 'Cancelado'
				AND metadata->>'time' < $5 AND metadata->>'end_time' > $4
				AND ($6 = '' OR id::text <> $6))`, appt.TenantID, appt.AgentID, appt.Date, appt.Time, appt.EndTime, excludeID).Scan(&occupied); err != nil {
				return err
			}
			if occupied {
				return appointment.ErrConflict
			}
		}
		return write(tx)
	})
}

func (r *appointmentPostgresRepository) Delete(ctx context.Context, tenantID, id string) error {
	// P2/item 36: o soft delete carrega o tenant na própria sentença.
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return fmt.Errorf("appointment not found: tenant context required")
	}
	_, err := r.db.ExecContext(ctx, "UPDATE calendar.appointments SET deleted_at = NOW() WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL", id, tenantID)
	return err
}

func (r *appointmentPostgresRepository) List(ctx context.Context, tenantID string, filters appointment.ListFilters) ([]*appointment.Appointment, int64, error) {
	limit := filters.Limit
	if limit <= 0 {
		limit = 10
	}
	offset := filters.Offset
	if offset < 0 {
		offset = 0
	}

	where := []string{"tenant_id = $1", "deleted_at IS NULL"}
	args := []any{tenantID}
	argN := 2

	if filters.AccessAgentID != "" {
		if filters.TeamScope {
			// Gestores veem a equipe, mas compromissos particulares continuam
			// exclusivos do responsável. Registros legados sem visibility só são
			// compartilhados quando têm um vínculo profissional verificável.
			where = append(where, fmt.Sprintf(`(
				(metadata->>'agent_id') = $%d OR (
					COALESCE(metadata->>'visibility', '') <> 'private' AND (
						(metadata->>'visibility') = 'team' OR
						COALESCE(metadata->>'lead_id', '') <> '' OR
						COALESCE(metadata->>'deal_id', '') <> '' OR
						COALESCE(metadata->>'customer_account_id', '') <> '' OR
						COALESCE(metadata->>'property_id', '') <> ''
					)
				)
			)`, argN))
		} else {
			where = append(where, fmt.Sprintf("(metadata->>'agent_id') = $%d", argN))
		}
		args = append(args, filters.AccessAgentID)
		argN++
	}

	if filters.AgentID != "" {
		where = append(where, fmt.Sprintf("(metadata->>'agent_id') = $%d", argN))
		args = append(args, filters.AgentID)
		argN++
	}
	if filters.PropertyID != "" {
		where = append(where, fmt.Sprintf("(metadata->>'property_id') = $%d", argN))
		args = append(args, filters.PropertyID)
		argN++
	}
	if filters.LeadID != "" {
		where = append(where, fmt.Sprintf("(metadata->>'lead_id') = $%d", argN))
		args = append(args, filters.LeadID)
		argN++
	}
	if filters.DealID != "" {
		where = append(where, fmt.Sprintf("(metadata->>'deal_id') = $%d", argN))
		args = append(args, filters.DealID)
		argN++
	}
	if filters.Date != "" {
		where = append(where, fmt.Sprintf("(metadata->>'date') = $%d", argN))
		args = append(args, filters.Date)
		argN++
	}
	if filters.StartDate != "" {
		where = append(where, fmt.Sprintf("(metadata->>'date') >= $%d", argN))
		args = append(args, filters.StartDate)
		argN++
	}
	if filters.EndDate != "" {
		where = append(where, fmt.Sprintf("(metadata->>'date') <= $%d", argN))
		args = append(args, filters.EndDate)
		argN++
	}
	if filters.Status != "" {
		where = append(where, fmt.Sprintf("(metadata->>'status') = $%d", argN))
		args = append(args, filters.Status)
		argN++
	}
	if filters.Type != "" {
		where = append(where, fmt.Sprintf("(metadata->>'type') = $%d", argN))
		args = append(args, filters.Type)
		argN++
	}
	if filters.ClientName != "" {
		where = append(where, fmt.Sprintf("(metadata->>'client') ILIKE $%d", argN))
		args = append(args, "%"+filters.ClientName+"%")
		argN++
	}
	if filters.AgentName != "" {
		where = append(where, fmt.Sprintf("(metadata->>'agent') ILIKE $%d", argN))
		args = append(args, "%"+filters.AgentName+"%")
		argN++
	}
	if filters.Priority != "" {
		where = append(where, fmt.Sprintf("(metadata->>'priority') = $%d", argN))
		args = append(args, filters.Priority)
		argN++
	}
	if s := strings.TrimSpace(filters.Search); s != "" {
		where = append(where, fmt.Sprintf("((metadata->>'title') ILIKE $%d OR (metadata->>'client') ILIKE $%d OR (metadata->>'agent') ILIKE $%d)", argN, argN, argN))
		args = append(args, "%"+s+"%")
		argN++
	}

	whereSQL := "WHERE " + strings.Join(where, " AND ")
	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM calendar.appointments "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf("SELECT metadata FROM calendar.appointments %s ORDER BY (metadata->>'date') DESC, (metadata->>'time') DESC, id DESC LIMIT $%d OFFSET $%d", whereSQL, argN, argN+1)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*appointment.Appointment, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var a appointment.Appointment
		if err := json.Unmarshal(metadata, &a); err != nil {
			return nil, 0, err
		}
		out = append(out, &a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return out, total, nil
}

func (r *appointmentPostgresRepository) FindConflicts(ctx context.Context, tenantID, agentID, date, startTime, endTime, excludeID string) ([]*appointment.Appointment, error) {
	args := []any{tenantID, agentID, date, startTime, endTime}
	where := `
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND (metadata->>'agent_id') = $2
		  AND (metadata->>'date') = $3
		  AND COALESCE(metadata->>'status','') <> 'Cancelado'
		  AND (metadata->>'time') < $5
		  AND (metadata->>'end_time') > $4
	`
	if excludeID != "" {
		args = append(args, excludeID)
		where += fmt.Sprintf(" AND id::text <> $%d", len(args))
	}

	rows, err := r.db.QueryContext(ctx, "SELECT metadata FROM calendar.appointments "+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*appointment.Appointment, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var a appointment.Appointment
		if err := json.Unmarshal(metadata, &a); err != nil {
			return nil, err
		}
		out = append(out, &a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
