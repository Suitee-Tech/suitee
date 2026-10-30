package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	ps "github.com/oginoino/toca/toca-financial/internal/domain/platform_subscription"
	"github.com/oginoino/toca/toca-financial/internal/providers"
)

type PlatformSubscriptionService struct {
	Repo                 ps.Repository
	Provider             ps.Provider
	ReturnURL            string
	DevMode              bool
	Enabled              bool
	AccessControlEnabled bool
}

// Access returns the tenant's current plan entitlements. On the first
// authenticated app access, it grants one no-card 15-day Pro trial. The
// repository serializes the grant, so simultaneous first requests cannot
// extend or duplicate it.
func (s *PlatformSubscriptionService) Access(ctx context.Context, tenant string) (*ps.Access, error) {
	state, err := s.Repo.Get(ctx, tenant)
	if err != nil {
		return nil, err
	}
	// A oferta exibida (inclusive a primeira cobrança do teste) é a do catálogo
	// atual, não a do dia em que o teste foi concedido.
	state = CatalogOffer(state)
	if !s.AccessControlEnabled {
		access := resolvedAccess(state, time.Now(), false)
		access.SubscriptionEnabled = s.Enabled
		return access, nil
	}

	now := time.Now().UTC()
	if needsAccessUpdate(state, now) {
		state, err = s.Repo.Update(ctx, tenant, func(_ context.Context, current *ps.State) error {
			if shouldGrantInitialTrial(current) {
				plan, ok := FindPlatformPlanCycle("pro", "MONTHLY")
				if !ok || plan.TrialDays <= 0 {
					return ps.ErrInvalidPlan
				}
				grantedAt := now
				endsAt := now.AddDate(0, 0, plan.TrialDays)
				current.TrialPlan = &plan
				current.TrialGrantedAt = &grantedAt
				current.TrialEndsAt = &endsAt
				current.Status = "TRIALING"
				current.DevMode = s.DevMode
			}
			if current.TrialPlan != nil && current.SubscriptionID == "" && current.TrialEndsAt != nil && !current.TrialEndsAt.After(now) && (current.Status == "TRIALING" || current.Status == "PENDING") {
				current.Status = "EXPIRED"
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		state = CatalogOffer(state)
	}
	access := resolvedAccess(state, now, true)
	access.SubscriptionEnabled = s.Enabled
	return access, nil
}

func shouldGrantInitialTrial(state *ps.State) bool {
	return state != nil && (state.Status == "" || state.Status == "NONE") && state.SubscriptionID == "" && state.Checkout == nil && state.Plan == nil && state.TrialPlan == nil && state.TrialGrantedAt == nil && state.TrialEndsAt == nil
}

func needsAccessUpdate(state *ps.State, now time.Time) bool {
	if state == nil {
		return false
	}
	return shouldGrantInitialTrial(state) || (state.TrialPlan != nil && state.SubscriptionID == "" && state.TrialEndsAt != nil && !state.TrialEndsAt.After(now) && (state.Status == "TRIALING" || state.Status == "PENDING"))
}

func resolvedAccess(state *ps.State, now time.Time, controlEnabled bool) *ps.Access {
	access := &ps.Access{
		ControlEnabled: controlEnabled,
		Status:         "NONE",
		Features:       []string{},
	}
	if state == nil {
		if !controlEnabled {
			access.Entitled = true
			access.Status = "UNRESTRICTED"
			access.Features = append([]string(nil), proFeatures...)
		}
		return access
	}
	access.Status = state.Status
	access.TrialEndsAt = state.TrialEndsAt
	access.PaidThrough = state.PaidThrough
	plan := state.Plan
	if state.TrialPlan != nil && state.TrialEndsAt != nil && now.Before(*state.TrialEndsAt) {
		plan = state.TrialPlan
		access.Status = "TRIALING"
		access.Entitled = true
	} else {
		switch state.Status {
		case "ACTIVE":
			access.Entitled = plan != nil && (state.PaidThrough == nil || state.PaidThrough.After(now))
		case "TRIALING":
			access.Entitled = plan != nil && state.TrialEndsAt != nil && state.TrialEndsAt.After(now)
		case "CANCELLED", "PAST_DUE":
			access.Entitled = plan != nil && state.PaidThrough != nil && state.PaidThrough.After(now)
		}
	}
	if plan != nil {
		// An upgrade changes tenant entitlements as soon as the PROVIDER confirms
		// the scheduled billing change — only a confirmed change carries the
		// provider id. The provider still applies the new price at renewal;
		// downgrades keep the current entitlements until then. A change that
		// failed at the provider keeps only the recoverable intent (no id) and
		// must not unlock anything.
		if access.Entitled && state.PendingChange != nil && state.PendingChange.ID != "" &&
			planAddsBenefits(plan, &state.PendingChange.Plan) {
			plan = &state.PendingChange.Plan
		}
		plan = planWithCatalogFeatures(plan)
		access.Plan = plan
	}
	if plan != nil && access.Entitled {
		access.Features = append([]string(nil), plan.Features...)
	}
	if !controlEnabled {
		access.Entitled = true
		if access.Status == "NONE" {
			access.Status = "UNRESTRICTED"
		}
		access.Features = append([]string(nil), proFeatures...)
		return access
	}
	if !access.Entitled {
		switch {
		case state.Status == "PAST_DUE":
			access.Reason = "payment_required"
		case state.TrialGrantedAt != nil || state.Status == "EXPIRED" || state.Status == "CANCELLED":
			access.Status = "EXPIRED"
			access.Reason = "trial_or_paid_period_ended"
		default:
			access.Reason = "subscription_required"
		}
	}
	return access
}

// planAddsBenefits reports a strict entitlement increase. A candidate must
// preserve every current feature and limit, and improve at least one of them.
func planAddsBenefits(current, candidate *ps.Plan) bool {
	if current == nil || candidate == nil ||
		candidate.Limits.Leads < current.Limits.Leads ||
		candidate.Limits.Properties < current.Limits.Properties ||
		candidate.Limits.Users < current.Limits.Users {
		return false
	}

	added := candidate.Limits.Leads > current.Limits.Leads ||
		candidate.Limits.Properties > current.Limits.Properties ||
		candidate.Limits.Users > current.Limits.Users
	currentFeatures := planWithCatalogFeatures(current).Features
	candidateFeatures := planWithCatalogFeatures(candidate).Features
	candidateSet := make(map[string]struct{}, len(candidateFeatures))
	for _, feature := range candidateFeatures {
		candidateSet[feature] = struct{}{}
	}
	for _, feature := range currentFeatures {
		if _, ok := candidateSet[feature]; !ok {
			return false
		}
	}
	if len(candidateFeatures) > len(currentFeatures) {
		added = true
	}
	return added
}

// planWithCatalogFeatures resolves the capabilities of a persisted plan from
// the server catalog, so a plan saved before feature entitlements existed keeps
// the capabilities its tier promises.
func planWithCatalogFeatures(plan *ps.Plan) *ps.Plan {
	if plan == nil {
		return nil
	}
	resolved := *plan
	cycle := resolved.Cycle
	if cycle == "" {
		cycle = "MONTHLY"
	}
	if catalogPlan, ok := FindPlatformPlanCycle(resolved.Tier, cycle); ok {
		resolved.Features = append([]string(nil), catalogPlan.Features...)
	} else {
		resolved.Features = nil
	}
	return &resolved
}

// CatalogOffer projects a persisted state onto the current offer. The trial
// plan is an offer, not a contract: it follows today's catalog (price, limits,
// description, highlights) so the page tells the customer what the first charge
// will actually be after a price change — the stored snapshot keeps documenting
// what was granted on the day. A contracted plan keeps the amount it was signed
// at; only its capabilities are resolved from the catalog. Read-time projection
// only: nothing here is persisted.
func CatalogOffer(state *ps.State) *ps.State {
	if state == nil {
		return nil
	}
	projected := *state
	if trial := state.TrialPlan; trial != nil {
		cycle := trial.Cycle
		if cycle == "" {
			cycle = "MONTHLY"
		}
		// Tier and cycle stay as granted; the rest is the current offer.
		if catalogPlan, ok := FindPlatformPlanCycle(trial.Tier, cycle); ok {
			catalogPlan.Tier = trial.Tier
			catalogPlan.Cycle = trial.Cycle
			projected.TrialPlan = &catalogPlan
		}
	}
	projected.Plan = planWithCatalogFeatures(state.Plan)
	return &projected
}

func (s *PlatformSubscriptionService) Select(ctx context.Context, tenant, tier, cycle string) (*ps.State, error) {
	if !s.Enabled || s.Provider == nil {
		return nil, ps.ErrUnavailable
	}
	plan, ok := FindPlatformPlanCycle(tier, cycle)
	if !ok || plan.ContactOnly {
		return nil, ps.ErrInvalidPlan
	}
	usage, err := s.Repo.Usage(ctx, tenant)
	if err != nil {
		return nil, err
	}
	if usage.Leads > plan.Limits.Leads || usage.Properties > plan.Limits.Properties || usage.Users > plan.Limits.Users {
		return nil, fmt.Errorf("%w: uso acima dos limites do plano", ps.ErrConflict)
	}
	product, err := s.Provider.EnsureProduct(ctx, plan)
	if err != nil {
		return nil, err
	}
	fresh := false
	// The requested plan is the catalogue offer; the checkout carries the free
	// period this tenant is actually entitled to.
	checkoutPlan := plan
	// Commit the operation reference before contacting the provider, so a lost
	// response can be recovered by externalId without creating a second checkout.
	adjusted := false
	state, err := s.Repo.Update(ctx, tenant, func(txCtx context.Context, x *ps.State) error {
		if x.SubscriptionID != "" && (x.Status == "ACTIVE" || x.Status == "TRIALING" || x.Status == "PAST_DUE") {
			return nil
		}
		// Returning before the paid period ends is welcome: the checkout defers
		// the first charge to its end, so the customer never pays twice for the
		// same days. Only a period too long to defer has to wait.
		if x.PaidThrough != nil && x.PaidThrough.After(time.Now()) && x.Status == "CANCELLED" {
			if owed := remainingTrialDays(x.PaidThrough, time.Now()); owed > maxDeferralDays {
				return fmt.Errorf("%w: faltam %d dias do período já pago; a nova contratação fica disponível em %s", ps.ErrConflict,
					owed, x.PaidThrough.Format("02/01/2006"))
			}
		}
		if x.Checkout != nil {
			if x.Checkout.Plan.Tier == tier && x.Checkout.Plan.Cycle == cycle {
				// Mesmo plano: reusa o checkout apenas se o provedor ainda
				// aceita o link. Um checkout expirado/cancelado ou removido do
				// provedor não pode voltar para a tela: "Continuar pagamento"
				// abriria uma página morta e o cliente ficaria sem saída. Ao
				// tratar o checkout como ausente, o caminho normal abaixo
				// recalcula o período de teste/pago restante com
				// nextCheckoutPlan — recriar com o plano antigo poderia
				// conceder de novo os dias grátis.
				usable, err := s.checkoutUsable(txCtx, x.Checkout)
				if err != nil {
					return err
				}
				if usable {
					return nil
				}
				x.Checkout = nil
			} else {
				// Trocar de plano com um checkout em aberto era um beco sem saída: a
				// tela mandava "encerre o checkout pendente" e o cliente ficava preso
				// (em trial, o clique seguinte devolvia 409 sem dizer o motivo). Quem
				// muda de ideia quer o outro plano: o checkout antigo é substituído,
				// nunca cobrado duas vezes.
				if err := s.discardCheckout(txCtx, *x.Checkout); err != nil {
					return err
				}
			}
		}
		// Decide the free period before clearing anything: what the customer
		// already owns is exactly what this checkout must respect.
		// PaidThrough and TrialEndsAt also stay, so the days already paid for
		// keep showing while the new subscription waits to start charging.
		checkoutPlan = nextCheckoutPlan(x, plan, time.Now())
		x.SubscriptionID = ""
		x.Plan = nil
		x.LastEventAt = nil
		x.CancelledAt = nil
		x.CancellationReason = ""
		adjusted = checkoutPlan.TrialDays != plan.TrialDays
		x.Checkout = &ps.Checkout{ExternalID: "suitee-" + uuid.NewString(), Plan: checkoutPlan, ProductID: product, CreatedAt: time.Now().UTC()}
		x.Status = "PENDING"
		x.DevMode = s.DevMode
		fresh = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if adjusted {
		// A different free period is a different product at the provider.
		if state.Checkout != nil {
			if product, err = s.Provider.EnsureProduct(ctx, state.Checkout.Plan); err != nil {
				return nil, err
			}
			if state, err = s.Repo.Update(ctx, tenant, func(_ context.Context, x *ps.State) error {
				if x.Checkout == nil || x.Checkout.ID != "" {
					return nil
				}
				x.Checkout.ProductID = product
				return nil
			}); err != nil {
				return nil, err
			}
		}
	}
	if state.SubscriptionID != "" && (state.Status == "ACTIVE" || state.Status == "TRIALING" || state.Status == "PAST_DUE") {
		_, reserveErr := s.Repo.Update(ctx, tenant, func(txCtx context.Context, x *ps.State) error {
			usage, err := s.Repo.Usage(txCtx, tenant)
			if err != nil {
				return err
			}
			if usage.Leads > plan.Limits.Leads || usage.Properties > plan.Limits.Properties || usage.Users > plan.Limits.Users {
				return fmt.Errorf("%w: uso acima dos limites do plano", ps.ErrConflict)
			}
			if x.Status != "ACTIVE" && x.Status != "TRIALING" {
				return fmt.Errorf("%w: regularize o pagamento antes de alterar o plano", ps.ErrConflict)
			}
			if x.Plan != nil && x.Plan.Tier == tier && x.Plan.Cycle == cycle {
				// The requested plan is the one in force: this is the way back
				// from a scheduled change, not a change onto itself.
				return nil
			}
			x.PendingChange = &ps.Change{Plan: plan, ProductID: product, RequestedAt: time.Now().UTC()}
			return nil
		})
		if reserveErr != nil {
			return nil, reserveErr
		}
		return s.Repo.Update(ctx, tenant, func(txCtx context.Context, x *ps.State) error {
			if x.PendingChange == nil {
				if x.Plan != nil && x.Plan.Tier == tier && x.Plan.Cycle == cycle {
					// Nothing scheduled and the plan is already the current one.
					return nil
				}
				return ps.ErrConflict
			}
			if (x.Status != "ACTIVE" && x.Status != "TRIALING") || x.PendingChange.Plan.Tier != tier || x.PendingChange.Plan.Cycle != cycle {
				return ps.ErrConflict
			}
			id, err := s.Provider.ChangePlan(txCtx, x.SubscriptionID, product)
			if err != nil {
				return err
			}
			x.PendingChange = &ps.Change{ID: id, Plan: plan, ProductID: product, RequestedAt: time.Now().UTC()}
			return nil
		})
	}
	return s.Repo.Update(ctx, tenant, func(txCtx context.Context, x *ps.State) error {
		if x.Checkout == nil || x.Checkout.ExternalID != state.Checkout.ExternalID {
			return ps.ErrConflict
		}
		return s.createCheckout(txCtx, x, product, plan, fresh)
	})
}

// createCheckout materializes the local reference at the provider. Callers hold
// the tenant lock, so a concurrent retry cannot create a second checkout.
func (s *PlatformSubscriptionService) createCheckout(txCtx context.Context, x *ps.State, product string, plan ps.Plan, fresh bool) error {
	if x.Checkout == nil {
		return ps.ErrConflict
	}
	if x.Checkout.ID != "" {
		return nil
	}
	remote, err := s.Provider.FindCheckout(txCtx, x.Checkout.ExternalID)
	if errors.Is(err, ps.ErrNotFound) {
		// The provider never received the operation, or removed the checkout.
		// Recreating at once could race an in-flight create, so a stale intent
		// must age out first — the same rule cancellation uses to clear it.
		if !fresh && time.Since(x.Checkout.CreatedAt) < time.Minute {
			return fmt.Errorf("%w: checkout em processamento; atualize para recuperar a confirmação", ps.ErrUnavailable)
		}
		x.Checkout.ProductID = product
		remote, err = s.Provider.CreateCheckout(txCtx, *x.Checkout, s.ReturnURL)
	}
	if err != nil {
		return err
	}
	if remote.ID == "" || remote.ExternalID != x.Checkout.ExternalID || remote.Amount != plan.AmountCents || remote.DevMode != s.DevMode || !providers.SafeAbacateURL(remote.URL) || len(remote.Items) != 1 || remote.Items[0].Quantity != 1 {
		return ps.ErrConflict
	}
	x.Checkout.ID = remote.ID
	x.Checkout.URL = remote.URL
	x.Checkout.ProductID = remote.Items[0].ID
	return nil
}

// checkoutPaid reports a checkout that already produced a payment. It must not
// be replaced while the webhook that confirms the subscription is in flight.
func checkoutPaid(status string) bool {
	return status == "PAID" || status == "COMPLETED"
}

// checkoutGone reports a checkout the customer can no longer pay. Unknown
// statuses stay reusable: deleting a live checkout by accident is worse than
// revisiting a link the provider may still accept.
func checkoutGone(status string) bool {
	switch status {
	case "EXPIRED", "CANCELLED", "CANCELED", "FAILED", "DECLINED":
		return true
	default:
		return false
	}
}

// checkoutUsable validates a checkout before the screen reuses its URL. The
// local checkout is only an intent; the provider is the authority on whether
// the link still exists. A false result with nil error means the checkout is
// stale and must be rebuilt from the current state — including the remaining
// trial/paid days — instead of being handed back to the customer.
func (s *PlatformSubscriptionService) checkoutUsable(ctx context.Context, c *ps.Checkout) (bool, error) {
	if c == nil || c.ID == "" {
		// createCheckout recovers a checkout whose create response was lost by
		// externalId; it already has the age guard for that case.
		return true, nil
	}
	remote, err := s.Provider.FindCheckout(ctx, c.ExternalID)
	if errors.Is(err, ps.ErrNotFound) {
		if time.Since(c.CreatedAt) <= time.Minute {
			// The create may still be in flight. Rebuilding now could charge twice.
			return false, ps.ErrUnavailable
		}
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if remote == nil {
		return false, ps.ErrConflict
	}
	if checkoutPaid(remote.Status) {
		return false, fmt.Errorf("%w: o pagamento do checkout do plano %s já foi aprovado; atualize para confirmar a assinatura", ps.ErrConflict, c.Plan.Name)
	}
	if checkoutGone(remote.Status) {
		if time.Since(c.CreatedAt) <= time.Minute {
			return false, ps.ErrUnavailable
		}
		return false, nil
	}
	if remote.ID == "" || remote.ExternalID != c.ExternalID || remote.DevMode != s.DevMode ||
		remote.Amount != c.Plan.AmountCents || !providers.SafeAbacateURL(remote.URL) ||
		len(remote.Items) != 1 || remote.Items[0].Quantity != 1 {
		return false, ps.ErrConflict
	}
	c.ID = remote.ID
	c.URL = remote.URL
	if remote.Items[0].ID != "" {
		c.ProductID = remote.Items[0].ID
	}
	return true, nil
}

// CancellationReasons are the reasons an administrator can record. The
// provider cancels immediately; the reason exists so retention can be measured.
// discardCheckout remove no provedor o checkout que será substituído por outro
// plano. Um checkout já PAGO não pode ser descartado: o pagamento está em
// processamento e a assinatura dele precisa ser confirmada antes de qualquer
// troca — nesse caso a operação segue conflitante, mas com motivo explícito.
func (s *PlatformSubscriptionService) discardCheckout(ctx context.Context, atual ps.Checkout) error {
	remote, err := s.Provider.FindCheckout(ctx, atual.ExternalID)
	if errors.Is(err, ps.ErrNotFound) {
		// O provedor nunca recebeu (ou já removeu) o checkout: nada a descartar.
		return nil
	}
	if err != nil {
		return err
	}
	if checkoutGone(remote.Status) {
		// Link expirado/cancelado: não há cobrança para remover.
		return nil
	}
	id := remote.ID
	if id == "" {
		id = atual.ID
	}
	if checkoutPaid(remote.Status) {
		return fmt.Errorf("%w: o checkout do plano %s já foi pago; aguarde a confirmação para trocar de plano", ps.ErrConflict, atual.Plan.Name)
	}
	if id == "" {
		return nil
	}
	return s.Provider.DeleteCheckout(ctx, id)
}

var CancellationReasons = []string{"requested_by_admin", "too_expensive", "missing_features", "not_using", "switching", "other"}

func ValidCancellationReason(reason string) bool {
	for _, allowed := range CancellationReasons {
		if reason == allowed {
			return true
		}
	}
	return false
}

// RevertChange removes a scheduled change: the provider replaces the pending
// update with the plan that is already in force, so the next renewal charges
// what the customer has today.
func (s *PlatformSubscriptionService) RevertChange(ctx context.Context, tenant string) (*ps.State, error) {
	if !s.Enabled || s.Provider == nil {
		return nil, ps.ErrUnavailable
	}
	return s.Repo.Update(ctx, tenant, func(txCtx context.Context, x *ps.State) error {
		if x.PendingChange == nil {
			return fmt.Errorf("%w: não há alteração agendada", ps.ErrConflict)
		}
		if x.SubscriptionID == "" || x.Plan == nil {
			return fmt.Errorf("%w: assinatura sem plano vigente", ps.ErrConflict)
		}
		product, err := s.Provider.EnsureProduct(txCtx, *x.Plan)
		if err != nil {
			return err
		}
		if _, err = s.Provider.ChangePlan(txCtx, x.SubscriptionID, product); err != nil {
			return err
		}
		x.PendingChange = nil
		return nil
	})
}

// ChangePaymentMethod restarts the subscription on a new card. The provider
// tokenizes the card at checkout and offers no API to replace it, so the only
// supported path is a new checkout — allowed while no paid period is being
// consumed, which is also when nothing would be charged twice.
func (s *PlatformSubscriptionService) ChangePaymentMethod(ctx context.Context, tenant string) (*ps.State, error) {
	if !s.Enabled || s.Provider == nil {
		return nil, ps.ErrUnavailable
	}
	return s.Repo.Update(ctx, tenant, func(txCtx context.Context, x *ps.State) error {
		if x.Plan == nil {
			return fmt.Errorf("%w: contrate um plano antes de cadastrar o cartão", ps.ErrConflict)
		}
		// A long period already paid (an annual plan, for instance) cannot be
		// deferred by the provider, so the change has to wait for it.
		if owed := remainingTrialDays(x.PaidThrough, time.Now()); owed > maxDeferralDays {
			return fmt.Errorf("%w: faltam %d dias do período já pago; a troca fica disponível em %s", ps.ErrConflict,
				owed, x.PaidThrough.Format("02/01/2006"))
		}
		if x.SubscriptionID != "" {
			if err := s.Provider.Cancel(txCtx, x.SubscriptionID); err != nil {
				return err
			}
		}
		// The offer is the catalogue plan, but the free period is whatever is
		// left of the trial this customer already owns.
		plan := nextCheckoutPlan(x, *x.Plan, time.Now())
		product, err := s.Provider.EnsureProduct(txCtx, plan)
		if err != nil {
			return err
		}
		x.SubscriptionID = ""
		x.PendingChange = nil
		x.PaymentMethod = nil
		// PaidThrough stays: the customer keeps using what was already paid for
		// while the new subscription waits to start charging.
		x.CancelledAt = nil
		x.CancellationReason = ""
		x.LastEventAt = nil
		x.Plan = nil
		x.Checkout = &ps.Checkout{ExternalID: "suitee-" + uuid.NewString(), Plan: plan, ProductID: product, CreatedAt: time.Now().UTC()}
		x.Status = "PENDING"
		x.DevMode = s.DevMode
		return nil
	})
}

// ChangePaymentMethodAndCheckout restarts the subscription on a new card and
// hands back a checkout the customer can pay with it.
func (s *PlatformSubscriptionService) ChangePaymentMethodAndCheckout(ctx context.Context, tenant string) (*ps.State, error) {
	if _, err := s.ChangePaymentMethod(ctx, tenant); err != nil {
		return nil, err
	}
	state, err := s.Repo.Get(ctx, tenant)
	if err != nil {
		return nil, err
	}
	if state.Checkout == nil || state.Checkout.Plan.Tier == "" {
		return nil, ps.ErrConflict
	}
	plan := state.Checkout.Plan
	return s.Repo.Update(ctx, tenant, func(txCtx context.Context, x *ps.State) error {
		if x.Checkout == nil || x.Checkout.ExternalID != state.Checkout.ExternalID {
			return ps.ErrConflict
		}
		return s.createCheckout(txCtx, x, x.Checkout.ProductID, plan, true)
	})
}

func (s *PlatformSubscriptionService) Cancel(ctx context.Context, tenant, reason string) (*ps.State, error) {
	if !s.Enabled || s.Provider == nil {
		return nil, ps.ErrUnavailable
	}
	if reason == "" {
		reason = "requested_by_admin"
	}
	if !ValidCancellationReason(reason) {
		return nil, ps.ErrInvalidPlan
	}
	return s.Repo.Update(ctx, tenant, func(txCtx context.Context, x *ps.State) error {
		if x.Status == "CANCELLED" {
			return nil
		}
		if x.SubscriptionID != "" {
			if err := s.Provider.Cancel(txCtx, x.SubscriptionID); err != nil {
				return err
			}
			now := time.Now().UTC()
			x.Status = "CANCELLED"
			x.CancelledAt = &now
			x.CancellationReason = reason
			x.LastEventAt = &now
			x.PendingChange = nil
			return nil
		}
		if x.Checkout != nil {
			// Recover the id after a lost create response before deleting remotely.
			remote, err := s.Provider.FindCheckout(txCtx, x.Checkout.ExternalID)
			if errors.Is(err, ps.ErrNotFound) {
				if time.Since(x.Checkout.CreatedAt) <= time.Minute {
					// The create may still be in flight; do not discard it yet.
					return ps.ErrUnavailable
				}
				// The provider no longer knows this checkout. Keeping the local
				// reference used to return 409 forever when the link expired
				// (typically a trial checkout opened days earlier), leaving the
				// customer unable to leave the pending-payment panel. There is
				// nothing to delete remotely: just clear the local intent.
				x.Checkout = nil
				x.Status = "NONE"
				return nil
			}
			if err != nil {
				return err
			}
			if checkoutPaid(remote.Status) {
				return fmt.Errorf("%w: o checkout do plano %s já foi pago; aguarde a confirmação para encerrar o checkout", ps.ErrConflict, x.Checkout.Plan.Name)
			}
			if checkoutGone(remote.Status) {
				if time.Since(x.Checkout.CreatedAt) <= time.Minute {
					return ps.ErrUnavailable
				}
				x.Checkout = nil
				x.Status = "NONE"
				return nil
			}
			id := remote.ID
			if id == "" {
				id = x.Checkout.ID
			}
			if id != "" {
				if err := s.Provider.DeleteCheckout(txCtx, id); err != nil {
					return err
				}
			}
			x.Checkout = nil
			x.Status = "NONE"
			return nil
		}
		return ps.ErrConflict
	})
}
func (s *PlatformSubscriptionService) HandleEvent(ctx context.Context, e ps.Event) error {
	if !s.Enabled || e.DevMode != s.DevMode || e.APIVersion != 2 || e.ID == "" {
		return ps.ErrConflict
	}
	switch e.Event {
	case "subscription.completed", "subscription.renewed", "subscription.trial_started",
		"subscription.cancelled", "subscription.payment_failed":
	default:
		return nil
	}
	sub := e.Data.Subscription
	if sub.ID == "" || sub.UpdatedAt.IsZero() {
		return ps.ErrConflict
	}
	checkoutID := e.Data.Checkout.ID
	if checkoutID == "" {
		checkoutID = sub.CheckoutID
	}
	tenant, err := s.Repo.ResolveTenant(ctx, sub.ID, checkoutID, e.Data.Checkout.ExternalID)
	if err != nil {
		return err
	}
	_, err = s.Repo.Update(ctx, tenant, func(txCtx context.Context, x *ps.State) error {
		if x.DevMode != s.DevMode {
			return ps.ErrConflict
		}
		added, err := s.Repo.RememberEvent(txCtx, tenant, e.ID, e.Event)
		if err != nil || !added {
			return err
		}
		if x.SubscriptionID != "" && x.SubscriptionID != sub.ID {
			return ps.ErrConflict
		}
		// Initial checkout may be paid again by a different customer. Only one
		// provider subscription can own this tenant; extra payments are rejected.
		if x.SubscriptionID == "" {
			if x.Checkout == nil || !(x.Checkout.ID == checkoutID && checkoutID != "" || x.Checkout.ExternalID == e.Data.Checkout.ExternalID && e.Data.Checkout.ExternalID != "") {
				return ps.ErrConflict
			}
			x.SubscriptionID = sub.ID
			plan := x.Checkout.Plan
			x.Plan = &plan
			// From this point the provider-backed plan carries the same remaining
			// trial window; the no-card first-access grant has been converted.
			x.TrialPlan = nil
		}
		// The card used at checkout is what renewals will charge; the event is
		// the only place the provider tells us which one it was.
		if info := e.Data.PayerInformation; info.Method != "" || info.CARD.Brand != "" || info.CARD.Number != "" {
			method := &ps.PaymentMethod{Kind: info.Method, Brand: info.CARD.Brand, Last4: info.CARD.Number, UpdatedAt: time.Now().UTC()}
			if method.Kind == "" {
				method.Kind = "CARD"
			}
			x.PaymentMethod = method
		}
		paid := e.Event == "subscription.completed" || e.Event == "subscription.renewed"
		if paid {
			payment := e.Data.Payment
			if payment.ID == "" || !paidPaymentStatus(payment.Status) || payment.PaidAmount <= 0 || payment.PaidAmount != payment.Amount || payment.UpdatedAt.IsZero() {
				return ps.ErrConflict
			}
			receipt := payment.ReceiptURL
			// The receipt is a display link. An unexpected host must not block a
			// confirmed payment from activating the subscription; the UI already
			// refuses to render links outside the provider.
			if receipt != "" && !providers.SafeAbacateURL(receipt) {
				receipt = ""
			}
			paymentPlan := x.Plan
			if x.PendingChange != nil && sub.Amount == x.PendingChange.Plan.AmountCents && sub.Frequency == x.PendingChange.Plan.Cycle {
				paymentPlan = &x.PendingChange.Plan
			}
			// A delayed renewal may refer to the previous catalog tier after a
			// newer plan payment was already applied. Persist its real payment,
			// while the chronological guard below preserves the current plan.
			if paymentPlan == nil || sub.Amount != paymentPlan.AmountCents || sub.Frequency != paymentPlan.Cycle {
				if historical, ok := platformPlanForBillingAmount(sub.Amount, sub.Frequency); ok {
					paymentPlan = &historical
				}
			}
			if paymentPlan == nil || sub.Amount != paymentPlan.AmountCents || sub.Frequency != paymentPlan.Cycle || payment.Amount != paymentPlan.AmountCents {
				return ps.ErrConflict
			}
			end := CycleEnd(payment.UpdatedAt, paymentPlan.Cycle)
			if x.PaidThrough == nil || end.After(*x.PaidThrough) {
				x.PaidThrough = &end
			}
			if err = s.Repo.SavePayment(txCtx, tenant, ps.Payment{ID: payment.ID, SubscriptionID: sub.ID, AmountCents: payment.PaidAmount, PaidAt: payment.UpdatedAt, ReceiptURL: receipt}); err != nil {
				return err
			}
		}
		// Delayed deliveries may add history, but cannot roll status or plan back.
		if x.LastEventAt != nil && !sub.UpdatedAt.After(*x.LastEventAt) && e.Event != "subscription.cancelled" {
			return nil
		}
		if x.Status == "CANCELLED" && e.Event != "subscription.cancelled" {
			return nil
		}
		switch e.Event {
		case "subscription.trial_started":
			// No charge happens at trial start, and the event payload carries
			// neither the trial length nor its end date — the subscription
			// record does, so the provider stays the authority on the offer.
			remote, remoteErr := s.Provider.GetSubscription(txCtx, sub.ID)
			if remoteErr != nil {
				return remoteErr
			}
			if remote == nil || remote.DevMode != s.DevMode || remote.Status == "CANCELLED" ||
				remote.TrialEndsAt == nil || remote.TrialEndsAt.IsZero() {
				return ps.ErrConflict
			}
			plan := x.Plan
			if plan == nil {
				if candidate, ok := platformPlanForBillingAmount(sub.Amount, sub.Frequency); ok {
					plan = &candidate
				}
			}
			if plan == nil || sub.Amount != plan.AmountCents || sub.Frequency != plan.Cycle || remote.Amount != plan.AmountCents {
				return ps.ErrConflict
			}
			x.Plan = plan
			x.TrialPlan = nil
			x.Status = "TRIALING"
			x.TrialEndsAt = remote.TrialEndsAt
			if x.TrialGrantedAt == nil {
				granted := time.Now().UTC()
				x.TrialGrantedAt = &granted
			}
			x.Checkout = nil
		case "subscription.cancelled":
			x.Status = "CANCELLED"
			x.CancelledAt = sub.CancelledAt
			// The provider reports no reason for an admin-initiated cancel;
			// keep the local one instead of erasing it with an empty string.
			if sub.CancelledDueTo != "" || x.CancellationReason == "" {
				x.CancellationReason = sub.CancelledDueTo
			}
			x.PendingChange = nil
		case "subscription.payment_failed":
			x.Status = "PAST_DUE"
		default:
			if sub.Status != "ACTIVE" {
				return ps.ErrConflict
			}
			plan := x.Plan
			if x.PendingChange != nil && sub.Amount == x.PendingChange.Plan.AmountCents && sub.Frequency == x.PendingChange.Plan.Cycle {
				p := x.PendingChange.Plan
				plan = &p
				x.PendingChange = nil
			}
			if plan == nil || sub.Amount != plan.AmountCents || sub.Frequency != plan.Cycle {
				return ps.ErrConflict
			}
			x.Plan = plan
			x.Status = "ACTIVE"
			x.Checkout = nil
			end := CycleEnd(e.Data.Payment.UpdatedAt, plan.Cycle)
			if x.PaidThrough == nil || end.After(*x.PaidThrough) {
				x.PaidThrough = &end
			}
		}
		if x.LastEventAt == nil || sub.UpdatedAt.After(*x.LastEventAt) {
			x.LastEventAt = &sub.UpdatedAt
		}
		return nil
	})
	return err
}

// paidPaymentStatus accepts the statuses the provider uses for a captured
// charge. The published v2 example shows PAID, but the real payload reports
// APPROVED — reading only one of them drops confirmed payments.
func paidPaymentStatus(status string) bool {
	return status == "APPROVED" || status == "PAID"
}

// trialAlreadyGranted reports whether this tenant ever received the free
// period. A subscription created before this rule existed is treated as
// already served: existing customers are not new customers.
func trialAlreadyGranted(x *ps.State) bool {
	return x.TrialGrantedAt != nil || x.TrialEndsAt != nil || x.SubscriptionID != "" || x.Status == "TRIALING"
}

// remainingTrialDays preserves what is left of a running trial. Whole days are
// enough because the provider closes the trial at the end of the last day.
func remainingTrialDays(until *time.Time, now time.Time) int {
	if until == nil || !until.After(now) {
		return 0
	}
	days := int(until.Sub(now).Hours() / 24)
	if days < 1 {
		return 1
	}
	return days
}

// maxDeferralDays is the longest free period the provider accepts on a product
// (its `trialDays` range is 1..90). It bounds how long a new subscription can
// wait before charging when a customer changes the card mid-period.
const maxDeferralDays = 90

// deferredDays is how long the next subscription must wait before its first
// charge. A customer keeps what they already own — the rest of a running trial
// or the period already paid — so nothing is charged twice.
func deferredDays(x *ps.State, now time.Time) int {
	days := remainingTrialDays(x.TrialEndsAt, now)
	if paid := remainingTrialDays(x.PaidThrough, now); paid > days {
		days = paid
	}
	if days > maxDeferralDays {
		days = maxDeferralDays
	}
	return days
}

// nextCheckoutPlan adjusts the catalogue offer to what this tenant still owns:
// the full trial on the very first checkout, the deferral above afterwards, and
// nothing once the free period was used and nothing is paid ahead.
func nextCheckoutPlan(x *ps.State, plan ps.Plan, now time.Time) ps.Plan {
	if plan.TrialDays <= 0 || !trialAlreadyGranted(x) {
		return plan
	}
	plan.TrialDays = deferredDays(x, now)
	return plan
}

// AddDate normalizes Jan 31 into March; clamp to the target month's last day.
func CycleEnd(start time.Time, cycle string) time.Time {
	months := 1
	if cycle == "ANNUALLY" {
		months = 12
	}
	first := time.Date(start.Year(), start.Month()+time.Month(months), 1, start.Hour(), start.Minute(), start.Second(), start.Nanosecond(), start.Location())
	last := first.AddDate(0, 1, -1).Day()
	day := start.Day()
	if day > last {
		day = last
	}
	return first.AddDate(0, 0, day-1)
}

// Refresh recovers a checkout whose response was lost, without creating a charge.
func (s *PlatformSubscriptionService) Refresh(ctx context.Context, tenant string) (*ps.State, error) {
	x, err := s.Repo.Get(ctx, tenant)
	if err != nil {
		return nil, err
	}
	if !s.Enabled || x.Checkout == nil || x.Checkout.ID != "" {
		return CatalogOffer(x), nil
	}
	remote, err := s.Provider.FindCheckout(ctx, x.Checkout.ExternalID)
	if errors.Is(err, ps.ErrNotFound) {
		return CatalogOffer(x), nil
	}
	if err != nil {
		return nil, err
	}
	if remote.ID == "" || remote.Amount != x.Checkout.Plan.AmountCents || remote.DevMode != s.DevMode || remote.ExternalID != x.Checkout.ExternalID || !providers.SafeAbacateURL(remote.URL) || len(remote.Items) != 1 || remote.Items[0].Quantity != 1 {
		return nil, ps.ErrConflict
	}
	updated, err := s.Repo.Update(ctx, tenant, func(_ context.Context, current *ps.State) error {
		if current.Checkout != nil && current.Checkout.ExternalID == x.Checkout.ExternalID {
			current.Checkout.ID = remote.ID
			current.Checkout.URL = remote.URL
			current.Checkout.ProductID = remote.Items[0].ID
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return CatalogOffer(updated), nil
}
