package captcha

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisStore struct {
	client redis.UniversalClient
	prefix string
}

func NewRedisStore(client redis.UniversalClient, prefix string) *RedisStore {
	return &RedisStore{
		client: client,
		prefix: prefix,
	}
}

func (r *RedisStore) Set(id string, value string) error {
	key := r.prefix + id
	return r.client.Set(context.Background(), key, value, 1*time.Minute).Err()
}

func (r *RedisStore) Get(key string, clear bool) string {
	fullKey := r.prefix + key
	val, err := r.client.Get(context.Background(), fullKey).Result()
	if err != nil {
		return ""
	}
	if clear {
		r.client.Del(context.Background(), fullKey)
	}
	return val
}

func (r *RedisStore) Verify(id, answer string, clear bool) bool {
	saved := r.Get(id, clear)
	return saved == answer
}
