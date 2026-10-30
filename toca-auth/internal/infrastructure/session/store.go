package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	redisclient "github.com/oginoino/toca-auth/internal/infrastructure/redis"
)

// Session represents a user session stored in Redis.
type Session struct {
	ID           string    `json:"id"`
	AccountID    string    `json:"account_id"`
	TenantID     string    `json:"tenant_id,omitempty"`
	TokenFamily  string    `json:"token_family"`
	DeviceInfo   string    `json:"device_info,omitempty"`
	IP           string    `json:"ip,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// Store manages sessions in Redis.
type Store struct {
	redis *redisclient.Client
	ttl   time.Duration
}

// NewStore creates a new session store.
func NewStore(redis *redisclient.Client, sessionTTL time.Duration) *Store {
	return &Store{redis: redis, ttl: sessionTTL}
}

func sessionKey(id string) string { return "session:" + id }
func accountKey(id string) string { return "sessions:" + id }
func familyKey(id string) string  { return "token_family:" + id }

// GenerateID creates a cryptographically random session/family ID.
func GenerateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Create persists a new session in Redis.
func (s *Store) Create(ctx context.Context, sess *Session) error {
	data, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	if err := s.redis.Set(ctx, sessionKey(sess.ID), data, s.ttl); err != nil {
		return err
	}
	if err := s.redis.SAdd(ctx, accountKey(sess.AccountID), sess.ID); err != nil {
		return err
	}
	return s.redis.Expire(ctx, accountKey(sess.AccountID), s.ttl)
}

// Get retrieves a session by ID.
func (s *Store) Get(ctx context.Context, id string) (*Session, error) {
	data, err := s.redis.Get(ctx, sessionKey(id))
	if err != nil {
		return nil, err
	}
	var sess Session
	if err := json.Unmarshal([]byte(data), &sess); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}
	return &sess, nil
}

// UpdateLastActive refreshes the session activity timestamp.
func (s *Store) UpdateLastActive(ctx context.Context, id string) error {
	sess, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	sess.LastActiveAt = time.Now()
	data, err := json.Marshal(sess)
	if err != nil {
		return err
	}
	remaining := time.Until(sess.ExpiresAt)
	if remaining <= 0 {
		return fmt.Errorf("session expired")
	}
	return s.redis.Set(ctx, sessionKey(id), data, remaining)
}

// Delete removes a single session.
func (s *Store) Delete(ctx context.Context, id string) error {
	sess, err := s.Get(ctx, id)
	if err != nil {
		// Session may already be gone — not an error.
		return nil
	}
	if err := s.redis.Delete(ctx, sessionKey(id)); err != nil {
		return err
	}
	return s.redis.SRem(ctx, accountKey(sess.AccountID), id)
}

// DeleteAllByAccountID removes all sessions for an account.
func (s *Store) DeleteAllByAccountID(ctx context.Context, accountID string) ([]string, error) {
	ids, err := s.redis.SMembers(ctx, accountKey(accountID))
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		_ = s.redis.Delete(ctx, sessionKey(id))
	}
	_ = s.redis.Delete(ctx, accountKey(accountID))
	return ids, nil
}

// ListByAccountID returns all active session IDs for an account.
func (s *Store) ListByAccountID(ctx context.Context, accountID string) ([]string, error) {
	return s.redis.SMembers(ctx, accountKey(accountID))
}

// SetFamilyLatestJTI stores the latest refresh token JTI for a token family (for reuse detection).
func (s *Store) SetFamilyLatestJTI(ctx context.Context, familyID, jti string, ttl time.Duration) error {
	return s.redis.Set(ctx, familyKey(familyID), jti, ttl)
}

// GetFamilyLatestJTI returns the latest refresh JTI for a token family.
func (s *Store) GetFamilyLatestJTI(ctx context.Context, familyID string) (string, error) {
	return s.redis.Get(ctx, familyKey(familyID))
}

// DeleteFamily removes a token family record.
func (s *Store) DeleteFamily(ctx context.Context, familyID string) error {
	return s.redis.Delete(ctx, familyKey(familyID))
}

// IncrInviteCounter increments a daily invitation counter and sets 24h TTL on first use.
func (s *Store) IncrInviteCounter(ctx context.Context, key string) (int64, error) {
	count, err := s.redis.Incr(ctx, key)
	if err != nil {
		return 0, err
	}
	// Set TTL only on first increment (count == 1)
	if count == 1 {
		_ = s.redis.Expire(ctx, key, 24*time.Hour)
	}
	return count, nil
}
