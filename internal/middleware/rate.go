package middleware

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/redis/go-redis/v9"
)

// RateMw applies named, shared Redis fixed-window policies to Hertz routes.
// The user dimension reads userId set by authentication middleware, never request input.
// Construct once at startup; UseRateMw selects a policy while registering a route.
type RateMw struct {
	policies map[string]conf.RateLimitPolicy
	trusted  []netip.Prefix
	counter  rateCounter
	now      func() time.Time
}

type rateCounter interface {
	incr(ctx context.Context, key string, window time.Duration) (int64, error)
}
type redisRateCounter struct{ client redis.UniversalClient }

// Each increment and its initial TTL execute together. Keys contain the policy,
// dimension and fixed-window start, so replicas share quotas and policies stay isolated.
var rateIncrScript = redis.NewScript(`
local current = redis.call('INCR', KEYS[1])
if current == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return current
`)

func (c redisRateCounter) incr(ctx context.Context, key string, window time.Duration) (int64, error) {
	return rateIncrScript.Run(ctx, c.client, []string{key}, window.Milliseconds()).Int64()
}

func NewRateMw(cfg *conf.Config, client redis.UniversalClient) (*RateMw, error) {
	if cfg == nil {
		return nil, fmt.Errorf("rate limit configuration is missing")
	}
	if err := cfg.RateLimit.Validate(); err != nil {
		return nil, err
	}
	trusted, err := cfg.RateLimit.TrustedProxies()
	if err != nil {
		return nil, err
	}
	policies := make(map[string]conf.RateLimitPolicy, len(cfg.RateLimit.Policies))
	for name, policy := range cfg.RateLimit.Policies {
		policies[name] = policy
	}
	mw := &RateMw{policies: policies, trusted: trusted, now: time.Now}
	if client != nil {
		mw.counter = redisRateCounter{client: client}
	}
	return mw, nil
}

// UseRateMw returns the handler for a configured policy. A missing name is a
// registration error and panics before the server starts accepting requests.
// Disabled dimensions do not read identity/IP or contact Redis.
func (m *RateMw) UseRateMw(policyName string) app.HandlerFunc {
	if m == nil {
		panic("rate limit middleware is not configured")
	}
	policy, ok := m.policies[policyName]
	if !ok {
		panic(fmt.Sprintf("rate limit policy %q is not configured", policyName))
	}
	return func(ctx context.Context, c *app.RequestContext) {
		if !policy.User.Enabled && !policy.IP.Enabled {
			c.Next(ctx)
			return
		}
		var userID, ip string
		if policy.User.Enabled {
			value, exists := c.Get("userId")
			uid, valid := anyToUint64(value)
			if !exists || !valid || uid == 0 {
				abortRate(c, 401, 2, "authentication required")
				return
			}
			userID = strconv.FormatUint(uid, 10)
		}
		if policy.IP.Enabled {
			ip = clientIP(c, m.trusted)
			if ip == "" {
				abortRate(c, 503, 12, "rate limit is temporarily unavailable")
				return
			}
		}
		if m.counter == nil {
			abortRate(c, 503, 12, "rate limit is temporarily unavailable")
			return
		}
		now := m.now()
		for _, dimension := range []struct {
			name, id string
			rule     conf.RateLimitRule
		}{
			{"user", userID, policy.User}, {"ip", ip, policy.IP},
		} {
			if !dimension.rule.Enabled {
				continue
			}
			window := time.Duration(dimension.rule.WindowSeconds) * time.Second
			bucket := now.Truncate(window)
			key := fmt.Sprintf("rate_limit:v1:%s:%s:%s:%d:%d", policyName, dimension.name, dimension.id, dimension.rule.WindowSeconds, bucket.UnixMilli())
			count, err := m.counter.incr(ctx, key, window)
			if err != nil {
				abortRate(c, 503, 12, "rate limit is temporarily unavailable")
				return
			}
			if count > dimension.rule.MaxRequests {
				remaining := bucket.Add(window).Sub(now)
				seconds := int64(remaining / time.Second)
				if remaining%time.Second != 0 {
					seconds++
				}
				if seconds < 1 {
					seconds = 1
				}
				c.Header("Retry-After", strconv.FormatInt(seconds, 10))
				abortRate(c, 429, 10, "too many requests")
				return
			}
		}
		c.Next(ctx)
	}
}
func abortRate(c *app.RequestContext, status int, code int32, message string) {
	c.AbortWithStatusJSON(status, struct {
		Code int32  `json:"code"`
		Msg  string `json:"msg"`
	}{code, message})
}

// Only a trusted immediate peer may supply the forwarded chain. With an empty
// allowlist, the socket peer is used and all forwarding headers are ignored.
func clientIP(c *app.RequestContext, trusted []netip.Prefix) string {
	remote := remoteIP(c)
	if remote == "" {
		return ""
	}
	addr, err := netip.ParseAddr(remote)
	if err != nil || !trustedAddress(addr, trusted) {
		return remote
	}
	var forwarded []string
	c.Request.Header.VisitAll(func(key, value []byte) {
		if strings.EqualFold(string(key), "X-Forwarded-For") {
			forwarded = append(forwarded, strings.Split(string(value), ",")...)
		}
	})
	return forwardedClientIP(remote, forwarded, trusted)
}
func remoteIP(c *app.RequestContext) string {
	addr := c.RemoteAddr()
	if addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		host = addr.String()
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return ""
	}
	return ip.Unmap().String()
}
func forwardedClientIP(remote string, forwarded []string, trusted []netip.Prefix) string {
	for i := len(forwarded) - 1; i >= 0; i-- {
		ip, err := netip.ParseAddr(strings.TrimSpace(forwarded[i]))
		// An invalid intermediary breaks the trust chain; never use values to its left.
		if err != nil {
			return remote
		}
		if !trustedAddress(ip, trusted) {
			return ip.Unmap().String()
		}
	}
	return remote
}
func trustedAddress(addr netip.Addr, trusted []netip.Prefix) bool {
	for _, prefix := range trusted {
		if prefix.Contains(addr.Unmap()) {
			return true
		}
	}
	return false
}
