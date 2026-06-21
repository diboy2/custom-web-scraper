package cache

import (
	"context"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultTTL = 10 * time.Minute

type Cache struct {
	client *redis.Client
	ttl    time.Duration
}

func New(addr string) *Cache {
	return &Cache{
		client: redis.NewClient(&redis.Options{Addr: addr}),
		ttl:    defaultTTL,
	}
}

func (c *Cache) Get(ctx context.Context, url string) (string, bool) {
	val, err := c.client.Get(ctx, url).Result()
	if err != nil {
		return "", false
	}
	return val, true
}

func (c *Cache) Set(ctx context.Context, url, html string) {
	if err := c.client.Set(ctx, url, html, c.ttl).Err(); err != nil {
		log.Printf("Cache set failed for %s: %v", url, err)
	}
}

func (c *Cache) Close() error {
	return c.client.Close()
}
