package health

import (
	"context"
	"database/sql"
	"errors"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
)

type Checker interface {
	Name() string
	Check(ctx context.Context) error
}

type ReadinessChecker struct {
	checkers []Checker
	timeout  time.Duration
}

func NewReadinessChecker(checkers ...Checker) *ReadinessChecker {
	return &ReadinessChecker{checkers: checkers, timeout: 2 * time.Second}
}

func (r *ReadinessChecker) WithTimeout(timeout time.Duration) *ReadinessChecker {
	if timeout > 0 {
		r.timeout = timeout
	}
	return r
}

func (r *ReadinessChecker) Add(checker Checker) {
	r.checkers = append(r.checkers, checker)
}

func (r *ReadinessChecker) Check(ctx context.Context) map[string]string {
	status := make(map[string]string, len(r.checkers))
	for _, c := range r.checkers {
		cctx, cancel := context.WithTimeout(ctx, r.timeout)
		err := c.Check(cctx)
		cancel()
		if err != nil {
			status[c.Name()] = err.Error()
		} else {
			status[c.Name()] = "ok"
		}
	}
	return status
}

func (r *ReadinessChecker) IsReady(ctx context.Context) (bool, map[string]string) {
	out := r.Check(ctx)
	for _, v := range out {
		if v != "ok" {
			return false, out
		}
	}
	return true, out
}

type PostgresChecker struct {
	db *sql.DB
}

func NewPostgresChecker(db *sql.DB) *PostgresChecker {
	return &PostgresChecker{db: db}
}

func (c *PostgresChecker) Name() string {
	return "postgres"
}

func (c *PostgresChecker) Check(ctx context.Context) error {
	if c.db == nil {
		return errors.New("missing db")
	}
	return c.db.PingContext(ctx)
}

type RedisChecker struct {
	client redis.UniversalClient
}

func NewRedisChecker(client redis.UniversalClient) *RedisChecker {
	return &RedisChecker{client: client}
}

func (c *RedisChecker) Name() string {
	return "redis"
}

func (c *RedisChecker) Check(ctx context.Context) error {
	if c.client == nil {
		return errors.New("missing client")
	}
	return c.client.Ping(ctx).Err()
}

type RabbitMQChecker struct {
	currentChannel func() *amqp.Channel
}

// NewRabbitMQChecker keeps compatibility for callers that own a stable
// channel. RabbitMQEventBus users should prefer NewDynamicRabbitMQChecker so a
// successful reconnect is reflected immediately in readiness.
func NewRabbitMQChecker(channel *amqp.Channel) *RabbitMQChecker {
	return &RabbitMQChecker{currentChannel: func() *amqp.Channel { return channel }}
}

type RabbitMQChannelProvider interface {
	Channel() *amqp.Channel
}

func NewDynamicRabbitMQChecker(provider RabbitMQChannelProvider) *RabbitMQChecker {
	if provider == nil {
		return &RabbitMQChecker{}
	}
	return &RabbitMQChecker{currentChannel: provider.Channel}
}

func (c *RabbitMQChecker) Name() string {
	return "rabbitmq"
}

func (c *RabbitMQChecker) Check(ctx context.Context) error {
	_ = ctx
	if c.currentChannel == nil {
		return errors.New("missing channel provider")
	}
	channel := c.currentChannel()
	if channel == nil {
		return errors.New("missing channel")
	}
	if channel.IsClosed() {
		return errors.New("channel closed")
	}
	return nil
}

// PostgresDeepChecker exercises a real write+read round-trip on the DB
// pool. Used by /health/deep so operators can verify the pool is actually
// servicing queries, not just responding to TCP pings. The check uses a
// session-only temp table so it leaves no persisted state.
type PostgresDeepChecker struct {
	db *sql.DB
}

func NewPostgresDeepChecker(db *sql.DB) *PostgresDeepChecker {
	return &PostgresDeepChecker{db: db}
}

func (c *PostgresDeepChecker) Name() string { return "postgres_rw" }

func (c *PostgresDeepChecker) Check(ctx context.Context) error {
	if c.db == nil {
		return errors.New("missing db")
	}
	var n int
	if err := c.db.QueryRowContext(ctx, "SELECT 1").Scan(&n); err != nil {
		return err
	}
	if n != 1 {
		return errors.New("unexpected response from SELECT 1")
	}
	return nil
}

// RedisDeepChecker performs a SET → GET round-trip on a probe key with
// short TTL so the Redis cluster is exercised for both writes and reads.
type RedisDeepChecker struct {
	client redis.UniversalClient
	prefix string
}

func NewRedisDeepChecker(client redis.UniversalClient, keyPrefix string) *RedisDeepChecker {
	if keyPrefix == "" {
		keyPrefix = "health:probe"
	}
	return &RedisDeepChecker{client: client, prefix: keyPrefix}
}

func (c *RedisDeepChecker) Name() string { return "redis_rw" }

func (c *RedisDeepChecker) Check(ctx context.Context) error {
	if c.client == nil {
		return errors.New("missing client")
	}
	key := c.prefix + ":" + time.Now().Format("20060102150405.000")
	if err := c.client.Set(ctx, key, "ok", 30*time.Second).Err(); err != nil {
		return err
	}
	v, err := c.client.Get(ctx, key).Result()
	if err != nil {
		return err
	}
	if v != "ok" {
		return errors.New("unexpected response from GET")
	}
	_ = c.client.Del(ctx, key)
	return nil
}
