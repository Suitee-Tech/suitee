package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"time"

	"github.com/oginoino/toca-core/internal/domain/core/partnership"
	"github.com/oginoino/toca/pkg/database"
)

type partnershipPostgresRepository struct {
	db *database.PostgresDB
}

func NewPartnershipPostgresRepository(db *database.PostgresDB) PartnershipRepository {
	return &partnershipPostgresRepository{db: db}
}

// Reads expose only the public identity of participant companies, never members or contacts.
const partnershipProjection = `metadata || jsonb_build_object('companies', jsonb_build_object(
 requester_id::text, (SELECT jsonb_build_object('id',a.id,'name',a.metadata#>>'{Company,Name}','tax_id',a.metadata#>>'{Company,TaxID}','city',a.metadata#>>'{Company,Address,city}','state',a.metadata#>>'{Company,Address,state}','active',a.metadata#>'{Company,IsActive}') FROM toca_auth.accounts a WHERE a.id=requester_id),
 target_id::text, (SELECT jsonb_build_object('id',a.id,'name',a.metadata#>>'{Company,Name}','tax_id',a.metadata#>>'{Company,TaxID}','city',a.metadata#>>'{Company,Address,city}','state',a.metadata#>>'{Company,Address,state}','active',a.metadata#>'{Company,IsActive}') FROM toca_auth.accounts a WHERE a.id=target_id)))`

func enqueuePartnership(ctx context.Context, db *database.PostgresDB, tx *sqlx.Tx, p *partnership.Partnership) error {
	var evt events.Event
	switch p.EventAction {
	case "invited":
		evt = contracts.PartnershipInvitedEvent{Data: contracts.PartnershipInvitedEventPayload{PartnershipID: p.ID, RequesterID: p.RequesterID, TargetID: p.TargetID, Type: string(p.Type), CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339)}}
	case "accepted":
		evt = contracts.PartnershipAcceptedEvent{Data: contracts.PartnershipAcceptedEventPayload{PartnershipID: p.ID, RequesterID: p.RequesterID, TargetID: p.TargetID, Type: string(p.Type), AcceptedAt: p.UpdatedAt.UTC().Format(time.RFC3339)}}
	}
	// EnqueueOutboxInTxService roteia a linha do outbox para o pool de serviço
	// (events.outbox_events é deny-by-default no pool de aplicação).
	if evt != nil {
		if _, err := events.EnqueueOutboxInTxService(ctx, tx, db, evt); err != nil {
			return err
		}
	}
	_, err := events.EnqueueOutboxInTxService(ctx, tx, db, contracts.PartnershipChangedEvent{ID: uuid.NewString(), Data: map[string]interface{}{"partnership_id": p.ID, "requester_id": p.RequesterID, "target_id": p.TargetID, "version": p.Version}})
	return err
}
func (r *partnershipPostgresRepository) Create(ctx context.Context, p *partnership.Partnership) error {
	metadata, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return r.db.RunInTx(ctx, func(tx *sqlx.Tx) error {
		// Serializes invitations in both directions without rewriting legacy rows.
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(LEAST($1::text,$2::text)||':'||GREATEST($1::text,$2::text),0))`, p.RequesterID, p.TargetID); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM toca_auth.accounts WHERE id::text IN ($1,$2) AND metadata->>'Type'='company' AND metadata#>>'{Company,IsActive}'='true'`, p.RequesterID, p.TargetID).Scan(&count); err != nil {
			return err
		}
		if count != 2 {
			return partnership.ErrCompany
		}
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM core.partnerships WHERE deleted_at IS NULL AND status IN ('active','pending','suspended') AND ((requester_id::text=$1 AND target_id::text=$2) OR (requester_id::text=$2 AND target_id::text=$1))`, p.RequesterID, p.TargetID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return partnership.ErrDuplicate
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO core.partnerships(id,requester_id,target_id,status,metadata) VALUES($1,$2,$3,$4,$5)`, p.ID, p.RequesterID, p.TargetID, p.Status, string(metadata)); err != nil {
			return err
		}
		return enqueuePartnership(ctx, r.db, tx, p)
	})
}

func (r *partnershipPostgresRepository) GetByID(ctx context.Context, id string) (*partnership.Partnership, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, "SELECT "+partnershipProjection+" FROM core.partnerships WHERE id = $1 AND deleted_at IS NULL", id).Scan(&metadata)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, partnership.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var p partnership.Partnership
	if err := json.Unmarshal(metadata, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *partnershipPostgresRepository) GetByRequesterID(ctx context.Context, requesterID string) ([]*partnership.Partnership, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+partnershipProjection+`
		FROM core.partnerships
		WHERE requester_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`, requesterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*partnership.Partnership, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var p partnership.Partnership
		if err := json.Unmarshal(metadata, &p); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *partnershipPostgresRepository) GetByTargetID(ctx context.Context, targetID string) ([]*partnership.Partnership, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+partnershipProjection+`
		FROM core.partnerships
		WHERE target_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*partnership.Partnership, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var p partnership.Partnership
		if err := json.Unmarshal(metadata, &p); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *partnershipPostgresRepository) List(ctx context.Context, filter map[string]interface{}) ([]*partnership.Partnership, error) {
	where := "WHERE deleted_at IS NULL"
	args := make([]any, 0)
	argN := 1

	if v, ok := filter["requester_id"]; ok {
		where += fmt.Sprintf(" AND requester_id = $%d", argN)
		args = append(args, v)
		argN++
	}
	if v, ok := filter["target_id"]; ok {
		where += fmt.Sprintf(" AND target_id = $%d", argN)
		args = append(args, v)
		argN++
	}
	if v, ok := filter["status"]; ok {
		where += fmt.Sprintf(" AND status = $%d", argN)
		args = append(args, v)
	}

	rows, err := r.db.QueryContext(ctx, "SELECT "+partnershipProjection+" FROM core.partnerships "+where+" ORDER BY created_at DESC", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*partnership.Partnership, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var p partnership.Partnership
		if err := json.Unmarshal(metadata, &p); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *partnershipPostgresRepository) Update(ctx context.Context, actor string, p *partnership.Partnership) error {
	if p.Version < 1 || len(p.AuditLogs) == 0 {
		return partnership.ErrConflict
	}
	metadata, err := json.Marshal(p)
	if err != nil {
		return err
	}
	latest, err := json.Marshal([]partnership.AuditLog{p.AuditLogs[len(p.AuditLogs)-1]})
	if err != nil {
		return err
	}
	return r.db.RunInTx(ctx, func(tx *sqlx.Tx) error {
		if p.EventAction == "accepted" {
			var count int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM toca_auth.accounts WHERE id::text IN ($1,$2) AND metadata->>'Type'='company' AND metadata#>>'{Company,IsActive}'='true'`, p.RequesterID, p.TargetID).Scan(&count); err != nil {
				return err
			}
			if count != 2 {
				return partnership.ErrCompany
			}
		}
		result, err := tx.ExecContext(ctx, `UPDATE core.partnerships SET status=$1,metadata=jsonb_set($2::jsonb,'{audit_logs}',COALESCE(metadata->'audit_logs','[]'::jsonb)||$5::jsonb),updated_at=NOW() WHERE id=$3 AND (requester_id::text=$4 OR target_id::text=$4) AND deleted_at IS NULL AND COALESCE((metadata->>'version')::int,0)=$6`, p.Status, string(metadata), p.ID, actor, string(latest), p.Version-1)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return partnership.ErrConflict
		}
		return enqueuePartnership(ctx, r.db, tx, p)
	})
}

func (r *partnershipPostgresRepository) Delete(ctx context.Context, actorTenantID, id string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE core.partnerships SET deleted_at = NOW()
		WHERE id = $1 AND (requester_id = $2 OR target_id = $2) AND deleted_at IS NULL`, id, actorTenantID)
	return err
}

func (r *partnershipPostgresRepository) AddAuditLog(ctx context.Context, actorTenantID, id string, log partnership.AuditLog) error {
	b, err := json.Marshal(log)
	if err != nil {
		return err
	}
	arr := []byte("[" + string(b) + "]")
	_, err = r.db.ExecContext(ctx, `
		UPDATE core.partnerships
		SET metadata = jsonb_set(metadata, '{audit_logs}', COALESCE(metadata->'audit_logs','[]'::jsonb) || $1::jsonb, true),
		    updated_at = NOW()
		WHERE id = $2 AND (requester_id = $3 OR target_id = $3) AND deleted_at IS NULL
	`, string(arr), id, actorTenantID)
	return err
}
