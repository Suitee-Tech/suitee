package repositories

import (
	"log"

	"github.com/redis/go-redis/v9"
)

func NewRedisClient(redisURI string) *redis.Client {
	opts, err := redis.ParseURL(redisURI)
	if err != nil {
		log.Printf("Failed to parse Redis URI: %v", err)
		return nil
	}
	return redis.NewClient(opts)
}
