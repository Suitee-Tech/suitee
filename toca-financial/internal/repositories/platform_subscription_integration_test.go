package repositories_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/config"
	appctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	ps "github.com/oginoino/toca/toca-financial/internal/domain/platform_subscription"
	"github.com/oginoino/toca/toca-financial/internal/providers"
	repository "github.com/oginoino/toca/toca-financial/internal/repositories"
	service "github.com/oginoino/toca/toca-financial/internal/services"
	"github.com/stretchr/testify/require"
)

func TestPlatformSubscriptionPostgresCheckoutPaymentAndRLS(t *testing.T) {
	uri := os.Getenv("TOCA_SUBSCRIPTION_TEST_DATABASE_URI")
	if uri == "" {
		t.Skip("TOCA_SUBSCRIPTION_TEST_DATABASE_URI is not configured")
	}
	ctx := context.Background()
	db, err := database.ConnectPostgres(ctx, config.DatabaseConfig{URI: uri, MaxPoolSize: 6})
	require.NoError(t, err)
	defer db.Close()
	tenant := uuid.NewString()
	other := uuid.NewString()
	plan, _ := service.FindPlatformPlanCycle("starter", "MONTHLY")
	var remote ps.RemoteCheckout
	var mu sync.Mutex
	remoteSub := ps.RemoteSubscription{ID: "subs_" + uuid.NewString(), Status: "ACTIVE", DevMode: true, Amount: plan.AmountCents}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		require.Equal(t, "Bearer sandbox-test", r.Header.Get("Authorization"))
		var data any
		switch r.URL.Path {
		case "/products/list":
			data = []map[string]any{{"id": "prod_test", "externalId": "suitee-starter-monthly-49700", "price": 49700, "cycle": "MONTHLY", "status": "ACTIVE", "currency": "BRL", "devMode": true}}
		case "/subscriptions/checkouts/get":
			if remote.ID == "" {
				w.WriteHeader(404)
				return
			}
			data = remote
		case "/subscriptions/create":
			var request struct {
				ExternalID string `json:"externalId"`
				Items      []struct {
					ID       string `json:"id"`
					Quantity int    `json:"quantity"`
				} `json:"items"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			remote = ps.RemoteCheckout{ID: "bill_" + uuid.NewString(), ExternalID: request.ExternalID, Amount: 49700, DevMode: true, Status: "PENDING", URL: "https://app.abacatepay.com/pay/test", Items: request.Items}
			remoteSub.CheckoutID = remote.ID
			data = remote
		case "/subscriptions/get":
			data = remoteSub
		case "/subscriptions/cancel":
			remoteSub.Status = "CANCELLED"
			data = remoteSub
		case "/subscriptions/checkouts/delete":
			data = map[string]bool{"deleted": true}
		default:
			t.Errorf("unexpected provider endpoint %s", r.URL.Path)
			w.WriteHeader(500)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "error": nil, "data": data})
	}))
	defer server.Close()
	provider := providers.NewAbacateSubscriptionProvider("sandbox-test", true)
	provider.BaseURL = server.URL
	repo := repository.NewPlatformSubscriptionRepository(db)
	svc := &service.PlatformSubscriptionService{Repo: repo, Provider: provider, DevMode: true, Enabled: true, ReturnURL: "https://app.example.com/assinatura"}
	requestCtx := appctx.WithTenantID(ctx, tenant)
	x, err := svc.Select(requestCtx, tenant, "starter", "MONTHLY")
	require.NoError(t, err)
	require.Equal(t, "PENDING", x.Status)
	require.NoError(t, db.ExecService(ctx, `INSERT INTO crm.leads VALUES($1,$2,NULL),($3,$2,now())`, uuid.NewString(), tenant, uuid.NewString()))
	require.NoError(t, db.ExecService(ctx, `INSERT INTO core.properties VALUES($1,$2,NULL)`, uuid.NewString(), tenant))
	members := `{"Company":{"Members":[{"account_id":"member-one"},{"account_id":"member-two","is_active":false}]}}`
	require.NoError(t, db.ExecService(ctx, `INSERT INTO toca_auth.accounts VALUES($1,$2)`, tenant, members))
	usage, err := repo.Usage(requestCtx, tenant)
	require.NoError(t, err)
	require.Equal(t, ps.Limits{Leads: 1, Properties: 1, Users: 1}, usage)
	at := time.Date(2026, 10, 3, 18, 0, 0, 0, time.UTC)
	var event ps.Event
	event.ID = "log_" + uuid.NewString()
	event.Event = "subscription.completed"
	event.APIVersion = 2
	event.DevMode = true
	event.Data.Subscription.ID = remoteSub.ID
	event.Data.Subscription.Status = "ACTIVE"
	event.Data.Subscription.Amount = 49700
	event.Data.Subscription.Frequency = "MONTHLY"
	event.Data.Subscription.UpdatedAt = at
	event.Data.Checkout.ID = x.Checkout.ID
	event.Data.Payment.ID = "char_" + uuid.NewString()
	event.Data.Payment.Status = "PAID"
	event.Data.Payment.Amount = 49700
	event.Data.Payment.PaidAmount = 49700
	event.Data.Payment.UpdatedAt = at
	require.NoError(t, svc.HandleEvent(ctx, event))
	require.NoError(t, svc.HandleEvent(ctx, event))
	x, err = repo.Get(requestCtx, tenant)
	require.NoError(t, err)
	require.Equal(t, "ACTIVE", x.Status)
	payments, err := repo.Payments(requestCtx, tenant)
	require.NoError(t, err)
	require.Len(t, payments, 1)
	cancelled, err := svc.Cancel(requestCtx, tenant, "too_expensive")
	require.NoError(t, err)
	require.Equal(t, "CANCELLED", cancelled.Status)
	require.Equal(t, *x.PaidThrough, *cancelled.PaidThrough)
	// No authenticated caller can edit payment state through the Supabase Data API.
	conn, err := db.ServicePool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()
	_, err = conn.Exec(ctx, `SET ROLE authenticated`)
	require.NoError(t, err)
	defer conn.Exec(ctx, `RESET ROLE`)
	claims, _ := json.Marshal(map[string]any{"role": "authenticated", "app_metadata": map[string]string{"tenant_id": tenant}})
	_, err = conn.Exec(ctx, `SELECT set_config('request.jwt.claims',$1,false)`, string(claims))
	require.NoError(t, err)
	var count int
	err = conn.QueryRow(ctx, `SELECT count(*) FROM financial.platform_subscriptions WHERE tenant_id=$1`, tenant).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	_, err = conn.Exec(ctx, `UPDATE financial.platform_subscriptions SET metadata='{"status":"ACTIVE"}' WHERE tenant_id=$1`, tenant)
	require.Error(t, err)
	_, err = conn.Exec(ctx, `INSERT INTO financial.platform_subscription_payments(payment_id,tenant_id,subscription_id,amount_cents,paid_at) VALUES('forged',$1,'subs_forged',1,now())`, tenant)
	require.Error(t, err)
	claims, _ = json.Marshal(map[string]any{"role": "authenticated", "app_metadata": map[string]string{"tenant_id": other}})
	_, err = conn.Exec(ctx, `SELECT set_config('request.jwt.claims',$1,false)`, string(claims))
	require.NoError(t, err)
	err = conn.QueryRow(ctx, `SELECT count(*) FROM financial.platform_subscriptions WHERE tenant_id=$1`, tenant).Scan(&count)
	require.NoError(t, err)
	require.Zero(t, count)
	err = conn.QueryRow(ctx, `SELECT count(*) FROM financial.platform_subscription_payments WHERE tenant_id=$1::uuid`, tenant).Scan(&count)
	require.NoError(t, err)
	require.Zero(t, count)
}
