package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/collection"
	"github.com/oginoino/toca/toca-financial/internal/domain/reminder_rule"
)

// stubCollectionRepo is a hand-rolled fake instead of mockgen to keep the
// surface intentional: the engine only needs FindByChargeID at evaluation
// time. The map is keyed by charge ID and holds the Collection that
// "contains" that charge — empty value means no collection registered,
// which mimics the production "not yet escalated" case.
type stubCollectionRepo struct {
	byCharge map[string]*collection.Collection
	err      error
}

func (s *stubCollectionRepo) FindByChargeID(_ context.Context, _, chargeID string) (*collection.Collection, error) {
	if s.err != nil {
		return nil, s.err
	}
	if c, ok := s.byCharge[chargeID]; ok {
		return c, nil
	}
	return nil, nil
}

// The methods below are unused by the engine but required by the
// repository interface. Returning zero values is safe — the engine never
// reaches them. Kept as stubs (not panics) so future engine additions that
// brush against them get a deterministic empty result during tests.
func (s *stubCollectionRepo) Create(_ context.Context, _ *collection.Collection) error { return nil }
func (s *stubCollectionRepo) CreateIdempotent(_ context.Context, _ *collection.Collection) error {
	return nil
}
func (s *stubCollectionRepo) GetByID(_ context.Context, _, _ string) (*collection.Collection, error) {
	return nil, nil
}
func (s *stubCollectionRepo) Update(_ context.Context, _ *collection.Collection) error { return nil }
func (s *stubCollectionRepo) List(_ context.Context, _, _ string, _, _ int64) ([]*collection.Collection, int64, error) {
	return nil, 0, nil
}
func (s *stubCollectionRepo) AddAction(_ context.Context, _, _ string, _ collection.Action) error {
	return nil
}
func (s *stubCollectionRepo) FindByContract(_ context.Context, _, _ string) ([]*collection.Collection, error) {
	return nil, nil
}
func (s *stubCollectionRepo) FindActiveByDebtor(_ context.Context, _, _ string) (*collection.Collection, error) {
	return nil, nil
}
func (s *stubCollectionRepo) GetStats(_ context.Context, _ string) (*collection.CollectionStats, error) {
	return nil, nil
}

type fakeRuleRepo struct {
	rules []*reminder_rule.ReminderRule
}

func (f *fakeRuleRepo) Create(_ context.Context, _ *reminder_rule.ReminderRule) error { return nil }
func (f *fakeRuleRepo) GetByID(_ context.Context, _, _ string) (*reminder_rule.ReminderRule, error) {
	return nil, nil
}
func (f *fakeRuleRepo) ListByTenant(_ context.Context, _ string) ([]*reminder_rule.ReminderRule, error) {
	return f.rules, nil
}
func (f *fakeRuleRepo) ListEnabled(_ context.Context, _ string) ([]*reminder_rule.ReminderRule, error) {
	out := make([]*reminder_rule.ReminderRule, 0)
	for _, r := range f.rules {
		if r.Enabled {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *fakeRuleRepo) Update(_ context.Context, _ *reminder_rule.ReminderRule) error { return nil }
func (f *fakeRuleRepo) Delete(_ context.Context, _, _ string) error                   { return nil }
func (f *fakeRuleRepo) ListDistinctTenantIDs(_ context.Context) ([]string, error)     { return nil, nil }

type stubNotifier struct {
	calls       []string
	lastOptions NotifyOptions
	err         error
}

func (s *stubNotifier) NotifyWithOptions(_ context.Context, _, chargeID string, opts NotifyOptions) (*NotifyResult, error) {
	s.calls = append(s.calls, chargeID)
	s.lastOptions = opts
	if s.err != nil {
		return nil, s.err
	}
	return &NotifyResult{ChargeID: chargeID, Channels: []string{"email"}}, nil
}

func makeChargeDue(t *testing.T, id string, due time.Time, status charge.Status) *charge.Charge {
	t.Helper()
	c, err := charge.New("c-1", "p-1", "o-1", "prop-1", "t-1", charge.TypeRent, "Aluguel", "2026-05", 1500, due)
	if err != nil {
		t.Fatalf("charge.New: %v", err)
	}
	c.ID = id
	c.Status = status
	return c
}

func TestEvaluateAndDispatch_firesMatchingRules(t *testing.T) {
	due := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	chRepo := &mockChargeRepo{
		charges: []*charge.Charge{makeChargeDue(t, "ch-1", due, charge.StatusPending)},
		total:   1,
	}
	r3before, _ := reminder_rule.New("t-1", "3 dias antes", reminder_rule.TriggerBeforeDue, 3, []reminder_rule.Channel{reminder_rule.ChannelEmail}, "msg")
	r3before.Enabled = true
	rules := &fakeRuleRepo{rules: []*reminder_rule.ReminderRule{r3before}}
	svc := NewReminderRuleService(rules, chRepo)
	notifier := &stubNotifier{}
	svc.SetChargeService(notifier)

	res, err := svc.EvaluateAndDispatch(context.Background(), "t-1", time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.NotifiesSent != 1 {
		t.Fatalf("expected 1 notify sent, got %+v", res)
	}
	if len(notifier.calls) != 1 || notifier.calls[0] != "ch-1" {
		t.Fatalf("unexpected calls: %v", notifier.calls)
	}
	// Confirm the rule's template, SentBy and SentByLabel flowed through.
	if notifier.lastOptions.Template != "msg" {
		t.Errorf("expected rule template forwarded, got %q", notifier.lastOptions.Template)
	}
	if notifier.lastOptions.SentBy != "rule:"+r3before.ID {
		t.Errorf("expected SentBy=rule:%s, got %q", r3before.ID, notifier.lastOptions.SentBy)
	}
	if notifier.lastOptions.SentByLabel != "Régua: 3 dias antes" {
		t.Errorf("expected SentByLabel=Régua: 3 dias antes, got %q", notifier.lastOptions.SentByLabel)
	}
	if len(notifier.lastOptions.Channels) != 1 || notifier.lastOptions.Channels[0] != "email" {
		t.Errorf("expected channels=[email] from rule, got %v", notifier.lastOptions.Channels)
	}
}

func TestEvaluateAndDispatch_DoesNotNotifyReversedCharge(t *testing.T) {
	due := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	chRepo := &mockChargeRepo{
		charges: []*charge.Charge{makeChargeDue(t, "ch-reversed", due, charge.StatusReversed)},
		total:   1,
	}
	rule, _ := reminder_rule.New("t-1", "no vencimento", reminder_rule.TriggerOnDue, 0, []reminder_rule.Channel{reminder_rule.ChannelEmail}, "msg")
	rule.Enabled = true
	svc := NewReminderRuleService(&fakeRuleRepo{rules: []*reminder_rule.ReminderRule{rule}}, chRepo)
	notifier := &stubNotifier{}
	svc.SetChargeService(notifier)

	res, err := svc.EvaluateAndDispatch(context.Background(), "t-1", due)
	if err != nil {
		t.Fatalf("EvaluateAndDispatch() error = %v", err)
	}
	if res.ChargesMatched != 0 || res.NotifiesSent != 0 || len(notifier.calls) != 0 {
		t.Fatalf("REVERSED charge reached reminder flow: result=%+v calls=%v", res, notifier.calls)
	}
}

func TestEvaluateAndDispatch_skipsAlreadyRemindedToday(t *testing.T) {
	due := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	c := makeChargeDue(t, "ch-1", due, charge.StatusPending)
	// Today's reminder already recorded — engine should skip on the same day.
	c.RemindersSent = []charge.ReminderEntry{{SentAt: time.Date(2026, 5, 17, 9, 0, 0, 0, time.UTC), Channels: []string{"email"}}}
	chRepo := &mockChargeRepo{charges: []*charge.Charge{c}, total: 1}

	r, _ := reminder_rule.New("t-1", "3 dias antes", reminder_rule.TriggerBeforeDue, 3, []reminder_rule.Channel{reminder_rule.ChannelEmail}, "msg")
	r.Enabled = true
	svc := NewReminderRuleService(&fakeRuleRepo{rules: []*reminder_rule.ReminderRule{r}}, chRepo)
	notifier := &stubNotifier{}
	svc.SetChargeService(notifier)

	res, err := svc.EvaluateAndDispatch(context.Background(), "t-1", time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if res.NotifiesSent != 0 || res.NotifySkipped != 1 {
		t.Fatalf("expected 0 sent / 1 skipped, got %+v", res)
	}
	if len(notifier.calls) != 0 {
		t.Fatalf("expected no notify calls, got %v", notifier.calls)
	}
}

func TestEvaluateAndDispatch_ignoresPaidCharges(t *testing.T) {
	due := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	chRepo := &mockChargeRepo{
		charges: []*charge.Charge{makeChargeDue(t, "ch-1", due, charge.StatusPaid)},
		total:   1,
	}
	r, _ := reminder_rule.New("t-1", "3 dias antes", reminder_rule.TriggerBeforeDue, 3, []reminder_rule.Channel{reminder_rule.ChannelEmail}, "msg")
	r.Enabled = true
	svc := NewReminderRuleService(&fakeRuleRepo{rules: []*reminder_rule.ReminderRule{r}}, chRepo)
	notifier := &stubNotifier{}
	svc.SetChargeService(notifier)

	res, err := svc.EvaluateAndDispatch(context.Background(), "t-1", time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if res.NotifiesSent != 0 || len(notifier.calls) != 0 {
		t.Fatalf("paid charge should be ignored, got %+v calls=%v", res, notifier.calls)
	}
}

func TestEvaluateAndDispatch_capturesNotifyErrors(t *testing.T) {
	due := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	chRepo := &mockChargeRepo{
		charges: []*charge.Charge{makeChargeDue(t, "ch-1", due, charge.StatusPending)},
		total:   1,
	}
	r, _ := reminder_rule.New("t-1", "no dia", reminder_rule.TriggerOnDue, 0, []reminder_rule.Channel{reminder_rule.ChannelEmail}, "msg")
	r.Enabled = true
	svc := NewReminderRuleService(&fakeRuleRepo{rules: []*reminder_rule.ReminderRule{r}}, chRepo)
	notifier := &stubNotifier{err: errors.New("upstream timeout")}
	svc.SetChargeService(notifier)

	res, err := svc.EvaluateAndDispatch(context.Background(), "t-1", due)
	if err != nil {
		t.Fatal(err)
	}
	if res.NotifiesSent != 0 {
		t.Fatalf("expected no successful sends, got %+v", res)
	}
	if len(res.Errors) != 1 {
		t.Fatalf("expected 1 error recorded, got %v", res.Errors)
	}
}

func TestEvaluateAndDispatch_requiresChargeService(t *testing.T) {
	svc := NewReminderRuleService(&fakeRuleRepo{}, &mockChargeRepo{})
	if _, err := svc.EvaluateAndDispatch(context.Background(), "t-1", time.Now()); err == nil {
		t.Fatal("expected error when chargeSvc is nil")
	}
}

// ---------------------------------------------------------------------------
// Issue #446 — pause régua when Collection is NEGOTIATED / JUDICIAL /
// GUARANTEE_ACTIVATED / RESOLVED. The cases below cover both the happy
// path (no collection → behave like before) and every paused status so the
// guard is locked against accidental regressions.
// ---------------------------------------------------------------------------

// helper: builds the engine with one rule + one matching charge, and a
// caller-supplied stubCollectionRepo so each test only declares what it cares
// about. Returns the engine, the notifier (to assert calls), and the rule
// so tests can name it in error messages.
func newPausedEngineFixture(t *testing.T, colRepo *stubCollectionRepo) (*ReminderRuleService, *stubNotifier, *reminder_rule.ReminderRule, time.Time) {
	t.Helper()
	due := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	ref := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC) // 3 days before due
	chRepo := &mockChargeRepo{
		charges: []*charge.Charge{makeChargeDue(t, "ch-1", due, charge.StatusPending)},
		total:   1,
	}
	r, _ := reminder_rule.New("t-1", "3 dias antes", reminder_rule.TriggerBeforeDue, 3, []reminder_rule.Channel{reminder_rule.ChannelEmail}, "msg")
	r.Enabled = true
	svc := NewReminderRuleService(&fakeRuleRepo{rules: []*reminder_rule.ReminderRule{r}}, chRepo)
	notifier := &stubNotifier{}
	svc.SetChargeService(notifier)
	if colRepo != nil {
		svc.SetCollectionRepo(colRepo)
	}
	return svc, notifier, r, ref
}

// makeCollection returns a Collection with the requested status pointing at
// chargeID. Only the fields the engine reads are populated.
func makeCollection(chargeID string, status collection.Status) *collection.Collection {
	return &collection.Collection{
		ID:        "col-" + chargeID,
		TenantID:  "t-1",
		ChargeIDs: []string{chargeID},
		Status:    status,
	}
}

func TestEvaluateAndDispatch_pausedWhenCollectionNegotiated(t *testing.T) {
	colRepo := &stubCollectionRepo{
		byCharge: map[string]*collection.Collection{
			"ch-1": makeCollection("ch-1", collection.StatusNegotiated),
		},
	}
	svc, notifier, _, ref := newPausedEngineFixture(t, colRepo)

	res, err := svc.EvaluateAndDispatch(context.Background(), "t-1", ref)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.NotifiesSent != 0 {
		t.Errorf("expected 0 sent, got %d", res.NotifiesSent)
	}
	if res.NotifyPausedByCollection != 1 {
		t.Errorf("expected 1 paused_by_collection, got %d", res.NotifyPausedByCollection)
	}
	if res.ChargesMatched != 1 {
		t.Errorf("expected charge to still be counted as matched, got %d", res.ChargesMatched)
	}
	if len(notifier.calls) != 0 {
		t.Errorf("expected no notify dispatch, got %v", notifier.calls)
	}
}

func TestEvaluateAndDispatch_pausedWhenCollectionJudicial(t *testing.T) {
	colRepo := &stubCollectionRepo{
		byCharge: map[string]*collection.Collection{
			"ch-1": makeCollection("ch-1", collection.StatusJudicial),
		},
	}
	svc, notifier, _, ref := newPausedEngineFixture(t, colRepo)

	res, err := svc.EvaluateAndDispatch(context.Background(), "t-1", ref)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.NotifyPausedByCollection != 1 || len(notifier.calls) != 0 {
		t.Fatalf("JUDICIAL must pause régua: %+v calls=%v", res, notifier.calls)
	}
}

func TestEvaluateAndDispatch_pausedWhenCollectionGuaranteeActivated(t *testing.T) {
	colRepo := &stubCollectionRepo{
		byCharge: map[string]*collection.Collection{
			"ch-1": makeCollection("ch-1", collection.StatusGuaranteeActivated),
		},
	}
	svc, notifier, _, ref := newPausedEngineFixture(t, colRepo)

	res, err := svc.EvaluateAndDispatch(context.Background(), "t-1", ref)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.NotifyPausedByCollection != 1 || len(notifier.calls) != 0 {
		t.Fatalf("GUARANTEE_ACTIVATED must pause régua: %+v calls=%v", res, notifier.calls)
	}
}

func TestEvaluateAndDispatch_pausedWhenCollectionResolved(t *testing.T) {
	colRepo := &stubCollectionRepo{
		byCharge: map[string]*collection.Collection{
			"ch-1": makeCollection("ch-1", collection.StatusResolved),
		},
	}
	svc, notifier, _, ref := newPausedEngineFixture(t, colRepo)

	res, err := svc.EvaluateAndDispatch(context.Background(), "t-1", ref)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.NotifyPausedByCollection != 1 || len(notifier.calls) != 0 {
		t.Fatalf("RESOLVED must pause régua: %+v calls=%v", res, notifier.calls)
	}
}

func TestEvaluateAndDispatch_proceedsWhenCollectionActive(t *testing.T) {
	colRepo := &stubCollectionRepo{
		byCharge: map[string]*collection.Collection{
			"ch-1": makeCollection("ch-1", collection.StatusActive),
		},
	}
	svc, notifier, _, ref := newPausedEngineFixture(t, colRepo)

	res, err := svc.EvaluateAndDispatch(context.Background(), "t-1", ref)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.NotifiesSent != 1 {
		t.Fatalf("ACTIVE collection should not pause, got %+v", res)
	}
	if res.NotifyPausedByCollection != 0 {
		t.Fatalf("ACTIVE should not bump paused counter, got %d", res.NotifyPausedByCollection)
	}
	if len(notifier.calls) != 1 || notifier.calls[0] != "ch-1" {
		t.Errorf("expected notifier call for ch-1, got %v", notifier.calls)
	}
}

func TestEvaluateAndDispatch_proceedsWhenNoCollection(t *testing.T) {
	// Empty map → FindByChargeID returns (nil, nil). Engine must treat
	// "sem collection" as "ainda não escalou para delinquency" and
	// dispatch normalmente. Esse é o caminho mais comum (a maioria das
	// charges nunca chega ao stage de Collection).
	colRepo := &stubCollectionRepo{byCharge: map[string]*collection.Collection{}}
	svc, notifier, _, ref := newPausedEngineFixture(t, colRepo)

	res, err := svc.EvaluateAndDispatch(context.Background(), "t-1", ref)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.NotifiesSent != 1 || res.NotifyPausedByCollection != 0 || len(notifier.calls) != 1 {
		t.Fatalf("no collection should not pause, got %+v calls=%v", res, notifier.calls)
	}
}

func TestEvaluateAndDispatch_proceedsWhenCollectionRepoUnwired(t *testing.T) {
	// Without SetCollectionRepo the engine must keep the legacy behaviour
	// — covers tenants ainda não migrados para a delinquency engine. This
	// is the safety net that lets us roll out the feature gradually.
	svc, notifier, _, ref := newPausedEngineFixture(t, nil)

	res, err := svc.EvaluateAndDispatch(context.Background(), "t-1", ref)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.NotifiesSent != 1 || res.NotifyPausedByCollection != 0 || len(notifier.calls) != 1 {
		t.Fatalf("nil collectionRepo should behave like before, got %+v calls=%v", res, notifier.calls)
	}
}

func TestEvaluateAndDispatch_failOpenOnCollectionRepoError(t *testing.T) {
	// Fail-open: a transient DB error in the collection lookup must NOT
	// kill the régua para os outros locatários. The engine records the
	// error and proceeds with the dispatch — perdemos a salvaguarda nesse
	// tick, mas o dia seguinte (ou o monitoring on the Errors slice) pega.
	colRepo := &stubCollectionRepo{err: errors.New("postgres connection reset")}
	svc, notifier, _, ref := newPausedEngineFixture(t, colRepo)

	res, err := svc.EvaluateAndDispatch(context.Background(), "t-1", ref)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.NotifiesSent != 1 {
		t.Fatalf("fail-open: expected dispatch to proceed, got %+v", res)
	}
	if len(res.Errors) != 1 {
		t.Fatalf("expected the lookup error to be recorded, got %v", res.Errors)
	}
	if len(notifier.calls) != 1 {
		t.Errorf("expected one notify call, got %v", notifier.calls)
	}
}

// isDunningPausedByCollection guards a small enum; this table test pins
// the contract so adding a future status doesn't accidentally pause the
// régua. Update the table along with the helper when a new Status lands.
func TestIsDunningPausedByCollection_table(t *testing.T) {
	cases := []struct {
		status collection.Status
		paused bool
	}{
		{collection.StatusActive, false},
		{collection.StatusNegotiated, true},
		{collection.StatusJudicial, true},
		{collection.StatusGuaranteeActivated, true},
		{collection.StatusResolved, true},
		{"", false}, // unknown / no collection registered
	}
	for _, tc := range cases {
		if got := isDunningPausedByCollection(tc.status); got != tc.paused {
			t.Errorf("isDunningPausedByCollection(%q) = %v, want %v", tc.status, got, tc.paused)
		}
	}
}
