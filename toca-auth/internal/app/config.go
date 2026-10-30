package app

import (
	"fmt"

	"github.com/oginoino/toca/pkg/config"
)

type Config struct {
	config.BaseConfig
	Redis       RedisConfig
	FrontendURL string
	BcryptCost  int
	// PasswordResetSigningKey assina o token_hash + validade do link de
	// redefinição. Sem ele, o link ainda depende apenas do TTL do Supabase.
	PasswordResetSigningKey string
	PasswordResetTTLMinutes int
}

type RedisConfig struct {
	URI string
}

func LoadAppConfig() *Config {
	fmt.Println("Loading app config...")
	return &Config{
		BaseConfig: config.LoadBaseConfig(),
		Redis: RedisConfig{
			URI: config.GetEnv("REDIS_URI", "redis://localhost:6379"),
		},
		FrontendURL: config.GetEnv("OPERATOR_FRONTEND_URL", config.GetEnv("FRONTEND_URL", "http://localhost:5173")),
		BcryptCost:  config.GetEnvInt("BCRYPT_COST", 12),
		PasswordResetSigningKey: config.GetEnv(
			"PASSWORD_RESET_SIGNING_KEY",
			config.GetEnv("INTERNAL_API_KEY", ""),
		),
		PasswordResetTTLMinutes: config.GetEnvInt("PASSWORD_RESET_TTL_MINUTES", 60),
	}
}
