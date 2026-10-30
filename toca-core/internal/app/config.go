package app

import (
	"fmt"
	"time"

	"github.com/oginoino/toca/pkg/config"
)

type Config struct {
	config.BaseConfig
	AddressVerification AddressVerificationConfig
}

// AddressVerificationConfig controls the provider-backed address gate used by
// property create/update operations.
type AddressVerificationConfig struct {
	Mode            string
	Provider        string
	BaseURL         string
	Timeout         time.Duration
	CacheTTL        time.Duration
	CacheMaxEntries int
}

func LoadAppConfig() *Config {
	fmt.Println("Loading app config...")
	return &Config{
		BaseConfig: config.LoadBaseConfig(),
		AddressVerification: AddressVerificationConfig{
			Mode:            config.GetEnv("ADDRESS_VERIFICATION_MODE", "best_effort"),
			Provider:        config.GetEnv("ADDRESS_VERIFICATION_PROVIDER", "viacep"),
			BaseURL:         config.GetEnv("ADDRESS_VERIFICATION_BASE_URL", "https://viacep.com.br"),
			Timeout:         config.GetEnvDuration("ADDRESS_VERIFICATION_TIMEOUT", 2*time.Second),
			CacheTTL:        config.GetEnvDuration("ADDRESS_VERIFICATION_CACHE_TTL", 24*time.Hour),
			CacheMaxEntries: config.GetEnvInt("ADDRESS_VERIFICATION_CACHE_MAX_ENTRIES", 10000),
		},
	}
}
