package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/oginoino/toca/pkg/database"
	ps "github.com/oginoino/toca/toca-financial/internal/domain/platform_subscription"
)

type PlatformSubscriptionRepository struct{ db *database.PostgresDB }

func NewPlatformSubscriptionRepository(db *database.PostgresDB) *PlatformSubscriptionRepository {
	return &PlatformSubscriptionRepository{db: db}
}
func (r *PlatformSubscriptionRepository) Get(ctx context.Context, tenant string) (*ps.State, error) {
	if tenant == "" {
		return nil, errors.New("tenant context required")
	}
	var raw []byte
	err := r.db.QueryRow(ctx, `SELECT metadata FROM financial.platform_subscriptions WHERE tenant_id=$1 AND deleted_at IS NULL`, tenant).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return &ps.State{Status: "NONE"}, nil
	}
	if err != nil {
		return nil, err
	}
	var s ps.State
	if err = json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	if s.Status == "" {
		s.Status = "NONE"
	}
	return &s, nil
}
func (r *PlatformSubscriptionRepository) Update(ctx context.Context, tenant string, fn func(context.Context, *ps.State) error) (*ps.State, error) {
	if tenant == "" {
		return nil, errors.New("tenant context required")
	}
	var result *ps.State
	// Browser roles have SELECT only; payment state can only be written after
	// server authorization or webhook validation, on the verified service pool.
	err := r.db.RunInServiceContext(ctx, func(txCtx context.Context) error {
		if _, err := r.db.Exec(txCtx, `SELECT pg_advisory_xact_lock(hashtextextended($1,874031))`, tenant); err != nil {
			return err
		}
		s, err := r.Get(txCtx, tenant)
		if err != nil {
			return err
		}
		if err = fn(txCtx, s); err != nil {
			return err
		}
		raw, err := json.Marshal(s)
		if err != nil {
			return err
		}
		_, err = r.db.Exec(txCtx, `INSERT INTO financial.platform_subscriptions(tenant_id,metadata) VALUES($1,$2)
   ON CONFLICT(tenant_id) WHERE deleted_at IS NULL DO UPDATE SET metadata=EXCLUDED.metadata,updated_at=now()`, tenant, string(raw))
		result = s
		return err
	})
	return result, err
}
func (r *PlatformSubscriptionRepository) ResolveTenant(ctx context.Context, sub, checkout, external string) (string, error) {
	var tenant string
	err := r.db.QueryServiceRow(ctx, `SELECT tenant_id FROM financial.platform_subscriptions WHERE deleted_at IS NULL AND
  (($1<>'' AND metadata->>'subscription_id'=$1) OR ($2<>'' AND metadata->'checkout'->>'id'=$2) OR
  ($3<>'' AND metadata->'checkout'->>'external_id'=$3))`, sub, checkout, external).Scan(&tenant)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ps.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return tenant, nil
}
func (r *PlatformSubscriptionRepository) RememberEvent(ctx context.Context, tenant, id, kind string) (bool, error) {
	tag, err := r.db.Exec(ctx, `INSERT INTO financial.platform_subscription_events(tenant_id,event_id,event_type) VALUES($1::uuid,$2,$3) ON CONFLICT(event_id) DO NOTHING`, tenant, id, kind)
	if err != nil {
		return false, err
	}
	n, err := tag.RowsAffected()
	return n == 1, err
}
func (r *PlatformSubscriptionRepository) SavePayment(ctx context.Context, tenant string, p ps.Payment) error {
	_, err := r.db.Exec(ctx, `INSERT INTO financial.platform_subscription_payments(tenant_id,payment_id,subscription_id,amount_cents,paid_at,receipt_url)
 VALUES($1::uuid,$2,$3,$4,$5,$6) ON CONFLICT(payment_id) DO NOTHING`, tenant, p.ID, p.SubscriptionID, p.AmountCents, p.PaidAt, p.ReceiptURL)
	return err
}
func (r *PlatformSubscriptionRepository) Payments(ctx context.Context, tenant string) ([]ps.Payment, error) {
	rows, err := r.db.Query(ctx, `SELECT payment_id,subscription_id,amount_cents,paid_at,receipt_url FROM financial.platform_subscription_payments WHERE tenant_id=$1::uuid ORDER BY paid_at DESC LIMIT 100`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ps.Payment{}
	for rows.Next() {
		var p ps.Payment
		if err = rows.Scan(&p.ID, &p.SubscriptionID, &p.AmountCents, &p.PaidAt, &p.ReceiptURL); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Payment returns one confirmed payment of this tenant, or nil when the id
// belongs to someone else: the receipt endpoint must not leak other customers.
func (r *PlatformSubscriptionRepository) Payment(ctx context.Context, tenant, id string) (*ps.Payment, error) {
	var p ps.Payment
	err := r.db.QueryRow(ctx, `SELECT payment_id,subscription_id,amount_cents,paid_at,receipt_url
  FROM financial.platform_subscription_payments WHERE tenant_id=$1::uuid AND payment_id=$2`, tenant, id).
		Scan(&p.ID, &p.SubscriptionID, &p.AmountCents, &p.PaidAt, &p.ReceiptURL)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// TenantIdentity names the customer on the receipt. Read on the service pool
// because the company record is not owned by the browser role.
func (r *PlatformSubscriptionRepository) TenantIdentity(ctx context.Context, tenant string) (string, string, error) {
	var name, document sql.NullString
	err := r.db.QueryServiceRow(ctx, `SELECT COALESCE(metadata->'Company'->>'Name', metadata->'Company'->>'LegalName',''),
  COALESCE(metadata->'Company'->>'TaxID','') FROM toca_auth.accounts WHERE id=$1::uuid`, tenant).Scan(&name, &document)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	return name.String, document.String, nil
}

func (r *PlatformSubscriptionRepository) Usage(ctx context.Context, tenant string) (ps.Limits, error) {
	var counts ps.Limits
	// The company membership list is authoritative, rather than a paged list of brokers.
	err := r.db.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM crm.leads WHERE tenant_id=$1::uuid AND deleted_at IS NULL),
 (SELECT count(*) FROM core.properties WHERE tenant_id=$1::uuid AND deleted_at IS NULL),
 (SELECT count(DISTINCT m->>'account_id') FROM toca_auth.accounts a,
  LATERAL jsonb_array_elements(COALESCE(a.metadata->'Company'->'Members','[]'::jsonb)) m
  WHERE a.id=$1::uuid AND m->>'account_id' IS NOT NULL AND COALESCE(m->>'is_active','true')<>'false')`, tenant).Scan(&counts.Leads, &counts.Properties, &counts.Users)
	return counts, err
}
