package platform_subscription

import (
	"context"
	"errors"
	"time"
)

var (
	ErrConflict    = errors.New("subscription operation conflicts with current state")
	ErrInvalidPlan = errors.New("invalid subscription plan")
	ErrUnavailable = errors.New("subscription provider unavailable")
	ErrNotFound    = errors.New("subscription not found")
	// ErrMethodUnavailable é a loja do provedor sem cobrança recorrente
	// habilitada (CARD/PIX Automático). Repetir não resolve: é configuração da
	// conta no provedor, então não pode virar 503 "tente de novo".
	ErrMethodUnavailable = errors.New("recurring payment method unavailable for this store")
)

type Plan struct {
	Tier        string `json:"tier"`
	Name        string `json:"name"`
	AmountCents int64  `json:"amount_cents"`
	Cycle       string `json:"cycle"`
	Description string `json:"description"`
	Limits      Limits `json:"limits"`
	ContactOnly bool   `json:"contact_only"`
	// TrialDays is the free period granted before the first charge.
	TrialDays int `json:"trial_days"`
	// Highlights are the benefits shown on the plan card.
	Highlights []string `json:"highlights,omitempty"`
	// Features are the server-enforced product capabilities included in this plan.
	Features []string `json:"features,omitempty"`
}
type Limits struct {
	Leads      int `json:"leads"`
	Properties int `json:"properties"`
	Users      int `json:"users"`
}
type Checkout struct {
	ExternalID string    `json:"external_id"`
	ID         string    `json:"id"`
	URL        string    `json:"url"`
	Plan       Plan      `json:"plan"`
	ProductID  string    `json:"product_id"`
	CreatedAt  time.Time `json:"created_at"`
}
type Change struct {
	ID          string    `json:"id"`
	Plan        Plan      `json:"plan"`
	ProductID   string    `json:"product_id"`
	RequestedAt time.Time `json:"requested_at"`
}
type State struct {
	Status         string     `json:"status"`
	Plan           *Plan      `json:"plan"`
	SubscriptionID string     `json:"subscription_id,omitempty"`
	Checkout       *Checkout  `json:"checkout,omitempty"`
	PendingChange  *Change    `json:"pending_change,omitempty"`
	PaidThrough    *time.Time `json:"paid_through,omitempty"`
	TrialEndsAt    *time.Time `json:"trial_ends_at,omitempty"`
	// TrialGrantedAt records that the free period was already given to this
	// tenant: it is granted once, never again per checkout.
	TrialGrantedAt *time.Time `json:"trial_granted_at,omitempty"`
	// TrialPlan is the no-card first-access trial offer. It remains separate
	// from Plan, which represents the provider-backed subscription.
	TrialPlan          *Plan          `json:"trial_plan,omitempty"`
	PaymentMethod      *PaymentMethod `json:"payment_method,omitempty"`
	LastEventAt        *time.Time     `json:"last_event_at,omitempty"`
	CancelledAt        *time.Time     `json:"cancelled_at,omitempty"`
	CancellationReason string         `json:"cancellation_reason,omitempty"`
	DevMode            bool           `json:"dev_mode"`
}

// Access is the entitlement decision consumed by the app and API gateway.
// Features is empty whenever the plan is not currently entitled.
type Access struct {
	ControlEnabled      bool       `json:"access_control_enabled"`
	SubscriptionEnabled bool       `json:"subscription_enabled"`
	Entitled            bool       `json:"entitled"`
	Status              string     `json:"status"`
	Plan                *Plan      `json:"plan"`
	Features            []string   `json:"features"`
	TrialEndsAt         *time.Time `json:"trial_ends_at,omitempty"`
	PaidThrough         *time.Time `json:"paid_through,omitempty"`
	Reason              string     `json:"reason,omitempty"`
}

// PaymentMethod is what the provider will charge on renewal. The card is
// tokenized at checkout and the provider exposes no API to replace it, so the
// reference is informational and comes from the payment event.
type PaymentMethod struct {
	Kind      string    `json:"kind"`
	Brand     string    `json:"brand,omitempty"`
	Last4     string    `json:"last4,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Payment struct {
	ID             string    `json:"id"`
	SubscriptionID string    `json:"subscription_id"`
	AmountCents    int64     `json:"amount_cents"`
	PaidAt         time.Time `json:"paid_at"`
	ReceiptURL     string    `json:"receipt_url,omitempty"`
}
type Event struct {
	ID         string `json:"id"`
	Event      string `json:"event"`
	APIVersion int    `json:"apiVersion"`
	DevMode    bool   `json:"devMode"`
	Data       struct {
		Subscription struct {
			ID             string     `json:"id"`
			CheckoutID     string     `json:"checkoutId"`
			Amount         int64      `json:"amount"`
			Status         string     `json:"status"`
			Frequency      string     `json:"frequency"`
			TrialDays      *int       `json:"trialDays"`
			TrialEndsAt    *time.Time `json:"trialEndsAt"`
			CreatedAt      time.Time  `json:"createdAt"`
			UpdatedAt      time.Time  `json:"updatedAt"`
			CancelledAt    *time.Time `json:"canceledAt"`
			CancelledDueTo string     `json:"cancelledDueTo"`
		} `json:"subscription"`
		Checkout struct {
			ID         string `json:"id"`
			ExternalID string `json:"externalId"`
			Items      []struct {
				ID string `json:"id"`
			} `json:"items"`
		} `json:"checkout"`
		PayerInformation struct {
			Method string `json:"method"`
			CARD   struct {
				Number string `json:"number"`
				Brand  string `json:"brand"`
			} `json:"CARD"`
		} `json:"payerInformation"`
		Payment struct {
			ID         string    `json:"id"`
			Amount     int64     `json:"amount"`
			PaidAmount int64     `json:"paidAmount"`
			Status     string    `json:"status"`
			ReceiptURL string    `json:"receiptUrl"`
			UpdatedAt  time.Time `json:"updatedAt"`
		} `json:"payment"`
	} `json:"data"`
}

// Update serializes one tenant with a transaction-scoped advisory lock.
// RememberEvent, SavePayment and state changes share that same transaction.
type Repository interface {
	Get(context.Context, string) (*State, error)
	Usage(context.Context, string) (Limits, error)
	Update(context.Context, string, func(context.Context, *State) error) (*State, error)
	ResolveTenant(context.Context, string, string, string) (string, error)
	RememberEvent(context.Context, string, string, string) (bool, error)
	SavePayment(context.Context, string, Payment) error
	Payments(context.Context, string) ([]Payment, error)
	Payment(context.Context, string, string) (*Payment, error)
	TenantIdentity(context.Context, string) (string, string, error)
}
type RemoteCheckout struct {
	ID         string `json:"id"`
	ExternalID string `json:"externalId"`
	URL        string `json:"url"`
	Amount     int64  `json:"amount"`
	Status     string `json:"status"`
	DevMode    bool   `json:"devMode"`
	Items      []struct {
		ID       string `json:"id"`
		Quantity int    `json:"quantity"`
	} `json:"items"`
}
type RemoteSubscription struct {
	ID          string     `json:"id"`
	CheckoutID  string     `json:"checkoutId"`
	Amount      int64      `json:"amount"`
	Status      string     `json:"status"`
	Frequency   string     `json:"frequency"`
	TrialDays   *int       `json:"trialDays"`
	TrialEndsAt *time.Time `json:"trialEndsAt"`
	DevMode     bool       `json:"devMode"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}
type Provider interface {
	EnsureProduct(context.Context, Plan) (string, error)
	FindCheckout(context.Context, string) (*RemoteCheckout, error)
	CreateCheckout(context.Context, Checkout, string) (*RemoteCheckout, error)
	DeleteCheckout(context.Context, string) error
	GetSubscription(context.Context, string) (*RemoteSubscription, error)
	Cancel(context.Context, string) error
	ChangePlan(context.Context, string, string) (string, error)
}
