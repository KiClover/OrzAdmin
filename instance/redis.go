package instance

import (
	"context"
	"crypto/tls"
	"strings"
	"time"

	"github.com/KiClover/orzAdmin/conf"
	internallog "github.com/KiClover/orzAdmin/internal/log"
	"github.com/redis/go-redis/v9"
)

// redisLogger 让 Redis 相关日志统一带 [REDIS] 前缀。
var redisLogger = internallog.WithModule(internallog.ModuleRedis)

func NewRedis(cfg *conf.Config, ctx context.Context) redis.UniversalClient {
	ctx, span := tracer.Start(ctx, "instance.NewRedis")
	defer span.End()
	opt := redis.UniversalOptions{
		Addrs:    strings.Split(cfg.RedisConf.Host, ","),
		DB:       cfg.RedisConf.DB,
		Username: cfg.RedisConf.User,
		Password: cfg.RedisConf.Password,
	}

	if cfg.RedisConf.Master != "" {
		opt.MasterName = cfg.RedisConf.Master
	}

	if cfg.RedisConf.TLS {
		opt.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	rds := redis.NewUniversalClient(&opt)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := rds.Ping(ctx).Err()

	if err != nil {
		redisLogger.Errorf("redis connect ping err: %v", err)
	}

	return rds
}
