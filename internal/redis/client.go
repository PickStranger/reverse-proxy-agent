package redis

import (
	"context"
	"encoding/json"
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

// ─── 기존 블랙리스트 함수 (그대로 유지) ───────────────────────

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

// ─── 새로 추가: fingerprint 임시 저장 ─────────────────────────

// session_token 키로 fingerprint 저장 (TTL 60초)
func (c *Client) StoreFingerprint(ctx context.Context, sessionToken string, data interface{}, ttl time.Duration) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, "fp:"+sessionToken, b, ttl).Err()
}

// session_token 키로 fingerprint 조회
func (c *Client) GetFingerprint(ctx context.Context, sessionToken string, dest interface{}) error {
	val, err := c.rdb.Get(ctx, "fp:"+sessionToken).Result()
	if err == redis.Nil {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(val), dest)
}

// 역할 끝난 session_token 삭제
func (c *Client) DeleteFingerprint(ctx context.Context, sessionToken string) error {
	return c.rdb.Del(ctx, "fp:"+sessionToken).Err()
}
