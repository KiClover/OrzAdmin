//go:build integration

package middleware

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/redis/go-redis/v9"
)

func TestRateMwRedisSharedWindow(t *testing.T) {
	addr := os.Getenv("RATE_LIMIT_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set RATE_LIMIT_TEST_REDIS_ADDR to run Redis integration")
	}
	ctx := context.Background()
	one := redis.NewClient(&redis.Options{Addr: addr})
	two := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { one.Close(); two.Close() })
	if err := one.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	policy := fmt.Sprintf("ratetest_%d", time.Now().UnixNano())
	cfg := &conf.Config{RateLimit: conf.RateLimitConf{Policies: map[string]conf.RateLimitPolicy{policy: {
		User: conf.RateLimitRule{Enabled: true, WindowSeconds: 60, MaxRequests: 5},
		IP:   conf.RateLimitRule{Enabled: true, WindowSeconds: 60, MaxRequests: 100},
	}}}}
	first, err := NewRateMw(cfg, one)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewRateMw(cfg, two)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	first.now = func() time.Time { return now }
	second.now = first.now
	h1, h2 := rateTestServer(first, policy), rateTestServer(second, policy)
	var keys []string
	for _, bucket := range []time.Time{now.Truncate(time.Minute), now.Truncate(time.Minute).Add(time.Minute)} {
		keys = append(keys,
			fmt.Sprintf("rate_limit:v1:%s:user:7001:60:%d", policy, bucket.UnixMilli()),
			fmt.Sprintf("rate_limit:v1:%s:ip:203.0.113.10:60:%d", policy, bucket.UnixMilli()))
	}
	// Delete only this test's unique keys; shared developer Redis data is untouched.
	t.Cleanup(func() {
		if err := one.Del(ctx, keys...).Err(); err != nil {
			t.Error(err)
		}
	})
	var wg sync.WaitGroup
	results := make(chan int, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		h := h1
		if i%2 != 0 {
			h = h2
		}
		go func() { defer wg.Done(); results <- rateRequest(h, "7001", "", "").Code }()
	}
	wg.Wait()
	close(results)
	accepted := 0
	for status := range results {
		if status == 200 {
			accepted++
		} else if status != 429 {
			t.Fatalf("unexpected status: %d", status)
		}
	}
	if accepted != 5 {
		t.Fatalf("two instances accepted %d requests, want 5", accepted)
	}
	if count, err := one.Get(ctx, keys[0]).Int64(); err != nil || count != 20 {
		t.Fatalf("Redis user count=%d err=%v", count, err)
	}
	for _, key := range keys[:2] {
		ttl, err := one.PTTL(ctx, key).Result()
		if err != nil || ttl <= 0 || ttl > time.Minute {
			t.Fatalf("TTL=%v err=%v for %s", ttl, err, key)
		}
	}
	now = now.Add(time.Minute)
	for i := 0; i < 6; i++ {
		want := 200
		if i == 5 {
			want = 429
		}
		if got := rateRequest(h2, "7001", "", "").Code; got != want {
			t.Fatalf("next window request %d status=%d want=%d", i, got, want)
		}
	}
	// A real transport failure must stop the chain, without an in-memory fallback.
	if err := two.Close(); err != nil {
		t.Fatal(err)
	}
	if got := rateRequest(h2, "7002", "", "").Code; got != 503 {
		t.Fatalf("closed Redis status=%d want=503", got)
	}
}
