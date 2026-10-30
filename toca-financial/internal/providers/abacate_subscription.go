package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/logger"
	ps "github.com/oginoino/toca/toca-financial/internal/domain/platform_subscription"
)

type AbacateSubscriptionProvider struct {
	BaseURL string
	APIKey  string
	DevMode bool
	Client  *http.Client
}

func NewAbacateSubscriptionProvider(key string, dev bool) *AbacateSubscriptionProvider {
	return &AbacateSubscriptionProvider{BaseURL: "https://api.abacatepay.com/v2", APIKey: key, DevMode: dev, Client: &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (p *AbacateSubscriptionProvider) request(ctx context.Context, method, path string, body any, out any) error {
	if p.APIKey == "" {
		return ps.ErrUnavailable
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(p.BaseURL, "/")+path, reader)
	if err != nil {
		return ps.ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Suitee-Subscriptions/1.0")
	res, err := p.Client.Do(req)
	if err != nil {
		return ps.ErrUnavailable
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return ps.ErrUnavailable
	}
	var envelope struct {
		Data       json.RawMessage `json:"data"`
		Success    bool            `json:"success"`
		Error      json.RawMessage `json:"error"`
		Pagination struct {
			HasMore bool   `json:"hasMore"`
			Next    string `json:"next"`
		} `json:"pagination"`
	}
	decoded := json.Unmarshal(raw, &envelope) == nil
	// The v2 API answers a missing resource with HTTP 400 and a textual error
	// ("Billing not found", "Subscription not found"), not 404. Absence must be
	// read from the payload: treating it as an outage leaves the local checkout
	// reference stranded and turns a recoverable state into a 5xx.
	if res.StatusCode == http.StatusNotFound || (decoded && notFoundError(envelope.Error)) {
		return ps.ErrNotFound
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		// A causa do provedor ("CARD is not available for this store") ia para o
		// lixo: a tela dizia só "atualize a assinatura" e ninguém descobria que o
		// problema era a CONTA no provedor. Ela é registrada no log (para o
		// suporte) e não devolvida ao cliente — corpo de erro do provedor pode
		// ecoar credencial, e o teste de sanitização trava isso.
		if decoded {
			if message := providerMessage(envelope.Error); message != "" {
				logger.Warn("abacate.request_failed",
					"status", res.StatusCode, "request", method+" "+path, "provider_error", message)
			}
		}
		// Recusa de cobrança recorrente é configuração da conta, não
		// indisponibilidade: repetir não resolve, então merece erro próprio.
		if decoded && paymentMethodUnavailable(envelope.Error) {
			return ps.ErrMethodUnavailable
		}
		return fmt.Errorf("%w (HTTP %d)", ps.ErrUnavailable, res.StatusCode)
	}
	if !decoded || !envelope.Success {
		return ps.ErrUnavailable
	}
	if out != nil {
		if page, ok := out.(*abacateProductPage); ok {
			if json.Unmarshal(envelope.Data, &page.Items) != nil {
				return ps.ErrUnavailable
			}
			page.HasMore = envelope.Pagination.HasMore
			page.Next = envelope.Pagination.Next
			return nil
		}
		if err = json.Unmarshal(envelope.Data, out); err != nil {
			return ps.ErrUnavailable
		}
	}
	return nil
}

// providerMessage devolve a mensagem textual do provedor, limitada e sem
// quebras de linha, para entrar em log/telemetria sem virar despejo de corpo.
func providerMessage(raw json.RawMessage) string {
	var message string
	if json.Unmarshal(raw, &message) != nil {
		return ""
	}
	message = strings.Join(strings.Fields(message), " ")
	if len(message) > 200 {
		message = message[:200]
	}
	return message
}

// paymentMethodUnavailable reconhece a recusa de cobrança recorrente da loja
// ("CARD is not available for this store", "PIX Automático is not available for
// this store"): é configuração da conta no provedor, não indisponibilidade.
func paymentMethodUnavailable(raw json.RawMessage) bool {
	message := strings.ToLower(providerMessage(raw))
	if message == "" {
		return false
	}
	return strings.Contains(message, "not available for this store") ||
		strings.Contains(message, "payment method") && strings.Contains(message, "not available")
}

// notFoundError reads the provider's textual error without echoing it back.
func notFoundError(raw json.RawMessage) bool {
	var message string
	if json.Unmarshal(raw, &message) != nil {
		return false
	}
	return strings.Contains(strings.ToLower(message), "not found")
}

type abacateProduct struct {
	ID         string `json:"id"`
	ExternalID string `json:"externalId"`
	Price      int64  `json:"price"`
	Currency   string `json:"currency"`
	Cycle      string `json:"cycle"`
	Status     string `json:"status"`
	DevMode    bool   `json:"devMode"`
	TrialDays  *int   `json:"trialDays"`
}
type abacateProductPage struct {
	Items   []abacateProduct
	HasMore bool
	Next    string
}

func (p *AbacateSubscriptionProvider) EnsureProduct(ctx context.Context, plan ps.Plan) (string, error) {
	external := productExternalID(plan)
	// The paginated catalog lags behind newly created products, so a product
	// that exists can look missing and be created twice. The direct lookup by
	// externalId answers authoritatively and is tried first.
	found, err := p.productByExternalID(ctx, external)
	if err != nil {
		return "", err
	}
	if found == nil {
		if found, err = p.searchCatalog(ctx, external); err != nil {
			return "", err
		}
	}
	if found != nil {
		if found.Price != plan.AmountCents || found.Currency != "BRL" || found.Cycle != plan.Cycle ||
			found.Status != "ACTIVE" || found.DevMode != p.DevMode || trialDaysOf(*found) != plan.TrialDays {
			return "", ps.ErrConflict
		}
		return found.ID, nil
	}
	body := map[string]any{"externalId": external, "name": "Suitee " + plan.Name + " " + plan.Cycle, "price": plan.AmountCents, "currency": "BRL", "cycle": plan.Cycle}
	if plan.TrialDays > 0 {
		body["trialDays"] = plan.TrialDays
	}
	var created abacateProduct
	if err = p.request(ctx, http.MethodPost, "/products/create", body, &created); err != nil {
		return "", err
	}
	if created.ID == "" || created.Price != plan.AmountCents || created.Cycle != plan.Cycle || created.DevMode != p.DevMode || trialDaysOf(created) != plan.TrialDays {
		return "", ps.ErrConflict
	}
	return created.ID, nil
}

// productByExternalID returns nil when the provider has no such product.
func (p *AbacateSubscriptionProvider) productByExternalID(ctx context.Context, external string) (*abacateProduct, error) {
	var x abacateProduct
	if err := p.request(ctx, http.MethodGet, "/products/get?externalId="+url.QueryEscape(external), nil, &x); err != nil {
		if errors.Is(err, ps.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if x.ID == "" {
		return nil, nil
	}
	return &x, nil
}

func (p *AbacateSubscriptionProvider) searchCatalog(ctx context.Context, external string) (*abacateProduct, error) {
	cursor := ""
	seen := map[string]bool{}
	for {
		var page abacateProductPage
		path := "/products/list?limit=100&externalId=" + url.QueryEscape(external)
		if cursor != "" {
			path += "&after=" + url.QueryEscape(cursor)
		}
		if err := p.request(ctx, http.MethodGet, path, nil, &page); err != nil {
			return nil, err
		}
		for _, x := range page.Items {
			if x.ExternalID == external {
				found := x
				return &found, nil
			}
		}
		if !page.HasMore {
			return nil, nil
		}
		if page.Next == "" || seen[page.Next] {
			return nil, ps.ErrUnavailable
		}
		cursor = page.Next
		seen[cursor] = true
	}
}

// productExternalID identifies the offer, including the free period: a catalog
// entry without trial is a different product at the provider.
func productExternalID(plan ps.Plan) string {
	return fmt.Sprintf("suitee-%s-%s-%d-t%d", plan.Tier, strings.ToLower(plan.Cycle), plan.AmountCents, plan.TrialDays)
}

func trialDaysOf(x abacateProduct) int {
	if x.TrialDays == nil {
		return 0
	}
	return *x.TrialDays
}
func (p *AbacateSubscriptionProvider) FindCheckout(ctx context.Context, external string) (*ps.RemoteCheckout, error) {
	var x ps.RemoteCheckout
	err := p.request(ctx, http.MethodGet, "/subscriptions/checkouts/get?externalId="+url.QueryEscape(external), nil, &x)
	if err != nil {
		return nil, err
	}
	return &x, nil
}
func (p *AbacateSubscriptionProvider) CreateCheckout(ctx context.Context, c ps.Checkout, returnURL string) (*ps.RemoteCheckout, error) {
	var x ps.RemoteCheckout
	err := p.request(ctx, http.MethodPost, "/subscriptions/create", map[string]any{
		"items": []map[string]any{{"id": c.ProductID, "quantity": 1}}, "methods": []string{"CARD"}, "externalId": c.ExternalID,
		"returnUrl": returnURL, "completionUrl": returnURL + "?payment=returned", "retryPolicy": map[string]int{"maxRetry": 3, "retryEvery": 1},
	}, &x)
	if err != nil {
		return nil, err
	}
	return &x, nil
}
func (p *AbacateSubscriptionProvider) DeleteCheckout(ctx context.Context, id string) error {
	var deleted ps.RemoteCheckout
	// A checkout the provider no longer has is already in the desired state:
	// refusing here would strand the local reference forever.
	if err := p.request(ctx, http.MethodPost, "/subscriptions/checkouts/delete?id="+url.QueryEscape(id), nil, &deleted); err != nil {
		if errors.Is(err, ps.ErrNotFound) {
			return nil
		}
		return err
	}
	// Deleting a paid checkout does not cancel the subscription it generated.
	// Keep the local reference until the completed event arrives, then allow
	// cancellation through the subs_ id rather than hiding an active renewal.
	if deleted.Status == "PAID" {
		return ps.ErrConflict
	}
	return nil
}
func (p *AbacateSubscriptionProvider) GetSubscription(ctx context.Context, id string) (*ps.RemoteSubscription, error) {
	var x ps.RemoteSubscription
	err := p.request(ctx, http.MethodGet, "/subscriptions/get?id="+url.QueryEscape(id), nil, &x)
	if err != nil {
		return nil, err
	}
	return &x, nil
}
func (p *AbacateSubscriptionProvider) Cancel(ctx context.Context, id string) error {
	current, err := p.GetSubscription(ctx, id)
	if err != nil {
		return err
	}
	if current.DevMode != p.DevMode {
		return ps.ErrConflict
	}
	if current.Status == "CANCELLED" {
		return nil
	}
	var cancelled ps.RemoteSubscription
	if err := p.request(ctx, http.MethodPost, "/subscriptions/cancel", map[string]string{"id": id}, &cancelled); err != nil {
		return err
	}
	// The documented cancel response already carries the new status. Demanding
	// an immediately consistent re-read reported failure for a cancellation the
	// provider had accepted: its reads lag behind writes (the list endpoint
	// showed a cancelled subscription as active for a while).
	if cancelled.Status == "CANCELLED" {
		return nil
	}
	current, err = p.GetSubscription(ctx, id)
	if err != nil {
		return err
	}
	if current.Status != "CANCELLED" || current.DevMode != p.DevMode {
		return ps.ErrConflict
	}
	return nil
}
func (p *AbacateSubscriptionProvider) ChangePlan(ctx context.Context, id, product string) (string, error) {
	var x struct {
		ID             string `json:"id"`
		SubscriptionID string `json:"subscriptionId"`
		ProductID      string `json:"productId"`
		Status         string `json:"status"`
	}
	if err := p.request(ctx, http.MethodPost, "/subscriptions/change-plan", map[string]any{"id": id, "productId": product, "quantity": 1}, &x); err != nil {
		return "", err
	}
	if x.ID == "" || x.SubscriptionID != id || x.ProductID != product || x.Status != "PENDING" {
		return "", ps.ErrConflict
	}
	return x.ID, nil
}

// Provider links are rendered only when they remain on the hosted payment UI.
func SafeAbacateURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == "app.abacatepay.com" && u.User == nil
}
