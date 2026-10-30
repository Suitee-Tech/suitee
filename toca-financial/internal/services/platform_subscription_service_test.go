package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	ps "github.com/oginoino/toca/toca-financial/internal/domain/platform_subscription"
	"github.com/stretchr/testify/require"
)

type subscriptionTestTx struct {
	state    *ps.State
	events   map[string]bool
	payments map[string]ps.Payment
}
type subscriptionTestKey struct{}
type subscriptionMemory struct {
	mu       sync.Mutex
	states   map[string]*ps.State
	events   map[string]bool
	payments map[string]ps.Payment
	usage    ps.Limits
}

func newSubscriptionMemory() *subscriptionMemory {
	return &subscriptionMemory{states: map[string]*ps.State{}, events: map[string]bool{}, payments: map[string]ps.Payment{}}
}
func cloneState(x *ps.State) *ps.State {
	if x == nil {
		return &ps.State{Status: "NONE"}
	}
	raw, _ := json.Marshal(x)
	var out ps.State
	_ = json.Unmarshal(raw, &out)
	return &out
}
func (r *subscriptionMemory) Get(_ context.Context, tenant string) (*ps.State, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneState(r.states[tenant]), nil
}
func (r *subscriptionMemory) Usage(context.Context, string) (ps.Limits, error) { return r.usage, nil }
func (r *subscriptionMemory) Update(ctx context.Context, tenant string, fn func(context.Context, *ps.State) error) (*ps.State, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	tx := &subscriptionTestTx{state: cloneState(r.states[tenant]), events: map[string]bool{}, payments: map[string]ps.Payment{}}
	for k, v := range r.events {
		tx.events[k] = v
	}
	for k, v := range r.payments {
		tx.payments[k] = v
	}
	if err := fn(context.WithValue(ctx, subscriptionTestKey{}, tx), tx.state); err != nil {
		return nil, err
	}
	r.states[tenant] = tx.state
	r.events = tx.events
	r.payments = tx.payments
	return cloneState(tx.state), nil
}
func (r *subscriptionMemory) ResolveTenant(_ context.Context, sub, checkout, external string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for tenant, x := range r.states {
		if sub != "" && x.SubscriptionID == sub {
			return tenant, nil
		}
		if x.Checkout != nil && (checkout != "" && x.Checkout.ID == checkout || external != "" && x.Checkout.ExternalID == external) {
			return tenant, nil
		}
	}
	return "", ps.ErrNotFound
}
func (r *subscriptionMemory) RememberEvent(ctx context.Context, _ string, id, _ string) (bool, error) {
	tx := ctx.Value(subscriptionTestKey{}).(*subscriptionTestTx)
	if tx.events[id] {
		return false, nil
	}
	tx.events[id] = true
	return true, nil
}
func (r *subscriptionMemory) SavePayment(ctx context.Context, _ string, p ps.Payment) error {
	tx := ctx.Value(subscriptionTestKey{}).(*subscriptionTestTx)
	tx.payments[p.ID] = p
	return nil
}
func (r *subscriptionMemory) Payments(context.Context, string) ([]ps.Payment, error) { return nil, nil }
func (r *subscriptionMemory) Payment(_ context.Context, _, id string) (*ps.Payment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.payments {
		if p.ID == id {
			copied := p
			return &copied, nil
		}
	}
	return nil, nil
}
func (r *subscriptionMemory) TenantIdentity(context.Context, string) (string, string, error) {
	return "Imobiliária Teste", "12.345.678/0001-90", nil
}

type subscriptionProviderFake struct {
	mu            sync.Mutex
	subscriptions map[string]*ps.RemoteSubscription
	checkouts     map[string]*ps.RemoteCheckout
	creates       int
	changes       int
	loseCreate    bool
	dropCreate    bool
	changeError   bool
	cancelled     bool
}

func (p *subscriptionProviderFake) EnsureProduct(_ context.Context, plan ps.Plan) (string, error) {
	// The real provider identifies an offer by tier, cycle and free period.
	return fmt.Sprintf("prod_%s%s_t%d", plan.Tier, plan.Cycle, plan.TrialDays), nil
}
func (p *subscriptionProviderFake) FindCheckout(_ context.Context, id string) (*ps.RemoteCheckout, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if x := p.checkouts[id]; x != nil {
		return x, nil
	}
	return nil, ps.ErrNotFound
}
func (p *subscriptionProviderFake) CreateCheckout(_ context.Context, c ps.Checkout, _ string) (*ps.RemoteCheckout, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.creates++
	if p.dropCreate {
		// The provider never received the operation: nothing exists remotely.
		return nil, ps.ErrUnavailable
	}
	x := &ps.RemoteCheckout{ID: "bill_" + c.ExternalID, ExternalID: c.ExternalID, Amount: c.Plan.AmountCents, DevMode: true, URL: "https://app.abacatepay.com/pay/test", Status: "PENDING"}
	x.Items = append(x.Items, struct {
		ID       string `json:"id"`
		Quantity int    `json:"quantity"`
	}{c.ProductID, 1})
	p.checkouts[c.ExternalID] = x
	if p.loseCreate {
		return nil, ps.ErrUnavailable
	}
	return x, nil
}
func (p *subscriptionProviderFake) DeleteCheckout(context.Context, string) error { return nil }
func (p *subscriptionProviderFake) GetSubscription(_ context.Context, id string) (*ps.RemoteSubscription, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.subscriptions == nil {
		return nil, ps.ErrNotFound
	}
	sub := p.subscriptions[id]
	if sub == nil {
		return nil, ps.ErrNotFound
	}
	copied := *sub
	return &copied, nil
}
func (p *subscriptionProviderFake) Cancel(context.Context, string) error {
	p.cancelled = true
	return nil
}
func (p *subscriptionProviderFake) ChangePlan(context.Context, string, string) (string, error) {
	if p.changeError {
		return "", ps.ErrUnavailable
	}
	p.mu.Lock()
	p.changes++
	p.mu.Unlock()
	return "subu_test", nil
}

// subscriptionFixtureProvider returns the provider of an existing fixture.
// Um checkout já pago não pode ser descartado: o pagamento está em
// processamento e a assinatura dele precisa ser confirmada antes da troca. O
// cliente recebe o motivo exato (não o texto genérico).
func TestPlatformSubscriptionTrocaDePlanoComCheckoutPago(t *testing.T) {
	s, r, p := subscriptionFixture()
	ctx := context.Background()

	estado, err := s.Select(ctx, "tenant-a", "starter", "MONTHLY")
	require.NoError(t, err)
	require.NotNil(t, estado.Checkout)

	// o provedor confirma o pagamento do checkout em aberto
	remoto := p.checkouts[estado.Checkout.ExternalID]
	require.NotNil(t, remoto)
	remoto.Status = "PAID"

	_, err = s.Select(ctx, "tenant-a", "pro", "MONTHLY")
	require.ErrorIs(t, err, ps.ErrConflict)
	require.Contains(t, err.Error(), "já foi pago", "o conflito precisa dizer que o checkout foi pago")

	depois, err := r.Get(ctx, "tenant-a")
	require.NoError(t, err)
	require.Equal(t, "starter", depois.Checkout.Plan.Tier, "o checkout pago não pode ser substituído")
	require.Equal(t, estado.Checkout.ExternalID, depois.Checkout.ExternalID)
}

func subscriptionFixtureProvider(t *testing.T, s *PlatformSubscriptionService) *subscriptionProviderFake {
	t.Helper()
	p, ok := s.Provider.(*subscriptionProviderFake)
	require.True(t, ok)
	return p
}

func subscriptionFixture() (*PlatformSubscriptionService, *subscriptionMemory, *subscriptionProviderFake) {
	r := newSubscriptionMemory()
	p := &subscriptionProviderFake{checkouts: map[string]*ps.RemoteCheckout{}, subscriptions: map[string]*ps.RemoteSubscription{}}
	return &PlatformSubscriptionService{Repo: r, Provider: p, Enabled: true, DevMode: true, ReturnURL: "https://app.example.com/assinatura"}, r, p
}
func paidSubscriptionEvent(id, kind, checkout string, plan ps.Plan, at time.Time) ps.Event {
	var e ps.Event
	e.ID = id
	e.Event = kind
	e.APIVersion = 2
	e.DevMode = true
	e.Data.Subscription.ID = "subs_test"
	e.Data.Subscription.Amount = plan.AmountCents
	e.Data.Subscription.Status = "ACTIVE"
	e.Data.Subscription.Frequency = plan.Cycle
	e.Data.Subscription.UpdatedAt = at
	e.Data.Checkout.ID = checkout
	e.Data.Payment.ID = "char_" + id
	e.Data.Payment.Amount = plan.AmountCents
	e.Data.Payment.PaidAmount = plan.AmountCents
	e.Data.Payment.Status = "APPROVED"
	e.Data.Payment.UpdatedAt = at
	return e
}
func TestPlatformSubscriptionCheckoutConcurrencyAndLostResponse(t *testing.T) {
	s, r, p := subscriptionFixture()
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Select(ctx, "tenant-a", "starter", "MONTHLY")
			if err != nil && !errors.Is(err, ps.ErrUnavailable) {
				t.Errorf("unexpected: %v", err)
			}
		}()
	}
	wg.Wait()
	require.Equal(t, 1, p.creates)
	x, err := r.Get(ctx, "tenant-a")
	require.NoError(t, err)
	require.Equal(t, "PENDING", x.Status)
	require.Nil(t, x.Plan)
	// Trocar de plano com um checkout em aberto substitui o checkout antigo.
	// Antes devolvia 409 ("encerre o checkout pendente") e prendia o cliente —
	// em trial, o segundo clique simplesmente falhava.
	antes := p.creates
	trocado, err := s.Select(ctx, "tenant-a", "pro", "MONTHLY")
	require.NoError(t, err)
	require.Equal(t, antes+1, p.creates, "o plano novo precisa gerar um checkout novo")
	require.Equal(t, "pro", trocado.Checkout.Plan.Tier)
	require.Equal(t, "MONTHLY", trocado.Checkout.Plan.Cycle)
	require.Equal(t, "PENDING", trocado.Status)
	other, _ := r.Get(ctx, "tenant-b")
	require.Equal(t, "NONE", other.Status)
	s, r, p = subscriptionFixture()
	p.loseCreate = true
	_, err = s.Select(ctx, "tenant-a", "starter", "MONTHLY")
	require.ErrorIs(t, err, ps.ErrUnavailable)
	x, _ = r.Get(ctx, "tenant-a")
	require.Empty(t, x.Checkout.ID)
	x, err = s.Refresh(ctx, "tenant-a")
	require.NoError(t, err)
	require.NotEmpty(t, x.Checkout.ID)
	_, err = s.Select(ctx, "tenant-a", "starter", "MONTHLY")
	require.NoError(t, err)
	require.Equal(t, 1, p.creates)
}
func TestPlatformSubscriptionStaleCheckoutAgesOutAndIsRebuilt(t *testing.T) {
	s, r, p := subscriptionFixture()
	ctx := context.Background()
	// The provider never received the operation, so only the local intent exists.
	p.dropCreate = true
	_, err := s.Select(ctx, "tenant-a", "starter", "MONTHLY")
	require.ErrorIs(t, err, ps.ErrUnavailable)
	x, _ := r.Get(ctx, "tenant-a")
	require.Empty(t, x.Checkout.ID)
	// A recent intent may still be in flight; recreating now could double charge.
	_, err = s.Select(ctx, "tenant-a", "starter", "MONTHLY")
	require.ErrorIs(t, err, ps.ErrUnavailable)
	require.Equal(t, 1, p.creates)
	// After it ages out, the same request rebuilds the checkout instead of
	// leaving the administrator stuck on an unrecoverable reference.
	r.states["tenant-a"].Checkout.CreatedAt = time.Now().Add(-2 * time.Minute)
	p.dropCreate = false
	x, err = s.Select(ctx, "tenant-a", "starter", "MONTHLY")
	require.NoError(t, err)
	require.Equal(t, "bill_"+x.Checkout.ExternalID, x.Checkout.ID)
	require.True(t, strings.HasPrefix(x.Checkout.URL, "https://app.abacatepay.com/"))
	require.Equal(t, 2, p.creates)
}

func TestPlatformSubscriptionPaymentsScheduleCancellationAndReplay(t *testing.T) {
	s, r, p := subscriptionFixture()
	ctx := context.Background()
	x, err := s.Select(ctx, "tenant-a", "starter", "MONTHLY")
	require.NoError(t, err)
	at := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)
	e := paidSubscriptionEvent("first", "subscription.completed", x.Checkout.ID, x.Checkout.Plan, at)
	require.NoError(t, s.HandleEvent(ctx, e))
	require.NoError(t, s.HandleEvent(ctx, e))
	require.Len(t, r.payments, 1)
	x, _ = r.Get(ctx, "tenant-a")
	require.Equal(t, "ACTIVE", x.Status)
	require.Equal(t, time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC), *x.PaidThrough)
	x, err = s.Select(ctx, "tenant-a", "pro", "ANNUALLY")
	require.NoError(t, err)
	require.Equal(t, "starter", x.Plan.Tier)
	require.Equal(t, "pro", x.PendingChange.Plan.Tier)
	renewal := paidSubscriptionEvent("renewal", "subscription.renewed", "bill_renewal", x.PendingChange.Plan, at.AddDate(0, 1, 0))
	require.NoError(t, s.HandleEvent(ctx, renewal))
	x, _ = r.Get(ctx, "tenant-a")
	require.Equal(t, "pro", x.Plan.Tier)
	require.Nil(t, x.PendingChange)
	require.Len(t, r.payments, 2)
	// Cancel even if initial webhook is delivered late; a cancelled subscription never reactivates.
	x, err = s.Cancel(ctx, "tenant-a", "")
	require.NoError(t, err)
	require.True(t, p.cancelled)
	end := *x.PaidThrough
	older := e
	older.ID = "delayed"
	older.Data.Payment.ID = e.Data.Payment.ID
	require.NoError(t, s.HandleEvent(ctx, older)) // old payment cannot revert the annual plan or cancellation
	require.Equal(t, "CANCELLED", r.states["tenant-a"].Status)
	require.Equal(t, end, *r.states["tenant-a"].PaidThrough)
}
func TestPlatformSubscriptionAcceptsBothDocumentedPaidStatuses(t *testing.T) {
	for _, status := range []string{"APPROVED", "PAID"} {
		s, r, _ := subscriptionFixture()
		x, err := s.Select(context.Background(), "tenant-a", "starter", "MONTHLY")
		require.NoError(t, err)
		e := paidSubscriptionEvent("status-"+status, "subscription.completed", x.Checkout.ID, x.Checkout.Plan, time.Now())
		e.Data.Payment.Status = status
		require.NoError(t, s.HandleEvent(context.Background(), e), status)
		require.Equal(t, "ACTIVE", r.states["tenant-a"].Status, status)
	}
}

func TestPlatformSubscriptionDropsUnsafeReceiptButKeepsPayment(t *testing.T) {
	s, r, _ := subscriptionFixture()
	x, err := s.Select(context.Background(), "tenant-a", "starter", "MONTHLY")
	require.NoError(t, err)
	e := paidSubscriptionEvent("unsafe-receipt", "subscription.completed", x.Checkout.ID, x.Checkout.Plan, time.Now())
	e.Data.Payment.ReceiptURL = "https://evil.test/receipt"
	require.NoError(t, s.HandleEvent(context.Background(), e))
	require.Equal(t, "ACTIVE", r.states["tenant-a"].Status)
	require.Len(t, r.payments, 1)
	// The link is discarded, not the payment: the provider confirmed the charge.
	require.Empty(t, r.payments[e.Data.Payment.ID].ReceiptURL)
}

func TestPlatformSubscriptionRejectsUnpaidWrongModeAndRollsBack(t *testing.T) {
	for _, mutate := range []func(*ps.Event){func(e *ps.Event) { e.DevMode = false }, func(e *ps.Event) { e.Data.Payment.Status = "PENDING" }, func(e *ps.Event) { e.Data.Payment.Status = "REFUNDED" }, func(e *ps.Event) { e.Data.Subscription.Amount = 1 }, func(e *ps.Event) { e.Data.Payment.PaidAmount = 1 }} {
		s, r, _ := subscriptionFixture()
		x, err := s.Select(context.Background(), "tenant-a", "starter", "MONTHLY")
		require.NoError(t, err)
		e := paidSubscriptionEvent("bad", "subscription.completed", x.Checkout.ID, x.Checkout.Plan, time.Now())
		mutate(&e)
		require.ErrorIs(t, s.HandleEvent(context.Background(), e), ps.ErrConflict)
		require.Empty(t, r.events)
		require.Empty(t, r.payments)
		require.Equal(t, "PENDING", r.states["tenant-a"].Status)
	}
}
func TestPlatformSubscriptionTrialActivationUpgradeAndConversion(t *testing.T) {
	s, r, p := subscriptionFixture()
	ctx := context.Background()
	x, err := s.Select(ctx, "tenant-a", "starter", "MONTHLY")
	require.NoError(t, err)
	require.Equal(t, 15, x.Checkout.Plan.TrialDays)

	started := time.Now().UTC().Add(-4 * 24 * time.Hour)
	// Ten and a half days left: whole days are what the provider accepts.
	trialEnd := time.Now().UTC().Add(10*24*time.Hour + 12*time.Hour)
	trial := paidSubscriptionEvent("trial", "subscription.trial_started", x.Checkout.ID, x.Checkout.Plan, started)
	// The real payload reports ACTIVE and carries no trial dates.
	trial.Data.Subscription.Status = "ACTIVE"
	days := 15
	trial.Data.Subscription.TrialDays = &days
	p.subscriptions["subs_test"] = &ps.RemoteSubscription{ID: "subs_test", Amount: 49700, Status: "ACTIVE", DevMode: true, TrialDays: &days, TrialEndsAt: &trialEnd}
	require.NoError(t, s.HandleEvent(ctx, trial))
	state, _ := r.Get(ctx, "tenant-a")
	require.Equal(t, "TRIALING", state.Status)
	require.Equal(t, trialEnd, *state.TrialEndsAt)
	require.Nil(t, state.Checkout)
	require.Nil(t, state.PaidThrough)
	// The trial charges nothing, so no payment is recorded.
	require.Empty(t, r.payments)
	require.Equal(t, "starter", state.Plan.Tier)

	// A trial can still be changed before the first charge.
	upgraded, err := s.Select(ctx, "tenant-a", "pro", "MONTHLY")
	require.NoError(t, err)
	require.Equal(t, "starter", upgraded.Plan.Tier)
	require.Equal(t, "pro", upgraded.PendingChange.Plan.Tier)

	// The first charge at the end of the trial activates the plan.
	converted := paidSubscriptionEvent("converted", "subscription.completed", "", *upgraded.Plan, trialEnd)
	require.NoError(t, s.HandleEvent(ctx, converted))
	state, _ = r.Get(ctx, "tenant-a")
	require.Equal(t, "ACTIVE", state.Status)
	require.NotNil(t, state.PaidThrough)
	require.Len(t, r.payments, 1)
}

func TestPlatformSubscriptionTrialRequiresTheProviderRecord(t *testing.T) {
	s, r, p := subscriptionFixture()
	ctx := context.Background()
	x, err := s.Select(ctx, "tenant-a", "starter", "MONTHLY")
	require.NoError(t, err)
	trial := paidSubscriptionEvent("trial-bad", "subscription.trial_started", x.Checkout.ID, x.Checkout.Plan, time.Now())
	trial.Data.Subscription.Status = "ACTIVE"
	// Without the provider record there is no trial window to trust.
	require.ErrorIs(t, s.HandleEvent(ctx, trial), ps.ErrNotFound)
	require.Equal(t, "PENDING", r.states["tenant-a"].Status)
	// A record without an end date is refused too.
	p.subscriptions["subs_test"] = &ps.RemoteSubscription{ID: "subs_test", Amount: 49700, Status: "ACTIVE", DevMode: true}
	require.ErrorIs(t, s.HandleEvent(ctx, trial), ps.ErrConflict)
	require.Equal(t, "PENDING", r.states["tenant-a"].Status)
}

func TestPlatformSubscriptionCancellationReasonsAreRestricted(t *testing.T) {
	for _, reason := range CancellationReasons {
		require.True(t, ValidCancellationReason(reason), reason)
	}
	for _, reason := range []string{"", "porque sim", "TOO_EXPENSIVE", "drop table"} {
		require.False(t, ValidCancellationReason(reason), reason)
	}
	s, _, _ := subscriptionFixture()
	_, err := s.Cancel(context.Background(), "tenant-a", "porque sim")
	require.ErrorIs(t, err, ps.ErrInvalidPlan)
}

func TestPlatformSubscriptionRevertsScheduledChange(t *testing.T) {
	s, _, p := subscriptionFixture()
	ctx := context.Background()
	x, err := s.Select(ctx, "tenant-a", "starter", "MONTHLY")
	require.NoError(t, err)
	at := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	require.NoError(t, s.HandleEvent(ctx, paidSubscriptionEvent("paid", "subscription.completed", x.Checkout.ID, x.Checkout.Plan, at)))
	scheduled, err := s.Select(ctx, "tenant-a", "pro", "ANNUALLY")
	require.NoError(t, err)
	require.Equal(t, "pro", scheduled.PendingChange.Plan.Tier)
	require.Equal(t, 1, p.changes)

	reverted, err := s.RevertChange(ctx, "tenant-a")
	require.NoError(t, err)
	require.Nil(t, reverted.PendingChange)
	require.Equal(t, "starter", reverted.Plan.Tier)
	// The provider is told to charge the plan in force again.
	require.Equal(t, 2, p.changes)
	// Without a scheduled change there is nothing to revert.
	_, err = s.RevertChange(ctx, "tenant-a")
	require.ErrorIs(t, err, ps.ErrConflict)
}

func TestPlatformSubscriptionTrialIsGrantedOnce(t *testing.T) {
	s, r, _ := subscriptionFixture()
	ctx := context.Background()
	// First checkout ever: the full free period.
	first, err := s.Select(ctx, "tenant-a", "starter", "MONTHLY")
	require.NoError(t, err)
	require.Equal(t, 15, first.Checkout.Plan.TrialDays)
	require.Equal(t, "prod_starterMONTHLY_t15", first.Checkout.ProductID)

	started := time.Now().UTC()
	// A sliver past the day boundary keeps the remaining count at whole days.
	trialEnd := started.AddDate(0, 0, 15).Add(time.Minute)
	trial := paidSubscriptionEvent("trial", "subscription.trial_started", first.Checkout.ID, first.Checkout.Plan, started)
	trial.Data.Subscription.Status = "ACTIVE"
	days := 15
	p := subscriptionFixtureProvider(t, s)
	p.subscriptions["subs_test"] = &ps.RemoteSubscription{ID: "subs_test", Amount: 49700, Status: "ACTIVE", DevMode: true, TrialDays: &days, TrialEndsAt: &trialEnd}
	require.NoError(t, s.HandleEvent(ctx, trial))
	state, _ := r.Get(ctx, "tenant-a")
	require.Equal(t, "TRIALING", state.Status)
	require.NotNil(t, state.TrialGrantedAt)

	// Cancelling and subscribing again must not hand out a second free period.
	_, err = s.Cancel(ctx, "tenant-a", "other")
	require.NoError(t, err)
	again, err := s.Select(ctx, "tenant-a", "starter", "MONTHLY")
	require.NoError(t, err)
	require.Equal(t, 15, again.Checkout.Plan.TrialDays, "o restante do teste continua valendo")
	require.Equal(t, "prod_starterMONTHLY_t15", again.Checkout.ProductID)

	// Once the free period is over, the next checkout charges right away.
	_, err = s.Cancel(ctx, "tenant-a", "other")
	require.NoError(t, err)
	state, _ = r.Get(ctx, "tenant-a")
	past := time.Now().UTC().Add(-time.Hour)
	state.TrialEndsAt = &past
	r.states["tenant-a"] = state
	paid, err := s.Select(ctx, "tenant-a", "starter", "MONTHLY")
	require.NoError(t, err)
	require.Zero(t, paid.Checkout.Plan.TrialDays, "sem teste: a cobrança é imediata")
	require.Equal(t, "prod_starterMONTHLY_t0", paid.Checkout.ProductID)
}

func TestPlatformSubscriptionPaymentMethodRestartsOnANewCard(t *testing.T) {
	s, r, p := subscriptionFixture()
	ctx := context.Background()
	x, err := s.Select(ctx, "tenant-a", "starter", "MONTHLY")
	require.NoError(t, err)
	started := time.Now().UTC().Add(-4 * 24 * time.Hour)
	// Ten and a half days left: whole days are what the provider accepts.
	trialEnd := time.Now().UTC().Add(10*24*time.Hour + 12*time.Hour)
	trial := paidSubscriptionEvent("trial", "subscription.trial_started", x.Checkout.ID, x.Checkout.Plan, started)
	trial.Data.Subscription.Status = "ACTIVE"
	trial.Data.PayerInformation.Method = "CARD"
	trial.Data.PayerInformation.CARD.Number = "4242"
	trial.Data.PayerInformation.CARD.Brand = "VISA"
	days := 15
	p.subscriptions["subs_test"] = &ps.RemoteSubscription{ID: "subs_test", Amount: 49700, Status: "ACTIVE", DevMode: true, TrialDays: &days, TrialEndsAt: &trialEnd}
	require.NoError(t, s.HandleEvent(ctx, trial))
	state, _ := r.Get(ctx, "tenant-a")
	require.Equal(t, "VISA", state.PaymentMethod.Brand)
	require.Equal(t, "4242", state.PaymentMethod.Last4)

	// During the trial nothing was charged, so the card can be replaced.
	restarted, err := s.ChangePaymentMethodAndCheckout(ctx, "tenant-a")
	require.NoError(t, err)
	require.True(t, p.cancelled)
	require.Equal(t, "starter", restarted.Checkout.Plan.Tier)
	require.Equal(t, "PENDING", restarted.Status)
	require.Empty(t, restarted.SubscriptionID)
	require.Nil(t, restarted.PaymentMethod)
	// The free period is not served twice: only what is left follows along.
	require.Equal(t, trialEnd, *restarted.TrialEndsAt)
	require.Equal(t, 10, restarted.Checkout.Plan.TrialDays)
	require.Equal(t, "prod_starterMONTHLY_t10", restarted.Checkout.ProductID)
	// The customer must receive a payable checkout, not just a local reference.
	require.NotEmpty(t, restarted.Checkout.ID)
	require.True(t, strings.HasPrefix(restarted.Checkout.URL, "https://app.abacatepay.com/"), restarted.Checkout.URL)
}

func TestPlatformSubscriptionPaymentMethodDefersToThePaidPeriod(t *testing.T) {
	s, r, _ := subscriptionFixture()
	ctx := context.Background()
	x, err := s.Select(ctx, "tenant-a", "starter", "MONTHLY")
	require.NoError(t, err)
	// Paid through next month: the period is still running.
	at := time.Now().UTC().Add(-time.Hour)
	require.NoError(t, s.HandleEvent(ctx, paidSubscriptionEvent("paid", "subscription.completed", x.Checkout.ID, x.Checkout.Plan, at)))
	state, _ := r.Get(ctx, "tenant-a")
	require.Equal(t, "ACTIVE", state.Status)

	// The card can be replaced while the paid period runs: the new subscription
	// starts charging when that period ends, instead of charging twice.
	restarted, err := s.ChangePaymentMethodAndCheckout(ctx, "tenant-a")
	require.NoError(t, err)
	require.Equal(t, "PENDING", restarted.Status)
	require.Equal(t, state.PaidThrough.UTC(), restarted.PaidThrough.UTC())
	require.InDelta(t, remainingTrialDays(state.PaidThrough, time.Now()), restarted.Checkout.Plan.TrialDays, 1)
	require.GreaterOrEqual(t, restarted.Checkout.Plan.TrialDays, 28)
	require.NotEmpty(t, restarted.Checkout.URL)
}

func TestPlatformSubscriptionReturnsBeforeThePaidPeriodEnds(t *testing.T) {
	s, r, _ := subscriptionFixture()
	ctx := context.Background()
	x, err := s.Select(ctx, "tenant-a", "starter", "MONTHLY")
	require.NoError(t, err)
	at := time.Now().UTC().Add(-time.Hour)
	require.NoError(t, s.HandleEvent(ctx, paidSubscriptionEvent("paid", "subscription.completed", x.Checkout.ID, x.Checkout.Plan, at)))
	_, err = s.Cancel(ctx, "tenant-a", "too_expensive")
	require.NoError(t, err)
	cancelled, _ := r.Get(ctx, "tenant-a")
	require.Equal(t, "CANCELLED", cancelled.Status)
	require.True(t, cancelled.PaidThrough.After(time.Now()))

	// Coming back is allowed: the checkout waits for the paid period to end.
	back, err := s.Select(ctx, "tenant-a", "starter", "MONTHLY")
	require.NoError(t, err)
	require.Equal(t, "PENDING", back.Status)
	require.InDelta(t, remainingTrialDays(cancelled.PaidThrough, time.Now()), back.Checkout.Plan.TrialDays, 1)
	require.NotEmpty(t, back.Checkout.URL)
}

func TestPlatformSubscriptionPaymentMethodWaitsOnlyForLongPaidPeriods(t *testing.T) {
	s, r, _ := subscriptionFixture()
	ctx := context.Background()
	x, err := s.Select(ctx, "tenant-a", "starter", "ANNUALLY")
	require.NoError(t, err)
	started := time.Now().UTC()
	annual := paidSubscriptionEvent("annual", "subscription.completed", x.Checkout.ID, x.Checkout.Plan, started)
	require.NoError(t, s.HandleEvent(ctx, annual))
	state, _ := r.Get(ctx, "tenant-a")
	require.Equal(t, "ACTIVE", state.Status)
	require.Greater(t, remainingTrialDays(state.PaidThrough, time.Now()), 90)

	// The provider cannot defer almost a year, so this change has to wait — and
	// the customer is told when.
	_, err = s.ChangePaymentMethod(ctx, "tenant-a")
	require.ErrorIs(t, err, ps.ErrConflict)
	require.Contains(t, err.Error(), "a troca fica disponível em")
	after, _ := r.Get(ctx, "tenant-a")
	require.Equal(t, state.SubscriptionID, after.SubscriptionID)
}

func TestPlatformSubscriptionKeepsLocalCancellationReason(t *testing.T) {
	s, r, _ := subscriptionFixture()
	ctx := context.Background()
	x, err := s.Select(ctx, "tenant-a", "starter", "MONTHLY")
	require.NoError(t, err)
	at := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	require.NoError(t, s.HandleEvent(ctx, paidSubscriptionEvent("paid", "subscription.completed", x.Checkout.ID, x.Checkout.Plan, at)))
	_, err = s.Cancel(ctx, "tenant-a", "")
	require.NoError(t, err)
	require.Equal(t, "requested_by_admin", r.states["tenant-a"].CancellationReason)
	// The provider's own cancelled event carries no reason; it must not erase
	// the reason recorded when the administrator asked for the cancellation.
	cancelled := paidSubscriptionEvent("cancelled", "subscription.cancelled", "", x.Checkout.Plan, at.Add(time.Hour))
	cancelled.Data.Subscription.CancelledAt = &at
	require.NoError(t, s.HandleEvent(ctx, cancelled))
	require.Equal(t, "CANCELLED", r.states["tenant-a"].Status)
	require.Equal(t, "requested_by_admin", r.states["tenant-a"].CancellationReason)
}

func TestPlatformSubscriptionPlanChangeFailureKeepsRecoverableIntentAndDowngradeChecksUsage(t *testing.T) {
	s, r, p := subscriptionFixture()
	plan, _ := FindPlatformPlanCycle("pro", "MONTHLY")
	r.states["tenant-a"] = &ps.State{Status: "ACTIVE", SubscriptionID: "subs_test", Plan: &plan, DevMode: true}
	r.usage.Leads = 800
	_, err := s.Select(context.Background(), "tenant-a", "starter", "MONTHLY")
	require.ErrorIs(t, err, ps.ErrConflict)
	r.usage.Leads = 0
	p.changeError = true
	_, err = s.Select(context.Background(), "tenant-a", "starter", "MONTHLY")
	require.ErrorIs(t, err, ps.ErrUnavailable)
	require.Equal(t, "pro", r.states["tenant-a"].Plan.Tier)
	require.Equal(t, "starter", r.states["tenant-a"].PendingChange.Plan.Tier)
	p.changeError = false
	_, err = s.Select(context.Background(), "tenant-a", "starter", "MONTHLY")
	require.NoError(t, err)
	for _, cycle := range []string{"MONTHLY", "ANNUALLY"} {
		plan, ok := FindPlatformPlanCycle("starter", cycle)
		require.True(t, ok)
		require.Equal(t, int64(49700)*map[string]int64{"MONTHLY": 1, "ANNUALLY": 12 * 80}[cycle]/map[string]int64{"MONTHLY": 1, "ANNUALLY": 100}[cycle], plan.AmountCents)
	}
	_, err = s.Select(context.Background(), "tenant-a", "enterprise", "MONTHLY")
	require.ErrorIs(t, err, ps.ErrInvalidPlan)
}

// A oferta do teste é exibida com os preços do catálogo atual: o retrato
// gravado no dia da concessão serve de registro do que foi concedido, mas a
// página não pode prometer a primeira cobrança pelo preço antigo depois de uma
// mudança de tabela.
func TestCatalogOfferKeepsTrialTermsAndRefreshesTheOfferedPrice(t *testing.T) {
	endsAt := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	grantedAt := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	legacyTrial := ps.Plan{Tier: "pro", Name: "Pro", Cycle: "MONTHLY", AmountCents: 229700, TrialDays: 15,
		Limits: ps.Limits{Leads: 1000, Properties: 3000, Users: 20}, Features: []string{"crm"}}
	state := &ps.State{Status: "TRIALING", TrialPlan: &legacyTrial, TrialGrantedAt: &grantedAt, TrialEndsAt: &endsAt, DevMode: true}

	projected := CatalogOffer(state)

	current, ok := FindPlatformPlanCycle("pro", "MONTHLY")
	require.True(t, ok)
	require.Equal(t, int64(98700), projected.TrialPlan.AmountCents)
	require.Equal(t, current.Limits, projected.TrialPlan.Limits)
	require.Equal(t, current.Features, projected.TrialPlan.Features)
	// Identidade e prazo concedidos não mudam.
	require.Equal(t, "pro", projected.TrialPlan.Tier)
	require.Equal(t, "MONTHLY", projected.TrialPlan.Cycle)
	require.Equal(t, 15, projected.TrialPlan.TrialDays)
	require.Equal(t, &endsAt, projected.TrialEndsAt)
	// A projeção é só de leitura: o retrato gravado permanece intacto.
	require.Equal(t, int64(229700), state.TrialPlan.AmountCents)
	require.Equal(t, []string{"crm"}, state.TrialPlan.Features)
}

// Plano contratado mantém o valor assinado (o comprovante e a renovação seguem
// o contrato); só as capacidades são resolvidas no catálogo.
func TestCatalogOfferPreservesContractedAmountForPaidPlan(t *testing.T) {
	contracted := ps.Plan{Tier: "pro", Name: "Pro", Cycle: "MONTHLY", AmountCents: 229700, TrialDays: 15}
	state := &ps.State{Status: "ACTIVE", SubscriptionID: "subs_1", Plan: &contracted}

	projected := CatalogOffer(state)

	require.Equal(t, int64(229700), projected.Plan.AmountCents)
	current, ok := FindPlatformPlanCycle("pro", "MONTHLY")
	require.True(t, ok)
	require.Equal(t, current.Features, projected.Plan.Features)
}

// Estado vazio, tier fora do catálogo e plano ausente não viram pânico nem
// inventam oferta.
func TestCatalogOfferHandlesMissingAndUnknownPlans(t *testing.T) {
	require.Nil(t, CatalogOffer(nil))

	unknown := ps.Plan{Tier: "legacy-tier", Cycle: "MONTHLY", AmountCents: 1000}
	state := &ps.State{Status: "TRIALING", TrialPlan: &unknown}
	projected := CatalogOffer(state)
	require.Equal(t, "legacy-tier", projected.TrialPlan.Tier)
	require.Equal(t, int64(1000), projected.TrialPlan.AmountCents)

	empty := CatalogOffer(&ps.State{Status: "NONE"})
	require.Nil(t, empty.TrialPlan)
	require.Nil(t, empty.Plan)
}

// Preços antigos do Pro continuam resolvendo para o plano Pro: um evento
// atrasado de assinatura já contratada (renovação, pagamento, início de teste)
// precisa ser conciliado, e nunca pode reaparecer como oferta nova.
func TestPlatformPlanForBillingAmountRecognizesLegacyAndCurrentPrices(t *testing.T) {
	for _, tc := range []struct {
		name       string
		amount     int64
		cycle      string
		wantTier   string
		wantAmount int64
		wantOK     bool
	}{
		{"pro mensal atual", 98700, "MONTHLY", "pro", 98700, true},
		{"pro anual atual", 947520, "ANNUALLY", "pro", 947520, true},
		{"pro mensal antigo", legacyProMonthlyAmountCents, "MONTHLY", "pro", legacyProMonthlyAmountCents, true},
		{"pro anual antigo", legacyProAnnualAmountCents, "ANNUALLY", "pro", legacyProAnnualAmountCents, true},
		{"starter mensal", 49700, "MONTHLY", "starter", 49700, true},
		{"valor desconhecido", 123456, "MONTHLY", "", 0, false},
		{"ciclo sem preço antigo", legacyProMonthlyAmountCents, "WEEKLY", "", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, ok := platformPlanForBillingAmount(tc.amount, tc.cycle)
			require.Equal(t, tc.wantOK, ok)
			if !ok {
				return
			}
			require.Equal(t, tc.wantTier, plan.Tier)
			require.Equal(t, tc.wantAmount, plan.AmountCents)
			require.Equal(t, tc.cycle, plan.Cycle)
			// A oferta antiga reconhecida não é vitrine: o catálogo público
			// segue só com os preços atuais.
			for _, published := range PlatformPlans() {
				if published.ContactOnly {
					continue
				}
				require.NotEqual(t, legacyProMonthlyAmountCents, published.AmountCents)
				require.NotEqual(t, legacyProAnnualAmountCents, published.AmountCents)
			}
		})
	}
}

// Assinatura contratada pelo preço antigo continua renovando: o evento chega
// com R$ 2.297,00, o plano gravado é o mesmo valor e o pagamento é registrado
// pelo valor real cobrado — sem virar conflito por causa do catálogo novo.
func TestPlatformSubscriptionKeepsLegacyPricedProRenewing(t *testing.T) {
	s, r, _ := subscriptionFixture()
	ctx := context.Background()
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	legacy := ps.Plan{Tier: "pro", Name: "Pro", Cycle: "MONTHLY", AmountCents: legacyProMonthlyAmountCents,
		Features: []string{FeatureCRM}}
	paidThrough := now.AddDate(0, 0, 5)
	lastEvent := now.AddDate(0, 0, -30)
	r.states["tenant-a"] = &ps.State{Status: "ACTIVE", SubscriptionID: "subs_test", Plan: &legacy,
		PaidThrough: &paidThrough, LastEventAt: &lastEvent, DevMode: true}

	e := paidSubscriptionEvent("legacy-renewal", "subscription.renewed", "", legacy, now)
	require.NoError(t, s.HandleEvent(ctx, e))

	state, err := r.Get(ctx, "tenant-a")
	require.NoError(t, err)
	require.Equal(t, "ACTIVE", state.Status)
	require.Equal(t, "pro", state.Plan.Tier)
	require.Equal(t, legacyProMonthlyAmountCents, state.Plan.AmountCents)
	require.Len(t, r.payments, 1)
	for _, payment := range r.payments {
		require.Equal(t, legacyProMonthlyAmountCents, payment.AmountCents)
	}
	require.True(t, state.PaidThrough.After(paidThrough))
}

// Teste iniciado antes da mudança de tabela: o evento do provedor chega com o
// preço antigo e o estado local ainda não tem plano. O valor antigo resolve o
// Pro pelo catálogo — sem isso o webhook devolveria conflito e o teste ficaria
// sem plano (e sem acesso) até alguém intervir.
func TestPlatformSubscriptionResolvesLegacyPriceWhenLocalPlanIsMissing(t *testing.T) {
	s, r, p := subscriptionFixture()
	ctx := context.Background()
	now := time.Now().UTC()
	trialEnd := now.Add(10 * 24 * time.Hour)
	legacy := ps.Plan{Tier: "pro", Name: "Pro", Cycle: "MONTHLY", AmountCents: legacyProMonthlyAmountCents}
	days := 15
	// Assinatura já reconhecida pelo provedor, mas sem plano materializado no
	// estado local (o caso dos registros anteriores ao catálogo atual).
	state := &ps.State{DevMode: true, SubscriptionID: "subs_test"}
	r.states["tenant-a"] = state

	trial := paidSubscriptionEvent("legacy-trial", "subscription.trial_started", "", legacy, now.Add(-4*24*time.Hour))
	trial.Data.Subscription.Status = "ACTIVE"
	trial.Data.Subscription.TrialDays = &days
	p.subscriptions["subs_test"] = &ps.RemoteSubscription{ID: "subs_test", Amount: legacyProMonthlyAmountCents,
		Status: "ACTIVE", DevMode: true, TrialDays: &days, TrialEndsAt: &trialEnd}
	require.NoError(t, s.HandleEvent(ctx, trial))

	updated, err := r.Get(ctx, "tenant-a")
	require.NoError(t, err)
	require.Equal(t, "TRIALING", updated.Status)
	require.NotNil(t, updated.Plan)
	require.Equal(t, "pro", updated.Plan.Tier)
	require.Equal(t, legacyProMonthlyAmountCents, updated.Plan.AmountCents)
	require.Empty(t, r.payments)
}

// planAddsBenefits exige ganho estrito: nada pode ser perdido (limite ou
// recurso) e pelo menos uma dimensão precisa crescer. É a regra que decide se
// a mudança agendada pode liberar benefícios na hora.
func TestPlanAddsBenefitsRequiresStrictGainWithoutLoss(t *testing.T) {
	starter, _ := FindPlatformPlanCycle("starter", "MONTHLY")
	pro, _ := FindPlatformPlanCycle("pro", "MONTHLY")
	enterprise, _ := FindPlatformPlanCycle("enterprise", "MONTHLY")

	moreLeads := pro
	moreLeads.Limits.Leads = pro.Limits.Leads + 500
	fewerLeads := pro
	fewerLeads.Limits.Leads = starter.Limits.Leads - 1
	// Os recursos vêm do catálogo pelo tier: um plano gravado sem `features`
	// (registros antigos) não perde nem ganha recurso por isso.
	persistedWithoutFeatures := pro
	persistedWithoutFeatures.Features = nil
	relabelled := pro
	relabelled.Name = "Pro Anual"

	for _, tc := range []struct {
		name      string
		current   *ps.Plan
		candidate *ps.Plan
		want      bool
	}{
		{"starter para pro", &starter, &pro, true},
		{"pro para starter", &pro, &starter, false},
		{"mesmo plano", &pro, &pro, false},
		{"mesmo plano com outro nome", &pro, &relabelled, false},
		{"plano gravado sem features", &persistedWithoutFeatures, &pro, false},
		{"plano gravado sem features contra o tier maior", &persistedWithoutFeatures, &moreLeads, true},
		{"mais leads mantendo recursos", &pro, &moreLeads, true},
		{"limite menor", &starter, &fewerLeads, false},
		{"enterprise não tem limites para comparar", &pro, &enterprise, false},
		{"sem plano atual", nil, &pro, false},
		{"sem alvo", &pro, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, planAddsBenefits(tc.current, tc.candidate))
		})
	}
}

// Upgrade confirmado pelo provedor: a empresa inteira passa a ver os recursos e
// limites do plano maior na hora, enquanto a cobrança continua sendo a do plano
// vigente até a renovação.
func TestPlatformSubscriptionConfirmedUpgradeUnlocksEntitlementsImmediately(t *testing.T) {
	s, r, _ := subscriptionFixture()
	s.AccessControlEnabled = true
	ctx := context.Background()
	starter, _ := FindPlatformPlanCycle("starter", "MONTHLY")
	pro, _ := FindPlatformPlanCycle("pro", "MONTHLY")
	paidThrough := time.Now().UTC().AddDate(0, 0, 20)
	r.states["tenant-a"] = &ps.State{Status: "ACTIVE", SubscriptionID: "subs_test", Plan: &starter,
		PaidThrough: &paidThrough, DevMode: true,
		PendingChange: &ps.Change{ID: "subu_confirmado", Plan: pro, ProductID: "prod_pro", RequestedAt: time.Now().UTC()}}

	access, err := s.Access(ctx, "tenant-a")
	require.NoError(t, err)
	require.True(t, access.Entitled)
	require.Equal(t, "pro", access.Plan.Tier)
	require.Contains(t, access.Features, FeatureClientPortal)
	require.Contains(t, access.Features, FeatureReports)
	require.Equal(t, 1000, access.Plan.Limits.Leads)
	require.Equal(t, 20, access.Plan.Limits.Users)

	// O estado gravado segue no Starter: a cobrança do plano maior só entra na
	// próxima renovação.
	state, err := r.Get(ctx, "tenant-a")
	require.NoError(t, err)
	require.Equal(t, "starter", state.Plan.Tier)
	require.Equal(t, starter.AmountCents, state.Plan.AmountCents)
	require.NotNil(t, state.PendingChange)
}

// A intenção recuperável (mudança que o provedor recusou, sem id) NÃO libera
// nada: sem confirmação do provedor não existe upgrade para conceder.
func TestPlatformSubscriptionUnconfirmedUpgradeKeepsCurrentEntitlements(t *testing.T) {
	s, r, _ := subscriptionFixture()
	s.AccessControlEnabled = true
	ctx := context.Background()
	starter, _ := FindPlatformPlanCycle("starter", "MONTHLY")
	pro, _ := FindPlatformPlanCycle("pro", "MONTHLY")
	paidThrough := time.Now().UTC().AddDate(0, 0, 20)
	r.states["tenant-a"] = &ps.State{Status: "ACTIVE", SubscriptionID: "subs_test", Plan: &starter,
		PaidThrough: &paidThrough, DevMode: true,
		PendingChange: &ps.Change{Plan: pro, ProductID: "prod_pro", RequestedAt: time.Now().UTC()}}

	access, err := s.Access(ctx, "tenant-a")
	require.NoError(t, err)
	require.Equal(t, "starter", access.Plan.Tier)
	require.NotContains(t, access.Features, FeatureClientPortal)
	require.Equal(t, 200, access.Plan.Limits.Leads)
}

// Downgrade confirmado mantém os benefícios do plano vigente até a renovação —
// a redução só vale quando o provedor cobrar o plano menor.
func TestPlatformSubscriptionConfirmedDowngradeKeepsBenefitsUntilRenewal(t *testing.T) {
	s, r, _ := subscriptionFixture()
	s.AccessControlEnabled = true
	ctx := context.Background()
	starter, _ := FindPlatformPlanCycle("starter", "MONTHLY")
	pro, _ := FindPlatformPlanCycle("pro", "MONTHLY")
	paidThrough := time.Now().UTC().AddDate(0, 0, 20)
	r.states["tenant-a"] = &ps.State{Status: "ACTIVE", SubscriptionID: "subs_test", Plan: &pro,
		PaidThrough: &paidThrough, DevMode: true,
		PendingChange: &ps.Change{ID: "subu_reducao", Plan: starter, ProductID: "prod_starter", RequestedAt: time.Now().UTC()}}

	access, err := s.Access(ctx, "tenant-a")
	require.NoError(t, err)
	require.Equal(t, "pro", access.Plan.Tier)
	require.Contains(t, access.Features, FeatureClientPortal)
	require.Equal(t, 1000, access.Plan.Limits.Leads)
}

func TestPlatformSubscriptionRebuildsStaleSamePlanCheckoutWithRemainingTrial(t *testing.T) {
	s, r, p := subscriptionFixture()
	ctx := context.Background()
	plan, ok := FindPlatformPlanCycle("starter", "MONTHLY")
	require.True(t, ok)
	plan.TrialDays = 15
	granted := time.Now().UTC().Add(-10 * 24 * time.Hour)
	trialEnd := time.Now().UTC().Add(5*24*time.Hour + 2*time.Hour)
	r.states["tenant-a"] = &ps.State{
		Status:         "EXPIRED",
		TrialGrantedAt: &granted,
		TrialEndsAt:    &trialEnd,
		DevMode:        true,
		Checkout: &ps.Checkout{
			ExternalID: "suitee-stale",
			ID:         "bill_stale",
			URL:        "https://app.abacatepay.com/pay/bill_stale",
			Plan:       plan,
			ProductID:  "prod_starterMONTHLY_t15",
			CreatedAt:  time.Now().UTC().Add(-2 * time.Hour),
		},
	}

	// The provider no longer has this checkout (the link expired/was removed).
	state, err := s.Select(ctx, "tenant-a", "starter", "MONTHLY")
	require.NoError(t, err)
	require.Equal(t, "PENDING", state.Status)
	require.NotNil(t, state.Checkout)
	require.NotEqual(t, "suitee-stale", state.Checkout.ExternalID)
	require.NotEqual(t, "bill_stale", state.Checkout.ID)
	require.Equal(t, 5, state.Checkout.Plan.TrialDays, "a recriação precisa preservar só os dias de teste restantes")
	require.Equal(t, "prod_starterMONTHLY_t5", state.Checkout.ProductID)
	require.Equal(t, 1, p.creates)
}

func TestPlatformSubscriptionCancelClearsCheckoutTheProviderLost(t *testing.T) {
	s, r, p := subscriptionFixture()
	ctx := context.Background()
	state, err := s.Select(ctx, "tenant-a", "starter", "MONTHLY")
	require.NoError(t, err)
	delete(p.checkouts, state.Checkout.ExternalID)

	// A create response can still be in flight; the recent intent is preserved.
	_, err = s.Cancel(ctx, "tenant-a", "other")
	require.ErrorIs(t, err, ps.ErrUnavailable)
	preserved, err := r.Get(ctx, "tenant-a")
	require.NoError(t, err)
	require.NotNil(t, preserved.Checkout)

	// Once it ages out, a 404 from the provider must clear the local reference
	// instead of returning 409 forever.
	preserved.Checkout.CreatedAt = time.Now().UTC().Add(-2 * time.Minute)
	r.states["tenant-a"] = preserved
	_, err = s.Cancel(ctx, "tenant-a", "other")
	require.NoError(t, err)
	after, err := r.Get(ctx, "tenant-a")
	require.NoError(t, err)
	require.Nil(t, after.Checkout)
	require.Equal(t, "NONE", after.Status)
}

func TestPlatformSubscriptionSamePlanPaidCheckoutIsNotReplaced(t *testing.T) {
	s, r, p := subscriptionFixture()
	ctx := context.Background()
	state, err := s.Select(ctx, "tenant-a", "starter", "MONTHLY")
	require.NoError(t, err)
	remote := p.checkouts[state.Checkout.ExternalID]
	require.NotNil(t, remote)
	remote.Status = "PAID"

	_, err = s.Select(ctx, "tenant-a", "starter", "MONTHLY")
	require.ErrorIs(t, err, ps.ErrConflict)
	require.Contains(t, err.Error(), "já foi aprovado")
	require.Equal(t, 1, p.creates)

	after, err := r.Get(ctx, "tenant-a")
	require.NoError(t, err)
	require.NotNil(t, after.Checkout)
	require.Equal(t, state.Checkout.ExternalID, after.Checkout.ExternalID)
}
