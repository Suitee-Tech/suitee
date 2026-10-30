package addressverification

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func response(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestViaCEPProviderLookupMapsOfficialJSONWithoutNetwork(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://viacep.test/ws/01001000/json/" {
			t.Fatalf("request URL = %q", req.URL.String())
		}
		if req.Header.Get("Accept") != "application/json" {
			t.Fatalf("Accept = %q", req.Header.Get("Accept"))
		}
		return response(http.StatusOK, `{
			"cep":"01001-000","logradouro":"Praça da Sé","bairro":"Sé",
			"localidade":"São Paulo","uf":"SP"
		}`), nil
	})}
	provider, err := NewViaCEPProvider("https://viacep.test/", client)
	if err != nil {
		t.Fatal(err)
	}

	result, err := provider.Lookup(context.Background(), "01001000")
	if err != nil {
		t.Fatal(err)
	}
	if result.PostalCode != "01001000" || result.Street != "Praça da Sé" || result.State != "São Paulo" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestViaCEPProviderMapsErroTrueToNotFound(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(http.StatusOK, `{"erro":true}`), nil
	})}
	provider, err := NewViaCEPProvider("https://viacep.test", client)
	if err != nil {
		t.Fatal(err)
	}

	_, err = provider.Lookup(context.Background(), "99999999")
	if !errors.Is(err, ErrPostalCodeNotFound) {
		t.Fatalf("error = %v, want ErrPostalCodeNotFound", err)
	}
}

func TestViaCEPProviderSurfacesTransportAndInvalidResponses(t *testing.T) {
	tests := []struct {
		name      string
		transport roundTripFunc
	}{
		{
			name: "transport failure",
			transport: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("connection refused")
			},
		},
		{
			name: "non 200",
			transport: func(*http.Request) (*http.Response, error) {
				return response(http.StatusTooManyRequests, "rate limited"), nil
			},
		},
		{
			name: "malformed json",
			transport: func(*http.Request) (*http.Response, error) {
				return response(http.StatusOK, "{"), nil
			},
		},
		{
			name: "incomplete payload",
			transport: func(*http.Request) (*http.Response, error) {
				return response(http.StatusOK, `{"cep":"01001-000","localidade":"São Paulo"}`), nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewViaCEPProvider("https://viacep.test", &http.Client{Transport: tt.transport})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := provider.Lookup(context.Background(), "01001000"); err == nil {
				t.Fatal("expected provider error")
			}
		})
	}
}

func TestNewViaCEPProviderValidatesDependencies(t *testing.T) {
	if _, err := NewViaCEPProvider("not-a-url", http.DefaultClient); err == nil {
		t.Fatal("expected invalid URL error")
	}
	if _, err := NewViaCEPProvider(DefaultViaCEPBaseURL, nil); err == nil {
		t.Fatal("expected missing client error")
	}
}
