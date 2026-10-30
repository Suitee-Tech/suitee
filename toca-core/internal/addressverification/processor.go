// Package addressverification normalizes addresses and verifies Brazilian
// postal codes through a replaceable provider before a property is persisted.
package addressverification

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/oginoino/toca-core/internal/domain/core/address"
)

var (
	// ErrInvalidPostalCode identifies a CEP that cannot be normalized to eight digits.
	ErrInvalidPostalCode = errors.New("invalid postal code")
	// ErrPostalCodeNotFound identifies a syntactically valid CEP absent from the provider.
	ErrPostalCodeNotFound = errors.New("postal code not found")
	// ErrInvalidAddress identifies an address that remains incomplete after processing.
	ErrInvalidAddress = errors.New("invalid address")
	// ErrVerificationUnavailable identifies a provider or timeout failure in strict mode.
	ErrVerificationUnavailable = errors.New("address verification unavailable")
)

// Mode defines what happens when the external provider is unavailable.
type Mode string

const (
	// ModeDisabled only normalizes and validates the user-supplied address.
	ModeDisabled Mode = "disabled"
	// ModeBestEffort persists a valid normalized address as unverified on provider failure.
	ModeBestEffort Mode = "best_effort"
	// ModeStrict rejects the write when the provider cannot verify the CEP.
	ModeStrict Mode = "strict"
)

// ParseMode validates a configured verification mode.
func ParseMode(value string) (Mode, error) {
	mode := Mode(strings.ToLower(strings.TrimSpace(value)))
	switch mode {
	case ModeDisabled, ModeBestEffort, ModeStrict:
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported address verification mode %q", value)
	}
}

// CanonicalAddress is the provider-neutral portion of an address returned by
// a postal-code authority. Number and user complement intentionally stay out:
// ViaCEP verifies a postal code, not an individual property entrance.
type CanonicalAddress struct {
	PostalCode   string
	Street       string
	Neighborhood string
	City         string
	State        string
	StateCode    string
}

// Provider abstracts ViaCEP (or a future paid/fallback provider).
type Provider interface {
	Lookup(ctx context.Context, postalCode string) (CanonicalAddress, error)
}

// ProcessorConfig controls availability behavior and the bounded in-memory cache.
type ProcessorConfig struct {
	Mode            Mode
	Timeout         time.Duration
	CacheTTL        time.Duration
	CacheMaxEntries int
	Now             func() time.Time
	OnFallback      func(context.Context, error)
}

type cacheEntry struct {
	result    CanonicalAddress
	notFound  bool
	expiresAt time.Time
}

// Processor applies deterministic normalization and optional provider verification.
// It is safe for concurrent use by HTTP handlers.
type Processor struct {
	provider Provider
	config   ProcessorConfig

	mu    sync.Mutex
	cache map[string]cacheEntry
}

// NewProcessor constructs a processor. Enabled modes require a provider;
// disabled mode never performs external I/O.
func NewProcessor(provider Provider, config ProcessorConfig) (*Processor, error) {
	if config.Mode == "" {
		config.Mode = ModeBestEffort
	}
	if _, err := ParseMode(string(config.Mode)); err != nil {
		return nil, err
	}
	if config.Mode != ModeDisabled && provider == nil {
		return nil, errors.New("address verification provider is required when verification is enabled")
	}
	if config.Timeout <= 0 {
		return nil, errors.New("address verification timeout must be greater than zero")
	}
	if config.CacheTTL <= 0 {
		return nil, errors.New("address verification cache TTL must be greater than zero")
	}
	if config.CacheMaxEntries <= 0 {
		return nil, errors.New("address verification cache max entries must be greater than zero")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.OnFallback == nil {
		config.OnFallback = func(ctx context.Context, err error) {
			slog.WarnContext(ctx, "address verification provider unavailable; using normalized input", "error", err)
		}
	}

	return &Processor{
		provider: provider,
		config:   config,
		cache:    make(map[string]cacheEntry),
	}, nil
}

// Process normalizes an address, optionally verifies its Brazilian CEP and
// mutates it to the canonical representation that should be persisted.
func (p *Processor) Process(ctx context.Context, value *address.Address) error {
	if value == nil {
		return fmt.Errorf("%w: address is required", ErrInvalidAddress)
	}

	if err := Normalize(value); err != nil {
		return err
	}

	// Never trust an API client's is_verified flag. Only a successful provider
	// lookup in this process may set it to true.
	value.IsVerified = false

	// ViaCEP only covers Brazil. International addresses are still normalized
	// and validated, but stay explicitly unverified in every mode.
	if p.config.Mode == ModeDisabled || value.CountryCode != "BR" {
		return validateNormalized(value)
	}

	result, err := p.lookup(ctx, value.PostalCode)
	if err == nil {
		applyCanonical(value, result)
		value.IsVerified = true
		return validateNormalized(value)
	}

	// A definitive "not found" is a validation failure, not an availability
	// failure, so best_effort must not allow it through.
	if errors.Is(err, ErrPostalCodeNotFound) {
		return err
	}

	if p.config.Mode == ModeStrict {
		return fmt.Errorf("%w: %v", ErrVerificationUnavailable, err)
	}

	// In best_effort mode an outage may degrade verification, but it may not
	// degrade required-field validation or allow a forged verified flag.
	if validationErr := validateNormalized(value); validationErr != nil {
		return validationErr
	}
	p.config.OnFallback(ctx, err)
	return nil
}

func (p *Processor) lookup(ctx context.Context, postalCode string) (CanonicalAddress, error) {
	if cached, ok := p.cacheGet(postalCode); ok {
		if cached.notFound {
			return CanonicalAddress{}, fmt.Errorf("%w: %s", ErrPostalCodeNotFound, postalCode)
		}
		return cached.result, nil
	}

	lookupCtx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	result, err := p.provider.Lookup(lookupCtx, postalCode)
	if err != nil {
		if errors.Is(err, ErrPostalCodeNotFound) {
			p.cachePut(postalCode, cacheEntry{notFound: true})
		}
		return CanonicalAddress{}, err
	}
	p.cachePut(postalCode, cacheEntry{result: result})
	return result, nil
}

func (p *Processor) cacheGet(key string) (cacheEntry, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	entry, ok := p.cache[key]
	if !ok {
		return cacheEntry{}, false
	}
	if !p.config.Now().Before(entry.expiresAt) {
		delete(p.cache, key)
		return cacheEntry{}, false
	}
	return entry, true
}

func (p *Processor) cachePut(key string, entry cacheEntry) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.config.Now()
	for cachedKey, cached := range p.cache {
		if !now.Before(cached.expiresAt) {
			delete(p.cache, cachedKey)
		}
	}
	if _, exists := p.cache[key]; !exists && len(p.cache) >= p.config.CacheMaxEntries {
		// Bounded memory matters more than a sophisticated eviction policy here;
		// evict the entry that expires first.
		var oldestKey string
		var oldestExpiry time.Time
		for cachedKey, cached := range p.cache {
			if oldestKey == "" || cached.expiresAt.Before(oldestExpiry) {
				oldestKey = cachedKey
				oldestExpiry = cached.expiresAt
			}
		}
		delete(p.cache, oldestKey)
	}
	entry.expiresAt = now.Add(p.config.CacheTTL)
	p.cache[key] = entry
}

// Normalize produces a stable representation without making external calls.
func Normalize(value *address.Address) error {
	if value == nil {
		return fmt.Errorf("%w: address is required", ErrInvalidAddress)
	}

	value.Street = normalizeText(value.Street)
	value.Number = normalizeText(value.Number)
	value.Complement = normalizeText(value.Complement)
	value.Neighborhood = normalizeText(value.Neighborhood)
	value.District = normalizeText(value.District)
	value.City = normalizeText(value.City)
	value.State = normalizeText(value.State)
	value.StateCode = strings.ToUpper(normalizeText(value.StateCode))
	value.Country = normalizeText(value.Country)
	value.CountryCode = strings.ToUpper(normalizeText(value.CountryCode))
	value.Longitude = strings.TrimSpace(value.Longitude)
	value.Latitude = strings.TrimSpace(value.Latitude)
	value.ReferencePoint = normalizeText(value.ReferencePoint)

	if value.CountryCode == "" && (value.Country == "" || isBrazilName(value.Country)) {
		value.CountryCode = "BR"
	}
	if value.CountryCode == "BR" {
		value.Country = "Brasil"
		postalCode, err := normalizePostalCode(value.PostalCode)
		if err != nil {
			return err
		}
		value.PostalCode = postalCode
	} else {
		// The domain can store international properties even though the current
		// provider cannot verify them. Preserve their postal-code semantics.
		value.PostalCode = strings.ToUpper(normalizeText(value.PostalCode))
	}
	value.FormattedAddress = formatAddress(value)
	return nil
}

func normalizePostalCode(value string) (string, error) {
	var digits strings.Builder
	for _, r := range strings.TrimSpace(value) {
		switch {
		case unicode.IsDigit(r) && r <= unicode.MaxASCII:
			digits.WriteRune(r)
		case unicode.IsSpace(r), r == '-', r == '.':
			// Common human-readable separators are discarded.
		default:
			return "", fmt.Errorf("%w: CEP must contain exactly eight digits", ErrInvalidPostalCode)
		}
	}
	if digits.Len() != 8 {
		return "", fmt.Errorf("%w: CEP must contain exactly eight digits", ErrInvalidPostalCode)
	}
	return digits.String(), nil
}

func normalizeText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func isBrazilName(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "brasil", "brazil":
		return true
	default:
		return false
	}
}

func applyCanonical(value *address.Address, canonical CanonicalAddress) {
	value.PostalCode = canonical.PostalCode
	// Some valid Brazilian CEPs identify only a city and return no street or
	// neighborhood. Keep the user's required detail when the authority has no
	// more-specific value.
	if canonical.Street != "" {
		value.Street = canonical.Street
	}
	if canonical.Neighborhood != "" {
		value.Neighborhood = canonical.Neighborhood
	}
	if canonical.City != "" {
		value.City = canonical.City
	}
	if canonical.State != "" {
		value.State = canonical.State
	}
	if canonical.StateCode != "" {
		value.StateCode = canonical.StateCode
	}
	value.Country = "Brasil"
	value.CountryCode = "BR"
	value.FormattedAddress = formatAddress(value)
}

func formatAddress(value *address.Address) string {
	parts := make([]string, 0, 4)
	streetAndNumber := value.Street
	if value.Number != "" {
		if streetAndNumber != "" {
			streetAndNumber += ", "
		}
		streetAndNumber += value.Number
	}
	if streetAndNumber != "" {
		parts = append(parts, streetAndNumber)
	}
	if value.Complement != "" {
		parts = append(parts, value.Complement)
	}
	if value.Neighborhood != "" {
		parts = append(parts, value.Neighborhood)
	}
	cityState := value.City
	if value.StateCode != "" {
		if cityState != "" {
			cityState += " - "
		}
		cityState += value.StateCode
	}
	if cityState != "" {
		parts = append(parts, cityState)
	}
	formatted := strings.Join(parts, ", ")
	if value.CountryCode == "BR" && len(value.PostalCode) == 8 {
		cep := value.PostalCode[:5] + "-" + value.PostalCode[5:]
		if formatted != "" {
			formatted += ", "
		}
		formatted += "CEP " + cep
	} else if value.PostalCode != "" {
		if formatted != "" {
			formatted += ", "
		}
		formatted += value.PostalCode
	}
	return formatted
}

func validateNormalized(value *address.Address) error {
	if err := value.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidAddress, err)
	}
	return nil
}

// IsValidationError reports whether an error should become a client-side 400.
func IsValidationError(err error) bool {
	return errors.Is(err, ErrInvalidPostalCode) ||
		errors.Is(err, ErrPostalCodeNotFound) ||
		errors.Is(err, ErrInvalidAddress)
}

// IsUnavailable reports a strict-mode provider/timeout failure.
func IsUnavailable(err error) bool {
	return errors.Is(err, ErrVerificationUnavailable)
}
