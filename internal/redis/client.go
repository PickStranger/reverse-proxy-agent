package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type Client struct {
	rdb *redis.Client
}

func NewClient(addr string) *Client {
	rdb := redis.NewClient(&redis.Options{
		Addr: addr,
	})
	return &Client{rdb: rdb}
}

func (c *Client) AddBlacklist(ctx context.Context, hash string, duration time.Duration) error {
	return c.rdb.Set(ctx, "blacklist:"+hash, "blocked", duration).Err()
}

func (c *Client) IsBlacklisted(ctx context.Context, hash string) (bool, error) {
	val, err := c.rdb.Get(ctx, "blacklist:"+hash).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return val == "blocked", nil
}
