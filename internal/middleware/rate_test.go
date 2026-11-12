package middleware

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/network"
)

// Only the shared Redis boundary is replaced; the middleware and HTTP chain are real.
type rateTestCounter struct {
	mu     sync.Mutex
	values map[string]int64
	err    error
}

func (s *rateTestCounter) incr(_ context.Context, key string, _ time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return 0, s.err
	}
	if s.values == nil {
		s.values = make(map[string]int64)
	}
	s.values[key]++
	return s.values[key], nil
}
func rateTestConfig(user, ip conf.RateLimitRule) *conf.Config {
	return &conf.Config{RateLimit: conf.RateLimitConf{Policies: map[string]conf.RateLimitPolicy{
		"redeem": {User: user, IP: ip}, "login": {IP: ip},
	}}}
}
func rateTestMiddleware(t *testing.T, cfg *conf.Config, counter *rateTestCounter) *RateMw {
	t.Helper()
	mw, err := NewRateMw(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	mw.counter = counter
	mw.now = func() time.Time { return time.Unix(120, 100_000_000) }
	return mw
}

type rateTestConn struct {
	network.Conn
	addr net.Addr
}

func (c rateTestConn) RemoteAddr() net.Addr { return c.addr }
func rateTestServer(mw *RateMw, policy string) *server.Hertz {
	h := server.New(server.WithHostPorts("127.0.0.1:0"))
	h.GET("/test", func(ctx context.Context, c *app.RequestContext) {
		if uid := string(c.GetHeader("Test-User")); uid != "" {
			c.Set("userId", uid)
		}
		ip := string(c.GetHeader("Test-Peer"))
		if ip == "" {
			ip = "203.0.113.10"
		}
		c.SetConn(rateTestConn{Conn: c.GetConn(), addr: &net.TCPAddr{IP: net.ParseIP(ip), Port: 1234}})
		c.Next(ctx)
	}, mw.UseRateMw(policy), func(_ context.Context, c *app.RequestContext) { c.String(200, "business reached") })
	return h
}
func rateRequest(h *server.Hertz, user, peer, xff string) *ut.ResponseRecorder {
	return ut.PerformRequest(h.Engine, http.MethodGet, "/test", nil,
		ut.Header{Key: "Test-User", Value: user}, ut.Header{Key: "Test-Peer", Value: peer}, ut.Header{Key: "X-Forwarded-For", Value: xff})
}
func TestRateMwOptionalDimensions(t *testing.T) {
	for _, tc := range []struct {
		name     string
		user, ip conf.RateLimitRule
		want     []int
	}{
		{name: "both disabled", want: []int{200, 200, 200}},
		{name: "user only", user: conf.RateLimitRule{Enabled: true, WindowSeconds: 600, MaxRequests: 2}, want: []int{200, 200, 429}},
		{name: "ip only", ip: conf.RateLimitRule{Enabled: true, WindowSeconds: 60, MaxRequests: 1}, want: []int{200, 429, 429}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			counter := &rateTestCounter{}
			mw := rateTestMiddleware(t, rateTestConfig(tc.user, tc.ip), counter)
			h := rateTestServer(mw, "redeem")
			for i, want := range tc.want {
				r := rateRequest(h, "7001", "", "")
				if r.Code != want {
					t.Fatalf("request %d: got %d %s want %d", i, r.Code, r.Body.String(), want)
				}
			}
			for key := range counter.values {
				if !tc.ip.Enabled && strings.Contains(key, ":ip:") {
					t.Fatalf("disabled IP counted: %s", key)
				}
				if !tc.user.Enabled && strings.Contains(key, ":user:") {
					t.Fatalf("disabled user counted: %s", key)
				}
			}
		})
	}
}
func TestRateMwDisabledNeedsNeitherRedisNorIdentity(t *testing.T) {
	mw, err := NewRateMw(rateTestConfig(conf.RateLimitRule{}, conf.RateLimitRule{}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if r := rateRequest(rateTestServer(mw, "redeem"), "", "", ""); r.Code != 200 {
		t.Fatalf("disabled: %d %s", r.Code, r.Body.String())
	}
}
func TestRateMwUsesIndependentWindowsAndLimits(t *testing.T) {
	counter := &rateTestCounter{}
	cfg := rateTestConfig(conf.RateLimitRule{Enabled: true, WindowSeconds: 600, MaxRequests: 3}, conf.RateLimitRule{Enabled: true, WindowSeconds: 60, MaxRequests: 1})
	mw := rateTestMiddleware(t, cfg, counter)
	h := rateTestServer(mw, "redeem")
	for i, want := range []int{200, 429, 200, 429, 200} {
		if i == 2 {
			mw.now = func() time.Time { return time.Unix(180, 100_000_000) }
		}
		if i == 3 {
			mw.now = func() time.Time { return time.Unix(240, 100_000_000) }
		}
		if i == 4 {
			mw.now = func() time.Time { return time.Unix(600, 100_000_000) }
		}
		r := rateRequest(h, "7001", "", "")
		if r.Code != want {
			t.Fatalf("request %d got %d want %d: %s", i, r.Code, want, r.Body.String())
		}
		if i == 1 && string(r.Header().Peek("Retry-After")) != "60" {
			t.Fatalf("ceil Retry-After: %s", r.Header().Peek("Retry-After"))
		}
	}
}
func TestRateMwSharesPolicyAcrossInstancesAndSeparatesPolicies(t *testing.T) {
	cfg := rateTestConfig(conf.RateLimitRule{}, conf.RateLimitRule{Enabled: true, WindowSeconds: 60, MaxRequests: 1})
	counter := &rateTestCounter{}
	one := rateTestMiddleware(t, cfg, counter)
	two := rateTestMiddleware(t, cfg, counter)
	for i, tc := range []struct {
		mw     *RateMw
		policy string
		want   int
	}{{one, "redeem", 200}, {two, "redeem", 429}, {two, "login", 200}} {
		r := rateRequest(rateTestServer(tc.mw, tc.policy), "", "", "")
		if r.Code != tc.want {
			t.Fatalf("request %d got %d want %d", i, r.Code, tc.want)
		}
	}
}
func TestRateMwIPLimitSharedAcrossUsers(t *testing.T) {
	mw := rateTestMiddleware(t, rateTestConfig(conf.RateLimitRule{}, conf.RateLimitRule{Enabled: true, WindowSeconds: 60, MaxRequests: 1}), &rateTestCounter{})
	h := rateTestServer(mw, "redeem")
	for i, user := range []string{"7001", "7002"} {
		want := 200
		if i == 1 {
			want = 429
		}
		if r := rateRequest(h, user, "", ""); r.Code != want {
			t.Fatalf("user %s: %d want %d", user, r.Code, want)
		}
	}
}
func TestRateMwMissingUserFailsBeforeCounting(t *testing.T) {
	counter := &rateTestCounter{}
	mw := rateTestMiddleware(t, rateTestConfig(conf.RateLimitRule{Enabled: true, WindowSeconds: 60, MaxRequests: 1}, conf.RateLimitRule{}), counter)
	h := rateTestServer(mw, "redeem")
	for _, user := range []string{"", "0", "-1", "garbage"} {
		if r := rateRequest(h, user, "", ""); r.Code != 401 {
			t.Fatalf("user %q got %d", user, r.Code)
		}
	}
	if len(counter.values) != 0 {
		t.Fatal("unauthenticated request counted")
	}
}
func TestRateMwRedisFailureStopsBusinessAndHidesError(t *testing.T) {
	cfg := rateTestConfig(conf.RateLimitRule{Enabled: true, WindowSeconds: 60, MaxRequests: 1}, conf.RateLimitRule{})
	for _, counter := range []*rateTestCounter{nil, {err: errors.New("redis secret internal host")}} {
		mw := rateTestMiddleware(t, cfg, counter)
		if counter == nil {
			mw.counter = nil
		}
		r := rateRequest(rateTestServer(mw, "redeem"), "7001", "", "")
		if r.Code != 503 || strings.Contains(r.Body.String(), "secret") || strings.Contains(r.Body.String(), "business reached") {
			t.Fatalf("unsafe fail-open/error: %d %s", r.Code, r.Body.String())
		}
	}
}
func TestRateMwUnknownPolicyFailsAtRegistration(t *testing.T) {
	mw := rateTestMiddleware(t, rateTestConfig(conf.RateLimitRule{}, conf.RateLimitRule{}), nil)
	defer func() {
		if recover() == nil {
			t.Fatal("unknown policy must fail during registration")
		}
	}()
	mw.UseRateMw("typo")
}
func TestRateMwTrustedProxyAndSpoofedHeaders(t *testing.T) {
	cfg := rateTestConfig(conf.RateLimitRule{}, conf.RateLimitRule{Enabled: true, WindowSeconds: 60, MaxRequests: 1})
	cfg.RateLimit.TrustedProxyCIDRs = []string{"10.0.0.0/8"}
	mw := rateTestMiddleware(t, cfg, &rateTestCounter{})
	h := rateTestServer(mw, "redeem")
	cases := []struct {
		peer, xff string
		want      int
	}{
		{"10.0.0.2", "fake, 198.51.100.1, 10.0.0.3", 200},
		{"10.0.0.2", "198.51.100.2", 200},
		{"10.0.0.2", "spoof, 198.51.100.1", 429},
		{"203.0.113.9", "198.51.100.3", 200},
		{"203.0.113.9", "198.51.100.4", 429},
	}
	for i, tc := range cases {
		r := rateRequest(h, "", tc.peer, tc.xff)
		if r.Code != tc.want {
			t.Fatalf("request %d got %d want %d", i, r.Code, tc.want)
		}
	}
}
func TestRateForwardedMalformedChainDoesNotTrustLeftSide(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	if got := forwardedClientIP("10.0.0.2", []string{"198.51.100.1", "garbage"}, trusted); got != "10.0.0.2" {
		t.Fatalf("malformed chain trusted spoofable value: %s", got)
	}
}
func TestRateMwConcurrentLimit(t *testing.T) {
	mw := rateTestMiddleware(t, rateTestConfig(conf.RateLimitRule{}, conf.RateLimitRule{Enabled: true, WindowSeconds: 60, MaxRequests: 5}), &rateTestCounter{})
	h := rateTestServer(mw, "redeem")
	var wg sync.WaitGroup
	results := make(chan int, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- rateRequest(h, "", "", "").Code }()
	}
	wg.Wait()
	close(results)
	successes := 0
	for code := range results {
		if code == 200 {
			successes++
		} else if code != 429 {
			t.Fatal(fmt.Sprintf("unexpected status %d", code))
		}
	}
	if successes != 5 {
		t.Fatalf("concurrent accepted %d want 5", successes)
	}
}
