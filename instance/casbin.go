package instance

import (
	"context"
	"fmt"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/casbin/casbin/v2"
	entadapter "github.com/casbin/ent-adapter"
	rediswatcher "github.com/casbin/redis-watcher/v2"
	"github.com/redis/go-redis/v9"
)

func NewCasbin(cfg *conf.Config, ctx context.Context) *casbin.Enforcer {
	ctx, span := tracer.Start(ctx, "instance.NewCasbin")
	defer span.End()
	adapter, err := entadapter.NewAdapter("mysql", cfg.CasbinDatabaseConf.GetDSN())
	if err != nil {
		panic(fmt.Errorf("failed to create ent adapter: %v", err))
	}
	enforcer, err := casbin.NewEnforcer("etc/casbin.conf", adapter)
	if err != nil {
		panic(fmt.Errorf("failed to new casbin: %v", err))
	}

	// create redis watcher
	watcher, _ := rediswatcher.NewWatcher(cfg.RedisConf.Host, rediswatcher.WatcherOptions{
		Options: redis.Options{
			Password: cfg.RedisConf.Password,
		},
		Channel: "/casbin",
	})

	// set watcher
	err = enforcer.SetWatcher(watcher)
	if err != nil {
		panic(fmt.Errorf("failed to set watcher: %v", err))
	}
	err = watcher.SetUpdateCallback(func(msg string) {
		enforcer.LoadPolicy()
	})
	if err != nil {
		panic(fmt.Errorf("failed to set update callback: %v", err))
	}
	return enforcer
}
