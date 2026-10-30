package session

import (
	"context"
	"time"

	redisclient "github.com/oginoino/toca-auth/internal/infrastructure/redis"
)

// Blacklist manages revoked token JTIs in Redis.
type Blacklist struct {
	redis *redisclient.Client
}

// NewBlacklist creates a new token blacklist.
func NewBlacklist(redis *redisclient.Client) *Blacklist {
	return &Blacklist{redis: redis}
}

func blacklistKey(jti string) string { return "blacklist:" + jti }

// Add blacklists a token JTI until its original expiration time.
func (b *Blacklist) Add(ctx context.Context, jti string, expiresAt time.Time) error {
	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		// Token already expired — no need to blacklist.
		return nil
	}
	return b.redis.Set(ctx, blacklistKey(jti), "1", ttl)
}

// IsBlacklisted checks if a JTI has been revoked.
func (b *Blacklist) IsBlacklisted(ctx context.Context, jti string) (bool, error) {
	if jti == "" {
		return false, nil
	}
	return b.redis.Exists(ctx, blacklistKey(jti))
}
