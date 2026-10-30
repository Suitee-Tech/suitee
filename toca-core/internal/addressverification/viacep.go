package addressverification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const DefaultViaCEPBaseURL = "https://viacep.com.br"

// HTTPDoer makes the ViaCEP adapter deterministic in tests and replaceable by
// an instrumented/retrying client in production.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// ViaCEPProvider implements Provider using ViaCEP's JSON endpoint.
type ViaCEPProvider struct {
	baseURL string
	client  HTTPDoer
}

// NewViaCEPProvider returns a provider without performing any network call.
func NewViaCEPProvider(baseURL string, client HTTPDoer) (*ViaCEPProvider, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid ViaCEP base URL %q", baseURL)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("unsupported ViaCEP URL scheme %q", parsed.Scheme)
	}
	if client == nil {
		return nil, errors.New("ViaCEP HTTP client is required")
	}
	return &ViaCEPProvider{baseURL: baseURL, client: client}, nil
}

func (p *ViaCEPProvider) Lookup(ctx context.Context, postalCode string) (CanonicalAddress, error) {
	requestURL := p.baseURL + "/ws/" + url.PathEscape(postalCode) + "/json/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return CanonicalAddress{}, fmt.Errorf("building ViaCEP request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "suitee-toca-core/1.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return CanonicalAddress{}, fmt.Errorf("requesting ViaCEP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return CanonicalAddress{}, fmt.Errorf("ViaCEP returned HTTP %d", resp.StatusCode)
	}

	var payload struct {
		PostalCode   string `json:"cep"`
		Street       string `json:"logradouro"`
		Neighborhood string `json:"bairro"`
		City         string `json:"localidade"`
		State        string `json:"estado"`
		StateCode    string `json:"uf"`
		Error        bool   `json:"erro"`
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 1<<20))
	if err := decoder.Decode(&payload); err != nil {
		return CanonicalAddress{}, fmt.Errorf("decoding ViaCEP response: %w", err)
	}
	if payload.Error {
		return CanonicalAddress{}, fmt.Errorf("%w: %s", ErrPostalCodeNotFound, postalCode)
	}

	normalizedCEP, err := normalizePostalCode(payload.PostalCode)
	if err != nil || payload.City == "" || len(strings.TrimSpace(payload.StateCode)) != 2 {
		return CanonicalAddress{}, errors.New("ViaCEP returned an incomplete address")
	}
	if normalizedCEP != postalCode {
		return CanonicalAddress{}, errors.New("ViaCEP returned a different postal code")
	}
	stateCode := strings.ToUpper(normalizeText(payload.StateCode))
	state := normalizeText(payload.State)
	if state == "" {
		state = brazilianStateName(stateCode)
	}
	return CanonicalAddress{
		PostalCode:   normalizedCEP,
		Street:       normalizeText(payload.Street),
		Neighborhood: normalizeText(payload.Neighborhood),
		City:         normalizeText(payload.City),
		State:        state,
		StateCode:    stateCode,
	}, nil
}

func brazilianStateName(code string) string {
	return map[string]string{
		"AC": "Acre", "AL": "Alagoas", "AP": "Amapá", "AM": "Amazonas",
		"BA": "Bahia", "CE": "Ceará", "DF": "Distrito Federal", "ES": "Espírito Santo",
		"GO": "Goiás", "MA": "Maranhão", "MT": "Mato Grosso", "MS": "Mato Grosso do Sul",
		"MG": "Minas Gerais", "PA": "Pará", "PB": "Paraíba", "PR": "Paraná",
		"PE": "Pernambuco", "PI": "Piauí", "RJ": "Rio de Janeiro", "RN": "Rio Grande do Norte",
		"RS": "Rio Grande do Sul", "RO": "Rondônia", "RR": "Roraima", "SC": "Santa Catarina",
		"SP": "São Paulo", "SE": "Sergipe", "TO": "Tocantins",
	}[code]
}
