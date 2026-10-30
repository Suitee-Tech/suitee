package services

import (
	"context"
	"testing"
	"time"

	ps "github.com/oginoino/toca/toca-financial/internal/domain/platform_subscription"
	"github.com/stretchr/testify/require"
)

// accessTestService devolve o serviço do fixture com o enforcement ligado — é o
// estado do beta, onde o controle de planos aplica os bloqueios.
func accessTestService() (*PlatformSubscriptionService, *subscriptionMemory) {
	s, r, _ := subscriptionFixture()
	s.AccessControlEnabled = true
	return s, r
}

func seedState(t *testing.T, r *subscriptionMemory, tenant string, seed func(*ps.State)) {
	t.Helper()
	_, err := r.Update(context.Background(), tenant, func(_ context.Context, x *ps.State) error {
		seed(x)
		return nil
	})
	require.NoError(t, err)
}

func stateOf(t *testing.T, r *subscriptionMemory, tenant string) *ps.State {
	t.Helper()
	state, err := r.Get(context.Background(), tenant)
	require.NoError(t, err)
	return state
}

func hasFeature(features []string, feature string) bool {
	for _, item := range features {
		if item == feature {
			return true
		}
	}
	return false
}

// O primeiro acesso autenticado inicia o trial Pro de 15 dias, sem cartão, e ele
// nunca é reiniciado para a mesma imobiliária.
func TestPlatformSubscriptionAccess_GrantsInitialTrialOnce(t *testing.T) {
	s, r := accessTestService()
	ctx := context.Background()

	access, err := s.Access(ctx, "tenant-trial")
	require.NoError(t, err)
	require.True(t, access.Entitled)
	require.True(t, access.ControlEnabled)
	require.Equal(t, "TRIALING", access.Status)
	require.NotNil(t, access.Plan)
	require.Equal(t, "pro", access.Plan.Tier)
	require.NotNil(t, access.TrialEndsAt)
	require.WithinDuration(t, time.Now().UTC().AddDate(0, 0, TrialDays), *access.TrialEndsAt, time.Minute)
	for _, feature := range proFeatures {
		require.True(t, hasFeature(access.Features, feature), "trial Pro deve liberar %s", feature)
	}

	// O estado persistido é o que a contratação posterior respeita: o trial fica
	// registrado uma única vez, com a data em que começou.
	state := stateOf(t, r, "tenant-trial")
	require.NotNil(t, state.TrialGrantedAt)
	require.NotNil(t, state.TrialEndsAt)
	require.NotNil(t, state.TrialPlan)
	require.Equal(t, "TRIALING", state.Status)
	grantedAt := *state.TrialGrantedAt
	endsAt := *state.TrialEndsAt

	second, err := s.Access(ctx, "tenant-trial")
	require.NoError(t, err)
	require.Equal(t, "TRIALING", second.Status)
	state = stateOf(t, r, "tenant-trial")
	require.WithinDuration(t, grantedAt, *state.TrialGrantedAt, time.Second)
	require.WithinDuration(t, endsAt, *state.TrialEndsAt, time.Second)
}

// Fim do trial: o acesso vira EXPIRED, sem funcionalidades, com o motivo que a
// interface usa para explicar a escolha do plano.
func TestPlatformSubscriptionAccess_ExpiresTrial(t *testing.T) {
	s, r := accessTestService()
	ctx := context.Background()

	plan, ok := FindPlatformPlanCycle("pro", "MONTHLY")
	require.True(t, ok)
	past := time.Now().UTC().Add(-time.Hour)
	granted := past.AddDate(0, 0, -TrialDays)
	seedState(t, r, "tenant-expired", func(x *ps.State) {
		x.Status = "TRIALING"
		x.TrialPlan = &plan
		x.TrialGrantedAt = &granted
		x.TrialEndsAt = &past
	})

	access, err := s.Access(ctx, "tenant-expired")
	require.NoError(t, err)
	require.False(t, access.Entitled)
	require.Empty(t, access.Features)
	require.Equal(t, "EXPIRED", access.Status)
	require.Equal(t, "trial_or_paid_period_ended", access.Reason)
	// O acesso não carrega plano quando nada está liberado (é o que o gateway
	// usa para decidir), mas o estado da assinatura mantém o plano
	// experimentado: é dele que a tela de planos monta o card "Plano Atual".
	require.Nil(t, access.Plan)
	require.NotNil(t, stateOf(t, r, "tenant-expired").TrialPlan)

	// A expiração é persistida e a consulta seguinte não reinicia o trial.
	require.Equal(t, "EXPIRED", stateOf(t, r, "tenant-expired").Status)
	again, err := s.Access(ctx, "tenant-expired")
	require.NoError(t, err)
	require.False(t, again.Entitled)
	require.Equal(t, "EXPIRED", again.Status)
}

// Quem já teve assinatura não recebe trial de novo.
func TestPlatformSubscriptionAccess_NeverRegrantsTrialAfterSubscription(t *testing.T) {
	s, r := accessTestService()
	ctx := context.Background()

	granted := time.Now().UTC().AddDate(0, 0, -90)
	expired := time.Now().UTC().AddDate(0, 0, -75)
	seedState(t, r, "tenant-used", func(x *ps.State) {
		x.Status = "EXPIRED"
		x.TrialGrantedAt = &granted
		x.TrialEndsAt = &expired
	})

	access, err := s.Access(ctx, "tenant-used")
	require.NoError(t, err)
	require.False(t, access.Entitled)
	require.Nil(t, access.Plan)
	require.Equal(t, "EXPIRED", access.Status)
	require.Equal(t, "trial_or_paid_period_ended", access.Reason)
}

// Assinatura vigente: as funcionalidades vêm do plano contratado — Starter não
// recebe relatórios, portal, assinatura digital nem inteligência.
func TestPlatformSubscriptionAccess_ResolvesTierFeatures(t *testing.T) {
	s, r := accessTestService()
	ctx := context.Background()
	paidThrough := time.Now().UTC().AddDate(0, 0, 20)

	starter, ok := FindPlatformPlanCycle("starter", "MONTHLY")
	require.True(t, ok)
	seedState(t, r, "tenant-starter", func(x *ps.State) {
		x.Status = "ACTIVE"
		x.SubscriptionID = "subs_starter"
		// Plano cru, sem Features: é o formato gravado antes desta rodada.
		x.Plan = &ps.Plan{Tier: starter.Tier, Name: starter.Name, Cycle: starter.Cycle, AmountCents: starter.AmountCents, Limits: starter.Limits}
		x.PaidThrough = &paidThrough
	})

	access, err := s.Access(ctx, "tenant-starter")
	require.NoError(t, err)
	require.True(t, access.Entitled)
	require.Equal(t, "ACTIVE", access.Status)
	for _, feature := range starterFeatures {
		require.True(t, hasFeature(access.Features, feature), "Starter deve manter %s", feature)
	}
	for _, feature := range []string{FeatureReports, FeatureIntelligence, FeatureMCP, FeatureClientPortal, FeatureElectronicSignature} {
		require.False(t, hasFeature(access.Features, feature), "Starter não deve liberar %s", feature)
	}

	pro, ok := FindPlatformPlanCycle("pro", "MONTHLY")
	require.True(t, ok)
	seedState(t, r, "tenant-pro", func(x *ps.State) {
		x.Status = "ACTIVE"
		x.SubscriptionID = "subs_pro"
		x.Plan = &pro
		x.PaidThrough = &paidThrough
	})
	access, err = s.Access(ctx, "tenant-pro")
	require.NoError(t, err)
	require.True(t, access.Entitled)
	for _, feature := range proFeatures {
		require.True(t, hasFeature(access.Features, feature), "Pro deve liberar %s", feature)
	}
}

// Falha de cobrança: o acesso continua até o fim do período já pago e, depois
// dele, o motivo é regularização — não "teste encerrado".
func TestPlatformSubscriptionAccess_PastDueKeepsPaidPeriod(t *testing.T) {
	s, r := accessTestService()
	ctx := context.Background()
	pro, ok := FindPlatformPlanCycle("pro", "MONTHLY")
	require.True(t, ok)

	future := time.Now().UTC().AddDate(0, 0, 5)
	seedState(t, r, "tenant-grace", func(x *ps.State) {
		x.Status = "PAST_DUE"
		x.SubscriptionID = "subs_grace"
		x.Plan = &pro
		x.PaidThrough = &future
	})
	access, err := s.Access(ctx, "tenant-grace")
	require.NoError(t, err)
	require.True(t, access.Entitled, "período pago ainda cobre o acesso")
	require.True(t, hasFeature(access.Features, FeatureReports))

	pastDue := time.Now().UTC().Add(-24 * time.Hour)
	seedState(t, r, "tenant-late", func(x *ps.State) {
		x.Status = "PAST_DUE"
		x.SubscriptionID = "subs_late"
		x.Plan = &pro
		x.PaidThrough = &pastDue
	})
	access, err = s.Access(ctx, "tenant-late")
	require.NoError(t, err)
	require.False(t, access.Entitled)
	require.Equal(t, "payment_required", access.Reason)
	require.Empty(t, access.Features)
}

// Com o enforcement desligado (produção e ambiente local por padrão) ninguém é
// bloqueado, todas as funcionalidades aparecem liberadas e nenhum trial é
// iniciado — o relógio do teste grátis só começa quando o controle ligar.
func TestPlatformSubscriptionAccess_DisabledKeepsEveryoneUnrestricted(t *testing.T) {
	s, r, _ := subscriptionFixture()
	ctx := context.Background()

	access, err := s.Access(ctx, "tenant-off")
	require.NoError(t, err)
	require.False(t, access.ControlEnabled)
	require.True(t, access.Entitled)
	require.Equal(t, "UNRESTRICTED", access.Status)
	require.True(t, hasFeature(access.Features, FeatureReports))
	state := stateOf(t, r, "tenant-off")
	require.Nil(t, state.TrialGrantedAt)
	require.Nil(t, state.TrialEndsAt)
	require.Equal(t, "NONE", state.Status)
}

// Cancelou mas ainda tem período pago: o acesso segue até o fim dele.
func TestPlatformSubscriptionAccess_CancelledKeepsPaidPeriod(t *testing.T) {
	s, r := accessTestService()
	ctx := context.Background()
	starter, ok := FindPlatformPlanCycle("starter", "MONTHLY")
	require.True(t, ok)

	future := time.Now().UTC().AddDate(0, 0, 10)
	seedState(t, r, "tenant-cancelled", func(x *ps.State) {
		x.Status = "CANCELLED"
		x.SubscriptionID = "subs_cancelled"
		x.Plan = &starter
		x.PaidThrough = &future
	})
	access, err := s.Access(ctx, "tenant-cancelled")
	require.NoError(t, err)
	require.True(t, access.Entitled)

	past := time.Now().UTC().Add(-time.Hour)
	seedState(t, r, "tenant-cancelled", func(x *ps.State) {
		x.PaidThrough = &past
	})
	access, err = s.Access(ctx, "tenant-cancelled")
	require.NoError(t, err)
	require.False(t, access.Entitled)
	require.Equal(t, "trial_or_paid_period_ended", access.Reason)
}
