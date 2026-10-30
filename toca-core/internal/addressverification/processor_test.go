package addressverification

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oginoino/toca-core/internal/domain/core/address"
)

type providerFunc func(context.Context, string) (CanonicalAddress, error)

func (fn providerFunc) Lookup(ctx context.Context, postalCode string) (CanonicalAddress, error) {
	return fn(ctx, postalCode)
}

func validAddress() address.Address {
	return address.Address{
		Street:       "Rua informada",
		Number:       "  123 ",
		Complement:   " Apto   4 ",
		Neighborhood: "Bairro informado",
		City:         "Cidade informada",
		State:        "São Paulo",
		StateCode:    "sp",
		PostalCode:   "01001-000",
		Country:      "Brazil",
		CountryCode:  "br",
		IsVerified:   true,
	}
}

func processorConfig(mode Mode) ProcessorConfig {
	return ProcessorConfig{
		Mode:            mode,
		Timeout:         50 * time.Millisecond,
		CacheTTL:        time.Hour,
		CacheMaxEntries: 10,
		OnFallback:      func(context.Context, error) {},
	}
}

func TestProcessorCanonicalizesVerifiedBrazilianAddress(t *testing.T) {
	provider := providerFunc(func(_ context.Context, postalCode string) (CanonicalAddress, error) {
		if postalCode != "01001000" {
			t.Fatalf("provider received CEP %q", postalCode)
		}
		return CanonicalAddress{
			PostalCode:   "01001000",
			Street:       "Praça da Sé",
			Neighborhood: "Sé",
			City:         "São Paulo",
			State:        "São Paulo",
			StateCode:    "SP",
		}, nil
	})
	processor, err := NewProcessor(provider, processorConfig(ModeStrict))
	if err != nil {
		t.Fatal(err)
	}
	value := validAddress()

	if err := processor.Process(context.Background(), &value); err != nil {
		t.Fatal(err)
	}

	if !value.IsVerified {
		t.Fatal("expected provider lookup to mark address verified")
	}
	if value.PostalCode != "01001000" || value.Street != "Praça da Sé" || value.City != "São Paulo" {
		t.Fatalf("unexpected canonical address: %+v", value)
	}
	if value.Number != "123" || value.Complement != "Apto 4" {
		t.Fatalf("property-specific fields were not preserved: %+v", value)
	}
	if value.Country != "Brasil" || value.CountryCode != "BR" {
		t.Fatalf("country was not normalized: %+v", value)
	}
	wantFormatted := "Praça da Sé, 123, Apto 4, Sé, São Paulo - SP, CEP 01001-000"
	if value.FormattedAddress != wantFormatted {
		t.Fatalf("formatted_address = %q, want %q", value.FormattedAddress, wantFormatted)
	}
}

func TestProcessorCachesSuccessfulLookupsUntilTTL(t *testing.T) {
	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	var calls atomic.Int32
	provider := providerFunc(func(context.Context, string) (CanonicalAddress, error) {
		calls.Add(1)
		return CanonicalAddress{
			PostalCode: "01001000", Street: "Praça da Sé", Neighborhood: "Sé",
			City: "São Paulo", State: "São Paulo", StateCode: "SP",
		}, nil
	})
	config := processorConfig(ModeStrict)
	config.Now = func() time.Time { return now }
	processor, err := NewProcessor(provider, config)
	if err != nil {
		t.Fatal(err)
	}

	first, second := validAddress(), validAddress()
	if err := processor.Process(context.Background(), &first); err != nil {
		t.Fatal(err)
	}
	if err := processor.Process(context.Background(), &second); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("provider calls = %d, want 1 cache hit", got)
	}

	now = now.Add(2 * time.Hour)
	third := validAddress()
	if err := processor.Process(context.Background(), &third); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("provider calls after expiry = %d, want 2", got)
	}
}

func TestProcessorCachesDefinitiveNotFound(t *testing.T) {
	var calls atomic.Int32
	provider := providerFunc(func(context.Context, string) (CanonicalAddress, error) {
		calls.Add(1)
		return CanonicalAddress{}, ErrPostalCodeNotFound
	})
	processor, err := NewProcessor(provider, processorConfig(ModeBestEffort))
	if err != nil {
		t.Fatal(err)
	}

	for range 2 {
		value := validAddress()
		err := processor.Process(context.Background(), &value)
		if !errors.Is(err, ErrPostalCodeNotFound) {
			t.Fatalf("error = %v, want ErrPostalCodeNotFound", err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("provider calls = %d, want 1 cached not-found", got)
	}
}

func TestProcessorBestEffortFallbackPersistsValidInputUnverified(t *testing.T) {
	providerErr := errors.New("upstream unavailable")
	provider := providerFunc(func(context.Context, string) (CanonicalAddress, error) {
		return CanonicalAddress{}, providerErr
	})
	var observed error
	config := processorConfig(ModeBestEffort)
	config.OnFallback = func(_ context.Context, err error) { observed = err }
	processor, err := NewProcessor(provider, config)
	if err != nil {
		t.Fatal(err)
	}
	value := validAddress()

	if err := processor.Process(context.Background(), &value); err != nil {
		t.Fatal(err)
	}
	if value.IsVerified {
		t.Fatal("best-effort fallback must not preserve a client-supplied verified flag")
	}
	if value.PostalCode != "01001000" || value.StateCode != "SP" || value.CountryCode != "BR" {
		t.Fatalf("fallback address was not normalized: %+v", value)
	}
	if !errors.Is(observed, providerErr) {
		t.Fatalf("fallback observer got %v, want provider error", observed)
	}
}

func TestProcessorBestEffortDoesNotPersistIncompleteFallback(t *testing.T) {
	provider := providerFunc(func(context.Context, string) (CanonicalAddress, error) {
		return CanonicalAddress{}, errors.New("upstream unavailable")
	})
	processor, err := NewProcessor(provider, processorConfig(ModeBestEffort))
	if err != nil {
		t.Fatal(err)
	}
	value := validAddress()
	value.Street = ""

	err = processor.Process(context.Background(), &value)
	if !errors.Is(err, ErrInvalidAddress) {
		t.Fatalf("error = %v, want ErrInvalidAddress", err)
	}
}

func TestProcessorStrictModeRejectsProviderTimeout(t *testing.T) {
	provider := providerFunc(func(ctx context.Context, _ string) (CanonicalAddress, error) {
		<-ctx.Done()
		return CanonicalAddress{}, ctx.Err()
	})
	config := processorConfig(ModeStrict)
	config.Timeout = time.Millisecond
	processor, err := NewProcessor(provider, config)
	if err != nil {
		t.Fatal(err)
	}
	value := validAddress()

	err = processor.Process(context.Background(), &value)
	if !errors.Is(err, ErrVerificationUnavailable) {
		t.Fatalf("error = %v, want ErrVerificationUnavailable", err)
	}
	if value.IsVerified {
		t.Fatal("timed-out lookup must remain unverified")
	}
}

func TestProcessorRejectsInvalidCEPBeforeProvider(t *testing.T) {
	var calls atomic.Int32
	provider := providerFunc(func(context.Context, string) (CanonicalAddress, error) {
		calls.Add(1)
		return CanonicalAddress{}, nil
	})
	processor, err := NewProcessor(provider, processorConfig(ModeBestEffort))
	if err != nil {
		t.Fatal(err)
	}
	value := validAddress()
	value.PostalCode = "01A01-000"

	err = processor.Process(context.Background(), &value)
	if !errors.Is(err, ErrInvalidPostalCode) {
		t.Fatalf("error = %v, want ErrInvalidPostalCode", err)
	}
	if calls.Load() != 0 {
		t.Fatal("provider must not be called for malformed CEP")
	}
}

func TestProcessorDoesNotSendInternationalAddressToViaCEP(t *testing.T) {
	var calls atomic.Int32
	provider := providerFunc(func(context.Context, string) (CanonicalAddress, error) {
		calls.Add(1)
		return CanonicalAddress{}, nil
	})
	processor, err := NewProcessor(provider, processorConfig(ModeStrict))
	if err != nil {
		t.Fatal(err)
	}
	value := validAddress()
	value.Country = "United States"
	value.CountryCode = "us"
	value.State = "California"
	value.StateCode = "ca"
	value.PostalCode = " 94105 "

	if err := processor.Process(context.Background(), &value); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("ViaCEP must not receive non-Brazilian addresses")
	}
	if value.IsVerified {
		t.Fatal("international address must stay unverified")
	}
	if value.PostalCode != "94105" {
		t.Fatalf("international postal code = %q, want preserved semantics", value.PostalCode)
	}
}

func TestProcessorDisabledStillNormalizesAndClearsVerified(t *testing.T) {
	processor, err := NewProcessor(nil, processorConfig(ModeDisabled))
	if err != nil {
		t.Fatal(err)
	}
	value := validAddress()

	if err := processor.Process(context.Background(), &value); err != nil {
		t.Fatal(err)
	}
	if value.IsVerified {
		t.Fatal("disabled mode must not trust a client-supplied verified flag")
	}
	if value.PostalCode != "01001000" || value.Number != "123" {
		t.Fatalf("disabled mode failed to normalize: %+v", value)
	}
}

func TestProcessorCacheCapacityIsBounded(t *testing.T) {
	var calls atomic.Int32
	provider := providerFunc(func(_ context.Context, postalCode string) (CanonicalAddress, error) {
		calls.Add(1)
		return CanonicalAddress{
			PostalCode: postalCode, Street: "Rua", Neighborhood: "Bairro",
			City: "Cidade", State: "São Paulo", StateCode: "SP",
		}, nil
	})
	config := processorConfig(ModeStrict)
	config.CacheMaxEntries = 1
	processor, err := NewProcessor(provider, config)
	if err != nil {
		t.Fatal(err)
	}

	for _, cep := range []string{"01001000", "01310100", "01001000"} {
		value := validAddress()
		value.PostalCode = cep
		if err := processor.Process(context.Background(), &value); err != nil {
			t.Fatal(err)
		}
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("provider calls = %d, want 3 after capacity eviction", got)
	}
}
